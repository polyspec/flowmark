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
- T1.4.5: 40-ops states recovery on open, the per-operation portable lock, the normative commit protocol with its prepare record, the corpus revision token, and the cost model.
- T1.4.6: 50-constraints states strict parse, types, keys and references, the state-machine setting, the human-write rulings, and the key-required codec validation.
- T1.5.1: 60-dialect writes the capability table in full — the IR surface, the strengthened update, the on_duplicate ruling, the rejected surfaces, and the Go-only marking.
- T1.5.2: 70-sync states the adapter contract scope and the settled rulings — truth ownership, provenance, git-aware drift handling, and rebuildable projections.
- T1.5.3: 80-migration states the additive-first scope, the codemod transaction, the reference-rewrite exception, and the checklist cutover.
- T1.5.4: 90-conformance states the corpus-vector contract, the required round-trip and idempotency properties, and the Go-first runner.
- T1.5.5: 100-checklist writes the application in full — the tables, the five-state machine with terminals and required fields, the WIP gate, the board as one more consumer, and the cutover.
- T1.5.6: 110-open-questions registers the seven open decisions with recommendations and deciding conditions.
- T1.4.1: 00-overview writes the scope, the enforcement summary, the glossary, the non-goals, and the document map.
- T1.4.7: the Korean documents follow the dry style — action verbs, no metaphor, English technical terms; T1.4 completes with its seven sub-items.
- T2.1: the Go module starts with the document skeleton — the strict parse and the canonical render of the header comments, the title, the preamble, the frontmatter block and the H2 sections — with the P1 and P2 round-trip tests and the header diagnostics.
- T2.5: the codec proves P1 and P2 over the repository corpus — every selected document parses without a diagnostic and re-renders byte for byte, and a CRLF copy re-renders stably; the grammar takes the H2 and H3 headings and the header-title adjacency of the real documents.
- T2.4: every parse rule reports a located diagnostic — the fixed rule names with line and column — and a fenced block is payload: the structural rules read unfenced lines only, and an unclosed frontmatter fence names its opening line.
- T2.2: the frontmatter parses as the YAML 1.2 core subset — scalars, quoted scalars, sequences and literal blocks — and re-emits canonically; comments, the flow style, anchors, tab indents and duplicate keys are located rejections.
- T2.3: the child blocks extract as rows — the checklist items of the kit grammar with their states, IDs, titles and continuation lines, and the markdown tables with headers and cells — from the unfenced payload only; a malformed item is a located rejection. T2 completes.
