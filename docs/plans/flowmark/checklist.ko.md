<!-- doc-id: execution-checklist -->
<!-- source-sha256: 4e449ac8b099529525fbb8e3d75a56a9f724727169d579ac8a8e53f1fff0bd0e -->
# 실행 체크리스트

| ID | Task | Deliverables | Verification | State |
|---|---|---|---|---|
| T1.0 | 문서 쌍과 이 checklist로 저장소를 bootstrap한다 | README, AGENTS, CHANGELOG, 이 checklist가 영어·한국어로 존재하고 `.flowmark/`가 ignore되며 `git log`에 bootstrap commit이 있다. | - | [o] |
| T1.1 | 적대 감사 결과를 설계 계획에 접는다 (T1.0 선행) | 감사 blocker 전부가 docs/plans/flowmark/plan.ko.md의 판정으로 나타나고, Korean twin이 English source-sha256을 가지며, 계획 개정이 commit된다. | - | [o] |
| T1.1-1 | 세 개의 face과 soksak sidecar 판정을 명시한다 (T1.1 후속) | 계획이 하나의 Go binary가 CLI·MCP server·soksak sidecar(protocol v1)의 세 thin face을 가지고, GUI는 자기 repo의 soksak plugin임을 명시한다. | - | [o] |
| T1.2 | polyspec/kit을 vendor하고 tracker를 구성한다 (T1.0 선행) | config/checklist.json과 config/documents.json을 작성한 상태에서 vendored tree에 `make kit-check`와 `make documents-check`가 통과한다. | - | [o] |
| T1.3 | stopgap ID 참조 검사기를 등록한다 (T1.2 선행) | docs/의 모든 task ID token이 checklist 항목으로 해석되고 그 검사가 문서 tooling의 일부가 된다. | - | [o] |
| T1.3-1 | 참조 검사는 kit 검사처럼 Git이 무시하지 않는 checkout의 file을 읽는다 (T1.3 후속). 원인: 검사가 directory tree 전체를 돌아, 무시되는 `.claude/worktrees/` 아래 agent worktree와 그 kit fixture 때문에 같은 checkout에서 `make documents-check`는 통과하고 `make refs-check`는 실패했다 | 무시되는 directory 아래의 문서는 읽지 않고 Git이 무시하지 않는 새 문서는 읽으며, 검사가 kit의 `trackedFiles`를 쓰고, agent worktree가 있는 상태에서 `node --test tests/check-refs.test.mjs`와 `make refs-check`가 통과한다. | - | [o] |
| T1.4 | 여섯 개의 core 명세를 전체 문서 쌍으로 쓴다 (T1.2 선행) | 모든 판정이 근거를 동반한 명제로 나타난다; T1.4.1부터 T1.4.7까지 완료되면 완료된다. | - | [o] |
| T1.4.1 | docs/spec/00-overview.md(.ko)를 쓴다 | - | - | [o] |
| T1.4.2 | docs/spec/10-canonical-format.md(.ko)를 쓴다 | - | - | [o] |
| T1.4.3 | docs/spec/20-document-model.md(.ko)를 쓴다 | - | - | [o] |
| T1.4.4 | docs/spec/30-schema.md(.ko)를 쓴다 | - | - | [o] |
| T1.4.5 | docs/spec/40-ops.md(.ko)를 쓴다 | - | - | [o] |
| T1.4.6 | docs/spec/50-constraints.md(.ko)를 쓴다 | - | - | [o] |
| T1.4.7 | 한국어 문서를 dry 문체로 다시 쓴다 — 비유 제거, 동작 동사 사용, 기술 용어 영문 유지 (T1.4.6 선행) | - | - | [o] |
| T1.4.8 | 문서 모델에 aggregate와 분할 shape를 명시한다: 단일 파일, 1:n 연결 파일, 1:n:1 (T1.4.7 후속) | - | - | [o] |
| T1.5 | 나머지 명세를 쓴다 (T1.4 선행) | capability table 전체, 범위를 정한 skeleton 3종, 그리고 100-checklist와 110-open-questions 전체; T1.5.1부터 T1.5.6까지 완료되면 완료된다. | - | [o] |
| T1.5.1 | capability table을 전체로 담은 docs/spec/60-dialect.md(.ko)를 쓴다 | - | - | [o] |
| T1.5.2 | 범위 skeleton으로 docs/spec/70-sync.md(.ko)를 쓴다 | - | - | [o] |
| T1.5.3 | 범위 skeleton으로 docs/spec/80-migration.md(.ko)를 쓴다 | - | - | [o] |
| T1.5.4 | 범위 skeleton으로 docs/spec/90-conformance.md(.ko)를 쓴다 | - | - | [o] |
| T1.5.5 | docs/spec/100-checklist.md(.ko)를 전체로 쓴다 | - | - | [o] |
| T1.5.6 | docs/spec/110-open-questions.md(.ko)를 전체로 쓴다 | - | - | [o] |
| T2 | Go codec: strict parser와 canonical renderer (T1.4 선행) | - | - | [o] |
| T2.1 | Go module을 document skeleton으로 시작한다: header, 제목, preamble, frontmatter block, H2 section을 canonical round-trip으로 parse하고 render한다 (T1.4 선행) | - | - | [o] |
| T2.2 | frontmatter를 YAML 1.2 core subset으로 parse하고 canonical로 재출력한다 (T2.1 선행) | - | - | [o] |
| T2.3 | child block — checklist 항목과 markdown table — 을 row로 parse한다 (T2.2 선행) | - | - | [o] |
| T2.4 | 모든 parse 규칙을 고정 rule 이름의 located diagnostic으로 보고한다 (T2.1 선행) | - | - | [o] |
| T2.5 | 이 저장소의 corpus에서 P1과 P2를 golden set으로 증명한다 (T2.1 선행) | - | - | [o] |
| T2.5-1 | corpus golden 테스트가 config/documents.json의 문서를 선택한다 (T2.5 후속). 원인: 그 walk가 네 개의 고정 디렉터리 밖의 markdown 파일을 전부 골라, 다른 패키지의 테스트 fixture가 테스트를 깨뜨렸다. | 패키지의 testdata 디렉터리 아래 markdown fixture가 있어도 `go test ./codec/`이 통과하고, 테스트는 `make documents-check`와 같은 문서를 읽는다. | - | [o] |
| T3 | Validator: corpus-global 제약 (T2 선행) | - | - | [o] |
| T3.1 | corpus의 key와 참조를 검증한다: doc-id와 item ID의 유일성, 산문의 모든 task-ID token 해석 (T2 선행) | - | - | [o] |
| T3.2 | config/documents.json의 선택으로 corpus directory를 검사하는 flowmark validate를 소유 명령으로 연결한다 (T3.1 선행) | - | - | [o] |
| T4 | Executor: parse된 corpus 위의 IR kind (T3 선행) | - | - | [o] |
| T4.1 | corpus row 위에서 one·all·count를 where·order·limit와 함께 실행한다 (T3 선행) | - | - | [o] |
| T4.2 | sum·avg·paginate·group_count를 type-directed ordering으로 실행한다 (T4.1 선행) | - | - | [o] |
| T4.3 | markdown table에서 항목을 읽는다: header row가 state·id·title column을 정의하는 table이 그 문서의 항목 목록의 table 형태이며 행 순서대로 항목이다 (T4.2 선행) | `All("items")`이 그런 table의 행을 state glyph·ID·title·문서별 order와 함께 반환하고 codec이 모든 data row의 절대 줄을 기록한다. | - | [o] |
| T5 | orm markdown-executor 통합: manifest setting과 state-machine sub-grammar (T4 선행) | - | - | [o] |
| T6 | Sync adapter: 계약과 database adapter (T5 선행) | - | - | [o] |
| T6.1 | corpus를 SQL로 export한다: row의 derived·deterministic INSERT dump (T4 선행) | - | - | [o] |
| T6.2 | rowset을 provenance header와 readonly drift 규칙으로 markdown에 materialize한다 (T6.1 선행) | - | - | [o] |
| T7 | 세 개의 face — CLI·MCP server·soksak sidecar — orm model API 위의 thin adapter (T5 선행) | - | - | [o] |
| T7.1 | CLI face: corpus 위의 validate·items·transition (T3 선행) | - | - | [o] |
| T7.1-1 | CLI items answer가 path·assignee·due column을 실는다 — sidecar answer와 같은 column이고 executor 행이 path를 직접 가진다 (T7.1 후속) | assignee와 due가 있는 corpus에서 `flowmark items <root>`가 path·assignee·due로 끝나는 6개 tab column을 출력하며 writer에 대한 test가 있다. | - | [o] |
| T7.2 | MCP server face: stdio 위의 initialize·tools/list·tools/call (T7.1 선행) | - | - | [o] |
| T7.3 | soksak sidecar face: stdio 위의 protocol v1 message loop (T7.1 선행) | - | - | [o] |
| T7.3-1 | sidecar와 MCP의 각 message를 즉시 flush한다. 원인: Serve가 bufio flush를 입력이 끝날 때까지 미뤄서, session을 열어 두는 host — 예제 Wails host, MCP client — 가 process가 멈출 때까지 응답을 받지 못했다. protocol과 JSON-RPC session 모두 session이 진행되는 동안 각 message를 전달한다. 완료 기준: 모든 응답이 write 시점에, 입력이 끝나기 전에 flush되고 pipe가 열려 있는 상태에서 응답 한 건을 읽는 회귀 시험이 이를 덮는다. | - | - | [o] |
| T8 | Checklist application: local issue model (T3 선행) | - | - | [o] |
| T8.1 | 전환 table을 인코딩한다: terminal, 필수 field, superseded 항목 생성 (T3 선행) | - | - | [o] |
| T8.2 | corpus에 전환을 적용한다: 상태 재작성, revision 줄 추가, WIP gate 유지 (T8.1 선행) | - | - | [o] |
| T8.3 | `flowmark items`와 `flowmark transition`을 소유 명령으로 연결한다 (T8.2 선행) | - | - | [o] |
| T8.4 | 이슈를 write path로 입력한다: add가 유일한 ID의 대기 항목을 추가하고 rm이 하나를 지우며 하위를 가진 부모는 거부한다 — CLI·MCP tool·sidecar operation 모두 (T8.3 후속) | - | - | [o] |
| T8.4-1 | sidecar add가 path 없는 요청을 첫 문서로 보내며 answer가 문서가 정한 ID를 말한다 (T8.4 후속) | path를 실어줄 items answer가 아직 없는 빈 corpus에 sidecar로 첫 이슈를 추가하면 첫 문서에 항목이 쓰이고 ` added`가 아니라 `T1 added`로 답한다. | - | [o] |
| T8.4-2 | repository는 빌드된 실행 파일을 추적하지 않는다: 루트의 `flowmark`를 추적에서 빼고 ignore하며, build는 `build/`에만 쓴다 (T8.4 후속). 원인: repository 루트에서 실행한 `go build`가 4.4 MB 실행 파일 `flowmark`를 썼고 T8.4 commit이 그것을 담았다 | `git ls-files`에 빌드된 실행 파일이 없고, 루트에서 `go build ./cmd/flowmark`를 해도 `git status`가 깨끗하며, `make build`는 `build/flowmark`에만 쓴다. | - | [o] |
| T8.4-3 | `go vet`이 예전 write path를 통과한다: `MoveItem`이 옮길 줄을 append 한 번으로 복사한다 (T8.4 후속). 원인: T8.4 commit이 복사를 값이 없는 두 번째 `append`로 감쌌고, `go vet`이 이를 보고한다 | `go vet ./...`이 app/에 대해 아무것도 보고하지 않고, `go test ./app/`이 통과한다. | - | [o] |
| T8.4-4 | sidecar/의 `TestAddWithoutPathOnEmptyCorpus`가 매번 통과한다 (T8.4 후속). 원인: 격리 실행 15번 중 한 번, add 응답 앞에 `changed` message가 하나 더 와서 실패했다. watcher의 변경 message와 operation 응답의 순서가 정해져 있지 않다 | 원인을 밝혀 재시도가 아니라 제거로 고치고, `go test -count=200 -run TestAddWithoutPathOnEmptyCorpus ./sidecar/`와 `go test -race ./sidecar/`가 통과한다. | - | [o] |
| T8.5 | Mermaid block이 유일한 transition 권위다: Transition이 edge·필수 필드·glyph를 문서 자신의 state machine에서 읽고 AddItem이 `[*]` 줄에서 초기 상태를 취하며 ApplySchema가 Mermaid block을 다시 쓰고 — 내장 table은 남지 않는다 (T8.4 후속) | 사용자 정의 상태 이름을 가진 문서가 자신이 선언한 edge대로 전이한다 — 선언 안 된 edge는 거부, edge label이 필수 evidence를 정하고 새 항목은 선언된 초기 상태에 놓인다 — 그리고 Go source에서 hardcoded state map을 `grep`으로 찾을 수 없다. | - | [o] |
| T8.6 | write path가 table 형식을 섬긴다: transition·edit·add·remove·move가 items table의 행 cell을 다시 쓴다 — 문서당 하나의 문법, 행과 table의 혼재는 거부한다 (T8.5 후속, T4.3 선행) | table 형식 문서에서 transition이 문서 자신의 edge대로 state cell을 다시 쓰고 add가 자동 ID로 행을 추가하며 edit이 title cell을 고치고 move가 행 순서를 바꾸고 remove가 행을 지운다. | - | [o] |
| T7.3-2 | corpus를 감시해 changed event를 push한다: sidecar가 열린 surface root의 fingerprint를 poll하고 push하며 page가 session 동안 갱신된다 (T7.3-1 후속) | - | - | [o] |
| T7.3-3 | sidecar 답은 온전한 한 줄로 남고 watcher의 기준 fingerprint는 첫 답보다 앞선다: poll goroutine과 요청 loop가 하나의 writer를 하나의 lock으로 공유하고, root의 fingerprint는 첫 답을 만들기 전에 잡는다 (T7.3-2 후속) | 동시에 나가는 changed event와 답이 한 줄 안에서 섞이지 않고(`-race` 아래 test가 온전한 JSON 줄만 읽는다), root의 첫 items 답 직후의 편집도 changed event를 push한다. | - | [o] |
| T9 | Conformance vector: corpus before, request, 기대 corpus after (T4 선행) | - | - | [o] |
| T9.1 | vector 형식을 정의하고 첫 vector를 executor로 실행한다 (T4 선행) | - | - | [o] |
| T10 | 자기 repo의 soksak plugin: 칸반 보드와 issue tracker — 또 하나의 thin consumer (T8 선행) | - | - | [o] |
| T10-1 | 보드는 문서가 갖지 않은 상태를 보여주지 않는다. 원인: drop이 전이를 판정하기 전에 카드를 옮겼고, 거절(done → doing처럼 선언되지 않은 edge, 또는 evidence가 필요한 edge)은 되돌림 없이 return해서 markdown은 상태를 지키는데 카드는 틀린 column에 남았다. 빈 items 답(마지막 item 삭제)은 toast로만 보이고 옛 카드를 남겼고, changed event는 items만 새로 읽고 state machine은 읽지 않아 Mermaid block을 고치면 column이 낡았다 (T10 후속) | 순수 함수 `planDrop`이 DOM을 옮기기 전에 모든 drop을 판정하고(거절은 아무것도 옮기지 않고 보드를 새로 읽는다), 모든 operation은 수락이든 거절이든 items 새로고침으로 끝나며, 빈 답은 column을 비우고, changed event는 schema를 다시 읽어 달라졌을 때만 column을 다시 만든다. plugin의 `node --test`가 각 경우를 덮는다. | - | [o] |
| T10-2 | 새 이슈 row는 column 바닥에 붙지 않고 그 column의 마지막 카드 바로 아래에 따라온다. column 전체가 drop을 받는다. 원인: 카드 목록이 column을 채우도록 늘어나서(`flex:1`) 키 큰 이웃에 맞춰 늘어난 column은 카드와 row 사이에 틈이 생겼고, 그 틈이나 row 위의 drop은 목록을 찾지 못했다 (T10-1 후속) | row가 짧은 column과 빈 column에서도 마지막 카드 바로 아래에 놓이고, column 안 어디에 drop해도 그 column의 목록에 들어가며, `node --test ui/`가 drop 대상을 덮는다. | - | [o] |
| T10-3 | 모든 column은 보드의 전체 높이를 쓴다. 어떤 column도 이웃에서 높이를 받지 않는다. 원인: 보드 grid가 row를 가장 긴 column에 맞췄고(`align-content:start`) 짧은 column이 거기에 늘어나서, 높이가 가장 붐비는 column을 따랐다 — 독립도 전체도 아니었다 (T10-2 후속) | 렌더된 창에서 카드가 어떻든 모든 column이 보드 위에서 아래까지 뻗고, 카드가 넘치는 column은 자기 안에서 스크롤하며, 렌더링을 screenshot으로 관측한다. | - | [o] |
| T10-4 | 새 이슈 row는 column 맨 아래에 고정된다: 카드 목록이 column을 채우고 자기 안에서 스크롤하며, 카드가 몇 개든 row는 그 아래에 계속 보인다 (T10-3 후속 — T10-3이 모든 column에 보드 전체 높이를 주었다) | 렌더된 창에서 row가 짧은 column, 빈 column, 넘치는 column 모두 column 바닥에 놓이고, 렌더링을 screenshot으로 관측한다. | - | [o] |
| T10-5 | plugin repository는 빌드된 실행 파일을 추적하지 않는다: `example/example`은 ignore하고 `go build`로 다시 만든다 (T10 후속). 원인: example host binary(16 MB)가 example host와 함께 commit되었고, 다시 빌드한 사본이 계속 변경으로 보였다 | plugin repository의 `git ls-files`에 `example/example`이 나오지 않고 재빌드해도 `git status`가 깨끗하다. | - | [o] |
| T11 | 항목에 사람과 날짜: 항목 title이 `@assignee`와 마감 date·datetime(`YYYY-MM-DD`, `YYYY-MM-DDTHH:MM[:SS]`, zone 선택)으로 끝날 수 있고 문서가 사람 table(`id`/`name` header)을 선언된 사람의 registry로 가지며 빈 담당자는 self다 — 이름을 추측하지 않는다 — items·people surface가 이들을 노출한다 (T4.3 선행) | codec이 끝 token들을 title에서 분리하고 시간이 붙은 마감도 받으며 `All("items")`이 list·table 항목 모두에 assignee·due를 실고 `All("people")`이 사람 행을 반환하며 sidecar의 items answer가 assignee·due column을 얻고 `people` answer를 가진다. | - | [o] |
| T13 | Git이 모든 commit message와 push를 검사한다: `config/checklist.json`이 hook `pre-push`와 `commit-msg`를 나열하고, `.githooks/commit-msg`가 message에 kit의 `check-commits.mjs`를 실행하며, `make hooks`가 `core.hooksPath`를 설정한다. 원인: hook이 설치된 적이 없고 `commit-msg`가 나열되지 않아, commit 5b983ec가 subject에 필요한 `(#task)` 없이 들어갔다. `check-commits.mjs`를 실행하면 그 subject를 거절한다 | `make hooks-check`가 통과하고, `config/commits.json`을 어기는 subject의 commit을 hook이 거절한다. | - | [o] |
| T14 | restore가 request hook 뒤에 key를 고른다(orm protocol 1.5, orm `T140`). 원인: orm `T140`은 planner가 hook 뒤에 set 값과 and로 이은 eq 조건에서 restore의 key를 고르게 하고 model은 set만 보내는데, flowmark executor는 where에서만 key를 읽었다. 인수 조건: executor가 orm의 `ir.RestoreKey`에서 key, 조건, 새 값을 얻는다; docs/spec/60-dialect.md와 50-constraints.md(+ko)가 이를 적는다; irexec restore case가 set에서 온 key, unique key, 좁히는 조건, key 없음, `or`를 덮으며 red를 먼저 본다. 증거: 변경 전 새 irexec restore case는 executor가 where에서만 key를 읽어 실패했다; executor는 이제 orm의 ir.RestoreKey에서 key, 조건, 새 값을 얻으며, irexec, catalog, introspect, ops, store, sync, validate test가 orm v0.0.11에서 통과한다; orm의 markdown conformance가 이 executor로 restore case를 통과한다. | - | - | [o] |
| T14-1 | release된 orm을 고정한다(T14에 이어). 원인: T14는 아직 어느 orm release에도 없는 orm의 `ir.RestoreKey`가 필요하므로, flowmark는 release된 orm tag를 받는다는 규칙과 달리 push된 orm `T140` commit을 pseudo-version으로 고정한다. 인수 조건: go.mod가 `T140`을 release하는 orm tag를 고정한다; 이것이 pseudo-version 고정의 제거 조건이다. 증거: orm `T140` commit의 pseudo-version 고정은 main에 오지 않았다: merge한 go.mod가 `T140`을 담은 release인 orm v0.0.11을 고정한다. | - | - | [o] |
| T16 | markdown executor가 request parameter의 offset_p와 count_p로 canonical ORM root page limit을 받는다 | the ORM IR decoder dependency used by irexec, and native executor cases for a page with parameterized offset and count | `cd packages/go && go test ./irexec -run ^TestParameterizedPageLimit$` | [o] |
| T17 | TestLockWaitOverflowRetries는 첫 wake가 시작한 attempt가 link를 마치기 전에 lock을 놓을 수 있다. 그러면 waiter는 그 wake에 획득하고, 테스트는 attempt를 2가 아니라 1로 센다. 원인: 테스트의 tried 신호는 lock-attempt step이 시작할 때 보내지며(lock.go:195), 그 attempt가 os.Link를 부르기(lock.go:198) 전이다. windows-latest의 run 41027928190에서 해제가 그 link보다 먼저 왔다(wait_test.go:181) | TestLockWaitOverflowRetries holds its third attempt at the start of that attempt, by the lock-attempt step the code already publishes, and releases the lock only while that attempt is held, so no ordering of the attempt before it is assumed; no sleep and no retry loop is added. The red of the old test under the same ordering is shown in the commit body. | `cd packages/go && go test ./store/journal -run ^TestLockWait -count=50` `cd packages/go && go vet ./store/journal` | [o] |
| T18 | Windows 실행 41027928190에서 BenchmarkShardInsert sharded events=20000이 120초 간격을 넘겨 시간 초과. 원인은 미확정: setup 반복 가설은 반증됨(testing/benchmark.go:331-352 launch가 본문을 한 번 실행), insert마다 Windows watch가 overflow를 내는지는 미확정(store/journal/tree_windows.go:63,72,81이 Overflow 설정, tree.go:105-109가 tree를 비우고 corpus.overflow 발행). macOS는 overflow를 보고하지 않으므로(tree_darwin.go 머리말) 같은 시간은 증거가 아님. macOS에서 insert마다 두 layout 모두 corpus.changed 2개, corpus.read는 flat 4.4, sharded 6.2(추가 목록 /events/2026-10). 벤치마크는 두 수치를 보고하지 않음 | packages/go/writepath/bench_shard_test.go BenchmarkShardInsert reports overflows/op and tree-reads/op per op; TestShardInsertMetrics counts a forced overflow; the fix waits for the Windows measurement of these metrics | cd packages/go && go test ./writepath -run ^TestShardInsertMetrics$ -count=1 && cd packages/go && go test ./writepath -run xxx -bench 'ShardInsert/.*/events=5000' -benchtime 3x -count=1 | [ ] |
