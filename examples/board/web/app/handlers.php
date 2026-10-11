<?php

declare(strict_types=1);

use Flowmark\Board\Assets;
use Flowmark\Board\Posts;
use Polyspec\Hyper\NotFound;
use Polyspec\Hyper\Reply;
use Polyspec\Hyper\Request;
use Polyspec\Hyper\Result;

const POSTS_PER_PAGE = 10;

/**
 * Returns the requested page number limited to the existing pages, and the page count.
 *
 * @return array{int, int}
 */
function currentPage(Request $request, Posts $posts): array
{
    $pages = max(1, $posts->pageCount(POSTS_PER_PAGE));

    return [min($pages, max(1, $request->queryInt('page', 1))), $pages];
}

/**
 * Returns the post of the route parameter `id`, or reports a missing resource.
 *
 * @return array{id: string, title: string, author: string, body: string, created_at: int}
 */
function findPost(Request $request, Posts $posts): array
{
    $id = (string) $request->param('id');
    // The key of a post is the prefix `P` and a number: the key_prefix of the posts table.
    $post = preg_match('/^P[1-9]\d{0,15}$/D', $id) === 1 ? $posts->find($id) : null;

    return $post ?? throw new NotFound();
}

// Loaders and actions of the regions and routes that app.json declares.
return [
    // The post page, the one route with the parameter `id`, links the reader stylesheet (HY-64).
    'shared' => fn (Assets $assets, Request $request): array => ['assets' => $assets->urls(), 'reader' => $request->param('id') !== null],

    'regions' => [
        'left' => fn (Request $request, Posts $posts): array => [
            'path' => $request->path(),
            'count' => $posts->count(),
        ],
    ],

    'routes' => [
        'board.list' => [
            'load' => function (Request $request, Posts $posts): array {
                [$page, $pages] = currentPage($request, $posts);

                return ['page' => $page, 'pages' => $pages];
            },
            'regions' => [
                // The notice is on the first page only; another page has no notice region (HY-75).
                'notice' => fn (Request $request, Posts $posts): ?array => currentPage($request, $posts)[0] === 1 ? [
                    'notice' => ['text' => '게시판 예제입니다. 정렬과 공지 닫기는 서버 요청 없이 처리됩니다.', 'closed' => false],
                ] : null,
                'rows' => function (Request $request, Posts $posts): array {
                    [$page] = currentPage($request, $posts);

                    return [
                        'posts' => $posts->page($page, POSTS_PER_PAGE),
                        'sort' => '',
                        'compact' => false,
                        'highlight' => $request->flash('created'),
                    ];
                },
            ],
        ],

        'board.show' => [
            'load' => fn (Request $request, Posts $posts): array => ['post' => findPost($request, $posts)],
            'regions' => [
                // The post page embeds its data, so the reader changes without a request; the list embeds none and
                // the browser requests its data when it first changes a region (HY-31, HY-92).
                'reader' => function (Request $request, Reply $reply, Posts $posts): array {
                    $reply->embedData();

                    return ['post' => findPost($request, $posts), 'large' => false];
                },
            ],
        ],

        'board.create' => [
            'post' => function (Request $request, Posts $posts): Result {
                $values = [
                    'title' => trim($request->formString('title')),
                    'author' => trim($request->formString('author')),
                    'body' => trim($request->formString('body')),
                ];
                $errors = array_filter([
                    'title' => match (true) {
                        $values['title'] === '' => '제목을 입력하세요.',
                        mb_strlen($values['title']) > 100 => '제목은 100자 이하로 입력하세요.',
                        default => null,
                    },
                    'author' => match (true) {
                        $values['author'] === '' => '작성자를 입력하세요.',
                        mb_strlen($values['author']) > 30 => '작성자는 30자 이하로 입력하세요.',
                        default => null,
                    },
                    'body' => $values['body'] === '' ? '내용을 입력하세요.' : null,
                ]);
                if ($errors !== []) {
                    return Result::invalid(['values' => $values, 'errors' => $errors]);
                }
                $id = $posts->create($values['title'], $values['author'], $values['body']);

                return Result::redirect('/board')->flash('created', $id)->changed('posts');
            },
        ],
    ],
];
