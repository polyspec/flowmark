<!-- doc-id: spec-checklist -->
# The checklist application

[한국어](100-checklist.ko.md)

The checklist application is the first application on the corpus: the engine this repository
runs on, and the local issue model the boards read. This specification pins its tables, its
state machine and its kit alignment.

## Tables

| Table | Columns ( essentials ) |
| --- | --- |
| `checklists` | `id` (PK), `title`, `version`, `created`, `updated` |
| `checklist_items` | `id` (PK, the task-ID grammar), `checklist_id` (FK), `parent_id` (self FK, nullable), `status`, `title`, `order`, `supersedes_id` (FK, nullable), `created`, `updated`, `closed` |
| `item_dependencies` | `item_id` (FK), `depends_on_id` (FK), `kind` (`prerequisite` or `follow-up`) |
| `item_revisions` | append-only: `id`, `item_id` (FK), `from_status`, `to_status`, `reason`, `evidence`, `at` |
| `changelogs` | the projection of `item_revisions` — generated, never hand-written |

One checklist per repository. The transition to a new checklist item follows the application
rules: a new issue related to a completed item spawns a suffixed item (`T8.0.14-1`) with
`supersedes_id`; the completed item stays terminal.

## The state machine

| Transition | Required fields |
| --- | --- |
| waiting → doing | — |
| doing → done | evidence |
| doing → bypassed | cause, retry condition |
| bypassed → doing | — (resume when the retry condition holds; no approval wait) |
| waiting → impossible | cause |
| done → (any) | refused — terminal; spawn a superseded item instead |

- The five states map one-to-one to the kit states `[ ]` `[~]` `[o]` `[!]` `[-]`, and the kit
  strict rules — a bypass needs `cause:` and `retry:`, an impossible item needs `cause:` — are
  the same requirements the validator enforces from the table above.
- **The WIP gate is an application rule**: starting a new item is refused while the number of
  `doing` items grows without completed results, unless the user explicitly requests the
  interruption. Parallel independent work is allowed.
- A board (the kanban view) is one more consumer: the columns are the five states, the cards are
  `checklist_items`, the edges are `item_dependencies`. An illegal drag fails the same
  transition validation as an illegal CLI call, and the WIP gate is a column limit. No
  view-specific write path exists.

## Commit discipline (encoded)

- An item becomes done in the commit that completes it, after its owning command passes; the
  same commit writes the changelog entries.
- `item_revisions` is append-only and derives the changelog; the projection rebuilds from
  on-file data.
- The commit message convention and the red/green discipline live in AGENTS.md, not here — this
  specification defines the data model that enforces them.

## Kit alignment and the cutover

- Until the cutover, kit reads the checklist documents with its own grammar and the reference
  check (`make refs-check`) covers the dangling-reference class.
- The cutover is one transaction ([migration](80-migration.md)): the tracker leaves
  `config/checklist.json`, the documents convert to corpus format, and `flowmark validate`
  passes in the same commit.
