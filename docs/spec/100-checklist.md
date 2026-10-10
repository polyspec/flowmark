<!-- doc-id: spec-checklist -->
# The application schema

[한국어](100-checklist.ko.md)

The application schema is `flowmark.dbs`, the standard catalog document that flowmark provides for
issue tracking: the items, the people, the links, the history and the views that the boards read.
A corpus extends it by adding columns. This specification pins its tables and its state machine.

## Tables

| Table | Storage | Columns and settings |
| --- | --- | --- |
| `items` | `store document list` | `id varchar(32)` with `key_prefix 'T'`; `parent varchar(32) null` → items; `state varchar(16)`; `title varchar(255)` with `title title`; `body text` with `markdown body` and `body body`; `assignee varchar(32) null` → people; `start_on date null`, `start_at datetime(0) null`, `due_on date null`, `due_at datetime(0) null`; `priority i16 null`; `position i32` with `order position`; `evidence text null`, `cause text null`, `retry text null`, each `markdown` |
| `people` | `store document table` | `id varchar(32)` with `key_prefix 'P'`; `name varchar(255)` with `title name` |
| `item_links` | `store document table` | `id i64 identity`; `from_item varchar(32)` → items; `to_item varchar(32)` → items; `kind varchar(16)` |
| `item_history` | `store block fk_history_item table` | `id i64 identity`; `item varchar(32)` → items by `fk_history_item`; `from_state`, `to_state` as `state`; `at datetime(6)`; `evidence`, `cause`, `retry` as on items, each `markdown` |
| `views` | `store document table` | `id i64 identity`; `title varchar(255)` with `title title`; `body text` with `markdown body` and `body body`; `kind varchar(16)`; `entity varchar(63)`; `filter text` and `sort text` with `codec filter ordered_json` and `codec sort ordered_json`; `group_by varchar(63) null` |

Every foreign key has its leading index. The checks are `start_on is null or start_at is null`,
`due_on is null or due_at is null`,
`ck_item_links_kind (kind in ('blocks', 'relates', 'duplicates', 'supersedes'))` and
`ck_views_kind (kind in ('kanban', 'list', 'gantt'))`.

- **Two columns per point in time** keep the ruling that a due may carry a time: no dbspec type
  holds "date or datetime", and a datetime-only column would turn a date-only due into an ambiguous
  midnight. A gantt draws a date-only start from the start of its day and a date-only due through
  the end of its day.
- **A blank assignee is self**; no name is ever guessed for it.
- **A gantt dependency edge** is an `item_links` row of kind `blocks`.

## The state machine

| Line | Setting |
| --- | --- |
| initial | `state_machine state initial waiting` |
| waiting → doing | — |
| doing → done | `require (evidence)` |
| doing → bypassed | `require (cause, retry)` |
| bypassed → doing | — (resume when the retry condition holds; no approval wait) |
| waiting → impossible | `require (cause)` |
| terminal | `done`, `impossible` |
| checkbox | `waiting '[ ]'`, `doing '[~]'`, `done '[o]'`, `bypassed '[!]'`, `impossible '[-]'` |
| history | `history item_history row item from from_state to to_state at at` |

- A done item is never reopened: a new issue related to it is a new item with an `item_links` row
  of kind `supersedes`.
- **No WIP limit is declared.** The owner's rule — completion before starting, with independent
  items allowed in parallel — is a working rule that no count expresses, so a number would be
  invented; the engine enforces no WIP rule. A corpus that wants a count declares
  `state_machine state limit doing <n>`.

## Views

- A view is a row of `views`. `filter` holds an IR `Group` and `sort` an IR `Order` list, validated
  with `ir.Validate` against the runtime model on write and on validate; a view made invalid by a
  catalog change is a located diagnostic. A filter saved in a GUI never edits the catalog.
- A kanban, list or gantt view requires its entity table to declare `title` and `body`.
- A kanban drop sets `order` only when the view has no `sort` (storage order) and both neighbours
  share the card's parent; a drop across columns changes the state. Any other drop is refused and
  the board says why — it never moves a card it does not store. After an accepted drop the board
  redraws from the rows the operation returns, never from the drop position. A drop across columns
  places the card where the view's sort puts it; without a `sort`, the state change keeps the card's
  `order`.
- No view has a write path of its own: an illegal drag fails the same validation as an illegal CLI
  write.

## This repository's checklists

This repository is not a corpus. Its checklists stay kit-tracked documents managed by hand; the
application schema serves the corpora that flowmark runs, not the repository that builds it.
