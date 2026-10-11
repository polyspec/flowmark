// Stores board posts in a flowmark corpus, which orm reaches through the DSN `markdown://`, as src/Posts.php does.
// The interface is that of hyper's board example, with promises since orm's client is asynchronous: an id is the key
// of the post (`P1`), and a creation time is Unix seconds, read from the UTC datetime of the column.
import { OrmError, type Db } from '@polyspec/orm';
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
    const result = await new PostModel().connect(this.db)
      .removeAllColumns().addColumnId().addColumnTitle().addColumnCreated()
      .joinAuthorWithId(new People().removeAllColumns().addColumnName())
      .orderByCreatedDesc().orderByIdDesc()
      .getsPage(page, perPage);
    return result.items.toArray().map((row: Record<string, unknown>) => ({
      id: String(row.id),
      title: String(row.title),
      author: String((row.people_model as Record<string, unknown>).name),
      created_at: seconds(row.created),
    }));
  }

  // Returns one post, or null when it does not exist.
  async find(id: string): Promise<Post | null> {
    try {
      const row = (await new PostModel().connect(this.db)
        .joinAuthorWithId(new People().removeAllColumns().addColumnName())
        .id(id).get()).toArray() as Record<string, unknown>;
      return {
        id: String(row.id),
        title: String(row.title),
        author: String((row.people_model as Record<string, unknown>).name),
        body: String(row.body),
        created_at: seconds(row.created),
      };
    } catch (error) {
      if (error instanceof OrmError && error.code === 'NO_ROWS') return null;
      throw error;
    }
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
      let authorId: string;
      try {
        authorId = (await new People().connect(this.db).name(author).get()).getId();
      } catch (error) {
        if (!(error instanceof OrmError) || error.code !== 'NO_ROWS') throw error;
        authorId = (await new People().connect(this.db).setName(author).create()).getId();
      }
      const created = new Date(this.now() * 1000);
      const post = await new PostModel().connect(this.db).setTitle(title).setAuthor(authorId).setBody(body).setCreated(created).create();
      return post.getId();
    });
  }
}
