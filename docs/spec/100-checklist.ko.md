<!-- doc-id: spec-checklist -->
<!-- source-sha256: 786ecbc08d6a24f83d148285619fb1ba209e254826169edcd2c3d0df988c9d08 -->
# 응용 schema

[English](100-checklist.md)

응용 schema는 flowmark가 issue 추적을 위해 제공하는 표준 catalog 문서 `flowmark.dbs`다: 보드가 읽는
항목, 사람, link, 이력, view. corpus는 column을 추가해 이를 확장한다. 이 spec은 그 table과 상태기계를
고정한다.

## Table

| Table | 저장 | Column과 setting |
| --- | --- | --- |
| `items` | `store document list` | `id varchar(32)`와 `key_prefix 'T'`, `parent varchar(32) null` → items, `state varchar(16)`, `title varchar(255)`와 `title title`, `body text`와 `markdown body`·`body body`, `assignee varchar(32) null` → people, `start_on date null`·`start_at datetime(0) null`·`due_on date null`·`due_at datetime(0) null`, `priority i16 null`, `position i32`와 `order position`, `evidence text null`·`cause text null`·`retry text null`(각각 `markdown`) |
| `people` | `store document table` | `id varchar(32)`와 `key_prefix 'P'`, `name varchar(255)`와 `title name` |
| `item_links` | `store document table` | `id i64 identity`, `from_item varchar(32)` → items, `to_item varchar(32)` → items, `kind varchar(16)` |
| `item_history` | `store block fk_history_item table` | `id i64 identity`, `fk_history_item`으로 items를 가리키는 `item varchar(32)`, `state`와 같은 type의 `from_state`·`to_state`, `at datetime(6)`, items와 같은 `evidence`·`cause`·`retry`(각각 `markdown`) |
| `views` | `store document table` | `id i64 identity`, `title varchar(255)`와 `title title`, `body text`와 `markdown body`·`body body`, `kind varchar(16)`, `entity varchar(63)`, `filter text`와 `sort text`(각각 `codec … ordered_json`), `group_by varchar(63) null` |

모든 foreign key는 선행 index를 갖는다. check는 `start_on is null or start_at is null`,
`due_on is null or due_at is null`,
`ck_item_links_kind (kind in ('blocks', 'relates', 'duplicates', 'supersedes'))`,
`ck_views_kind (kind in ('kanban', 'list', 'gantt'))`다.

- **시점마다 두 column**은 마감이 시각을 가질 수 있다는 판정을 지킨다: "date 또는 datetime"을 담는 dbspec
  type은 없고, datetime만 쓰는 column은 날짜만 있는 마감을 모호한 자정으로 바꾼다. 간트는 날짜만 있는
  시작을 그날의 시작부터, 날짜만 있는 마감을 그날의 끝까지 그린다.
- **빈 담당자는 self다.** 이름을 추측하지 않는다.

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
| checkbox | `waiting '[ ]'`, `doing '[~]'`, `done '[o]'`, `bypassed '[!]'`, `impossible '[-]'` |
| history | `history item_history row item from from_state to to_state at at` |

- 완료된 항목은 다시 열지 않는다: 그와 관련된 새 이슈는 kind `supersedes`의 `item_links` 행을 가진 새
  항목이다.
- **WIP limit는 선언하지 않는다.** 사용자의 규칙 — 시작보다 완료, 독립 항목은 병렬 허용 — 은 어떤 개수로도
  표현되지 않는 작업 규칙이라 숫자는 지어낸 것이 된다. 엔진은 WIP 규칙을 강제하지 않는다. 개수를 원하는
  corpus는 `state_machine state limit doing <n>`을 선언한다.

## View

- view는 `views`의 행이다. `filter`는 IR `Group`을, `sort`는 IR `Order` 목록을 담으며, 쓰기와 validate에서
  runtime model에 대해 `ir.Validate`로 검증한다. catalog 변경으로 무효가 된 view는 위치가 있는 진단이다.
  GUI에서 저장한 filter는 catalog를 고치지 않는다.
- 칸반, 목록, 간트 view는 entity table이 `title`과 `body`를 선언할 것을 요구한다.
- 칸반 drop은 view에 `sort`가 없고(저장 순서) 양쪽 이웃이 card의 parent를 공유할 때만 `order`를 정한다.
  column을 넘는 drop은 상태를 바꾼다. 그 밖의 drop은 거절하고 보드가 이유를 말한다 — 저장하지 않는 card는
  옮기지 않는다. 받아들여진 drop 뒤 보드는 drop 위치가 아니라 operation이 돌려준 행으로 다시 그린다.
- 어떤 view도 자기 write path를 갖지 않는다: 불법 drag는 불법 CLI 쓰기와 같은 검증에 실패한다.

## 이 repository의 checklist

이 repository는 corpus가 아니다. 그 checklist는 손으로 관리하는 kit 추적 문서로 남는다. 응용 schema는
flowmark가 돌리는 corpus를 위한 것이지 flowmark를 만드는 repository를 위한 것이 아니다.
