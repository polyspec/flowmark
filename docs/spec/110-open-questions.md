<!-- doc-id: spec-open-questions -->
# Open questions

[한국어](110-open-questions.ko.md)

Each entry states the question, the current recommendation and the condition that decides it.
A decision promotes its entry into the owning specification and removes it here.

| # | Question | Recommendation | Decided by |
| --- | --- | --- | --- |
| 1 | The license of this repository | Apache-2.0 (the patent grant, for a schema-language derivative) | before the first public push |
| 2 | The corpus scale envelope (documents, child rows) and the p95 latency budget of 40-ops | ≤10k documents / ≤100k child rows; hundreds of milliseconds per command | with the first codec benchmarks at T2 |
| 3 | Children of a suffixed item (`T8.0.14-1.3`) | allow — restated as a proposition in the canonical format; confirm with use | the first renumber codemod |
| 4 | Frontmatter comment support | keep rejected in v1; revisit only when a requirement names it | a requirement that names it |
| 5 | The MCP tool list and the per-call surface of the three faces | a tool per model operation plus a board subscription; one operation per call under the portable lock | the T7 mini-spec |
| 6 | The orm-side ordering of the manifest-setting extensions (markdown traits, checklist sugar, the state-machine sub-grammar) | land the state machine first — the checklist application needs it at T8 | the T5 plan with orm |
| 7 | Re-vendor kit after the user tags the next kit release | `make kit-sync KIT_TAG=<tag>` replaces the local-path lock record | the kit release tag |

Two standing notes. The design plan stays the ruling record until each specification absorbs its
part; when they disagree, the specification wins and the plan gets the correction. And the
open-question list itself is reviewed at the close of every phase — an entry that no longer has
a deciding condition is decided or dropped, never left to rot.
