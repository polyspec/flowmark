<!-- doc-id: execution-checklist -->
<!-- source-sha256: 9148dd3ed7c4028d5908f9ed8405fbffbe8455e4ba9d2eabe112b394d085fd71 -->
# 실행 체크리스트

## T1a 설계 session

- [o] T1.0 문서 쌍과 이 checklist로 저장소를 bootstrap한다
  - 완료 기준: README, AGENTS, CHANGELOG, 이 checklist가 영어·한국어로 존재하고 `.flowmark/`가 ignore되며 `git log`에 bootstrap commit이 있다.
- [o] T1.1 적대 감사 결과를 설계 계획에 접는다 (T1.0 선행)
  - 완료 기준: 감사 blocker 전부가 docs/plans/design-plan.ko.md의 판정으로 나타나고, Korean twin이 English source-sha256을 가지며, 계획 개정이 commit된다.
- [o] T1.1-1 세 개의 face과 soksak sidecar 판정을 명시한다 (T1.1 후속)
  - 완료 기준: 계획이 하나의 Go binary가 CLI·MCP server·soksak sidecar(protocol v1)의 세 thin face을 가지고, GUI는 자기 repo의 soksak plugin임을 명시한다.
- [o] T1.2 polyspec/kit을 vendor하고 tracker를 구성한다 (T1.0 선행)
  - 완료 기준: config/checklist.json과 config/documents.json을 작성한 상태에서 vendored tree에 `make kit-check`와 `make documents-check`가 통과한다.
- [o] T1.3 stopgap ID 참조 검사기를 등록한다 (T1.2 선행)
  - 완료 기준: docs/의 모든 task ID token이 checklist 항목으로 해석되고 그 검사가 문서 tooling의 일부가 된다.

## T1b 명세

- [o] T1.4 여섯 개의 core 명세를 전체 문서 쌍으로 쓴다 (T1.2 선행)
  - 완료 기준: 모든 판정이 근거를 동반한 명제로 나타난다; T1.4.1부터 T1.4.7까지 완료되면 완료된다.
- [o] T1.4.1 docs/spec/00-overview.md(.ko)를 쓴다
- [o] T1.4.2 docs/spec/10-canonical-format.md(.ko)를 쓴다
- [o] T1.4.3 docs/spec/20-document-model.md(.ko)를 쓴다
- [o] T1.4.4 docs/spec/30-schema.md(.ko)를 쓴다
- [o] T1.4.5 docs/spec/40-ops.md(.ko)를 쓴다
- [o] T1.4.6 docs/spec/50-constraints.md(.ko)를 쓴다
- [o] T1.4.7 한국어 문서를 dry 문체로 다시 쓴다 — 비유 제거, 동작 동사 사용, 기술 용어 영문 유지 (T1.4.6 선행)
- [ ] T1.5 나머지 명세를 쓴다 (T1.4 선행)
  - 완료 기준: capability table 전체, 범위를 정한 skeleton 3종, 그리고 100-checklist와 110-open-questions 전체; T1.5.1부터 T1.5.6까지 완료되면 완료된다.
- [o] T1.5.1 capability table을 전체로 담은 docs/spec/60-dialect.md(.ko)를 쓴다
- [o] T1.5.2 범위 skeleton으로 docs/spec/70-sync.md(.ko)를 쓴다
- [o] T1.5.3 범위 skeleton으로 docs/spec/80-migration.md(.ko)를 쓴다
- [o] T1.5.4 범위 skeleton으로 docs/spec/90-conformance.md(.ko)를 쓴다
- [o] T1.5.5 docs/spec/100-checklist.md(.ko)를 전체로 쓴다
- [o] T1.5.6 docs/spec/110-open-questions.md(.ko)를 전체로 쓴다

## 이후 phase

- [~] T2 Go codec: strict parser와 canonical renderer (T1.4 선행)
- [o] T2.1 Go module을 document skeleton으로 시작한다: header, 제목, preamble, frontmatter block, H2 section을 canonical round-trip으로 parse하고 render한다 (T1.4 선행)
- [o] T2.2 frontmatter를 YAML 1.2 core subset으로 parse하고 canonical로 재출력한다 (T2.1 선행)
- [ ] T2.3 child block — checklist 항목과 markdown table — 을 row로 parse한다 (T2.2 선행)
- [o] T2.4 모든 parse 규칙을 고정 rule 이름의 located diagnostic으로 보고한다 (T2.1 선행)
- [o] T2.5 이 저장소의 corpus에서 P1과 P2를 golden set으로 증명한다 (T2.1 선행)
- [ ] T3 Validator: corpus-global 제약 (T2 선행)
- [ ] T4 Executor: parse된 corpus 위의 IR kind (T3 선행)
- [ ] T5 orm markdown-executor 통합: manifest setting과 state-machine sub-grammar (T4 선행)
- [ ] T6 Sync adapter: 계약과 database adapter (T5 선행)
- [ ] T7 세 개의 face — CLI·MCP server·soksak sidecar — orm model API 위의 thin adapter (T5 선행)
- [ ] T8 Checklist application: local issue model (T3 선행)
- [ ] T9 Conformance vector: corpus before, request, 기대 corpus after (T4 선행)
- [ ] T10 자기 repo의 soksak plugin: 칸반 보드와 issue tracker — 또 하나의 thin consumer (T8 선행)
