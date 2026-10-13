<!-- doc-id: agents -->
<!-- source-sha256: 28d95d7df1bd885b1dd8f47d4baa5752c039816db35d1dd3a6ee94e0490740c0 -->
# 개발 규칙

[English](AGENTS.md)

## 목적

flowmark는 markdown 문법을 가진 텍스트 DB다: 이 저장소는 엔진과 그 얇은 face, sync adapter(database,
GitHub, Jira), 응용 schema이고, [polyspec/orm](https://github.com/polyspec/orm)은 DSN `markdown://`으로
이것에 접속하는 query layer다. 판정은 `docs/plans/design-plan.ko.md`와 그 옆의 계획에 있고, 각 계획은
`docs/plans/`에 자기 checklist를 갖는다.

## 원칙

모든 polyspec repository의 작업 원칙은 kit이 여기에 byte 그대로 vendor하는 [PRINCIPLES.md](PRINCIPLES.ko.md)이며,
이 repository의 모든 작업을 구속한다. 이 파일은 flowmark만의 규칙만 적는다: 아래 항목은 원칙을 엔진에 맞게
구체화한 것이고, 그다음 절은 이 repository의 규칙이다.

- polling이 아니라 event, 엔진에서는 이렇게 적용한다: 파일 변경은 운영체제의 알림으로 알며, 잃은 event를
  알리는 알림(overflow)도 그 자체로 event이고 그 소비자는 영향받은 tree를 다시 읽는다 — 이 경로는 알림 규약이
  요구하므로 있으며 그렇게 적는다. 알림을 보내지 않는 파일 시스템은 지원하지 않으며 그것도 적는다. polling이나
  timer는 정공법의 장치가 없을 때만 그렇다고 적고 고르며, 결코 주 경로가 아니다.
- 멱등과 관측 가능성, 엔진에서는 이렇게 적용한다: journal replay, canon, install, 같은 bytes의 쓰기는 멱등이다.
  insert처럼 본래 멱등이 아닌 요청은 그 규칙대로 반복을 거부한다. event가 일어났다는 것은 찾아서가 아니라 그것이
  게시한 것으로 알게 된다. 죽은 process는 아무것도 게시하지 않으므로, 엔진은 시작할 때 자기 상태의 선언된 장소 —
  lock과 journal — 를 한 번 읽는다. 이것은 검색이 아니라 선언된 장소의 읽기다.
- 경로, 엔진에서는 이렇게 적용한다: 엔진은 symlink를 만들지 않고 따라가지도 않는다. corpus에서 찾은 symlink는 그
  규칙대로 보고한다.

## 규칙

- 사용자 지시가 우선한다. branch와 worktree는 PRINCIPLES.md를 따른다. agent tool이 만드는
  worktree는 tool이 이름을 붙이므로 그 이름을 그대로 쓰며, 그 작업이 병합되는 즉시 branch와 함께
  삭제한다.
- 작업이 병합되지 않은 채 멈춘 worktree는 버려진 것이다. 즉시 검토한다: 보존할 가치가 있는 것은
  worktree를 제거할 조건을 적은 checklist 항목이 되고, 나머지는 버린다. 그 뒤 worktree를 branch와 함께
  삭제한다. 버려진 worktree를 그대로 두지 않는다. 무엇을 담고 있는지 아무도 알 수 없게 되기 때문이다.
- 병렬 항목은 항목 하나나 판정 묶음 하나마다 별도 worktree에서 진행한다. worktree는 code, spec,
  plan을 고친다. changelog와 checklist는 main에 병합하는 쪽만, 병합 다음 commit에서 쓰므로 병렬
  작업이 그 파일에서 충돌하지 않는다. worktree branch는 병합 전에 main 위로 rebase하며, rebase 뒤에
  모든 검사가 통과한다.
- `make test`는 전체 모음이다. repository의 node test와 `scripts/go-test.mjs`를 실행하며,
  이 script는 일부 Go test에 필요한 대소문자 구분 폴더를 마련하고(macOS에서는 `var/` 아래의
  Case-sensitive APFS image, Windows에서는 `fsutil`, 그 밖에는 검사한 폴더) 그 폴더로
  `go test ./...`를 실행한다. 대소문자를 접는 file system에서 맨 `go test ./...`는 그 test를 설계대로
  실패시키며, 필요한 변수를 밝힌다. PRINCIPLES.md가 정한 대로 이것은 활성 checklist의 모든 항목이
  끝났을 때 한 번 실행하며, 항목을 개발하는 동안에는 바뀐 것을 소유한 검사만 실행한다: 바뀐
  package의 `go test`와 바뀐 node test 파일.
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
- 항목은 그것을 끝내는 commit에서, 소유 검사가 통과한 뒤 `[o]`가 된다. 그 commit은 CHANGELOG.md와
  CHANGELOG.ko.md의 `## Unreleased`에 entry도 함께 쓴다. 미커밋 변경은 항목 하나로 제한하며, 미완료
  항목을 핑계로 다른 항목을 더해 그 범위를 넓히지 않는다. 기능마다 검토, 자기 test, 기록을 마친 뒤
  commit한다.
- 결함의 checklist 항목은 관측 가능한 올바름의 기준을 남기며, 그 RED case가 고치기 전에 의도한
  이유로 실패함을 확인한다(PRINCIPLES.md, 검증).
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
