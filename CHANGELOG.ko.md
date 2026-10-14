<!-- doc-id: changelog -->
<!-- source-sha256: f2999a021d5836905abf2095c0518ac30520e06a1b1b1881bef0d4a9b122acac -->
# Changelog

## Unreleased

- T12-4: catalog가 dbspec/dbspec-set.json이 선언한 문서 set을 orm의 set reader로 읽는다. set이 나열하지 않은 .dbs 파일, dbspec/ 밖의 나열된 문서, external 항목, set 파일 자체의 오류는 catalog 진단이며, 모든 corpus가 set 파일을 갖는다.
- T12-3: catalog가 table과 foreign key 진단의 위치를 1행 1열로 대체하던 정규식 대신 dbspec.Parse의 선언 위치로 정한다. test가 아닌 code의 정규식 감사에서 GFM delimiter cell, token 파일, lock 이름, 출처를 밝힌 markdown-it-katex guard는 각각 그 문법의 유일한 읽기다.
- T14-1: flowmark가 orm v0.0.11을 고정한다.
- T14: restore가 orm의 ir.RestoreKey로 request hook 뒤에 key를 고른다(orm protocol 1.5).
- T12-1: flowmark는 branch가 아니라 release tag v0.0.35에서 kit을 공유 PRINCIPLES.md와 함께 vendor한다; AGENTS.md는 그것으로 연결하고 flowmark의 규칙만 남기며, 전체 test 모음은 활성 checklist의 모든 항목이 끝났을 때 한 번, 항목마다 소유 검사만 실행한다.
- T12.1-38-2-1 (fix): slug 기록기의 test가 남기던 stub gh의 임시 폴더를 지운다.
- T12-2: 버려진 worktree는 즉시 검토하며, 보존할 가치가 있는 것은 제거 조건을 가진 checklist 항목이 되고 worktree는 branch와 함께 삭제한다(AGENTS.md); 버려진 agent worktree 둘을 제거했다.
- T12.1-38-1: engine이 GFM table을 각 viewer처럼 읽으며, GitHub, GitLab, VS Code에서 기록한 table vector 54개를 더해 확인한다: table cell은 inline 내용이라 그 footnote 참조가 footnote 번호를 매기고 그 raw HTML heading과 id를 읽으며, 각 `\|`는 `|`로 읽고 header의 column 너머 cell은 버린다; GitHub과 GitLab에서 header row 앞의 문단은 pipe의 escape를 풀고 어떤 link label도 정의하지 않는다; body row는 각 viewer의 cell 한도에서 멈춘다; VS Code의 markdown-it table rule은 모든 block 시작과 모든 문단 줄에서 가장 먼저 돌므로, heading, fence, HTML block, math block, list item, block quote 줄도 뒤에 delimiter row가 오면 header row다.
- T12.1-38-2 (fix): `make slug-vectors`는 연결이 응답 전에 실패한 GitHub 요청을 세 번까지 다시 보내므로, timeout된 연결 하나가 요청 수천 건의 기록을 끝내지 않는다; HTTP 오류는 여전히 기록을 끝낸다.
- T12: markdown database가 완성되었다. checklist의 모든 항목이 끝났다 — 문법, validator, release된 orm v0.0.9 위의 `markdown://` IR 실행기, 규모 시험을 거친 write path, 얼굴들, crudui가 렌더하는 천 글의 게시판, 세 platform gate, 복잡한 foreign key corpus.
- T12.12: flowmark가 pseudo-version 대신 release된 orm v0.0.9를 요구한다. 그 tag는 IR executor protocol 항목 T61과 T62, validator series T82–T102를 담으며 전 suite가 그 위에서 통과한다.
- T12.14: 복잡한 foreign key의 corpus: golden이 다섯 table 사슬, 자기 참조, 쌍방향 쌍, 다섯 table에게 참조되는 table, block 행으로의 링크와 모든 저장 종류를 선언한다. engine은 사슬을 쓰고 그 사슬을 지나 join 하며 관계의 관계를 조립하고, 이동에서 참조를 유지하며, 빈 참조와 자기 참조된 삭제, 사슬 변이 잡은 삭제, 홀로 남은 쌍방향 쌍의 한쪽을 거절한다 — model이 담을 수 없는 복합 key는 test가 고정하는 schema error로 남는다.
- T12.11.2-2 (수정): board의 정렬이 두 얼굴에서 동작한다 — query API를 잘못 불러 PHP 정렬이 항상 비었고, 정렬은 이제 페이지를 자르기 전에 목록 전체에 적용된다(제목은 요청의 column order로, 작성자 이름은 요청의 order 항목이 join된 column을 못 가리키므로 목록을 읽어 정렬). 페이지가 정렬을 따른다.
- T12.11.2-2 (수정): board의 pager가 링크를 윈도잉한다 — 첫 페이지, 마지막 페이지, 현재 주변 다섯 개와 간격 표시, crudui의 pagination 모델처럼 — 그리고 링크가 정렬을 유지한다. 목록 자체는 항상 요청한 페이지의 열 개 글을 보인다.
- T12.11.2-2: board의 form, 목록, 검증이 두 얼굴에서 crudui로 돈다: `src/PostForm.php`와 `node/postForm.ts`의 같은 field·목록 명세, error와 숨은 CSRF token을 함께 렌더하는 글쓰기 form, server가 렌더하는 목록, crudui validator의 검증(422와 field error, 303과 목록 이동), app과 함께 제공되는 crudui stylesheet.
- T12.13: platform gate: `make test`와 writepath benchmark가 push마다 Linux, Windows, macOS에서 돈다(`.github/workflows/test.yml`), 건너뛴 test 없이 green. 그 과정의 Windows 결함을 고쳤고 benchmark artifact를 실행마다 보관하며, 그 수치를 spec 40의 규모 봉투와 비교했다.
- T12.4-10: orm의 "Markdown IR executor" 표는 spec 60의 capability 표를 행마다 옮긴 것이며, orm의 `make markdown-table-check`가 그것이 고정한 flowmark 판(`444f27f`)에 대해 지킨다. spec 60의 한국어 capability 표는 orm의 문체 규칙 안의 표현으로 고쳤다.
- T12.4-6: `time`은 IR executor 메시지에서 컬럼의 precision과 무관하게 `HH:MM:SS.ffffff`라는 한 형식을 가진다. 실행기는 그 형식의 parameter(p 뒤는 0)를 읽고 time을 그 형식으로 답하며, p자리 형식과 p 뒤의 0이 아닌 자리는 `CODEC_ENCODE`로 거절한다(이전에는 p자리를 읽고 답했다).
- T12.4-5 (수정): parameter 검사는 orm client의 메시지에만 적용한다. face 작업의 값은 엔진 자신의 쓰기이므로 catalog의 규칙이 판정하며, 틀린 type의 값을 쓰는 face 쓰기는 spec 50이 말하는 대로 다시 `CONSTRAINT`이다.
- T12.4-5: IR 실행기가 row를 읽기 전에 모든 parameter를 그 slot의 type과 대조한다. slot의 type 밖의 값(JSON 종류, 정수 범위, decimal의 scale과 precision, 0001~9999 밖의 date, varchar의 길이, text의 U+0000, 문자열이 아닌 `contains` 값)과 non-null 컬럼에 할당하는 NULL은 `CODEC_ENCODE`로 답하며, 같은 입력에 대한 capability 오류와 write fault를 대신한다.
- T12.4-2: IR 실행기가 orm의 스칼라 컬럼 subquery 규칙을 따른다. 한 행의 limit과 primary key를 포함한 order가 없는 스칼라 컬럼 subquery는 orm의 validator에서 `IR_INVALID`로 답하고, 실행기는 여러 행에 대한 자체의 이전 답 대신 subquery의 order의 첫 행을 취하며 일치하는 행이 없으면 NULL이다.
- T12.4-4: IR 실행기가 orm의 upsert 규칙을 따른다. 둘 이상의 key가 만날 수 있거나 충돌 key를 다 할당하지 않는 `on_duplicate` insert는 orm의 validator에서 `IR_INVALID`로 답하며, 실행기 자체의 이전 답은 지웠다. upsert의 `affected`는 계속 1이다.
- T12.4-8: IR 실행기가 orm의 열 쌍 규칙을 따른다. 만나지 않는 두 컬럼과 컬럼의 type이나 codec이 허용하지 않는 열 비교 연산자는 orm의 validator에서 `OPERATOR_NOT_ALLOWED`로 답하며, 실행기 자체의 이전 답은 지웠다.
- T12.4-7: IR 실행기가 orm의 key 갱신 규칙을 따른다. primary key나 identity 컬럼(`restore`는 soft delete 컬럼도)을 할당하는 `update`나 `restore`는 orm의 validator에서 `IR_INVALID`로 답하며, 그에 대한 실행기 자체의 이전 답은 지웠다.
- T12.4-9: IR 실행기가 insert는 값이나 NULL로 할당한다는 orm의 규칙을 따른다. insert의 `set`에 있는 `plus_p`와 `minus_p`는 orm의 validator에서 `IR_INVALID`로 답하며, 실행기 자체의 이전 답은 지웠다.
- T12.4-3: IR 실행기가 orm의 `key_by` 규칙을 따른다. nullable 컬럼으로 key를 정한 relation은 orm의 validator에서 `IR_INVALID`로 답하며, 그에 대한 실행기 자체의 이전 답은 지웠다.
- T12.4-1: IR 실행기가 orm의 `group_by` 규칙을 따른다. group된 행 읽기, group된 sum, group된 subquery, group 밖 order는 orm의 validator에서 `IR_INVALID`로 답하며, 그에 대한 실행기 자체의 이전 답은 지웠다.
- T12.1-38: 엔진이 GFM table을 GitHub, GitLab, VS Code처럼 읽는다. header row, 같은 수의 cell의 delimiter row, 그 뒤의 body row가 CommonMark가 문단으로 읽는 줄들을 받아 들이므로, 어느 viewer도 읽지 않는 heading은 엔진의 heading이 아니고 body row 뒤의 setext 밑줄은 아무것도 밑줄 긋지 않는다. VS Code의 markdown-it는 heading 줄도 header row로, cell 하나의 pipe 없는 delimiter row도 받는다.
- T12.5-3: 엔진이 규모 한계를 밝힌다. 문서 하나에 1 MiB, folder와 corpus에 문서 20,000개이며 file 크기, folder와 corpus의 문서, lock 아래 session의 benchmark에서 나왔다. 한계를 넘은 corpus는 validator가 알리고(`scale.file`, `scale.folder`, `scale.corpus`) 문서를 1 MiB 넘게 쓰는 commit은 `scale.exceeded`를 내며, 둘 다 쓰인다.
- T12.5-6: 한 corpus를 여러 process가 동시에 열 때 둘이 lock folder를 함께 만들어도 실패하지 않는다. folder를 먼저 만들고 나중에 판정한다.
- T12.5-5: 열기에 실패한 corpus는 lock을 남기지 않는다. lock 해제는 lock file을 여는데 macOS의 감시가 문서마다 descriptor를 쥐어 남은 것이 없었으므로, 감시를 먼저 닫고 lock을 해제한다.
- T12.11.2-1: board 애플리케이션이 hyper의 Node.js 서버에서도 orm의 TypeScript client와 `markdown://` 위에 `examples/board/web/node`로 돈다. PHP 서버와 Node.js 서버는 같은 행과 글 하나의 같은 페이지를 낸다.
- T12.11.2: board 애플리케이션이 hyper의 PHP 서버 위에서 orm과 `markdown://`로 `examples/board/web`에서 돈다. 글 1000개를 1월부터 10월까지 달마다 100개씩 나누며, 두 board corpus 모두 `posts`는 `created`의 달로 샤드되고 `examples/board/seed`가 엔진으로 글을 쓴다.
- T12.1-35: `files`나 `document` table이 시간 shard(`store ... shard <column> <year|month|day>`, orm T119)를 선언할 수 있다. 행은 `<table>/<period>/<key>.flowmark.md`나 기간 문서 `<table>/<period>.flowmark.md`에 서고, insert는 행을 자기 기간의 파일에 놓고, shard column 갱신은 행을 옮기며 그 행으로의 링크를 같은 쓰기에서 다시 쓰고, 행이 없는 기간에는 파일이 없으며, validator는 다른 기간의 파일에 있는 행을 `shard.period`로 보고한다.
- T12.7: database로의 export가 완료되었다. codec column은 저장된 cell을 유지하고, list 끝의 host stage hex와 ip는 풀었다가 orm이 다시 적용하며, client stage 앞의 host stage만 거절한다. 앞선 항목의 identity key와 `datetime(p)` bind까지 합쳐 catalog의 모든 table이 export된다.
- T12.4: IR executor가 완료되었다. orm protocol 아래에서 모든 IR 종류에 답하며, orm이 필요 없는 판정은 적용되어 있다. orm 항목을 기다리는 열 가지 판정은 하위 항목 T12.4-1부터 T12.4-10이며 각각 orm bump 뒤에 적용된다.
- T12.7(일부): export test가 자체 binder 대신 orm의 `DB.Execute`로 statement를 적용하므로 모든 precision의 SQLite `datetime(p)`와 `time(p)` column이 database가 요구하는 대로 bind된다. orm pin은 그것을 담은 commit으로 올라간다.
- T12.7(일부): database로의 export가 identity table의 key를 지킨다. 행은 orm의 `load` insert로 적재되고 orm의 identity restart가 뒤따라 다음에 생성되는 key가 적재된 가장 큰 key를 잇는다. codec table만 아직 거절한다.
- T12.1-37: 상세 anchor 충돌 규칙이 각 viewer의 block을 읽는다. GitHub과 GitLab의 footnote 정의, VS Code의 math block, GitHub의 footnote 번호, GitLab의 wikilink다. GitLab의 snippet prefix는 corpus file에 적용되지 않음을 명시한다.
- T12.5-4: codec의 parse와 render 비용이 file byte에 선형이다. MB당 약 48 ms와 35 ms, 할당 50배와 39배다(전에는 1 MB parse에 2.1초, 456배).
- T12.1-33: 충돌 규칙이 GitHub이 anchor를 주는 raw HTML `<h1>`–`<h6>` heading을 읽고, 모든 heading element를 기록된 vector대로 GitHub의 parser처럼 읽는다. 엔진이 렌더하지 않는 내용으로 이어지는 element는 지금까지의 slug가 시작하는 anchor를 차지하는 것으로 본다.
- T12.1-31: S_GL은 math span을 GitLab이 렌더하는 대로(글자 그대로의 source) 읽고, 끝의 Σ를 ς로, footnote 참조를 comrak처럼 읽는다. S_VS는 VS Code의 기본 markdown-math처럼 math span을 텍스트로 보지 않는다. slug vector는 고정된 gitlab-glfm-markdown을 GitLab의 option으로 build해 GitLab도 기록한다.
- T12.1-32: vendor한 Unicode data folder가 Unicode License v3 원문을 함께 담으며, package 문서가 이를 밝히고 test가 요구한다.
- T12.1-34: 표준 응용 schema는 item 하나에 file 하나를 저장한다. `items`는 `store files`이고, 그 이력과 그 item이 시작하는 link는 그 file의 block이며, `people`과 `views`는 각각 문서 하나다. 그래서 item의 transition, link, move는 그 item의 file만 쓴다.
- T12.1-36: `files` 저장은 `order`를 sparse rank로 저장한다. 이동, insert, parent 변경은 빈 rank가 있으면 놓인 행의 file만 다시 쓰고, 빈 rank가 없으면 묶음을 한 번 다시 번호 매긴다. delete는 형제를 다시 쓰지 않는다. 읽기는 여전히 1부터 센 위치를 보고, validator의 `order`는 공유된 rank만 보고한다. 형제 1,000개 중 1번 위치 insert가 file 1,001개 대신 1개를 쓴다.
- T12.5-2: operation이 더 이상 바뀌지 않은 file을 읽지 않는다. 열린 corpus는 운영체제의 file 알림(kqueue, inotify, ReadDirectoryChangesW)으로 최신을 유지하고, overflow면 전부 다시 읽으며, commit 대상은 언제나 lock 아래에서 다시 읽고, bytes와 link 대상이 그대로인 문서의 parse를 재사용한다. 10k 문서 읽기가 CPU 3.9초에서 68ms로 줄었고, 모든 face가 자기가 연 corpus를 닫는다.
- T12.11: 하나의 dataset을 세 저장 방식(document list, document table, 행마다 file)으로 담은 issue tracker(`examples/tracker`). 엔진으로 같은 행을 읽어 낸다. 하드코딩 목록 grep은 test `tests/inventory`로 두었고 결과는 0건이다.
- T12.11.1: board 예제(`examples/board`): 댓글과 답글이 달린 post. 댓글을 post file 안 block으로 두는 변형과 자기 file로 두는 변형이 있고, 엔진으로 만들고, 댓글과 답글을 달고, 고치고, 지우면 두 변형이 같은 행을 낸다.
- T12.3-6: write path의 commit이 validator의 하나뿐인 view 판정으로 자기가 쓰는 행에 `codec`과 `view` 규칙을 판정한다. 쓰기가 할당한 decode할 수 없는 codec 값이나 유효하지 않게 남은 view 행은 그 column에서 규칙을 밝히는 `CONSTRAINT`다.
- T12.10: CLI, MCP server, sidecar가 write path를 통해 어느 table에서나 schema, rows, row.get, row.insert, row.update, row.move, row.delete, transition, views를 실행하고 wire JSON과 `{code, error}` 실패로 답한다. views는 view table을, 또는 저장된 filter와 sort를 통한 view의 행을 답한다. `flowmark install`은 document file, files table folder, `.gitattributes` 줄 `*.flowmark.md text eol=lf`를 만든다. history repair와 unlock이 명령이 된다. 모든 명령은 0, 1, 2로 끝난다. 옛 codec, executor, corpus, app 경로와 `make validate`를 지웠다. 답은 더 이상 HTML escape하지 않는다.
- T12.3-1: 응용 schema가 view table(문서 `flowmark`의 `views`)을 선언한다. validator는 entity, filter, sort, group이 유효하지 않은 view를 orm의 `ir.Validate`로 검사해 `view`로, stage가 decode할 수 없는 값을 `codec`으로 보고한다. 마지막 stage가 `yaml`인 codec은 거절한다.
- T12.3-5: spec 30이 이력 block이 자기 상태 행 아래에만 서는 근거를 밝히고, 배치 key마다 catalog case를 둔다.
- T12.9: `flowmark serve <corpus> --port <port>`는 orm client가 loopback TCP로 접속하는 server다. `.flowmark/server.json`의 token, corpus와 token을 확인하는 connect message, control message로 다루는 transaction과 savepoint, 쉬는 transaction을 rollback하고 그 연결을 닫는 `--tx-idle-ms`를 갖춘다.
- T12.8: conformance vector를 저장 문법에 맞게 다시 썼다. spec 90이 vector 형식을 정하고, 69개 vector가 files, document list와 table, block list와 table 위에서 write path를 지나 실행되며 equivalence group, 반복, write path 자신의 답을 다룬다.
- T12.7(일부): database로의 export가 orm으로 렌더된다. orm의 schema statement 다음에 foreign key 순서와 저장 순서로 행마다 planner insert 하나를 두며, soft delete된 행도 포함한다. identity와 codec table은 orm이 필요한 것을 제공할 때까지 거절한다. provenance header writer는 지웠다.
- T12.6: introspection은 모든 문서를 catalog에 대해 검증하고, 각 `dbspec/` 문서의 canonical 형식과 path, 이름, manifest hash로 된 catalog를 모든 진단과 함께 돌려준다. `flowmark introspect`는 이를 JSON으로 쓰고 0, 1, 2로 끝난다.
- T12.5-1: `flowmark canon <corpus> [file…]`가 지정한 file을, 지정하지 않으면 다시 쓸 수 있는 모든 corpus 문서를 lock 아래 journal을 남기는 transaction 하나로 canonical 형식으로 다시 쓴다. `format.block`, `format.section`, `format.encoding`, 행 사이의 `parent.cycle`, 또는 사용할 수 없는 block이나 document table로의 link가 있는 file을 이름으로 주면 `CONSTRAINT`로 거절하고 아무것도 쓰지 않는다. file을 주지 않으면 그런 file은 보고하고 그대로 둔다. 명령은 canon이 실행되면 0, 거절이나 실패면 1, 잘못된 명령줄이면 2로 끝난다.
- T12.3-4: catalog는 배치 foreign key column이 nullable인 block table을 table 이름에서 거절한다. block 행은 그 key가 참조하는 행의 범위 안에 서며, NULL은 어떤 행도 참조하지 않는다. 이력 table의 저장 규칙은 이 하나의 규칙에 기대고, spec 30은 이력 행을 row column이 아닌 column으로 배치하는 setting 오류를 적는다.
- T12.4: IR executor가 T12.4 판정을 반영한다. contains는 ASCII 대소문자를 접고 contains_binary는 code point로 비교한다. NULL은 오름차순에서 처음에 정렬된다. files table의 저장 순서는 전순서다. 시계는 쓸 때 자르고 여섯 자리로 비교한다. 범위 밖의 상대 시각은 NULL이다. join은 soft delete된 자식을 제외한다. flatten은 one relation으로 답한다. 읽을 수 없는 답 값은 INTERNAL이다. restrict는 쓰기 전의 행에 대해 성립한다. 거절된 쓰기는 결함을 한 순서로 보고하며, 행마다 중복 결함은 최대 하나다. insert는 자신이 해소하는 dangling link를 다시 쓴다. on_duplicate를 포함한 모든 key 검사는 A–Z만 접는다. 대소문자만 다른 key 둘을 가진 문서에 대한 쓰기가 commit된다. catalog는 그 밖의 이력 저장을 거절한다. spec 10, 20, 30, 40, 50, 60, 80과 plan은 모든 판정을 적고, orm을 기다리는 지점은 각자 orm 항목을 적는다. `scripts/go-test.mjs`는 재사용한 mount를 분리하지 않는다.
- T12.5: write path가 IR executor와 journal을 잇는다. 모든 operation은 corpus lock을 잡고 중단된 commit을 먼저 복구한다. transaction 밖의 요청은 lock 아래 transaction 하나다. transaction과 savepoint는 lock을 잡고 자기 post-image를 읽는다. commit은 대상을 확인하고, lock 아래 다시 읽은 corpus가 바뀌었으면 쓰기를 그 위에서 다시 실행하며, journal로 쓴다. prepare 기록이 commit 시점이다. lock 기록은 획득마다 token을 가진다. panic이 나도 lock을 놓는다. format 진단이 있는 file을 다시 써야 하는 쓰기는 요청에서 거절되며, canon, add-column migration, file 전체 delete는 면제된다. flowmark history repair는 executor 쓰기다. commit은 validator의 evaluator로 check를 판정한다.
- T12.1-30: 상세 anchor 충돌 규칙이 접은 형태 대신 각 viewer의 정확한 slug를 따른다. S_GH는 GitHub의 heading id(Unicode 17.0.0 word 문자, alt 텍스트 없음)를, S_VS는 VS Code의 것(그 slugify.ts, Unicode 13.0.0 집합, Final_Sigma, alt 텍스트)을 재현하며, 둘 다 `make slug-vectors`가 viewer에서 모든 code point에 대해 기록한 id와 대조한다. S_GL은 GitLab이 고정한 comrak 0.56.0의 것을 재현한다. slug 입력은 CommonMark 0.31.2가 렌더한 텍스트다. 텍스트의 numeric character reference는 그 code point다. HTML id나 name은 소문자로 바꾼 뒤, GitHub의 `user-content-` 형태로도 비교한다.
- T13: Git이 `config/commits.json`을 어기는 message를 거절하는 `commit-msg` hook과 `pre-push` push gate를 실행한다. `config/checklist.json`이 둘을 나열하고 `make hooks`가 설치한다.
- T12.3-2: validator는 type이 아닌 값이 무엇이든(그런 atom을 true로, 이어 false로 평가해) 행의 논리 값에 대해 check 술어가 false이면 `check`를 보고하고, 같은 판정을 commit용으로 export한다. `history.chain`은 연속성만, `state.transition`은 선언되지 않은 이력 쌍을, `state.required`는 필수 값이 없는 선언된 쌍을 보고하며, type이 아닌 상태, `from`, `to`는 `type`만이고, 행 자신의 `from`은 여전히 앞 행에 대해 판정한다.
- T12.3-3: unique key는 orm의 binary collation처럼 바이트 단위로 비교한다. primary key는 file 이름이자 anchor이기 때문에만 대소문자를 무시한다. `key.duplicate`는 key가 바이트 단위로 같으면 그 행을 밝혀 바이트 단위로 같다고 말하고, 그렇지 않으면 대소문자를 무시하고 같은 첫 key를 밝힌다.
- T12.4(일부): IR executor(`irexec`)를 추가한다. orm request message를 decode하고, store 행 위에서 IR Where, join, relation, aggregate, pagination으로 읽기에 답하며, 사용할 수 없는 table이 필요한 요청을 거부하고, 렌더링 전에 요청의 key 검사를 corpus에 대해 실행하며, 쓰기에는 `{affected, keys}`와 write path가 commit할 post-image로 답한다. spec이 아직 정하지 않은 질문은 판정될 때까지 `CAPABILITY_UNSUPPORTED`로 답한다.
- T12.3: validator는 이력 table이 어떻게 저장되든 행의 이력을 이력 foreign key로, corpus 순서로 찾으므로, `document table` 이력이 더는 거짓 `history.gap`을 보고하지 않는다. `scripts/go-test.mjs`는 `spawnSync go ENOENT`로 실패하는 대신 PATH에 없는 `go`를 밝힌다.
- T12.1-4-1: readability의 기준은 spec이다. 구조는 CommonMark 0.31.2로, table과 task-list 항목은 GFM 0.29로 판단한다. fragment target — heading slug, 그리고 CommonMark가 code 밖에서 읽는 raw HTML의 id와 name — 은 GitHub, GitLab, VS Code로 판단한다. spec이 이를 정의하지 않으며 link `[k](file#anchor)`가 그 viewer들에서 풀려야 하기 때문이다. MkDocs와 Python-Markdown은 기준에서 철회한다. MkDocs는 T12.1-2에서 `--` key 규칙의 근거로만 처음 이름 붙었고 T12.1-4에서 그것을 요구하는 어떤 요구 없이 지정 viewer가 되었으며, Python-Markdown은 어떤 spec도 구현하지 않으므로 그것을 따르는 일은 한 구현의 일탈을 쫓는 일이었다. 함께 철회한다: T12.1-2와 T12.1-4의 `--` key 규칙(어느 기준 viewer도 연속 hyphen을 접지 않으므로 `a--b`는 유효한 key다), T12.1-4의 F_M fold와 MkDocs의 `_<n>` 번호, T12.1-14의 Python-Markdown reference definition과 attribute-list 충돌 규칙(`{#…}`는 CommonMark와 GFM에서 글자 그대로의 텍스트다), 그리고 T12.2.2, T12.2.2-1, T12.2.2-6에서 이를 구현한 code(`store/attr.go`, `foldM`, F_M definition)와 그것을 고정하던 test. 근거를 바로잡아 남기는 것: `-_` key 규칙(CommonMark §6.2), `-<n>` 반복 번호, code 밖 raw HTML에 대한 HTML `id`/`name` 규칙. T12.1-22, T12.1-26, T12.1-27은 그 원인과 함께 impossible로 철회한다.
- T12.5(일부): write path에 spec 40의 portable lock과 journal이 생겼다(`store/journal`). lock `.flowmark/lock`은 hard link로 만드는 JSON holder record(process start와 machine 식별자)다. 잡힌 lock은 운영체제의 변경 알림(inotify, kqueue, ReadDirectoryChangesW)과 holder의 종료가 이끄는 대기로 deadline까지 기다리며, holder와 그 상태를 밝히는 `LOCK_TIMEOUT`으로 실패한다. stale lock은 `flowmark unlock`의 claim 절차만 지운다. commit은 content-addressed post-image와 rename마다의 사본, pre-image를 stage하고, prepare record를 쓰고, rename하고(delete는 stage로, 없는 folder는 먼저 만들어), commit record를 쓰고 stage를 거두며, 각 단계를 platform별로 지속시킨다. recovery는 하나뿐인 stage를 완료하거나 버리고, crash 중 고친 대상은 해결할 stage file을 밝히며 거부한다. FAT32, exFAT, corpus 안의 mount point는 거부한다. lock과 journal event는 observer에 publish된다. spec 40은 모든 판정을 적으며, 그중에는 지속성에 Unix의 folder sync와 Windows의 write-through rename이 필요하다는 것이 있다. Linux와 Windows 경로는 compile과 vet을 통과하며, 실행은 T12.13에서 한다.
- T12.1-29: setext heading의 문단 앞머리에 있는 link reference definition을 더는 heading 텍스트로 읽지 않는다. CommonMark 0.31.2가 읽는 대로다. setext `people row P1` 위의 `[r]: /u`는 `format.block`이 아니라 definition과 그 뒤의 비정식 상세 heading이며, section heading도 같다. definition만 있는 문단에 밑줄이 붙어도 heading이 아니다.
- T12.1-28: 상세, section, block heading의 진단은 그 텍스트의 첫 글자에 위치한다. setext heading이면 열 1, 문서의 section은 고정된 열 5가 아니라 어느 level에서든 그 자리, `files` 행은 열 1이 아니라 그 자리다. setext section heading이나 block heading은 일반 canonical form 검사로만이 아니라 비정식 heading 철자로 보고된다.
- T12.3: validator(`validate`)를 추가한다. corpus walk, format 규칙, 그리고 validator 규칙 `type`, `reference.dangling`, `key.duplicate`, `order`, `state.limit`, `parent.cycle`, `history.gap`, `history.chain`을 읽을 수 있는 행마다 보고하며, table을 사용할 수 없는 동안에는 그 table의 완전한 행이 필요한 규칙을 보고하지 않는다. `state.limit`는 limit를 넘은 각 행에, `key.duplicate`는 primary key와 모든 unique key의 첫 자리 다음의 각 자리에 보고하며, table의 모든 block과 file에 걸친 corpus 순서를 따른다. unique key는 값을 바이트 단위로 비교하고, NULL과 type이 아닌 값은 비교에 참여하지 않는다. column의 type이 아닌 order 값은 쓰기와 validator 모두에서 type 결함뿐이며 형제 수에 세어지고, parent가 type이 아닌 행은 어느 형제 묶음에도 속하지 않는다. key가 그 column의 type이 아닌 외래 key는 읽기에서 `type`뿐이며 결코 `reference.dangling`이 아니고, 그런 key를 쓰는 쓰기는 `CONSTRAINT`이며 결코 `FOREIGN_KEY`가 아니다. 모든 golden이 validator에서 깨끗하도록 store golden을 고쳤고 test가 이를 지킨다. `make test`는 Go 테스트에 필요한 대소문자 구분 폴더를 macOS에서는 Case-sensitive APFS image로, Windows에서는 `fsutil`로 마련한다. view는 T12.3-1을 기다린다.
- T12.1-25: `<spec>#<anchor>: …` 꼴의 test case 이름이 그 spec의 heading이 아닌 anchor를 적으면 test가 실패한다. anchor는 GitHub와 같은 방식으로 계산한다. 없는 anchor(`10#order`, `10#storage-order`, `20#document-order`, `20#readable-values`)를 적던 case 이름 13개는 이제 각자 고정하는 규칙을 적는다.
- T12.1-20: spec이 `markdown` column의 section은 행 scope 하나에 최대 한 번 선다고 적는다. 같은 column 이름의 두 번째 heading은 어떻게 적었든, 값이 같더라도 `format.section`이다. 두 값의 뜻은 확정적이지 않고, canon은 한 값의 바이트를 버려야만 고칠 수 있기 때문이다. codec은 이미 그렇게 했으며, test가 `files`와 document 행에서 이를 고정한다.
- T12.1-21: 두 block의 행이 바이트 단위로 공유하는 key로의 link가 규칙으로 target을 유도한다. `parent`는 자기 block의 행에서 유도한다. 그 밖의 foreign key는 참조된 table의 두 행 이상이 그 key를 공유하면 유도된 target이 없으며, target 검사 없이 읽고 읽은 그대로 쓴다. 그래서 file의 진단이 더는 행의 순서에 달리지 않는다. 참조된 table의 배치 사슬이 `files` table에 닿으면 codec은 linker에 묻고, linker는 새 `ErrKeyShared`로 file 사이에서 공유된 key를 알린다. 그런 link에 target을 간직하지 않은 쓰기는 그 column에서 `CONSTRAINT`다.
- T12.1-23: 쓰기 거절이 자식 block을 똑같이 이름 붙인다. `files` 행의 선언되지 않은 block은 document 행과 같이 그 block의 이름에서 거절된다. 행은 그 key column에 최대 하나의 `DUPLICATE_KEY`를 가진다. 바이트 단위로 같은 key를 가진 앞선 행 가운데 첫 행을 밝히고, 없으면 대소문자만 다른 행 가운데 byte 순서에서 가장 작은 key를 가진 행을 밝히며, 같은 key가 여럿이면 처음 준 행을 밝힌다.
- T12.1-24: spec이 이력 table의 저장에 근거를 둔다. 이력 table은 상태 table 아래의 `block <fk> table`로, 또는 `document table`로 저장된다. 이력 규칙은 그 행과 foreign key 위에서 정해지며 그 자리 위에서 정해지지 않기 때문이다. block은 행의 이력을 행 곁에 둔다. document table은 어떤 깊이에서도 row scope 3을 가지므로, `markdown` column을 require하는 row scope 5의 block 상태기계도 그 이력이 document table이면 load된다.
- T12.1-16: table header 행은 GFM이 읽듯 읽는다. 양 끝 pipe가 없거나(column 하나인 header는 둘 다 없어도 된다), tab 없이 세 칸까지 들여 쓰거나, 끝에 space, tab, 수직 탭, 폼 피드가 있으면 `format.noncanonical`이고, 네 column 들여쓰기는 `format.block`이다. 마지막 pipe가 없는 행은 마지막 `\|`까지 포함해 마지막 cell을 온전히 지키며 `format.noncanonical`이다. 전에는 1바이트가 조용히 잘렸다. 들여쓰기에 tab이 있는 delimiter 행은 `format.block`이다.
- T12.1-17: list 표지 뒤의 연속은 column으로 재며 tab은 다음 4의 배수까지 나아가고, 그 너비의 space로 판정한다. 두에서 네 column은 `format.noncanonical`, 다섯 이상은 `format.block`이다. 더 넓은 행 아래 두 칸 들여 쓴 행은 CommonMark가 형제로 읽으므로 `format.block`이다. 두 칸 문법이 자리를 주지 않는, 그 행의 내용 들여쓰기에 선 행은 그 행의 자식이며 `format.noncanonical`이다. 이는 표지 뒤의 tab이 내용 들여쓰기를 옮기므로 `format.block`이라던 T12.1-12 판정을 대체한다. 그 근거는 모든 column에서 성립하지 않는다. column 2 mod 4의 tab은 너비가 한 column이라 아무것도 옮기지 않으며, column으로 재면 tab은 CommonMark가 읽는 그 space와 정확히 같게 판정된다.
- T12.1-18: list나 table과 그 뒤 요소 사이에는 빈 줄이 정확히 하나 서며, file 끝에는 없다. 그 자리의 space와 tab만 있는 줄은 빈 줄이므로 더는 `format.block`이 아니다. 다른 개수, 또는 공백을 가진 빈 줄은 `format.noncanonical`이다.
- T12.1-19: `# Items`처럼 table을 다른 대소문자로 이름 붙인 H1은 `format.noncanonical`이다. H1과 상세 heading의 table 이름은 viewer가 소문자로 만들듯 소문자로 만들므로, `İ`는 `i`와 U+0307이 되어 다른 table을 가리킨다.
- T12.1-14: spec이 anchor 충돌 codec에 필요했던 것을 적으며, codec은 그것을 따른다. heading의 텍스트는 file 전체의 link reference definition과 함께 읽는다: F_G에서는 CommonMark가 읽는 대로, F_M에서는 Python-Markdown이 읽는 대로이며, label 앞에는 공백 문자 하나가 올 수 있다. image는 F_G에서 alt 텍스트로, 두 fold 모두에서 텍스트 없이 센다. 한 줄에서 id를 정하는 모든 attribute list `{…}`는, code 안이든 아니든, backslash가 `{`를 escape하지 않는 한, Python-Markdown이 어느 요소에 붙이든 상세 anchor와 비교한다. tab은 공백으로 센다. `id`와 `name` key는 대소문자와 무관하게 비교하며, key의 철자마다 마지막 token이 이긴다. 읽기는 충돌을 `{`, `<`, 또는 heading 텍스트의 첫 글자에 위치시키며, 쓰기는 그 텍스트를 담은 cell, field, 제목의 column을 밝힌다. 상세 heading의 table 이름은 소문자로 만든 뒤 맞추며, 낱말 사이를 가르는 것은 공백, tab, 줄 바꿈뿐이다. setext 밑줄로 쓰였거나, 낱말 앞·사이·뒤에 정식 공백 하나가 아닌 공백이나 tab의 run이 있는 상세 heading은 `format.noncanonical`이며, 낱말 사이의 다른 공백 문자(U+00A0 등)는 `format.block`이다. codec의 테스트는 각 판정을 고정하며, codec이 아직 따르지 않던 판정의 테스트는 변경 전에 red였다. table 행의 각 cell과 list 행의 key, 제목, state, 각 field는 같은 바이트로 쓰되 각자 자기 column 아래에서 쓴다. spec 10과 spec 50이 판정을 근거와 함께 영문과 국문으로 적는다.
- T12.1-15: spec이 block table의 state machine이 column을 `require`할 수 있는지를 정한다. 상태 table과 이력 table이 모두 `markdown`으로 선언한 require된 column은 이력 행의 section이므로, H6를 넘는 layout은 깊이 규칙이 이미 정한 대로 이력 table 이름에서 schema 오류다. 두 table 어느 쪽도 `markdown`으로 선언하지 않은 require된 column은 cell이며 어떤 깊이에서도 허용된다. 둘 중 한쪽만 `markdown`으로 선언한 column은 이력 table 이름에서 schema 오류다: 이력 행은 값의 사본을 담고, section과 한 줄 자리는 서로 다른 값을 받기 때문이다. catalog는 `checkHistoryMarks`에서 그 어긋남을 검사하며, `TestLoadRequireOfMarkdownPastH6`이 이 판정을 고정하고 검사를 더하기 전에는 red였다. spec 30과 spec 100이 근거와 함께 영문과 국문으로 판정을 적는다.
- T12.1-12: spec이 `document` codec이 규칙 없이 내린 판단을 적으며, codec은 그것을 따른다. document file의 행은 H1 바로 아래에 빈 줄 없이 서며, 그 자리의 빈 줄은 `format.noncanonical`이다. delimiter 행은 `| --- | --- |`이고, GFM이 읽는 다른 delimiter 형식은 `format.noncanonical`, 그 밖은 `format.block`이다. 상세 heading의 table 이름은 대소문자가 달라도 되며(`format.noncanonical`), 다른 table의 이름은 `format.block`이고, key는 대소문자를 구분한다. 상세 섹션은 자기가 이름 붙인 key를 바이트 단위로 가진 행을 취한다. 제목 token은 접두로 선언된 column과 맞으며, 맞는 token 뒤에 다른 token이 오면 `format.noncanonical`, 제목을 끝내면 `format.block`이고, 제목은 맞는 token을 모두 이스케이프하며 token 사이의 공백(tab 포함)을 그대로 둔다. field 앞에서 list 제목을 끝내는 tab은 `format.noncanonical`이다. list 표지 뒤의 tab은 item 내용의 들여쓰기를 옮기므로 `format.block`이다. 거절된 쓰기는 file이 아니라 요청에 위치한다: 각 거절은 table, key, 준 행들 가운데 index, column, rule을 밝히며, `store.Refusal`이 `RenderFile`과 `RenderDocument`의 줄 위치 진단을 대신한다. 바이트 단위로 공유된 key는 index를 싣고, 대소문자만 같은 쌍은 spec상 쓰기가 도입하는 행을 거절하며(executor의 검사, T12.4), codec의 backstop은 byte 순서에서 더 큰 key를 거절한다. 형제 순서와 순환 결함은 행마다이고, 없는 부모를 가진 행은 자기 결함만 가진다. `RenderFile`은 files table이 아닌 table에 대해 오류도 돌려준다. 읽기 쪽은 writer의 거절을 parser가 그 행에 대해 읽은 요소(제목, field, section이나 body)의 자리에 위치시키며, 메시지에 행과 column을 싣는다. 행 전체의 결함은 행의 첫 줄이다. 상세 heading의 table 이름과 `row`는 viewer가 하듯 소문자로 만든 뒤 비교한다. `catalog.Diagnostic`은 바뀌지 않았다. 읽기 진단이 있는 file을 다시 쓰는 커밋 수준의 거절과 executor가 렌더링 전에 실행하는 key 검사(T12.4)는 spec에 적었으며, 여기서는 구현하지 않았다.
- T12.1-13: spec이 block codec이 규칙 없이 내린 판단을 적으며, 이제 두 spec이 어긋나지 않는다. block이나 document 행의 `parent`는 자기 file이 아니라 자기 block의 행을 가리킨다: 다른 block의 부모는 쓰기에서 `CONSTRAINT`, 읽기에서 `format.block`이며, 읽기는 거짓 `parent.cycle`을 더하지 않는다. block heading 뒤의 빈 줄은 개수와 무관하게 `format.noncanonical`이며, space와 tab만 있는 줄도 빈 줄로 센다. 자식 block은 행 scope에 한 번만 서고 두 번째는 `format.section`이다. 두 행의 block들이 공유하는 상세 key는 block이 배정한다. block 행은 자기 `files` 행을 그 file의 key에서 유도한 file 이름으로 link한다. codec은 이 판단을 따른다: 빈 줄 규칙은 모든 빈 줄을 건너뛰며 진단은 하나이고, file 자신의 행으로 가는 link는 linker 없이 유도하므로 render가 이를 `FOREIGN_KEY`로 거절하지 않는다. 쓰기는 없는 행을 가리키는 parent를 한 번만 `CONSTRAINT`로 거절하며, 그 옆에 붙던 거짓 `parent.cycle`은 없다.

