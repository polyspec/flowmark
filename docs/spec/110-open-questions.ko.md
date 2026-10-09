<!-- doc-id: spec-open-questions -->
<!-- source-sha256: 3e42becc2859074fd14b78fcf3cc7420b704ac8ceb47f43f3f268ea1bb72ffb1 -->
# Open questions

[English](110-open-questions.md)

각 항목은 질문, 현재 권고, 결정 조건을 서술한다. 결정이 나면 해당 항목은 소유 명세로 이동하고
여기에서 제거한다.

| # | 질문 | 권고 | 결정 주체 |
| --- | --- | --- | --- |
| 1 | 이 저장소의 license | Apache-2.0 (patent grant — schema-language 파생물에 대해) | 첫 공개 push 이전 |
| 2 | corpus 규모 envelope (문서 수, child row 수)과 40-ops의 p95 latency 예산 | 문서 ≤10k / child row ≤100k; command당 수백 ms | T2의 첫 codec benchmark |
| 3 | suffix 항목의 하위 항목 (`T8.0.14-1.3`) | 허용 — canonical format에 명제로 서술됨. 사용으로 확정 | 첫 renumber codemod |
| 4 | frontmatter comment 지원 | v1 거부 유지. 요구사항이 이름을 붙일 때만 재검토 | 그 요구사항 |
| 5 | 세 face의 MCP tool 목록과 호출별 표면 | model 연산당 tool 하나 + board 구독. portable lock 아래 호출당 하나의 연산 | T7 mini-spec |
| 6 | manifest setting 확장(markdown trait, checklist sugar, state-machine sub-grammar)의 orm 반영 순서 | state machine 먼저 — checklist application이 T8에서 필요로 한다 | orm과의 T5 계획 |
| 7 | 사용자가 다음 kit release에 tag를 붙인 뒤 재vendor | `make kit-sync KIT_TAG=<tag>`가 local path lock 기록을 교체한다 | kit release tag |

상시 note 두 개. 설계 계획은 각 명세가 자기 부분을 흡수할 때까지 ruling 기록으로 남는다. 둘이
어긋나면 명세가 이기고 계획이 수정을 받는다. 그리고 open-question 목록 자체를 모든 phase 종료
시 검토한다 — 결정 조건이 사라진 항목은 결정하거나 제거한다. 방치하지 않는다.
