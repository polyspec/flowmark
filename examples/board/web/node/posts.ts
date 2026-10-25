// Stores board posts in a flowmark corpus, which orm reaches through the DSN `markdown://`, as src/Posts.php does.
// The interface is that of hyper's board example, with promises since orm's client is asynchronous: an id is the key
// of the post (`P1`), and a creation time is Unix seconds, read from the UTC datetime of the column.
import type { Db } from '@polyspec/orm';
import { People, Posts as PostModel } from './model/models.js';

export interface PostRow {
  id: string;
  title: string;
  author: string;
  created_at: number;
}

export interface Post extends PostRow {
  body: string;
}

/** The name of the author that a row of posts joined with its person holds; the join of a post always holds it. */
function authorName(row: Record<string, unknown>): string {
  const person = row.people_model;
  if (typeof person === 'object' && person !== null && 'name' in person) return String(person.name);
  throw new Error(`the post ${String(row.id)} holds no joined author name`);
}

/** Writes Unix seconds as the `YYYY-MM-DD HH:MM:SS` UTC datetime of a column. */
function datetime(seconds: number): string {
  return new Date(seconds * 1000).toISOString().slice(0, 19).replace('T', ' ');
}

/** Reads the Unix seconds of a datetime column value that is `YYYY-MM-DD HH:MM:SS` in UTC. */
function seconds(datetime: unknown): number {
  if (datetime instanceof Date) return Math.floor(datetime.getTime() / 1000);
  return Math.floor(Date.parse(`${String(datetime).replace(' ', 'T').replace(/\.0+$/, '')}Z`) / 1000);
}

export class Posts {
  private readonly db: Db;
  private readonly now: () => number;

  // `now` returns the creation time of a new post in Unix seconds.
  constructor(db: Db, now: () => number) {
    this.db = db;
    this.now = now;
  }

  // Returns one page of posts, newest first.
  async page(page: number, perPage: number): Promise<PostRow[]> {
    return this.pageSorted(page, perPage, '');
  }

  // Returns one page of posts in the order the sort names: '' is the newest first, 'title' the titles in code point
  // order and 'author' the authors' names in code point order. A title is a column of the posts, so its order runs in
  // the request; an author's name lives in the joined people, which an order item of the request cannot name, so the
  // whole list is read and the page is cut after it is sorted.
  async pageSorted(page: number, perPage: number, sort: string): Promise<PostRow[]> {
    const query = new PostModel().connect(this.db)
      .removeAllColumns().addColumnId().addColumnTitle().addColumnCreated()
      .joinAuthorWithId(new People().removeAllColumns().addColumnName());
    let items: Record<string, unknown>[];
    if (sort === 'title') {
      items = (await query.orderByTitleAsc().orderByIdAsc().getsPage(page, perPage)).items.toArray();
    } else if (sort === 'author') {
      items = (await query.orderByCreatedDesc().orderByIdDesc().gets()).toArray();
      const nameOf = authorName;
      items.sort((a, b) => nameOf(a) < nameOf(b) ? -1 : nameOf(a) > nameOf(b) ? 1 : String(a.id) < String(b.id) ? -1 : 1);
      items = items.slice((page - 1) * perPage, page * perPage);
    } else {
      items = (await query.orderByCreatedDesc().orderByIdDesc().getsPage(page, perPage)).items.toArray();
    }
    const result = { items: { toArray: () => items } };
    return result.items.toArray().map((row: Record<string, unknown>) => ({
      id: String(row.id),
      title: String(row.title),
      author: authorName(row),
      created_at: seconds(row.created),
    }));
  }

  // Returns one post, or null when it does not exist.
  async find(id: string): Promise<Post | null> {
    // orm's get() is null when no row matches.
    const found = await new PostModel().connect(this.db)
      .joinAuthorWithId(new People().removeAllColumns().addColumnName())
      .id(id).get();
    if (found === null) return null;
    const row = found.toArray();
    return {
      id: String(row.id),
      title: String(row.title),
      author: authorName(row),
      body: String(row.body),
      created_at: seconds(row.created),
    };
  }

  // Returns the number of posts.
  async count(): Promise<number> {
    return Number(await new PostModel().connect(this.db).getCount());
  }

  // Returns the number of pages for a page size.
  async pageCount(perPage: number): Promise<number> {
    return Math.floor(((await this.count()) + perPage - 1) / perPage);
  }

  // Creates a post by the named author, who is created when the corpus has no person of that name, and returns its id.
  async create(title: string, author: string, body: string): Promise<string> {
    return this.db.transaction(async () => {
      const person = await new People().connect(this.db).name(author).get();
      const authorId = person === null ? (await new People().connect(this.db).setName(author).create()).getId() : person.getId();
      const post = await new PostModel().connect(this.db).setTitle(title).setAuthor(authorId).setBody(body).setCreated(datetime(this.now())).create();
      return post.getId();
    });
  }
}