- T12.2.5: `store/testdata/equivalence` 아래에 storage 동등성 dataset을 더했다. table `entries`의 다섯 행을 다섯 corpus로 저장하며, 각 corpus는 자기 catalog를 가진다: files(행마다 file 하나), document list와 document table(`entries.flowmark.md`), 그리고 files table `plans` 아래의 block list와 block table이다. 다섯 catalog는 같은 column을 선언하고, block corpus만 placement foreign key `plan`을 더한다. 행은 title, 비어 있지 않은 body와 빈 body, 없음·빈 값·값이 있는 상태의 nullable summary section, text, int, bool, date와 NULL scalar, 그리고 상세 section이 없는 행을 포함하는 self foreign key `parent` 계층을 담는다. 이 계층은 list corpus에서 들여쓰기이고 table corpus에서는 명시적 `parent` column이다. 테스트는 다섯 corpus가 NULL을 포함해 모든 공유 column에서 같은 행으로 parse되는지, 각 저장이 행과 golden bytes를 왕복하는지 요구한다. file과 document fuzz target은 이 dataset의 golden을 seed로 읽으며, corpus마다 selector byte 하나로 고른다.
- T12.2.4-1: store fixture에 `list` storage이고 table에 상태기계가 있는 block을 더했다: 각 items row 아래의 `item_checks`는 세 상태마다 checkbox glyph, 중첩 행, order column, 그리고 행의 상세 section 안의 `history` block(`item_check_history`)을 가진다. 허용과 거절 사례가 그것이 쓰는 각 규칙을 고정하고, items golden이 그 P1과 P2 왕복을 담으며, fuzz target은 그 golden으로 시드된다. catalog의 heading 깊이 규칙이 history block을 level 7에서 거절했으나 그 행은 거기서 heading을 쓰지 않는다. 이제 규칙은 행이 H6를 넘는 heading을 쓰는 layout만 거절하며, spec 30이 그렇게 적는다. body만 있는 level 7 행은 유효하고, level 7의 markdown column은 여전히 거절된다.

