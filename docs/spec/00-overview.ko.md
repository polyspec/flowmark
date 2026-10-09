<!-- doc-id: spec-overview -->
<!-- source-sha256: d4b1c1e09d0b2b6492d42d0c58f44f49fd9dd3017c3c9ff416963a3c3cff5e3c -->
# 개요

[English](00-overview.md)

flowmark는 markdown 문법을 가진 텍스트 DB다. 파일이 database이고, 읽기와 쓰기는 하나의 검증된
경로를 지나며, 저장 형식은 사람이 읽는 문서로 남는다. audit trail과 branch와 rollback은 git이
가진다.

## 범위

- **orm이 core다.** [polyspec/orm](https://github.com/polyspec/orm)이 markdown corpus를
  MySQL·PostgreSQL·SQLite에 이은 네 번째 backend로 실행한다. shared planner가 IR request를
  검증하고, markdown executor는 request를 직접 소비하며 SQL을 render하지 않는다.
- **flowmark는 통합·응용 layer다.** 하나의 Go binary가 세 개의 thin face — CLI, MCP server,
  soksak sidecar(soksak core 명세의 protocol v1) — 을 가지고, 저장소는 sync adapter(database,
  GitHub, Jira)와 checklist application을 담는다. GUI는 자기 repo의 soksak plugin으로 출시되고,
  모든 view는 자기 write path 없이 또 하나의 thin consumer다.
- **corpus가 truth다.** corpus는 dbspec schema가 기술하는 markdown 문서의 directory tree다.
  모든 write는 transaction 안에서 codec과 validator를 지나고, 모든 읽기는 같은 model API를
  지난다. 외부 database는, 있다하더라도 sync adapter의 derived 또는 owning 쪽일 뿐 감춰진
  mirror가 아니다.

## 강제력

DB 수준의 강제력을 가진 문서 형식: grammar 밖의 key, dangling 참조, type error, 불법 상태 전환은
모두 located diagnostic `{file, line, column, rule, message}`가 된다 — 도구의 write에도, 사람의
편집에도 똑같이. 멱등성은 관례가 아니라 명제다: parse된 canonical 문서를 emit하면 원본이
재현되고, 같은 입력의 반복은 같은 결과와 상태를 낳는다.

## 용어

| 용어 | 뜻 |
| --- | --- |
| corpus | database인 markdown 문서의 directory tree |
| codec | 하나의 문서에 대한 strict parser와 canonical renderer |
| canonical form | renderer가 소유하는 byte 형식; [canonical format](10-canonical-format.ko.md) 참조 |
| skeleton | renderer가 다시 쓸 수 있는 문서의 구조 부분 |
| post-image | transaction commit 후의 corpus 상태 |
| prepare record | rename 앞에 써서 commit을 복구 가능하게 만드는 journal 기록 |
| truth ownership | sync 쌍의 어느 쪽이 table의 authority를 갖는지 |
| adapter | 하나의 sync 계약에 연결된 외부 system(database, tracker) |
| face | binary의 진입점 하나: CLI, MCP server, soksak sidecar |
| twin | 영어 문서의 Korean projection, `source-sha256`으로 묶임 |
| doc-id | header comment가 담는 고유한 문서 신원 |

## 비목표

- SQL text를 만들지도 받지도 않는다; plan layer는 markdown을 위해 아무것도 render하지 않는다.
- derived query mirror가 없다: mirror를 인정하더라도 full-rebuild equivalence를 증명하고 write
  path가 없어야 한다. in-memory index와 parsed-row cache는 mirror가 아니라 가속기다.
- flowmark 자신의 상주 daemon이 없다; face는 host(shell, MCP host, soksak host)의 생명주기 안에서
  돌고 작업 사이에 lock을 잡지 않는다.
- 범용 wiki engine이 아니다: grammar는 schema를 가진 문서를 위해 존재하고, 자유 산문은 schema가
  정한 곳에만 위치한다.

## 문서 지도

판정은 [설계 계획](../plans/design-plan.ko.md)에 있고, 이 명세 집이 그것을 확정한다:
[canonical format](10-canonical-format.ko.md), [문서 모델](20-document-model.ko.md),
[schema](30-schema.ko.md), [운영](40-ops.ko.md), [제약](50-constraints.ko.md),
[capability table](60-dialect.ko.md), [sync](70-sync.ko.md), [migration](80-migration.ko.md),
[conformance](90-conformance.ko.md), [checklist application](100-checklist.ko.md),
[open questions](110-open-questions.ko.md).
