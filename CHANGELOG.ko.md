<!-- doc-id: changelog -->
<!-- source-sha256: 677af3d3c681fa104a13ef1bf705aa4d023ac16c29a13f5c568f6feaf78b0b15 -->
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
  보고한다. `make test`가 이것을 덮는다.
- T1.1-1: 하나의 Go binary가 세 개의 thin 얼굴 — CLI, MCP server, soksak sidecar(soksak core
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