- T12.2.2-6: `files`와 `document` codec이 spec 10의 상세 anchor 충돌 규칙을 적용한다. 상세 heading이 아닌 heading은 fold(F_G나 F_M, 끝 숫자 run을 뗀 것)가 상세 anchor의 fold와 같으면 거절한다. anchor와 같은 id를 정하는 attribute list와 anchor와 같은 HTML `id` 또는 `name`도 대소문자를 가리지 않고 거절한다. 쓰기가 충돌을 들이면 다른 heading을 이름으로 든 `CONSTRAINT`이며, 중복 key는 `DUPLICATE_KEY`로 남는다. 읽기의 충돌은 `format.block`이다. heading은 중첩된 것까지 CommonMark block 구조에서 얻고, inline 텍스트는 link, emphasis, code span, entity, raw HTML을 읽는 최소 reader가 commonmark.js 0.31.2에 대조한다. 모듈은 `golang.org/x/text` v0.42.0을 직접 의존성으로 추가한다. fold가 `norm.NFKD`를 쓰는데 표준 라이브러리에는 Unicode 17.0.0의 분해가 없기 때문이며, 테스트가 `norm.Version`을 17.0.0으로 고정한다. `internal/htmlentity`는 `entities` 3.0.1 데이터의 이름 참조를 담으며, 그 데이터에서 생성되고 테스트가 새로 만든 것과 대조한다.

