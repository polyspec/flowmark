<!-- doc-id: execution-checklist -->
# Execution checklist

## T1a Design session

- [o] T1.0 Bootstrap the repository with the document pairs and this checklist
  - Completion criteria: README, AGENTS, CHANGELOG and this checklist exist in English and Korean, `.flowmark/` is ignored, and `git log` holds the bootstrap commit.
- [o] T1.1 Fold the adversarial audit into the design plan (requires T1.0)
  - Completion criteria: every audit blocker appears as a ruling in docs/plans/design-plan.md, the Korean twin holds the English source-sha256, and the plan revision commits.
- [o] T1.1-1 State the three faces and the soksak sidecar ruling (follow-up of T1.1)
  - Completion criteria: the plan names one Go binary with the CLI, the MCP server and the soksak sidecar (protocol v1) as its three thin faces, and the GUI as a soksak plugin in its own repository.
- [o] T1.2 Vendor polyspec/kit and configure the trackers (requires T1.0)
  - Completion criteria: `make kit-check` and `make documents-check` pass on the vendored tree with config/checklist.json and config/documents.json authored.
- [o] T1.3 Register the stopgap ID reference checker (requires T1.2)
  - Completion criteria: every task ID token in docs/ resolves to a checklist item and the check is part of the document tooling.

## T1b Specifications

- [o] T1.4 Write the six core specifications as full document pairs (requires T1.2)
  - Completion criteria: every ruling appears as a proposition with its grounding; it completes when T1.4.1 through T1.4.7 complete.
- [o] T1.4.1 Write docs/spec/00-overview.md(.ko)
- [o] T1.4.2 Write docs/spec/10-canonical-format.md(.ko)
- [o] T1.4.3 Write docs/spec/20-document-model.md(.ko)
- [o] T1.4.4 Write docs/spec/30-schema.md(.ko)
- [o] T1.4.5 Write docs/spec/40-ops.md(.ko)
- [o] T1.4.6 Write docs/spec/50-constraints.md(.ko)
- [o] T1.4.7 Rewrite the Korean documents in the dry style — no metaphor, action verbs, English technical terms (requires T1.4.6)
- [o] T1.4.8 State the aggregate and split shapes in the document model: one file, one-to-many linked files, and one-to-many-to-one (follow-up of T1.4.7)
- [o] T1.5 Write the remaining specifications (requires T1.4)
  - Completion criteria: the capability table in full, three scoped skeletons, and 100-checklist and 110-open-questions in full; it completes when T1.5.1 through T1.5.6 complete.
- [o] T1.5.1 Write docs/spec/60-dialect.md(.ko) with the capability table in full
- [o] T1.5.2 Write docs/spec/70-sync.md(.ko) as a scoped skeleton
- [o] T1.5.3 Write docs/spec/80-migration.md(.ko) as a scoped skeleton
- [o] T1.5.4 Write docs/spec/90-conformance.md(.ko) as a scoped skeleton
- [o] T1.5.5 Write docs/spec/100-checklist.md(.ko) in full
- [o] T1.5.6 Write docs/spec/110-open-questions.md(.ko) in full

## Later phases

- [o] T2 Codec in Go: strict parser and canonical renderer (requires T1.4)
- [o] T2.1 Bootstrap the Go module with the document skeleton: parse and render the header, the title, the preamble, the frontmatter block and the H2 sections with a canonical round-trip (requires T1.4)
- [o] T2.2 Parse the frontmatter as the YAML 1.2 core subset and re-emit it canonically (requires T2.1)
- [o] T2.3 Parse the child blocks — checklist items and markdown tables — into rows (requires T2.2)
- [o] T2.4 Report every parse rule as a located diagnostic with the fixed rule names (requires T2.1)
- [o] T2.5 Prove P1 and P2 over the corpus of this repository with a fuzz-free golden set (requires T2.1)
- [o] T3 Validator: corpus-global constraints (requires T2)
- [o] T3.1 Validate the corpus keys and references: unique doc-ids and item IDs, and every task-ID token in prose resolves (requires T2)
- [o] T3.2 Wire `flowmark validate` as the owning command over a corpus directory (requires T3.1)
- [o] T4 Executor: the IR kinds over the parsed corpus (requires T3)
- [o] T4.1 Run one, all and count over the corpus rows with where, order and limit (requires T3)
- [o] T4.2 Run sum, avg, paginate and group_count with the type-directed ordering (requires T4.1)
- [o] T4.3 Read items from a markdown table: a table whose header row names the state, id and title columns is a document's item list in table form, its rows items in row order (requires T4.2)
  - Completion criteria: `All("items")` returns the rows of such a table with their state glyphs, IDs, titles and per-document order, and the codec records the absolute line of every data row.
- [o] T5 orm markdown-executor integration: manifest settings and the state-machine sub-grammar (requires T4)
- [o] T6 Sync adapters: the contract and the database adapter (requires T5)
- [o] T6.1 Export the corpus to SQL: a derived, deterministic INSERT dump of the rows (requires T4)
- [o] T6.2 Materialize a rowset into markdown with the provenance header and the readonly drift rules (requires T6.1)
- [o] T7 The three faces — CLI, MCP server and soksak sidecar — as thin adapters over the orm model API (requires T5)
- [o] T7.1 The CLI face: validate, items and transition over the corpus (requires T3)
- [o] T7.2 The MCP server face: initialize, tools/list and tools/call over stdio (requires T7.1)
- [o] T7.3 The soksak sidecar face: the protocol v1 message loop over stdio (requires T7.1)
- [o] T7.3-1 Flush each sidecar and MCP message on its own. Cause: Serve held the bufio flush until the input ended, so a host that keeps the session open — the example Wails host, an MCP client — received no answer until the process stopped; the protocol and the JSON-RPC session both deliver each message while the session runs. Acceptance: every answer flushes on write, before the input ends, with a regression test that reads one answer while the pipe stays open.
- [o] T8 Checklist application: the local issue model (requires T3)
- [o] T8.1 Encode the transition table: terminals, required fields and the superseded spawn (requires T3)
- [o] T8.2 Apply a transition to the corpus: rewrite the state, append the revision line and hold the WIP gate (requires T8.1)
- [o] T8.3 Wire `flowmark items` and `flowmark transition` as the owning commands (requires T8.2)
- [o] T8.4 Enter issues through the write path: add appends one waiting item with a unique ID, rm removes one and refuses a parent that carries children — on the CLI, the MCP tools and the sidecar operations (follow-up of T8.3)
- [o] T8.5 The Mermaid block is the only transition authority: Transition reads the edges, the required fields and the glyphs from the document's own state machine, AddItem takes the initial state from the `[*]` line, ApplySchema rewrites the Mermaid block, and no built-in table remains (follow-up of T8.4)
  - Completion criteria: a document with custom state names transitions by its own declared edges — an undeclared edge is refused, an edge label names the required evidence, a new item lands in the declared initial state — and `grep` finds no hardcoded state map in the Go sources.
- [o] T7.3-2 Watch the corpus and push a changed event: the sidecar polls the fingerprints of the open surface roots and pushes, the page refreshes while the session runs (follow-up of T7.3-1)
- [o] T9 Conformance vectors: corpus before, request, expected corpus after (requires T4)
- [o] T9.1 Define the vector shape and run the first vectors through the executor (requires T4)
- [o] T10 The soksak plugin in its own repository: the kanban board and issue tracker as one more thin consumer (requires T8)
