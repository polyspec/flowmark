<!-- doc-id: spec-checklist -->
<!-- source-sha256: ddd4a25333674ce2d1d097ff30d9ea129b065000812fb511d85d5a319775b0c0 -->
# Checklist application

[English](100-checklist.md)

checklist application은 corpus 위의 첫 application이다: 이 저장소가 사용하는 engine이고 board가
읽는 local issue model이다. 이 명세는 table, state machine, kit 정합성을 확정한다.

## Table

| Table | column (핵심) |
| --- | --- |
| `checklists` | `id` (PK), `title`, `version`, `created`, `updated` |
| `checklist_items` | `id` (PK, task-ID grammar), `checklist_id` (FK), `parent_id` (self FK, nullable), `status`, `title`, `order`, `supersedes_id` (FK, nullable), `created`, `updated`, `closed` |
| `item_dependencies` | `item_id` (FK), `depends_on_id` (FK), `kind` (`prerequisite` 또는 `follow-up`) |
| `item_revisions` | append-only: `id`, `item_id` (FK), `from_status`, `to_status`, `reason`, `evidence`, `at` |
| `changelogs` | `item_revisions`의 projection — 자동 생성하고 수작업으로 작성하지 않는다 |

저장소당 checklist 하나다. 완료된 항목에 연관된 새 이슈는 suffix 항목(`T8.0.14-1`)을
`supersedes_id`와 함께 생성한다. 완료된 항목은 terminal을 유지한다.

## State machine

| 전환 | 필수 field |
| --- | --- |
| waiting → doing | — |
| doing → done | evidence |
| doing → bypassed | cause, retry condition |
| bypassed → doing | — (retry 조건이 성립하면 재개한다. 승인 대기 없음) |
| waiting → impossible | cause |
| done → (모든 상태) | 거부 — terminal. 대신 superseded 항목을 생성한다 |

- 다섯 상태는 kit 상태 `[ ]` `[~]` `[o]` `[!]` `[-]`와 일대일로 대응한다. kit의 strict 규칙 —
  우회는 `cause:`와 `retry:` 필요, 불가는 `cause:` 필요 — 는 validator가 위 table에서 강제하는
  요구사항과 같다.
- **WIP gate는 application 규칙이다**: 완료 결과 없이 `doing` 항목 수가 늘어나는 동안 새 항목
  시작을 거부한다. 사용자가 명시적으로 중단을 요청한 경우는 예외다. 독립적인 병렬 작업은
  허용한다.
- board (kanban view)는 또 하나의 consumer다: column은 다섯 상태, card는 `checklist_items`,
  edge는 `item_dependencies`다. 불법 drag는 불법 CLI 호출과 같은 전환 검증으로 실패하고 WIP
  gate는 column 제한이다. view 전용 write path는 존재하지 않는다.

## Commit 규율 (인코딩된 것)

- 항목은 그것을 완료하는 commit에서, 소유 명령 통과 후 done이 된다. 같은 commit이 changelog
  entry를 작성한다.
- `item_revisions`는 append-only이고 changelog를 유도한다. projection은 파일 내 data에서
  재구축한다.
- commit message 관례와 red/green 규율은 AGENTS.md에 있다 — 이 명세는 그것을 강제하는 data
  model을 정의한다.

## Kit 정합성과 cutover

- cutover 전까지 kit가 자기 grammar로 checklist 문서를 읽고 reference 검사(`make refs-check`)가
  dangling-reference 계열을 검사한다.
- cutover는 하나의 transaction이다 ([migration](80-migration.ko.md)): tracker가
  `config/checklist.json`에서 제거되고, 문서가 corpus 형식으로 변환되고, 같은 commit에서
  `flowmark validate`가 통과한다.
