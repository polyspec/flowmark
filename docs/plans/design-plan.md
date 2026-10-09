<!-- doc-id: plans-design-plan -->
# flowmark Design Plan (T1)

Revision 2 — folds the findings of the three-lane adversarial audit (engine core, orm integration, operations) and the multi-platform directive into the rulings below. Every audit blocker became a ruling; the audit's unbroken claims are listed at the end.

## Context

The user manages engineering specs as markdown and runs development through checklists (five states `[ ]` `[~]` `[o]` `[!]` `[-]`, dotted hierarchical IDs, prerequisite/follow-up dependencies), changelogs, and commit discipline. The problem: document-based operation has **no idempotent, database-level enforcement** — format violations, dangling foreign keys, and illegal status transitions stop no one.

**flowmark's identity (user-confirmed)**: a **text database whose grammar is markdown**. The files themselves are the database and it runs fully standalone without an external database. **orm is the core** (the markdown executor); **flowmark is the integration and application layer** — bidirectional sync with databases, GitHub, Jira and other trackers, and on top of it a local kanban board and issue tracker (GUI as one more thin consumer): GitHub and Jira have their own boards, but here the data is managed locally, in git, offline. Core value: the storage format is a document humans read, and git provides audit, branching, and rollback for free.

**User decisions**:

