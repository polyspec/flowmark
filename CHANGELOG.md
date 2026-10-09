<!-- doc-id: changelog -->
# Changelog

## Unreleased

- T1.0: the repository bootstraps with its document pairs (README, AGENTS, CHANGELOG), the
  `.flowmark/` ignore rule and this execution checklist pair.
- T1.1: the design plan folds the three-lane adversarial audit into rulings — the executor seam
  in orm, two-clause idempotency, dbspec-driven frontmatter embedding, the prepare-record commit
  protocol with one portable per-operation lock, restrict-only foreign keys, the rejected
  identity column, type-directed ordering, adapter-based sync.
- T1.2: the repository vendors polyspec/kit whole-directory with its lock (`make kit-sync`,
  `make kit-check`) and configures the trackers: config/checklist.json reads the
  execution-checklist pair with the five states and config/documents.json selects the document
  pairs and the changelog; `make documents-check` passes.
- T1.3: `make refs-check` reads the checklists of config/checklist.json and the prose of every
  selected document (fenced blocks and inline code excluded) and reports each task ID that
  resolves to no item as a finding with its file, line and column; `make test` covers it.
- T1.1-1: one Go binary carries three thin faces — the CLI, the MCP server and the soksak sidecar
  (protocol v1 of the soksak core specification) — and the GUI ships as a soksak plugin in its own
  repository; the repository stays single-language Go with a `package.json` and a `sidecar.json`
  manifest, and per-language packages appear only when a requirement names them.
- T1.4.2: 10-canonical-format states the two idempotency propositions, the skeleton with the preservation boundary, the document grammar, the frontmatter profile, the task-ID grammar with type-directed ordering, and the encoding rules.
- T1.4.3: 20-document-model states folder as table, file as row, the column mappings, child tables, the translation pair as one row, and the frontmatter bindings.
- T1.4.4: 30-schema states the dbspec-authority derivation, the literal-block embedding, the standalone provisional authority, the schema stamp, and the human-write rulings.
