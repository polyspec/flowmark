<!-- doc-id: agents -->
<!-- source-sha256: 1901c92be9f3a9764421d2574f5dfa40ccb4b843b6c6f37f5b9ab29d939d10be -->
# 개발 규칙

[English](AGENTS.md)

## 목적

flowmark는 markdown 문법을 가진 텍스트 DB다: 이 저장소는 엔진과 그 얇은 face, sync adapter(database,
GitHub, Jira), 응용 schema이고, [polyspec/orm](https://github.com/polyspec/orm)은 DSN `markdown://`으로
이것에 접속하는 query layer다. 판정은 `docs/plans/flowmark/plan.ko.md`와 그 옆의 계획에 있고, 각 계획은
`docs/plans/`에 자기 checklist를 갖는다.

## 원칙

모든 polyspec repository의 작업 원칙은 kit이 여기에 byte 그대로 vendor하는 [PRINCIPLES.md](PRINCIPLES.ko.md)이며,
이 repository의 모든 작업을 구속한다. 이 파일은 flowmark만의 규칙만 적는다: 아래 항목은 원칙을 엔진에 맞게
구체화한 것이고, 그다음 절은 이 repository의 규칙이다.

- Refines P17: polling이 아니라 event, 엔진은 `packages/go/cmd/flowmark/serve.go`에서 이렇게 적용한다: 파일 변경은 운영체제의 알림으로 알며, 잃은 event를
  알리는 알림(overflow)도 그 자체로 event이고 그 소비자는 영향받은 tree를 다시 읽는다 — 이 경로는 알림 규약이
  요구하므로 있으며 그렇게 적는다. 알림을 보내지 않는 파일 시스템은 지원하지 않으며 그것도 적는다. polling이나
  timer는 정공법의 장치가 없을 때만 그렇다고 적고 고르며, 결코 주 경로가 아니다.
  Enforced by: none, 리뷰에서의 판단이다.
- Refines P17: 멱등과 관측 가능성, 엔진은 `packages/go/irexec/executor.go`에서 이렇게 적용한다: journal replay, canon, install, 같은 bytes의 쓰기는 멱등이다.
  insert처럼 본래 멱등이 아닌 요청은 그 규칙대로 반복을 거부한다. event가 일어났다는 것은 찾아서가 아니라 그것이
  게시한 것으로 알게 된다. 죽은 process는 아무것도 게시하지 않으므로, 엔진은 시작할 때 자기 상태의 선언된 장소 —
  lock과 journal — 를 한 번 읽는다. 이것은 검색이 아니라 선언된 장소의 읽기다.
  Enforced by: none, 리뷰에서의 판단이다.
- Refines P17: 경로, 엔진은 `packages/go/store/corpus.go`에서 이렇게 적용한다: 엔진은 symlink를 만들지 않고 따라가지도 않는다. corpus에서 찾은 symlink는 그
  규칙대로 보고한다.
  Enforced by: none, 리뷰에서의 판단이다.

## 규칙

- Own: 공유 tool은 package `@polyspec/kit`이다. root `package.json`의 devDependency이며 release asset URL과 integrity로
  고정된다. Makefile은 옆의 kit checkout의 파일인 `../kit/scripts/kit/kit-bootstrap.mk`를 include하고, 그 `kit.copies`의
  파일(`config/layout.kit.yml`과 그것이 이름 붙인 schema 세 개)은 `make kit-sync`가 설치된 package에서 쓰고 `make kit-check`가
  비교하는 사본이며, `PRINCIPLES.md`, 그 짝, `.editorconfig`, `.claude/settings.json`은 `../kit`의 파일로 가는 link다(kit K614,
  K621). 바꿀 때는 kit에서 바꾼다. flowmark는 kit tool을 make target이나
  `$(KIT)/<tool>.$(KIT_EXT)`로 실행하고, 일반적인 필요는 자기 `package.json`에 선언된 생태계 library에서 얻는다.
  kit이 없거나 잘못된 tool은 여기서 복사하거나 고치지 않고 kit에 보고한다.
  Enforced by: `make kit-check`
- Own: `make flowmark-test`는 이 저장소 자체 코드의 전체 모음이다: `tests/`의 node test와 `scripts/go-test.mts`를
  실행하며, 일부 Go test가 필요로 하는 대소문자 구분 폴더를 마련한 뒤(macOS에서는 `var/` 아래의 Case-sensitive APFS
  image, Windows에서는 `fsutil`, 그 밖에서는 탐지하는 폴더) 그것으로 저장소의 각 Go module(`packages/go`, `examples`, `tests/conformance/go`)에서 `go test ./...`를 실행한다. 대소문자를 접는
  파일 시스템에서 맨 `go test ./...`를 실행하면 설계대로 그 test들이 실패하며, 필요한 변수를 알려 준다. CI workflow는
  Linux, Windows, macOS에서 같은 두 명령을 실행한다.
  Enforced by: `make flowmark-test`
- Own: 엔진은 database이므로 그 unit test는 촘촘하다: 모든 문법과 제약 규칙에는 허용과 거절의 사례가 최소 하나씩 있고
  표 주도이며 edge case를 포함한다. 모든 storage에는 모든 column type과 NULL 및 빈 형태를 덮는 P1/P2 golden
  fixture가 있다. parse와 emit은 golden fixture를 seed로 `go test -fuzz`로 fuzz하며, fuzzer가 찾은 실패 입력은
  회귀 사례로 남긴다. 규칙을 더하는 항목은 이 사례들이 생기기 전에는 끝나지 않는다.
  Enforced by: none, 리뷰에서의 판단이다.
- Own: 다중 플랫폼은 필수 요구다: lock, journal, watch, 파일 시스템의 플랫폼별 코드는 CI matrix의 각 플랫폼에서 돌고,
  이식 가능한 구현을 플랫폼 꼼수로 대신하지 않는다. lock은 kit의 holder-lock 선례를 따르며, 현재 요구를 만족하는 가장
  단순한 구현이 이긴다.
  Enforced by: `make flowmark-test`
- Own: 각 repository는 자기 checklist를 운영한다: 다른 repository의 작업은 거기서 추적하고, 여기서는 그 ID를 code span으로
  참조한다. 그래서 `make refs-check`는 다른 repository의 항목을 이 repository의 항목으로 읽지 않는다.
  Enforced by: `make refs-check`
- Own: Git 파일은 의존성이 아닌 외부 프로젝트를 이름으로 적지 않는다. polyspec repository(kit, orm, hyper, crudui,
  template 등)는 내부 library이므로 인용할 수 있다.
  Enforced by: none, 리뷰에서의 판단이다.