- T12.2.2-5: heading scanner가 CommonMark 0.31.2처럼 `#` 연속 뒤에 tab이 오는 ATX heading을 읽는다(줄 끝은 이미 읽었다). hash 뒤의 tab은 제목, section, block, 상세 heading의 비정규 철자(`format.noncanonical`)이며, scanner, section 경계, 상세 heading 파싱이 같은 읽기를 따른다. `##x`와 `#` 일곱 개는 여전히 텍스트다. spec 10은 상세 heading 규칙에 tab을 적는다.

- T12.2.4: `block` storage codec이 부모 row의 scope 안의 child block을 parse하고 쓴다. block은 부모 section과 같은 level의 section이다: `files` row는 markdown section 뒤에 table 이름 순서로 level 2에 block을 두고, `document`와 block row는 detail section 안에서 그 section level에 둔다. block은 list 또는 table이며, 그 행들의 detail section이 뒤따르고, 재귀적이다. 행이 없는 block은 쓰지 않으며, 빈 block heading은 `format.noncanonical`이다. block 행은 중첩이 뜻하는 placement foreign key를 값으로 갖지 않고, `Row.Blocks`가 block마다 행을 저장 순서로 담는다. 쓰기는 선언되지 않은 block, 다른 table의 행, block 행에 준 placement key를 거절한다. T12.2.3의 쓰기 guard와 읽기 거절은 제거했고, production codec이 쓰는 `sp`를 test 파일에서 `store/row.go`로 옮겼다.

