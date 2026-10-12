<?php

declare(strict_types=1);

use Flowmark\Board\Assets;
use Flowmark\Board\PostForm;
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

// pagerWindow numbers the pager links: the first page, the last and a window of five around the current, with a gap
// marker between the ranges that are not adjacent, as crudui's pagination model windows its page buttons.
function pagerWindow(int $page, int $pages): array
{
    $out = [];
    $add = static function (int $n) use (&$out): void {
        $out[] = ['n' => $n];
    };
    $gap = static function () use (&$out): void {
        if (($last = $out[count($out) - 1] ?? null) === null || !isset($last['gap'])) {
            $out[] = ['gap' => true];
        }
    };
    for ($n = 1; $n <= $pages; $n++) {
        $inWindow = $n === 1 || $n === $pages || ($n >= $page - 2 && $n <= $page + 2);
        if ($inWindow) {
            $add($n);
        } elseif ($n === 2 || $n === $pages - 1) {
            $gap();
        }
    }
    return $out;
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

                return ['page' => $page, 'pages' => $pages, 'pager' => pagerWindow($page, $pages)];
            },
            'regions' => [
                // The notice is on the first page only; another page has no notice region (HY-75).
                'notice' => fn (Request $request, Posts $posts): ?array => currentPage($request, $posts)[0] === 1 ? [
                    'notice' => ['text' => '게시판 예제입니다. 정렬과 공지 닫기는 서버 요청 없이 처리됩니다.', 'closed' => false],
                ] : null,
                'rows' => function (Request $request, Posts $posts): array {
                    [$page] = currentPage($request, $posts);
                    // crudui renders the rows it is given and derives neither the page nor the total, so the loader
                    // slices the page and the template's pager keeps the ?page= links.
                    $postsOfPage = $posts->page($page, POSTS_PER_PAGE);
                    // The sort the region kept comes back on the request; the crudui list is rendered server-side.
                    $sort = $request->query('sort', '');
                    $key = match ($sort) { 'title' => 'title', 'author' => 'author', default => '' };
                    if ($key !== '') {
                        usort($postsOfPage, static fn (array $a, array $b): int => $a[$key] <=> $b[$key]);
                    }

                    return [
                        'listHtml' => PostForm::renderList($postsOfPage),
                        'sort' => $key,
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
                    return Result::invalid(['formHtml' => PostForm::render($values, $judged['errors'], $request->csrfToken())]);
                }
                $id = $posts->create($values['title'], $values['author'], $values['body']);

                return Result::redirect('/board')->flash('created', $id)->changed('posts');
            },
        ],
    ],
];
