<?php

declare(strict_types=1);

namespace Flowmark\Board;

use Flowmark\Board\Model\People;
use Flowmark\Board\Model\Posts as PostModel;
use Polyspec\Orm\Db;
use Polyspec\Orm\OrmException;

/**
 * Stores board posts in a flowmark corpus, which orm reaches through the DSN `markdown://`.
 * The interface is that of hyper's board example, so its handlers run unchanged: an id is the
 * key of the post (`P1`), and a creation time is Unix seconds, read from the UTC datetime of the
 * column.
 */
final class Posts
{
    /** `$now` returns the creation time of a new post in Unix seconds. */
    public function __construct(private readonly Db $db, private readonly \Closure $now)
    {
    }

    /**
     * Returns one page of posts, newest first.
     *
     * @return list<array{id: string, title: string, author: string, created_at: int}>
     */
    public function page(int $page, int $perPage): array
    {
        return $this->pageSorted($page, $perPage, '');
    }

    /**
     * Returns one page of posts in the order the sort names: '' is the newest first, 'title' the titles in code point
     * order and 'author' the authors' names in code point order. A title is a column of the posts, so its order runs
     * in the request; an author's name lives in the joined people, which an order item of the request cannot name, so
     * the whole list is read and the page is cut after it is sorted.
     */
    public function pageSorted(int $page, int $perPage, string $sort): array
    {
        $query = (new PostModel())->connect($this->db)
            ->removeAllColumns()->addColumnId()->addColumnTitle()->addColumnCreated()
            ->joinAuthorWithId((new People())->removeAllColumns()->addColumnName());
        if ($sort === 'title') {
            $rows = $query->orderByTitleAsc()->orderByIdAsc()->getsPage($page, $perPage)->items->toArray();
        } elseif ($sort === 'author') {
            $rows = $query->orderByCreatedDesc()->orderByIdDesc()->gets()->toArray();
            usort($rows, static fn (array $a, array $b): int => [$a['people_model']['name'], $a['id']] <=> [$b['people_model']['name'], $b['id']]);
            $rows = array_slice($rows, ($page - 1) * $perPage, $perPage);
        } else {
            $rows = $query->orderByCreatedDesc()->orderByIdDesc()->getsPage($page, $perPage)->items->toArray();
        }

        $posts = [];
        foreach ($rows as $row) {
            $posts[] = [
                'id' => (string) $row['id'],
                'title' => (string) $row['title'],
                'author' => (string) $row['people_model']['name'],
                'created_at' => self::seconds($row['created']),
            ];
        }

        return $posts;
    }

    /**
     * Returns one post, or null when it does not exist.
     *
     * @return array{id: string, title: string, author: string, body: string, created_at: int}|null
     */
    public function find(string $id): ?array
    {
        try {
            $row = (new PostModel())->connect($this->db)
                ->joinAuthorWithId((new People())->removeAllColumns()->addColumnName())
                ->id($id)->get()->toArray();
        } catch (OrmException $e) {
            if ($e->code_ === 'NO_ROWS') {
                return null;
            }
            throw $e;
        }

        return [
            'id' => (string) $row['id'],
            'title' => (string) $row['title'],
            'author' => (string) $row['people_model']['name'],
            'body' => (string) $row['body'],
            'created_at' => self::seconds($row['created']),
        ];
    }

    /** Returns the number of posts. */
    public function count(): int
    {
        return (int) (new PostModel())->connect($this->db)->getCount();
    }

    /** Returns the number of pages for a page size. */
    public function pageCount(int $perPage): int
    {
        return intdiv($this->count() + $perPage - 1, $perPage);
    }

    /** Creates a post by the named author, who is created when the corpus has no person of that name, and returns its id. */
    public function create(string $title, string $author, string $body): string
    {
        return $this->db->transaction(function () use ($title, $author, $body): string {
            try {
                $authorId = (new People())->connect($this->db)->name($author)->get()->getId();
            } catch (OrmException $e) {
                if ($e->code_ !== 'NO_ROWS') {
                    throw $e;
                }
                $authorId = (new People())->connect($this->db)->setName($author)->create()->getId();
            }
            $created = (new \DateTimeImmutable('@' . ($this->now)()))->setTimezone(new \DateTimeZone('UTC'));
            $post = (new PostModel())->connect($this->db)
                ->setTitle($title)->setAuthor($authorId)->setBody($body)->setCreated($created)->create();

            return $post->getId();
        });
    }

    /** Reads the Unix seconds of a datetime column value that is `YYYY-MM-DD HH:MM:SS` in UTC. */
    private static function seconds(mixed $datetime): int
    {
        if ($datetime instanceof \DateTimeInterface) {
            return $datetime->getTimestamp();
        }

        return (new \DateTimeImmutable((string) $datetime, new \DateTimeZone('UTC')))->getTimestamp();
    }
}
