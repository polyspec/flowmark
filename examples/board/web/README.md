<!-- doc-id: examples-board-web -->
# The board application

[한국어](README.ko.md)

A board of posts as a web application: [hyper](https://github.com/polyspec/hyper) renders the pages, and the posts
stand in a flowmark corpus that [orm](https://github.com/polyspec/orm) reaches through the DSN `markdown://`. The page
handlers and templates are hyper's board example; its data layer is replaced by `src/Posts.php`, which keeps the same
five methods (`page`, `find`, `count`, `pageCount`, `create`) over the orm models in `src/Model/`.

The corpus is sized for 1000 posts: `posts` is `store files shard created month`, so a month stands in its own
folder, `posts/2026-01/` to `posts/2026-10/`, 100 posts each.

## Run

The paths below are relative to the repository; `<flowmark>` is a built `flowmark` (`make flowmark-build`).

1. Make a corpus with the people and no posts, and install and seed it. The seed writes every post through the engine:

   ```sh
   mkdir var/board && cp -R examples/board/block/dbspec examples/board/block/people.flowmark.md var/board/
   <flowmark> install var/board
   go -C examples run ./board/seed ../var/board 2026 1 10 100
   ```

2. Serve the corpus; it prints the port, and the token stands in `var/board/.flowmark/server.json`:

   ```sh
   <flowmark> serve var/board --port 0
   ```

3. Install the application's dependencies and build its program and assets with hyper's tools (`composer install`
   and `npm install` in this folder; `hyper-build-server` and `hyper-build-assets` of the hyper repository), then
   start PHP with the DSN of the corpus:

   ```sh
   FLOWMARK_DSN='markdown://127.0.0.1:<port>/board?token=<token>' php -S 127.0.0.1:8090 -t public public/index.php
   ```

## Forms, lists and validation by crudui

The create form, the board list and the validation of a submission are declared once per face and
run by [crudui](https://github.com/polyspec/crudui): `src/PostForm.php` (the Composer package
`polyspec/crudui-generator`) and `node/postForm.ts` (npm file dependencies on the generator,
HTML renderer and validator packages) hold the same field specification — 제목, 작성자, 내용, each
with its rule and its Korean message — and the same list columns. The server renders the form with
`renderForm` (the action, the hidden CSRF token, the errors of a refused submission) and the list
with `renderList`; a POST is validated by the crudui validator, an empty one answering 422 with the
error of each field under it. The templates output the rendered HTML with `| raw`, the escape hatch
for HTML from a trusted renderer, and the crudui stylesheet is `public/assets/styles/crudui.css`
(copied from the crudui checkout; re-copy it when crudui is bumped). After editing the templates,
rebuild the server program and the assets with the `hyper-build-server` and `hyper-build-assets`
commands above. The list sort is a server request (`?sort=`), because the server renders the list.

## Run it on the Node.js server

The same application runs on hyper's Node.js server, over orm's TypeScript client: `node/` holds its `main.ts`,
`handlers.ts` and `posts.ts` (the same five methods, asynchronous) and the models in `node/model/`, generated from the
same catalog (in this folder, `node node_modules/@polyspec/orm/dist/bin/orm-gen.js gen --set ../block/dbspec/dbspec-set.json
--out node/model --scan node/posts.ts`; `--scan` makes it
declare the condition and join methods that `posts.ts` calls). After `npm install` in this folder and the steps 1 and 2
above, `npm run build:node` bundles the server, and the DSN of the corpus goes in the environment:

```sh
FLOWMARK_DSN='markdown://127.0.0.1:<port>/board?token=<token>' BOARD_SESSIONS=<session folder> BOARD_PORT=8091 node build/node/server.mjs
```

Both servers answer the same: the rows of every page of the list and the page of a post come out the same.

The models in `src/Model/` are generated from the corpus catalog by the orm-gen of the orm checkout beside flowmark (in
this folder, `php ../../../../orm/vendor-php/bin/orm-gen gen --set ../block/dbspec/dbspec-set.json --out src/Model
--namespace 'Flowmark\Board\Model'`); a catalog change that changes its hash needs them
generated again, and the server refuses a model of another hash (`SCHEMA_HASH_MISMATCH`).

## Measured at 1000 posts

On the development machine, with the corpus served by `flowmark serve`: a page of ten posts 8 ms through the data
layer and 35 ms as a page, one post 6 ms, a new post 48 ms.
