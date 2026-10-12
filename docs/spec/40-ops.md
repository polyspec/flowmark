<!-- doc-id: spec-ops -->
# Operations

[한국어](40-ops.ko.md)

This specification pins the operational semantics: opening a corpus, the server and its
sessions, the lock, the transaction and commit protocol, the clock and the failure vocabulary.
The order of the commit protocol is normative.

## Opening a corpus

- Opening runs **recovery first**, under the lock. It removes the pending lock records of ended
  holders ([the lock](#the-lock)) and leaves every `*.clearing` claim alone. `.flowmark/tx/` holds
  at most one stage, named by 32 lowercase hex digits of 128 random bits from the OS random source;
  an entry of another name or form, or a second stage, refuses opening and is named. The stage is
  collected when it holds a commit record, completed when it holds a prepare record, and discarded
  otherwise. Recovery is idempotent: a replay interrupted at any step is completed by the next.
  Recovery runs again at every acquisition of the lock, before the operation reads the corpus: a stage
  found under the lock is a commit whose holder ended, as SQLite rolls a hot journal back before it
  reads. Grounding: a server opened before another process crashed in a commit would otherwise read a
  half-applied corpus once `flowmark unlock` has removed the stale lock.
- A target whose current hash matches neither recorded hash — a change landed while the commit was
  interrupted — is refused and reported, never silently overwritten. The refusal names the target,
  both recorded hashes and the stage files that hold them, and leaves the stage. It is resolved
  when the target holds one of its recorded states (an absent state by removing the file): the
  person copies the edit aside, copies one stage file over the target and opens the corpus again,
  which completes the commit; the edit is then made again as a new change. Until then every open
  refuses the same way. A stage removed by hand leaves the commit half-applied, so the refusal
  never names that as a resolution.
- Opening loads the catalog, walks the corpus, parses the documents and derives the indexes: the
  first full read, which reads and parses every document once. An opened corpus is kept in memory,
  current by the operating system's file notifications, and each operation reads under the lock
  only what changed since the operation before it. The period folders of a sharded table
  ([time shards](20-document-model.md#time-shards)) are folders like any other: each is watched when
  it is read, and a period folder that appears under the table folder changes the table folder's
  entries, so the next operation reads it:
  - A folder or a document is watched before it is read, so no change after its read is missed. On
    macOS kqueue `EVFILT_VNODE` with `EV_CLEAR` on each folder (`NOTE_WRITE | NOTE_DELETE |
    NOTE_RENAME | NOTE_REVOKE`) and each document (those and `NOTE_EXTEND | NOTE_ATTRIB`), each
    opened with `O_EVTONLY`, one descriptor apiece; a folder stays watched until it goes away, so its
    rename always drops what lies under it. On Linux inotify, one instance per opened corpus, on each
    folder (`IN_CREATE | IN_DELETE | IN_MOVED_FROM | IN_MOVED_TO | IN_MODIFY | IN_CLOSE_WRITE |
    IN_ATTRIB` and the folder's own events) and each document (`IN_MODIFY | IN_CLOSE_WRITE |
    IN_ATTRIB` and its own events), so a write through another hard link of a document is seen; two
    paths of one inode share its watch. On Windows one overlapped `ReadDirectoryChangesW` over the
    root and its subtree (`FILE_NOTIFY_CHANGE_FILE_NAME | FILE_NOTIFY_CHANGE_DIR_NAME |
    FILE_NOTIFY_CHANGE_LAST_WRITE | FILE_NOTIFY_CHANGE_SIZE`); a short (8.3) name it reports is
    resolved to its long name, and one that cannot be resolved is taken as an overflow. It names the
    path written, so a write through a hard link outside the corpus is not seen there until the
    document is a commit target or the corpus is opened again; a corpus document with such a link is
    not supported on Windows. These are the primitives of the lock wait
    ([the lock](#acquiring-and-waiting)).
  - Each operation, under the lock and before it reads, takes every notification queued since the
    operation before it, without waiting, and drops what each names: the bytes of a changed file, the
    entries of a changed folder, and, for a name that appeared, disappeared or was renamed, also what
    lies under it and the entries of its folder. The read then reads those paths from the disk again.
    Nothing runs between operations and no timer exists: the kernel keeps the notifications until the
    next operation takes them. Each drop is published as `corpus.changed`, and each folder or file read
    from the disk as `corpus.read`.
  - On macOS and Linux the kernel queues a notification in the system call that makes the change, so
    an operation sees every change completed before it took the lock. `ReadDirectoryChangesW` reports
    a write when the file system records the write time, which may be later.
  - A notification that reports lost events — `IN_Q_OVERFLOW`; zero bytes, an unreadable buffer or
    `ERROR_NOTIFY_ENUM_DIR` — drops everything, and the next read reads the whole corpus again,
    published as `corpus.overflow`: the notification contract requires this path. kqueue keeps one
    state per registration and loses no event, so it has no overflow. A failure while taking the
    notifications loses those it took, so it drops everything too and fails the operation as
    `DRIVER`, naming the cause.
  - A commit reads its targets from the disk under the lock whatever the notifications say, with the
    entries of every folder above them, and drops them again once it has written, so step 1 of the
    [commit protocol](#the-commit-protocol) never depends on the latency of a notification.
  - A document is parsed again only when its bytes changed or when a link target its parse derived
    from another file changed — a row whose detail section appeared or disappeared, a key that became
    shared, a dangling link whose row appeared. The rows of the other documents are kept between
    operations and never changed; a write changes copies of them.
  - A watch that cannot be armed fails the operation as `DRIVER`, naming the cause: on macOS the
    limit of open files (`ulimit -n`, `kern.maxfilesperproc`), on Linux
    `fs.inotify.max_user_watches`.

  Grounding: an operation that read the whole corpus spent 86% of its time opening and reading files
  (2.7–4.4 s for a read of 10k notes); a cache keyed by file content hash does not avoid that, since
  hashing reads every file. FSEvents is not used on macOS: it needs cgo and a run loop or a dispatch
  queue, while kqueue is already the primitive of the lock wait.

## The canon operation

- `flowmark canon <corpus> [file…]` rewrites the named files, or every corpus document when none is named,
  to canonical form in one transaction, and answers the files it rewrote; a canon that changes no byte writes nothing. A file is named by its corpus path: relative to the corpus root, with `/` on every platform, as a commit refusal names it; a file named twice is one target. Its targets are exempt from the commit refusal of a file with a `format.*` diagnostic. A named file with `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows refuses the command as `CONSTRAINT`, naming the file and the diagnostic, and nothing is written; a named `corpus.walk` entry refuses it the same way, and a named path that is neither a corpus document nor a `corpus.walk` entry refuses it as `CONFIG`. With no file named, its targets are the corpus documents without a `parent.cycle` among their rows whose only `format.*` diagnostics are `format.noncanonical`, and it reports every other corpus document and every `corpus.walk` entry, each with its diagnostic, and rewrites none of them. A `corpus.walk` file is never a target. Canon changes no logical value, so the key, reference and constraint checks never refuse it. Its link derivation can: a link to a `files` row derives from its key alone and needs no table, and a link to a block or document row needs that row's table. A target that holds a link to a row of an unavailable block or document table refuses a named-file canon as `CONSTRAINT`, naming the target and the file whose diagnostic makes the table unavailable; with no file named, it is reported with both files and not rewritten. A value with a validator diagnostic keeps its written text. It is the one operation that normalizes document bytes; a logical write never does.
  Grounding: a migration is a separate, explicit transaction, so a logical write never changes bytes
  the user did not write. A `corpus.walk` entry stands at a table's place with a diagnostic that a
  person repairs, as a malformed document does; any other path names nothing canon could rewrite,
  which is a mistake in the command. A link whose table is unavailable may name a row the engine
  cannot read, or a key that two rows share, so its derived target is unknown, and canon never
  writes a target it cannot derive.
- The command line is `flowmark canon [--lock-wait-ms <n>] <corpus> [file…]`: `--lock-wait-ms` is the
  lock wait of the CLI face ([acquiring and waiting](#acquiring-and-waiting)), 5000 by default and 0
  to refuse at once; `<corpus>` is the corpus root. It prints `[canon] rewrote <file>` for each file
  rewritten, `[canon] left <file>: <diagnostic>` for each file reported, and a count of both. It exits
  0 when canon ran, files reported included; 1 when it was refused or failed, with the failure on
  standard error; and 2 for a wrong command line — the exit codes of every command of the CLI
  ([the faces](#the-faces)).

## The server

- `flowmark serve <corpus> --port <port> [--new-token] [--lock-wait-ms <n>] [--tx-idle-ms <n>]` is
  the resident face that orm clients reach. `--port` is required; it binds `127.0.0.1:<port>`, loopback
  TCP only, and `--port 0` lets the operating system choose the port. Once listening it prints
  `[serve] listening on 127.0.0.1:<port> for the corpus <corpus>` on standard output, and each lock and
  journal event, and each failure to end a session, on standard error. A catalog with diagnostics
  refuses the start, naming them.
- On its first start it writes a random token to `.flowmark/server.json`: one line of JSON,
  `{"token"}`, and a line feed; the token is 32 lowercase hex digits of 128 random bits from the OS
  random source, and the file is created readable and writable by its owner only. The token is kept
  across restarts and replaced only by `flowmark serve <corpus> --new-token`. The file is written
  under a partial name and linked into place, so two servers starting at once keep the one token the
  first link placed; a file that does not hold that line, or a symlink there, refuses the start,
  naming `--new-token`.
- The DSN is `markdown://127.0.0.1:<port>/<corpus>?token=<token>`, where `<corpus>` is the corpus
  root's directory name. A corpus or token mismatch is `CONFIG`. Remote access and other
  authentication are outside v1.
- A connection is a session: JSON lines of requests and control messages, each answered in order by
  one line ([capability table](60-dialect.md) holds the request and answer shapes). The lock holder's
  session is `serve-<n>`, `n` counting the server's connections from 1.
- The first message of a connection is `{"op": "connect", "corpus", "token"}`, carrying the DSN's
  corpus and token, answered `{"ok": true}`. A corpus or token that differs, the token compared in
  constant time, or any other message first, is answered `CONFIG` and the server closes the
  connection; a second `connect` is `CONFIG`. Grounding: the DSN's corpus and token must reach the
  server before any request reads the corpus, as a PostgreSQL startup message precedes every query and
  a failed authentication ends the connection.
- A message is one JSON object on one line; a message with `op` is a control message, any other a
  request message. A failure is answered `{"error": {"code", "message"}}` with the orm code — the
  server is the orm clients' face, so `LOCK_TIMEOUT` is answered `CANCELED` and `CORPUS_CONFLICT`
  `WRITE_CONFLICT`. A line that is not a JSON object, an `op` that names no control message, and a
  field that its control message does not take are `IR_INVALID`, orm's code of a structurally wrong
  message. `begin` takes `read_only`, a boolean, false when absent; `savepoint`, `release` and
  `rollback_to` take `name`.
- `--tx-idle-ms` (default 5000, a positive integer) bounds the time an open transaction waits for its
  next message, from the answer to the previous one. When it passes, the transaction is rolled back,
  the lock released, `{"error": {"code": "CONNECTION_LOST", "message"}}` written on the connection, which
  the client reads as the answer to its next message, and the connection closed. Grounding: PostgreSQL's
  `idle_in_transaction_session_timeout` ends the session too (SQLSTATE 25P03), and orm's
  `CONNECTION_LOST` names an idle limit; a connection kept open after a silent rollback would run the
  client's next write outside the transaction it believes open. The default is the lock wait's, so an
  idle transaction holds the lock no longer than another operation waits for it by default. The idle
  bound is a timer because idleness is the absence of an event; besides a lock wait's deadline it is the session's only
  timer.
- A closed connection rolls its open transaction back, releases the lock and ends its request's lock
  wait; the server learns it from the connection's read, which runs while the request does, and the
  messages it has received but not begun to answer are not run — the end of the read is the client's
  close, which a server cannot tell from a half-close, and the session ends with it. On
  `SIGINT` or `SIGTERM` the server stops listening and closes every connection the same way.

## The faces

- The CLI, the MCP server and the soksak sidecar are thin faces of the write path. Each operation of
  a face is one operation of a session — a request of the IR-executor protocol, a history repair, a
  rewrite or an install — run as a request outside a transaction ([transactions](#transactions)),
  under the lock with the face's `--lock-wait-ms`; its request clock is the engine clock
  ([the clock](#the-clock)). No face reads or writes a corpus file itself. Grounding: one write path
  holds every rule, so no face can apply a rule otherwise or skip one.
- An operation is named by `operation` and works over any table of the catalog: no face names a
  table, a column or a storage. A value — `key`, each member of `values`, `position`, `parent` and
  every value answered — is in the runtime value model JSON of the protocol
  ([capability table](60-dialect.md#the-wire)). `key` is the value of the table's primary key.

| Operation | Members | Answer |
| --- | --- | --- |
| `schema` | — | `{"manifest_hash", "tables"}`: the catalog — each table with its storage, key, `title`, `body`, `order`, hierarchy column, `markdown` columns, columns, foreign keys, unique keys and state machine |
| `rows` | `table` | `{"rows"}`: every row of the table in storage order, as an `all` request without `columns` answers it |
| `row.get` | `table`, `key` | `{"row"}`: the row whose key is `key` |
| `row.insert` | `table`, `values` | `{"affected", "keys", "rows"}` |
| `row.update` | `table`, `key`, `values` | `{"affected", "keys", "rows"}` |
| `row.move` | `table`, `key`, `position`, optional `parent` | `{"affected", "keys", "rows"}` |
| `row.delete` | `table`, `key` | `{"affected", "keys", "rows"}` |
| `transition` | `table`, `key`, `state`, optional `values` | `{"affected", "keys", "rows"}` |
| `views` | optional `key` | `{"rows"}` of the view table, or `{"view", "rows"}` of the view `key` |

- `row.insert` is an `insert` assigning `values`; `row.update` an `update` of the row whose key is
  `key` assigning `values`; `row.delete` a `delete` of that row. `row.move` is an `update` of that row
  assigning `position` to the table's `order` column and, when `parent` is given, `parent` to its
  hierarchy column `parent` ([document model](20-document-model.md#order)); a table without `order`,
  or a `parent` for a table without the hierarchy, is `CONFIG`. `transition` is an `update` of that
  row assigning `state` to the column of the table's state machine together with `values`, which
  carry the columns its transition requires ([constraints](50-constraints.md#the-state-machine)); a
  table without a state machine is `CONFIG`. Each is checked and committed as any request of its
  kind; no face adds or drops a rule.
- A write answers `{affected, keys}` of its request and, as `rows`, the table's rows as `rows`
  answers them right after the commit. Grounding: a view redraws from the rows an operation returns,
  never from its own guess ([application schema](100-checklist.md#views)).
- An operation on the row named by `key` that matches no row — `row.get`, or a write that affects
  none — is `NO_ROWS`, naming the table and the key, orm's code of a one-row read that matched none.
  Grounding: a write to a named row that changes nothing would be a silent drop.
- An unknown `operation`, a missing member, a member the operation does not take, `values` that is
  not an object, and `values` of `transition` that assign the state column, which `state` gives, are
  `CONFIG`, naming it; a table or column the catalog does not declare is the
  request's `ENTITY_UNKNOWN` or `COLUMN_UNKNOWN`.
- `views` reads the view table of the catalog ([application schema](100-checklist.md#views)), never
  a table the face names: without `key` it answers the view table's rows as `rows` does; with `key`
  it answers `{"view", "rows"}`, the view row and the rows of its `entity` read by an `all` request
  whose where and params are the stored filter's and whose order is the stored sort — `group_by` is
  the view's to apply. Both reads run in one read-only transaction, so the view and its rows come
  from one corpus. The view is first judged as the validator judges it
  ([constraints](50-constraints.md#views)), and a view with a fault is `CONSTRAINT`, naming each
  fault; a read of the view table never is. A catalog without a view table refuses `views` as
  `CONFIG`. Grounding: a view is a stored request, so running it is that request, and a view the
  validator reports runs no request the client could not form itself.
- A failure is answered `{"code", "error"}`: the face name of the [failure vocabulary](#failure-vocabulary),
  which is the orm code for every code the vocabulary does not list, and the message.
- **The CLI.** Each operation is a command: `flowmark schema <corpus>`, `rows <corpus> <table>`,
  `row.get <corpus> <table> <key>`, `row.insert <corpus> <table> [assignment…]`,
  `row.update <corpus> <table> <key> [assignment…]`, `row.move <corpus> <table> <key> <position> [<parent>]`,
  `row.delete <corpus> <table> <key>`, `transition <corpus> <table> <key> <state> [assignment…]` and
  `views <corpus> [<key>]`.
  An assignment is `<column>=<text>` or `<column>:=<json>`; a positional value is `<text>` or
  `:=<json>`. A text is a value's readable text, sent as the wire value of its column's type — a
  JSON number for an integer or a float, a JSON bool for `bool`, a JSON string otherwise — and
  `:=` takes the wire value itself, `null` included. Every command takes `--lock-wait-ms <n>` (5000
  by default, 0 refuses at once), and a flag may stand before or after the positional arguments.
  The answer is written to standard output as one line of JSON. The other commands are
  `flowmark canon` ([the canon operation](#the-canon-operation)), `flowmark install <corpus>`
  ([schema](30-schema.md#install)), `flowmark history repair <corpus> <table> <key> [--from <state>]`
  ([constraints](50-constraints.md#a-state-changed-outside-the-engine)) and `flowmark unlock <corpus>
  [--holder <holder>]` ([a stale lock](#a-stale-lock)). Every command exits 0 when it ran, 1 when it
  was refused or failed, with `flowmark <command>: <code>: <message>` on standard error, and 2 for a
  wrong command line. A refused unlock is `CONFIG`, and a failure of the file system under it
  `DRIVER`.
- **`flowmark install`** is its own transaction under the lock, `CONFIG` inside an open one as canon
  is. It writes, in one journal commit, the file `<table>.flowmark.md` of each `document` table that
  has none — the empty table, `# <table>` and one LF — and `<corpus>/.gitattributes` with the line
  `*.flowmark.md text eol=lf` when no line of it is that line, appended after its last line (and a LF
  when its last line has none) or as a new file; then, under the same lock, the folder of each
  `files` table that has none. A path at a table's place that holds another kind of entry, or a
  `.gitattributes` that is no regular file, refuses the install as `CONFIG`, naming it, and nothing
  is written. It prints `[install] created <path>` for each file or folder created,
  `[install] added the line *.flowmark.md text eol=lf to .gitattributes` when it added it, and a
  count; an install that finds everything in place writes nothing. Grounding: install is idempotent,
  and a folder holds no bytes the journal could stage, so its creation follows the commit as a
  separate idempotent step.
- **The MCP server.** `flowmark mcp [--lock-wait-ms <n>] <corpus>` speaks JSON-RPC 2.0 over standard
  input and output, one message per line, with `initialize`, `tools/list` and `tools/call`. Each
  operation is one tool, named as the operation with `_` for `.`, whose arguments are its members;
  a tool answers one text content holding the answer's JSON, or the failure's JSON with `isError`
  true. An unknown method is the JSON-RPC error -32601, a line that is not JSON -32700 with a null
  id, and a notification is not answered.
- **The sidecar.** `flowmark sidecar [--lock-wait-ms <n>]` speaks protocol v1 of the soksak core
  specification (`docs/spec/sidecars.md`): a request `{"surface", "root", "body"}` runs the operation
  of its body over the corpus whose root is `root`, and is answered `{"surface", "body"}` with the
  request's `cids` when it has them; the body's `id`, when given, is copied into the answer body. A
  failure body is `{"id", "code", "error"}`, the member `error` stating the failed request as the
  protocol reads it. A `closed` message is answered `{"surface", "closed": true}`: a surface holds no
  resource. A line that is no request names no surface to answer, so it is written to standard
  error, which the host logs. The sidecar pushes no change event: a watch of the corpus driven by
  the operating system's notification belongs to the long-lived corpus of `T12.5-2`, and a polling
  watch is not a substitute.

## The lock

- **One portable lock.** Every operation — read or write, from any face — takes the same lock
  file, `.flowmark/lock` under the corpus root; the path is a constant, and `.flowmark/` is
  created when absent. It follows the kit holder-lock precedent: a lock file with holder identity
  and stale detection. No OS file lock is used; the platform primitives used are named in this
  section. Grounding: an OS lock (flock, fcntl, LockFileEx) has no portable cancellable wait, is
  granted only locally on some network file systems, and names no holder.
- The lock is held for one operation, or for one server transaction from `begin` to `commit` or
  `rollback`. A closed connection, or a transaction idle longer than the server flag
  `--tx-idle-ms`, rolls the transaction back and releases the lock ([the server](#the-server)).

### The holder record

- The lock file holds one line of JSON, `{"pid", "start", "host", "hostname", "session",
  "acquisition"}`, and a line feed; the holder is named `<pid>@<host>/<session>`. `acquisition` is
  the token of one acquisition: 32 lowercase hex digits of 128 random bits from the OS random source,
  which also name its pending record. A file that holds anything else is never taken for a free lock.
  Grounding: a session may acquire again in its process, so only a token per acquisition keeps two
  acquirers from writing byte-equal records, which the lost-reply rule below would take for one.
- `start` is the start of the process: on Linux the boot of `/proc/sys/kernel/random/boot_id` with
  field 22 of `/proc/<pid>/stat`; on macOS `kp_proc.p_starttime` of sysctl `kern.proc.pid.<pid>`;
  on Windows the `GetProcessTimes` creation time.
- `host` is the machine identifier: `/etc/machine-id` on Linux (machine-id(5)) with the inode of
  `/proc/self/ns/pid`, `gethostuuid(2)` on macOS, and
  `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` on Windows. A missing one refuses the lock,
  naming the cause. `hostname` is the host name, recorded for the messages only.
- A holder of this host has ended when no process has its ID, when that process's start differs
  (a reused ID), or when it is a zombie; a process that exists but whose start cannot be read
  counts as running. Whether a process ID exists: on Linux and macOS `kill(pid, 0)`, where `ESRCH`
  means none and success or `EPERM` means one exists; on Linux, when `/proc/<pid>/stat` cannot be
  read (hidepid), `kill(pid, 0)` is asked again, and `ESRCH` then means ended. On Windows
  `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION | SYNCHRONIZE)`, where `ERROR_INVALID_PARAMETER`
  means none and `ERROR_ACCESS_DENIED` means one exists; its liveness comes from
  `WaitForSingleObject(h, 0)`, never from the exit code, since an exit code of 259 equals
  `STILL_ACTIVE`.
- A holder of another host is unknown and never judged ended. A corpus is used from one host at a
  time; a release on another host reaches a waiter only where the file system reports remote
  changes, so such a wait may end at its deadline.

### Acquiring and waiting

- The record is written whole to `.flowmark/lock.<acquisition>.pending`,
  synced, and hard-linked to `.flowmark/lock`; the link fails when the lock exists, so the lock
  appears atomically with its whole record. A waiter writes its pending record once. A pending
  record takes no part in exclusion.
- A try whose link fails with `EEXIST` reads the lock. A record byte-equal to the acquirer's own
  (its acquisition token is unique) means the link took effect although its reply was lost (NFS), and the
  lock is held. A lock that disappears before it is read is tried again at once.
- An operation that finds the lock held by a running holder, or by one on another host, waits for
  its release and fails with `LOCK_TIMEOUT`, naming the holder and its state, when its deadline
  passes first. A holder that has ended refuses at once. The deadline is the wait's one timer: the
  CLI, MCP and sidecar take `--lock-wait-ms <n>` (default 5000; 0 refuses at once), and the server
  takes the same flag; a closed connection ends the wait. Grounding: an
  immediate refusal would move the retry loop into every caller.
- The wait is driven by the OS change notification on the lock: on Linux inotify on `.flowmark/`
  (`IN_DELETE | IN_MOVED_FROM | IN_DELETE_SELF | IN_MOVE_SELF | IN_UNMOUNT`), one instance shared
  by all waits of a process on one corpus, as a server's sessions are; on macOS kqueue
  `EVFILT_VNODE` (`NOTE_DELETE | NOTE_RENAME | NOTE_REVOKE`, `EV_CLEAR`) on the lock file opened
  with `O_EVTONLY`; on Windows an overlapped `ReadDirectoryChangesW` on `.flowmark/`, opened with
  `FILE_LIST_DIRECTORY`, `FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OVERLAPPED` and
  `FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE`.
- Each removal or rename of the lock file, each overflow (`IN_Q_OVERFLOW`; zero bytes returned;
  `ERROR_NOTIFY_ENUM_DIR`) and each event of the watched folder itself (removal, rename, unmount,
  `IN_IGNORED`) starts exactly one more acquisition attempt. No other event and no timer starts
  one; the removal of another name, such as a waiter's pending record, starts none.
- The watch is armed after the failed try, and a release between the two is never lost. On Linux
  and Windows the lock is tried once more after arming. On macOS, after a try fails, the lock is
  opened with `O_EVTONLY` (a lock that is absent is tried again at once) and registered, and the
  `fstat` of the descriptor is compared with the `lstat` of `.flowmark/lock`: a link count of 0,
  or another device or inode, starts one more attempt. Each later vnode event is judged by the
  same comparison: when the lock path still names the watched inode with a link count above 0,
  the event came from another name of that file (an acquirer's pending record) and starts no
  attempt; a change of one file starts one attempt, however many events it raises.
- A watch that cannot be armed refuses the wait at once, naming the cause: `EMFILE` names
  `fs.inotify.max_user_instances`, `ENOSPC` names `fs.inotify.max_user_watches`, and
  `ERROR_INVALID_FUNCTION` names the unsupported file system.
- The holder's exit is an event. When the holder is on this host and running, the waiter also
  watches its exit: on Linux a pidfd of `pidfd_open` (Linux 5.3 or later is required), waited on
  by the runtime's poller beside the inotify descriptor; on macOS `EVFILT_PROC NOTE_EXIT` in the
  same kqueue; on Windows the process handle (`SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION`),
  waited with the change event by `WaitForMultipleObjects` — a process whose handle this user may
  not open is not watched, and its release is learned from the lock alone. After arming, the
  holder is judged again, so a reused process ID is never watched. An exit starts no attempt: it
  judges the holder again, and an ended holder whose record the lock still holds refuses at once
  as `LOCK_TIMEOUT`, naming it as ended.
- On Windows every open of the lock file, a pending record or a stage file is `CreateFileW` with
  `FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE`, so a reader never blocks the holder's
  delete or an unlock.
- Acquiring, releasing, clearing and waiting are published as the events `lock.acquired`,
  `lock.released`, `lock.cleared` and `lock.waiting` (once per holder waited for, after the watch
  is armed).

### A stale lock

- A stale lock is never removed by an operation. `flowmark unlock <corpus>` removes it:
  1. Read the record R. Refuse a running holder, and a holder on another host without
     `--holder <pid>@<host>/<session>` naming it.
  2. Link `.flowmark/lock` to `.flowmark/lock.<first 32 hex digits of the SHA-256 of R>.clearing`.
     `EEXIST` refuses, naming that file as another unlock of the same holder, running or
     interrupted.
  3. Read the claim. A record other than R means a new holder took the lock: remove the claim and
     refuse.
  4. Otherwise remove `.flowmark/lock`, then the claim, and publish `lock.cleared` with the record.
- Grounding: only the holder and the one unlock that holds the claim ever remove the lock, so
  between the claim and the removal the lock still names R, and a running holder's lock is never
  moved.
- An interrupted claim is named in the refusal of the next unlock; the person removes it after
  checking that no unlock runs. A `LOCK_TIMEOUT` of an ended holder names `flowmark unlock
  <corpus>`, and one of a holder on another host names `--holder`.

## Transactions

- Requests inside a transaction read its post-image, held in the server's memory; a savepoint is a
  nested level of it, and `rollback_to` discards the levels above it. A journal stage in
  `.flowmark/tx/` exists only from step 3 to step 8 of one commit.
- Isolation. Engine sessions are serialized by the lock: a transaction holds it from `begin` to its
  end, so no engine write interleaves with it. Against an edit outside the engine a transaction is
  snapshot-like: the commit checks its targets (step 1) and runs its writes again on the re-read corpus
  (step 2), but it does not validate what its reads returned, so a decision made on a read row that an
  outside edit then changes in another file can commit — write skew. MySQL's `REPEATABLE READ` allows
  the same (probed on MySQL 8.4.11: two transactions each read two rows in a state, each moved a
  different one out of it, and both committed, leaving none), and so do PostgreSQL's `READ COMMITTED`
  and `REPEATABLE READ` (PostgreSQL documentation, Transaction Isolation; only `SERIALIZABLE` detects
  it). Grounding: an outside edit takes no lock, so no lock-based isolation can include it; validating
  every read would need the read set of every request, which no orm client sends.
- A request outside a transaction — a read, a write, a history repair — is one transaction under the
  lock: it takes the lock, reads the corpus, and a write commits before the lock is released.
- A request that fails inside a transaction changes nothing, and the transaction stays open at its
  state before the request. Grounding: SQL statement atomicity — a failed statement has no effect, and
  MySQL and SQLite keep the transaction open; the orm client rolls the transaction or its savepoint back
  when its callback fails.
- `savepoint <name>` sets a level at the transaction's present state; a savepoint of the same name is
  replaced, as the SQL standard destroys it. `release <name>` removes that savepoint and every one set
  after it, keeping their writes. `rollback_to <name>` discards the writes since that savepoint and every
  savepoint set after it, and keeps the savepoint. Grounding: the SQL standard's savepoint statements;
  PostgreSQL and SQLite keep a shadowed savepoint instead, which a client that names savepoints by depth,
  as orm does, never meets.
- A read-only transaction takes the lock and refuses every write as `READ_ONLY`, as PostgreSQL refuses a
  write in a read-only transaction (SQLSTATE 25006).
- A control message that its session's state does not allow is `CONFIG`, orm's code of a transaction
  misuse: `begin` while a transaction is open; `commit`, `rollback`, `savepoint`, `release` or
  `rollback_to` without one; a savepoint without a name; `release` or `rollback_to` of a name no
  savepoint of the transaction has. Grounding: SQLite, whose single writer lock this corpus lock follows,
  refuses a nested `BEGIN` and an unknown savepoint.
- A commit that fails before its prepare record writes nothing: it ends the transaction, its partial
  stage is discarded under the same lock, and the lock is released, as a failed SQL `COMMIT` rolls the
  transaction back. From the prepare record on the commit is decided: a step that fails after it is
  followed, under the same lock, by recovery, which completes the commit, and the commit succeeds;
  when that recovery fails too, the commit is `DRIVER` saying that it is prepared, completes at the
  next acquisition of the lock and must not be repeated. A failure to release the lock after a
  written commit says that the commit is written. Grounding: the prepare record is the commit point
  of the journal, as a durable commit record is in a write-ahead log, so a failure after it is never
  answered as a commit that did not happen while recovery would apply it. The lock is released on
  every way out of an operation, a panic included.
- The write path retries a single-operation write — a write, a history repair or a rewrite outside a
  transaction, on every face, a server request outside a transaction included — once when its commit
  meets `CORPUS_CONFLICT`: under the same lock, it reads the corpus again and runs the operation again.
  A server transaction is never retried by the server; the orm client receives `WRITE_CONFLICT` and
  never retries it. Grounding: one place retries for every face, and the retry never releases the lock
  between the read and the commit.
- `flowmark canon` and the add-column migration are each their own transaction: inside an open
  transaction, a read-only one included, they are `CONFIG`, never `READ_ONLY`, since they never run
  in a transaction they did not begin. Each names the format diagnostics of its targets that it is exempt for
  ([commit protocol](#the-commit-protocol), step 2).

## The commit protocol

1. Check that every target is unchanged since it was read; otherwise fail `CORPUS_CONFLICT`
   naming each changed target and its line: the first line at which its present bytes differ from
   the bytes read, and line 1 when the file is absent in one of the two.
2. Run the constraint checks against the corpus re-read under the lock with the post-image
   applied — not against a snapshot taken at `begin`, and not against a predicted touched-file
   set. The checks are the executor's checks of each write: when the re-read corpus differs from the
   corpus the transaction read — a document's bytes, or a table's unavailability — the transaction's
   writes run again, in order, on the re-read corpus. A write they refuse fails the commit with its
   code. A write that answers otherwise than it answered, or a net post-image that differs, fails
   `CORPUS_CONFLICT` naming each changed file and its first changed line: the writes depend on the
   change that landed, and the client already holds the first answers. This is the analogue of SQL's
   serialization failure (SQLSTATE 40001), which orm reports as `WRITE_CONFLICT`, not `DEADLOCK` (orm
   maps 40001 to `DEADLOCK`), since the client never retries it. When the re-read corpus is the one read, the writes were checked on it as
   they ran. The rewrite refusal below is checked as each request runs too, as SQL checks a constraint
   that is not deferred, so the request that must rewrite such a file fails at once. A validator diagnostic on another row (`type`, `key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, `order`, `state.transition`, `state.required`, `state.limit`, `history.gap`, `history.chain`, and the validator's `parent.cycle` across several `files` files) never fails the commit, except that a `type` diagnostic fails it when a constraint check of the commit must evaluate that value ([canonical format](10-canonical-format.md#readable-values)), and two rows with byte-for-byte equal keys make their table unavailable ([canonical format](10-canonical-format.md#diagnostics)); a `format.*` diagnostic fails it only as the rewrite refusal and the unavailable table state ([canonical format](10-canonical-format.md#diagnostics)). A commit that must rewrite a file — the row's own file, a file holding a link to a row whose derived target changes (its detail section appears or disappears, or an insert gives a dangling link its row), an `order` sibling — is refused when that file has a `format.*` diagnostic: `CONSTRAINT`, naming the file and the diagnostic, and naming `flowmark canon` when it is `format.noncanonical`. Three commits are exempt: `flowmark canon` for its targets ([operations](#the-canon-operation)); the add-column migration, for the diagnostics the added column causes, which do not make its table unavailable to that migration ([migration](80-migration.md)); and a delete that removes a `files` row's whole file from the rewrite refusal only — its restrict check still needs the rows of every table that may reference the row (its own table through `parent`, its child blocks and its history rows, and every table with a foreign key to it), so such a delete is `CONSTRAINT` when any of them is unavailable.
3. Write each post-image once into `.flowmark/tx/<id>/`, content-addressed as `<hash>`, and, as
   the source of rename n, a copy `<n>.<hash>` of it; both stay until the stage is collected.
   Before this step, the folder of every target and every folder to create is checked to be on
   the device of `.flowmark/tx/` (`st_dev`; on Windows the volume serial number of
   `GetFileInformationByHandle`); a mismatch refuses the commit, naming the folder and "a mount
   point inside the corpus is unsupported", and nothing is staged.
4. Write the pre-image of every target into the stage, content-addressed as `<hash>`.
5. Write the **prepare record**: every rename with its pre- and post-hashes. An absent file is the
   empty hash `""`: a create has an empty pre-hash, and a delete an empty post-hash.
6. Perform the renames through the runtime's atomic rename, after checking every target: one that
   holds its post-image is done, one that holds its pre-image is renamed, and any other refuses
   the whole stage. A delete renames its target into the stage as `<n>.deleted`. A create whose
   folder is absent first creates that folder, from the corpus root, following no symlink; on
   Windows each missing folder is created in the stage as `<n>.dir.<k>` and moved into place. A
   replay does the same: it skips a folder it finds in place and moves one it finds staged, and
   when it finds a target at its pre-hash and `<n>.<hash>` absent it writes the copy again from
   `<hash>`. An empty table folder left by an interruption is an empty table
   ([document model](20-document-model.md#storage)).
7. Write the commit record.
8. Collect the stage: the prepare record is renamed aside first, so a stage interrupted while it
   is collected holds the commit record alone and is collected again; collecting removes the
   `<n>.deleted` files.

Grounding: a rename gives the target the source's file. Distinct per-rename copies keep two
identical targets as two files and keep the target sharing no file with the stage, and both
recorded states stay in the stage for a recovery refusal. The journal needs no hard link.

Every hash of the journal and of recovery is the SHA-256 of the file's raw bytes, as 64
lowercase hex digits (raw-file-bytes hashing, [design plan](../plans/design-plan.md)). Each step
is published: `journal.prepared` after step 5, `journal.applied` after step 7,
`journal.collected` after step 8, and on opening `journal.replayed` or `journal.discarded`.

Durability across power loss needs each step's files and folder entries on disk before the next
step begins. A file is synced by the runtime's sync (`fsync`; `F_FULLFSYNC` on macOS;
`FlushFileBuffers` on Windows). A created, renamed or removed name is made durable on Linux and
macOS by syncing its folder, as POSIX defines, and on Windows, which has no folder flush, by
renaming with `MoveFileEx` and `MOVEFILE_WRITE_THROUGH`, which returns once the move is on disk;
so every stage file is written to a partial name and renamed into place, and a delete is a
rename. A created folder is made durable on Linux and macOS by syncing its parent, and on Windows
by creating it in the stage and moving it into place with `MOVEFILE_WRITE_THROUGH`. The lock is
never synced as a folder entry: a power loss ends its holder; its record is synced before its
link. On Windows a holder's liveness is read from its process handle, never from its exit code.

## The file system

The corpus root lies on one local file system with hard links and change notifications: APFS or
HFS+ on macOS, ext4, XFS, Btrfs or the like on Linux, NTFS (or ReFS with hard links) on Windows.
FAT32 and exFAT have no hard links, so the lock cannot appear atomically with its whole record;
they are not supported, and the lock's link failure there (`EPERM`, `ENOTSUP`,
`ERROR_INVALID_FUNCTION`, `ERROR_NOT_SUPPORTED`) refuses, naming that cause. A file system that
sends no change notification is not supported. A folder of the corpus that is another file
system's mount point is not supported, and a commit that would write there is refused
([commit protocol](#the-commit-protocol), step 3).

## The clock

Every engine-assigned time — `updated`, `soft_delete`, `default now`, a history row's `at` — is
the request's `now` for an orm request, and otherwise the engine's UTC clock truncated to
microseconds; one request has one clock value. Written into a `datetime(p)` column, it is
truncated to p fraction digits, never rounded; compared with a column, `now` and the relative time
functions keep six digits. Grounding: orm's clock rule — a clock the ORM writes or compares keeps
microseconds; MySQL `CURRENT_TIMESTAMP(p)` and orm's SQLite `now` slot truncate a written clock,
and MySQL and PostgreSQL compare a `datetime(p)` column with `NOW(6)` at six digits.

## Cost model

- A per-command p95 latency target is declared here (hundreds of milliseconds at the corpus
  envelope of [open questions](110-open-questions.md)).
- An operation on an opened corpus reads from the disk only what changed and parses only the
  documents whose parse can differ ([opening a corpus](#opening-a-corpus)); it still walks the rows
  of the corpus in memory, so its cost grows with the rows, not with the bytes. The benchmarks
  `BenchmarkOpen`, `BenchmarkRead` and `BenchmarkWrite` of `writepath` measure it at the envelope,
  10k one-row documents beside a small corpus, reporting the CPU time of each operation and the
  95th percentile of its wall time. On macOS (Apple M3 Pro, `T12.5-2`) a read takes 68 ms of CPU and
  a write — one transaction under the lock, committed through the journal — 216 ms, within the
  budget; their wall-time p95, 273 ms and 1.1 s, was measured on a machine whose load average was
  near 100 on 11 cores, so the wall-time p95 is measured again on an idle machine with Linux and
  Windows in `T12.13`. The first full read when a corpus is opened takes 1.2 s of CPU.
- The first full read is the startup cost of an opened corpus: it reads and parses every document
  and arms the watch of every folder and document, once.
- The corpus kept in memory holds each document's bytes, the parses of both read passes of the
  executor and the document's watch: its budget is 8 KB of heap per row at one row a document, the
  most a row costs, and `TestKeptHeapBudget` holds it (4.5 KB measured on macOS). A document of many
  rows is held whole; what parsing it allocates is the codec's cost below.
- A write still parses and renders each changed target whole, and the executor's post-image copies
  the rows of the corpus, so a write's cost grows with its target files and with the rows; bounding
  the size of a target file is [the scale envelope](#the-scale-envelope), and the cost per byte of a target is the codec's cost.
- **The codec's cost** is linear in the bytes of a file: the parse and the render of a file each take
  at most 0.1 s of CPU per megabyte of it and allocate at most 64 bytes per byte of it, for a document
  file of rows with detail sections. No pass compares each heading with each detail anchor, or each
  row with each row: a heading's slug finds the anchors it takes by lookup, and a detail section
  finds its row by key. `TestCodecAllocationBudget` of `store` holds the allocation budget at 1 and
  4 MB, and `BenchmarkParseDocument` and `BenchmarkRenderDocument` measure both per megabyte. On macOS
  (Apple M3 Pro, `T12.5-4`) the parse of 1 MB takes 48 ms and allocates 50 bytes per byte and its
  render 35 ms and 39, and at 5 MB 43 ms and 33 ms per megabyte with 53 and 41; before `T12.5-4`
  the parse took 2.1 s for 1 MB and 19.5 s for 5 MB, allocating 456 and 667 bytes per byte, growing
  with the square of the rows.
  Grounding: a parse reads the rows once, scans the blocks of each viewer's reading that the
  [collision rule](10-canonical-format.md#detail-headings-and-anchors) needs, reads the inline text of
  the headings and paragraphs in those readings, and renders the rows again to check the canonical
  form; each pass builds a structure of the size of the file, so the allocation is a constant
  multiple of the bytes, and 64 leaves the measured multiples their margin while a cost that grows
  faster than the file passes it by 4 MB. 0.1 s per megabyte keeps a write to a target of a megabyte
  within the hundreds of milliseconds of the latency target; allocation is the budget a test holds,
  since a machine's load changes the time and not the bytes.
- The commit checks every constraint whose
  outcome the transaction can change: the keys, references, checks, order, limits, transitions and
  `history.gap` of the target rows, and the rows they reference or that reference them, evaluated
  on the corpus re-read under the lock. Full `flowmark validate` runs on demand and on catalog
  change.
- A long-lived face (the sidecar, the server) may watch the corpus and push change events to its
  host; writes still go through the transaction protocol only.

## The scale envelope

The engine states the size of a corpus it serves within the latency target above, and says when a corpus passes it.

- **The envelope** is 1 MiB for one document, 20,000 documents for one folder and 20,000 documents for a corpus.
  `catalog.Envelope` holds the three numbers, and the specification and the code state no other.
- **The measurements** are the benchmarks of `writepath` (`BenchmarkFileSizeWrite`, `BenchmarkShardOpen`,
  `BenchmarkShardInsert`, `BenchmarkShardRead`, `BenchmarkSessions`), run on macOS (11 cores) at the date of `T12.5-3`:
  - The update of one row of a document takes 83 ms (p95) at 32 KB, 239 ms at 340 KB and 757 ms at 1.4 MB, 0.7 ms
    of CPU for each KB: a write parses, checks and renders its target whole. 1 MiB is the size at which the update of
    a row is still within hundreds of milliseconds.
  - With the documents of a folder from 5,000 to 20,000, the opening takes 336 ms, 697 ms and 1,245 ms (p95) and the
    insert of a document 99 ms, 129 ms and 249 ms: 0.06 ms for each document. At 20,000 documents the insert is at
    the latency target, which sets the number for a folder and for a corpus alike.
  - A shard by month leaves the opening, the insert and the read within noise. It keeps a folder from growing without
    end; it does not make a corpus of the same documents faster.
  - On macOS the watch holds a descriptor for every folder and document (20,009 at 10,000 documents, one for each
    document and folder): the number of documents is also the number of descriptors a process needs. An opening that
    cannot get them fails as `DRIVER` naming the file, and leaves no lock.
  - Hosting such a corpus on macOS raises the limits an operator controls, because the kernel caps open files twice
    besides the rlimit: per process (`kern.maxfilesperproc`) and system-wide (`kern.maxfiles`). A process that holds
    C open corpora of N documents needs about C·N descriptors, so the three are raised together — the CI workflow of
    `.github/workflows/test.yml` does it before its benchmarks and names the values the runner allowed in its log. The
    `descriptors` column of the benchmark artifacts counts the whole process, which holds the fixture's corpus and the
    freshly opened one, about two per document; one corpus alone holds one per folder and document, as the board
    measurement above shows.
  - The board of the examples is this shape at 10,000 posts, 1,000 in each of ten month folders, served by
    `flowmark serve`: the server is ready in 3.1 s, keeps 140 MB, holds 10,029 descriptors and answers a page of
    ten posts in 72 ms, one post in 44 ms and a new post in 156 ms through orm.
  - Sessions are not limited: the lock serializes them, so one write takes the same time and the last of sixteen
    sessions that write at once waits 1.3 s.
- **Past the envelope** the corpus is read and written as before. The validator reports it: `scale.file` at a
  document of more bytes than 1 MiB, `scale.folder` at a folder of more documents than the limit and `scale.corpus` at the
  corpus root when it holds more, each naming what is past, its size, its table and the limit. A commit that writes a
  document past 1 MiB publishes the event `scale.exceeded`, naming the file, its bytes and the limit.
  Grounding: a limit that refused a write would make a corpus that grew past it unusable by a mistake of
  size; the cost is the user's to decide, so the engine reports it. A write is refused only for what the engine
  cannot hold correctly: the descriptors of the watch that the process cannot get, which is `DRIVER`.
- The size of a document and of a folder is the question of a storage declaration: a document table that
  grows past 1 MiB is stored as `files` or sharded by time ([time shards](20-document-model.md#time-shards)).
- Linux and Windows are measured with the same benchmarks; a number that differs from macOS changes the
  envelope's grounding, not the means.

## Failure vocabulary

| Face name | orm code | Meaning |
| --- | --- | --- |
| `LOCK_TIMEOUT` | `CANCELED` | the lock wait ended — at its deadline, or at once when the holder has ended; names the holder and its state — `CANCELED`, as a SQLite `busy_timeout` is |
| `CORPUS_CONFLICT` | `WRITE_CONFLICT` | a target changed since it was read, or a change since the read makes the transaction's writes other writes; names each file and line |
| `DUPLICATE_KEY` | `DUPLICATE_KEY` | a primary or unique key value already exists; names both rows (table and key) |
| `FOREIGN_KEY` | `FOREIGN_KEY` | a reference names no row, or a referenced row is deleted while referenced |
| `CONSTRAINT` | `CONSTRAINT` | any other rule of the catalog refused the write — a check, a title, a transition, a required column, a limit, an order, a single-line place; names the rule, and the row and column ([canonical format](10-canonical-format.md#diagnostics)); or a write that must rewrite a non-canonical or malformed document, or a request, read or write, that needs the rows of an unavailable table ([canonical format](10-canonical-format.md#diagnostics)), naming the file and the diagnostic |
| `READ_ONLY` | `READ_ONLY` | a write in a read-only transaction |
| `CONFIG` | `CONFIG` | a control message that the session's state does not allow ([transactions](#transactions)), a `flowmark canon` path that names no corpus document ([the canon operation](#the-canon-operation)), or a face operation that is unknown, lacks a member or names a setting its table does not declare ([the faces](#the-faces)) |
| recovery refusal | `DRIVER` | names the target and both recorded hashes and the stage files that hold them; opening, and every operation that acquires the lock, refuses until resolved — `DRIVER`, orm's code of a database error no other code names, as SQLite's `SQLITE_CORRUPT` is; a failure of the file system under the journal is `DRIVER` too |

Every failure message names the cause and the fix.
