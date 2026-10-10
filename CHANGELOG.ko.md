<!-- doc-id: changelog -->
<!-- source-sha256: 06ba31105f0ba9f3155ce4ee6347ca6a3596e2b2913a39869926237194c2023a -->
# Changelog

## Unreleased

- T8.4-4: `TestAddWithoutPathOnEmptyCorpus`가 매번 통과한다. 이 test는 add 응답이 watcher의 변경
  event보다 먼저 온다고 가정했지만, 그 순서는 정해져 있지 않다: watcher는 add 자신의 쓰기를 보고하고,
  sidecar는 변경 event를 언제든 밀어 보낼 수 있다. 이제 test는 문자열 응답 하나가 `T1 added`이고,
  나머지 message는 모두 이 surface의 changed event임을 확인한다. `Serve`도 poll goroutine이 출력에
  아직 쓸 수 있는 동안 반환하지 않는다: `stop`이 poll loop들을 기다리며, `go test -race`가 출력
  buffer에서 보고한 data race를 없앤다. poll loop는 멈추면 두 poll 사이의 대기를 마저 채우지 않고
  바로 끝난다.

- T12.1-3: spec이 `files` codec에 필요한 판정을 적는다. 읽기에서 없는 non-null field나 section, 렌더링이 inline markdown을 넘어 텍스트와 다르게 보이는 한 줄 값, `-` 항목으로 시작하는 `files` body, field list 뒤에서 첫 줄이 두 column 이상(공백 또는 tab) 들여쓴 `files` body는 `format.block`이고, 렌더링이 한 줄 자리 둘레에서 떨어뜨리는 공백은 `format.noncanonical`이다. BOM이나 CR은 `format.noncanonical`이고, UTF-8이 아닌 byte나 U+0000은 `format.encoding`이다. 같은 읽기 결함은 쓰기에서 `CONSTRAINT`다. column type이 아닌 값, dangling 외래 key, 대소문자를 무시하고 겹치는 key는 validator 규칙(`type`, `reference.dangling`, `key.duplicate`)이며, dangling 외래 key나 중복 key를 새로 만드는 쓰기는 각각 `FOREIGN_KEY`, `DUPLICATE_KEY`다. 본문이 아닌 non-null `markdown` column은 언제나 있는 section이다. 마지막 `#` 연속이 공백이나 tab 뒤에 오거나 `#`뿐인 제목은 마지막 `#`을 `\#`로 쓰고, `\#`로 끝나는 제목은 거절한다. markdown 값은 줄 바꿈으로 시작하거나 끝나지 않는다. field list 뒤의 빈 줄은 비어 있지 않은 body나 section 앞에서만 서며, 문서는 LF 하나로 끝난다. `format.*` 진단이 있는 file을 다시 쓰는 logical write는 `CONSTRAINT`로 거절하며, `format.noncanonical`이면 `flowmark canon`을 밝힌다. spec 40은 문서 바이트를 정규화하는 유일한 연산인 `flowmark canon`을 추가하고, `format.*` 진단이 있는 file을 다시 써야 하는 commit을 거절한다. 거절은 `flowmark canon`의 대상, 추가된 column이 일으킨 진단에 대한 add-column migration, `files` 행의 file 전체를 지우는 delete를 면제하며, `format.block`, `format.section`, `format.encoding`이 있는 file을 가진 table은 그 행이 필요한 요청과 검사에 사용할 수 없다. `flowmark install`은 `.gitattributes`에 줄을 더하고, table folder가 없으면 빈 table이며, typed 값은 쓰기에서 canonical 형식으로 쓴다. table은 행이 있을 수 있는 모든 file을 가지며, block table은 placement parent의 file을 포함한다. 바이트 단위로 같은 key를 가진 두 행은 table을 사용할 수 없게 하고, 잘못된 type의 값을 평가하는 요청은 `CONSTRAINT`로 실패하며, 사용할 수 없는 table이 필요한 export는 실패하고 아무것도 쓰지 않는다. `corpus.walk` 항목이나 다른 대소문자의 suffix가 table의 자리에 있으면 사람이 corpus를 고치기 전까지 그 table은 사용할 수 없고, 고치면 다음 읽기에서 다시 사용할 수 있다. `type` 진단은 제약 검사가 그 값을 평가해야 할 때에만 commit을 실패시키며, 그 값을 읽는 export는 실패한다. `files` codec의 테스트 사례는 고정하는 spec section을 밝힌다.

