<!-- doc-id: plans-markdown-database -->
<!-- source-sha256: 7d86324f96a5227c913f7e014c2626e0e6aa87d9f1c6f8cc8d4107d3687b3311 -->
# markdown database 계획

[English](markdown-database-plan.md)

`markdown-database-checklist.md`의 T12 항목들의 계획이다: markdown database의 저장 문법, 관계,
계층, view. 각 판정은 T12.1에서 해당 spec으로 옮겨진다. spec과 이 계획이 다르면 spec이 이기고 이
계획을 정정한다.

## 배경

칸반 desync 수정(T10-1~T10-4)에서 markdown database에 규칙이 아니라 사례만 있다는 것이 드러났다.

- **세 형태는 demo 사례였다.** list, table, detailed demo는 각자 어떤 규칙도 정의하지 않은 형태를
  키웠다. detailed demo의 `## T2: 제목` 섹션은 어떤 code도 읽지 않으며, 복제된 제목은 이미 항목과
  어긋났다(`작성1` 대 `작성`).
- **모델에 기본 요소가 없다.** 항목에는 body column, parent field, 관계, 이력 기록이 없다. 자식은
  ID 문자열로 추측되어 `MoveItem`은 자식을 남기고 `EditTitle`은 `@assignee`와 마감 token을 지운다.
- **계층 판정이 틀렸다.** design plan(§1)은 markdown executor를 orm repository에 두었지만 orm은
  database 위의 query layer이고, code도 그 판정을 따른 적이 없다. orm에는 dbspec setting 두 개가
  있을 뿐이고 flowmark는 orm에서 아무것도 import하지 않는다.
- **동작이 하드코딩되어 있다**(하드코딩 목록 참조).

**목표.** markdown이 진짜 database로 동작한다. 유한하고 완전한 저장 문법이 모든 database 구성을
markdown 구성에 대응시키거나 이름을 들어 거절한다. 각 table은 database 정규화처럼 행을 한 file에
둘지 행마다 file로 둘지 고른다. orm은 다른 database에 접속하듯 DSN으로 markdown database에
접속한다. 칸반, 목록, 간트는 같은 행 위의 view다.

**확정된 것.** `.dbs` set이 schema의 유일한 출발점이다. 선언된 제목은 비지 않고, 선언된 body는
NULL이 아니며 비어 있을 수 있고, view의 entity table은 둘 다 선언한다. 하위 호환 없음, fallback
없음, 숨긴 오류 없음, 꼼수 없음, 규칙 우선.

## 판정

### flowmark가 엔진이고 orm은 IR executor로 접속한다

- 근거(orm `docs/protocol.md`, `docs/config.md`): orm client는 자기 process 안에서 validate와 plan을
  하고, DSN scheme이 고른 native driver로 외부 database에 접속한다. orm은 database 엔진을 담지 않는다.
  markdown 엔진 — parse, 제약, transaction, lock, journal — 은 저장 엔진의 일이다. orm 안에 두면
  다섯 client가 각자 엔진을 가져야 한다. 별도 엔진이면 구현은 하나다.
- SQL driver는 렌더된 SQL을 받는데 markdown 엔진은 이를 받을 수 없으므로 **orm protocol이 IR
  executor를 얻는다**(orm protocol §5): DSN scheme은 SQL dialect 대신 IR executor를 고를 수 있다.
  client는 요청을 검증(`ir.Validate`)하고 plan하지 않으며, 요청마다
  `{ir_version, manifest_hash, request, params, now}`를 보낸다. `params`는 runtime value model JSON을
  쓴다(decimal은 문자열, bytes는 base64, datetime은 UTC `YYYY-MM-DD HH:MM:SS.ffffff`). codec 단계는
  SQL과 같이 client에 남는다. executor는 읽기에는 요청의 형태(join, relation, `key_by`, `flatten`)로
  이미 조립된 행을, 쓰기에는 `{affected, keys}` — `keys`는 insert된 각 행의 생성되거나 주어진 primary
  key를 insert 순서로 담는다 — 를 답하거나, `errors.yaml`의 오류 하나로 답한다. orm protocol 항목은
  `WRITE_CONFLICT`(읽은 뒤 대상이 바뀜, file과 줄을 밝히며 client가 재시도하지 않음)를 추가한다.
  끝난 lock 대기는 SQLite의 `busy_timeout`처럼 `CANCELED`다. hash 불일치는 `SCHEMA_HASH_MISMATCH`,
  일치하는 행이 없는 `optimistic` update는 `OPTIMISTIC_LOCK`이다.
