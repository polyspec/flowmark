<!-- doc-id: plans-markdown-database -->
<!-- source-sha256: 6839226f67c4d76fae60bcec605a330c78530b807528ac74bd8d862da2d55ab2 -->
# markdown database 계획

[English](markdown-database-plan.md)

checklist 항목 T12의 계획이다. markdown database의 저장 문법, 관계, 계층, view를 정한다. 아래 각
판정은 T12.1에서 해당 spec으로 옮겨지며, spec과 이 계획이 다르면 spec이 이기고 이 계획을 정정한다.

## 배경

칸반 desync 수정(T10-1~T10-4)에서 markdown database에 규칙이 아니라 사례만 있다는 것이 드러났다.

- **세 형태는 demo 사례였다.** list, table, detailed demo는 각자 어떤 규칙도 정의하지 않은 형태를
  키웠다. detailed demo의 `## T2: 제목` 섹션은 어떤 code도 읽지 않으며, 복제된 제목은 이미 항목과
  어긋났다(`작성1` 대 `작성`).
- **모델에 기본 요소가 없다.** 항목에는 body column, parent field, 관계, 이력 기록이 없다. 자식은
  ID 문자열로 추측되어 `MoveItem`은 자식을 남기고, `EditTitle`은 `@assignee`와 마감 token을
  지운다.
- **계층 판정이 틀렸다.** design plan(§1)은 markdown executor를 orm repository에 두었지만, orm은
  database 위의 query layer이고(아래 판정), code도 그 판정을 따른 적이 없다. orm에는 dbspec
  setting 두 개가 있을 뿐이고 flowmark는 orm에서 아무것도 import하지 않는다.
- **동작이 하드코딩되어 있다**(하드코딩 목록 참조).

**목표.** markdown이 진짜 database로 동작한다.

- 유한하고 완전한 저장 문법이 모든 database 개념을 markdown 구성요소에 대응시킨다.
- 각 table은 database 정규화처럼, 행을 한 file에 모을지(aggregate) 행마다 file로 둘지(split)를
  고른다.
- orm은 다른 database에 접속하듯 DSN으로 markdown database에 접속한다.
- 칸반, 목록, 간트는 같은 행 위의 view다.

**확정된 것.** `.dbs` file이 schema의 유일한 출발점이다. 제목은 비어 있으면 안 되고, body
column은 필수이되 빈 텍스트를 가질 수 있다. 하위 호환 없음, fallback 없음, 숨긴 오류 없음,
꼼수 없음, 규칙 우선.

## 판정

### flowmark가 엔진이고 orm은 driver로 접속한다

- 근거(orm `docs/protocol.md`, `docs/config.md`): orm client는 IR을 자기 process 안에서 plan하고,
  DSN scheme이 고른 native driver로 외부 database에 접속한다. orm은 database 엔진을 담지 않으며,
  MySQL, PostgreSQL, SQLite는 모두 그 밖에 있다.
- markdown 엔진 — parse, 제약, transaction, lock, journal — 은 저장 엔진의 일이다. orm 안에
  두면 orm 자신의 계층을 깨고, 다섯 client(Go, PHP, Python, Rust, TypeScript)가 각자 엔진을
  가져야 한다. design plan의 "markdown은 Go 전용" 단서가 그 증상이다.
- 별도 엔진이면 구현은 하나이고, 각 orm client는 접속하고 IR을 보내고 행을 읽는 얇은 driver만
  얻는다.
- 엔진을 orm으로 옮기자던 앞선 추천은 이를 확인하지 않고 나온 것이므로 철회하고, design plan의
  seam 판정을 정정한다.

### `.dbs` file이 뼈대를 만들고 문서는 schema 사본을 갖지 않는다

- `.dbs` set은 database catalog처럼 schema다. `flowmark install`은 `CREATE TABLE`이 저장소를
  만들듯 그로부터 저장 뼈대 — 폴더, 부모 문서, 빈 블록 섹션 — 를 만든다.