- T1.3-1: 참조 검사는 kit 검사처럼 kit의 `trackedFiles`로 Git이 무시하지 않는 checkout의 file을 읽는다.
  `.claude/worktrees/` 아래 agent worktree 같은 무시되는 directory가 더 이상 결과를 바꾸지 않는다.

- T12.2.1-2: markdown table의 모든 외래 key는 하나의 column이며 어떤 table의 단일 column primary key를 가리킨다.
  composite 외래 key, 또는 unique이지만 key가 아닌 column을 가리키는 외래 key는 그 외래 key의 위치에 놓인
  `catalog` 진단이다. 행으로의 link는 그 행의 단일 column key만 가진다.

- T12.2.2: `files` 저장이 행마다 `<table>/<key>.flowmark.md` 파일 하나를 parse하고 render한다. H1
  제목(`\#` escape 포함), 저장되는 scalar column의 field list(읽을 수 있는 값 형식), 본문, nullable
  markdown column의 level-2 section, 외래 key의 link(목표는 `Linker`가 준다)를 다룬다. literal은 고정된
  orm commit `72e2b535`의 `CanonicalLiteral`로 읽는다. 비정규 표기는 `format.noncanonical`이다. 행은
  돌려주고, 그것을 emit하면 정규 file이 된다. BOM은 비정규 `format.noncanonical`이고, 잘못된 UTF-8은 치명적인
  `format.encoding`이다. 제 자리에 없는 file, symbolic link, 문서 이름의 folder, 다른 대소문자의 suffix는
  `corpus.walk`다. key 규칙을 깨는 key는 `format.block`이고, 규칙이 거절하는 쓰기는 `CONSTRAINT`다. child
  block section은 T12.2.4가 읽기 전까지 `format.section`이다. golden fixture는 byte 단위로 round-trip하며,
  parse와 emit은 fuzz한다.

- T12.1-2: 상세 heading은 `<h> <table> row <key>`이고 anchor는 `<table>-row-<소문자 key>`다. 여러 부모 아래
  반복되는 block이나 markdown column heading은 더 이상 상세 anchor를 가져갈 수 없다(`items-1`은 `items row 1`이
  아니다). key는 `--`를 담지 않고 `-`로 끝나지 않는다. 상세 anchor는 GitHub slug의 두 중복 처리(github-slugger와
  github.com) 모두에서 비어 있어야 하며, 그것을 가져가는 heading은 쓰기에서 `CONSTRAINT`, 읽기에서
  `format.block`이고, 공백이 다른 표기는 `format.noncanonical`이다. link는 GitHub식 heading slug를 쓰는 viewer에서
  이어진다고 명시한다. catalog의 same-file 이름 규칙은 철회한다: markdown column이나 block table은 같은 file의
  다른 table과 같은 이름일 수 있다. 자식 table이 부모의 markdown column과 같은 이름이면 거절하는 규칙은 남는다.

- T12.2.1-1: catalog는 `.dbs` 집합만 읽는다. `dbspec/flowmark.json`과 그 `translations` 필드를
  settings reader, Catalog API의 `Translations` 필드와 함께 제거한다. `dbspec/`의 다른 항목, 즉 다른
  이름의 file이나 folder는 모두 `catalog` 진단이며, 숨김 항목은 건너뛴다. spec 30의 이름 규칙을
  적용한다. Windows 장치 이름(`con`, `prn`, `aux`, `nul`, `com0`-`com9`, `lpt0`-`lpt9`)과 같은 table
  이름은 schema 오류다. 부모의 markdown column과 같은 이름의 child block table은 schema 오류다.
  같은 file에 상세 section이 올 수 있는 다른 table(document table과 그 block 후손, files table의
  block 후손)과 같은 이름의 markdown column 또는 block table은 schema 오류다. 이름 비교는 대소문자를
  무시한다. fixture catalog는 더 이상 `dbspec/flowmark.json`을 갖지 않는다.

- T12.1-1: spec이 읽을 수 있는 문서 판정을 명시한다. corpus 문서는 평범한 markdown 문서로 읽히는
  `*.flowmark.md` file이며 comment, frontmatter, doc-id, provenance, twin marker가 없다. `files` 행은
  `<table>/<key>.flowmark.md` file이고 key가 file 이름이며, key는 이식 가능한 file 이름이자 anchor다.
  spec 10은 field list, 읽을 수 있는 값 형식, 한 줄 자리, `<h> <table> <key>` 상세 heading과 anchor,
  섹션 순서, 파생 target을 가진 link로서의 외래 key를 정하고, spec 20과 50은 walk, key 규칙,
  `corpus.walk`를 정하며, spec 30은 `dbspec/flowmark.json`을 철회하고, sync provenance는 문서 밖으로
  옮긴다(open question 5). design plan, spec 00, spec 80은 철회된 흔적을 지우고, 계획은 polyspec
  스택의 board 앱을 더하고 Wails 예제 host를 유지한다.

