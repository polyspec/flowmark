<!-- doc-id: changelog -->
<!-- source-sha256: 037fbb7c107b0ff4858c3fec18f8a73d8201870c9953c53a0e67fcbbd29ce592 -->
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
- T1.3: `make refs-check`가 config/checklist.json의 checklist와 선택된 문서의 prose(fenced block과
  inline code 제외)를 읽고, 어떤 항목으로도 해석되지 않는 task ID를 file·line·column을 붙인 finding으로
  보고한다. `make test`가 이것을 검증한다.
- T1.1-1: 하나의 Go binary가 세 개의 thin face — CLI, MCP server, soksak sidecar(soksak core
  명세의 protocol v1) — 를 가지고, GUI는 자기 repo의 soksak plugin으로 출시된다. 저장소는
  `package.json`과 `sidecar.json` manifest를 얹은 단일 언어 Go로 유지하고, 언어별 package는
  요구사항이 이름을 붙일 때 만든다.
- T1.4.2: 10-canonical-format이 멱등성 명제 2개, 보존 boundary를 가진 skeleton, 문서 grammar, frontmatter profile, type-directed ordering을 가진 task-ID grammar, encoding 규칙을 서술한다.
- T1.4.3: 20-document-model이 folder=table, file=row, column mapping, child table, 하나의 row인 번역 쌍, frontmatter binding을 서술한다.
- T1.4.4: 30-schema가 dbspec authority 파생, literal block embedding, standalone 임시 authority, schema stamp, 사람 write 판정을 서술한다.
- T1.4.5: 40-ops가 열기 시 복구, 작업 단위 portable lock, prepare record를 가진 규범적 commit protocol, corpus revision token, 비용 모델을 서술한다.
- T1.4.6: 50-constraints가 strict parse, type, key와 참조, state-machine setting, 사람 write 판정, key 필수 codec 검증을 서술한다.
- T1.5.1: 60-dialect이 capability table을 전체로 쓴다 — IR 표면, 강화된 update, on_duplicate 판정, 거부 표면, Go-only 표시.
- T1.5.2: 70-sync가 adapter 계약 범위와 확정 판정 — truth ownership, provenance, git 인식 drift 처리, 재구축 가능 projection — 을 서술한다.
- T1.5.3: 80-migration이 additive-first 범위, codemod transaction, 참조 재작성 예외, checklist cutover을 서술한다.
- T1.5.4: 90-conformance가 corpus vector 계약, 필수 round-trip·멱등성 속성, Go-first runner를 서술한다.
- T1.5.5: 100-checklist가 application을 전체로 쓴다 — table, terminal과 필수 field를 가진 5상태 machine, WIP gate, 또 하나의 consumer인 board, cutover.
- T1.5.6: 110-open-questions가 열린 결정 7건을 권고와 결정 조건으로 등록한다.
- T1.4.1: 00-overview가 범위, 강제력 요약, 용어, 비목표, 문서 지도를 쓴다.
- T1.4.7: 한국어 문서가 dry 문체를 따른다 — 동작 동사, 비유 없음, 영문 기술 용어. T1.4가 7개 하위 항목과 함께 완료된다.
- T2.1: Go module이 document skeleton으로 시작한다 — header comment, 제목, preamble, frontmatter block, H2 section의 strict parse와 canonical render, P1·P2 round-trip test와 header diagnostic.
- T2.5: codec가 저장소 corpus에서 P1과 P2를 증명한다 — 선택된 모든 문서가 diagnostic 없이 parse되고 byte 단위로 재출력되며, CRLF 복사본이 안정적으로 재출력된다. grammar는 실제 문서의 H2·H3 heading과 header-title 인접을 받아들인다.
- T2.4: 모든 parse 규칙이 located diagnostic을 보고한다 — 고정 rule 이름과 line·column. fenced block은 payload가 되어 구조 규칙은 unfenced 줄만 읽고, 닫히지 않은 frontmatter fence는 여는 줄을 지정한다.
- T2.2: frontmatter가 YAML 1.2 core subset으로 parse된다 — scalar·quoted scalar·sequence·literal block — 그리고 canonical로 재출력된다. comment·flow style·anchor·tab indent·중복 key는 located 거부다.
- T2.3: child block이 row로 추출된다 — kit grammar의 checklist 항목(상태·ID·제목·연속 줄)과 markdown table(header·cell) — unfenced payload에서만. malformed 항목은 located 거부다. T2가 완료된다.
- T3.1: corpus validator가 문서 directory를 검사한다 — strict parse finding, corpus 전체의 doc-id·item ID 유일성, 산문의 모든 task-ID token(fenced block·inline code 제외)의 item 해석.
- T3.2: go run ./cmd/flowmark validate .
[validate] the corpus passes이 config/documents.json의 선택으로 corpus directory 위에서 를 실행하고 이 저장소의 corpus가 통과한다. T3가 완료된다.
- T4.1: executor가 corpus row 위에서 one·all·count를 실행한다 — 첫 table로 documents와 checklist items — where predicate·ordering·limit와 함께. task-ID ordering은 segment별 숫자라 `T8.0.9`가 `T8.0.10` 앞에 오고 suffix는 base를 따른다.
- T4.2: executor가 sum·avg·group_count·paginate를 실행한다 — 숫자 아닌 값 거부와 함께 정수 aggregate, 값별 count, total을 동반한 page. T4가 완료된다.
- T8.1: application이 전환 table을 인코딩한다 — done은 terminal, 우회는 원인과 재시도 조건을, 불가는 원인을 요구 — 다섯 상태는 kit glyph에 대응하고 superseded 생성은 `T8.0.14-1`, `T8.0.14-2`로 번호를 매긴다.
- T8.2: 전환 적용이 table과 필수 field와 WIP gate — done 수를 넘는 doing 항목은 최대 두 개 — 를 유지하고 item 줄의 상태 glyph만 그 자리에서 바꾸고 문서의 나머지 byte는 그대로 둔다.
- T8.3: `flowmark items`가 corpus 항목을 나열하고 `flowmark transition`이 state machine으로
  하나를 적용한다. walk가 dot root에 들어가고, Korean twin은 두 번째 key를 등록하지
  않으며, changelog 예시의 ID는 code span에 담는다. T8이 완료된다.
