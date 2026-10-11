<!-- doc-id: examples-board-web -->
<!-- source-sha256: 390ffad1180c2d73f89685a66f4259e0a0a9a6ee55dbfb818b9f03c2bf48449a -->
# 게시판 애플리케이션

[English](README.md)

게시글 게시판을 web 애플리케이션으로 만든 것이다. [hyper](https://github.com/polyspec/hyper)가 페이지를 렌더하고,
글은 [orm](https://github.com/polyspec/orm)이 DSN `markdown://`로 닿는 flowmark corpus에 있다. 페이지 handler와
template은 hyper의 board 예제이고, 그 data 계층을 `src/Posts.php`로 바꿨다. 같은 메서드 다섯 개(`page`,
`find`, `count`, `pageCount`, `create`)를 `src/Model/`의 orm model 위에 구현한다.

corpus는 글 1000개 규모로 잡았다. `posts`는 `store files shard created month`이므로 한 달이 자기 folder
`posts/2026-01/`부터 `posts/2026-10/`까지, 달마다 100개씩 선다.

## 실행

아래 경로는 저장소 기준이고 `<flowmark>`는 빌드한 `flowmark`(`make build`)다.

1. people만 있고 글이 없는 corpus를 만들어 install하고 시드한다. 시드는 모든 글을 엔진으로 쓴다:

   ```sh
   mkdir var/board && cp -R examples/board/block/dbspec examples/board/block/people.flowmark.md var/board/
   <flowmark> install var/board
   go run ./examples/board/seed var/board 2026 1 10 100
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

`src/Model/`의 model은 corpus catalog에서 생성한다(`orm-gen gen --out src/Model --namespace
'Flowmark\Board\Model' examples/board/block/dbspec/board.dbs`). hash를 바꾸는 catalog 변경은 model을 다시
생성해야 하며, server는 다른 hash의 model을 거절한다(`SCHEMA_HASH_MISMATCH`).

## 글 1000개에서 잰 값

개발 machine에서 corpus를 `flowmark serve`로 제공할 때: 글 열 개의 페이지는 data 계층 8 ms, 페이지로는
35 ms이고, 글 하나는 6 ms, 새 글은 48 ms다.