1. polyspec/orm gains markdown as its **fourth backend** (mysql/pgsql/sqlite/markdown). The orm model API is the **orthodox path** for CLI and MCP (thin adapters with no write logic of their own).
2. Schema derivation direction: **dbspec ⇒ frontmatter** — dbspec is the root grammar and authority; frontmatter DDL is an embedding of dbspec (detail in §4).
3. Documents are **EN/KO pairs from day one** (orm house style, kit-compatible).
4. **Vendor polyspec/kit now** — the fifth state `[-]` is **upstreamed in the polyspec/kit repository** (orm's `scripts/kit` is a hash-locked vendor copy that must never be edited in orm; it reaches orm only through `make kit-sync KIT_TAG=<tag>`). The user delegated authority over kit.
5. This session's deliverable = **design documents + repository bootstrap** (implementation is tracked as later phases on the checklist). Per the audit's scope finding, the spec documents themselves split into a second session (T1b); this session closes T1a.
6. Sync projection files: real markdown files exist on disk. `readonly` (default) or `writable` option (detail in §7).
7. The repository lives at **github.com/polyspec/flowmark** — a standalone repository inside the polyspec organization.
8. **Multi-platform is a hard requirement. No platform tricks.** The simplest implementation that fully satisfies the current requirements wins (kit's own rule, applied here): no flock, no POSIX-only durability assumptions — locks follow kit's `holder-lock` precedent (lock file + holder identity + stale detection, pure runtime primitives), and crash safety comes from the journal replay, not from OS-specific directory fsync semantics.

## Verified grounding (checked directly in the polyspec checkouts)

- `orm/docs/protocol.md:12,96` — IR kinds are bounded: `one|all|count|group_count|sum|avg|paginate|insert|update|delete|restore`. The request carries no SQL text → a **native markdown executor consuming the IR request is a bounded problem**. The audit confirmed expressiveness: every kind plus the Order/Pred/Func surface is computable over an in-memory corpus; the open questions were specification, not expressiveness.
- `orm/docs/dialects.md:184` — the three databases unify collation to binary codepoint order. This is the rule for **text comparison**; it does not transfer to typed keys (see the ordering ruling in §2).
- `kit/scripts/kit/documents-stamp.mjs`, `check-documents.mjs` — `source-sha256` is an **EN→KO translation sync marker** (the Korean twin holds the sha256 of the raw English file bytes). Data drift detection is therefore designed on `.flowmark/` sidecar state, and flowmark adopts **raw-file-bytes hashing** for the same marker (kit-compatible; canonicalization belongs in the check, not the hash).
- `kit` is its own repository (`polyspec/kit`); consumers hold a hash-locked vendor copy of `scripts/kit` + `tests/kit` written by `make kit-sync KIT_TAG=<tag>` and verified by `make kit-check`. A vendored file is changed in kit, never in the consumer.
- `kit/scripts/kit/checklist.mjs` — states are per-tracker data; the `[-]` impossible state is now read at both levels (upstreamed as kit K13, branch `feat/fifth-state-K13` pending merge).
- kit document rules: every English document needs a Korean twin (doc-id match, explicit anchors match, fenced blocks match, the twin holds source-sha256); plans/ and CHANGELOG are twin-checked too.

## Core design (what the spec documents will pin down)

### 1. Architecture and the seam

```
CLI / MCP (thin adapters — the orm model API is the orthodox path)
   ↓
polyspec/orm (.dbs → generated models → IR request)
   ↓ shared planner validates the IR (IR_INVALID rules unchanged)
markdown executor = the text database engine (standalone, consumes the IR request)
   ├─ codec: strict parser + canonical renderer (two-clause idempotency, §2)
   ├─ validator: corpus-global constraints (type/PK/UNIQUE/FK/CHECK/state machine)
   ├─ in-memory indexes derived from the parsed corpus (rebuilt on open, cached by content hash)
   ├─ transaction: stage → re-read validate → prepare record → renames → commit record
   └─ .flowmark/ sidecar: lock, tx journal, corpus revision, parsed-row cache
   ↓ optional
sync engine: md→db export / db→md materialize (§7)
```

**The seam ruling (resolves the layering circularity)**: markdown is a fourth **executor** registered beside the SQL executors, selected by the DSN scheme `markdown://<path>`. The shared planner validates the IR exactly as for SQL backends; the markdown executor consumes the **IR request** directly and does not render SQL plans — `getQuery()`, statement events, `subscribe()` and `force_index` return `CAPABILITY_UNSUPPORTED` (orm's existing precedent for unsupported capability). The executor implementation lives in the **orm repository** (flowmark contributes it at T5); until the other clients port it, the capability table marks markdown support **Go-only**. flowmark-the-repository then holds: the specs, the kit tooling, the checklist application, and the sync engine.

- A SQLite mirror is rejected: mirror↔corpus equivalence becomes a permanent bug class. If a mirror is ever admitted, its conformance conditions are (a) full-rebuild equivalence and (b) no write path.
- The IR-boundedness claim covers the request layer. **API parity is not claimed** — the capability table (§6) is the contract, in orm's `CAPABILITY_UNSUPPORTED`/`OPERATOR_NOT_ALLOWED` tradition.

### 2. Canonical format (`10-canonical-format.md`)

- **Idempotency is the dbspec two-clause proposition**, not a single byte-identity claim: `emit(parse(s)) == s` for canonical input `s`, and `emit(parse(emit(d))) == emit(d)` for any valid input `d`. The renderer owns exactly the **structural skeleton** — the frontmatter block, heading lines, the item and table lines it re-sorts, and the markers it writes — and never rewrites payload bytes in preserved runs (prose, fenced content, HTML comments other than owned markers). Non-canonical input is a `format.noncanonical` diagnostic plus a separate one-shot `flowmark canon` migration transaction, never a side effect of a logical write.
- The document preamble (title and text before the first H2) is a reserved long-text column; an H2 section maps to its declared long-text column; absent section = NULL, present-but-empty = empty text.
- **Ordering is type-directed**: ID-typed keys compare segment-wise numeric (`T8.0.9 < T8.0.10`), text compares by codepoint, composite keys lexicographically by component type. The SQL collation decision governs text comparison only; it is not imported as the ordering of typed keys. `group_count` groups NULLs together (SQL parity).
- ID grammar: `T<n>(.<n>)*(-<n>)?` with no leading zeros and no suffix `-0`; equality is string equality on the canonical form; the suffix is part of the item's identity and **travels with its base under renumber**. Children of suffixed items (`T8.0.14-1.3`) are allowed; the suffix binds to the last path segment. References are read structurally (parsed positions, code spans excluded); codemods that rewrite references are the explicit exception to payload preservation. `flowmark next-id` computes the next sibling ID — humans are not expected to compute IDs unaided.
- **Frontmatter profile**: YAML 1.2 core schema, block style in the value region, no anchors or tags. Frontmatter comments are rejected in v1 with a located diagnostic (a comment-preserving round-trip emitter is a stated non-goal until a requirement names it). Value syntax per dbspec type: decimal, datetime and uuid literals use dbspec's canonical forms; `bytes` is base64; a text column that may hold newlines cannot live in a markdown-table child cell.
- A translation pair is **one row**: `doc_id` is the primary key; the Korean twin file is a projection of the row, not a second row. `source-sha256` = sha256 of the raw English file bytes (kit-compatible).

### 3. Document-table model (`20-document-model.md`)

- Folder = table, file = row, H2 section = long-text column, structured body blocks (checklist items, markdown tables) = child tables (FK).
- Frontmatter = row data + `table` binding + `schema_version` stamp (+ the embedded DDL when standalone).

### 4. Schema (`30-schema.md`) — derivation ruling: dbspec ⇒ frontmatter

- **dbspec is the root grammar and authority.** Frontmatter DDL is not an independent language but a frontmatter embedding of the dbspec grammar: the fragment is always a **YAML literal block scalar** (`|`), which settles YAML-hostile tokens (`yes`/`on`/`null` as names, datetime colons, `#` inside the fragment) and keeps one grammar, one parser, one type system. dbspec diagnostics are offset-mapped into file coordinates. The fragment is canonicalized (LF, final newline) at embed time; the stamp carries the **canonical** fragment — there is no second, minified emission form.
- Rationale for the direction: (1) schema is law, data is rows — a per-document DDL is a copy of the law, and copies diverging is itself the drift bug class; (2) orm's engine/IR/conformance are bound to the dbspec manifest hash, so a second grammar doubles the parts; (3) dbspec's parser and type system are reused; (4) the reverse direction would compose the corpus-global schema by merging per-document declarations, creating a new conflict class — DDL reconciliation across branches.
- **Standalone documents** (single files outside a repository): the embedded fragment is that document's provisional authority, compiled to the same IR over a one-document set (no `use` declarations). On import into a corpus, the repository's .dbs becomes the authority and the fragment becomes a consistency check target.
- **Self-description is satisfied by projection**: the schema stamp (canonical fragment + manifest hash + `schema_version`) is rendered into frontmatter from the .dbs — a verified cache, not data; mismatch is a located `schema.drift` diagnostic, never a silent reparse.
- **Markdown-specific schema surface is manifest-level settings, never rendered types** (introspection coherence, the `updated`/`soft_delete` precedent): `markdown`-flavored column traits and checklist child-table sugar are settings; a real database introspects them back as plain `text`. The state machine is likewise a manifest-level setting with its own sub-grammar (as `audit` has), which the SQL backends ignore as they ignore `navigation`; the markdown validator enforces it.
- **`identity` columns are rejected on markdown corpora.** Keys are human-assigned (the ID grammar); an insert that omits the primary key is an error, and `flowmark next-id` is the allocation helper. Key non-reuse under git rewind is abandoned by design — git history, not a sequence, is the allocator of record.

### 5. Ops and transactions (`40-ops.md`)

- **Commit protocol** (order is normative): write post-image files into `.flowmark/tx/<id>/` content-addressed and fsync them; copy the pre-image of every target into the stage and fsync; write and fsync a **prepare record** listing every rename with its pre- and post-hashes; perform the renames through the runtime's atomic rename; write and fsync the commit record; collect the stage. **Recovery runs on every corpus open**: a prepare record completes, its absence discards the stage, and a target whose current hash matches neither recorded hash is refused and reported (a human edit landed mid-crash — never silently overwritten). Durability across power loss relies on the journal replay, not on OS-specific directory fsync.
- **One portable lock, per operation** (multi-platform ruling): every operation — read or write, CLI or MCP — takes the same lock file with holder identity and stale detection (kit `holder-lock` precedent) for the duration of the operation, with a timeout that reports the holder. There is no long-lived lock and no reader/writer split: readers cannot observe torn commits, an MCP server cannot lock the CLI out, and a hung holder is a timeout report, not a deadlock. Conflicts on commit (a target changed since read) fail with the file and line and a defined re-read-and-retry-once contract — no silent merge.
- **Validation runs at commit against the corpus freshly re-read under the lock** — not against a BEGIN snapshot, and not against a predicted touched-file set (cascade discovery makes that set unknowable). The read-hash/check is computed from what was actually read.
- **Corpus revision**: a monotonically increasing counter in `.flowmark/`, bumped by every commit. A multi-call read session (paginate) carries it and fails with `CORPUS_CHANGED` on mismatch (or degrades to keyset pagination). `orderByRandom` is seeded per revision.
- **Cost model**: a per-command p95 latency target is declared in 40-ops (order: hundreds of milliseconds at the corpus envelope); a `.flowmark/cache/` of parsed rows keyed by per-file content hash avoids re-parsing unchanged files; default per-transaction validation is the FK/UK closure of the post-image with full `flowmark validate` on demand and on schema change.
- Walk rules: skip `.flowmark/` and hidden entries; a symlink or hardlink inside the corpus is a located diagnostic; a nested corpus root is an error.

### 6. Constraints and the capability table (`50-constraints.md`, `60-dialect.md`)

- Strict parse, type/PK/UNIQUE/FK (dangling prerequisites rejected; NFC then exact match), CHECK. Soft-deleted rows still occupy unique keys and still satisfy FKs (SQL parity — stated so a soft delete never trips a spurious ruling). Validation of a corpus whose schema declares AES codec columns requires the key; a keyless run fails rather than approximates.
- The **state machine is a dbspec-level manifest setting** (transition table + terminals + required fields); the checklist app encodes `[o]`-terminal→supersede (`T8.0.14-1`), the WIP gate, and the `[!]` cause/retry and `[-]` cause requirements on top of it. The diagnostic shape matches kit: `{line, column, rule, message}`.
- **Human writes void the database's writer-monopoly premise; each feature rules on it**: FK actions are **restrict-only** on markdown corpora (`cascade`/`set_null` are rejected at schema parse — they are unimplementable for a writer that bypasses the executor); `audit` and `immutable` are advisory (validate-time rejection, no trigger-time prevention for human edits); a human-authored file missing a non-null defaulted column is rejected by validate, and `flowmark ingest` stamps defaults explicitly.
- **The capability table is written in T1b as full content** (it is the integration contract): reject the four `lock` modes (or accept `update|share` as documented no-ops and reject `*_nowait`); reject `isolation`/`timeoutMs`, `fulltext`/`Lb`; define the `on_duplicate` conflict target explicitly (the rule is written, and the divergence from sibling backends on multi-unique-key tables is documented); state that plain `update` is strengthened to optimistic semantics on markdown (every commit re-reads); rule `getQuery`, statement events, `subscribe`, `force_index` as `CAPABILITY_UNSUPPORTED`; savepoints map to nested staging.
- The markdown plan lifecycle (orm's dbplan/apply/recover machinery) is **scoped out of v1**: v1 supports corpus creation and additive column addition; dbplan chains on markdown corpora are rejected (80-migration states the scope and the reason).

### 7. Sync and adapters (optional, `70-sync.md`)

- The sync engine is an **adapter architecture with one contract**; the database is the first adapter, GitHub (issues/projects) and Jira follow: per-table truth ownership, a provenance header naming the owning system (`<!-- owner: <adapter>:<table>@<sha256-of-own-bytes> -->`), git-aware drift handling, a recorded materialized-from revision as the merge base, and explicit one-shot commands (no live daemon). An external tracker's items materialize as corpus rows — the checklist app's item model is the local issue model, so the local board and the remote tracker are two projections of the same rows.
- md→db: derived export (rebuildable). db→md (and every adapter→md): materialize — the hash is of the materialized file's own bytes, so the check is self-verifying and clone-safe, and its scope is stated honestly: accidents, not adversaries. The validator rejects edits to adapter-owned documents (readonly default); `writable` re-ingests after validation.
- **Readonly drift is git-aware**: the working tree differing from HEAD restores to HEAD and reports; a clean tree whose HEAD differs from the last ingested version accepts and re-baselines. Merge conflicts on adapter-owned paths are resolved by discarding both sides and re-materializing — a hand merge there is a diagnostic.
- The changelog projection derives from `item_revisions` — on-file data — never from the gitignored sidecar journal (a fresh clone must be able to rebuild every projection).

### 8. Migration (`80-migration.md`)

- v1 scope: corpus creation from the .dbs, additive column additions, and explicit transactional codemods (`flowmark renumber` rewrites the item and every structural reference in one transaction — the suffix travels with its base — and is rejected if any dangling reference or collision results). Additive-first; renames are explicit steps; dbplan chains are rejected (see §6).

### 9. Checklist app (`100-checklist.md`)

- Tables: `checklists`, `checklist_items` (self-FK parent, five states, supersedes FK), `item_dependencies` (prerequisite/follow-up), `item_revisions` (append-only: from/to, reason, evidence), `changelogs` (a projection of item_revisions).
- **Views are just another thin consumer**: a kanban board (columns = the five states, cards = checklist items, dependencies as edges) reads through the same orm queries and writes transitions through the same orm model API — an illegal drag is rejected by the validator exactly like an illegal CLI transition, and the WIP gate becomes a column limit. No view-specific write path may exist (registered as a later-phase item; the same principle covers any future UI).
- The kit→flowmark checklist cutover (when the app ships) is one transaction: drop the tracker from `config/checklist.json`, codemod the document to corpus format, and pass `flowmark validate` in the same commit. Until then a stopgap reference checker (every `T<n>…` token in docs/ resolves to a checklist item) covers the dangling-reference class that kit cannot check.

## Session plan (T1a this session; T1b next session)

**T1a — audit-folded design + bootstrap** (this session)

1. `git init` flowmark, `.gitignore` (`.flowmark/`), `README.md`+`README.ko.md`, `AGENTS.md`+`AGENTS.ko.md` (operating rules: checklist discipline, conventional commits, the [o]↔changelog↔commit unit, the twin workflow — a Korean-caught fix is ported to the English file before the next stamp, no interactive history edits across document commits), `CHANGELOG.md`+`CHANGELOG.ko.md` (`## Unreleased`). Commit: `chore: Bootstrap flowmark repository`.
2. **kit `[-]` upstream** — done on branch `feat/fifth-state-K13` (worktree `kit-fifth-state-K13`, red vectors → FIVE_STATES + cause rules → green, K13 row + AGENTS twins + stamped KO checklist). Merge into kit's `main` once the parallel work there stabilizes, then remove the worktree and branch. orm adopts later via `make kit-sync` when the user tags a kit release.
3. **Vendor kit whole-directory** into flowmark (`scripts/kit` + `tests/kit` + `kit.json` + `.kit/kit.lock.json` following the kit-sync contract; the lock records the kit commit) and author `config/checklist.json` (list format, `T<n>(.<n>)*(-<n>)?` ID pattern, five states, active `[~]`) and `config/documents.json`. Commit: `chore: Vendor polyspec/kit tooling`.
4. `docs/plans/execution-checklist.md`+`.ko.md` — the T1a/T1b/T2–T9 items in the user's format (doc-id, prerequisite/follow-up, cause/completion-criteria/evidence), maintained throughout.
5. This design plan (revision 2) and its Korean twin — committed with the audit folded in.

**T1b — spec documents** (next session; EN/KO pairs, one commit per document, twins stamped): `00-overview`, `10-canonical-format` (§2), `20-document-model` (§3), `30-schema` (§4), `40-ops` (§5), `50-constraints` (§6), then skeletons `60-dialect` (the capability table as full content), `70-sync`, `80-migration`, `90-conformance`, full `100-checklist`, `110-open-questions`. Acceptance criterion per document: every ruling appears as a proposition with its grounding (decision count, not prose volume).

**T9 conformance contract (stated now)**: markdown vectors are corpus-before + IR request + expected corpus-after / expected rows, run against the Go executor; SQL/binds comparison does not apply (there is no SQL). Cross-client comparison begins when other clients port the executor.

**Later phases (registered as `[ ]` checklist items only)**: T2 codec (Go), T3 validator, T4 executor, T5 orm markdown-executor integration (manifest settings, state-machine sub-grammar — orm-repo changes), T6 sync (the adapter contract and the database adapter; GitHub and Jira adapters register under it), T7 CLI/MCP (an MCP tool-list mini-spec first; per-operation locking is already the core ruling), T8 checklist app (the local issue model), T9 conformance vectors, T10 GUI (the local kanban board and issue tracker as one more thin consumer — no view-specific write path).

## What the audit could not break

- Between two conforming flowmark writers, per-operation locking + stage/prepare/commit loses no update (the real holes were readers, humans, and crash windows — now ruled above).
- The dbspec ⇒ frontmatter derivation direction: the standalone-fragment provisional authority covers offline documents, and the reverse direction genuinely creates the DDL-reconciliation conflict class this plan rejects.
- IR expressiveness over a corpus: all kinds, relations (limit_per_parent, if_parent, flatten, key_by, composite keys), tuple_in with subqueries, group_count, restore and optimistic map to unambiguous in-memory definitions given type-directed ordering and PK-sorted determinism.

## Open questions (kept in `110-open-questions.md`; promoted into a spec when decided)

- flowmark's own license — recommendation: Apache-2.0 (patent grant; a schema-language derivative next to an AGPL neighbor); decide before the first public push.
- The corpus scale envelope (e.g. ≤10k documents / ≤100k child rows) and the latency budget numbers that 40-ops declares.
- Confirming the `T8.0.14-1.3` suffix-children ruling (recommendation: allow — restated as a proposition in 10-canonical-format).
- Frontmatter comment support (rejected in v1; revisit when a requirement names it).
- The MCP tool list and its per-call surface (T7 mini-spec).
- The orm-side ordering of the manifest-setting extensions (T5).

## Verification (T1a)

- kit branch `feat/fifth-state-K13`: `node --test tests/kit/checklist.test.mjs` 13/13, `make documents-check` green (verified in the worktree).
- flowmark `make kit-check` (lock intact), `make documents-check` (pairs, doc-id, anchors, fences, source-sha256), checklist reading with five states — green.
- `git log` (all repositories): conventional commit shape, feature-sized units, every `[o]` item accompanied by a changelog entry (flowmark) or its owning row (kit).
- Spec completion criteria (verifiable without implementation): cross-references resolve, one name per concept, every ruling a proposition with grounding, the two-clause idempotency stated as such.
- T1b can start with `[~]` on `10-canonical-format`.
