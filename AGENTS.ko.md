<!-- doc-id: agents -->
<!-- source-sha256: df8e2b2d22460d512dc2dc525ba16214e6ecdb8c345f1c8bb6c6f3ec9c955572 -->
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

- Own: 공유 tool은 설치된 package `@polyspec/kit`이며 release asset URL과 lock integrity로 고정한다.
  Makefile은 `node_modules/@polyspec/kit/scripts/kit/kit-bootstrap.mk`를 include한다. `make kit-sync`는 선언된 사본을 쓰고
  `make kit-check`는 비교한다. 공유 파일은 kit에서 바꾼다. flowmark는 make target 또는 `$(KIT)/<tool>.$(KIT_EXT)`로
  kit tool을 실행하고 생태계 library는 자신의 package manifest에 선언한다.
  Enforced by: `make kit-check`
- Own: test 탐색, 실행과 대소문자 구분 자원은 Kit이 소유한다. `make test-go`는 추적된 `go.work`의 모든 module을,
  `make test-infrastructure`는 루트 TypeScript test를 실행한다. `CASE_SENSITIVE_ENV`는 Kit이 준비하고 정리하는
  `FLOWMARK_CASE_SENSITIVE_DIR`를 지정한다. CI는 언어별 잠긴 사전 의존성을 독립 설치하고 Linux, Windows, macOS에서
  provider의 build, test, gate 경로만 사용한다. benchmark는 Kit native runner로 writepath test와 함께 실행한다.
  Kit 지원 부족을 consumer runner로 대체하지 않고 Kit issue로 보고하며 모든 필수 검사와 provider release proof가
  성공해야 release한다.
  Enforced by: `make kit-check`
- Own: release는 `main`에서 성공한 `.github/workflows/ci.yml` push run의 id로 하는 `.github/workflows/release.yml`의
  dispatch다. 그 run의 job `ci-passed`는 kit의 gate를 포함한 다른 모든 job을 needs로 가진다:
  `gh workflow run release.yml -f run=<id>`. 그 version X.Y.Z는 그 run의 commit에 있는 CHANGELOG.md의 가장 새로운 section
  `## X.Y.Z`이며, `## Unreleased`에 항목이 있는 동안 release는 거부된다. `make release-tag`가 그 commit에 annotated tag
  `vX.Y.Z`와 `packages/go/vX.Y.Z`를 만든다. 손으로 tag를 만들고 옮기고 push하는 사람은 없으며, push한 tag는 release를
  시작하지 않는다.
  Enforced by: `make release-tag`
- Own: 엔진은 database이므로 그 unit test는 촘촘하다: 모든 문법과 제약 규칙에는 허용과 거절의 사례가 최소 하나씩 있고
  표 주도이며 edge case를 포함한다. 모든 storage에는 모든 column type과 NULL 및 빈 형태를 덮는 P1/P2 golden
  fixture가 있다. parse와 emit은 golden fixture를 seed로 `go test -fuzz`로 fuzz하며, fuzzer가 찾은 실패 입력은
  회귀 사례로 남긴다. 규칙을 더하는 항목은 이 사례들이 생기기 전에는 끝나지 않는다.
  Enforced by: none, 리뷰에서의 판단이다.
- Own: 다중 플랫폼은 필수 요구다: lock, journal, watch, 파일 시스템의 플랫폼별 코드는 CI matrix의 각 플랫폼에서 돌고,
  이식 가능한 구현을 플랫폼 꼼수로 대신하지 않는다. lock은 kit의 holder-lock 선례를 따르며, 현재 요구를 만족하는 가장
  단순한 구현이 이긴다.
  Enforced by: `make test-go`
- Own: 각 repository는 자기 checklist를 운영한다: 다른 repository의 작업은 거기서 추적하고, 여기서는 그 ID를 code span으로
  참조한다. 그래서 `make refs-check`는 다른 repository의 항목을 이 repository의 항목으로 읽지 않는다.
  Enforced by: `make refs-check`
- Own: Git 파일은 의존성이 아닌 외부 프로젝트를 이름으로 적지 않는다. polyspec repository(kit, orm, hyper, crudui,
  template 등)는 내부 library이므로 인용할 수 있다.
  Enforced by: none, 리뷰에서의 판단이다.
