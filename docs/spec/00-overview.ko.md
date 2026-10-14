<!-- doc-id: spec-overview -->
<!-- source-sha256: d85b5f89addb7bf99a9c73b6cd1f5da48b71a8fac0594150a2a221bcbdf0431f -->
# 개요

[English](00-overview.md)

flowmark는 markdown 문법을 가진 텍스트 DB다. 파일이 database이고, 읽기와 쓰기는 하나의 검증된
경로를 지나며, 저장 형식은 사람이 읽는 문서로 남는다. audit trail과 branch와 rollback은 git이
가진다.

## 범위

- **flowmark가 엔진이다.** Go binary 하나가 markdown database 엔진 — codec, validator, IR executor,
  lock과 journal을 갖는 write path, 이력 — 과 그 얇은 face를 담는다: server, CLI, MCP server, soksak
  sidecar(soksak core specification의 protocol v1). 모든 face는 같은 엔진을 돌리며 어느 것도 자기
  write path를 갖지 않는다.
- **orm은 flowmark에 IR executor로 접속한다.** [polyspec/orm](https://github.com/polyspec/orm)은
  query layer다: client는 자기 process 안에서 validate와 plan을 하고 DSN scheme이 고른 driver로
  database에 접속한다. scheme `markdown://`은 IR executor를 고른다 — client는 IR 요청을 검증하고 plan하지
  않으며 flowmark server로 보내고, server는 행이나 orm 오류 하나로 답한다. orm은 markdown 엔진을 담지
  않는다. 근거: orm 안의 엔진은 다섯 client가 각자 가져야 하고, 별도 엔진은 구현이 하나다.
- **catalog가 schema다.** corpus root는 모든 table과 그 저장과 setting을 선언하는 dbspec 문서의 set
  파일 `dbspec/dbspec-set.json`을 담는다([schema](30-schema.md)). database가 schema를 행마다가 아니라 한 번만 두듯
  문서는 schema 사본을 갖지 않는다. flowmark는 catalog와 IR을 orm의 Go `engine/dbspec`과 `engine/ir` package를 library로 써서 읽고, 이들을
  다시 구현하지 않는다.
- **corpus가 진실이다.** corpus는 catalog가 기술하는 markdown 문서의 directory tree다. 모든 쓰기는
  transaction 안에서 codec과 validator를 지나고, 모든 읽기는 같은 executor를 지난다. 외부 database가
  있다면 sync adapter의 파생 쪽이거나 소유 쪽이며, 숨은 mirror가 아니다.
- **view는 소비자다.** GUI는 자기 repository의 soksak plugin으로 나간다. 칸반, 목록, 간트는 같은 행
  위의 view이고 그 자체도 행으로 저장되며, 자기 write path를 갖지 않는다.

## 강제력의 약속

database 수준의 강제력을 가진 문서 형식: type을 벗어난 값, dangling 참조, 깨진 key, 불법 상태 전이,
엔진을 우회한 상태 변경은 위치가 있는 진단 `{file, line, column, rule, message}`이다 — 엔진 자신의
쓰기에도 사람의 편집에도 똑같이. idempotency는 관례가 아니라 명시한 명제다: parse한 canonical 문서를
emit하면 그대로 재현되고, 같은 입력으로 operation을 반복하면 같은 결과와 상태를 낸다.

## 용어

| 용어 | 뜻 |
| --- | --- |
| corpus | database인 markdown 문서의 directory tree |
| corpus root | command나 server에 준 directory. `dbspec/`과 `.flowmark/`를 담는다 |
| catalog | corpus root의 `dbspec/dbspec-set.json`이 선언한 문서 set |
| storage | table의 행이 file에 놓이는 방식: `files`, `document`, `block` |
| row scope | 행 하나에 속하는 문서의 부분: 그 file, 또는 그 상세 섹션 |
| codec | 저장 방식들의 strict parser와 canonical renderer |
| canonical form | renderer가 소유하는 byte 형식. [canonical format](10-canonical-format.md) 참조 |
| post-image | transaction이 commit된 뒤 읽힐 corpus |
| prepare record | rename 전에 쓰여 commit을 복구 가능하게 하는 journal 기록 |
| IR executor | 렌더된 SQL 대신 검증된 IR 요청을 받는 orm DSN 대상 |
| truth ownership | sync 쌍에서 table의 authority를 가진 쪽 |
| adapter | 하나의 sync 계약에 묶인 외부 시스템(database, tracker) |
| face | binary의 진입점 하나: server, CLI, MCP server, soksak sidecar |
| key | 행의 식별자. `files` 행의 file 이름(`<table>/<key>.flowmark.md`) |

## 하지 않는 것

- SQL 텍스트를 만들거나 받지 않는다. executor는 IR 요청을 소비한다.
- 파생 query mirror는 없다: mirror를 들인다면 전체 재구축 동등성을 증명하고 write path가 없어야 한다.
  memory index는 가속기이지 mirror가 아니다.
- v1에는 원격 접속이 없다: server는 loopback에만 bind하고 token으로 인증한다.
- 일반 wiki 엔진이 아니다: 문법은 catalog가 기술하는 문서를 위한 것이고, 자유 prose는 catalog가 정한
  자리에 산다.

## 문서 지도

판정은 [design plan](../plans/design-plan.ko.md)과
[markdown database 계획](../plans/markdown-database-plan.ko.md)에 있고, 이 spec 묶음이 그것을 고정한다:
[canonical format](10-canonical-format.md), [document model](20-document-model.md),
[schema](30-schema.md), [operations](40-ops.md), [constraints](50-constraints.md),
[capability table](60-dialect.md), [sync](70-sync.md), [migration](80-migration.md),
[conformance](90-conformance.md), [응용 schema](100-checklist.md),
[open questions](110-open-questions.md).
