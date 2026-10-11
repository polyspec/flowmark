<!-- doc-id: spec-ops -->
<!-- source-sha256: bc639a0ad15c02edc89d52393ac4c690324b79ee793c7426da60efa4ffe7cbf1 -->
# Operations

[English](40-ops.md)

이 spec은 운영 의미를 고정한다: corpus 열기, server와 그 session, lock, transaction과 commit protocol,
시계, 실패 어휘. commit protocol의 순서는 규범이다.

## Corpus 열기

- 열기는 lock 아래에서 **복구를 먼저** 실행한다. 끝난 holder의 pending lock record를 지우고
  ([lock](#lock)), `*.clearing` claim은 건드리지 않는다. `.flowmark/tx/`는 stage를 많아야 하나 가지며,
  그 이름은 OS random source의 128 random bit를 나타내는 소문자 hex 32자리다. 다른 이름이나 형태의
  항목, 또는 두 번째 stage는 열기를 거절하며 그 이름을 밝힌다. stage는 commit record가 있으면 거두고,
  prepare record가 있으면 완료하며, 둘 다 없으면 버린다. 복구는 idempotent다: 어느 단계에서 중단된
  replay든 다음 replay가 완료한다.
  복구는 lock을 잡을 때마다, operation이 corpus를 읽기 전에 다시 실행한다: lock 아래에서 찾은 stage는
  holder가 끝난 commit이며, SQLite가 읽기 전에 hot journal을 rollback하는 것과 같다. 근거: 그렇지 않으면
  다른 process가 commit 도중 끝나기 전에 열린 server가, `flowmark unlock`이 stale lock을 지운 뒤 반쯤
  적용된 corpus를 읽게 된다.
- 현재 hash가 기록된 어느 hash와도 맞지 않는 대상 — commit이 중단된 동안 들어온 변경 — 은 거절하고
  보고하며 조용히 덮어쓰지 않는다. 거절은 대상, 기록된 두 hash, 그 둘을 담은 stage file을 밝히고
  stage를 남긴다. 대상이 기록된 상태 중 하나를 가지면(없는 상태는 file을 지워서) 해결된다: 사람이
  그 편집을 따로 복사해 두고, stage file 하나를 대상 위에 복사한 뒤 corpus를 다시 열면 commit이
  완료되며, 그 편집은 새 변경으로 다시 한다. 그때까지 모든 열기가 같은 방식으로 거절한다. 손으로
  지운 stage는 commit을 반쯤 적용된 채로 남기므로 거절은 그것을 해결 방법으로 밝히지 않는다.
- 열기는 catalog를 읽고 corpus를 walk하며 문서를 parse하고 index를 유도한다: 모든 문서를 한 번 읽고
  parse하는 첫 전체 읽기다. 열린 corpus는 memory에 두고 운영체제의 file 알림으로 최신을 유지하며, 각
  operation은 lock 아래에서 바로 앞 operation 이후 바뀐 것만 읽는다. 샤드된 table의 기간
  folder([시간 shard](20-document-model.ko.md#시간-shard))는 다른 folder와 같다: 읽을 때 각각 지켜보며,
  table folder 아래에 기간 folder가 생기면 table folder의 항목이 바뀌므로 다음 operation이 그것을 읽는다:
  - folder나 문서는 읽기 전에 지켜보므로 읽은 뒤의 변경을 놓치지 않는다. macOS에서는 각 folder
    (`NOTE_WRITE | NOTE_DELETE | NOTE_RENAME | NOTE_REVOKE`)와 각 문서(그것들과 `NOTE_EXTEND |
    NOTE_ATTRIB`)에 `EV_CLEAR`로 kqueue `EVFILT_VNODE`를 걸며, 각각 `O_EVTONLY`로 열어 descriptor를 하나씩
    쓴다. folder는 사라질 때까지 계속 지켜보므로, 그 이름이 바뀌면 언제나 그 아래의 것을 버린다.
    Linux에서는 열린 corpus마다 instance 하나로 각 folder(`IN_CREATE | IN_DELETE | IN_MOVED_FROM |
    IN_MOVED_TO | IN_MODIFY | IN_CLOSE_WRITE | IN_ATTRIB`와 folder 자신의 event)와 각 문서(`IN_MODIFY |
    IN_CLOSE_WRITE | IN_ATTRIB`와 그 자신의 event)에 inotify를 건다. 그래서 문서의 다른 hard link를 통한
    쓰기도 보이며, 한 inode의 두 경로는 그 watch를 함께 쓴다. Windows에서는 root와 그 subtree에
    overlapped `ReadDirectoryChangesW` 하나(`FILE_NOTIFY_CHANGE_FILE_NAME | FILE_NOTIFY_CHANGE_DIR_NAME |
    FILE_NOTIFY_CHANGE_LAST_WRITE | FILE_NOTIFY_CHANGE_SIZE`)를 건다. 그것이 알리는 short(8.3) 이름은 긴
    이름으로 풀며, 풀 수 없는 것은 overflow로 받는다. 그것은 쓴 경로를 알리므로, corpus 밖의 hard link를
    통한 쓰기는 그 문서가 commit 대상이 되거나 corpus를 다시 열 때까지 거기서 보이지 않는다. 그런 link를
    가진 corpus 문서는 Windows에서 지원하지 않는다. 이것들은 lock 대기의 primitive다
    ([lock](#잡기와-기다리기)).
  - 각 operation은 읽기 전에 lock 아래에서, 바로 앞 operation 이후 쌓인 알림을 기다리지 않고 모두
    가져가 각 알림이 가리키는 것을 버린다: 바뀐 file의 bytes, 바뀐 folder의 항목, 그리고 나타나거나
    사라지거나 이름이 바뀐 이름이면 그 아래의 것과 그 folder의 항목까지. 이어지는 읽기가 그 경로들을
    disk에서 다시 읽는다. operation 사이에는 아무것도 돌지 않고 timer도 없다: kernel이 다음 operation이
    가져갈 때까지 알림을 간직한다. 버림은 각각 `corpus.changed`로, disk에서 읽은 folder나 file은 각각
    `corpus.read`로 발행한다.
  - macOS와 Linux에서는 kernel이 변경을 만드는 system call 안에서 알림을 queue에 넣으므로, operation은
    lock을 잡기 전에 끝난 모든 변경을 본다. `ReadDirectoryChangesW`는 file system이 쓰기 시각을 기록할
    때 쓰기를 알리며, 그것은 더 늦을 수 있다.
  - 잃은 event를 알리는 알림 — `IN_Q_OVERFLOW`, 0 byte, 읽을 수 없는 buffer, `ERROR_NOTIFY_ENUM_DIR`
    — 은 모든 것을 버리고, 다음 읽기가 corpus 전체를 다시 읽으며 `corpus.overflow`로 발행한다: 알림
    계약이 이 경로를 요구한다. kqueue는 등록마다 상태 하나를 두어 event를 잃지 않으므로 overflow가
    없다. 알림을 가져가다 실패하면 이미 가져간 알림을 잃으므로, 역시 모든 것을 버리고 원인을 밝히며
    operation을 `DRIVER`로 실패시킨다.
  - commit은 알림이 무엇을 말하든 lock 아래에서 그 대상을, 위의 모든 folder의 항목과 함께 disk에서
    읽고, 쓴 뒤 다시 버린다. 그래서 [commit protocol](#commit-protocol)의 1단계는 알림의 지연에 결코
    기대지 않는다.
  - 문서는 bytes가 바뀌었거나, 그 parse가 다른 file에서 유도한 link target이 바뀌었을 때 — detail
    section이 생기거나 사라진 행, 공유가 된 key, 행이 나타난 dangling link — 만 다시 parse한다. 다른
    문서의 행은 operation 사이에 그대로 두며 결코 바꾸지 않는다. 쓰기는 그 사본을 바꾼다.
  - 걸 수 없는 watch는 원인을 밝히며 operation을 `DRIVER`로 실패시킨다: macOS에서는 열린 file 한도
    (`ulimit -n`, `kern.maxfilesperproc`), Linux에서는 `fs.inotify.max_user_watches`.

  근거: corpus 전체를 읽던 operation은 시간의 86%를 file을 열고 읽는 데 썼다(10k note 읽기에
  2.7–4.4 s). file content hash를 key로 하는 cache는 그것을 피하지 못한다. hash를 구하려면 모든 file을
  읽기 때문이다. macOS에서 FSEvents는 쓰지 않는다: cgo와 run loop나 dispatch queue가 필요하지만, kqueue는
  이미 lock 대기의 primitive다.

## canon 연산

- `flowmark canon <corpus> [file…]`는 지정한 file을(file을 지정하지 않으면 corpus의 모든 문서를) 한 transaction에서 canonical 형식으로 다시 쓰고, 다시 쓴 file을 답한다. 바이트를 하나도 바꾸지 않는 canon은 아무것도 쓰지 않는다. file은 corpus 경로로 이름을 준다: corpus root에 대한 상대 경로이며, 모든 platform에서 `/`를 쓰고, commit 거절이 밝히는 그 경로다. 두 번 이름을 준 file은 대상 하나다. 대상은 `format.*` 진단이 있는 file의 commit 거절에서 면제된다. 이름을 준 file에 `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있으면 명령은 `CONSTRAINT`로 거절하며 file과 진단을 밝히고 아무것도 쓰지 않는다. 이름을 준 `corpus.walk` 항목도 같은 방식으로 거절하며, corpus 문서도 `corpus.walk` 항목도 아닌 경로를 이름으로 주면 `CONFIG`로 거절한다. file을 주지 않으면 대상은 그 행 사이에 `parent.cycle`이 없고 `format.*` 진단이 `format.noncanonical`뿐인 corpus 문서이며, 그 밖의 모든 corpus 문서와 모든 `corpus.walk` 항목은 각각의 진단과 함께 보고하고 어느 것도 다시 쓰지 않는다. `corpus.walk` file은 결코 대상이 아니다. canon은 논리 값을 바꾸지 않으므로 key, 참조, 제약 검사가 그것을 거절하지 않는다. 링크 유도는 거절할 수 있다: `files` row로의 링크는 그 key에서만 유도되어 table이 필요 없고, block이나 document row로의 링크는 그 row의 table이 필요하다. 사용할 수 없는 block 또는 document table의 row로의 링크를 가진 대상은 이름을 준 canon을 `CONSTRAINT`로 거절하며, 그 대상과 table을 사용할 수 없게 하는 진단의 file을 밝힌다. file을 주지 않으면 그 대상은 두 file과 함께 보고하고 다시 쓰지 않는다. validator 진단이 있는 값은 쓴 텍스트를 유지한다. 문서 바이트를 정규화하는 유일한 연산이며, 논리적 쓰기는 결코 정규화하지 않는다. 근거: migration은 별도의 명시적 transaction이므로, logical 쓰기는 사용자가 쓰지 않은 바이트를 바꾸지 않는다. `corpus.walk` 항목은 잘못된 문서처럼 사람이 고칠 진단을 가지고 table의 자리에 있다. 그 밖의 경로는 canon이 다시 쓸 수 있는 것을 가리키지 않으므로 명령의 실수다. table을 사용할 수 없는 링크는 engine이 읽을 수 없는 row나 두 row가 공유하는 key를 가리킬 수 있어 유도된 target을 알 수 없으며, canon은 유도할 수 없는 target을 결코 쓰지 않는다.
- 명령줄은 `flowmark canon [--lock-wait-ms <n>] <corpus> [file…]`이다: `--lock-wait-ms`는 CLI face의
  lock 대기([잡기와 기다리기](#잡기와-기다리기))로, 기본값은 5000이고 0이면 곧바로 거절한다. `<corpus>`는 corpus
  root다. 다시 쓴 file마다 `[canon] rewrote <file>`, 보고한 file마다 `[canon] left <file>: <진단>`, 그리고
  둘의 개수를 출력한다. canon이 실행되면 보고한 file이 있어도 0으로, 거절되거나 실패하면 실패를 표준
  오류에 쓰고 1로, 명령줄이 잘못되면 2로 끝난다 — CLI의 모든 명령의 종료 코드다
  ([face](#face)).

## Server

- `flowmark serve <corpus> --port <port> [--new-token] [--lock-wait-ms <n>] [--tx-idle-ms <n>]`는
  orm client가 접속하는 상주 face다. `--port`는 필수다. `127.0.0.1:<port>`, 즉 loopback TCP에만 bind하며
  `--port 0`이면 운영체제가 port를 고른다. listen을 시작하면 표준 출력에
  `[serve] listening on 127.0.0.1:<port> for the corpus <corpus>`를 쓰고, lock과 journal event 각각과
  session을 끝내지 못한 실패 각각을 표준 오류에 쓴다. catalog에 diagnostic이 있으면 그것을 밝히며
  시작을 거절한다.
- 처음 시작할 때 무작위 token을 `.flowmark/server.json`에 쓴다: JSON 한 줄 `{"token"}`과 line feed다.
  token은 OS 무작위 source의 128 bit를 소문자 hex 32자리로 쓴 것이며, file은 소유자만 읽고 쓸 수 있게
  만든다. token은 재시작해도 유지되고 `flowmark serve <corpus> --new-token`으로만 바뀐다. file은 partial
  이름으로 쓴 뒤 제자리에 link하므로, 동시에 시작한 두 server는 첫 link가 놓은 token 하나를 유지한다.
  그 한 줄을 담지 않은 file이나 그 자리의 symlink는 `--new-token`을 밝히며 시작을 거절한다.
- DSN은 `markdown://127.0.0.1:<port>/<corpus>?token=<token>`이고 `<corpus>`는 corpus root의 directory
  이름이다. corpus나 token 불일치는 `CONFIG`다. 원격 접속과 다른 인증은 v1 밖이다.
- 연결은 session이다: 요청과 control message의 JSON lines이며 각각 순서대로 한 줄로 답한다(요청과 답의
  형태는 [capability table](60-dialect.md)에 있다). lock holder의 session은 `serve-<n>`이며 `n`은 server의
  연결을 1부터 센다.
- 연결의 첫 message는 DSN의 corpus와 token을 담은 `{"op": "connect", "corpus", "token"}`이며
  `{"ok": true}`로 답한다. corpus나 token이 다르거나(token은 constant time으로 비교한다) 다른 message가
  먼저 오면 `CONFIG`로 답하고 server가 연결을 닫는다. 두 번째 `connect`는 `CONFIG`다. 근거: DSN의 corpus와
  token은 어떤 요청이 corpus를 읽기 전에 server에 닿아야 한다. PostgreSQL의 startup message가 모든
  query보다 앞서고 인증 실패가 연결을 끝내는 것과 같다.
- message는 한 줄의 JSON object 하나다. `op`가 있는 message는 control message이고 나머지는 요청
  message다. 실패는 orm code를 담은 `{"error": {"code", "message"}}`로 답한다 — server는 orm client의
  face이므로 `LOCK_TIMEOUT`은 `CANCELED`로, `CORPUS_CONFLICT`는 `WRITE_CONFLICT`로 답한다. JSON object가
  아닌 줄, 어떤 control message도 아닌 `op`, 그 control message가 받지 않는 field는 구조가 틀린
  message에 대한 orm code인 `IR_INVALID`다. `begin`은 boolean `read_only`를 받으며 없으면 false다.
  `savepoint`, `release`, `rollback_to`는 `name`을 받는다.
- `--tx-idle-ms`(기본 5000, 양의 정수)는 열린 transaction이 앞 message에 답한 때부터 다음 message를
  기다리는 시간의 상한이다. 그 시간이 지나면 transaction을 rollback하고 lock을 풀고,
  `{"error": {"code": "CONNECTION_LOST", "message"}}`를 연결에 쓰고(client는 이것을 다음 message의 답으로
  읽는다) 연결을 닫는다. 근거: PostgreSQL의 `idle_in_transaction_session_timeout`도 session을 끝내며
  (SQLSTATE 25P03), orm의 `CONNECTION_LOST`는 idle 한도를 이름 짓는다. 조용히 rollback한 뒤 연결을 열어
  두면 client가 열려 있다고 믿는 transaction 밖에서 client의 다음 쓰기가 실행된다. 기본값은 lock 대기의
  기본값과 같아서, 쉬는 transaction은 다른 operation이 기본으로 기다리는 시간보다 오래 lock을 잡지 않는다.
  idle 상한이 timer인 까닭은 쉼이 event의 부재이기 때문이며, lock 대기의 deadline 말고는 session의 유일한 timer다.
- 닫힌 연결은 열린 transaction을 rollback하고 lock을 풀며 요청의 lock 대기를 끝낸다. server는 요청이
  실행되는 동안에도 도는 연결의 read로 이를 알며, 받았지만 답하기 시작하지 않은 message는 실행하지 않는다
  — read의 끝은 client의 close이고 server는 이를 half-close와 구별할 수 없으며, session은 그와 함께
  끝난다. `SIGINT`나 `SIGTERM`을 받으면 server는 listen을
  멈추고 모든 연결을 같은 방식으로 닫는다.

## Face

- CLI, MCP server, soksak sidecar는 write path의 얇은 face다. face의 각 operation은 session의
  operation 하나 — IR-executor protocol의 요청, 이력 repair, rewrite, install — 이며, transaction 밖의
  요청([transaction](#transaction))으로 face의 `--lock-wait-ms`를 가지고 lock 아래에서 실행한다. 요청
  시계는 engine 시계다([시계](#시계)). 어떤 face도 corpus file을 직접 읽거나 쓰지 않는다. 근거: write
  path 하나가 모든 규칙을 쥐므로, 어떤 face도 규칙을 달리 적용하거나 건너뛸 수 없다.
- operation은 `operation`으로 이름을 주며 catalog의 어느 table에서나 동작한다: 어떤 face도 table,
  column, 저장 방식의 이름을 갖지 않는다. 값 — `key`, `values`의 각 member, `position`, `parent`, 답하는
  모든 값 — 은 protocol의 runtime value model JSON이다([capability table](60-dialect.ko.md#wire)).
  `key`는 table의 primary key 값이다.

| Operation | Member | 답 |
| --- | --- | --- |
| `schema` | — | `{"manifest_hash", "tables"}`: catalog — 각 table과 그 저장 방식, key, `title`, `body`, `order`, 계층 column, `markdown` column, column, foreign key, unique key, 상태기계 |
| `rows` | `table` | `{"rows"}`: table의 모든 행을 저장 순서로, `columns` 없는 `all` 요청이 답하는 대로 |
| `row.get` | `table`, `key` | `{"row"}`: key가 `key`인 행 |
| `row.insert` | `table`, `values` | `{"affected", "keys", "rows"}` |
| `row.update` | `table`, `key`, `values` | `{"affected", "keys", "rows"}` |
| `row.move` | `table`, `key`, `position`, 선택 `parent` | `{"affected", "keys", "rows"}` |
| `row.delete` | `table`, `key` | `{"affected", "keys", "rows"}` |
| `transition` | `table`, `key`, `state`, 선택 `values` | `{"affected", "keys", "rows"}` |
| `views` | 선택 `key` | view table의 `{"rows"}`, 또는 view `key`의 `{"view", "rows"}` |

- `row.insert`는 `values`를 대입하는 `insert`이고, `row.update`는 key가 `key`인 행의 `update`로
  `values`를 대입하며, `row.delete`는 그 행의 `delete`다. `row.move`는 그 행의 `update`로 `position`을
  table의 `order` column에, `parent`가 주어지면 `parent`를 계층 column `parent`에 대입한다
  ([document model](20-document-model.ko.md#순서)). `order`가 없는 table, 또는 계층이 없는 table의
  `parent`는 `CONFIG`다. `transition`은 그 행의 `update`로 `state`를 table 상태기계의 column에, 함께
  `values`를 대입하며 `values`는 전이가 요구하는 column을 담는다([constraints](50-constraints.ko.md#상태기계)).
  상태기계가 없는 table은 `CONFIG`다. 각각은 그 종류의 요청과 똑같이 검사되고 commit된다. 어떤
  face도 규칙을 더하거나 빼지 않는다.
- 쓰기는 요청의 `{affected, keys}`와 함께, `rows`로 commit 직후 `rows`가 답하는 대로의 table 행을
  답한다. 근거: view는 operation이 돌려준 행에서 다시 그리며, 결코 자기 추측에서 그리지 않는다
  ([application schema](100-checklist.ko.md#view)).
- `key`가 가리키는 행에 대한 operation이 어떤 행에도 맞지 않으면 — `row.get`, 또는 아무 행에도 영향이
  없는 쓰기 — table과 key를 밝히며 한 행 읽기가 아무것도 맞추지 못했을 때의 orm code인 `NO_ROWS`다.
  근거: 이름을 준 행에 대한 쓰기가 아무것도 바꾸지 않으면 조용한 누락이다.
- 알 수 없는 `operation`, 빠진 member, operation이 받지 않는 member, object가 아닌 `values`, 그리고
  `state`가 주는 상태 column을 대입하는 `transition`의 `values`는 그것을 밝히며 `CONFIG`다. catalog가 선언하지 않은 table이나 column은 요청의 `ENTITY_UNKNOWN`이나
  `COLUMN_UNKNOWN`이다.
- `views`는 face가 이름을 준 table이 아니라 catalog의 view table([application schema](100-checklist.ko.md#view))을
  읽는다: `key` 없이는 `rows`처럼 view table의 행을 답하고, `key`가 있으면 `{"view", "rows"}` — view
  행, 그리고 where와 params가 저장된 filter의 것이고 order가 저장된 sort인 `all` 요청으로 읽은 그
  `entity`의 행 — 를 답한다. `group_by`는 view가 적용한다. 두 읽기는 읽기 전용 transaction 하나에서
  실행되므로 view와 그 행은 한 corpus에서 온다. view는 먼저 validator가 판정하는 대로 판정되며
  ([constraints](50-constraints.ko.md#view)), 결함이 있는 view는 각 결함을 밝히며 `CONSTRAINT`다.
  view table의 읽기는 결코 그렇지 않다. view table이 없는 catalog는 `views`를 `CONFIG`로 거절한다.
  근거: view는 저장된 요청이므로 그것을 실행하는 것은 그 요청이며, validator가 보고하는 view는 client가
  스스로 만들 수 없는 요청을 실행하지 않는다.
- 실패는 `{"code", "error"}`로 답한다: [실패 어휘](#실패-어휘)의 face 이름 — 어휘가 나열하지 않는
  code는 orm code — 와 메시지다.
- **CLI.** 각 operation은 명령이다: `flowmark schema <corpus>`, `rows <corpus> <table>`,
  `row.get <corpus> <table> <key>`, `row.insert <corpus> <table> [assignment…]`,
  `row.update <corpus> <table> <key> [assignment…]`, `row.move <corpus> <table> <key> <position> [<parent>]`,
  `row.delete <corpus> <table> <key>`, `transition <corpus> <table> <key> <state> [assignment…]`,
  `views <corpus> [<key>]`.
  assignment는 `<column>=<text>`나 `<column>:=<json>`이고, 위치 값은 `<text>`나 `:=<json>`이다. text는
  값의 읽을 수 있는 텍스트이며 column type의 wire 값으로 보낸다 — 정수와 float는 JSON number, `bool`은
  JSON bool, 그 밖에는 JSON string. `:=`는 wire 값 자체를 받으며 `null`도 받는다. 모든 명령은
  `--lock-wait-ms <n>`(기본값 5000, 0이면 곧바로 거절)을 받고, flag는 위치 인자의 앞에도 뒤에도 올 수
  있다. 답은 표준 출력에 JSON 한 줄로 쓴다. 그 밖의 명령은 `flowmark canon`([canon 연산](#canon-연산)),
  `flowmark install <corpus>`([schema](30-schema.ko.md#install)),
  `flowmark history repair <corpus> <table> <key> [--from <state>]`
  ([constraints](50-constraints.ko.md#엔진-밖에서-바뀐-상태)), `flowmark unlock <corpus>
  [--holder <holder>]`([stale lock](#stale-lock))이다. 모든 명령은 실행되면 0으로, 거절되거나
  실패하면 표준 오류에 `flowmark <command>: <code>: <message>`를 쓰고 1로, 명령줄이 잘못되면 2로
  끝난다. 거절된 unlock은 `CONFIG`이고, 그 아래 file system의 실패는 `DRIVER`다.
- **`flowmark install`**은 lock 아래의 자기 transaction이며, canon처럼 열린 transaction 안에서는
  `CONFIG`다. journal commit 하나로, file이 없는 각 `document` table의 file `<table>.flowmark.md` —
  빈 table, `# <table>`과 LF 하나 — 와, 어느 줄도 `*.flowmark.md text eol=lf`가 아닐 때 그 줄을 마지막
  줄 뒤에(마지막 줄에 LF가 없으면 LF와 함께) 덧붙인, 또는 새 file로 만든 `<corpus>/.gitattributes`를
  쓴다. 그다음 같은 lock 아래에서 folder가 없는 각 `files` table의 folder를 만든다. table 자리의 경로에
  다른 종류의 항목이 있거나 `.gitattributes`가 일반 file이 아니면 그것을 밝히며 install을 `CONFIG`로
  거절하고 아무것도 쓰지 않는다. 만든 file이나 folder마다 `[install] created <path>`를, 줄을 더했으면
  `[install] added the line *.flowmark.md text eol=lf to .gitattributes`를, 그리고 개수를 출력한다.
  모든 것이 제자리에 있으면 아무것도 쓰지 않는다. 근거: install은 멱등이고, folder는 journal이 stage할
  바이트를 갖지 않으므로 그 생성은 commit 뒤의 별도 멱등 단계다.
- **MCP server.** `flowmark mcp [--lock-wait-ms <n>] <corpus>`는 표준 입출력으로 한 줄에 message
  하나씩 JSON-RPC 2.0을 말하며 `initialize`, `tools/list`, `tools/call`을 갖는다. 각 operation은
  `.`을 `_`로 바꾼 operation 이름의 tool 하나이며 그 인자는 operation의 member다. tool은 답의 JSON을
  담은 text content 하나로, 또는 `isError`가 true인 실패의 JSON으로 답한다. 알 수 없는 method는
  JSON-RPC 오류 -32601, JSON이 아닌 줄은 id가 null인 -32700이며, notification에는 답하지 않는다.
- **Sidecar.** `flowmark sidecar [--lock-wait-ms <n>]`은 soksak core spec(`docs/spec/sidecars.md`)의
  protocol v1을 말한다: 요청 `{"surface", "root", "body"}`는 root가 `root`인 corpus에서 body의
  operation을 실행하고, 요청에 `cids`가 있으면 그것과 함께 `{"surface", "body"}`로 답한다. body의
  `id`가 주어지면 답 body에 복사한다. 실패 body는 `{"id", "code", "error"}`이며, member `error`가
  protocol이 읽는 대로 실패한 요청을 말한다. `closed` message에는 `{"surface", "closed": true}`로
  답한다: surface는 아무 자원도 쥐지 않는다. 요청이 아닌 줄은 답할 surface를 밝히지 않으므로 표준
  오류에 쓰며, host가 그것을 log에 남긴다. sidecar는 변경 event를 보내지 않는다: 운영체제 알림으로
  움직이는 corpus watch는 `T12.5-2`의 오래 사는 corpus에 속하며, polling watch는 그 대체가 아니다.

## Lock

- **portable lock 하나.** 모든 operation — 읽기든 쓰기든, 어느 face에서든 — 은 같은 lock file,
  corpus root 아래 `.flowmark/lock`을 잡는다. 경로는 상수이며 `.flowmark/`가 없으면 만든다. kit
  holder-lock 선례를 따른다: holder 식별자와 stale 감지를 갖는 lock file이다. OS file lock은 쓰지
  않으며, 쓰는 platform primitive는 이 절에 이름을 밝힌다. 근거: OS lock(flock, fcntl, LockFileEx)은
  portable하게 취소할 수 있는 대기가 없고, 일부 network file system에서는 local로만 주어지며, holder를
  밝히지 않는다.
- lock은 operation 하나 동안, 또는 server transaction 하나의 `begin`부터 `commit`이나 `rollback`까지
  잡는다. 연결이 닫히거나 transaction이 server flag `--tx-idle-ms`보다 오래 쉬면 transaction을
  rollback하고 lock을 푼다([server](#server)).

### Holder record

- lock file은 JSON 한 줄 `{"pid", "start", "host", "hostname", "session",
  "acquisition"}`과 line feed를 담는다. holder의 이름은 `<pid>@<host>/<session>`이다. `acquisition`은 획득
  하나의 token이다: OS random source의 128 random bit를 나타내는 소문자 hex 32자리이며 그 pending record의
  이름도 된다. 그 밖의 것을 담은 file은 결코 빈 lock으로 여기지 않는다. 근거: session은 자기 process에서
  다시 획득할 수 있으므로, 획득마다의 token만이 두 획득자가 byte 단위로 같은 record를 쓰지 않게 하며, 그렇지
  않으면 아래의 응답 유실 규칙이 둘을 하나로 여긴다.
- `start`는 process의 시작이다: Linux에서는 `/proc/sys/kernel/random/boot_id`의 boot와
  `/proc/<pid>/stat`의 22번째 field, macOS에서는 sysctl `kern.proc.pid.<pid>`의 `kp_proc.p_starttime`,
  Windows에서는 `GetProcessTimes`의 creation time이다.
- `host`는 machine 식별자다: Linux에서는 `/etc/machine-id`(machine-id(5))와 `/proc/self/ns/pid`의 inode,
  macOS에서는 `gethostuuid(2)`, Windows에서는 `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`다.
  없으면 원인을 밝히며 lock을 거절한다. `hostname`은 host 이름이며 메시지를 위해서만 기록한다.
- 이 host의 holder는 그 ID를 가진 process가 없거나, 그 process의 start가 다르거나(재사용된 ID), 그
  process가 zombie이면 끝난 것이다. 존재하지만 start를 읽을 수 없는 process는 실행 중으로 센다. process
  ID가 있는지는 Linux와 macOS에서 `kill(pid, 0)`으로 정한다: `ESRCH`는 없음, 성공이나 `EPERM`은 있음이다.
  Linux에서 `/proc/<pid>/stat`을 읽을 수 없으면(hidepid) `kill(pid, 0)`을 다시 묻고, 그때 `ESRCH`는 끝남이다.
  Windows에서는 `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION | SYNCHRONIZE)`로 정한다:
  `ERROR_INVALID_PARAMETER`는 없음, `ERROR_ACCESS_DENIED`는 있음이다. 그 생존은 `WaitForSingleObject(h, 0)`로
  읽으며 exit code로는 결코 읽지 않는다. exit code 259는 `STILL_ACTIVE`와 같기 때문이다.
- 다른 host의 holder는 알 수 없음이며 결코 끝났다고 판정하지 않는다. corpus는 한 번에 한 host에서
  쓴다. 다른 host의 release는 file system이 원격 변경을 알리는 곳에서만 대기자에게 닿으므로, 그런 대기는
  deadline에 끝날 수 있다.

### 잡기와 기다리기

- record는 `.flowmark/lock.<acquisition>.pending`에 통째로 쓰고 sync한 뒤
  `.flowmark/lock`으로 hard link한다. lock이 있으면 link가 실패하므로 lock은 record 전체와 함께
  atomic하게 나타난다. 대기자는 pending record를 한 번만 쓴다. pending record는 배타에 관여하지 않는다.
- link가 `EEXIST`로 실패한 시도는 lock을 읽는다. 잡으려는 쪽 자신의 record와 byte 단위로 같은
  record(그 획득 token은 유일하다)는 응답을 잃었어도 link가 이루어졌다는 뜻이며(NFS), lock을 잡은 것이다.
  읽기 전에 사라진 lock은 곧바로 다시 시도한다.
- 실행 중인 holder나 다른 host의 holder가 잡은 lock을 만난 operation은 그 release를 기다리며, deadline이
  먼저 지나면 holder와 그 상태를 밝히는 `LOCK_TIMEOUT`으로 실패한다. 끝난 holder는 곧바로 거절한다.
  deadline이 대기의 유일한 timer다: CLI, MCP, sidecar는 `--lock-wait-ms <n>`(기본 5000, 0이면 곧바로
  거절)을 받고 server도 같은 flag를 받는다. 연결이 닫히면 대기가 끝난다. 근거:
  곧바로 거절하면 재시도 loop가 모든 호출자에게 옮겨 간다.
- 대기는 lock에 대한 OS 변경 알림이 이끈다: Linux에서는 `.flowmark/`에 대한 inotify(`IN_DELETE |
  IN_MOVED_FROM | IN_DELETE_SELF | IN_MOVE_SELF | IN_UNMOUNT`)이며, server의 session들처럼 한 process가 한
  corpus에서 하는 모든 대기가 instance 하나를 나눈다. macOS에서는 `O_EVTONLY`로 연 lock file에 대한 kqueue
  `EVFILT_VNODE`(`NOTE_DELETE | NOTE_RENAME | NOTE_REVOKE`, `EV_CLEAR`)다. Windows에서는 `FILE_LIST_DIRECTORY`,
  `FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OVERLAPPED`, `FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE`로
  연 `.flowmark/`에 대한 overlapped `ReadDirectoryChangesW`다.
- lock file의 제거나 rename 하나, overflow 하나(`IN_Q_OVERFLOW`, 0 byte 반환, `ERROR_NOTIFY_ENUM_DIR`),
  지켜보는 folder 자체의 event 하나(제거, rename, unmount, `IN_IGNORED`)는 정확히 한 번의 획득 시도를 더
  시작한다. 다른 event나 timer는 시도를 시작하지 않는다. 대기자의 pending record 같은 다른 이름의
  제거는 시도를 시작하지 않는다.
- watch는 실패한 시도 뒤에 arm하며, 그 사이의 release를 잃지 않는다. Linux와 Windows에서는 arm한 뒤
  lock을 한 번 더 시도한다. macOS에서는 시도가 실패한 뒤 lock을 `O_EVTONLY`로 열고(lock이 없으면
  곧바로 다시 시도한다) 등록한 뒤, descriptor의 `fstat`을 `.flowmark/lock`의 `lstat`과 비교한다: link
  count 0이나 다른 device 또는 inode는 시도를 한 번 더 시작한다. 이후의 vnode event도 같은 비교로
  판정한다: lock 경로가 여전히 link count가 0보다 큰 지켜보는 inode를 가리키면 그 event는 그 file의 다른
  이름(잡으려는 쪽의 pending record)에서 온 것이므로 시도를 시작하지 않는다. file 하나의 변경은 event를
  몇 번 일으키든 시도를 한 번 시작한다.
- arm할 수 없는 watch는 원인을 밝히며 대기를 곧바로 거절한다: `EMFILE`은
  `fs.inotify.max_user_instances`를, `ENOSPC`는 `fs.inotify.max_user_watches`를, `ERROR_INVALID_FUNCTION`은
  지원하지 않는 file system을 밝힌다.
- holder의 종료는 event다. holder가 이 host에서 실행 중이면 대기자는 그 종료도 지켜본다: Linux에서는
  `pidfd_open`의 pidfd(Linux 5.3 이상이 필요하다)를 runtime의 poller가 inotify descriptor 곁에서
  기다린다. macOS에서는 같은 kqueue의 `EVFILT_PROC NOTE_EXIT`이다. Windows에서는 process handle(`SYNCHRONIZE |
  PROCESS_QUERY_LIMITED_INFORMATION`)을 변경 event와 함께 `WaitForMultipleObjects`로 기다린다 — 이 사용자가
  handle을 열 수 없는 process는 지켜보지 않으며, 그 release는 lock으로만 알게 된다. arm한 뒤 holder를 다시
  판정하므로 재사용된 process ID를 지켜보는 일은 없다. 종료는 시도를 시작하지 않는다: holder를 다시
  판정하며, lock이 아직 그 record를 담고 있는 끝난 holder는 끝났다고 밝히는 `LOCK_TIMEOUT`으로 곧바로
  거절한다.
- Windows에서 lock file, pending record, stage file을 여는 일은 모두 `FILE_SHARE_READ | FILE_SHARE_WRITE |
  FILE_SHARE_DELETE`를 준 `CreateFileW`이므로, 읽는 쪽이 holder의 delete나 unlock을 막지 않는다.
- 잡기, 풀기, 치우기, 기다리기는 event `lock.acquired`, `lock.released`, `lock.cleared`,
  `lock.waiting`(기다리는 holder마다 한 번, watch를 arm한 뒤)으로 publish한다.

### Stale lock

- stale lock은 operation이 결코 지우지 않는다. `flowmark unlock <corpus>`가 지운다:
  1. record R을 읽는다. 실행 중인 holder는 거절하고, 다른 host의 holder는 그것을 가리키는
     `--holder <pid>@<host>/<session>`이 없으면 거절한다.
  2. `.flowmark/lock`을 `.flowmark/lock.<R의 SHA-256 앞 hex 32자리>.clearing`으로 link한다. `EEXIST`는 그
     file을 같은 holder의 다른 unlock — 실행 중이거나 중단된 — 으로 밝히며 거절한다.
  3. claim을 읽는다. R이 아닌 record는 새 holder가 lock을 잡았다는 뜻이다: claim을 지우고 거절한다.
  4. 그렇지 않으면 `.flowmark/lock`을, 이어서 claim을 지우고 그 record와 함께 `lock.cleared`를
     publish한다.
- 근거: holder와 claim을 가진 unlock 하나만 lock을 지우므로, claim과 제거 사이에 lock은 여전히 R을
  가리키며, 실행 중인 holder의 lock은 결코 옮겨지지 않는다.
- 중단된 claim은 다음 unlock의 거절이 밝히며, 사람은 실행 중인 unlock이 없음을 확인한 뒤 그것을
  지운다. 끝난 holder의 `LOCK_TIMEOUT`은 `flowmark unlock <corpus>`를, 다른 host의 holder의 것은
  `--holder`를 밝힌다.

## Transaction

- transaction 안의 요청은 server의 memory에 있는 그 post-image를 읽는다. savepoint는 그 중첩 단계이고
  `rollback_to`는 그 위의 단계를 버린다. `.flowmark/tx/`의 journal stage는 commit 하나의 3단계부터
  8단계까지만 존재한다.
- 격리. 엔진 session들은 lock으로 직렬화된다: transaction은 `begin`부터 끝까지 lock을 잡으므로 어떤 엔진
  쓰기도 그 사이에 끼어들지 않는다. 엔진 밖의 편집에 대해 transaction은 snapshot과 비슷하다: commit은 대상을
  확인하고(1단계) 다시 읽은 corpus에서 쓰기를 다시 실행하지만(2단계) 읽기가 돌려준 것은 검증하지 않으므로,
  읽은 행에 근거한 결정이 다른 file의 바깥 편집으로 바뀐 뒤에도 commit될 수 있다 — write skew다. MySQL의
  `REPEATABLE READ`도 같다(MySQL 8.4.11에서 확인: 두 transaction이 어떤 상태의 두 행을 각각 읽고 서로 다른
  하나를 그 상태에서 옮겼으며, 둘 다 commit되어 하나도 남지 않았다). PostgreSQL의 `READ COMMITTED`와
  `REPEATABLE READ`도 같다(PostgreSQL 문서 Transaction Isolation; `SERIALIZABLE`만 감지한다). 근거: 바깥
  편집은 lock을 잡지 않으므로 lock 기반 격리는 그것을 포함할 수 없고, 모든 읽기를 검증하려면 모든 요청의
  read set이 필요한데 orm client는 그것을 보내지 않는다.
- transaction 밖의 요청 — 읽기, 쓰기, 이력 repair — 은 lock 아래 transaction 하나다: lock을 잡고 corpus를
  읽으며, 쓰기는 lock을 풀기 전에 commit한다.
- transaction 안에서 실패한 요청은 아무것도 바꾸지 않으며, transaction은 그 요청 전 상태로 열린 채
  남는다. 근거: SQL 문장 원자성 — 실패한 문장은 효과가 없고 MySQL과 SQLite는 transaction을 열어 둔다.
  orm client는 callback이 실패하면 transaction이나 그 savepoint를 rollback한다.
- `savepoint <name>`은 transaction의 현재 상태에 단계를 둔다. 같은 이름의 savepoint는 SQL 표준이 그것을
  없애듯 대체된다. `release <name>`은 그 savepoint와 그 뒤에 둔 모든 savepoint를 지우고 그 쓰기는
  유지한다. `rollback_to <name>`은 그 savepoint 이후의 쓰기와 그 뒤에 둔 모든 savepoint를 버리고 그
  savepoint는 남긴다. 근거: SQL 표준의 savepoint 문장이다. PostgreSQL과 SQLite는 대신 가려진 savepoint를
  남기지만, orm처럼 깊이로 savepoint 이름을 짓는 client는 그 경우를 만나지 않는다.
- 읽기 전용 transaction은 lock을 잡으며, PostgreSQL이 읽기 전용 transaction 안의 쓰기를 거절하듯(SQLSTATE
  25006) 모든 쓰기를 `READ_ONLY`로 거절한다.
- session 상태가 허용하지 않는 control message는 orm의 transaction 오용 code인 `CONFIG`다: transaction이
  열린 동안의 `begin`, transaction 없는 `commit`, `rollback`, `savepoint`, `release`, `rollback_to`, 이름
  없는 savepoint, transaction의 어떤 savepoint도 갖지 않은 이름의 `release`나 `rollback_to`. 근거: 이
  corpus lock이 따르는 단일 writer lock의 SQLite는 중첩된 `BEGIN`과 모르는 savepoint를 거절한다.
- prepare record 전에 실패한 commit은 아무것도 쓰지 않는다: transaction을 끝내고, 같은 lock 아래에서
  부분 stage를 버리며, lock을 푼다. 실패한 SQL `COMMIT`이 transaction을 rollback하는 것과 같다. prepare
  record부터 commit은 결정된다: 그 뒤에 실패한 단계에는 같은 lock 아래 복구가 이어져 commit을 완료하며,
  commit은 성공한다. 그 복구도 실패하면 commit은 prepare되었고 다음 lock 획득에서 완료되며 반복하면 안
  된다고 밝히는 `DRIVER`다. 쓰인 commit 뒤에 lock을 풀지 못하면 그 실패는 commit이 쓰였다고 밝힌다. 근거:
  prepare record는 write-ahead log의 지속된 commit record처럼 journal의 commit 시점이므로, 그 뒤의 실패를
  복구가 적용할 commit인데도 일어나지 않은 commit으로 답하지 않는다. lock은 panic을 포함해 operation의
  모든 출구에서 풀린다.
- write path는 단일 operation 쓰기 — transaction 밖의 쓰기, 이력 repair, rewrite, 모든 face에서, transaction
  밖의 server 요청을 포함해 — 를 그 commit이 `CORPUS_CONFLICT`를 만나면 한 번 재시도한다: 같은 lock 아래에서
  corpus를 다시 읽고 operation을 다시 실행한다. server transaction은 server가 재시도하지 않으며 orm client는
  `WRITE_CONFLICT`를 받고 이를 재시도하지 않는다. 근거: 한 곳이 모든 face를 위해 재시도하며, 재시도는 읽기와
  commit 사이에 결코 lock을 풀지 않는다.
- `flowmark canon`과 add-column migration은 각각 자기 transaction이다: 읽기 전용을 포함해 열린
  transaction 안에서는 `READ_ONLY`가 아니라 `CONFIG`다. 자기가 시작하지 않은 transaction 안에서는 결코
  실행되지 않기 때문이다. 각각은 자기 대상의 format 진단 중 면제되는 것을 밝힌다([commit protocol](#commit-protocol),
  2단계).

## Commit protocol

1. 모든 대상이 읽은 뒤 바뀌지 않았는지 확인한다. 바뀌었으면 바뀐 대상마다 그 줄 — 지금의 바이트가 읽은
   바이트와 처음 다른 줄, 둘 중 하나에 file이 없으면 1번 줄 — 을 밝히는 `CORPUS_CONFLICT`로 실패한다.
2. post-image를 적용해 lock 아래 다시 읽은 corpus에 대해 제약 검사를 실행한다 — `begin` 때의 snapshot도,
   예측한 대상 file 집합도 아니다. 검사는 각 쓰기에 대한 executor의 검사다: 다시 읽은 corpus가
   transaction이 읽은 corpus와 다르면 — 문서의 바이트나 table의 사용 가능성이 — transaction의 쓰기를 다시
   읽은 corpus에서 순서대로 다시 실행한다. 그 쓰기가 거절되면 commit은 그 code로 실패한다. 처음과 다르게
   답하는 쓰기나 다른 net post-image는 바뀐 file마다 처음 바뀐 줄을 밝히는 `CORPUS_CONFLICT`로 실패한다:
   쓰기가 들어온 변경에 의존하며 client는 이미 처음 답을 가지고 있다. 이는 SQL serialization failure(SQLSTATE
   40001)에 해당하는 것이며, client가 재시도하지 않으므로 orm은 이를 `DEADLOCK`이 아닌 `WRITE_CONFLICT`로
   보고한다(orm은 40001을 `DEADLOCK`으로 대응시킨다). 다시 읽은 corpus가 읽은 corpus와 같으면 쓰기는 실행될 때 이미 그 위에서 검사되었다.
   아래의 다시 쓰기 거절도 SQL이 deferred가 아닌 제약을 검사하듯 각 요청이 실행될 때 검사하므로, 그런
   file을 다시 써야 하는 요청은 곧바로 실패한다. 다른 행의 validator 진단(`type`, `key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, `order`, `state.transition`, `state.required`, `state.limit`, `history.gap`, `history.chain`, 여러 `files` file에 걸친 `parent.cycle`)은 commit을 실패시키지 않되, `type` 진단은 commit의 제약 검사가 그 값을 평가해야 할 때 commit을 실패시키고, 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.ko.md#진단)); `format.*` 진단은 다시 쓰기 거절과 사용할 수 없는 table 상태([canonical format](10-canonical-format.ko.md#진단))가 정하는 경우에만 commit을 실패시킨다. file을 다시 써야 하는 commit — 행 자신의 file, 유도된 target이 바뀌는 행(상세 섹션이 생기거나 사라지거나, insert가 dangling link에 그 행을 주는 행)으로의 link를 가진 file, `order`의 형제 — 은 그 file이 `format.*` 진단을 가지면 거절한다: `CONSTRAINT`이며 file과 진단을 밝히고, `format.noncanonical`이면 `flowmark canon`을 밝힌다. 세 commit은 면제된다: 대상에 대한 `flowmark canon`([operations](#canon-연산)), 추가된 column이 일으킨 진단에 대한 add-column migration([migration](80-migration.ko.md)) — 그 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다 —, `files` 행의 file 전체를 지우는 delete는 rewrite 거절에서만 면제되며, restrict 검사는 그 행을 참조할 수 있는 모든 table의 행을 여전히 필요로 하며(자신의 table을 `parent`로 하는 것, 자식 block과 이력 행, 그 행으로의 foreign key를 가진 모든 table), 그 중 하나라도 사용할 수 없으면 그 delete는 `CONSTRAINT`다.
3. 각 post-image를 `.flowmark/tx/<id>/`에 `<hash>`로 content-addressed하여 한 번 쓰고, rename n의
   source로 그 사본 `<n>.<hash>`를 쓴다. 둘 다 stage를 거둘 때까지 남는다. 이 단계 전에 모든 대상의
   folder와 만들 모든 folder가 `.flowmark/tx/`의 device(`st_dev`, Windows에서는
   `GetFileInformationByHandle`의 volume serial number)에 있는지 확인한다. 다르면 folder와 "a mount point
   inside the corpus is unsupported"를 밝히며 commit을 거절하고, 아무것도 stage하지 않는다.
4. 모든 대상의 pre-image를 stage에 `<hash>`로 content-addressed하여 쓴다.
5. **prepare record**를 쓴다: pre-hash와 post-hash를 가진 모든 rename. 없는 file은 빈 hash `""`다:
   create는 빈 pre-hash를, delete는 빈 post-hash를 가진다.
6. 모든 대상을 확인한 뒤 runtime의 atomic rename으로 rename을 실행한다: post-image를 가진 대상은 끝난
   것이고, pre-image를 가진 대상은 rename하며, 그 밖의 대상은 stage 전체를 거절한다. delete는 대상을
   stage 안의 `<n>.deleted`로 rename한다. folder가 없는 create는 먼저 그 folder를 corpus root에서부터
   symlink를 따르지 않고 만든다. Windows에서는 없는 folder마다 stage 안에 `<n>.dir.<k>`로 만들어 제자리로
   옮긴다. replay도 똑같이 한다: 제자리에 있는 folder는 건너뛰고 stage에 있는 folder는 옮기며, 대상이
   pre-hash에 있고 `<n>.<hash>`가 없으면 `<hash>`에서 사본을 다시 쓴다. 중단으로 남은 빈 table folder는
   빈 table이다([문서 모델](20-document-model.ko.md#저장)).
7. commit record를 쓴다.
8. stage를 거둔다: prepare record를 먼저 옆으로 rename하므로, 거두는 도중 중단된 stage는 commit
   record만 가지며 다시 거두어진다. 거두기는 `<n>.deleted` file을 지운다.

근거: rename은 대상에게 source의 file을 준다. rename마다 별개의 사본은 같은 두 대상을 두 file로
유지하고 대상이 stage와 file을 나누지 않게 하며, 기록된 두 상태가 복구 거절을 위해 stage에 남는다.
journal은 hard link가 필요 없다.

journal과 복구의 모든 hash는 file의 raw byte의 SHA-256이며 소문자 hex 64자리다(raw-file-bytes hashing,
[design plan](../plans/design-plan.ko.md)). 각 단계는 publish된다: 5단계 뒤 `journal.prepared`, 7단계 뒤
`journal.applied`, 8단계 뒤 `journal.collected`, 열 때 `journal.replayed`나 `journal.discarded`다.

전원 손실을 넘는 내구성은 각 단계의 file과 folder 항목이 다음 단계가 시작하기 전에 disk에 있기를
요구한다. file은 runtime의 sync(`fsync`, macOS에서는 `F_FULLFSYNC`, Windows에서는 `FlushFileBuffers`)로
sync한다. 만들거나 rename하거나 지운 이름은 Linux와 macOS에서 POSIX가 정하듯 그 folder를 sync하여
지속되게 하고, folder flush가 없는 Windows에서는 move가 disk에 닿은 뒤 돌아오는 `MoveFileEx`와
`MOVEFILE_WRITE_THROUGH`로 rename하여 지속되게 한다. 그래서 모든 stage file은 부분 이름에 쓴 뒤 제자리로
rename하고, delete는 rename이다. 만든 folder는 Linux와 macOS에서 그 부모를 sync하여, Windows에서는 stage
안에 만들어 `MOVEFILE_WRITE_THROUGH`로 제자리에 옮겨 지속되게 한다. lock은 folder 항목으로 sync하지
않는다: 전원 손실은 그 holder를 끝내며, 그 record는 link 전에 sync한다. Windows에서 holder의 생존은 그
process handle에서 읽으며 결코 exit code에서 읽지 않는다.

## File system

corpus root는 hard link와 변경 알림을 가진 local file system 하나에 있다: macOS에서는 APFS나 HFS+,
Linux에서는 ext4, XFS, Btrfs 같은 것, Windows에서는 NTFS(또는 hard link를 가진 ReFS)다. FAT32와 exFAT은
hard link가 없어 lock이 record 전체와 함께 atomic하게 나타날 수 없으므로 지원하지 않으며, 거기서 lock의
link 실패(`EPERM`, `ENOTSUP`, `ERROR_INVALID_FUNCTION`, `ERROR_NOT_SUPPORTED`)는 그 원인을 밝히며 거절한다.
변경 알림을 보내지 않는 file system은 지원하지 않는다. 다른 file system의 mount point인 corpus folder는
지원하지 않으며, 거기에 쓰려는 commit은 거절한다([commit protocol](#commit-protocol), 3단계).

## 시계

엔진이 할당하는 모든 시각 — `updated`, `soft_delete`, `default now`, 이력 행의 `at` — 은 orm 요청이면
요청의 `now`, 아니면 microsecond로 자른 엔진의 UTC 시계다. 요청 하나는 시계 값 하나를 가진다.
`datetime(p)` column에 쓰일 때는 p자리 소수로 자르며 반올림하지 않고, column과 비교할 때 `now`와 상대
시각 함수는 여섯 자리를 유지한다. 근거: orm의 시계 규칙 — ORM이 쓰거나 비교하는 시계는
microsecond를 유지한다. MySQL `CURRENT_TIMESTAMP(p)`와 orm의 SQLite `now` slot은 쓰는 시계를 자르고,
MySQL과 PostgreSQL은 `datetime(p)` column을 `NOW(6)`와 여섯 자리로 비교한다.

## 비용 모델

- command별 p95 지연 목표를 여기서 선언한다([open questions](110-open-questions.md)의 corpus 규모에서
  수백 millisecond).
- 열린 corpus의 operation은 바뀐 것만 disk에서 읽고, parse가 달라질 수 있는 문서만 parse한다
  ([corpus 열기](#corpus-열기)). 그래도 memory 안에서 corpus의 행을 훑으므로 그 비용은 bytes가 아니라
  행에 따라 커진다. `writepath`의 benchmark `BenchmarkOpen`, `BenchmarkRead`, `BenchmarkWrite`가 이를
  규모 한계에서 잰다: 작은 corpus 옆에 행 하나짜리 문서 10k를 두고, 각 operation의 CPU 시간과 wall
  시간의 95번째 백분위를 보고한다. macOS(Apple M3 Pro, `T12.5-2`)에서 읽기는 CPU 68 ms, 쓰기 — lock
  아래의 transaction 하나를 journal로 commit — 는 216 ms로 예산 안이다. wall 시간 p95인 273 ms와 1.1 s는
  11 core에서 load average가 100 가까운 기계에서 쟀으므로, wall 시간 p95는 `T12.13`에서 Linux, Windows와
  함께 한가한 기계에서 다시 잰다. corpus를 열 때의 첫 전체 읽기는 CPU 1.2 s다.
- 첫 전체 읽기는 열린 corpus의 시작 비용이다: 모든 문서를 한 번 읽고 parse하며, 모든 folder와 문서의
  watch를 건다.
- memory에 둔 corpus는 각 문서의 bytes, executor의 두 읽기 pass의 parse, 문서의 watch를 가진다. 그
  예산은 문서마다 행 하나일 때 — 행이 가장 많이 드는 경우 — 행마다 heap 8 KB이며,
  `TestKeptHeapBudget`이 지킨다(macOS에서 4.5 KB). 행이 많은 문서도 통째로 가진다. 그것을 parse할 때
  할당하는 것은 아래 codec의 비용이다.
- 쓰기는 여전히 바뀐 대상마다 통째로 parse하고 render하며, executor의 post-image는 corpus의 행을
  복사하므로 쓰기 비용은 대상 file과 행에 따라 커진다. 대상 file의 크기를 묶는 것은 [규모 한계](#규모-한계)이며,
  대상의 byte당 비용은 codec의 비용이다.
- **codec의 비용**은 file의 byte에 선형이다: 상세 섹션을 가진 행들의 document file에서, file의 parse와
  render는 각각 그 megabyte당 CPU 0.1초 이하가 들고 그 byte당 64 byte 이하를 할당한다. 어떤 pass도 각
  heading을 각 상세 anchor와, 또는 각 행을 각 행과 비교하지 않는다: heading의 slug는 자기가 가져가는
  anchor를 조회로 찾고, 상세 섹션은 자기 행을 key로 찾는다. `store`의 `TestCodecAllocationBudget`이 1 MB와
  4 MB에서 할당 예산을 지키며, `BenchmarkParseDocument`와 `BenchmarkRenderDocument`가 megabyte당 둘 다를
  잰다. macOS(Apple M3 Pro, `T12.5-4`)에서 1 MB의 parse는 48 ms가 들고 byte당 50 byte를 할당하며, 그
  render는 35 ms와 39이고, 5 MB에서는 megabyte당 43 ms와 33 ms, 53과 41이다. `T12.5-4` 전에는 parse가
  1 MB에 2.1초, 5 MB에 19.5초가 들고 byte당 456과 667 byte를 할당했으며, 행 수의 제곱으로 커졌다. 근거: parse는 행을 한 번
  읽고, [충돌 규칙](10-canonical-format.ko.md#상세-heading과-anchor)이 필요로 하는 각 viewer 읽기의 block을
  훑고, 그 읽기들에서 heading과 문단의 inline 텍스트를 읽으며, 정식 형식을 검사하려고 행을 다시
  render한다. 각 pass는 file 크기의 구조를 만들므로 할당은 byte의 상수배이며, 64는 잰 배수에 여유를
  남기면서도 file보다 빨리 커지는 비용은 4 MB에서 넘는다. megabyte당 0.1초는 megabyte 대상으로의 쓰기를
  latency 목표의 수백 millisecond 안에 둔다. 할당이 test가 지키는 예산인 것은, 기계의 부하가 시간은
  바꾸지만 byte는 바꾸지 않기 때문이다.
- commit은 transaction이 결과를 바꿀 수 있는
  모든 제약을 검사한다: 대상 행의 key, 참조, check, 순서, limit, 전이, `history.gap`과, 그 행들이 참조하거나
  참조되는 행들이며 — lock 아래 다시 읽은 corpus에서 평가한다. 전체 `flowmark validate`는 요청할 때와
  catalog가 바뀔 때 실행한다.
- 오래 사는 face(sidecar, server)는 corpus를 지켜보다 host에 변경 event를 push할 수 있다. 쓰기는 여전히
  transaction protocol로만 간다.

## 규모 한계

엔진은 위의 지연 목표 안에서 제공하는 corpus의 크기를 밝히고, corpus가 그것을 넘으면 알린다.

- **한계**는 문서 하나에 1 MiB, folder 하나에 문서 20,000개, corpus 하나에 문서 20,000개다.
  `catalog.Envelope`가 이 세 수를 갖고, 명세와 code는 다른 수를 적지 않는다.
- **측정**은 `writepath`의 benchmark(`BenchmarkFileSizeWrite`, `BenchmarkShardOpen`, `BenchmarkShardInsert`,
  `BenchmarkShardRead`, `BenchmarkSessions`)이며 `T12.5-3` 시점의 macOS(11 core)에서 돌렸다:
  - 문서의 행 하나를 갱신하면 32 KB에서 83 ms(p95), 340 KB에서 239 ms, 1.4 MB에서 757 ms가 들고 KB당
    CPU 0.7 ms다. 쓰기가 대상을 통째로 parse하고 검사하고 렌더하기 때문이다. 1 MiB는 행 하나의 갱신이 아직
    수백 millisecond 안에 드는 크기다.
  - folder의 문서가 5,000개에서 20,000개일 때 열기는 336 ms, 697 ms, 1,245 ms(p95)이고 문서 삽입은 99 ms,
    129 ms, 249 ms다. 문서당 0.06 ms다. 문서 20,000개에서 삽입이 지연 목표에 닿으며, 이것이 folder와 corpus의
    수를 정한다.
  - 달 단위 shard는 열기, 삽입, 읽기를 노이즈 안에 둔다. shard는 folder가 끝없이 커지지 않게 할 뿐 같은
    문서의 corpus를 빠르게 하지 않는다.
  - macOS에서 감시는 folder와 문서마다 descriptor 하나를 쥔다(문서 10,000개에서 20,009개). 문서의 수는 process가
    필요로 하는 descriptor의 수이기도 하다. 그것을 얻지 못한 열기는 file을 밝혀 `DRIVER`로 실패하고 lock을
    남기지 않는다.
  - examples의 board가 글 10,000개, 달 folder 열 개에 1,000개씩일 때 이 모양이며 `flowmark serve`로 제공하면
    서버는 3.1초에 준비되고 140 MB를 쥐고 descriptor 10,029개를 잡으며 orm을 거쳐 글 열 개의 페이지에 72 ms,
    글 하나에 44 ms, 새 글에 156 ms로 답한다.
  - session은 제한하지 않는다. lock이 직렬화하므로 쓰기 하나의 시간은 같고, 동시에 쓰는 session 열여섯 중
    마지막은 1.3초를 기다린다.
- **한계를 넘으면** corpus는 전과 같이 읽고 쓴다. validator가 알린다: 1 MiB를 넘는 문서에 `scale.file`, 한계보다
  문서가 많은 folder에 `scale.folder`, 그만큼 많은 corpus의 root에 `scale.corpus`이며, 각각 넘은 것과 그 크기,
  table, 한계를 밝힌다. 문서를 1 MiB 넘게 쓴 commit은 file과 byte와 한계를 밝힌 `scale.exceeded` event를 낸다.
  근거: 쓰기를 거절하는 한계는 그 한계를 넘어 자란 corpus를 크기의 실수로 쓸 수 없게 만든다. 비용은 사용자가
  정할 일이므로 엔진은 알린다. 쓰기는 엔진이 올바르게 쥘 수 없는 것에만 거절한다. process가 얻지 못한 감시의
  descriptor이며 `DRIVER`다.
- 문서와 folder의 크기는 저장 선언의 문제다. 1 MiB를 넘어 자라는 document table은 `files`로 저장하거나 시간으로
  샤드한다([시간 shard](20-document-model.ko.md#시간-shard)).
- Linux와 Windows는 같은 benchmark로 잰다. macOS와 다른 수는 한계의 근거를 바꾸며 수단을 바꾸지 않는다.

## 실패 어휘

| face 이름 | orm code | 뜻 |
| --- | --- | --- |
| `LOCK_TIMEOUT` | `CANCELED` | lock 대기가 끝났다 — deadline에, 또는 holder가 끝났으면 곧바로. holder와 그 상태를 밝힌다 — SQLite의 `busy_timeout`이 그렇듯 `CANCELED`다 |
| `CORPUS_CONFLICT` | `WRITE_CONFLICT` | 읽은 뒤 대상이 바뀌었거나, 읽은 뒤의 변경이 transaction의 쓰기를 다른 쓰기로 만든다. 각 file과 줄을 밝힌다 |
| `DUPLICATE_KEY` | `DUPLICATE_KEY` | primary나 unique key 값이 이미 있다. 두 행(table과 key)을 밝힌다 |
| `FOREIGN_KEY` | `FOREIGN_KEY` | 참조가 어떤 행도 가리키지 않거나, 참조된 행을 참조되는 동안 지우려 했다 |
| `CONSTRAINT` | `CONSTRAINT` | catalog의 다른 규칙이 쓰기를 거절했다 — check, 제목, 전이, 필수 column, limit, 순서, 한 줄 자리. 규칙, 그리고 행과 column을 밝힌다([canonical format](10-canonical-format.ko.md#진단)); 비정규이거나 잘못된 문서를 다시 써야 하는 쓰기, 또는 사용할 수 없는 table의 행이 필요한 요청(읽기나 쓰기)은 file과 진단을 밝히며 `CONSTRAINT`다([canonical format](10-canonical-format.ko.md#진단)) |
| `READ_ONLY` | `READ_ONLY` | 읽기 전용 transaction 안의 쓰기 |
| `CONFIG` | `CONFIG` | session 상태가 허용하지 않는 control message([transaction](#transaction)), corpus 문서를 가리키지 않는 `flowmark canon` 경로([canon 연산](#canon-연산)), 또는 알 수 없거나 member가 빠졌거나 table이 선언하지 않은 setting을 가리키는 face operation([face](#face)) |
| 복구 거절 | `DRIVER` | 대상, 기록된 두 hash, 그 둘을 담은 stage file을 밝힌다. 해결될 때까지 열기와 lock을 잡는 모든 operation을 거절한다 — SQLite의 `SQLITE_CORRUPT`가 그렇듯 다른 code가 밝히지 않는 database 오류의 orm code인 `DRIVER`다. journal 아래 file system의 실패도 `DRIVER`다 |

모든 실패 메시지는 원인과 고치는 방법을 밝힌다.
