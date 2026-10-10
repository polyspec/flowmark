<!-- doc-id: changelog -->
# Changelog

## Unreleased

- T7.3-3: the sidecar answers stay whole lines. The watcher's poll goroutine and the request
  loop wrote to one buffered stream without a lock, so a pushed changed event could tear an
  answer line — the page then lost an answer and its view of the corpus. One lock now guards
  the stream, the session end flushes under it, and a root's fingerprint baseline is taken when
  the surface is watched, before its first answer, so an edit right after that answer is seen.

- T7.1-1: `flowmark items` prints the same item the sidecar serves: six tab-separated columns
  ending in the path, assignee and due. The executor rows carry the path directly, so the
  sidecar drops its documents join.

- T8.6: the write path serves the table form. A document whose items live in a table gets the
  same commands as the list form: a transition rewrites the state cell by the document's own
  edges, an add appends a row with the auto ID (NextID reads table rows), an edit rewrites the
  title cell, a move reorders the row lines and a remove deletes the row. One grammar per
  document — checklist lines and an items table together are refused.

- T11: items carry people and dates. An item title may close with `@assignee` and a
  `YYYY-MM-DD` due date — the codec splits the closing tokens from the title — and a document
  holds a people table by naming `id` and `name` columns. The first row is the owner: an item
  without an assignee is the owner's. `All("items")` gains assignee and due for list and table
  items alike, `All("people")` returns the people rows, and the sidecar items answer carries
  the two new columns with a `people` answer of its own.
- T11 correction: the people table is the registry of the declared people, not an owner
  declaration — a blank assignee reads as self and the board never guesses a name; the
  assignee shows as its id (`@max`) in one consistent form, and a due may carry a time
  (`YYYY-MM-DDTHH:MM[:SS]` with an optional zone).

- T4.3: items also live in table form. A markdown table whose header row names the state, id and
  title columns is the document's item list — the header the document writes is the definition —
  and `All("items")` returns its rows in row order; the codec records the absolute line of every
  data row so a write path can address them.

- T8.5: the Mermaid block of the document is the only transition authority. Transition reads the
  edges, the required evidence and the glyphs from the document's own state machine — an
  undeclared edge is refused — AddItem lands a new item in the state the `[*]` line names, the
  WIP gate counts the kit glyphs `[~]` against `[o]` regardless of the state names, ApplySchema
  rewrites the Mermaid block itself, and the built-in five-state table, the frontmatter schema
  reader, the unused document-driven wrappers and the dead corpus mermaid walker are gone.

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
- T6: the sync engine lands its v1 — the md-to-db export as a derived, deterministic SQL dump of the rows, and the materialization of a rowset into an adapter-owned document whose provenance header records the hash of its body, so the drift check is self-verifying and clone-safe. T6 completes.
- T5: the orm repository lands the manifest settings and the state-machine sub-grammar as the state_machine and markdown settings of dbspec (orm `T59` b74d8890 and `T60` fb218c7c): parse, validation, canonical emission and the settings model, with the engine suite green. The executor registration in the orm engine remains the engine roadmap; the capability contract stays spec 60-dialect.
- T10: the plugin starts in its own repository (soksak/plugins/flowmark, commit ae07ec9): the kanban board over the flowmark sidecar — five state columns, one card per item, the board.refresh command, and no write path of its own, so the board cannot break the state machine.
- T7.3-1: every sidecar and MCP message flushes on its own write. Cause: the deferred flush held each answer until the input ended, so a host that keeps the session open received nothing while it ran — the example Wails host of the plugin repository found it over a live round trip. A regression test reads one answer while the pipe stays open.
- T8.4: issues enter through the write path — `flowmark add` appends one waiting item with a unique-ID check and `flowmark rm` removes one, refusing a parent that carries children; the MCP tools flowmark_add and flowmark_remove and the sidecar operations add and remove carry the same surface.
- T7.3-2: the sidecar watches the corpus — it polls the fingerprint of every open surface root and pushes a changed event, so the page refreshes while the session runs instead of needing a restart.