- 연결은 session이다. 요청 외에 control message
  `{op: begin | commit | rollback | savepoint | release | rollback_to, name?}`를 실고, 각각 `{ok}`나
  오류로 답한다. `readOnly`는 지키며 다른 transaction처럼 lock을 잡는다. audit 값을 가진 transaction,
  `isolation`, `timeoutMs`는 `CAPABILITY_UNSUPPORTED`다.
- executor에 없는 모든 surface — `getQuery`, statement event, `force_index`, lock, local setting,
  privilege, AES rotation utility, schema install — 는 orm `docs/dialects.md`의 capability table
  하나에서 `CAPABILITY_UNSUPPORTED`다. Go client가 IR executor를 먼저 구현하고, 다른 client는 각자
  orm checklist 항목이다.
- 엔진을 orm으로 옮기자던 앞선 추천은 이를 확인하지 않고 나온 것이므로 철회하고, design plan의 seam
  판정을 정정한다.

### Server와 transaction

- `flowmark serve <corpus>`는 엔진의 상주 face다 — spec 00의 "상주 daemon 없음"과 spec 40의 "오래
  가는 lock 없음"을 개정한다. loopback TCP에 bind하고 무작위 token을 `.flowmark/server.json`에 한 번
  쓴다 — 재시작해도 유지되며 `flowmark serve --new-token`으로만 바뀐다. port는 필수 flag `--port`다.
  DSN은 `markdown://127.0.0.1:<port>/<corpus>?token=<token>`이고 `<corpus>`는 corpus root의 directory
  이름이다. corpus나 token 불일치는 `CONFIG`다.
- portable lock(spec 40)은 operation 하나 동안, 또는 transaction 하나의 `begin`부터 `commit`이나
  `rollback`까지 잡는다. holder 식별자는 `<pid>@<host>/<session>`이다. 연결이 닫히거나 transaction이
  `tx_idle_ms`(server flag)보다 오래 쉬면 transaction을 rollback하고 lock을 푼다. transaction 안의
  요청은 stage된 post-image를 읽고, savepoint는 중첩 stage다. CLI, MCP, sidecar는 같은 lock을 기다리며
  holder를 밝히는 `LOCK_TIMEOUT`으로 실패한다. spec 40은 각 face 이름을 orm code에 대응시킨다:
  `LOCK_TIMEOUT`은 `CANCELED`, `CORPUS_CONFLICT`는 `WRITE_CONFLICT`.
- face(CLI, MCP, sidecar)는 단일 operation 쓰기를 다시 읽은 뒤 한 번 재시도한다. server transaction은
  server가 재시도하지 않으며 orm client는 `WRITE_CONFLICT`를 받는다.

### Catalog

- corpus root는 command나 server에 준 directory이고 `dbspec/*.dbs`(문서 set, catalog)와
  `.flowmark/`를 담는다. 위로 거슬러 찾지 않는다.
- 문서는 schema stamp를 갖지 않는다: database가 schema를 행마다가 아니라 한 번만 두듯 `dbspec/` set이
  catalog다. spec 30의 stamp, 단독 문서, 내장 fragment는 철회한다.
- `flowmark install`은 `CREATE TABLE`이 저장소를 만들듯 catalog에서 저장 뼈대 — 폴더, 각 `document`
  table의 문서, 빈 블록 섹션 — 를 만든다. `markdown://`에 대한 orm의 schema install은
  `CAPABILITY_UNSUPPORTED`다.
- introspection은 모든 문서를 catalog에 대해 검증하고 catalog를 돌려준다. 실패한 문서는 위치가 있는
  진단이다. catalog 없는 corpus에서 catalog 초안을 만드는 도구는 이 계획에 속하지 않는다.
- **T8.5를 철회한다**: 문서 안 Mermaid 블록은 authority가 아니다. `ParseMermaid`, `ApplySchema`,
  `schema.mermaid` operation, 보드의 diagram 띠를 제거한다. Mermaid view(orm의 `erDiagram` 포함)는
  현재 요구가 아니다.
- 이 repository는 corpus가 아니다: `make validate`와 prose ID 검사기는 옛 엔진과 함께 제거하고,
  `make refs-check`는 kit이 추적하는 checklist를 계속 검사한다.

## 저장 문법

