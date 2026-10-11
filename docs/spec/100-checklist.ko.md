<!-- doc-id: spec-checklist -->
<!-- source-sha256: 8d783b9eb0f74a2023dd6a7dda46297489cf80f3e1bf885d3332add9b9c44e57 -->
# 응용 schema

[English](100-checklist.md)

응용 schema는 flowmark가 issue 추적을 위해 제공하는 표준 catalog 문서 `flowmark.dbs`다: 보드가 읽는
항목, 사람, link, 이력, view. corpus는 column을 추가해 이를 확장한다. 이 spec은 그 table과 상태기계를
고정한다.

## Table

| Table | 저장 | Column과 setting |
| --- | --- | --- |
| `items` | `store document list`, file `items.flowmark.md` | `id varchar(32)`와 `key_prefix 'T'`, `parent varchar(32) null` → items, `state varchar(16)`, `title varchar(255)`와 `title title`, `body text`와 `markdown body`·`body body`, `assignee varchar(32) null` → people, `start_on date null`·`start_at datetime(0) null`·`due_on date null`·`due_at datetime(0) null`, `priority i16 null`, `position i32`와 `order position`, `evidence text null`·`cause text null`·`retry text null`(각각 `markdown`) |
| `people` | `store document table`, file `people.flowmark.md` | `id varchar(32)`와 `key_prefix 'P'`, `name varchar(255)`와 `title name` |
| `item_links` | `store document table`, file `item_links.flowmark.md` | `id i64 identity`, `from_item varchar(32)` → items, `to_item varchar(32)` → items, `kind varchar(16)` |
| `item_history` | `store block fk_history_item table`, 자기 item의 상세 섹션 안 | `id i64 identity`, `fk_history_item`으로 items를 가리키는 `item varchar(32)`, `state`와 같은 type의 `from_state`·`to_state`, `at datetime(6)`, items와 같은 `evidence`·`cause`·`retry`(각각 `markdown`) |
| `views` | `store document table`, file `views.flowmark.md` | `id i64 identity`, `title varchar(255)`와 `title title`, `body text`와 `markdown body`·`body body`, `kind varchar(16)`, `entity varchar(63)`, `filter text`와 `sort text`(`codec filter ordered_json`과 `codec sort ordered_json`), `group_by varchar(63) null` |

모든 foreign key는 선행 index를 갖는다. check는 `start_on is null or start_at is null`,
`due_on is null or due_at is null`,
`ck_item_links_kind (kind in ('blocks', 'relates', 'duplicates', 'supersedes'))`,
`ck_views_kind (kind in ('kanban', 'list', 'gantt'))`다.

- **시점마다 두 column**은 마감이 시각을 가질 수 있다는 판정을 지킨다: "date 또는 datetime"을 담는 dbspec
  type은 없고, datetime만 쓰는 column은 날짜만 있는 마감을 모호한 자정으로 바꾼다. 간트는 날짜만 있는
  시작을 그날의 시작부터, 날짜만 있는 마감을 그날의 끝까지 그린다.
- **NULL 담당자는 self다.** 이름을 추측하지 않는다.
- **간트 의존 edge**는 kind `blocks`인 `item_links` 행이다.

## 파일과 link

- `items`, `people`, `item_links`, `views`의 행은 각각 이름으로 정해지는 document file 하나의 행이다
  (`items.flowmark.md`, `people.flowmark.md`, `item_links.flowmark.md`, `views.flowmark.md`).
  `item_history`
  행은 자기 item의 상세 섹션 안에 있는 block 행이다. key는 [document model](20-document-model.md)의 key
  규칙을 따른다: `T1`과 `P1`은 각각 `items`와 `people` table의 key다.
- 외래 key는 [canonical format](10-canonical-format.md)이 정한 대로 link다. 담당자 `P1`을 가진 진행 중인
  `items`의 list 행은 `- [~] T1 Write the spec assignee:[P1](people.flowmark.md)`로 쓴다: 사람 행은
  상세 섹션이 없으므로 target에 anchor가 없다.