- database는 schema를 행마다가 아니라 한 번만 둔다. 데이터 문서는 manifest hash stamp만 갖는다.
  문서마다 schema 사본을 두면 사본의 이탈이 그 자체로 버그 부류가 된다.
- **T8.5를 철회한다**: 문서 안 Mermaid 블록은 authority가 아니다. `ParseMermaid`, `ApplySchema`,
  `schema.mermaid` operation, 보드의 diagram 띠를 제거한다.
- Mermaid view(orm의 `erDiagram` 포함)는 현재 요구가 아니므로 범위 밖이다.

### Introspection

- orm은 MySQL, PostgreSQL, SQLite를 dbspec 문서로 introspect한다(`dialects.md#introspection`).
  manifest setting(`state_machine`, `markdown`)은 database에서 복원되지 않는다.
- markdown은 corpus가 stamp를 가질 때 정확히 dbspec으로 introspect된다. stamp가 없으면 구조는
  추론되지만 type, key, 제약은 추론되지 않는다. 그 결과는 옮기지 못한 것의 목록을 돌려주는
  import 도구이고, 조용히 확정된 추측이 아니다.

## 저장 문법

markdown 전용 선언은 모두 dbspec **manifest setting**이며, 기존 `markdown <column>` setting을
선례로 따른다. SQL backend는 이를 무시하고 introspection은 일반 column을 돌려준다. 문법이
하나이면 parser도 하나다 — flowmark는 orm의 Go `engine/dbspec` package를 library로 쓰고 다시
구현하지 않는다.

| 개념 | 규칙 |
| --- | --- |
| table 저장 | table마다 정확히 하나: `store file <dir>`(split, 행마다 file 하나) 또는 `store block <fk> list` / `store block <fk> table`(aggregate, 부모 행 안의 블록). 없는 table은 schema 오류이며 기본값을 추측하지 않는다. |
| 행 key | markdown table은 단일 column primary key를 갖는다. file 행의 key는 `<!-- doc-id: -->` 값이다 — file 이름은 의미도 사본도 갖지 않는다. block 행은 key를 행에 쓴다. |
| 제목 | `title <column>`: non-null이고 빈 텍스트를 거절하며 저장되는 모든 table에 필수다. file 행의 H1, list 행의 줄 텍스트, table 행의 cell이다. |
| body | `body <column>`: `markdown` column이며 저장되는 모든 table에 필수이고 빈 텍스트를 허용한다. file 행의 preamble, block 행의 상세 섹션 텍스트다. |
| 그 밖의 markdown column | heading이 column 이름인 행 scope의 섹션이다. 선언되지 않은 heading은 진단이다. |
| scalar column | file 행에서는 frontmatter key, table 행에서는 column 이름 header 아래의 cell(모든 column 존재), list 행에서는 줄 끝 inline field `column:value`다 — 선언된 column만 field이고, 공백이 있는 값은 `"..."`로 쓰며, renderer는 field로 읽힐 제목 token을 escape한다. `@`와 날짜 sigil은 제거한다. |
| 상태 column | `state_machine` column이다. list 행의 checkbox glyph는 `checkbox <column> <state> '<glyph>'`로 선언한다. |
| 행 scope | file 행의 scope는 그 file이다. block 행의 scope는 블록 섹션 아래 `<heading+1> <key>` 상세 섹션이다. heading은 key만 갖고 제목 사본을 갖지 않는다. 상세 섹션이 없으면 body는 빈 텍스트다. |
| 블록 위치 | 자식 table C의 블록은 부모 행 scope 안 `<다음 level> C` 섹션에 재귀적으로 놓인다. H6를 넘으면 schema 오류다. |
| 계층 | self foreign key `parent`. list에서는 들여쓰기가 parent이고 column을 쓰지 않으며, table과 file은 column을 쓴다. ID 문자열로 parent를 추측하지 않고 순환은 거절한다. |
| 순서 | block 행은 문서 순서를 지킨다. file 행은 선언된 order column을 따르고, 없으면 key 순서다. |
| 관계 | 모든 foreign key는 restrict-only다. file 간 one-to-many는 자식 table을 `store file`로 두고 foreign key를 그 frontmatter에 쓴다. aggregate나 split은 저장 선언이며 논리 관계는 같다. |
| 다대다와 link | `item_links(from, to, kind in (blocks, relates, duplicates))` 같은 link table이다. 간트의 의존 edge는 `blocks`다. |
| 이력 | 엔진이 `audit into`의 의미를 직접 수행한다. 이력 table은 감사 대상 행 아래의 block이고, 전이의 `require` column(evidence, cause, retry)이 그 행으로 영속된다. |
| 블록 형식 | 블록 섹션의 형식은 schema가 정하며, 다른 형식은 진단이다. |

