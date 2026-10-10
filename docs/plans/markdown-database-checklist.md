<!-- doc-id: plans-markdown-database-checklist -->
# The markdown database checklist

## T12 The markdown database

- [ ] T12 The markdown database: a finite storage grammar declared by the `dbspec/` catalog, flowmark as the engine that orm reaches as an IR executor by DSN, and views over the same rows. Cause: the list, table and detailed shapes were demo cases with no rule behind them — no body column, no parent, no relation, no history — the engine sat in flowmark while the plan placed it in orm, and several behaviours were hardcoded (follow-up of T8.5, T10, T11)
- [o] T12.1 Rewrite the rulings and the specifications 00, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100 and 110 on the markdown database plan: the seam correction, the T8.5 withdrawal, the storage grammar, the catalog, the server and transactions, the history and the views (requires T12)
  - Completion criteria: every ruling of the plan stands in its specification as a proposition with its grounding, the design plan states the corrected seam and the withdrawn T8.5 with the facts that withdrew them, and `make documents-check` and `make refs-check` pass.
- [ ] T12.2 The storage codec: parse and render `files`, `document` and `block` storages in their list and table forms, with P1/P2 for every storage; requires orm `T62` (requires T12.1)
- [o] T12.2.1 Load the catalog: read the `dbspec/*.dbs` set of a corpus root with orm's `engine/dbspec`, and `dbspec/flowmark.json` (translations); reject a missing catalog, a table without `store`, a composite key, `aes`/`aes_version`/`blind_index`, `audit`/`immutable`, `cascade`/`set_null`, an external `use`, a missing `history` or `initial` on a state machine, a missing `title` on a list or files storage, and a heading depth past H6, each as a located diagnostic (requires orm `T62`)
  - Completion criteria: a test per rejection and a loaded fixture catalog.
- [ ] T12.2.2 The `files` storage: parse and render one file per row — header comments, frontmatter of the stored scalar columns, H1 title, body, level-2 sections — with P1/P2 (requires T12.2.1)
  - Completion criteria: golden fixtures round-trip byte for byte, and each format diagnostic of spec 10 for this storage has a test.
- [ ] T12.2.3 The `document` storage: `<table>.md` with its list or table under the H1 and detail sections at H2, list fields parsed from the right with their escapes, table cells with their escapes, indentation as `parent`, derived `order` (requires T12.2.1)
  - Completion criteria: as T12.2.2.
- [ ] T12.2.4 The `block` storage: child blocks inside a parent row scope at the next level, recursively, written only when non-empty, with detail sections and their sections (requires T12.2.2, T12.2.3)
  - Completion criteria: as T12.2.2, including a block inside a block row's detail section.
- [ ] T12.2.5 One dataset in three storages: the same rows stored as files, document and block parse to equal rows, and P1/P2 hold for each (requires T12.2.4)
  - Completion criteria: a storage-equivalence test.
- [ ] T12.3 The validator: keys, foreign keys, cycles, titles, block shape, order, limits, the history gap and chain, and the views (requires T12.2)
- [ ] T12.4 The IR executor under the orm IR-executor protocol: params, assembled relations, write answers and errors; requires orm `T61` (requires T12.3)
- [ ] T12.5 The write path: the portable lock, the journal, transactions and savepoints, history rows and the history repair, order moves (requires T12.4)
- [ ] T12.6 Introspection: validate every document against the catalog and return the catalog (requires T12.3)
- [ ] T12.7 Sync on the runtime model: the export renders through orm's SQL dialect renderer (requires T12.4)
- [ ] T12.8 The conformance vectors rewritten for the storage grammar (requires T12.5)
- [ ] T12.9 The server: `flowmark serve`, the token, the session control messages; requires orm `T61` (requires T12.5)
- [ ] T12.10 The CLI, MCP and sidecar operations over any table, and `flowmark install` (requires T12.5)
- [ ] T12.11 The demos as one dataset in three storages, and the hardcoding inventory of the plan at zero (requires T12.10)
- [ ] T12.12 Require the orm release instead of the pseudo-version: once T12.11 is done, flowmark's go.mod requires the released orm tag that carries `T61` and `T62` (requires T12.11 and the orm release item)
  - Completion criteria: go.mod requires a tagged orm version, `go build ./...` and `go test ./...` pass.
