<!-- doc-id: execution-checklist -->
<!-- source-sha256: 011364e484928572817e9ab8174c6fbadc0bfd273fe5b107db8a3536986f0297 -->
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
- [o] T1.4.8 문서 모델에 aggregate와 분할 shape를 명시한다: 단일 파일, 1:n 연결 파일, 1:n:1 (T1.4.7 후속)
- [o] T1.5 나머지 명세를 쓴다 (T1.4 선행)
  - 완료 기준: capability table 전체, 범위를 정한 skeleton 3종, 그리고 100-checklist와 110-open-questions 전체; T1.5.1부터 T1.5.6까지 완료되면 완료된다.
- [o] T1.5.1 capability table을 전체로 담은 docs/spec/60-dialect.md(.ko)를 쓴다
- [o] T1.5.2 범위 skeleton으로 docs/spec/70-sync.md(.ko)를 쓴다
- [o] T1.5.3 범위 skeleton으로 docs/spec/80-migration.md(.ko)를 쓴다
- [o] T1.5.4 범위 skeleton으로 docs/spec/90-conformance.md(.ko)를 쓴다
- [o] T1.5.5 docs/spec/100-checklist.md(.ko)를 전체로 쓴다
- [o] T1.5.6 docs/spec/110-open-questions.md(.ko)를 전체로 쓴다

## 이후 phase

- [o] T2 Go codec: strict parser와 canonical renderer (T1.4 선행)
- [o] T2.1 Go module을 document skeleton으로 시작한다: header, 제목, preamble, frontmatter block, H2 section을 canonical round-trip으로 parse하고 render한다 (T1.4 선행)
- [o] T2.2 frontmatter를 YAML 1.2 core subset으로 parse하고 canonical로 재출력한다 (T2.1 선행)
- [o] T2.3 child block — checklist 항목과 markdown table — 을 row로 parse한다 (T2.2 선행)
- [o] T2.4 모든 parse 규칙을 고정 rule 이름의 located diagnostic으로 보고한다 (T2.1 선행)
- [o] T2.5 이 저장소의 corpus에서 P1과 P2를 golden set으로 증명한다 (T2.1 선행)
- [o] T3 Validator: corpus-global 제약 (T2 선행)
- [o] T3.1 corpus의 key와 참조를 검증한다: doc-id와 item ID의 유일성, 산문의 모든 task-ID token 해석 (T2 선행)
- [o] T3.2 config/documents.json의 선택으로 corpus directory를 검사하는 flowmark validate를 소유 명령으로 연결한다 (T3.1 선행)
- [o] T4 Executor: parse된 corpus 위의 IR kind (T3 선행)
- [o] T4.1 corpus row 위에서 one·all·count를 where·order·limit와 함께 실행한다 (T3 선행)
- [o] T4.2 sum·avg·paginate·group_count를 type-directed ordering으로 실행한다 (T4.1 선행)
- [o] T4.3 markdown table에서 항목을 읽는다: header row가 state·id·title column을 정의하는 table이 그 문서의 항목 목록의 table 형태이며 행 순서대로 항목이다 (T4.2 선행)
  - 완료 기준: `All("items")`이 그런 table의 행을 state glyph·ID·title·문서별 order와 함께 반환하고 codec이 모든 data row의 절대 줄을 기록한다.