- T12.2.3: `document` storage codec이 list와 table 형식의 `<table>.flowmark.md`를 parse하고 쓴다. H1, 그 아래의 행(list 행은 glyph, key, 제목, 오른쪽에서 escape와 함께 parse하는 field, 들여쓰기는 `parent`; table 행은 key가 먼저이고, pipe escape를 가진 cell, NULL과 빈 텍스트), 저장 순서의 detail section `## <table> row <key>`(본문과 section 포함)를 다룬다. `order`는 읽을 때 1부터 세는 형제 위치로 채우고, 쓸 때는 형제 집합마다 정확히 1..m으로 취한다. 쓰는 쪽은 행의 순서를 스스로 맞추며 정렬되지 않은 입력을 거절하지 않는다. 외래 key는 linker를 거친 link이며 dangling link는 쓰인 target을 유지한다. T12.2.4까지 쓰기는 catalog가 child block을 선언한 table을 거절하며, 오류는 table, block과 T12.2.4를 이름으로 든다. 그래서 어떤 쓰기도 block의 행을 떨어뜨리지 않는다. 읽기는 child block section을 담은 detail section을 같은 이름으로 거절한다. 시간 값이 `now`로 쓰이면 그 type이 아니다. 읽을 때는 쓰인 대로 두고 쓸 때는 거절하며, 텍스트 값 `now`는 텍스트다. 규칙마다 허용과 거절 사례가 있고 세 golden은 P1·P2이며, parse와 render fuzz target은 그 golden으로 seed된다.

