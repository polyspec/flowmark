<!-- doc-id: spec-dialect -->
# Capability table

[한국어](60-dialect.ko.md)

orm reaches a corpus through the DSN scheme `markdown://`, which selects an **IR executor** rather
than a SQL dialect ([operations](40-ops.md) holds the server and the DSN). This specification is
the integration contract: the wire, and what each orm surface does on a corpus or rejects with
`CAPABILITY_UNSUPPORTED` or `OPERATOR_NOT_ALLOWED`, in the orm tradition. The orm side of the
contract — the protocol section, the error code `WRITE_CONFLICT`, the executor-assigned columns and
the capability table in orm `docs/dialects.md` — is orm's own work, tracked in orm's checklist; the
Go client implements it first and every other client is its own orm item.

## The wire

- **Request**: `{ir_version, manifest_hash, request, params, now}` per request. The client validates
  the request (`ir.Validate`) and does not plan it. `params` use the runtime value model JSON —
  decimal as a string, bytes as base64, datetime as `YYYY-MM-DD HH:MM:SS.ffffff` UTC. Codec stages
  are applied by the client, as for SQL.
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
  each inserted row in insert order.
- **Error**: one code of orm `errors.yaml`; a hash mismatch is `SCHEMA_HASH_MISMATCH`.
- **Control messages**: `{op: begin | commit | rollback | savepoint | release | rollback_to, name?}`,
  each answered `{ok}` or with an error. A connection is a session.

## Surfaces

| Surface | Ruling |
| --- | --- |
| `one`, `all`, `count`, `group_count`, `sum`, `avg` | Supported over the in-memory corpus; `group_count` groups NULLs together |
| ordering | An IR `order` sorts by the column's type, NULL before every value ascending and after every value descending, ties in storage order — also for the groups of `group_count` and the ranking of `limit_per_parent`; a read without one returns storage order ([document model](20-document-model.md#order)). Grounding: MySQL and SQLite put NULL first ascending and PostgreSQL last; MySQL has no `NULLS FIRST`, so NULL-smallest is the one rule all three render without emulation |
| `paginate` | Supported; one request reads one consistent corpus |
| `insert` (single and multi-row) | Supported; `identity`, `key_prefix` and `order` columns are executor-assigned and may be omitted; defaults are written explicitly |
| `update` | Supported; a target changed since it was read fails `WRITE_CONFLICT`, never an overwrite |
| `delete`, `restore` | Supported; `restore` keeps its SQL shape (soft-delete column cleared, unique keys held while deleted); a referenced row's delete is `FOREIGN_KEY`, also when the same delete removes the referencing row ([constraints](50-constraints.md#keys-and-references)) |
| `optimistic` | Supported; no match is `OPTIMISTIC_LOCK` |
| `on_duplicate` | Supported with the conflict target **any unique key** (the MySQL rule); on a table with several unique keys this diverges from the PostgreSQL and SQLite target rule, and the divergence is stated here |
| `lock` `update`, `share` | Accepted as documented no-ops: a server transaction already holds the corpus lock |
| `lock` `update_nowait`, `share_nowait` | `CAPABILITY_UNSUPPORTED` |
| relations (`limit_per_parent`, `if_parent`, `flatten`, `key_by`, `no_cascade_delete`) | Supported; assembled by the executor, a `flatten` relation answered as a `one` relation, which the client model merges into the parent's array and JSON forms (orm's rule); `key_by` names a non-null column (`IR_INVALID` otherwise). Grounding: orm places the merge in the client model's array and JSON forms, where a name the parent holds keeps the parent's value; a merged answer would hand the client a row it cannot tell from the parent's own columns. A NULL collection key answers differently by client language, so a nullable `key_by` is refused by its shape |
| subqueries (`tuple_in`, `tuple_not_in`, scalar `Sub` aggregates) | Supported |
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
