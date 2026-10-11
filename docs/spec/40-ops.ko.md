<!-- doc-id: spec-ops -->
<!-- source-sha256: 13150fd36e54e8b7afd50a4a93265aec10e4bd964d3a7d0327b7b26865fa43b3 -->
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
- 열기는 catalog를 읽고 corpus를 walk하며 문서를 parse하고 index를 유도한다. 각 operation은 lock 아래에서
  corpus를 다시 읽는다. operation이 바뀌지 않은 file을 읽지 않는 방법 — 운영체제의 file 알림으로 최신을
  유지하는 오래 사는 corpus이며, commit 대상은 언제나 lock 아래에서 다시 읽는다 — 은 `T12.5-2`가 맡는다.
  file content hash를 key로 하는 cache는 그 일을 하지 못한다. hash를 구하려면 모든 file을 읽기 때문이다.

## canon 연산

- `flowmark canon <corpus> [file…]`는 지정한 file을(file을 지정하지 않으면 corpus의 모든 문서를) 한 transaction에서 canonical 형식으로 다시 쓴다. 대상은 `format.*` 진단이 있는 file의 commit 거절에서 면제된다. 이름을 준 file에 `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있으면 명령은 `CONSTRAINT`로 거절하며 file을 밝히고 아무것도 쓰지 않는다. file을 주지 않으면 대상은 그 행 사이에 `parent.cycle`이 없고 `format.*` 진단이 `format.noncanonical`뿐인 corpus 문서이며, 그 밖의 file은 보고하고 다시 쓰지 않는다. `corpus.walk` file은 결코 대상이 아니다. canon은 논리 값을 바꾸지 않으므로 key, 참조, 제약 검사가 그것을 거절하지 않는다. 링크 유도에 사용할 수 없는 table이 필요한 대상(`files` row로의 링크는 그 key에서만 유도되어 table이 필요 없고, block이나 document row로의 링크는 그 row의 table이 필요하다)은 이름을 준 canon을 `CONSTRAINT`로 거절하며 두 file을 밝히고, file을 주지 않으면 보고만 하고 다시 쓰지 않는다. validator 진단이 있는 값은 쓴 텍스트를 유지한다. 문서 바이트를 정규화하는 유일한 연산이며, 논리적 쓰기는 결코 정규화하지 않는다. 근거: migration은 별도의 명시적 transaction이므로, logical 쓰기는 사용자가 쓰지 않은 바이트를 바꾸지 않는다.

## Server

- `flowmark serve <corpus> --port <port>`는 orm client가 접속하는 상주 face다. `--port`는 필수다.
  loopback TCP에만 bind한다.
  처음 시작할 때 무작위 token을 `.flowmark/server.json`에 쓰며, token은 재시작해도 유지되고
  `flowmark serve --new-token`으로만 바뀐다.
- DSN은 `markdown://127.0.0.1:<port>/<corpus>?token=<token>`이고 `<corpus>`는 corpus root의 directory
  이름이다. corpus나 token 불일치는 `CONFIG`다. 원격 접속과 다른 인증은 v1 밖이다.
- 연결은 session이다: 요청과 control message의 JSON lines이며 각각 순서대로 답한다(요청과 답의 형태는
  [capability table](60-dialect.md)에 있다).

## Lock

- **portable lock 하나.** 모든 operation — 읽기든 쓰기든, 어느 face에서든 — 은 같은 lock file,
  corpus root 아래 `.flowmark/lock`을 잡는다. 경로는 상수이며 `.flowmark/`가 없으면 만든다. kit
  holder-lock 선례를 따른다: holder 식별자와 stale 감지를 갖는 lock file이다. OS file lock은 쓰지
  않으며, 쓰는 platform primitive는 이 절에 이름을 밝힌다. 근거: OS lock(flock, fcntl, LockFileEx)은
  portable하게 취소할 수 있는 대기가 없고, 일부 network file system에서는 local로만 주어지며, holder를
  밝히지 않는다.
