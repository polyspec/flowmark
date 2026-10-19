<!-- doc-id: readme -->
<!-- source-sha256: 81ed053dba6729d577d1fac17d1fbb9246bfbc9565856472551d87865ed541fd -->
# flowmark

[English](README.md)

markdown 문법을 가진 텍스트 DB. 파일 자체가 database다 — markdown 문서 folder 위에서 strict parsing,
canonical rendering, corpus-global 제약(type, key, foreign key, state machine)과 transaction을 제공하며
외부 database는 필요 없다.

- **flowmark가 엔진이다**: Go binary 하나가 markdown database 엔진과 그 얇은 face — server, CLI, MCP
  server, soksak sidecar — 를 담는다. corpus의 `dbspec/` catalog가 모든 table과 그 행이 저장되는 방식을
  선언한다: 행마다 file 하나, 모든 행을 file 하나에, 또는 부모 행 안에.
- **orm은 database로서 접속한다**: [polyspec/orm](https://github.com/polyspec/orm)은 query layer이고,
  client는 MySQL, PostgreSQL, SQLite에 접속하듯 DSN `markdown://`으로 flowmark에 접속한다.
- **엔진 위에**: database, GitHub, Jira 등 tracker와의 양방향 동기화(하나의 계약을 가진 adapter
  architecture), 그리고 칸반·목록·간트 view가 soksak plugin에서 도는 issue 추적 응용 schema — data를
  로컬에서, git 안에서, 오프라인으로 관리한다.

상태: 엔진을 저장 문법 위에 다시 짓는 중이다. 판정은 [docs/plans/flowmark/plan.ko.md](docs/plans/flowmark/plan.ko.md)와
[docs/plans/markdown-database/plan.ko.md](docs/plans/markdown-database/plan.ko.md)에, 추적은
[docs/plans/flowmark/checklist.ko.md](docs/plans/flowmark/checklist.ko.md)와
[docs/plans/markdown-database/checklist.ko.md](docs/plans/markdown-database/checklist.ko.md)에 있다.
공유 tooling은 [@polyspec/kit](https://github.com/polyspec/kit) npm package이며 devDependency다
(`make install-packages`가 설치하고 `make kit-check`가 그 사본을 검증한다).