- T12.2.2-4: `files` codec이 H1 제목과 field list 값에 spec 10의 보이지 않는 문자 규칙을 적용한다. 양 끝의 보이지 않는 문자는 쓰기에서 `CONSTRAINT`로 거절되며, 읽기에서 CommonMark가 떼어내는 space나 tab은 `format.noncanonical`이고, 끝의 그 밖의 보이지 않는 문자는 `format.block`이다. 보이지 않는 문자는 U+1680을 뺀 White_Space 문자, 제어 문자, prepended concatenation mark를 뺀 형식 문자, Default_Ignorable_Code_Point 문자이며, 마지막 것은 Unicode 17.0.0 data 파일에서 생성한 table이고 테스트가 최신성을 확인한다. list 행과 table cell은 T12.2.3에서 따른다.

- T12.1-11: spec과 plan이 열린 block의 읽기 쪽을 있는 그대로 적는다. 열린 채 남은 fence나 HTML block이 흡수한 줄은 CommonMark block 구조에서 경계가 아니고 viewer도 그 block 안에 보여 주므로 읽기에서 값의 텍스트다. 그런 block 뒤에 요소를 두는 쓰기만 `CONSTRAINT`다. 어떤 reader도 관찰할 수 없는 결함을 가리키는 열린 block의 읽기 `format.block` 구절은 철회하며, 최상위 열린 fence 문장은 이 규칙과 일관되게 고친다.

