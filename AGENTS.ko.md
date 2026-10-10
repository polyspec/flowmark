<!-- doc-id: agents -->
<!-- source-sha256: 4ed2861d30997045bce4d5a98902d6b7d760ea410c544689ee59aa5cf7c0ae01 -->
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
- 지시는 먼저 분류한다: checklist 항목(작업), memory(사용자와 agent만 알아야 하는 것), AGENTS(업무
  지침 — 여기에 중복 없이 두고 checklist나 changelog에는 넣지 않는다), 또는 단순 답변. 작업은
  우선순위를 정하고 문서와 checklist를 갱신한 뒤 진행한다. 지금 하는 작업을 먼저 마무리하며, 사용자의
  명시적 요청이나 새 작업이 먼저여야 한다는 명시된 판단이 있을 때만 중단한다 — 그 밖에는 지시를
  추가하고 우선순위대로 처리한다.
- checklist는 plan마다 파일 하나다: 한 plan의 작업은 checklist 파일 하나에 두고, 다른 plan은
  `docs/plans/`의 자기 plan 옆에 자기 파일을 갖는다 — design plan은 `execution-checklist.md`, 그 밖의
  plan은 `<plan>-checklist.md`. 모든 checklist 파일은 `config/checklist.json`의 tracker이고, Korean
  pair가 같은 항목 ID와 상태를 가지며, 항목 ID는 파일 사이에서도 유일하다. 항목 상태를 바꾸기 전에
  `make documents-check`를 실행한다. 저장소마다 자기 checklist를 운영한다: 다른 저장소의 작업은 그
  저장소에서 추적하고 여기서는 code span 안의 ID로 참조한다. 거대한 단위로 묶지 말고 세부항목을 쪼갠다.
- 문제는 새 checklist 항목이 된다. 기존 항목과 연관되면 ID로 그 항목의 하위 항목이 된다.
- 상태는 `[ ]` 대기, `[~]` 진행 중, `[o]` 완료, `[!]` 우회(항목에 `Cause:`와 `Retry:` 필수),
  `[-]` 불가(`Cause:` 필수). `[o]`는 terminal이다: 완료된 항목에 연관된 새 이슈는 되돌리지 않고
  suffix 항목(`T8.0.14-1`)을 만든다. 병렬 작업은 허용하되 `[~]` 수가 완료 없이 늘어나지 않게 한다:
  독립 항목은 함께 진행해도 되지만, 시작한 항목을 부분만 진행한 채 새 항목을 시작하는 것은 거절한다 —
  시작보다 완료가 먼저이므로 `[o]`가 `[~]`보다 꾸준히 더 늘어난다.
- `[!]`는 그것 없이는 다음 항목으로 넘어갈 수 없을 때만, 명시적 이유와 함께 쓴다. 우회는 완료가
  아니다. 항목에 원인과 재시도 조건을 기록하고, 재시도 조건이 성립하면 허가를 기다리지 않고 재개한다.
  감사는 `[!]` 항목과 그 원인·재시도 조건만 다루며, 무관한 test를 다시 돌리지 않는다.
- 항목은 그것을 끝내는 commit에서, 소유 명령이 통과한 뒤 `[o]`가 된다. 그 commit은 CHANGELOG.md와
  CHANGELOG.ko.md의 `## Unreleased`에 entry도 함께 쓴다. 미커밋 변경은 항목 하나로 제한하며, 미완료
  항목을 핑계로 다른 항목을 더해 그 범위를 넓히지 않는다. 기능마다 검토, 자기 test, 기록을 마친 뒤
  commit한다.
- 결함은 그것을 재현하는 RED test(또는 재현할 deterministic case)로 시작하고, 의도한 이유로
  실패함을 확인한 뒤 원인을 고치고 같은 case를 GREEN으로 통과시킨다. checklist 항목은 관측 가능한
  올바름의 기준을 남긴다.
- English 문서가 canonical이다. 모든 문서는 같은 정보를 가진 `.ko.md` twin이 있고 같은 변경에서
  함께 편집한다. 한국어에서 발견한 수정은 다음 `make documents-stamp` 전에 English 파일로 이식한다.
  정정은 fresh stamp를 가진 새 commit이다.
- commit message는 영문 `type(scope): Subject (#task)`: subject 50자 이하, 대문자 시작, 명령조,
  마침표 없음; 빈 줄; 72자 개행의 body(어떻게가 아니라 무엇을·왜); 필요하면 빈 줄 뒤 참조용 footer;
  type은 `feat`, `fix`, `docs`, `style`,
  `refactor`, `test`, `chore`. `min-0-max <max@blue.tools>`로 commit하고 추가 author
  trailer는 붙이지 않는다.
- 공유 tool은 polyspec/kit의 vendored copy다: `scripts/kit`, `tests/kit`, `kit.json`,
  `.kit/kit.lock.json`은 kit에서만 바뀌고 `make kit-sync KIT_TAG=<tag>`로 이 저장소에 들어온다.
  `make kit-check`가 검증한다. kit에 없거나 잘못된 도구는 kit에 보고하고 여기서 고치지 않는다.
- Multi-platform은 hard requirement다. 편법 금지: lock은 kit의 holder-lock 선례를 따르고 crash
  안전성은 journal replay에서 오며, 현재 요구사항을 완전히 충족하는 가장 단순한 구현을 선택한다.
- 한국어 문서는 기술 용어를 영어로 쓰고 문맥만 한국어로 쓴다. code comment와 commit
  message는 영어로 작성한다.
