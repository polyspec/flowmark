<!-- doc-id: spec-open-questions -->
# Open questions

[한국어](110-open-questions.ko.md)

Each entry states the question, the current recommendation and the condition that decides it.
A decision promotes its entry into the owning specification and removes it here.

| # | Question | Recommendation | Decided by |
| --- | --- | --- | --- |
| 1 | The license of this repository | Apache-2.0 (the patent grant, for a schema-language derivative) | before the first public push |
| 2 | The corpus scale envelope (documents, rows) and the p95 latency budget of 40-ops | ≤10k documents / ≤100k rows; hundreds of milliseconds per command | the first engine benchmarks at T12.4 |
| 3 | The order of the orm items that T12 requires | the protocol's IR executors and `WRITE_CONFLICT` first, then the markdown settings — the settings item needs the executor-assigned columns that the protocol item defines | the orm checklist entries |
| 4 | Re-vendor kit after the user tags the next kit release | `make kit-sync KIT_TAG=<tag>` replaces the local-path lock record | the kit release tag |
| 5 | Where the sync provenance of an adapter-owned document lives, now that a document carries no comment | None recorded; the adapter work decides it | the first adapter beyond the database export |
| 6 | The orm surfaces the database export waits on ([sync](70-sync.md#the-export)) | orm items for: an insert that keeps a stored identity value, with the statement that restarts a PostgreSQL identity past it; a public binding of a plan's parameters to a database driver, which orm's Go client keeps private; and a SQLite `datetime(p)` bind with p fraction digits, since its executor binds six while its renderer's CHECK requires p | the orm checklist entries |

Withdrawn with the markdown database plan: children of a suffixed item (keys are opaque, and the
suffix grammar is removed), and the MCP tool list (the faces operate over any table, T12.10).
Withdrawn with the readable-document ruling: the frontmatter comment question (a corpus document has
no frontmatter), and the `<!-- doc-id -->` and provenance header comments of a document.

Two standing notes. The design plan and the markdown database plan stay the ruling record until
each specification absorbs its part; when they disagree, the specification wins and the plan gets
the correction. And the open-question list itself is reviewed at the close of every phase — an
entry that no longer has a deciding condition is decided or dropped, never left to rot.