- T2.5-1: corpus golden 테스트가 네 개의 고정 디렉터리 밖의 모든 markdown 파일 대신 `config/documents.json`이 고른 문서를 읽는다. `make documents-check`와 같은 선택을 쓴다. 패키지의 `testdata` 디렉터리 아래 markdown fixture가 더는 `go test ./codec/`을 깨지 않으며, `config/documents.json`은 `**/testdata/**`를 문서에서 제외한다.

- T12.2.1: `catalog` package가 corpus root의 catalog를 적재한다. orm의 `engine/dbspec`으로 `dbspec/*.dbs`
  집합을, `dbspec/flowmark.json` 번역과 함께 읽으며, spec이 명한 각 경우를 위치가 정해진 진단으로 거절한다:
  catalog 없음, `store` 없는 table, 복합 key, `aes`/`aes_version`/`blind_index`, `audit`/`immutable`,
  `cascade`/`set_null`, 외부 `use`, `history` 또는 `initial` 없는 state machine, `title` 없는 list 또는
  files 저장, H6을 넘는 제목 깊이, block 순환. 적재된 catalog는 table마다 저장, key, title, body, order,
  markdown column과 state machine을 가진다. glyph 표기를 바로잡는다: spec 100, spec 30과 markdown database
  계획은 checkbox glyph를 `'[ ]'` 등 대신 대괄호 사이의 한 글자(`waiting ' '`, `doing '~'`, `done 'o'`,
  `bypassed '!'`, `impossible '-'`)로 쓴다. orm의 `checkbox` setting이 정확히 한 글자를 받기 때문이다.

- T12.1: 판정과 spec을 markdown database 계획 위에서 다시 쓴다. flowmark가 엔진이고 orm은 `markdown://`으로
  IR executor로서 접속한다. `dbspec/` catalog가 유일한 schema이고, 각 table은 `files`, `document`,
  `block` 저장을 선언하며, key는 불투명하다. canonical format은 행 scope, 섹션, list field, table cell을,
  operations는 server, session, commit 순서, 실패 code를, constraints는 이력·빈틈·사슬·repair를 갖는
  상태기계를 고정하며, `flowmark.dbs`가 응용 schema다. design plan은 revision 3을 기록하고 문서 안
  Mermaid authority(T8.5)는 철회한다.

- T8.4-3: `go vet`이 예전 write path를 통과한다. `MoveItem`은 옮길 줄을 `append` 한 번으로 복사하며,
  T8.4가 더한 값이 없는 두 번째 `append`는 제거한다.

- T8.4-2: repository는 빌드된 실행 파일을 추적하지 않는다. repository 루트의 `go build`가 쓴 루트의
  `flowmark`는 index에서 빠지고 ignore되며, `make build`는 `build/flowmark`에 쓴다.

- T10-5: plugin repository는 빌드된 실행 파일을 추적하지 않는다. 16 MB example host binary는
  ignore하고 `go build -o example .`로 다시 만든다. 변경은
  plugin repository에 있다.

- T10-4: 새 이슈 row는 column 맨 아래에 고정된다. 카드 목록이 column을 채우고 자기 안에서 스크롤
  하므로 카드가 몇 개든 row가 계속 보인다. 변경은 plugin repository에 있다.

- T10-3: 보드의 모든 column이 가장 붐비는 column의 높이가 아니라 보드의 전체 높이를 쓰고, 카드가
  넘치면 자기 안에서 스크롤한다. 변경은 plugin repository에 있다.

- T10-2: 새 이슈 row는 column의 마지막 카드 바로 아래에 따라오고 column 전체가 drop을 받는다.
  카드 목록이 column을 채우지 않아 늘어난 column에도 틈이 없다. 변경은 plugin repository에 있다.

- T10-1: 보드는 문서가 갖지 않은 상태를 보여주지 않는다. 거절된 drop(done → doing, 또는
  evidence가 필요한 edge)이 카드를 drop한 column에 남기고 markdown은 상태를 지켰다. plugin은 이제
  옮기기 전에 drop을 판정하고, 모든 operation 뒤에 문서에서 다시 그리며, 빈 답에 column을 비우고,
  changed event에 state machine을 다시 읽는다. 변경은 plugin repository에 있다.