markdown 전용 선언은 모두 dbspec **manifest setting**이며 `markdown <column>` setting을 선례로
따른다: SQL backend는 무시하고 introspection은 일반 column을 돌려준다. flowmark는 orm의 Go
`engine/dbspec`와 `engine/ir` package를 library로 쓰고 다시 구현하지 않는다.

### 저장

- table마다 정확히 하나, 없으면 schema 오류:
  - `store files` — corpus root 아래 table 이름의 폴더에 행마다 file 하나. insert는
    `<table>/<key>.md`를 쓴다. 엔진은 file 이름을 바꾸지 않으며 key가 바뀌어도 file은 그대로다.
  - `store document list` / `store document table` — 모든 행이 corpus root의 file `<table>.md` 하나에
    있다: `<!-- doc-id: <table> -->`, `# <table>`, 그 바로 아래 list나 table, H2의 상세 섹션. doc-id는
    corpus의 행과 `document` file 사이에서 유일하며 twin은 같은 doc-id를 공유한다.
  - `store block <fk> list` / `store block <fk> table` — 이름 붙은 foreign key가 가리키는 부모 행의
    scope 안의 행.

### Key

- markdown table은 단일 column primary key를 갖는다. composite key는 schema 오류다.
- `files` 행의 key는 `<!-- doc-id: -->` 값이다. file 이름은 의미가 없다.
- `identity`는 모든 markdown table에 허용된다: key를 생략한 insert는 lock 아래 할당된 `max+1`을
  받으며, git을 되감은 뒤의 재사용은 받아들이고 명시한다. `key_prefix <p>`를 가진 문자 key는
  `<p><n + 1>`로 할당되며, n은 `<p><n>` 꼴 key 가운데 선행 0 없는 가장 큰 10진 접미사다. orm은 이런
  column을 `identity`처럼 executor가 할당하는 것으로 다루므로 insert가 생략할 수 있다.
  `key_prefix` 없는 문자 key는 insert가 주어야 한다. `key_prefix` column은 `varchar`다(dbspec은
  `text` column을 key로 쓰지 않는다). 손으로 쓴 key는 유일하고 column type을 가지면 받아들이며,
  `<p><n>` 꼴이 아닌 key는 n에 세지 않는다. list나 table 저장의 key는 공백, `#`, `|`, 줄바꿈을 갖지
  않는다.
- key는 불투명하다: ID 문법은 없고 key에서 관계를 읽지 않는다. suffix 문법(`T8.0.14-1`),
  `taskIDLess`, spec 10의 "Task IDs and ordering"은 제거한다.

### 제목과 body

- `title <column>`과 `body <column>`은 table마다 선택하는 setting이다. `list` 저장과 `files` 저장은
  `title`을 요구하고(줄 텍스트, H1), `table` 저장은 cell로 선언할 수 있다. 제목은 non-null이며 빈
  텍스트를 거절한다.
- `body`는 `markdown` column이며 NULL이 아니다. 모든 저장에서 행 scope에 놓인다: file의 H1 뒤 텍스트,
  또는 행 상세 섹션의 첫 하위 heading 앞 텍스트. 상세 섹션이 없는 행의 body는 빈 텍스트다.
- `files` 행은 차례로 header comment, frontmatter, H1 제목, body, level-2 섹션이다(spec 10의 문서
  순서를 개정한다). body와 모든 `markdown` 섹션 값은 fenced block 밖에서만 세는(CommonMark block
  구조) 다음 level ≤ L heading에서 끝난다. 그런 값은 L보다 깊은 heading만 담으며, fence 밖에 level ≤ L
  heading 줄을 가진 값의 쓰기는 `CONSTRAINT`다.
- 칸반, 목록, 간트 view는 entity table이 둘 다 선언할 것을 요구한다. `flowmark.dbs`는 `items`에 둘 다
  선언한다.

### 행 scope, 섹션, 깊이

- `files` 행의 scope는 그 file이다(H1 제목 아래 level L = 2). block 저장의 섹션은 level b heading이고
  그 행들의 상세 섹션 `<h> <key>`는 h = b + 1에 있다. document 저장의 상세 섹션은 h = 2다. 상세 행의
  scope는 L = h + 1이다. heading은 key만 갖고 제목 사본을 갖지 않는다. 블록의 어떤 행도 가리키지 않는
  key의 상세 섹션은 `format.block`이다. 중첩된 list 행의 상세 섹션은 평평한 형제다.
