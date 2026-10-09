<!-- doc-id: changelog -->
<!-- source-sha256: 7300e4f2f3778789aa2da908c5a3cecd5e9ff295fc042f3c53b7ed3ec5aecbc4 -->
# Changelog

## Unreleased

- T1.0: 저장소가 문서 쌍(README, AGENTS, CHANGELOG), `.flowmark/` ignore 규칙, 이 실행 checklist
  쌍과 함께 bootstrap된다.
- T1.1: 설계 계획이 3개 lane의 적대 감사를 판정으로 접는다 — orm 안의 executor seam, 2절 멱등성,
  dbspec 기반 frontmatter embedding, 하나의 portable 작업 단위 lock을 가진 prepare-record commit
  protocol, restrict-only foreign key, 거부된 identity column, type-directed ordering, adapter 기반
  sync.
- T1.2: 저장소가 polyspec/kit을 lock과 함께 디렉터리 단위로 vendor하고(`make kit-sync`,
  `make kit-check`) tracker를 구성한다: config/checklist.json이 실행 checklist 쌍을 상태
  5종으로 읽고 config/documents.json이 문서 쌍과 changelog를 선택한다. `make documents-check`가
  통과한다.