- `item_links`의 table cell에서 `from_item`과 `to_item`은 `[T1](items.flowmark.md#items-row-t1)` 같은 link다.
  anchor는 item이 상세 섹션을 가질 때에만 있다.
- 여러 줄 evidence는 section이다: `items.flowmark.md` H1, `## items row T1`, `### item_history`, `#### item_history row <id>`, `##### evidence` — H6 안에 든다. `require` 목록이 이름 붙인 column `evidence`·`cause`·`retry`는 `items`와 `item_history` 양쪽의 `markdown` column이므로, 값이 있는 이력 행은 상세 section을 갖는다. `items`는 `document list`로 저장되며 그 행은 row scope 3에 서고, 이력 section은 H5에 선다. 블록 table에서 row scope 5 이상으로 같은 기계를 두면 H7이 필요하며 schema 오류다([schema](30-schema.md#the-history-table)). column을 더하는 것은 `items`의 저장을 결코 바꾸지 않는다.

## 상태기계

| 줄 | Setting |
| --- | --- |
| 초기 | `state_machine state initial waiting` |
| waiting → doing | — |
| doing → done | `require (evidence)` |
| doing → bypassed | `require (cause, retry)` |
| bypassed → doing | — (재시도 조건이 성립하면 재개. 승인 대기 없음) |
| waiting → impossible | `require (cause)` |
| 종료 | `done`, `impossible` |
| checkbox | `waiting ' '`, `doing '~'`, `done 'o'`, `bypassed '!'`, `impossible '-'` |
| history | `history item_history row item from from_state to to_state at at` |

- checkbox glyph는 정확히 한 글자이며 list 행의 대괄호 사이에 쓴다: list 행은 대괄호 사이에 glyph를 쓰므로
  `waiting`은 `- [ ] T1 …`, `doing`은 `- [~] T1 …`, `done`은 `- [o] T1 …`로 읽힌다.
- 완료된 항목은 다시 열지 않는다: 그와 관련된 새 이슈는 kind `supersedes`의 `item_links` 행을 가진 새
  항목이다.
- **WIP limit는 선언하지 않는다.** 사용자의 규칙 — 시작보다 완료, 독립 항목은 병렬 허용 — 은 어떤 개수로도
  표현되지 않는 작업 규칙이라 숫자는 지어낸 것이 된다. 엔진은 WIP 규칙을 강제하지 않는다. 개수를 원하는
  corpus는 `state_machine state limit doing <n>`을 선언한다.

## View

- view는 `views`의 행이다. `filter`는 IR `Group`을, `sort`는 IR `Order` 목록을 담으며, 쓰기와 validate에서
  runtime model에 대해 `ir.Validate`로 검증한다. catalog 변경으로 무효가 된 view는 위치가 있는 진단이다.
  GUI에서 저장한 filter는 catalog를 고치지 않는다.
- 칸반, 목록, 간트 view는 entity table이 `title`과 `body`를 선언할 것을 요구한다. `flowmark.dbs`는
  `items`에 둘 다 선언한다.
- 칸반 drop은 view에 `sort`가 없고(저장 순서) 양쪽 이웃이 card의 parent를 공유할 때만 `order`를 정한다.
  column을 넘는 drop은 상태를 바꾼다. 그 밖의 drop은 거절하고 보드가 이유를 말한다 — 저장하지 않는 card는
  옮기지 않는다. 받아들여진 drop 뒤 보드는 drop 위치가 아니라 operation이 돌려준 행으로 다시 그린다.
  `order`는 저장 순서 안의 같은 parent drop에서만 정해진다. column을 넘는 drop은 상태 변경뿐이므로
  card는 자기 `order`를 유지하고, view의 정렬 — 없으면 저장 순서 — 이 정하는 자리에 나타난다.
- 어떤 view도 자기 write path를 갖지 않는다: 불법 drag는 불법 CLI 쓰기와 같은 검증에 실패한다.

## 이 repository의 checklist

이 repository는 corpus가 아니다. 그 checklist는 손으로 관리하는 kit 추적 문서로 남는다. 응용 schema는
flowmark가 돌리는 corpus를 위한 것이지 flowmark를 만드는 repository를 위한 것이 아니다.