- T12.2.2-3: `files` codec이 spec 10의 열린 block 규칙을 쓰기에 적용한다. 열어 둔 fence나 HTML block이 뒤따르는 요소를 흡수하면 값은 `CONSTRAINT`로 거절된다. block scanner는 CommonMark 0.31.2대로 type 6과 7의 HTML block을 읽으며, type 7 block은 문단을 중단하지 않는다. list 항목 안에 열린 block이 최상위 heading으로 닫히면 잘못이 아니다.

- T12.1-9: spec과 plan이 단일 줄 값의 양 끝에 있는 보이지 않는 문자를 거절한다: U+1680을 뺀 Unicode White_Space 속성의 문자, 제어 문자(Cc), Prepended_Concatenation_Mark를 뺀 형식 문자(Cf, U+FEFF 포함), Default_Ignorable_Code_Point다(Unicode 17.0.0 기준). 그런 문자는 글리프 없이 그려지므로 독자는 볼 수 없고(U+1680과 prepended concatenation mark는 글리프가 있어 텍스트다) viewer들이 끝에서 떼어내는 문자 집합이 서로 다르므로, 모두 거절하면 viewer의 버전과 무관하게 각 viewer가 값의 텍스트를 보여 준다. 구분자의 정식 공백은 여전히 space다. 읽기에서 끝의 space나 tab은 CommonMark가 떼어내므로 `format.noncanonical`이며(list 제목의 끝에서 뒤따르는 토큰 앞의 tab은 `format.block`), 끝의 그 밖의 보이지 않는 문자는 쓰기 규칙이 다시 만들 수 없으므로 `format.block`이다. T12.1-8의 space와 tab 정의를 대체하며, framing 빈 줄을 space와 tab만 있는 줄로 명시한다.

- T12.1-7: spec이 setext 규칙의 근거를 적는다. setext heading은 level 1 또는 2이고 모든 행 scope의 level L은 2 이상이므로 L보다 깊은 setext 밑줄은 없으며, spec 10의 setext 거절이 값의 모든 최상위 setext heading을 덮는다. plan도 같은 근거를 적는다.

- T12.1-10: 섹션 경계와 값의 끝에 대해 하나의 규칙, 즉 CommonMark 0.31.2 block 구조를 spec이 적는다. 최상위 heading은 ATX heading이거나 최상위 문단을 닫는 setext heading이며, fence, 모든 type(1~7)의 HTML block, 컨테이너, 게으른 연속 줄, 문단 중단 규칙은 CommonMark가 정한 그대로 센다. fence나 HTML block을 열어 둔 값은 뒤따르는 요소가 그 안으로 흡수될 때에만 거절하며, 뒤따르는 최상위 요소가 닫는 컨테이너 안에 열린 채 남은 block은 결함이 아니다. 이것은 T12.1-6과 T12.1-8의 좁은 문장을 대체한다.

