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

The paths below are relative to the repository; `<flowmark>` is a built `flowmark` (`make build`).

1. Make a corpus with the people and no posts, and install and seed it. The seed writes every post through the engine:

   ```sh
   mkdir var/board && cp -R examples/board/block/dbspec examples/board/block/people.flowmark.md var/board/
   <flowmark> install var/board
   go run ./examples/board/seed var/board 2026 1 10 100
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

## Run it on the Node.js server

The same application runs on hyper's Node.js server, over orm's TypeScript client: `node/` holds its `main.ts`,
`handlers.ts` and `posts.ts` (the same five methods, asynchronous) and the models in `node/model/`, generated from the
same catalog (`orm-gen gen --schema ../block/dbspec/board.dbs --out node/model --scan node/posts.ts`; `--scan` makes it
declare the condition and join methods that `posts.ts` calls). After `npm install` in this folder and the steps 1 and 2
above, `npm run build:node` bundles the server, and the DSN of the corpus goes in the environment:

```sh
FLOWMARK_DSN='markdown://127.0.0.1:<port>/board?token=<token>' BOARD_SESSIONS=<session folder> BOARD_PORT=8091 node build/node/server.mjs
```

Both servers answer the same: the rows of every page of the list and the page of a post come out the same.

The models in `src/Model/` are generated from the corpus catalog (`orm-gen gen --out src/Model --namespace
'Flowmark\Board\Model' examples/board/block/dbspec/board.dbs`); a catalog change that changes its hash needs them
generated again, and the server refuses a model of another hash (`SCHEMA_HASH_MISMATCH`).

## Measured at 1000 posts

On the development machine, with the corpus served by `flowmark serve`: a page of ten posts 8 ms through the data
layer and 35 ms as a page, one post 6 ms, a new post 48 ms.