- T7.3-3: sidecar 답은 온전한 한 줄로 남는다. watcher의 poll goroutine과 요청 loop가 lock 없이
  하나의 buffered stream에 썼기 때문에 push된 changed event가 답 줄을 찢을 수 있었고, 그러면
  page가 답 하나와 corpus에 대한 view를 잃었다. 이제 하나의 lock이 stream을 지키고, session
  종료도 그 lock 아래서 flush하며, root의 fingerprint 기준선은 surface를 watch할 때 — 첫 답보다
  앞서 — 잡아 그 답 직후의 편집도 감지한다.

- T7.1-1: `flowmark items`가 sidecar가 섬기는 것과 같은 항목을 출력한다: path·assignee·due로
  끝나는 6개 tab column. executor 행이 path를 직접 실어 sidecar는 documents join을 버린다.

- T8.6: write path가 table 형식을 섬긴다. 항목이 table에 사는 문서가 list 형식과 같은
  명령을 받는다: transition이 문서 자신의 edge대로 state cell을 다시 쓰고 add가 자동
  ID로 행을 더하며(NextID가 table 행을 읽는다) edit이 title cell을 고치고 move가 행
  순서를 바꾸고 remove가 행을 지운다. 문서당 하나의 문법 — checklist 행과 items
  table의 혼재는 거부한다.

- T11: 항목이 사람과 날짜를 가진다. 항목 title은 `@assignee`와 `YYYY-MM-DD` 마감으로 끝날 수
  있고 — codec이 끝 token을 title에서 분리한다 — 문서는 `id`와 `name` column을 정의하는 사람
  table을 가진다. 첫 행이 owner다: 담당자 없는 항목은 owner의 것이다. `All("items")`이
  list·table 항목 모두에 assignee·due를 얻고 `All("people")`이 사람 행을 반환하며 sidecar의
  items answer가 두 column을 실고 `people` answer를 가진다.
- T11 정정: 사람 table은 선언된 사람의 registry이지 owner 선언이 아니다 — 빈 담당자는
  self로 읽히고 board는 이름을 추측하지 않는다. 담당자는 하나의 일관된 형태로 id(`@max`)를
  보여주고 마감은 시간을 가질 수 있다(`YYYY-MM-DDTHH:MM[:SS]`, zone 선택).

- T4.3: 항목은 table 형태로도 존재한다. header row가 state·id·title column을 정의하는 markdown table이
  그 문서의 항목 목록이다 — 문서가 쓴 header가 정의다 — 그리고 `All("items")`이 행 순서대로 행을
  반환한다. codec이 모든 data row의 절대 줄을 기록해 write path가 그들을 지목할 수 있다.

- T8.5: 문서의 Mermaid block이 유일한 transition 권위다. Transition이 edge·필수 evidence·glyph를
  문서 자신의 state machine에서 읽고 — 선언되지 않은 edge는 거부된다 — AddItem이 `[*]` 줄이 가리키는
  상태에 새 항목을 놓으며, WIP gate가 상태 이름과 무관하게 kit glyph `[~]`를 `[o]`에 대해 세고,
  ApplySchema가 Mermaid block 자체를 다시 쓰고, 내장 5상태 table·frontmatter schema reader·쓰이지
  않는 문서 기반 wrapper·죽은 corpus mermaid walker는 사라진다.

- T1.0: 저장소가 문서 쌍(README, AGENTS, CHANGELOG), `.flowmark/` ignore 규칙, 이 실행 checklist
  쌍과 함께 bootstrap된다.
- T1.1: 설계 계획이 3개 lane의 적대 감사를 판정으로 접는다 — orm 안의 executor seam, 2절 멱등성,
  dbspec 기반 frontmatter embedding, 하나의 portable 작업 단위 lock을 가진 prepare-record commit
  protocol, restrict-only foreign key, 거부된 identity column, type-directed ordering, adapter 기반
  sync.
- T1.2: 저장소가 polyspec/kit을 lock과 함께 디렉터리 단위로 vendor하고(`make kit-sync`,
  `make kit-check`) tracker를 구성한다: config/checklist.json이 실행 checklist 쌍을 상태
  5종으로 읽고 config/documents.json이 문서 쌍과 changelog를 선택한다. `make documents-check`가
  통과한다.
