<!-- doc-id: execution-checklist -->
# Execution checklist

A task is `[ ]` waiting, `[~]` in progress, `[o]` done, `[!]` bypassed with its cause and retry condition, or `[-]` impossible with its cause. A task becomes `[o]` in the commit that completes it, after its owning command passes.

## T1a Design session

- [o] T1.0 Bootstrap the repository with the document pairs and this checklist
  - Completion criteria: README, AGENTS, CHANGELOG and this checklist exist in English and Korean, `.flowmark/` is ignored, and `git log` holds the bootstrap commit.
- [~] T1.1 Fold the adversarial audit into the design plan (requires T1.0)
  - Completion criteria: every audit blocker appears as a ruling in docs/plans/design-plan.md, the Korean twin holds the English source-sha256, and the plan revision commits.
- [ ] T1.2 Vendor polyspec/kit and configure the trackers (requires T1.0)
  - Completion criteria: `make kit-check` and `make documents-check` pass on the vendored tree with config/checklist.json and config/documents.json authored.
- [ ] T1.3 Register the stopgap ID reference checker (requires T1.2)
  - Completion criteria: every task ID token in docs/ resolves to a checklist item and the check is part of the document tooling.

## T1b Specifications

- [ ] T1.4 Write 00-overview, 10-canonical-format, 20-document-model, 30-schema, 40-ops and 50-constraints as full document pairs (requires T1.2)
  - Completion criteria: every ruling appears as a proposition with its grounding.
- [ ] T1.5 Write the 60-dialect capability table and the 70-sync, 80-migration, 90-conformance skeletons, and 100-checklist and 110-open-questions in full (requires T1.4)

## Later phases

- [ ] T2 Codec in Go: strict parser and canonical renderer (requires T1.4)
- [ ] T3 Validator: corpus-global constraints (requires T2)
- [ ] T4 Executor: the IR kinds over the parsed corpus (requires T3)
- [ ] T5 orm markdown-executor integration: manifest settings and the state-machine sub-grammar (requires T4)
- [ ] T6 Sync adapters: the contract and the database adapter (requires T5)
- [ ] T7 CLI and MCP: thin adapters over the orm model API (requires T5)
- [ ] T8 Checklist application: the local issue model (requires T3)
- [ ] T9 Conformance vectors: corpus before, request, expected corpus after (requires T4)
- [ ] T10 Local GUI: the kanban board and issue tracker as one more thin consumer (requires T8)