- `markdown` column은 모든 저장에서 언제나 행 scope의 섹션이다 — field나 cell이 아니다.
- level L의 행 scope에서 nullable `markdown` column과 자식 블록은 각각 column이나 자식 table 이름을
  가진 level L heading이다. 없는 섹션은 NULL, 비어 있는 섹션은 빈 텍스트다. 부모의 markdown column과
  같은 이름의 자식 table은 schema 오류다. block이나 document table에서 `#`나 줄바꿈을 담은 key는
  거절한다. H6를 넘는 heading이 필요한 layout은 schema 오류다.

### Scalar column

- `files`: column마다 frontmatter key 하나.
- `table`(block 또는 document): column 이름 header 아래의 cell. 저장되는 모든 column은 header를 갖는다.
- `list`(block 또는 document): `- [<glyph>] <key> <title>[ <field>…]`, 상태 column이 없으면
  `- <key> <title>[ <field>…]`. field는 오른쪽부터 parse한다: token `<선언된 column>:<value>`이고,
  value는 공백, `"`, `\` 없는 bare token이거나 `\"`와 `\\`를 escape로 갖는 `"…"`다. table cell은
  `|`를 `\|`로 쓴다. 없는 field는 NULL, `col:""`은 빈 텍스트이며, 모든 non-null column은 쓴다 —
  엔진은 insert에서 default를 명시적으로 쓰고, non-null field가 빠진 행은 `format.block`이다. `<선언된 column>:`과 맞는 제목 token은 `<column>\:`로 쓴다. `@`와 날짜 sigil은 제거한다.
- 한 줄 자리(list 제목이나 field, table cell)에 줄바꿈을 가진 값은 쓰기에서 `CONSTRAINT`다. 배치
  foreign key와 `parent`는 block 행에 쓰지 않으며, 쓰면 `format.block`이다.

### 상태, 계층, 순서

- 상태 column은 `state_machine` column이다. 상태 column을 가진 `list` 저장은 모든 상태에 대해
  `checkbox <column> <state> '<glyph>'`를 선언하고 glyph는 서로 다르다.
- 계층은 self foreign key `parent`다: list에서는 들여쓰기가 parent이고 table과 file은 column을 쓴다.
  parent를 추측하지 않고 순환은 거절하며, 행의 블록 밖 `parent`는 쓰기에서 `CONSTRAINT`, 읽기에서
  `format.block`이다.
- `order <column>`은 default 없고 key, index, check에 들지 않는 non-null `i32`나 `i64` column을
  가리킨다. 값은 같은 배치 부모와 같은 `parent`를 가진 행 사이에서의 1부터의 위치다. block과
  document 저장에서는 쓰지 않는다 — 읽을 때 줄 순서에서 유도되며 쓰면 `format.block`이다. `files`
  저장에서는 frontmatter field이며 형제 값은 정확히 1..n이어야 한다(빈틈이나 중복은 validate 진단).
  insert는 이를 생략할 수 있고 — orm은 order column을 executor가 할당하는 것으로 다루므로 client는
  값을 보내지 않는다 — 행은 뒤에 붙는다. k를 주면 k에 놓는다. k로의 update는 행을 옮기고 형제가 같은
  transaction에서 따라간다. k < 1이나 k > n + 1은 `CONSTRAINT`이며 잘라 맞추지 않는다. `order`를
  할당하는 update는 정확히 한 행과 맞아야 하며 아니면 `CONSTRAINT`다. `order` 없이 `parent`를 바꾸면
  새 형제 뒤에 붙고 옛 형제는 빈틈을 닫는다.
- `order` column이 없으면 형제(같은 배치 부모, 같은 `parent`) 사이의 줄은 key 순서(정수 key는 숫자,
  문자는 codepoint)로 서며 각 자식은 부모를 따른다. 다른 줄 순서는 `format.noncanonical`이다.
- 저장 순서는 tree의 전위 순회다: 각 행, 그다음 형제 순서(order column, 없으면 key)의 자식. 다른 배치
  부모의 행은 부모의 저장 순서를 따른다. IR `order` 없는 읽기는 저장 순서를 돌려주고, order column에
  대한 IR `order`는 정수 값으로 정렬하며 동률은 저장 순서다. 행을 옮기면 그 subtree가 함께 옮겨진다.

### 관계, link, 이력

- 모든 foreign key는 restrict-only이며 `cascade`와 `set_null`은 거절한다. file 간 one-to-many는 자식
  table을 `store files`로 두고 foreign key를 그 frontmatter에 쓴다.
