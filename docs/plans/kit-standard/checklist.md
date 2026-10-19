<!-- doc-id: checklist-kit-standard -->
# The kit standard checklist

| ID | Task | Deliverables | Verification | State |
|---|---|---|---|---|
| T15 | The repository follows the kit standard 0.0.54 | flowmark follows the kit standard 0.0.54: its layout, its plans, its checks and its gates | `make kit-check` | [o] |
| T15-1 | Fix the findings of the kit gates: Go lint and the skips, the CI and the composer packages | golangci and deadcode pass in packages/go; no test skips; CI runs make flowmark-test and make refs-check; composer packages are published releases; knip declares its entries and binaries | `node --test tests/*.test.mts` | [!] cause: the type-aware TypeScript lint still fails: scripts/record-slugs.mts is untyped and the board example imports packages no install provides; retry: when make lint-ts passes with record-slugs.mts and the board example typed |
