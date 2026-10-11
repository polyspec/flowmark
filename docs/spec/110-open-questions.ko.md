<!-- doc-id: spec-open-questions -->
<!-- source-sha256: 1e836ac320bc84aed86dbbc671e7e4cde7d5a36649fcc459583f2d820e37e8be -->
# Open questions

[English](110-open-questions.md)

각 항목은 질문, 현재의 권고, 그것을 결정하는 조건을 적는다. 결정되면 항목은 소유 spec으로 옮겨지고
여기서 지운다.

| # | 질문 | 권고 | 결정 조건 |
| --- | --- | --- | --- |
| 1 | 이 repository의 license | Apache-2.0 (schema 언어 파생물을 위한 특허 허여) | 첫 공개 push 전 |
| 2 | corpus 규모(문서, 행)와 40-ops의 p95 지연 예산 | 결정됨: 문서 하나에 1 MiB, folder와 corpus에 문서 20,000개, command당 수백 millisecond([규모 한계](40-ops.ko.md#규모-한계)) | `T12.5-3`의 benchmark |
| 3 | T12가 요구하는 orm 항목의 순서 | protocol의 IR executor와 `WRITE_CONFLICT`가 먼저, 그다음 markdown setting — setting 항목은 protocol 항목이 정의하는 executor 할당 column을 필요로 한다 | orm checklist의 등재 |
| 4 | 사용자가 다음 kit release에 tag를 붙인 뒤 kit 재vendor | `make kit-sync KIT_TAG=<tag>`가 local path lock 기록을 대체한다 | kit release tag |
| 5 | comment를 갖지 않게 된 document에서, adapter가 소유한 document의 sync provenance가 어디에 놓이는가 | 아직 기록된 권고가 없다. adapter 작업이 정한다 | database export 너머의 첫 adapter |
| 6 | database export가 기다리는 orm의 surface([sync](70-sync.ko.md#export)) | 다음을 위한 orm 항목: 저장된 identity 값을 지키는 insert와 PostgreSQL identity를 그 값 너머로 다시 시작하는 statement; orm의 Go client가 비공개로 두는, plan의 parameter를 database driver에 bind하는 공개 기능; 그리고 p자리 소수를 가진 SQLite `datetime(p)` bind — 그 executor는 여섯 자리를 bind하지만 renderer의 CHECK는 p자리를 요구한다 | orm checklist의 등재 |

markdown database 계획으로 철회한 것: suffix 항목의 자식(key는 불투명하고 suffix 문법은 제거됨), MCP tool
목록(face는 모든 table 위에서 동작한다, T12.10). readable 문서 판정으로 철회한 것: frontmatter comment
질문(corpus 문서에는 frontmatter가 없다), 그리고 document의 `<!-- doc-id -->`와 provenance header comment.

두 가지 상시 원칙. design plan과 markdown database 계획은 각 spec이 자기 몫을 흡수할 때까지 판정 기록으로
남으며, 둘이 다르면 spec이 이기고 계획을 정정한다. 그리고 open question 목록 자체는 매 phase를 닫을 때
검토한다 — 더 이상 결정 조건이 없는 항목은 결정하거나 버리며 썩게 두지 않는다.
