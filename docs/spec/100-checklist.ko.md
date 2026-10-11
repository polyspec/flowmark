<!-- doc-id: spec-checklist -->
<!-- source-sha256: 9e010325bbfba5a07e7beba72724982589047ce7e816e77ae713238c27e605de -->
# 응용 schema

[English](100-checklist.md)

응용 schema는 flowmark가 issue 추적을 위해 제공하는 표준 catalog 문서 `flowmark.dbs`다: 보드가 읽는
항목, 사람, link, 이력, view. corpus는 column을 추가해 이를 확장한다. 이 spec은 그 table과 상태기계를
고정한다.

## Table

| Table | 저장 | Column과 setting |
| --- | --- | --- |
| `items` | `store files`, item 하나에 file `items/<key>.flowmark.md` 하나 | `id varchar(32)`와 `key_prefix 'T'`, `parent varchar(32) null` → items, `state varchar(16)`, `title varchar(255)`와 `title title`, `body text`와 `markdown body`·`body body`, `assignee varchar(32) null` → people, `start_on date null`·`start_at datetime(0) null`·`due_on date null`·`due_at datetime(0) null`, `priority i16 null`, `position i32`와 `order position`, `evidence text null`·`cause text null`·`retry text null`(각각 `markdown`) |
| `people` | `store document table`, file `people.flowmark.md` | `id varchar(32)`와 `key_prefix 'P'`, `name varchar(255)`와 `title name` |
| `item_links` | `store block fk_item_links_from table`, 자기 `from_item`의 file 안 | `id i64 identity`, `from_item varchar(32)` → items, `to_item varchar(32)` → items, `kind varchar(16)` |
| `item_history` | `store block fk_history_item table`, 자기 item의 file 안 | `id i64 identity`, `fk_history_item`으로 items를 가리키는 `item varchar(32)`, `state`와 같은 type의 `from_state`·`to_state`, `at datetime(6)`, items와 같은 `evidence`·`cause`·`retry`(각각 `markdown`) |
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

