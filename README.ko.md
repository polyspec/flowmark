<!-- doc-id: readme -->
<!-- source-sha256: b764b63ae7797429505cfcb36114203e6a52ada4a00caa26fcc870a736f03f20 -->
# flowmark

[English](README.md)

markdown 문법을 가진 텍스트 DB. 파일 자체가 database다 — markdown 문서 folder 위에서 strict parsing,
canonical rendering, corpus-global 제약(type, key, foreign key, state machine)과 transaction을 제공하며
외부 database는 필요 없다.

- **orm이 core다**: [polyspec/orm](https://github.com/polyspec/orm)이 MySQL·PostgreSQL·SQLite에 이어
  markdown을 네 번째 backend로 갖는다. 모든 읽기와 쓰기는 orm model API를 지나며, CLI와 MCP는
  자체 write logic이 없는 thin adapter다.
- **flowmark는 통합·응용 layer다**: database, GitHub, Jira 등 tracker와의 양방향 동기화(하나의 계약을 가진
  adapter architecture), 그리고 그 위의 local 칸반 보드와 issue tracker — data를 로컬에서, git 안에서,
  오프라인으로 관리한다.
- 첫 application은 이 저장소가 스스로 운영하는 checklist·changelog engine이다.

상태: 설계 단계. 판정은 [docs/plans/design-plan.ko.md](docs/plans/design-plan.ko.md)에,
실행 추적은 [docs/plans/execution-checklist.ko.md](docs/plans/execution-checklist.ko.md)에 있다.
공유 tooling은 [polyspec/kit](https://github.com/polyspec/kit)의 vendored copy다
(`make kit-sync KIT_TAG=<tag>`가 갱신하고 `make kit-check`가 검증한다).
