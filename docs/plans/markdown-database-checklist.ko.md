<!-- doc-id: plans-markdown-database-checklist -->
<!-- source-sha256: 140ae3004b79c01f4829f7fdad99c2069c0201028640ceeedfe437eb3052384f -->
# markdown database checklist

## T12 markdown database

- [ ] T12 markdown database: `dbspec/` catalog이 선언하는 유한한 저장 문법, orm이 DSN으로 IR executor로서 접속하는 엔진으로서의 flowmark, 같은 행 위의 view. 원인: list·table·detailed 형태는 규칙 없이 demo 사례였고 — body column도, parent도, 관계도, 이력도 없었다 — 계획은 엔진을 orm에 두었으나 엔진은 flowmark에 있었으며, 여러 동작이 하드코딩되어 있었다 (T8.5, T10, T11 후속)
- [o] T12.1 markdown database 계획을 바탕으로 판정과 spec 00, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110을 다시 쓴다: seam 정정, T8.5 철회, 저장 문법, catalog, server와 transaction, 이력, view (T12 선행)
  - 완료 기준: 계획의 모든 판정이 해당 spec에 근거를 갖춘 명제로 서고, design plan이 정정된 seam과 철회된 T8.5를 그것을 철회하게 한 사실과 함께 적으며, `make documents-check`와 `make refs-check`가 통과한다.
- [ ] T12.2 저장 codec: `files`, `document`, `block` 저장을 list와 table 형식으로 parse·render하고 모든 저장의 P1/P2를 갖는다. orm dbspec markdown setting 항목 선행 (T12.1 선행)
- [ ] T12.3 validator: key, foreign key, 순환, 제목, 블록 형식, 순서, limit, 이력 빈틈과 사슬, view (T12.2 선행)
- [ ] T12.4 orm IR executor protocol 아래의 IR executor: params, 조립된 relation, 쓰기 답과 오류. orm protocol IR executor 항목 선행 (T12.3 선행)
- [ ] T12.5 write path: portable lock, journal, transaction과 savepoint, 이력 행과 이력 repair, 순서 이동 (T12.4 선행)
- [ ] T12.6 introspection: 모든 문서를 catalog에 대해 검증하고 catalog를 돌려준다 (T12.3 선행)
- [ ] T12.7 runtime model 위의 sync: export는 orm의 SQL dialect renderer로 렌더한다 (T12.4 선행)
- [ ] T12.8 저장 문법에 맞게 다시 쓴 conformance vector (T12.5 선행)
- [ ] T12.9 server: `flowmark serve`, token, session control message. orm protocol IR executor 항목 선행 (T12.5 선행)
- [ ] T12.10 모든 table에 대한 CLI, MCP, sidecar operation과 `flowmark install` (T12.5 선행)
- [ ] T12.11 하나의 dataset을 세 저장 방식으로 둔 demo와 계획의 하드코딩 목록 0 (T12.10 선행)
