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
- T3.1: the corpus validator walks a directory of documents — the strict parse findings, the unique doc-ids and item IDs across the corpus, and every task-ID token in prose (fenced blocks and inline code excluded) resolving to an item.
- T3.2: `make validate` runs `flowmark validate` over the corpus directory with the selection
  of config/documents.json, and the corpus of this repository passes. T3 completes.
- T4.1: the executor runs one, all and count over the corpus rows — documents and checklist items as the first tables — with where predicates, ordering and limit; the task-ID ordering is segment-wise numeric, so `T8.0.9` sorts before `T8.0.10` and the suffix follows its base.
- T4.2: the executor runs sum, avg, group_count and paginate — integer aggregates with a non-digit rejection, per-value counts, and a page with its total. T4 completes.
- T8.1: the application encodes the transition table — done is terminal, a bypass carries its cause and retry condition, an impossible item its cause — with the five states mapped to the kit glyphs and the superseded spawn numbering `T8.0.14-1`, `T8.0.14-2`.
- T8.2: applying a transition holds the table, the required fields and the WIP gate — at most two doing items beyond the done count — and rewrites the state glyph of the item line in place, leaving every other byte of the document alone.
- T8.3: `flowmark items` lists the corpus items and `flowmark transition` applies one through
  the state machine; the walk now enters a dot root, a Korean twin registers no second
  keys, and the changelog examples carry their IDs in code spans. T8 completes.
- T1.4.8: 20-document-model states the aggregate and the split shapes — one file, one-to-many linked files, one-to-many-to-one — all built from the file-as-row, child-block and foreign-key-column primitives, with restrict-only keys.
- T9.1: the conformance vectors run — corpus before, request, expected rows as projections — with four vectors pinning the document order, the where count, the type-directed item order and the paginate total; spec 90 states the vector shape. T9 completes its first stage.
- T7: the three faces run from one binary — the CLI (validate, items, transition), the MCP server (initialize, tools/list, tools/call over stdio, one JSON-RPC message per line) and the soksak sidecar (the protocol v1 loop with surface routing and the closed handshake). T7 completes.
