<!-- doc-id: agents -->
<!-- source-sha256: 3d8fbc0fcabd6c66fd943ea6e02be550c484be023c5126c73f33ba3af7c0907f -->
# 개발 규칙

[English](AGENTS.md)

## 목적

flowmark는 markdown 문법을 가진 텍스트 DB다: [polyspec/orm](https://github.com/polyspec/orm)이
core(markdown executor, 네 번째 backend)이고 이 저장소는 통합·응용 layer — sync adapter(database,
GitHub, Jira), checklist·changelog engine, local board. 판정은 `docs/plans/design-plan.ko.md`에,
실행 추적은 `docs/plans/execution-checklist.ko.md`에 있다.

## 규칙

- 사용자 지시가 우선한다. `main`에서 직접 개발한다. branch는
  `{type}/{shortname}-{체크리스트 ID}`, worktree는 `{프로젝트}-{shortname}-{체크리스트 ID}`로,
  agent나 병렬 작업이 필요할 때만 만들고 통합한 뒤 즉시 제거한다.
- `docs/plans/execution-checklist.md`가 유일한 task list이며 Korean pair가 같은 항목 ID와 상태를
  가진다. 항목 상태를 바꾸기 전에 `make documents-check`를 실행한다. 저장소당 checklist 하나;
  거대한 단위로 묶지 말고 세부항목을 쪼갠다.
- 상태는 `[ ]` 대기, `[~]` 진행 중, `[o]` 완료, `[!]` 우회(항목에 `Cause:`와 `Retry:` 필수),
  `[-]` 불가(`Cause:` 필수). `[o]`는 terminal이다: 완료된 항목에 연관된 새 이슈는 되돌리지 않고
  suffix 항목(`T8.0.14-1`)을 만든다. 병렬 작업은 허용하되 `[~]` 수가 완료 없이 늘어나지 않게 한다.
- 항목은 그것을 끝내는 commit에서, 소유 명령이 통과한 뒤 `[o]`가 된다. 그 commit은 CHANGELOG.md와
  CHANGELOG.ko.md의 `## Unreleased`에 entry도 함께 쓴다. 미커밋 변경은 항목 하나로 제한한다.
- 결함은 그것을 재현하는 RED test(또는 재현할 deterministic case)로 시작하고, 의도한 이유로
  실패함을 확인한 뒤 원인을 고치고 같은 case를 GREEN으로 통과시킨다. checklist 항목은 관측 가능한
  올바름의 기준을 남긴다.
- English 문서가 canonical이다. 모든 문서는 같은 정보를 가진 `.ko.md` twin이 있고 같은 변경에서
  함께 편집한다. 한국어에서 발견한 수정은 다음 `make documents-stamp` 전에 English 파일로 이식한다.
  문서 commit에 대한 interactive history 편집 금지 — 수정은 fresh stamp를 가진 새 commit이다.
- commit message는 영문 `type(scope): Subject (#task)`: subject 50자 이하, 대문자 시작, 명령조,
  마침표 없음; 빈 줄; 72자 개행의 body(무엇을·왜); type은 `feat`, `fix`, `docs`, `style`,
  `refactor`, `test`, `chore`. `min-0-max <max@blue.tools>`로 commit하고 추가 author
  trailer는 붙이지 않는다.
- 공유 tool은 polyspec/kit의 vendored copy다: `scripts/kit`, `tests/kit`, `kit.json`,
  `.kit/kit.lock.json`은 kit에서만 바뀌고 `make kit-sync KIT_TAG=<tag>`로 이 저장소에 들어온다.
  `make kit-check`가 검증한다. kit에 없거나 잘못된 도구는 kit에 보고하고 여기서 고치지 않는다.
- Multi-platform은 hard requirement다. 편법 금지: lock은 kit의 holder-lock 선례를 따르고 crash
  안전성은 journal replay에서 오며, 현재 요구사항을 완전히 충족하는 가장 단순한 구현을 선택한다.
- 한국어 문서는 기술 용어를 영어로 쓰고 문맥만 한국어로 쓴다. code comment와 commit
  message는 영어로 작성한다.
