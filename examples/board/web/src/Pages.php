<?php

declare(strict_types=1);

namespace Flowmark\Board;

use Polyspec\Hyper\NotFound;
use Polyspec\Hyper\Request;

/** The paging of the post list and the post of a route, which the handlers of app/handlers.php read. */
final class Pages
{
    public const int POSTS_PER_PAGE = 10;

    /**
     * Returns the requested page number limited to the existing pages, and the page count.
     *
     * @return array{int, int}
     */
    public static function currentPage(Request $request, Posts $posts): array
    {
        $pages = max(1, $posts->pageCount(self::POSTS_PER_PAGE));

        return [min($pages, max(1, $request->queryInt('page', 1))), $pages];
    }

    /**
     * Returns the post of the route parameter `id`, or reports a missing resource.
     *
     * @return array{id: string, title: string, author: string, body: string, created_at: int}
     */
    public static function findPost(Request $request, Posts $posts): array
    {
        $id = (string) $request->param('id');
        // The key of a post is the prefix `P` and a number: the key_prefix of the posts table.
        $post = preg_match('/^P[1-9]\d{0,15}$/D', $id) === 1 ? $posts->find($id) : null;

        return $post ?? throw new NotFound();
    }

    // pagerWindow numbers the pager links: the first page, the last and a window of five around the current, with a gap
    // marker between the ranges that are not adjacent, as crudui's pagination model windows its page buttons.
    public static function pagerWindow(int $page, int $pages): array
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
}
