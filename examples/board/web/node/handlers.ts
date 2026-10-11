// Loaders and actions of the regions and routes that app.json declares, as app/handlers.php defines them.
import { NotFound, Result, type Handlers, type Request } from '@polyspec/hyper-server';
import type { Post, Posts } from './posts.js';

export interface Services {
  posts: Posts;
  assets: { hyper: string };
}

const POSTS_PER_PAGE = 10;

// Returns the requested page number limited to the existing pages, and the page count.
async function currentPage(request: Request, posts: Posts): Promise<[number, number]> {
  const pages = Math.max(1, await posts.pageCount(POSTS_PER_PAGE));
  return [Math.min(pages, Math.max(1, request.queryInt('page', 1))), pages];
}

// Returns the post of the route parameter `id`, or reports a missing resource.
async function findPost(request: Request, posts: Posts): Promise<Post> {
  const id = request.param('id') ?? '';
  // The key of a post is the prefix `P` and a number: the key_prefix of the posts table.
  const post = /^P[1-9][0-9]{0,15}$/.test(id) ? await posts.find(id) : null;
  if (post === null) throw new NotFound();
  return post;
}

// Removes the characters that PHP trim removes: space, tab, line feed, carriage return, NUL and vertical tab.
function trim(text: string): string {
  return text.replace(/^[ \t\n\r\0\x0B]+|[ \t\n\r\0\x0B]+$/g, '');
}

// Returns the number of characters, as PHP mb_strlen counts them.
function length(text: string): number {
  return [...text].length;
}

export const handlers: Handlers<Services> = {
  // The post page, the one route with the parameter `id`, links the reader stylesheet (HY-64).
  shared: ({ request, services }) => ({ assets: services.get('assets'), reader: request.param('id') !== null }),

  regions: {
    left: async ({ request, services }) => ({ path: request.path(), count: await services.get('posts').count() }),
  },

  routes: {
    'board.list': {
      load: async ({ request, services }) => {
        const [page, pages] = await currentPage(request, services.get('posts'));
        return { page, pages };
      },
      regions: {
        // The notice is on the first page only; another page has no notice region (HY-75).
        notice: async ({ request, services }) => ((await currentPage(request, services.get('posts')))[0] === 1 ? { notice: { text: '게시판 예제입니다. 정렬과 공지 닫기는 서버 요청 없이 처리됩니다.', closed: false } } : null),
        rows: async ({ request, services }) => {
          const posts = services.get('posts');
          const [page] = await currentPage(request, posts);
          return { posts: await posts.page(page, POSTS_PER_PAGE), sort: '', compact: false, highlight: request.flash('created') };
        },
      },
    },

    'board.show': {
      load: async ({ request, services }) => ({ post: await findPost(request, services.get('posts')) }),
      regions: {
        // The post page embeds its data, so the reader changes without a request; the list embeds none and the
        // browser requests its data when it first changes a region (HY-31, HY-92).
        reader: async ({ request, reply, services }) => {
          reply.embedData();
          return { post: await findPost(request, services.get('posts')), large: false };
        },
      },
    },

    'board.create': {
      post: async ({ request, services }) => {
        const values = { title: trim(request.formString('title')), author: trim(request.formString('author')), body: trim(request.formString('body')) };
        const errors: Record<string, string> = {};
        if (values.title === '') errors.title = '제목을 입력하세요.';
        else if (length(values.title) > 100) errors.title = '제목은 100자 이하로 입력하세요.';
        if (values.author === '') errors.author = '작성자를 입력하세요.';
        else if (length(values.author) > 30) errors.author = '작성자는 30자 이하로 입력하세요.';
        if (values.body === '') errors.body = '내용을 입력하세요.';
        if (Object.keys(errors).length > 0) return Result.invalid({ values, errors });
        const id = await services.get('posts').create(values.title, values.author, values.body);
        return Result.redirect('/board').flash('created', id).changed('posts');
      },
    },
  },
};