- T12.2.2-2: block scanner가 CommonMark container를 읽는다. list 항목(항목의 내용 들여쓰기 기준, 순서 있는 항목과
  tab 들여쓰기 항목 포함)이나 block quote 안의 heading, fence 안의 줄, type 1~5 HTML block 안의 줄은 섹션 경계가
  아니며, 최상위 heading만 body를 끝낸다. setext 밑줄은 최상위 문단을 닫을 때만 heading이고, 게으른 연속 줄은
  CommonMark 0.31.2가 읽는 그대로 문단 글이다.

- T12.1-8: spec이 다시 짠 `files` codec에 필요했던 읽기 규칙을 적는다. CommonMark type 1–5의 HTML block을 요소 앞에서 열어 둔 값은 쓰기에서 `CONSTRAINT`이듯 읽기에서 `format.block`이다. 열린 fence는 읽을 때 요소 앞에 설 수 없다: 그 안의 heading은 경계가 아니다. 해석한 텍스트가 비었거나 `\#`로 끝나는
  제목은 쓰기 규칙이 그 H1을 다시 만들 수 없으므로 읽기에서 `format.block`이다. 줄 끝은 CommonMark가 읽는 대로
  LF, CRLF, 뒤에 LF가 없는 CR이며, 그 중 CR이 있으면 `format.noncanonical`이다. 한 줄 자리 규칙의 공백은 space와
  tab이다.

- T12.2.2-1: `files` codec와 store가 T12.1-3과 T12.1-4의 판정을 따른다. H1 제목은 마지막 `\#`을 `#`으로 읽고,
  마지막 `#` 연속이 공백이나 tab 뒤에 오거나 제목 전체이면 마지막 `#`을 escape해 쓰며, `\#`로 끝나는 제목은
  `CONSTRAINT`다. 줄 바꿈으로 시작하거나 끝나는 markdown 값, CR이나 U+0000을 가진 값은 거절되고, 요소 앞에서
  fence나 HTML block을 열어 둔 값, setext heading을 가진 값도 거절된다. field list 뒤의 `files` body는 들여쓴
  줄로 시작하지 않는다. BOM과 CR은 `format.noncanonical`이고, U+0000은 `format.encoding`이며, typed scalar는 정식
  형식으로 쓴다. column type이 아닌 scalar, dangling 외래 key, 대소문자만 다른 중복 key는 읽은 그대로(raw 텍스트,
  기록된 target) 유지되어 validator의 `type`, `reference.dangling`, `key.duplicate`로 넘어간다. key는 `-`로 끝날 수
  있고 `-_`는 없다. 렌더링이 떨어뜨리는 한 줄 자리의 공백은 `format.noncanonical`이다. field list 바로 뒤의 body
  줄은 `format.block`이다.

- T12.1-6: spec이 T12.1-3의 질문에 답한다. `files` table의 자리에 있는 일반 file이나 symlink는 `corpus.walk`이며
  그 table은 사용할 수 없다. 자리의 폴더가 없으면 빈 table이다. field list가 없는 `files` 행의 첫 body나 섹션은
  H1 뒤 빈 줄 정확히 하나 다음에 온다. 열린 fence나 HTML block을 남기는 값은 그 뒤의 요소를 어느 쓰기가 놓든
  `CONSTRAINT`다. setext 규칙은 최상위 heading에만 적용되며, 섹션 경계도 그렇다. 잘못된 UTF-8 byte의
  `format.encoding`은 그 byte에 있고, 문서 전체나 `corpus.walk`의 결함은 줄 1, 열 1이며, `catalog` 열은 orm의 dbspec처럼
  code point를 센다. 파일 전체를 지우는 delete는 파일을 지우면 그 바이트를 다시 쓰지 않으므로 rewrite 거절에서 면제된다.

- T12.1-5: spec이 `document` codec에 필요했던 판정을 적는다. `now`는 `datetime` 기본값에서 현재 시각을 뜻하는
  keyword이며 저장되지 않는다: `now`로 쓰인 시간 값은 그 type이 아니며, 텍스트 `now`는 text다. 저장 순서를 벗어난
  table 행은 `format.noncanonical`이고, file 안 행 사이의 `parent` 순환은 읽기에서 `parent.cycle`이며 그 file을
  읽을 수 없게 한다. 같은 행의 두 번째 상세 섹션은 `format.block`이다. 제목 token `<column>\:`은 쓰기에서
  `CONSTRAINT`다. table cell에서 각 `|`는 `\|`로만 쓰며, 읽을 때 `\` 뒤의 `|`는 cell 경계가 아니다. `order`는 읽기에서
  형제 위치로 채우는 논리적 column이고, 쓰기는 형제 묶음의 값을 정확히 1..m(m은 쓰기 뒤의 묶음 크기)으로 받는다. codec은 저장 순서를 행의 집합에서
  계산하며 정렬되지 않은 집합을 거절하지 않는다. 진단의 column은 규칙이 이름 붙인 요소의 1부터 세는 code point 열이다.
  dangling 참조는 쓴 target을 간직하며, 두 행이 공유하는 key를 가리키는 `parent` cell은 `format.block`이다. markdown
  값은 setext heading을 담지 않으며, fence나 HTML block을 열린 채로 남기면 뒤따르는 요소가 있을 때 `CONSTRAINT`다.
  catalog는 checkbox가 없는 list storage의 state machine을 거절하며, nullable 상태 column은 dbspec이 이미 거절한다.

- T12.1-4: spec이 상세 anchor, key, identity의 판정을 적는다. 상세 heading의 anchor는 heading의 렌더된 inline
  텍스트에 대한 두 fold로 file의 heading들과 대조한다. GitHub, GitLab, VS Code를 위한 F_G는 Unicode 범주가 letter나
  십진 숫자가 아닌 문자를 지우고, 소문자로 바꾸고, NFKD로 분해하고, 결합 기호를 버리며, ASCII 글자와 숫자만 남긴다.
  MkDocs를 위한 F_M은 소문자로 바꾸고, NFKD로 분해하고, 결합 기호를 버리며, ASCII 글자와 숫자만 남긴다. 그래서
  `café`는 두 fold 모두에서 `cafe`가 된다. 상세 heading이 아닌 heading은 두 fold 중 하나에서 끝 십진 숫자 run을 뺀 뒤
  같으면 상세 heading의 anchor를 가져간다. 명명된 viewer들은 점유되거나 반복된 id를 숫자를 덧붙이거나 끝 숫자를
  올려 번호 매기기 때문이다. 두 상세 heading은 anchor가 같을 때에만 충돌하므로 key `P1`과 `P10`은 결코 충돌하지
  않는다. anchor를 id로 정하는 attribute list와 값이 anchor인 payload HTML의 `id`나 `name`도 거절한다. 충돌을 새로
  만드는 쓰기는 `CONSTRAINT`이며, 다만 중복 key는 `DUPLICATE_KEY`다. 읽기에서의 충돌은 `format.block`이다. 다만 대소문자만
  다른 key의 쌍은 `key.duplicate`뿐이다. 주장하는 viewer는 GitHub, GitLab, VS Code, MkDocs이며, Pandoc은 주장하지 않는다.
  key는 `--`와 `-_`를 담지 않고 ASCII이며, 음수 integer key는 거절된다. markdown table의 primary key는 integer, `bool`,
  `varchar`, `uuid`, `date` column이며, catalog는 다른 type을 table 위치에서 거절한다. `identity`는 `i64` key에만 허용되며,
  이는 dbspec이 이미 강제하고 catalog test가 고정한다. key의 날짜 범위는 0001부터 9999까지다.

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