- 다대다는 `identity` key를 가진 link table이다. 예:
  `item_links(id, from_item, to_item, kind)`와
  `check ck_item_links_kind (kind in ('blocks', 'relates', 'duplicates', 'supersedes'))`. 간트의 의존
  edge는 `blocks`이고, 완료된 항목의 대체는 새 항목과 `supersedes` link를 insert한다.
- `audit`와 `immutable`은 corpus에서 schema 오류다 — markdown에는 trigger가 없다. 대신
  `state_machine <column> history <table> row <fk column> from <column> to <column> at <column>`은
  엔진이 실행하는 모든 전이가 같은 transaction에서 이력 행 하나를 insert하게 한다. setting은 이력
  table의 column 이름을 지정한다(column 이름이 동작을 고르지 않는다): `<fk column>`은 선행 index를
  가진 상태 table로의 foreign key를 갖고, `from`과 `to`는 상태 column의 type, `at`은 `datetime(6)`이며,
  필수 column마다 그 이름과 type을 가진 nullable column 하나가 있다. `identity` key 외의 column은
  없고, 어긋나면 이력 table 이름에서 setting 오류다. 이력 table은 `block <fk> table`로 저장되며
  `title`이나 `body`를 선언하지 않는다. 이력은 전이만 쓰고 insert는 쓰지 않는다. 이력 행을 가진 행의
  삭제는 `FOREIGN_KEY`로 거절한다(restrict-only).
- corpus에서 `state_machine`을 가진 table은 `history`를 선언한다. 없으면 schema 오류다.
- `state_machine <column> initial <state>`는 반복할 수 있고, corpus에서 기계는 적어도 하나를 갖는다.
  기계의 상태는 `initial`, 전이, `terminal` 줄이 이름 붙인 상태들이다. `checkbox`는 그 집합을 정확히
  덮으며, 다른 상태를 이름 붙인 `checkbox`, `limit`, `initial`은 setting 오류다. insert의 상태는 초기
  상태이며, 집합 밖의 상태를 쓰면 `CONSTRAINT`다. 상태 column의 `default`는 초기 상태여야 하며 아니면
  setting 오류이고, 종료 상태이기도 한 초기 상태는 setting 오류다.
- 행의 상태가 마지막 이력 행의 `to`와 다르거나, 이력 행이 없을 때 상태가 초기 상태가 아니면 이력
  빈틈이다 — 사람의 편집이 엔진을 우회했다. validate는 행의 줄에서 `history.gap`을 보고하고, 이력
  행의 `from`이 이전 행의 `to`와 다르거나(첫 행이면 초기 상태가 아니거나) 그 쌍이 선언된 전이가 아니면
  `history.chain`을 보고한다. 둘 다 다른 쓰기를 막지 않는다. 빈틈 있는 행의 엔진 전이는 빈틈이 닫힐
  때까지 `CONSTRAINT`이고, 그 행의 다른 column update는 허용한다. 그러므로 엔진 밖의 상태 변경은
  사람이 일관된 이력을 위조하지 않는 한 보고된다.
- `flowmark history repair <table> <key> [--from <initial state>]`는 마지막 `to`에서 — 이력 없는 행은
  하나뿐인 초기 상태, 또는 초기 상태가 여럿이면 필수인 `--from`에서 — 현재 상태로 가는 이력 행 하나를
  insert한다. `at`은 엔진 시계이고 require column은 행에서 복사한다. 그 쌍이 선언된 전이가 아니거나,
  전이가 요구하는 column이 행에서 null이거나 비었거나, 이 행을 세어 현재 상태의 `limit`를 넘으면
  거절하며, 그때 사람이 편집으로 합법 상태를 되돌리거나 column을 채운다. repair는 사람이 commit하는
  보통 transaction이다.
- commit은 먼저 대상이 읽은 뒤 바뀌지 않았는지 확인하고(아니면 `WRITE_CONFLICT`), 그다음 lock 아래
  다시 읽은 corpus에서 `history.gap`을 포함한 제약 검사를 실행한다. 다른 행의 빈틈은 commit을
  실패시키지 않는다.
- 줄에 `require (c1, …)`를 가진 전이나 종료 상태 진입은 같은 update에서 나열된 모든 column을 할당해야
  한다. 할당되지 않은 column, `null`, 빈 텍스트는 그 column을 밝히는 `CONSTRAINT`다. 이력 행은 할당된
  값을 복사하고 상태 table은 그 값을 유지한다.
