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
- [o] T7.1-1 The CLI items answer carries the path, assignee and due columns — the same columns the sidecar answer holds, and the executor rows carry the path directly (follow-up of T7.1)
  - Completion criteria: `flowmark items <root>` over a corpus with an assignee and a due prints six tab-separated columns ending in the path, assignee and due, with a test over the writer.
- [o] T7.2 The MCP server face: initialize, tools/list and tools/call over stdio (requires T7.1)
- [o] T7.3 The soksak sidecar face: the protocol v1 message loop over stdio (requires T7.1)
- [o] T7.3-1 Flush each sidecar and MCP message on its own. Cause: Serve held the bufio flush until the input ended, so a host that keeps the session open — the example Wails host, an MCP client — received no answer until the process stopped; the protocol and the JSON-RPC session both deliver each message while the session runs. Acceptance: every answer flushes on write, before the input ends, with a regression test that reads one answer while the pipe stays open.
- [o] T8 Checklist application: the local issue model (requires T3)
- [o] T8.1 Encode the transition table: terminals, required fields and the superseded spawn (requires T3)
- [o] T8.2 Apply a transition to the corpus: rewrite the state, append the revision line and hold the WIP gate (requires T8.1)
- [o] T8.3 Wire `flowmark items` and `flowmark transition` as the owning commands (requires T8.2)
- [o] T8.4 Enter issues through the write path: add appends one waiting item with a unique ID, rm removes one and refuses a parent that carries children — on the CLI, the MCP tools and the sidecar operations (follow-up of T8.3)
- [o] T8.4-1 The sidecar add addresses the first document when the request carries no path, and the answer names the ID the document chose (follow-up of T8.4)
  - Completion criteria: adding the first issue to an empty corpus over the sidecar — no items answer exists yet to carry a path — writes the item into the first document and answers `T1 added`, not ` added`.
- [ ] T8.4-2 The repository tracks no built executable: the root `flowmark` is removed from tracking and ignored, and builds write only `build/` (follow-up of T8.4). Cause: a `go build` run in the repository root wrote the 4.4 MB executable `flowmark`, and the T8.4 commit took it in
  - Completion criteria: `git ls-files` names no built executable, a `go build ./cmd/flowmark` in the root leaves `git status` clean, and `make build` writes `build/flowmark` only.
- [o] T8.5 The Mermaid block is the only transition authority: Transition reads the edges, the required fields and the glyphs from the document's own state machine, AddItem takes the initial state from the `[*]` line, ApplySchema rewrites the Mermaid block, and no built-in table remains (follow-up of T8.4)
  - Completion criteria: a document with custom state names transitions by its own declared edges — an undeclared edge is refused, an edge label names the required evidence, a new item lands in the declared initial state — and `grep` finds no hardcoded state map in the Go sources.
- [o] T8.6 The write path serves the table form: transition, edit, add, remove and move rewrite the row cells of an items table — one grammar per document, a mix of lines and a table is refused (follow-up of T8.5, requires T4.3)
  - Completion criteria: on a table-format document a transition rewrites the state cell by the document's own edges, an add appends a row with the auto ID, an edit rewrites the title cell, a move reorders the rows and a remove deletes the row.
- [o] T7.3-2 Watch the corpus and push a changed event: the sidecar polls the fingerprints of the open surface roots and pushes, the page refreshes while the session runs (follow-up of T7.3-1)
- [o] T7.3-3 The sidecar answers stay whole lines and the watcher baseline predates the first answer: the poll goroutine and the request loop share one writer under one lock, and a root's fingerprint is taken before its first answer is built (follow-up of T7.3-2)
  - Completion criteria: concurrent changed events and answers never interleave inside a line (a test under `-race` reads only whole JSON lines), and an edit made right after the first items answer of a root still pushes a changed event.
- [o] T9 Conformance vectors: corpus before, request, expected corpus after (requires T4)
- [o] T9.1 Define the vector shape and run the first vectors through the executor (requires T4)
- [o] T10 The soksak plugin in its own repository: the kanban board and issue tracker as one more thin consumer (requires T8)
- [o] T10-1 The board never shows a state the document does not hold. Cause: a drop moved the card before the transition was judged, and a refusal — an undeclared edge such as done → doing, or an edge that needs evidence — returned without a rollback, so the card stayed in the wrong column while the markdown kept its state; an empty items answer (the last item removed) was shown as a toast and left the old cards; a changed event refreshed the items but not the state machine, so an edited Mermaid block left stale columns (follow-up of T10)
  - Completion criteria: a pure `planDrop` decides every drop before the DOM moves (a refusal moves nothing and refreshes the board), every operation ends in an items refresh whether it was accepted or refused, an empty answer clears the columns, and a changed event re-reads the schema and rebuilds the columns only when it differs; `node --test` over the plugin covers each case.
- [o] T10-2 The new-issue row follows the last card of its column instead of sticking to the column bottom, and the whole column takes a drop. Cause: the card list grew to fill the column (`flex:1`), so a column stretched to its taller neighbour left a gap between its cards and the row, and a drop on that gap or on the row found no list (follow-up of T10-1)
  - Completion criteria: the row sits directly under the last card in a short and in an empty column, a drop anywhere inside a column lands in that column's list, and `node --test ui/` covers the drop target.
- [o] T10-3 Every column takes the full height of the board; no column takes its height from its neighbour. Cause: the board grid sized its row to the tallest column (`align-content:start`) and the shorter columns stretched to it, so the heights followed the busiest column — neither independent nor full (follow-up of T10-2)
  - Completion criteria: in a rendered window every column spans from the board top to the board bottom whatever the cards are, a column with more cards than fit scrolls inside itself, and the rendering is observed in a screenshot.
- [o] T10-4 The new-issue row is pinned to the bottom of its column: the card list fills the column and scrolls inside itself, the row stays visible below it however many cards there are (follow-up of T10-3, which gave every column the full board height)
  - Completion criteria: in a rendered window the row sits at the column bottom in a short, an empty and an overflowing column alike, and the rendering is observed in a screenshot.
- [o] T10-5 The plugin repository tracks no built executable: `example/example` is ignored and rebuilt by `go build` (follow-up of T10). Cause: the example host binary (16 MB) was committed with the example host and rebuilt copies kept showing as changes
  - Completion criteria: `git ls-files` of the plugin repository names no `example/example` and a rebuild leaves `git status` clean.
- [o] T11 People and dates on the items: an item title may close with `@assignee` and a due date or datetime (`YYYY-MM-DD`, `YYYY-MM-DDTHH:MM[:SS]`, optional zone), a document holds a people table (an `id`/`name` header) as the registry of its declared people, a blank assignee is self — no name is ever guessed — and the items and people surfaces expose them (requires T4.3)
  - Completion criteria: the codec splits the closing tokens from the title and accepts a due with a time, `All("items")` carries assignee and due for list and table items alike, `All("people")` returns the people rows, and the sidecar items answer gains the assignee and due columns with a `people` answer of its own.
