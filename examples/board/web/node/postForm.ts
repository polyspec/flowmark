// The post form and the post list of the Node.js face, rendered and validated by crudui with the same
// specification the PHP face uses (src/PostForm.php): the form declares each field with its rule and its
// Korean message, the list declares the columns the board shows. The compiled form template is cached for
// the process; an instance is created per request.
import { compileForm, createForm } from '@polyspec/crudui-generator-core';
import { renderForm, renderList } from '@polyspec/crudui-generator-html';
import { validate } from '@polyspec/crudui-validator';

const form = {
  type: 'group',
  properties: {
    title: {
      type: 'text', label: '제목', attrs: { maxlength: 100 },
      validate: { required: true, maxlength: 100 },
      messages: { required: '제목을 입력하세요.', maxlength: '제목은 100자 이하로 입력하세요.' },
    },
    author: {
      type: 'text', label: '작성자', attrs: { maxlength: 30 },
      validate: { required: true, maxlength: 30 },
      messages: { required: '작성자를 입력하세요.', maxlength: '작성자는 30자 이하로 입력하세요.' },
    },
    body: {
      type: 'textarea', label: '내용', attrs: { rows: 12 },
      validate: { required: true },
      messages: { required: '내용을 입력하세요.' },
    },
  },
} as const;

const list = {
  columns: {
    id: { field: 'id', label: '번호', format: 'text' },
    title: { field: 'title', label: '제목', format: { type: 'link', href: '/board/{=id}' } },
    author: { field: 'author', label: '작성자', format: 'text' },
    created: { field: 'created', label: '작성일', format: 'datetime' },
  },
} as const;

const template = compileForm(structuredClone(form));

export interface PostValues {
  title: string;
  author: string;
  body: string;
}

/** The judgement of the submitted values: whether they hold, and the error of every field that does not. */
export function judge(values: PostValues): { valid: boolean; errors: { path: string; message: string }[] } {
  const result = validate(structuredClone(form), values);
  return { valid: result.valid, errors: result.errors };
}

/** Renders the create form: the fields with the values they hold, their errors under them, and the CSRF token hidden. */
export function createFormHtml(values: Partial<PostValues>, errors: { path: string; message: string }[], csrf: string): string {
  return renderForm(createForm(template, values as Record<string, unknown>, { language: 'ko' }), {
    action: { method: 'post', url: '/board/create' },
    hidden: { _csrf: csrf },
    errors,
  });
}

/** Renders one page of posts as the board list. */
export function listHtml(posts: { id: string; title: string; author: string; created_at: number }[]): string {
  const rows = posts.map(post => ({
    id: post.id,
    title: post.title,
    author: post.author,
    created: new Date(post.created_at * 1000).toISOString().slice(0, 16).replace('T', ' '),
  }));
  return renderList(structuredClone(list), rows, { language: 'ko' });
}
