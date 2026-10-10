<!-- doc-id: spec-open-questions -->
<!-- source-sha256: 0ed71d6ef31e581dea6a728811a42eacaffee266441cb41c693fab0389d8a90d -->
# Open questions

[English](110-open-questions.md)

각 항목은 질문, 현재의 권고, 그것을 결정하는 조건을 적는다. 결정되면 항목은 소유 spec으로 옮겨지고
여기서 지운다.

| # | 질문 | 권고 | 결정 조건 |
| --- | --- | --- | --- |
| 1 | 이 repository의 license | Apache-2.0 (schema 언어 파생물을 위한 특허 허여) | 첫 공개 push 전 |
| 2 | corpus 규모(문서, 행)와 40-ops의 p95 지연 예산 | 문서 ≤10k / 행 ≤100k, command당 수백 millisecond | T12.4의 첫 엔진 benchmark |
| 3 | frontmatter comment 지원 | v1에서는 계속 거절. 요구가 이름 붙일 때만 다시 본다 | 그것을 이름 붙이는 요구 |
| 4 | T12가 요구하는 orm 항목의 순서 | protocol의 IR executor와 `WRITE_CONFLICT`가 먼저, 그다음 markdown setting — setting 항목은 protocol 항목이 정의하는 executor 할당 column을 필요로 한다 | orm checklist의 등재 |
| 5 | 사용자가 다음 kit release에 tag를 붙인 뒤 kit 재vendor | `make kit-sync KIT_TAG=<tag>`가 local path lock 기록을 대체한다 | kit release tag |

markdown database 계획으로 철회한 것: suffix 항목의 자식(key는 불투명하고 suffix 문법은 제거됨), MCP tool
목록(face는 모든 table 위에서 동작한다, T12.10).

두 가지 상시 원칙. design plan과 markdown database 계획은 각 spec이 자기 몫을 흡수할 때까지 판정 기록으로
남으며, 둘이 다르면 spec이 이기고 계획을 정정한다. 그리고 open question 목록 자체는 매 phase를 닫을 때
검토한다 — 더 이상 결정 조건이 없는 항목은 결정하거나 버리며 썩게 두지 않는다.