- `state_machine <column> limit <state> <n>`은 그 상태에 있는 table 행을 센다. 이를 넘게 할 전이,
  insert, update는 limit를 밝히는 `CONSTRAINT`다. 이미 넘은 corpus(사람의 편집)는 다른 쓰기를 막지
  않는 validate 진단이다. 상대 규칙 `doing < done + 2`는 done 항목이 쌓일수록 제한이 사라지므로
  제거한다.

### 번역

- twin은 doc-id가 어떤 행의 doc-id와 같은 file `<name>.<lang>.md`다. 행이 아니며 엔진은 쓰지 않는다.
  언어 목록은 `dbspec/flowmark.json`의 corpus setting `translations`이고 kit의
  `config/documents.json`이 이를 반영한다. 원본이 바뀐 행은 kit의 `source-sha256` 규칙에 따라 twin을
  낡게 만든다.

### 구성 대응

| dbspec 구성 | markdown corpus에서 |
| --- | --- |
| type | 모든 type, 모든 저장에서 dbspec canonical literal 형식. `bytes`는 base64 |
| primary key | 단일 column. composite는 schema 오류 |
| unique key, check | commit에서 강제. 술어가 unknown(NULL)인 check는 통과하고 unique key는 SQL처럼 NULL을 서로 다르게 본다 |
| index | 받아들임. memory에서 유도 |
| default | insert에서 명시적으로 씀 |
| `null` | 저장마다 문법이 정한 대로 없는 field, cell, 섹션 |
| `identity` | 허용. lock 아래 `max+1` |
| foreign key | restrict-only. `cascade`, `set_null` 거절 |
| `codec` | orm 요청에서는 client가 적용. 엔진이 시작한 쓰기(CLI, MCP, sidecar, 이력)는 orm의 Go codec package로 적용하고 검증은 같은 package로 decode. 저장 값은 encode된 값(byte를 내는 단계는 base64) |
| `aes`, `aes_version`, `blind_index` | schema 오류 — key는 client를 떠나지 않는다 |
| `soft_delete`, `updated`, `default now` | 엔진이 할당: orm 요청이면 요청의 `now`, 아니면 microsecond로 자른 엔진의 UTC 시계 |
| `select explicit`, `entity`, `navigation` | client 쪽. 저장에 영향 없음 |
| `audit`, `immutable` | schema 오류. 대신 `state_machine … history` |
| `state_machine`, `markdown` | 엔진이 강제 |
| `use` 문서 | `dbspec/` set 안에서. 외부 문서는 schema 오류 |

### 응용 schema와 view

- `flowmark.dbs`는 column을 추가해 확장하는 표준 응용 schema다: `items`(`id varchar(32)`와
  `key_prefix 'T'`, `parent varchar(32) null`, `state`, `title varchar(255)`와 `title title`,
  `body text`와 `markdown body`·`body body`, `assignee varchar(32) null` → people,
  `start_on date null`, `start_at datetime(0) null`, `due_on date null`, `due_at datetime(0) null`,
  `priority i16 null`, `position i32`와 `order position`, `evidence text null`, `cause text null`,
  `retry text null`와 `markdown evidence`·`markdown cause`·`markdown retry`)와 check
  `start_on is null or start_at is null`, `due_on is null or due_at is null`, 그리고
  `people(id varchar(32), name varchar(255))`와 `key_prefix 'P'`·`title name`, `item_links(id i64
  identity, from_item varchar(32), to_item varchar(32), kind varchar(16))`와 check
  `ck_item_links_kind`, `item_history`, `views`. 모든 foreign key(`parent`, `assignee`, `from_item`,
  `to_item`, `item`)는 선행 index를 갖는다.
- 시점마다 두 column을 두는 것은 마감이 시각을 가질 수 있다는 T11 판정을 지키기 위함이다: "date 또는
  datetime"을 담는 dbspec type은 없고, datetime만 쓰는 column은 날짜만 있는 마감을 모호한 자정으로
  바꾼다. 간트는 날짜만 있는 시작을 그날의 시작부터, 날짜만 있는 마감을 그날의 끝까지 그린다.
