<!-- doc-id: agents -->
<!-- source-sha256: b96d934cf25005b5f5d7aa453d343a35476c05ae21070649661098c063863bb5 -->
# 개발 규칙

[English](AGENTS.md)

## 목적

flowmark는 markdown 문법을 가진 텍스트 DB다: 이 저장소는 엔진과 그 얇은 face, sync adapter(database,
GitHub, Jira), 응용 schema이고, [polyspec/orm](https://github.com/polyspec/orm)은 DSN `markdown://`으로
이것에 접속하는 query layer다. 판정은 `docs/plans/design-plan.ko.md`와 그 옆의 계획에 있고, 각 계획은
`docs/plans/`에 자기 checklist를 갖는다.

## 원칙

- 여기의 모든 것은 작업하는 사람의 책임이다. 규칙은 이미 있어서가 아니라 옳아서 따른다. 틀린 기존
  규칙, code 경로, 문서는 보고하고 바로잡으며, code를 읽거나 고치다 발견한 이상한 점은 즉시 보고하고
  규칙을 새로 세우거나 바로잡아야 하는지 따진다.
- 규칙이 먼저다. 논리적 규칙을 세우고 그것을 지키는 방식으로 개발하며, 세운 규칙을 따르지 않는
  동작은 결함이다. 모든 결정의 기준은 무엇이 옳은가이며, 기존 원칙, 규칙, 선례가 그렇게 말한다는 것은
  기준이 아니다. 기준은 영원하지 않다: 상황이 그것이 틀렸음을 보이면 근거를 밝혀 고친다. 다만 test에서
  달성이 어렵다는 이유로 기준을 낮추지 않는다 — 그것은 기준에 대한 배신이다.
- 판단이 어렵거나 새로운 논리가 생기면 agent로 적대 검증하며, agent에게 반론과 함께 해결안을 요청하고
  이견이 남지 않을 때까지 반복한다. 적대 검증의 범위는 첫 라운드 전에 확정한다: 검토할 명제를 적힌 그대로,
  그리고 그 밖에 있는 것. 지적은 명제가 틀렸거나, 다른 문서와 모순되거나, 다룬다고 한 경우를 빠뜨렸음을
  보일 때만 인정한다. 범위 옆의 새 질문은 다음 라운드가 아니라 별도 checklist 항목이 된다. 범위가 확정되지
  않으면 라운드는 수렴하지 않는다.
- 하위 호환을 두지 않는다. 옛 경로는 호환 layer, fallback, migration shim 뒤에 남기지 않고 제거한다.
- 오류를 숨기지 않는다. 잘못된 입력을 그럴듯한 입력으로 바꾸는 fallback, 입력이나 출력의 조용한
  버림, 삼킨 오류는 없다.
- fallback은 정말 필요한 곳에만, 그 필요를 밝혀 둔다. 습관처럼 넣은 fallback은 독이다.
- polling이 아니라 event다. 변경은 event로 발행(publish)하고 subscriber나 consumer가 받아 처리한다.
  polling이나 timer는 정공법이 없을 때만 고르는 대안이며, 그렇다고 밝히고, 주 경로로 쓰지 않는다.
  file 변경은 운영체제의 알림으로 안다. 이벤트를 잃었다고 알리는 알림(overflow)도 그 자체로 event이며,
  그 consumer는 해당 tree를 다시 읽는다. 이 경로는 알림 계약이 요구하기 때문에 있고, 그렇다고 밝힌다.
  알림을 보내지 않는 file system은 지원하지 않으며, 그것도 밝힌다.
- 복구, 적용, 변환 연산은 멱등이다: journal replay, canon, install, 같은 byte의 쓰기. insert처럼 본래
  멱등이 아닌 요청은 반복될 때 그 규칙대로 거절한다. 당장 무언가가 돌아가는 것처럼 보이게 임의로 고쳐
  두지 않는다. 일어나는 일은 관측할 수 있고 기계적이다: 사건이 일어났다는 것은 찾아서 아는 것이 아니라
  그 사건이 발행한 것으로 안다. 죽은 process는 아무것도 발행하지 않으므로, engine은 시작할 때 자기
  상태의 선언된 자리 — lock과 journal — 를 한 번 읽는다. 이것은 탐색이 아니라 선언된 자리의 읽기다.
- 경로는 선언된 root에서 선언적으로, 그리고 발견으로 해석하며, 가정한 작업 directory에 대한 상대
  경로로 해석하지 않는다. engine은 symlink를 만들지도 따라가지도 않으며, corpus에서 발견한 symlink는
  그 규칙대로 보고한다.
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
- 확정된 판정은 최종안으로 문서에 남긴다: 판정을 확정하는 그 변경에서 계획과 spec이 근거와 함께 적는다.
  대화, checklist 항목, commit message, agent 보고에만 있는 판정은 확정된 것이 아니며, 그에 의존하는
  작업은 그것이 적힐 때까지 기다린다.

## 규칙

- 사용자 지시가 우선한다. `main`에서 직접 개발한다. branch는
  `{type}/{shortname}-{체크리스트 ID}`, worktree는 `{프로젝트}-{shortname}-{체크리스트 ID}`로,
  agent나 병렬 작업이 필요할 때 만들고 main에 병합되는 즉시 삭제한다 — 병합 후 남은 branch나
  worktree는 용량을 차지하고 폴더를 파편화하며 병합 여부 확인에 혼란을 준다. agent tool이 만드는
  worktree는 tool이 이름을 붙이므로 그 이름을 그대로 쓰며, 그 작업이 병합되는 즉시 branch와 함께
  삭제한다.
- 병렬 항목은 항목 하나나 판정 묶음 하나마다 별도 worktree에서 진행한다. worktree는 code, spec,
  plan을 고친다. changelog와 checklist는 main에 병합하는 쪽만, 병합 다음 commit에서 쓰므로 병렬
  작업이 그 파일에서 충돌하지 않는다. worktree branch는 병합 전에 main 위로 rebase하며, rebase 뒤에
  모든 검사가 통과한다.
- `make test`가 test의 owning command다. repository의 node test와 `scripts/go-test.mjs`를 실행하며,
  이 script는 일부 Go test에 필요한 대소문자 구분 폴더를 마련하고(macOS에서는 `var/` 아래의
  Case-sensitive APFS image, Windows에서는 `fsutil`, 그 밖에는 검사한 폴더) 그 폴더로
  `go test ./...`를 실행한다. 대소문자를 접는 file system에서 맨 `go test ./...`는 그 test를 설계대로
  실패시키며, 필요한 변수를 밝힌다.
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
- 엔진은 database이므로 그 unit test는 촘촘하다: 모든 문법과 제약 규칙에는 허용되는 case와 거절되는 case가 최소 하나씩 있고, table 기반이며 edge case를 포함한다. 모든 저장에는 모든 column type과 NULL 형태 및 빈 형태를 덮는 P1/P2 golden fixture가 있다. parse와 emit은 golden fixture를 seed로 `go test -fuzz`를 사용해 fuzz하며, fuzzer가 찾은 실패 입력은 regression case로 남는다. 규칙을 추가하는 항목은 이 case들이 있을 때까지 끝난 것이 아니다.
- English 문서가 canonical이다. 모든 문서는 같은 정보를 가진 `.ko.md` twin이 있고 같은 변경에서
  함께 편집한다. 한국어에서 발견한 수정은 다음 `make documents-stamp` 전에 English 파일로 이식한다.
  정정은 fresh stamp를 가진 새 commit이다.
- commit message는 영문 `type(scope): Subject (#task)`: subject 50자 이하, 대문자 시작, 명령조,
  마침표 없음; 빈 줄; 72자 개행의 body(어떻게가 아니라 무엇을·왜); 필요하면 빈 줄 뒤 참조용 footer;
  type은 `feat`, `fix`, `docs`, `style`,
  `refactor`, `test`, `chore`. `min-median-max <max@blue.tools>`로 commit하고 추가 author
  trailer는 붙이지 않는다.
- 공유 tool은 polyspec/kit의 vendored copy다: `scripts/kit`, `tests/kit`, `kit.json`,
  `.kit/kit.lock.json`은 kit에서만 바뀌고 `make kit-sync KIT_TAG=<tag>`로 이 저장소에 들어온다.
  `make kit-check`가 검증한다. kit에 없거나 잘못된 도구는 kit에 보고하고 여기서 고치지 않는다.
- Multi-platform은 hard requirement다. 편법 금지: lock은 kit의 holder-lock 선례를 따르고 crash
  안전성은 journal replay에서 오며, 현재 요구사항을 완전히 충족하는 가장 단순한 구현을 선택한다.
- 한국어 문서는 기술 용어를 영어로 쓰고 문맥만 한국어로 쓴다. code comment와 commit
  message는 영어로 작성한다.