- item은 자기 file `items/<key>.flowmark.md`다: H1인 title, field, body, `markdown` column, 그다음
  transition들의 block `## item_history`와 그 item이 시작하는 link들의 block `## item_links`.
  [document model](20-document-model.ko.md#저장)이 `files` 행과 그 child block을 저장하는 그대로다.
  `people`과 `views`의 행은 각각 document file 하나, `people.flowmark.md`와 `views.flowmark.md`의
  행이다. key는 document model의 key 규칙을 따른다: `T1`과 `P1`은 각각 `items`와 `people` table의
  key다.
- **어떤 file도 project의 수명과 함께 자라지 않는다.** item에 대한 쓰기, transition — 그 이력 행을
  item의 block에 쓴다 —, item이 시작하는 link, item의 move는 그 item의 file만 쓰므로, 서로 다른
  item의 transition과 link는 서로 다른 file을 건드리고, item의 이력과 나가는 link는 item과 함께
  읽힌다. 두 document file은 팀과 그 저장된 view로 한정되며, 이는 한 일의 양과 함께 자라지 않는다.
  근거: 모든 item과 그 이력과 link를 담는 document file은 쓰기마다 통째로 parse되고 다시 쓰이므로
  ([비용 모델](40-ops.ko.md#비용-모델)) 그 비용과 git merge 충돌이 project와 함께 자란다. 이력을
  `document table`로 저장하면 table의 모든 transition이 한 file에 쌓이며, 이력 table의 저장은 그것을
  block의 깊이와 맞바꾼다([schema](30-schema.ko.md#이력-table)). 각 file의 크기는 가정하지 않고
  관측한다: file을 재는 envelope와 그것을 넘은 진단은 `T12.5-3`의 몫이다.
- `item_links`는 `from_item`으로 놓인다(`store block fk_item_links_from table`): link는 자기가
  시작하는 item에 속하고, `to_item`과 `kind`가 block의 cell이며, block 행의 `from_item`은 그 배치이지
  cell이 아니다. item으로 들어오는 link는 `to_item`이 그 item을 가리키는 행이며, 다른 모든 foreign
  key처럼 table에 대한 query로 읽는다.
- `position`은 같은 `parent`를 가진 item들 사이에서 item의 위치가 아니라 **rank**를 담는다: 읽기는
  column을 1부터 세는 위치로 채우고, move는 item에 새 이웃 사이의 rank를 주며, 그런 rank가 있는 한
  그 file만 쓴다([document model](20-document-model.ko.md#순서)). 형제 사이에 덧붙는 item은 마지막
  rank보다 1000 큰 rank를 받으므로, 엔진이 쓴 corpus는 `position: 1000`, `position: 2000`, …로 읽힌다.
- 외래 key는 [canonical format](10-canonical-format.md)이 정한 대로 link다. 담당자가 `P1`인 item의
  field는 `- assignee: [P1](../people.flowmark.md)`로 쓴다: 사람 행은 상세 섹션이 없으므로 target에
  anchor가 없다. link block의 `to_item` cell은 item의 file을 `[T3](T3.flowmark.md)`로 link하며,
  `parent` field도 부모의 file을 같은 방식으로 link한다.
- 여러 줄 evidence는 section이다: item의 title을 H1으로 가진 `items/T1.flowmark.md`, `## evidence`.
  이력 행 안의 그 사본은 `## item_history`, `### item_history row <id>`, `#### evidence` — H6 안에
  든다. `require` 목록이 이름 붙인 column `evidence`·`cause`·`retry`는 `items`와 `item_history` 양쪽의
  `markdown` column이므로, 값이 있는 이력 행은 상세 section을 갖는다. `items`는 `files`로 저장되며 그
  행은 row scope 2에 서고, 이력 section은 H4에 선다. 블록 table에서 row scope 5 이상으로 같은 기계를
  두면 H7이 필요하며, 그 이력 table이 `document table`로 저장되지 않는 한 schema 오류다
  ([schema](30-schema.ko.md#이력-table)). column을 더하는 것은 `items`의 저장을 결코 바꾸지 않는다.

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
  `waiting`은 `- [ ] T1 …`, `doing`은 `- [~] T1 …`, `done`은 `- [o] T1 …`로 읽힌다. 표준 저장은
  상태를 item file의 field `- state: doing`으로 쓴다. glyph는 `items`를 list로 저장하는 corpus를 위해
  기계를 온전히 유지하며, 그런 corpus는 glyph가 필요하다
  ([schema](30-schema.ko.md#dbspec-구성에-대한-corpus-규칙)). `schema` operation은 glyph를 답한다.
- 완료된 항목은 다시 열지 않는다: 그와 관련된 새 이슈는 kind `supersedes`의 `item_links` 행을 가진 새
  항목이다.
- **WIP limit는 선언하지 않는다.** 사용자의 규칙 — 시작보다 완료, 독립 항목은 병렬 허용 — 은 어떤 개수로도
  표현되지 않는 작업 규칙이라 숫자는 지어낸 것이 된다. 엔진은 WIP 규칙을 강제하지 않는다. 개수를 원하는
  corpus는 `state_machine state limit doing <n>`을 선언한다.

## View

- view table은 이름이 `flowmark`인 catalog 문서 — 이 응용 schema — 의 table `views`이며, 어떤 manifest
  setting도 그것을 선언하지 않는다. 그 view column은 각각 codec 없는 non-null `varchar`나 `text`인 `kind`와
  `entity`, 각각 정확히 `codec <column> ordered_json`을 가진 non-null `varchar`나 `text`인 `filter`와
  `sort`, codec 없는 `varchar`나 `text`인 `group_by`다. 다른 모양은 table 이름에서 catalog 오류다. 이름이
  `flowmark`인 문서가 없거나 그 `flowmark` 문서가 `views`를 선언하지 않는 catalog에는 view table이 없다.
  근거: view는 SQL database에 아무 뜻이 없고, orm은 셋 모두에서 같은 뜻으로 render되는 setting만
  받아들이므로 orm은 view table을 선언하지 않는다. view는 보드가 읽는 응용 데이터이므로, view를 정의하는
  응용이 그 table을 이름 붙인다. 응용이 자기 schema가 선언한 table에 묶이는 것과 같다. column 이름이 동작을
  고르지 않는다는 schema 규칙([schema](30-schema.ko.md#markdown-setting))은 모든 catalog에 적용되는 저장
  setting을 다스린다. 이 선언은 이 spec이 소유하는 한 문서의 한 table을 이름 붙인다.
- view는 `views`의 행이다. `filter`는 `{"where": <IR Group>, "params": [<value>, …]}`를 담으며 filter
  없음은 `{"where":{"items":[]},"params":[]}`다. `sort`는 IR `Order` 목록을 담으며 `[]`는 정렬 없음이다.
  `group_by`는 view가 묶는 field를 이름 붙이며 없으면 NULL이다. 이것들은 [constraints](50-constraints.ko.md#view)가
  정하는 대로 쓰기와 validate에서 runtime model에 대해 `ir.Validate`로 검증한다. catalog 변경으로 무효가
  된 view는 위치가 있는 진단이다. GUI에서 저장한 filter는 catalog를 고치지 않는다.
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