- `items` 기계는 전부 선언된다: `state varchar(16)`, `state_machine state initial waiting`, spec 100의
  전이·종료·`require` 줄(waiting → doing; doing → done require evidence; doing → bypassed require
  cause, retry; bypassed → doing; waiting → impossible require cause; done과 impossible은 종료),
  `checkbox state waiting '[ ]'`, `doing '[~]'`, `done '[o]'`, `bypassed '[!]'`, `impossible '[-]'`,
  `history item_history row item from from_state to to_state at at`. `item_history(id i64 identity,
  item varchar(32), from_state, to_state, at, evidence, cause, retry)`는 같은 세 `markdown` column을
  선언하고 `foreign key fk_history_item (item) references items (id)`와 그 index를 갖는다. 여러 줄
  evidence는 섹션이다(`items.md` H1, `## T1`, `### item_history`, `#### <id>`, `##### evidence` — H6
  안).
- `flowmark.dbs`는 `limit`를 선언하지 않는다: 사용자의 WIP 규칙 — 시작보다 완료, 독립 항목은 병렬
  허용 — 은 어떤 개수로도 표현되지 않는 작업 규칙이며 숫자는 지어낸 것이 된다. `limit` setting은 숫자를
  정하는 schema를 위해 남는다. 엔진은 WIP 규칙을 강제하지 않는다: 상대 gate(`app/apply.go:40,178`)는
  제거하고, 사용자의 규칙은 AGENTS의 작업 규칙으로 남으며, spec 100의 WIP gate 판정과 design plan §9의
  "column limit"은 T12.1에서 정정한다.
- `flowmark.dbs`의 저장: `items`는 `store document list`, `people`, `item_links`, `views`는
  `store document table`, `item_history`는 `store block fk_history_item table`.
- view는 데이터다: `views(id i64 identity, title varchar(255), body text, kind varchar(16),
  entity varchar(63), filter text, sort text, group_by varchar(63) null)`와
  `check ck_views_kind (kind in ('kanban', 'list', 'gantt'))`, `codec filter ordered_json`,
  `codec sort ordered_json`, `title title`, `markdown body`, `body body`. `filter`는 IR `Group`을,
  `sort`는 IR `Order` 목록을 담으며, 쓰기와 validate에서 runtime model에 대해 `ir.Validate`로 검증한다.
  schema 변경으로 무효가 된 view는 위치가 있는 진단이다. GUI에서 저장한 filter는 schema를 고치지
  않는다.
- 칸반 drop은 view에 `sort`가 없고(저장 순서) 양쪽 이웃이 card의 parent를 공유할 때만 `order`를
  정한다. column을 넘는 drop은 상태를 바꾼다. 그 밖의 drop은 거절하고 보드가 이유를 말한다 — 저장하지
  않는 card는 옮기지 않는다. 받아들여진 drop 뒤 보드는 drop 위치가 아니라 operation이 돌려준 행으로
  다시 그리며, column을 넘는 drop은 view의 정렬이 정하는 자리에 card를 놓는다.

## 계층

```
application code ── orm client (Go/PHP/Python/Rust/TS) ── IR executor ── DSN markdown:// ──┐
                                                                                          ▼
flowmark = the markdown database engine (one Go binary)
   engine: codec · validator · IR executor · write path (lock, journal, transactions) · history
   app:    application schema (flowmark.dbs) · install (catalog → skeleton) · sync
   faces:  server · CLI · MCP · soksak sidecar — thin faces of one engine
   uses:   orm engine/dbspec and engine/ir as libraries
                                                                                          ▲
soksak plugin ── sidecar protocol v1 ────────────────────────────────────────────────────┘
   views (kanban, list, gantt) · detail modal · no write path of its own
```

## 단계

repository마다 자기 checklist를 운영한다. orm과 plugin의 작업은 그쪽에서 추적하고 여기서는 이름만
든다.

- **flowmark** (`markdown-database-checklist.md`):
  - T12.1 판정과 spec 00, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, design plan의 seam 정정과
    T8.5 철회.
  - T12.2 모든 저장의 P1/P2를 갖는 저장 codec(orm setting 선행).
  - T12.3 validator(key, foreign key, 순환, 제목, 블록 형식, 순서, limit, 이력, view).
  - T12.4 IR executor protocol 아래의 IR executor(params, 조립된 relation, 쓰기 답, 오류. orm protocol
    항목 선행).
  - T12.5 write path: lock, journal, transaction, savepoint, 이력과 그 repair, 순서 이동.
  - T12.6 introspection.
  - T12.7 runtime model 위의 sync — export는 orm의 SQL dialect renderer로 렌더한다.
  - T12.8 문법에 맞게 다시 쓴 conformance vector.
  - T12.9 server(`serve`, token, session control message. orm protocol 항목 선행).
  - T12.10 모든 table에 대한 CLI, MCP, sidecar operation과 `flowmark install`.
  - T12.11 하나의 dataset을 세 저장 방식으로 둔 demo와 0이 된 하드코딩 목록.
  각 항목은 자신이 대체하는 옛 경로를 제거한다.
