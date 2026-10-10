<!-- doc-id: agents -->
<!-- source-sha256: b1af5e6923e485f0954988ca2ba68310b9feb9059b6ea71d03a7f4bfb300aa70 -->
# 개발 규칙

[English](AGENTS.md)

## 목적

flowmark는 markdown 문법을 가진 텍스트 DB다: [polyspec/orm](https://github.com/polyspec/orm)이
core(markdown executor, 네 번째 backend)이고 이 저장소는 통합·응용 layer — sync adapter(database,
GitHub, Jira), checklist·changelog engine, local board. 판정은 `docs/plans/design-plan.ko.md`에,
실행 추적은 `docs/plans/execution-checklist.ko.md`에 있다.

## 원칙

- 여기의 모든 것은 작업하는 사람의 책임이다. 규칙은 이미 있어서가 아니라 옳아서 따른다. 틀린 기존
  규칙, code 경로, 문서는 보고하고 바로잡으며, code를 읽거나 고치다 발견한 이상한 점은 즉시 보고하고
  규칙을 새로 세우거나 바로잡아야 하는지 따진다.
- 규칙이 먼저다. 논리적 규칙을 세우고 그것을 지키는 방식으로 개발하며, 세운 규칙을 따르지 않는
  동작은 결함이다. 한 번 세운 기준을 test를 통과시키려고 낮추지 않는다 — 그것은 기준에 대한 배신이다.
  기준 자체가 틀렸다면 문제로 제기하고 명시적으로 정정한다.
- 하위 호환을 두지 않는다. 옛 경로는 호환 layer, fallback, migration shim 뒤에 남기지 않고 제거한다.
- 오류를 숨기지 않는다. 잘못된 입력을 그럴듯한 입력으로 바꾸는 fallback, 입력이나 출력의 조용한
  버림, 삼킨 오류는 없다.
- 꼼수는 어떤 경우에도 허락하지 않는다. 정석으로 요구를 충족할 수 없고 꼼수만 가능하다면 조용히
  하지 않고 정확히 밝힌다.
- 현재 요구사항을 완전히 충족하는 가장 단순한 구현을 선택한다. 불확실한 미래를 위한 추상화, 설정,
  간접 계층은 두지 않는다. component는 모듈화하고 관심사를 명확히 분리하며, 아키텍처는 장기적
  관점에서 정한다 — 나중에 교체해야 할 임시방편은 받아들이지 않는다. 먼저 올바르게, 올바름을 지키는
  한에서만 빠르게. 문제를 또 다른 문제로 해결하지 않는다.
- 잘못된 변경은 의도와 달랐는지가 아니라 해로움과 옳음으로 판단한다. 해로운 변경은 즉시 되돌리고,
  해롭지 않지만 옳지 않거나 불필요한 변경은 제거하며, 옳은 변경은 남기되 작업과 무관하면 실제 이유로
  별도 commit한다 — 무관한 변경을 관련된 것처럼 적은 기록은 거짓이다.
- 사용자에게 하는 질문은 필요해야 한다. 이미 결정되었거나 명확한 것, 버릇처럼 하는 질문, 혼란스럽게
  하는 질문, 무언가를 누락시키는 질문은 하지 않으며, 필요한 질문은 빠뜨리지 않는다. 새 이정표가 다시
  연 결정은 다시 묻는다.
- 작업에는 문서, AGENTS, memory 갱신이 포함된다.

## 규칙

- 사용자 지시가 우선한다. `main`에서 직접 개발한다. branch는
  `{type}/{shortname}-{체크리스트 ID}`, worktree는 `{프로젝트}-{shortname}-{체크리스트 ID}`로,
  agent나 병렬 작업이 필요할 때 만들고 main에 병합되는 즉시 삭제한다 — 병합 후 남은 branch나
  worktree는 용량을 차지하고 폴더를 파편화하며 병합 여부 확인에 혼란을 준다.
- main에 병합될 수준이 아닌 test code는 의미가 있으면 cherry-pick하고 아니면 폐기하며, 반드시
  commit 전에 처리한다. 바로 제거할 수 없으면 checklist 하위 항목에 제거 조건을 명시한다.
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
  `refactor`, `test`, `chore`. `min-median-max <max@blue.tools>`로 commit하고 추가 author
  trailer는 붙이지 않는다.
- 공유 tool은 polyspec/kit의 vendored copy다: `scripts/kit`, `tests/kit`, `kit.json`,
  `.kit/kit.lock.json`은 kit에서만 바뀌고 `make kit-sync KIT_TAG=<tag>`로 이 저장소에 들어온다.
  `make kit-check`가 검증한다. kit에 없거나 잘못된 도구는 kit에 보고하고 여기서 고치지 않는다.
- Multi-platform은 hard requirement다. 편법 금지: lock은 kit의 holder-lock 선례를 따르고 crash
  안전성은 journal replay에서 오며, 현재 요구사항을 완전히 충족하는 가장 단순한 구현을 선택한다.
- 한국어 문서는 기술 용어를 영어로 쓰고 문맥만 한국어로 쓴다. code comment와 commit
  message는 영어로 작성한다.
