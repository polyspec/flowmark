<!-- doc-id: execution-checklist -->
<!-- source-sha256: 527ed0475bf41ff556140e5509485b4ed42954d281f3c8260b765837fc420971 -->
# 실행 체크리스트

## T1a 설계 session

- [o] T1.0 문서 쌍과 이 checklist로 저장소를 bootstrap한다
  - 완료 기준: README, AGENTS, CHANGELOG, 이 checklist가 영어·한국어로 존재하고 `.flowmark/`가 ignore되며 `git log`에 bootstrap commit이 있다.
- [o] T1.1 적대 감사 결과를 설계 계획에 접는다 (T1.0 선행)
  - 완료 기준: 감사 blocker 전부가 docs/plans/design-plan.ko.md의 판정으로 나타나고, Korean twin이 English source-sha256을 가지며, 계획 개정이 commit된다.
- [o] T1.2 polyspec/kit을 vendor하고 tracker를 구성한다 (T1.0 선행)
  - 완료 기준: config/checklist.json과 config/documents.json을 작성한 상태에서 vendored tree에 `make kit-check`와 `make documents-check`가 통과한다.
- [o] T1.3 stopgap ID 참조 검사기를 등록한다 (T1.2 선행)
  - 완료 기준: docs/의 모든 task ID token이 checklist 항목으로 해석되고 그 검사가 문서 tooling의 일부가 된다.

## T1b 명세

- [ ] T1.4 00-overview, 10-canonical-format, 20-document-model, 30-schema, 40-ops, 50-constraints를 전체 문서 쌍으로 쓴다 (T1.2 선행)
  - 완료 기준: 모든 판정이 근거를 동반한 명제로 나타난다.
- [ ] T1.5 60-dialect capability table과 70-sync, 80-migration, 90-conformance skeleton, 그리고 100-checklist와 110-open-questions를 전체로 쓴다 (T1.4 선행)

## 이후 phase

- [ ] T2 Go codec: strict parser와 canonical renderer (T1.4 선행)
- [ ] T3 Validator: corpus-global 제약 (T2 선행)
- [ ] T4 Executor: parse된 corpus 위의 IR kind (T3 선행)
- [ ] T5 orm markdown-executor 통합: manifest setting과 state-machine sub-grammar (T4 선행)
- [ ] T6 Sync adapter: 계약과 database adapter (T5 선행)
- [ ] T7 CLI와 MCP: orm model API 위의 thin adapter (T5 선행)
- [ ] T8 Checklist application: local issue model (T3 선행)
- [ ] T9 Conformance vector: corpus before, request, 기대 corpus after (T4 선행)
- [ ] T10 Local GUI: 칸반 보드와 issue tracker — 또 하나의 thin consumer (T8 선행)
