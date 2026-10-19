<!-- doc-id: examples-board-web -->
<!-- source-sha256: 86126512497fb52fc41824ad851eeef61f5b60dbadbd4945fd515c48613164d8 -->
# 게시판 애플리케이션

[English](README.md)

게시글 게시판을 web 애플리케이션으로 만든 것이다. [hyper](https://github.com/polyspec/hyper)가 페이지를 렌더하고,
글은 [orm](https://github.com/polyspec/orm)이 DSN `markdown://`로 닿는 flowmark corpus에 있다. 페이지 handler와
template은 hyper의 board 예제이고, 그 data 계층을 `src/Posts.php`로 바꿨다. 같은 메서드 다섯 개(`page`,
`find`, `count`, `pageCount`, `create`)를 `src/Model/`의 orm model 위에 구현한다.

corpus는 글 1000개 규모로 잡았다. `posts`는 `store files shard created month`이므로 한 달이 자기 folder
`posts/2026-01/`부터 `posts/2026-10/`까지, 달마다 100개씩 선다.

## 실행

아래 경로는 저장소 기준이고 `<flowmark>`는 빌드한 `flowmark`(`make flowmark-build`)다.

1. people만 있고 글이 없는 corpus를 만들어 install하고 시드한다. 시드는 모든 글을 엔진으로 쓴다:

   ```sh
   mkdir var/board && cp -R examples/board/block/dbspec examples/board/block/people.flowmark.md var/board/
   <flowmark> install var/board
   go -C examples run ./board/seed ../var/board 2026 1 10 100
   ```

2. corpus를 serve한다. port를 출력하고 token은 `var/board/.flowmark/server.json`에 있다:

   ```sh
   <flowmark> serve var/board --port 0
   ```

3. 애플리케이션의 의존성을 설치하고(이 folder에서 `composer install`과 `npm install`) hyper repository의
   `hyper-build-server`와 `hyper-build-assets`로 program과 asset을 빌드한 뒤, corpus의 DSN으로 PHP를 시작한다:

   ```sh
   FLOWMARK_DSN='markdown://127.0.0.1:<port>/board?token=<token>' php -S 127.0.0.1:8090 -t public public/index.php
   ```

## crudui로 하는 form, 목록, 검증

글쓰기 form, board 목록, 제출의 검증은 얼굴마다 한 번 선언되고
[crudui](https://github.com/polyspec/crudui)로 돈다: `src/PostForm.php`(Composer path 저장소의
`polyspec/crudui-generator`)와 `node/postForm.ts`(generator, HTML renderer, validator package의
npm file 의존성)가 같은 field 명세 — 제목, 작성자, 내용, 각각 규칙과 한국어 message — 와 같은
목록 column을 가진다. server는 form을 `renderForm`으로(action, 숨은 CSRF token, 거절된 제출의
error와 함께), 목록을 `renderList`로 렌더하고 POST는 crudui validator로 검증하며, 빈 제출은
field마다 그 밑에 error를 남기며 422로 답한다. template은 신뢰하는 renderer의 HTML을 위한
탈출구인 `| raw`로 렌더된 HTML을 출력하고 crudui stylesheet는 `public/assets/styles/crudui.css`다
(crudui checkout에서 복사했다; crudui를 올리면 다시 복사한다). template을 고친 뒤에는 위의
`hyper-build-server`와 `hyper-build-assets` 명령으로 server program과 asset을 다시 만든다.
목록 정렬은 server가 목록을 렌더하므로 server 요청이다(`?sort=`).

## Node.js 서버에서 실행

같은 애플리케이션이 orm의 TypeScript client 위에서 hyper의 Node.js 서버로도 돈다. `node/`에 `main.ts`,
`handlers.ts`, `posts.ts`(같은 메서드 다섯 개, 비동기)와 같은 catalog에서 생성한 `node/model/`의 model이
있다(`orm-gen gen --set ../block/dbspec/dbspec-set.json --out node/model --scan node/posts.ts`. `--scan`은
`posts.ts`가 부르는 조건과 join 메서드를 선언하게 한다). 이 folder에서 `npm install`을 하고 위의 1, 2단계를
마친 뒤 `npm run build:node`로 서버를 번들하며, corpus의 DSN은 환경변수로 준다:

```sh
FLOWMARK_DSN='markdown://127.0.0.1:<port>/board?token=<token>' BOARD_SESSIONS=<session folder> BOARD_PORT=8091 node build/node/server.mjs
```

두 서버는 같게 답한다. 목록의 모든 페이지의 행과 글 하나의 페이지가 같게 나온다.

`src/Model/`의 model은 corpus catalog에서 생성한다(`orm-gen gen --set examples/board/block/dbspec/dbspec-set.json --out src/Model
--namespace 'Flowmark\Board\Model'`). hash를 바꾸는 catalog 변경은 model을 다시
생성해야 하며, server는 다른 hash의 model을 거절한다(`SCHEMA_HASH_MISMATCH`).

## 글 1000개에서 잰 값

개발 machine에서 corpus를 `flowmark serve`로 제공할 때: 글 열 개의 페이지는 data 계층 8 ms, 페이지로는
35 ms이고, 글 하나는 6 ms, 새 글은 48 ms다.