- [o] T5 orm markdown-executor 통합: manifest setting과 state-machine sub-grammar (T4 선행)
- [o] T6 Sync adapter: 계약과 database adapter (T5 선행)
- [o] T6.1 corpus를 SQL로 export한다: row의 derived·deterministic INSERT dump (T4 선행)
- [o] T6.2 rowset을 provenance header와 readonly drift 규칙으로 markdown에 materialize한다 (T6.1 선행)
- [o] T7 세 개의 face — CLI·MCP server·soksak sidecar — orm model API 위의 thin adapter (T5 선행)
- [o] T7.1 CLI face: corpus 위의 validate·items·transition (T3 선행)
- [o] T7.2 MCP server face: stdio 위의 initialize·tools/list·tools/call (T7.1 선행)
- [o] T7.3 soksak sidecar face: stdio 위의 protocol v1 message loop (T7.1 선행)
- [o] T7.3-1 sidecar와 MCP의 각 message를 즉시 flush한다. 원인: Serve가 bufio flush를 입력이 끝날 때까지 미뤄서, session을 열어 두는 host — 예제 Wails host, MCP client — 가 process가 멈출 때까지 응답을 받지 못했다. protocol과 JSON-RPC session 모두 session이 진행되는 동안 각 message를 전달한다. 완료 기준: 모든 응답이 write 시점에, 입력이 끝나기 전에 flush되고 pipe가 열려 있는 상태에서 응답 한 건을 읽는 회귀 시험이 이를 덮는다.
- [o] T8 Checklist application: local issue model (T3 선행)
- [o] T8.1 전환 table을 인코딩한다: terminal, 필수 field, superseded 항목 생성 (T3 선행)
- [o] T8.2 corpus에 전환을 적용한다: 상태 재작성, revision 줄 추가, WIP gate 유지 (T8.1 선행)
- [o] T8.3 `flowmark items`와 `flowmark transition`을 소유 명령으로 연결한다 (T8.2 선행)
- [o] T8.4 이슈를 write path로 입력한다: add가 유일한 ID의 대기 항목을 추가하고 rm이 하나를 지우며 하위를 가진 부모는 거부한다 — CLI·MCP tool·sidecar operation 모두 (T8.3 후속)
- [o] T8.4-1 sidecar add가 path 없는 요청을 첫 문서로 보내며 answer가 문서가 정한 ID를 말한다 (T8.4 후속)
  - 완료 기준: path를 실어줄 items answer가 아직 없는 빈 corpus에 sidecar로 첫 이슈를 추가하면 첫 문서에 항목이 쓰이고 ` added`가 아니라 `T1 added`로 답한다.
- [o] T8.5 Mermaid block이 유일한 transition 권위다: Transition이 edge·필수 필드·glyph를 문서 자신의 state machine에서 읽고 AddItem이 `[*]` 줄에서 초기 상태를 취하며 ApplySchema가 Mermaid block을 다시 쓰고 — 내장 table은 남지 않는다 (T8.4 후속)
  - 완료 기준: 사용자 정의 상태 이름을 가진 문서가 자신이 선언한 edge대로 전이한다 — 선언 안 된 edge는 거부, edge label이 필수 evidence를 정하고 새 항목은 선언된 초기 상태에 놓인다 — 그리고 Go source에서 hardcoded state map을 `grep`으로 찾을 수 없다.
- [o] T8.6 write path가 table 형식을 섬긴다: transition·edit·add·remove·move가 items table의 행 cell을 다시 쓴다 — 문서당 하나의 문법, 행과 table의 혼재는 거부한다 (T8.5 후속, T4.3 선행)
  - 완료 기준: table 형식 문서에서 transition이 문서 자신의 edge대로 state cell을 다시 쓰고 add가 자동 ID로 행을 추가하며 edit이 title cell을 고치고 move가 행 순서를 바꾸고 remove가 행을 지운다.
- [o] T7.3-2 corpus를 감시해 changed event를 push한다: sidecar가 열린 surface root의 fingerprint를 poll하고 push하며 page가 session 동안 갱신된다 (T7.3-1 후속)
- [o] T9 Conformance vector: corpus before, request, 기대 corpus after (T4 선행)
- [o] T9.1 vector 형식을 정의하고 첫 vector를 executor로 실행한다 (T4 선행)
- [o] T10 자기 repo의 soksak plugin: 칸반 보드와 issue tracker — 또 하나의 thin consumer (T8 선행)
- [o] T11 항목에 사람과 날짜: 항목 title이 `@assignee`와 마감 date·datetime(`YYYY-MM-DD`, `YYYY-MM-DDTHH:MM[:SS]`, zone 선택)으로 끝날 수 있고 문서가 사람 table(`id`/`name` header)을 선언된 사람의 registry로 가지며 빈 담당자는 self다 — 이름을 추측하지 않는다 — items·people surface가 이들을 노출한다 (T4.3 선행)
  - 완료 기준: codec이 끝 token들을 title에서 분리하고 시간이 붙은 마감도 받으며 `All("items")`이 list·table 항목 모두에 assignee·due를 실고 `All("people")`이 사람 행을 반환하며 sidecar의 items answer가 assignee·due column을 얻고 `people` answer를 가진다.
