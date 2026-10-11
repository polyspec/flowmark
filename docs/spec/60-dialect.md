<!-- doc-id: spec-dialect -->
# Capability table

[한국어](60-dialect.ko.md)

orm reaches a corpus through the DSN scheme `markdown://`, which selects an **IR executor** rather
than a SQL dialect ([operations](40-ops.md) holds the server and the DSN). This specification is
the integration contract: the wire, and what each orm surface does on a corpus, or the orm error code it answers —
`CAPABILITY_UNSUPPORTED` for a surface the executor does not run, `OPERATOR_NOT_ALLOWED`,
`IR_INVALID` and `CODEC_ENCODE` as orm's rules give them for every backend. The orm side of the
contract — the protocol section, the error code `WRITE_CONFLICT`, the executor-assigned columns and
the capability table in orm `docs/dialects.md` — is orm's own work, tracked in orm's checklist; the
Go client implements it first and every other client is its own orm item.

## The wire

- **Request**: `{ir_version, manifest_hash, request, params, now}` per request. The client validates
  the request (`ir.Validate`) and does not plan it. `params` use the runtime value model JSON —
  decimal as a string, bytes as base64, datetime as `YYYY-MM-DD HH:MM:SS.ffffff` UTC and time as
  `HH:MM:SS.ffffff`, whose digits past the column's precision p are zeros the value does not hold — a
  parameter with another digit there is not a value of `datetime(p)` or `time(p)` (`CODEC_ENCODE`).
  Codec stages are applied by the client, as for SQL. A parameter that is not a value of the type of
  the slot it fills — the column it is compared with or assigned to, a column function's result,
  `text` for `contains` and `contains_binary`, the `i32` or `f64` interval of a relative time
  function — fails the request with `CODEC_ENCODE` before any row is read, naming the parameter index
  and the type: its JSON kind, the type's range, a decimal's scale and precision, a time's or
  datetime's precision, a `varchar`'s length, U+0000 in text, and a uuid's lower-case canonical form
  are checked, as every orm client checks a bound value
  ([orm protocol](https://github.com/polyspec/orm/blob/main/docs/protocol.md)). NULL is a value of
  every slot except the assignment of a non-null column, where it is `CODEC_ENCODE`. Grounding:
  orm's code for a bound value outside its slot is `CODEC_ENCODE`, and a check made before reading
  makes the answer independent of the rows, as a bind failure is; the values of a face operation are the engine's own write and are not checked so — the catalog's rules judge them and refuse with `CONSTRAINT` ([constraints](50-constraints.md)); `IR_INVALID` is structural, and
  this request is well formed.
- **Answer to a read**: `{"rows": [...]}` for `one`, `all` and `group_count`, `{"count": n}` for
  `count`, `{"value": v}` for `sum` and `avg`, `{"rows": [...], "count": n}` for `paginate`, `n`
  counting every matching row. A row is a JSON object: its outputs in projection order, then each join
  (a nested object, or null for a left join without a match) and each relation (a `one` relation an
  object or null, a `many` relation an array, or an object keyed by `key_by`); a `flatten` relation is
  answered as a `one` relation, and the client model merges it
  ([orm protocol](https://github.com/polyspec/orm/blob/main/docs/protocol.md)). Values use the value
  model of the parameters: a datetime `YYYY-MM-DD HH:MM:SS.ffffff` and a time `HH:MM:SS.ffffff`, a
  decimal and bytes as strings, an integer or `f64` a JSON number, a `bool` a JSON bool, a date and a
  uuid their readable text. A stored value the answer cannot write in that form is `INTERNAL`, never
  answered as its stored text.
- **Answer to a write**: `{affected, keys}`, where `keys` lists the generated or given primary key of
  each inserted row in insert order, the met row's key for an `on_duplicate` update. `affected` counts
  each inserted or updated row once — an insert with `on_duplicate` is 1 — and an update counts the
  rows it matched. Grounding: orm's clients count an update's matched rows on every database, and an
  upsert that updates is 2 on MySQL and 1 on PostgreSQL and SQLite until orm normalizes it to 1.
- **Error**: one code of orm `errors.yaml`, written `{"error": {"code", "message"}}`; a hash mismatch
  is `SCHEMA_HASH_MISMATCH`.
- **Control messages**: `{op: begin | commit | rollback | savepoint | release | rollback_to, name?}`,
  and `read_only`, a boolean, on `begin`; each answered `{"ok": true}` or with an error. A connection
  is a session, whose first message is `{"op": "connect", "corpus", "token"}` with the DSN's corpus
  and token ([operations](40-ops.md#the-server)).

## Surfaces

| Surface | Ruling |
| --- | --- |
| `one`, `all`, `count`, `group_count`, `sum`, `avg` | Supported over the in-memory corpus; `group_count` groups NULLs together |
| ordering | An IR `order` sorts by the column's type, NULL before every value ascending and after every value descending, ties in storage order — also for the groups of `group_count` and the ranking of `limit_per_parent`; a read without one returns storage order ([document model](20-document-model.md#order)). Grounding: MySQL and SQLite put NULL first ascending and PostgreSQL last; MySQL has no `NULLS FIRST`, so NULL-smallest is the one rule all three render without emulation |
| `group_by` | Valid on the root of `count` and `group_count` only, elsewhere `IR_INVALID`; with it, an `order` item names a group column, a column function of one, or `random` (`IR_INVALID` otherwise) — orm's rule. Grounding: every row read selects the primary key, so a grouped row read is MySQL 1055, PostgreSQL 42803 and an arbitrary row on SQLite, and so is an order by a column that is not grouped |
| `paginate` | Supported; one request reads one consistent corpus |
| `insert` (single and multi-row) | Supported; `identity`, `key_prefix` and `order` columns are executor-assigned and may be omitted; defaults are written explicitly; an insert assigns by value or NULL — `plus_p` and `minus_p` are `IR_INVALID` there and valid in `on_duplicate` (orm's rule). Grounding: MySQL computes such an assignment from the default and PostgreSQL and SQLite refuse it |
| `update` | Supported; a target changed since it was read fails `WRITE_CONFLICT`, never an overwrite; an assignment of the primary key or an identity column is `IR_INVALID` ([document model](20-document-model.md#keys)) |
| `delete`, `restore` | Supported; `restore` keeps its SQL shape (soft-delete column cleared, unique keys held while deleted); a referenced row's delete is `FOREIGN_KEY`, also when the same delete removes the referencing row ([constraints](50-constraints.md#keys-and-references)); a `restore` that assigns the primary key, an identity or the soft delete column is `IR_INVALID` |
| `optimistic` | Supported; no match is `OPTIMISTIC_LOCK` |
| `on_duplicate` | Supported when exactly one key — the primary key, unless the insert leaves it to `identity` or `key_prefix`, and each unique key — can meet the inserted row and the insert assigns all its columns; that key is the conflict target, and any other `on_duplicate` is `IR_INVALID` (orm's rule, which makes MySQL's any-key rule and the PostgreSQL and SQLite conflict target the same key); the primary key meets a row as its comparison below states. Grounding: MySQL meets a row on any unique key, in its own key order, while PostgreSQL and SQLite name one conflict target and answer another key's meeting `DUPLICATE_KEY`; with one key that can meet the row the three are the same by construction |
| primary key comparison | Without regard to the case of A–Z, unlike orm's binary collation on SQL, because a key is a file name and an anchor ([document model](20-document-model.md#keys)): an insert whose key equals another without regard to case is `DUPLICATE_KEY`, and an `on_duplicate` insert whose conflict key is the primary key meets that row — the byte-equal key first, else the least in byte order — which keeps its key. Grounding: `ON DUPLICATE KEY UPDATE` and `ON CONFLICT` fire on the duplicate the conflict key's index reports, by that index's comparison |
| `lock` `update`, `share` | Accepted as documented no-ops: a server transaction already holds the corpus lock |
| `lock` `update_nowait`, `share_nowait` | `CAPABILITY_UNSUPPORTED` |
| relations (`limit_per_parent`, `if_parent`, `flatten`, `key_by`, `no_cascade_delete`) | Supported; assembled by the executor, a `flatten` relation answered as a `one` relation, which the client model merges into the parent's array and JSON forms (orm's rule); `key_by` names a non-null column (`IR_INVALID` otherwise). Grounding: orm places the merge in the client model's array and JSON forms, where a name the parent holds keeps the parent's value; a merged answer would hand the client a row it cannot tell from the parent's own columns. A NULL collection key answers differently by client language, so a nullable `key_by` is refused by its shape |
| subqueries (`tuple_in`, `tuple_not_in`, scalar `Sub` aggregates) | Supported |
| a scalar column subquery (`columns.sub` without `agg`) | Valid with `limit` count 1 and an `order` that includes every primary key column of its entity and no `random`, otherwise `IR_INVALID` (orm's rule); its value is the column of that row, NULL when none matches. Grounding: a scalar subquery of several rows is MySQL 1242, PostgreSQL 21000 and the first row on SQLite, so only a shape that yields one row has one result |
| column comparisons (`*_col`, `in` a subquery, join `left`/`right`, relation keys) | Two columns meet as dbspec's check operands do — integers with integers, `decimal` of one scale, `f64`, `varchar` and `text`, `time` or `datetime` of one precision, and `uuid`, `date`, `bool` or `bytes` each with itself; another pair is `OPERATOR_NOT_ALLOWED` (orm's rule). Grounding: PostgreSQL refuses `integer = varchar`, MySQL coerces and SQLite compares by affinity, and `date = datetime`, decimals of two scales and an integer with an `f64` near 2^53 disagree across the three |
| `contains` (the `Lk` operator) | Supported: true when the column's text holds the parameter's text as a substring, the ASCII letters A–Z and a–z compared without regard to case and every other character by code point; a NULL column or parameter is unknown, and `%`, `_` and `\` in the parameter are literal. Grounding: SQL parity — PostgreSQL `ILIKE` under orm's `COLLATE "C"`, SQLite `LIKE` and orm's MySQL ASCII fold give this result |
| `contains_binary` (the `Lb` operator) | Supported: true when the column's text holds the parameter's text as a substring, compared by code point; a NULL column or parameter is unknown, and `%`, `_` and `\` in the parameter are literal. Grounding: MySQL `LIKE` under `utf8mb4_0900_bin`, PostgreSQL `LIKE` under `COLLATE "C"` and SQLite `instr` agree |
| `now`, `default now`, `updated`, `soft_delete` | The request's `now`, one value per request: truncated to p digits when written into `datetime(p)`, six digits when compared ([operations](40-ops.md#the-clock)); a relative time function whose result lies outside 0001-01-01 to 9999-12-31 23:59:59.999999, or whose interval cannot be represented, is NULL (orm's rule). Grounding: MySQL and SQLite give NULL past the range, and NULL is the one result the other dialect can be guarded to |
| `soft_delete` | Every read excludes soft-deleted rows — the root, each join child (in its join condition, so a left join binds null), each relation and each subquery; `restore` alone matches one. Grounding: orm's rule that reads always exclude soft-deleted rows |
| codec columns | Applied by the client on an orm request; a write the engine originates (CLI, MCP, sidecar, history) applies the codec through orm's Go codec package, and validation decodes with the same package; the stored value is the encoded one, a byte-producing stage as base64 |
| indexes | Accepted; derived in memory |
| `select explicit`, `entity`, `navigation` | Client-side; no storage effect |
| `aes`, `aes_version`, `blind_index` | Schema error on a corpus — the key never leaves the client |
| `audit`, `immutable` settings | Schema error on a corpus; `state_machine … history` instead |
| transactions | `begin` to `commit`/`rollback` over the session; `readOnly` honoured and takes the lock |
| savepoints | Nested stages |
| `orderByRandom` | Supported; the order is not seeded |
| a transaction with audit values | `CAPABILITY_UNSUPPORTED` |
| `isolation`, `timeoutMs` options | `CAPABILITY_UNSUPPORTED` |
| `getQuery()`, statement events, `subscribe()` | `CAPABILITY_UNSUPPORTED` — there is no SQL text |
| `force_index` | `CAPABILITY_UNSUPPORTED` — indexes are derived, not named |
| `fulltext` | `OPERATOR_NOT_ALLOWED`: SQLite has no full-text index on a table (orm dialects.md) |
| lock, local-setting, privilege and AES-rotation utilities | `CAPABILITY_UNSUPPORTED` |
| schema install, dbplan chains | `CAPABILITY_UNSUPPORTED`; `flowmark install` creates a corpus ([schema](30-schema.md)), and evolution follows [migration](80-migration.md) |
| an unavailable table (a file with `format.block`, `format.section`, `format.encoding` or a `parent.cycle` among its rows, two rows with byte-for-byte equal keys, or a `corpus.walk` entry at its place) | every request that needs its rows, read or write, fails `CONSTRAINT`, naming the file and the diagnostic; no answer comes from the other files' rows alone ([canonical format](10-canonical-format.md#diagnostics)) |

The conformance contract for this table is [conformance](90-conformance.md): corpus before,
request, expected corpus after or rows.
