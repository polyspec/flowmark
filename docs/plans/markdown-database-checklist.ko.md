<!-- doc-id: plans-markdown-database-checklist -->
<!-- source-sha256: 0c5d32799a4f127fba21e4b5eee3110f2b5223f2b4253cf3d5194e22c6ab0cf5 -->
# markdown database checklist

## T12 markdown database

- [ ] T12 markdown database: `.dbs` 선언에서 나오는 유한한 저장 문법, orm이 DSN으로 접속하는 엔진으로서의 flowmark, 같은 행 위의 view. 원인: list·table·detailed 형태는 규칙 없이 demo 사례였고 — body column도, parent도, 관계도, 이력도 없었다 — 계획은 엔진을 orm에 두었으나 엔진은 flowmark에 있었으며, 여러 동작이 하드코딩되어 있었다 (T8.5, T10, T11 후속)
- [~] T12.1 판정과 spec을 다시 쓴다: seam 판정(flowmark가 엔진이고 orm은 driver로 접속), 저장 문법, settings, 관계, server, view. T8.5를 철회하고 작업 원칙을 AGENTS에 적는다 (T12 선행)
  - 완료 기준: design plan과 spec 10, 20, 30, 40, 50, 60, 100이 계획의 모든 저장 규칙을 근거와 함께 명제로 적고, T8.5의 Mermaid authority와 orm 내 executor가 그것을 철회하게 한 사실과 함께 철회되며, AGENTS가 작업 원칙을 담고, `make documents-check`와 `make refs-check`가 통과한다.
- [ ] T12.2 orm dbspec에 markdown 저장 settings — `store`, `title`, `body`, `checkbox`, 상태기계 `limit` — 를 red-then-green rule test와 orm 문서 쌍과 함께 추가한다 (T12.1 선행)
- [ ] T12.3 저장 문법 위에 엔진을 다시 쓴다: 모든 저장 방식의 P1/P2를 갖는 codec, validator(foreign key, 순환, title, 블록 형식), IR 전체 executor, portable lock과 journal을 갖는 write path, 영속되는 이력, markdown→dbs introspection. 옛 엔진 경로는 제거한다 (T12.2 선행)
- [ ] T12.4 face를 table 범위로 일반화한다: DSN server, 모든 table에 대한 CLI·MCP·sidecar operation, `flowmark install`, 하나의 dataset을 세 저장 방식으로 다시 만든 demo (T12.3 선행)
- [ ] T12.5 orm Go `markdown` driver가 flowmark server에 접속해 모든 IR kind에 대한 conformance를 통과한다. 다른 client는 orm checklist에 각자 항목을 갖는다 (T12.4 선행)
- [ ] T12.6 plugin이 같은 행 위에서 view를 전환한다 — 칸반, 목록, 간트 — 상세 modal, 계층, 분리된 UI 문자열, 구조로 판정하는 answer와 함께 (T12.4 선행)
