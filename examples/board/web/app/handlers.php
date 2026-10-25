<?php

declare(strict_types=1);

use Flowmark\Board\Assets;
use Flowmark\Board\Pages;
use Flowmark\Board\PostForm;
use Flowmark\Board\Posts;
use Polyspec\Hyper\Reply;
use Polyspec\Hyper\Request;
use Polyspec\Hyper\Result;

// Loaders and actions of the regions and routes that app.json declares.
return [
    // The post page, the one route with the parameter `id`, links the reader stylesheet (HY-64).
    'shared' => fn (Assets $assets, Request $request): array => [
        'assets' => $assets->urls(),
        'reader' => $request->param('id') !== null,
    ],

    'regions' => [
        'left' => fn (Request $request, Posts $posts): array => [
            'path' => $request->path(),
            'count' => $posts->count(),
        ],
    ],

    'routes' => [
        'board.list' => [
            'load' => function (Request $request, Posts $posts): array {
                [$page, $pages] = Pages::currentPage($request, $posts);

                return ['page' => $page, 'pages' => $pages, 'pager' => Pages::pagerWindow($page, $pages)];
            },
            'regions' => [
                // The notice is on the first page only; another page has no notice region (HY-75).
                'notice' => fn (Request $request, Posts $posts): ?array
                    => Pages::currentPage($request, $posts)[0] === 1 ? [
                        'notice' => ['text' => '게시판 예제입니다. 정렬과 공지 닫기는 서버 요청 없이 처리됩니다.', 'closed' => false],
                    ] : null,
                'rows' => function (Request $request, Posts $posts): array {
                    [$page] = Pages::currentPage($request, $posts);
                    // crudui renders the rows it is given and derives neither the page nor the total, so the loader
                    // pages the sorted list and the template's pager keeps the ?sort=&page= links.
                    $sort = match ($request->query()->last('sort') ?? '') {
                        'title' => 'title',
                        'author' => 'author',
                        default => '',
                    };

                    return [
                        'listHtml' => PostForm::renderList($posts->pageSorted($page, POSTS_PER_PAGE, $sort)),
                        'sort' => $sort,
                        'compact' => false,
                        'highlight' => $request->flash('created'),
                    ];
                },
            ],
        ],

        'board.show' => [
            'load' => fn (Request $request, Posts $posts): array => ['post' => Pages::findPost($request, $posts)],
            'regions' => [
                // The post page embeds its data, so the reader changes without a request; the list embeds none and
                // the browser requests its data when it first changes a region (HY-31, HY-92).
                'reader' => function (Request $request, Reply $reply, Posts $posts): array {
                    $reply->embedData();

                    return ['post' => Pages::findPost($request, $posts), 'large' => false];
                },
            ],
        ],

        'board.create' => [
            'load' => fn (Request $request): array => ['formHtml' => PostForm::render([], [], $request->csrfToken())],
            'post' => function (Request $request, Posts $posts): Result {
                // The crudui form posts its fields under the key prefix form; the server validates the same spec.
                $form = $_POST['form'] ?? null;
                if (!is_array($form)) {
                    return Result::invalid(['formHtml' => PostForm::render([], [], $request->csrfToken())]);
                }
                $values = [
                    'title' => trim((string) ($form['title'] ?? '')),
                    'author' => trim((string) ($form['author'] ?? '')),
                    'body' => trim((string) ($form['body'] ?? '')),
                ];
                $judged = PostForm::validate($values);
                if (!$judged['valid']) {
                    return Result::invalid([
                        'formHtml' => PostForm::render($values, $judged['errors'], $request->csrfToken()),
                    ]);
                }
                $id = $posts->create($values['title'], $values['author'], $values['body']);

                return Result::redirect('/board')->flash('created', $id)->changed('posts');
            },
        ],
    ],
];
