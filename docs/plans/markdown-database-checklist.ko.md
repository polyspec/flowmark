<!-- doc-id: plans-markdown-database-checklist -->
<!-- source-sha256: d790dc2ea02c1cbd6951e7d3d0400cd3392ac2d010996e691181b3827af80749 -->
# markdown database checklist

## T12 markdown database

- [ ] T12 markdown database: `dbspec/` catalog이 선언하는 유한한 저장 문법, orm이 DSN으로 IR executor로서 접속하는 엔진으로서의 flowmark, 같은 행 위의 view. 원인: list·table·detailed 형태는 규칙 없이 demo 사례였고 — body column도, parent도, 관계도, 이력도 없었다 — 계획은 엔진을 orm에 두었으나 엔진은 flowmark에 있었으며, 여러 동작이 하드코딩되어 있었다 (T8.5, T10, T11 후속)
- [o] T12.1 markdown database 계획을 바탕으로 판정과 spec 00, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110을 다시 쓴다: seam 정정, T8.5 철회, 저장 문법, catalog, server와 transaction, 이력, view (T12 선행)
  - 완료 기준: 계획의 모든 판정이 해당 spec에 근거를 갖춘 명제로 서고, design plan이 정정된 seam과 철회된 T8.5를 그것을 철회하게 한 사실과 함께 적으며, `make documents-check`와 `make refs-check`가 통과한다.
- [ ] T12.2 저장 codec: `files`, `document`, `block` 저장을 list와 table 형식으로 parse·render하고 모든 저장의 P1/P2를 갖는다. orm `T62` 선행 (T12.1 선행)
- [ ] T12.2.1 catalog를 적재한다: corpus root의 `dbspec/*.dbs` 집합을 orm의 `engine/dbspec`으로, `dbspec/flowmark.json`(번역)을 함께 읽는다. catalog 없음, `store` 없는 table, 복합 key, `aes`/`aes_version`/`blind_index`, `audit`/`immutable`, `cascade`/`set_null`, 외부 `use`, state machine의 `history` 또는 `initial` 없음, list 또는 files 저장의 `title` 없음, H6을 넘는 제목 깊이를 각각 위치가 정해진 진단으로 거절한다 (orm `T62` 선행)
  - 완료 기준: 거절마다 테스트 하나와 적재된 fixture catalog 하나.
- [ ] T12.2.2 `files` 저장: 행마다 파일 하나를 parse·render한다 — 머리 주석, 저장되는 scalar column의 frontmatter, H1 제목, 본문, level-2 section — P1/P2를 갖는다 (T12.2.1 선행)
  - 완료 기준: golden fixture가 byte 단위로 round-trip하고, 이 저장에 대한 spec 10의 형식 진단마다 테스트가 있다.
- [ ] T12.2.3 `document` 저장: H1 아래의 list 또는 table과 H2의 detail section을 가진 `<table>.md`를 다룬다. list field는 오른쪽에서부터 escape와 함께 parse하고, table cell은 escape와 함께 다루며, 들여쓰기는 `parent`로, `order`는 파생된 값으로 본다 (T12.2.1 선행)
  - 완료 기준: T12.2.2와 같다.
- [ ] T12.2.4 `block` 저장: 부모 row의 scope 안의 child block을 다음 level에 재귀적으로 두며, 비어 있지 않을 때만 쓴다. detail section과 그 section을 포함한다 (T12.2.2, T12.2.3 선행)
  - 완료 기준: T12.2.2와 같으며, block row의 detail section 안의 block을 포함한다.
- [ ] T12.2.5 하나의 dataset을 세 저장 방식으로: 같은 행을 files, document, block으로 저장하면 같은 행으로 parse되고, 각 저장마다 P1/P2가 성립한다 (T12.2.4 선행)
  - 완료 기준: 저장 등가성 테스트.
- [ ] T12.3 validator: key, foreign key, 순환, 제목, 블록 형식, 순서, limit, 이력 빈틈과 사슬, view (T12.2 선행)
- [ ] T12.4 orm IR executor protocol 아래의 IR executor: params, 조립된 relation, 쓰기 답과 오류. orm `T61` 선행 (T12.3 선행)
- [ ] T12.5 write path: portable lock, journal, transaction과 savepoint, 이력 행과 이력 repair, 순서 이동 (T12.4 선행)
- [ ] T12.6 introspection: 모든 문서를 catalog에 대해 검증하고 catalog를 돌려준다 (T12.3 선행)
- [ ] T12.7 runtime model 위의 sync: export는 orm의 SQL dialect renderer로 렌더한다 (T12.4 선행)
- [ ] T12.8 저장 문법에 맞게 다시 쓴 conformance vector (T12.5 선행)
- [ ] T12.9 server: `flowmark serve`, token, session control message. orm `T61` 선행 (T12.5 선행)
- [ ] T12.10 모든 table에 대한 CLI, MCP, sidecar operation과 `flowmark install` (T12.5 선행)
- [ ] T12.11 하나의 dataset을 세 저장 방식으로 둔 demo와 계획의 하드코딩 목록 0 (T12.10 선행)
- [ ] T12.12 pseudo-version 대신 orm release를 요구한다: T12.11이 끝나면 flowmark의 go.mod는 `T61`과 `T62`를 담은 released orm tag를 요구한다 (T12.11 선행, orm release 항목 선행)
  - 완료 기준: go.mod가 tag된 orm 버전을 요구하고, `go build ./...`와 `go test ./...`가 통과한다.