- lock은 operation 하나 동안, 또는 server transaction 하나의 `begin`부터 `commit`이나 `rollback`까지
  잡는다. 연결이 닫히거나 transaction이 server flag `tx_idle_ms`보다 오래 쉬면 transaction을
  rollback하고 lock을 푼다.

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
  거절)을 받고 server는 flag `lock_wait_ms`(기본 5000)를 받는다. 연결이 닫히면 대기가 끝난다. 근거:
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
   file을 다시 써야 하는 요청은 곧바로 실패한다. 다른 행의 validator 진단(`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.transition`, `state.required`, `state.limit`, `history.gap`, `history.chain`, 여러 `files` file에 걸친 `parent.cycle`)은 commit을 실패시키지 않되, `type` 진단은 commit의 제약 검사가 그 값을 평가해야 할 때 commit을 실패시키고, 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.ko.md#진단)); `format.*` 진단은 다시 쓰기 거절과 사용할 수 없는 table 상태([canonical format](10-canonical-format.ko.md#진단))가 정하는 경우에만 commit을 실패시킨다. file을 다시 써야 하는 commit — 행 자신의 file, 상세 섹션이 생기거나 사라지는 행으로의 link를 가진 file, `order`의 형제 — 은 그 file이 `format.*` 진단을 가지면 거절한다: `CONSTRAINT`이며 file과 진단을 밝히고, `format.noncanonical`이면 `flowmark canon`을 밝힌다. 세 commit은 면제된다: 대상에 대한 `flowmark canon`([operations](#canon-연산)), 추가된 column이 일으킨 진단에 대한 add-column migration([migration](80-migration.ko.md)) — 그 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다 —, `files` 행의 file 전체를 지우는 delete는 rewrite 거절에서만 면제되며, restrict 검사는 그 행을 참조할 수 있는 모든 table의 행을 여전히 필요로 하며(자신의 table을 `parent`로 하는 것, 자식 block과 이력 행, 그 행으로의 foreign key를 가진 모든 table), 그 중 하나라도 사용할 수 없으면 그 delete는 `CONSTRAINT`다.
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
요청의 `now`, 아니면 microsecond로 자른 엔진의 UTC 시계다.

## 비용 모델

- command별 p95 지연 목표를 여기서 선언한다([open questions](110-open-questions.md)의 corpus 규모에서
  수백 millisecond).
- 지금은 모든 corpus 문서를 다시 읽는 operation의 읽기 비용은 `T12.5-2`가 맡는다([corpus 열기](#corpus-열기)).
  commit은 transaction이 결과를 바꿀 수 있는
  모든 제약을 검사한다: 대상 행의 key, 참조, check, 순서, limit, 전이, `history.gap`과, 그 행들이 참조하거나
  참조되는 행들이며 — lock 아래 다시 읽은 corpus에서 평가한다. 전체 `flowmark validate`는 요청할 때와
  catalog가 바뀔 때 실행한다.
- 오래 사는 face(sidecar, server)는 corpus를 지켜보다 host에 변경 event를 push할 수 있다. 쓰기는 여전히
  transaction protocol로만 간다.

## 실패 어휘

| face 이름 | orm code | 뜻 |
| --- | --- | --- |
| `LOCK_TIMEOUT` | `CANCELED` | lock 대기가 끝났다 — deadline에, 또는 holder가 끝났으면 곧바로. holder와 그 상태를 밝힌다 — SQLite의 `busy_timeout`이 그렇듯 `CANCELED`다 |
| `CORPUS_CONFLICT` | `WRITE_CONFLICT` | 읽은 뒤 대상이 바뀌었거나, 읽은 뒤의 변경이 transaction의 쓰기를 다른 쓰기로 만든다. 각 file과 줄을 밝힌다 |
| `DUPLICATE_KEY` | `DUPLICATE_KEY` | primary나 unique key 값이 이미 있다. 두 행(table과 key)을 밝힌다 |
| `FOREIGN_KEY` | `FOREIGN_KEY` | 참조가 어떤 행도 가리키지 않거나, 참조된 행을 참조되는 동안 지우려 했다 |
| `CONSTRAINT` | `CONSTRAINT` | catalog의 다른 규칙이 쓰기를 거절했다 — check, 제목, 전이, 필수 column, limit, 순서, 한 줄 자리. 규칙, 그리고 행과 column을 밝힌다([canonical format](10-canonical-format.ko.md#진단)); 비정규이거나 잘못된 문서를 다시 써야 하는 쓰기, 또는 사용할 수 없는 table의 행이 필요한 요청(읽기나 쓰기)은 file과 진단을 밝히며 `CONSTRAINT`다([canonical format](10-canonical-format.ko.md#진단)) |
| `READ_ONLY` | `READ_ONLY` | 읽기 전용 transaction 안의 쓰기 |
| `CONFIG` | `CONFIG` | session 상태가 허용하지 않는 control message([transaction](#transaction)) |
| 복구 거절 | `DRIVER` | 대상, 기록된 두 hash, 그 둘을 담은 stage file을 밝힌다. 해결될 때까지 열기와 lock을 잡는 모든 operation을 거절한다 — SQLite의 `SQLITE_CORRUPT`가 그렇듯 다른 code가 밝히지 않는 database 오류의 orm code인 `DRIVER`다. journal 아래 file system의 실패도 `DRIVER`다 |

모든 실패 메시지는 원인과 고치는 방법을 밝힌다.
