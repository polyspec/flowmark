<!-- doc-id: changelog -->
<!-- source-sha256: f6545e08ebbe17219b94206d997fa8169304de3dbd5c685c69c460a5a23ad448 -->
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