### 응용 schema와 view

- `flowmark.dbs`는 표준 응용 schema이고 column을 추가해 확장한다: `items`(id, parent, state,
  title, body, assignee → people, start, due, priority), `people`, `item_links`, `item_history`,
  `views`.
- view(칸반, 목록, 간트)는 **데이터**다: `views`의 행이므로, GUI에서 저장한 filter가 schema를
  고치지 않는다.
- WIP limit은 엔진이 전이 시점에 강제하므로 schema에 속한다: `state_machine <column> limit
  <state> <n>`. 상대 규칙 `doing < done + 2`는 done 항목이 쌓일수록 제한이 사라지므로 틀렸고
  제거한다.

## 계층

```
application code ── orm client (Go/PHP/Python/Rust/TS) ── markdown driver ── DSN markdown:// ──┐
                                                                                               ▼
flowmark = the markdown database engine (one Go binary)
   engine: codec · validator · IR executor · write path (lock + journal, spec 40) · introspection
   app:    application schema (flowmark.dbs) · application rules · install (.dbs → skeleton) · sync
   faces:  server (for the orm drivers) · CLI · MCP · soksak sidecar — thin faces of one engine
   uses:   orm engine/dbspec (parser, manifest hash) · engine/ir (request types) — as libraries
                                                                                               ▲
soksak plugin ── sidecar protocol v1 ─────────────────────────────────────────────────────────┘
   views (kanban/list/gantt) · detail modal · no write path of its own
```

- **Server.** loopback TCP에 bind하고 DSN은 `markdown://127.0.0.1:<port>`이며, wire는 orm IR
  요청(manifest hash 포함)과 행 answer의 JSON lines다. manifest hash가 다르면 거절한다. 원격 접속과
  인증은 v1 밖이며 spec이 그렇게 명시한다.
- **동시성.** 사람, git, CLI, server가 같은 file을 만지므로 모든 operation은 spec 40의
  operation 단위 portable lock을 잡는다.
- **orm은 얻는다**: dbspec setting `store`, `title`, `body`, `checkbox`, `state_machine … limit`,
  그리고 client마다 capability table을 갖는 `markdown` driver. orm의 interface parity 규칙에 따라
  client마다 orm checklist 항목을 둔다.
- **flowmark는 다시 쓴다**: 저장 문법 위에 엔진을 다시 쓰고 `app/schema.go`, `firstFlowmarkFile`,
  sigil 문법, 상대 WIP 규칙을 호환 layer 없이 제거한다.

## 단계

1. **T12.1 판정과 spec.** design plan의 seam 판정을 정정하고 T8.5를 철회하며 이탈의 사실을
   기록한다. spec 20(저장), 30(settings), 10(문법), 40(server와 lock), 50(관계와 순환),
   60(capability), 100(응용 schema, view, WIP)을 개정한다. 작업 원칙과 branch, worktree, test 폐기
   규칙을 AGENTS에 적고 memory를 갱신한다.
2. **T12.2 orm dbspec setting**: red 후 green, orm 문서 쌍과 vector와 함께.
3. **T12.3 엔진 재작성**: orm `engine/dbspec`와 `engine/ir` 위에서 P1/P2를 갖는 저장 codec,
   validator, IR 전체 executor, lock과 journal과 이력을 갖는 write path, introspection. 옛 엔진
   경로는 제거한다.