- **orm**(자기 checklist): protocol의 IR executor와 capability table, dbspec setting `store`,
  `key_prefix`, `title`, `body`, `checkbox`, `order`, `state_machine … limit`,
  `state_machine … history`, `state_machine … initial`과 corpus 거절, corpus에서의 `history`와
  `initial` 요구, `tests/markdown/flowmark.version`에 고정한 flowmark build를 상대로 한 conformance를
  갖는 Go `markdown` IR executor, 다른 client마다 항목 하나.
- **plugin**(자기 checklist): view 전환 — 칸반, group·sort·filter를 갖는 목록, 날짜 쌍과 `blocks`
  edge를 갖는 간트, 상세 modal(모든 column, body 편집, 자식, link, 이력), 계층, 분리된 UI 문자열,
  구조로 판정하는 answer.

각 항목은 red, green, changelog 쌍을 갖는 `[o]`, commit 순서로 진행한다. 미커밋 변경은 항목 하나를
덮는다.

## 재작성으로 제거되는 발견 사항 (덧대지 않음)

- `EditTitle`은 `@assignee`와 마감을 지운다. `MoveItem`은 자식을 남긴다. `ApplySchema`의
  continuation 검사는 항상 거짓이다(`app/schema.go:100`). table 문서는 WIP gate를 우회한다. validator는
  table 형식 ID를 등록하지 않아 중복이 통과하고 prose 참조가 dangling으로 읽힌다. 전이 evidence는
  저장되지 않는다.
- 모르는 필수 field는 `requiredMet`을 검사 없이 통과한다. frontmatter는 parse되지만 어떤 code도 읽지
  않는다.
- spec 40의 lock과 journal은 구현되어 있지 않다.

## 하드코딩 목록 (모든 항목을 catalog에서 읽거나 제거한다)

| 위치 | 하드코딩 |
| --- | --- |
| `app/apply.go:15,40,178` | WIP glyph `[~]`와 `[o]`, `WIPHeadroom = 2` |
| `app/apply.go:159-167` | 이름 `evidence`, `cause`, `retry`와 부분 문자열 검사 |
| `app/apply.go:315`, `corpus/validate.go:24` | ID 접두사 `T` |
| `executor/executor.go:217` | `taskIDLess`, ID 문법 정렬 |
| `codec/blocks.go:71`, `executor/executor.go:112` | column 이름으로 추측하는 table 종류 |
| `codec/blocks.go:24-45` | `@assignee`와 날짜 sigil |
| `app/schema.go` | 문서 안 Mermaid(제거) |
| `sidecar/loop.go:351` | `firstFlowmarkFile` |
| `sync/export.go` | `documents`와 `items` table, 손으로 렌더한 SQL |
| `corpus/validate.go:51`, `**/*.md` | twin 접미사와 selection |
| plugin `ui/board.js` | code 안 UI 문자열, 문장으로 맞추는 answer |

## 검증

- orm: dbspec rule test와 IR executor protocol test가 통과하고, Go executor가 고정된 flowmark build를
  상대로 모든 IR kind의 conformance를 통과한다.
- flowmark: 모든 저장의 P1/P2. 하나의 dataset을 세 저장 방식으로 두면 같은 행을 낸다. conformance
  vector(corpus 전, 요청, corpus 후)가 통과한다.
- Introspection: 유효한 corpus는 catalog를 돌려주고, 무효한 문서는 위치가 있는 진단이다.
- Write path: `go test -race`, 다중 process 쓰기, transaction 도중 시도한 CLI 쓰기는 기다린 뒤 server
  session을 밝히는 `LOCK_TIMEOUT`으로 실패하고, transaction 도중 들어온 사람의 편집은 file과 줄을
  밝히는 `CORPUS_CONFLICT`로 commit을 실패시키며, journal replay를 갖는 crash 모의, 유휴와 연결 종료
  rollback.
- GUI: 관측 서버와 headless Chrome으로 칸반, 목록, 간트, modal을 캡처하고, 거절된 전이가 제자리로
  돌아온다.
- 문서와 하드코딩: `make documents-check`, `make refs-check`, `make kit-check`가 통과하고 목록을
  grep하면 아무것도 나오지 않는다.