- T1.3: `make refs-check`가 config/checklist.json의 checklist와 선택된 문서의 prose(fenced block과
  inline code 제외)를 읽고, 어떤 항목으로도 해석되지 않는 task ID를 file·line·column을 붙인 finding으로
  보고한다. `make test`가 이것을 검증한다.
- T1.1-1: 하나의 Go binary가 세 개의 thin face — CLI, MCP server, soksak sidecar(soksak core
  명세의 protocol v1) — 를 가지고, GUI는 자기 repo의 soksak plugin으로 출시된다. 저장소는
  `package.json`과 `sidecar.json` manifest를 얹은 단일 언어 Go로 유지하고, 언어별 package는
  요구사항이 이름을 붙일 때 만든다.
- T1.4.2: 10-canonical-format이 멱등성 명제 2개, 보존 boundary를 가진 skeleton, 문서 grammar, frontmatter profile, type-directed ordering을 가진 task-ID grammar, encoding 규칙을 서술한다.
- T1.4.3: 20-document-model이 folder=table, file=row, column mapping, child table, 하나의 row인 번역 쌍, frontmatter binding을 서술한다.
- T1.4.4: 30-schema가 dbspec authority 파생, literal block embedding, standalone 임시 authority, schema stamp, 사람 write 판정을 서술한다.
- T1.4.5: 40-ops가 열기 시 복구, 작업 단위 portable lock, prepare record를 가진 규범적 commit protocol, corpus revision token, 비용 모델을 서술한다.
- T1.4.6: 50-constraints가 strict parse, type, key와 참조, state-machine setting, 사람 write 판정, key 필수 codec 검증을 서술한다.
- T1.5.1: 60-dialect이 capability table을 전체로 쓴다 — IR 표면, 강화된 update, on_duplicate 판정, 거부 표면, Go-only 표시.
- T1.5.2: 70-sync가 adapter 계약 범위와 확정 판정 — truth ownership, provenance, git 인식 drift 처리, 재구축 가능 projection — 을 서술한다.
- T1.5.3: 80-migration이 additive-first 범위, codemod transaction, 참조 재작성 예외, checklist cutover을 서술한다.
- T1.5.4: 90-conformance가 corpus vector 계약, 필수 round-trip·멱등성 속성, Go-first runner를 서술한다.
- T1.5.5: 100-checklist가 application을 전체로 쓴다 — table, terminal과 필수 field를 가진 5상태 machine, WIP gate, 또 하나의 consumer인 board, cutover.
- T1.5.6: 110-open-questions가 열린 결정 7건을 권고와 결정 조건으로 등록한다.
- T1.4.1: 00-overview가 범위, 강제력 요약, 용어, 비목표, 문서 지도를 쓴다.
- T1.4.7: 한국어 문서가 dry 문체를 따른다 — 동작 동사, 비유 없음, 영문 기술 용어. T1.4가 7개 하위 항목과 함께 완료된다.
- T2.1: Go module이 document skeleton으로 시작한다 — header comment, 제목, preamble, frontmatter block, H2 section의 strict parse와 canonical render, P1·P2 round-trip test와 header diagnostic.
- T2.5: codec가 저장소 corpus에서 P1과 P2를 증명한다 — 선택된 모든 문서가 diagnostic 없이 parse되고 byte 단위로 재출력되며, CRLF 복사본이 안정적으로 재출력된다. grammar는 실제 문서의 H2·H3 heading과 header-title 인접을 받아들인다.
- T2.4: 모든 parse 규칙이 located diagnostic을 보고한다 — 고정 rule 이름과 line·column. fenced block은 payload가 되어 구조 규칙은 unfenced 줄만 읽고, 닫히지 않은 frontmatter fence는 여는 줄을 지정한다.
- T2.2: frontmatter가 YAML 1.2 core subset으로 parse된다 — scalar·quoted scalar·sequence·literal block — 그리고 canonical로 재출력된다. comment·flow style·anchor·tab indent·중복 key는 located 거부다.
- T2.3: child block이 row로 추출된다 — kit grammar의 checklist 항목(상태·ID·제목·연속 줄)과 markdown table(header·cell) — unfenced payload에서만. malformed 항목은 located 거부다. T2가 완료된다.
- T3.1: corpus validator가 문서 directory를 검사한다 — strict parse finding, corpus 전체의 doc-id·item ID 유일성, 산문의 모든 task-ID token(fenced block·inline code 제외)의 item 해석.
- T3.2: go run ./cmd/flowmark validate .
[validate] the corpus passes이 config/documents.json의 선택으로 corpus directory 위에서 를 실행하고 이 저장소의 corpus가 통과한다. T3가 완료된다.
- T4.1: executor가 corpus row 위에서 one·all·count를 실행한다 — 첫 table로 documents와 checklist items — where predicate·ordering·limit와 함께. task-ID ordering은 segment별 숫자라 `T8.0.9`가 `T8.0.10` 앞에 오고 suffix는 base를 따른다.
- T4.2: executor가 sum·avg·group_count·paginate를 실행한다 — 숫자 아닌 값 거부와 함께 정수 aggregate, 값별 count, total을 동반한 page. T4가 완료된다.
- T8.1: application이 전환 table을 인코딩한다 — done은 terminal, 우회는 원인과 재시도 조건을, 불가는 원인을 요구 — 다섯 상태는 kit glyph에 대응하고 superseded 생성은 `T8.0.14-1`, `T8.0.14-2`로 번호를 매긴다.
- T8.2: 전환 적용이 table과 필수 field와 WIP gate — done 수를 넘는 doing 항목은 최대 두 개 — 를 유지하고 item 줄의 상태 glyph만 그 자리에서 바꾸고 문서의 나머지 byte는 그대로 둔다.
- T8.3: `flowmark items`가 corpus 항목을 나열하고 `flowmark transition`이 state machine으로
  하나를 적용한다. walk가 dot root에 들어가고, Korean twin은 두 번째 key를 등록하지
  않으며, changelog 예시의 ID는 code span에 담는다. T8이 완료된다.
