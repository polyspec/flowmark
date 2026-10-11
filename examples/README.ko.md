<!-- doc-id: examples -->
<!-- source-sha256: 26c55a44d895479abfe8446bac71c9243350a7d4f296c29be510fda3672d3248 -->
# Examples

[English](README.md)

엔진의 예제 corpus들이다. 각 folder가 corpus root이며, `dbspec/` catalog와 엔진이 쓴
`*.flowmark.md` 문서를 담는다. 모든 행은 엔진을 거쳐 쓰였고, 이 folder의 test
(`go test ./examples/`, `make test`에 포함)는 face의 operation만으로 corpus의 복사본을 읽고
바꾸므로 corpus는 valid하고 canonical한 상태를 유지한다.

어느 face로든 열 수 있다. 예: `flowmark rows examples/tracker/aggregate-list items`.

## The tracker in three storages

응용 schema(`flowmark.dbs`: `items`, `people`, `item_links`, `item_history`, `views`)의 issue
tracker가 dataset 하나 — 모든 state에 걸친 item 여섯 개, 그 history, link 두 개, 사람 두 명,
view 세 개 — 를 세 corpus에 담는다. 세 corpus는 `items`의 store setting 하나만 다르다.

| Corpus | `items` storage | item이 놓이는 곳 |
| --- | --- | --- |
| `tracker/aggregate-list` | `store document list` | `items.flowmark.md`의 list item 하나 |
| `tracker/aggregate-table` | `store document table` | `items.flowmark.md`의 table 행 하나 |
| `tracker/split-file` | `store files` | 자기 파일 `items/<key>.flowmark.md` |

item의 body, markdown column, history block은 두 aggregate corpus에서는 그 item의 detail
section이고, split corpus에서는 그 item 파일의 나머지 부분이다. 세 corpus는 모든 table에서 같은
행을 읽어 내며, test가 이를 확인한다.

## The board

`posts`(title, body, author, created)와 `comments`(body, author, created, 답글을 위한 `parent`)로
된 게시판이 markdown의 일대다 관계를 보인다. 같은 행을 담는 두 corpus가 있다(`board.dbs`).

| Corpus | `comments` storage | comment가 놓이는 곳 |
| --- | --- | --- |
| `board/block` | `store block fk_comments_post list` | 그 post 파일의 `## comments` 아래 list item 하나, 답글은 그 comment 아래 들여쓰기 |
| `board/files` | `store files` | 자기 파일 `comments/<key>.flowmark.md`, 그 field list가 post로 link한다 |

두 corpus 모두에서 post는 자기 파일 `posts/<key>.flowmark.md`이고, 모든 문서는 markdown viewer에서
평범한 게시판 페이지로 읽힌다: post, link로 된 field, comment. test는 두 corpus에서 엔진으로 post를
만들고, comment를 달고, 답글을 달고, 고치고 지우며, 단계마다 같은 행을 요구한다.