- T1.4.8: 20-document-model이 aggregate와 분할 shape — 단일 파일, 1:n 연결 파일, 1:n:1 — 을 명시한다. 모두 file-as-row·child block·foreign-key column primitive로 구성되고 key는 restrict뿐이다.
- T9.1: conformance vector가 실행된다 — corpus before, request, projection인 기대 rows — 네 vector가 문서 순서·where count·type-directed 항목 순서·paginate total을 고정하고 spec 90이 vector 형식을 서술한다. T9가 첫 단계를 완료한다.
- T7: 세 face가 하나의 binary에서 실행된다 — CLI (validate·items·transition), MCP server (stdio 위 initialize·tools/list·tools/call, 줄당 JSON-RPC message 하나), soksak sidecar (surface routing과 closed handshake를 가진 protocol v1 loop). T7이 완료된다.
- T6: sync engine의 v1이 자리한다 — row의 derived·deterministic SQL dump인 md-to-db export와, provenance header가 body의 hash를 기록하는 adapter 소유 문서로의 rowset materialization. drift 검사가 자기 검증되고 clone에 안전하다. T6이 완료된다.
- T5: orm 저장소가 manifest setting과 state-machine sub-grammar를 dbspec의 state_machine·markdown setting으로 착륙시켰다(orm T59 b74d8890, T60 fb218c7c). parse·검증·canonical 출력·settings model과 engine suite가 green이다. orm engine의 executor 등록은 engine 로드맵으로 남고 능력 계약은 spec 60-dialect에 있다.