- T1.4.8: 20-document-model이 aggregate와 분할 shape — 단일 파일, 1:n 연결 파일, 1:n:1 — 을 명시한다. 모두 file-as-row·child block·foreign-key column primitive로 구성되고 key는 restrict뿐이다.
- T9.1: conformance vector가 실행된다 — corpus before, request, projection인 기대 rows — 네 vector가 문서 순서·where count·type-directed 항목 순서·paginate total을 고정하고 spec 90이 vector 형식을 서술한다. T9가 첫 단계를 완료한다.
- T7: 세 face가 하나의 binary에서 실행된다 — CLI (validate·items·transition), MCP server (stdio 위 initialize·tools/list·tools/call, 줄당 JSON-RPC message 하나), soksak sidecar (surface routing과 closed handshake를 가진 protocol v1 loop). T7이 완료된다.
- T6: sync engine의 v1이 자리한다 — row의 derived·deterministic SQL dump인 md-to-db export와, provenance header가 body의 hash를 기록하는 adapter 소유 문서로의 rowset materialization. drift 검사가 자기 검증되고 clone에 안전하다. T6이 완료된다.
- T5: orm 저장소가 manifest setting과 state-machine sub-grammar를 dbspec의 state_machine·markdown setting으로 착륙시켰다(orm `T59` b74d8890, `T60` fb218c7c). parse·검증·canonical 출력·settings model과 engine suite가 green이다. orm engine의 executor 등록은 engine 로드맵으로 남고 능력 계약은 spec 60-dialect에 있다.
- T10: plugin이 자기 저장소(soksak/plugins/flowmark, commit ae07ec9)에서 시작한다: flowmark sidecar 위의 칸반 보드 — 다섯 상태 칸럼, 항목마다 카드, board.refresh 명령, 자기 write path는 없어 보드가 state machine을 깨뜨릴 수 없다.
- T7.3-1: sidecar와 MCP의 모든 message가 자기 write에서 flush된다. 원인: 지연 flush가 응답을 입력이 끝날 때까지 붙잡아 session을 열어 둔 host가 실행 중 아무것도 받지 못했다 — plugin 저장소의 예제 Wails host가 실제 왕복에서 발견했다. pipe가 열려 있는 상태에서 응답 한 건을 읽는 회귀 시험이 이를 덮는다.
- T8.4: 이슈가 write path로 들어온다 — `flowmark add`가 유일 ID 검사와 함께 대기 항목을 추가하고 `flowmark rm`이 하나를 지우되 하위를 가진 부모는 거부한다. MCP tool flowmark_add·flowmark_remove와 sidecar operation add·remove가 같은 표면을 가진다.
- T7.3-2: sidecar가 corpus를 감시한다 — 열린 surface root의 fingerprint를 poll하고 changed event를 push해 page가 session 동안 스스로 갱신된다.