4. **T12.4 face**: DSN server, CLI·MCP·sidecar의 table 범위 operation(`schema`, `rows`,
   `row.get`, `row.insert`, `row.update`, `row.move`, `row.delete`, `transition`, `views`),
   `flowmark install`, 하나의 dataset을 세 저장 방식으로 둔 demo.
5. **T12.5 orm Go `markdown` driver**: flowmark server를 상대로 한 conformance. 다른 client는
   각자의 orm 항목.
6. **T12.6 plugin**: view 전환(칸반, group/sort/filter를 갖는 목록, start·due·`blocks` edge를
   갖는 간트), 상세 modal(모든 column, body 편집, 자식, link, 이력), 계층, 분리된 UI 문자열,
   구조로 판정하는 answer.

각 단계는 red test, green, changelog 쌍을 갖는 `[o]`와 commit 순서로 진행한다. 미커밋 변경은
항목 하나를 덮는다. branch나 worktree는 main에 병합되는 즉시 삭제한다. 이 repository의
checklist는 계속 수동으로 관리한다.

## 재작성으로 제거되는 발견 사항 (덧대지 않음)

- `EditTitle`은 `@assignee`와 마감을 지운다. `MoveItem`은 자식을 남긴다. `ApplySchema`의
  continuation 검사는 항상 거짓이다(`app/schema.go:100`). table 문서는 WIP gate를 우회한다.
  validator는 table 형식 ID를 등록하지 않아 중복이 통과하고 prose 참조가 dangling으로 읽힌다.
  전이 evidence는 저장되지 않는다.
- 모르는 필수 field는 `requiredMet`을 검사 없이 통과한다. frontmatter는 parse되지만 어떤 code도
  읽지 않는다.
- spec 40의 lock과 journal은 구현되어 있지 않다.

## 하드코딩 목록 (모든 항목을 schema에서 읽는다)

| 위치 | 하드코딩 |
| --- | --- |
| `app/apply.go:15,40,178` | WIP glyph `[~]`와 `[o]`, `WIPHeadroom = 2` |
| `app/apply.go:159-167` | 이름 `evidence`, `cause`, `retry`와 부분 문자열 검사 |
| `app/apply.go:315`, `corpus/validate.go:24` | ID 접두사 `T`. ID 문법은 column check가 된다 |
| `codec/blocks.go:71`, `executor/executor.go:112` | column 이름으로 추측하는 table 종류 |
| `codec/blocks.go:24-45` | `@assignee`와 날짜 sigil |
| `app/schema.go` | 문서 안 Mermaid(제거) |
| `sidecar/loop.go:351` | `firstFlowmarkFile` |
| `corpus/validate.go:51`, `**/*.md` | twin 접미사와 selection. 하나의 configuration에서 정한다 |
| `ui/board.js`(plugin) | code 안 UI 문자열, 문장으로 맞추는 answer |

## 검증

- orm: dbspec rule test가 통과하고, Go driver가 flowmark server를 상대로 모든 IR kind의
  conformance를 통과한다.
- flowmark: 세 저장 방식의 P1/P2 round trip. 하나의 dataset을 세 저장 방식으로 두면 같은 행을
  낸다. conformance vector(corpus 전, 요청, corpus 후)가 통과한다.
- Introspection: stamp가 있는 corpus는 그 `.dbs`를 내고, 없는 corpus는 옮기지 못한 것의 목록을
  낸다.
- Write path: `go test -race`, 다중 process 동시 쓰기, journal replay를 갖는 crash 모의.
- GUI: 관측 서버와 headless Chrome으로 칸반, 목록, 간트, modal을 캡처하고, 거절된 전이가 제자리로
  돌아온다.
- 문서와 하드코딩: `make documents-check`, `make refs-check`, `make kit-check`가 통과하고 목록을
  grep하면 아무것도 나오지 않는다.
