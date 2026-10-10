<!-- doc-id: spec-open-questions -->
# Open questions

[한국어](110-open-questions.ko.md)

Each entry states the question, the current recommendation and the condition that decides it.
A decision promotes its entry into the owning specification and removes it here.

| # | Question | Recommendation | Decided by |
| --- | --- | --- | --- |
| 1 | The license of this repository | Apache-2.0 (the patent grant, for a schema-language derivative) | before the first public push |
| 2 | The corpus scale envelope (documents, rows) and the p95 latency budget of 40-ops | ≤10k documents / ≤100k rows; hundreds of milliseconds per command | the first engine benchmarks at T12.4 |
| 3 | Frontmatter comment support | keep rejected in v1; revisit only when a requirement names it | a requirement that names it |
| 4 | The order of the orm items that T12 requires | the protocol's IR executors and `WRITE_CONFLICT` first, then the markdown settings — the settings item needs the executor-assigned columns that the protocol item defines | the orm checklist entries |
| 5 | Re-vendor kit after the user tags the next kit release | `make kit-sync KIT_TAG=<tag>` replaces the local-path lock record | the kit release tag |

Withdrawn with the markdown database plan: children of a suffixed item (keys are opaque, and the
suffix grammar is removed), and the MCP tool list (the faces operate over any table, T12.10).

Two standing notes. The design plan and the markdown database plan stay the ruling record until
each specification absorbs its part; when they disagree, the specification wins and the plan gets
the correction. And the open-question list itself is reviewed at the close of every phase — an
entry that no longer has a deciding condition is decided or dropped, never left to rot.
