<!-- doc-id: plans-markdown-database-checklist -->
# The markdown database checklist

## T12 The markdown database

- [ ] T12 The markdown database: a finite storage grammar from the `.dbs` declarations, flowmark as the engine that orm reaches by DSN, and views over the same rows. Cause: the list, table and detailed shapes were demo cases with no rule behind them — no body column, no parent, no relation, no history — the engine sat in flowmark while the plan placed it in orm, and several behaviours were hardcoded (follow-up of T8.5, T10, T11)
- [~] T12.1 Rewrite the rulings and the specifications: the seam ruling (flowmark is the engine, orm reaches it through a driver), the storage grammar, the settings, the relations, the server and the views; withdraw T8.5; write the working principles into AGENTS (requires T12)
  - Completion criteria: the design plan and the specifications 10, 20, 30, 40, 50, 60 and 100 state every storage rule of the plan as a proposition with its grounding, the T8.5 Mermaid authority and the in-orm executor are withdrawn with the facts that withdrew them, AGENTS holds the working principles, and `make documents-check` and `make refs-check` pass.
- [ ] T12.2 Add the markdown storage settings to the orm dbspec — `store`, `title`, `body`, `checkbox` and the state-machine `limit` — with red-then-green rule tests and the orm document pairs (requires T12.1)
- [ ] T12.3 Rewrite the engine on the storage grammar: the codec with P1/P2 for every storage, the validator (foreign keys, cycles, title, block shape), the full IR executor, the write path with the portable lock and the journal, persisted history and the markdown-to-dbs introspection; the old engine paths are removed (requires T12.2)
- [ ] T12.4 Generalize the faces to tables: the DSN server, the CLI, MCP and sidecar operations over any table, `flowmark install`, and the demos rebuilt as one dataset in three storages (requires T12.3)
- [ ] T12.5 The orm Go `markdown` driver reaches the flowmark server and passes the conformance over every IR kind; the other clients get their own orm checklist items (requires T12.4)
- [ ] T12.6 The plugin switches views over the same rows — kanban, list and gantt — with a detail modal, the hierarchy, separated UI strings and answers judged by structure (requires T12.4)
