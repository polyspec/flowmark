<!-- doc-id: spec-checklist -->
# The application schema

[한국어](100-checklist.ko.md)

The application schema is `flowmark.dbs`, the standard catalog document that flowmark provides for
issue tracking: the items, the people, the links, the history and the views that the boards read.
A corpus extends it by adding columns. This specification pins its tables and its state machine.

## Tables

| Table | Storage | Columns and settings |
| --- | --- | --- |
| `items` | `store document list`, file `items.flowmark.md` | `id varchar(32)` with `key_prefix 'T'`; `parent varchar(32) null` → items; `state varchar(16)`; `title varchar(255)` with `title title`; `body text` with `markdown body` and `body body`; `assignee varchar(32) null` → people; `start_on date null`, `start_at datetime(0) null`, `due_on date null`, `due_at datetime(0) null`; `priority i16 null`; `position i32` with `order position`; `evidence text null`, `cause text null`, `retry text null`, each `markdown` |
| `people` | `store document table`, file `people.flowmark.md` | `id varchar(32)` with `key_prefix 'P'`; `name varchar(255)` with `title name` |
| `item_links` | `store document table`, file `item_links.flowmark.md` | `id i64 identity`; `from_item varchar(32)` → items; `to_item varchar(32)` → items; `kind varchar(16)` |
| `item_history` | `store block fk_history_item table`, inside the detail section of its item | `id i64 identity`; `item varchar(32)` → items by `fk_history_item`; `from_state`, `to_state` as `state`; `at datetime(6)`; `evidence`, `cause`, `retry` as on items, each `markdown` |
| `views` | `store document table`, file `views.flowmark.md` | `id i64 identity`; `title varchar(255)` with `title title`; `body text` with `markdown body` and `body body`; `kind varchar(16)`; `entity varchar(63)`; `filter text` and `sort text` with `codec filter ordered_json` and `codec sort ordered_json`; `group_by varchar(63) null` |

Every foreign key has its leading index. The checks are `start_on is null or start_at is null`,
`due_on is null or due_at is null`,
`ck_item_links_kind (kind in ('blocks', 'relates', 'duplicates', 'supersedes'))` and
`ck_views_kind (kind in ('kanban', 'list', 'gantt'))`.

- **Two columns per point in time** keep the ruling that a due may carry a time: no dbspec type
  holds "date or datetime", and a datetime-only column would turn a date-only due into an ambiguous
  midnight. A gantt draws a date-only start from the start of its day and a date-only due through
  the end of its day.
- **A NULL assignee is self**; no name is ever guessed for it.
- **A gantt dependency edge** is an `item_links` row of kind `blocks`.

## Files and links

- The rows of `items`, `people`, `item_links` and `views` are the rows of one document file each,
  named by the table; the `item_history` rows are block rows inside the detail section of their
  item. The keys follow the key rules of the [document model](20-document-model.md): `T1` and `P1`
  are keys of the `items` and `people` tables.
- A foreign key is a link, as the [canonical format](10-canonical-format.md) states. A list row of
  `items` with the assignee `P1`, in progress, is written
  `- [~] T1 Write the spec assignee:[P1](people.flowmark.md)`: the person row has no detail section,
  so the target has no anchor.
- In the table cells of `item_links`, `from_item` and `to_item` are links such as
  `[T1](items.flowmark.md#items-t1)`, the anchor present exactly when the item has a detail section.
- A multi-line evidence is a section: `items.flowmark.md` H1, `## items T1`, `### item_history`, `#### item_history <id>`, `##### evidence` — within H6.

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
| checkbox | `waiting ' '`, `doing '~'`, `done 'o'`, `bypassed '!'`, `impossible '-'` |
| history | `history item_history row item from from_state to to_state at at` |

- A checkbox glyph is exactly one character, written between the brackets of a list row: a list
  row writes the glyph between brackets, so `waiting` reads `- [ ] T1 …`, `doing` reads `- [~] T1 …`
  and `done` reads `- [o] T1 …`.
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
- A kanban, list or gantt view requires its entity table to declare `title` and `body`;
  `flowmark.dbs` declares both on `items`.
- A kanban drop sets `order` only when the view has no `sort` (storage order) and both neighbours
  share the card's parent; a drop across columns changes the state. Any other drop is refused and
  the board says why — it never moves a card it does not store. After an accepted drop the board
  redraws from the rows the operation returns, never from the drop position. `order` is set only by a
  same-parent drop in storage order; a cross-column drop is a state change only, so the card keeps
  its `order` and shows where the view's sort — or, without one, storage order — puts it.
- No view has a write path of its own: an illegal drag fails the same validation as an illegal CLI
  write.

## This repository's checklists

This repository is not a corpus. Its checklists stay kit-tracked documents managed by hand; the
application schema serves the corpora that flowmark runs, not the repository that builds it.
