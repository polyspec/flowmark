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

- [~] T1.4 Write the six core specifications as full document pairs (requires T1.2)
  - Completion criteria: every ruling appears as a proposition with its grounding; it completes when T1.4.1 through T1.4.7 complete.
- [~] T1.4.1 Write docs/spec/00-overview.md(.ko)
- [o] T1.4.2 Write docs/spec/10-canonical-format.md(.ko)
- [o] T1.4.3 Write docs/spec/20-document-model.md(.ko)
- [o] T1.4.4 Write docs/spec/30-schema.md(.ko)
- [o] T1.4.5 Write docs/spec/40-ops.md(.ko)
- [o] T1.4.6 Write docs/spec/50-constraints.md(.ko)
- [ ] T1.4.7 Rewrite the Korean documents in the dry style — no metaphor, action verbs, English technical terms (requires T1.4.6)
- [ ] T1.5 Write the remaining specifications (requires T1.4)
  - Completion criteria: the capability table in full, three scoped skeletons, and 100-checklist and 110-open-questions in full; it completes when T1.5.1 through T1.5.6 complete.
- [ ] T1.5.1 Write docs/spec/60-dialect.md(.ko) with the capability table in full
- [ ] T1.5.2 Write docs/spec/70-sync.md(.ko) as a scoped skeleton
- [ ] T1.5.3 Write docs/spec/80-migration.md(.ko) as a scoped skeleton
- [ ] T1.5.4 Write docs/spec/90-conformance.md(.ko) as a scoped skeleton
- [ ] T1.5.5 Write docs/spec/100-checklist.md(.ko) in full
- [ ] T1.5.6 Write docs/spec/110-open-questions.md(.ko) in full

## Later phases

- [ ] T2 Codec in Go: strict parser and canonical renderer (requires T1.4)
- [ ] T3 Validator: corpus-global constraints (requires T2)
- [ ] T4 Executor: the IR kinds over the parsed corpus (requires T3)
- [ ] T5 orm markdown-executor integration: manifest settings and the state-machine sub-grammar (requires T4)
- [ ] T6 Sync adapters: the contract and the database adapter (requires T5)
- [ ] T7 The three faces — CLI, MCP server and soksak sidecar — as thin adapters over the orm model API (requires T5)
- [ ] T8 Checklist application: the local issue model (requires T3)
- [ ] T9 Conformance vectors: corpus before, request, expected corpus after (requires T4)
- [ ] T10 The soksak plugin in its own repository: the kanban board and issue tracker as one more thin consumer (requires T8)
