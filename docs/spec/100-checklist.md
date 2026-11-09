<!-- doc-id: spec-checklist -->
# The application schema

[한국어](100-checklist.ko.md)

The application schema is `flowmark.dbs`, the standard catalog document that flowmark provides for
issue tracking: the items, the people, the links and the views that the boards read.
A corpus extends it by adding columns. This specification pins its tables and its states.

## Tables

| Table | Storage | Columns and settings |
| --- | --- | --- |
| `items` | `store files`, one file `items/<key>.flowmark.md` an item | `id varchar(32)`; `parent varchar(32) null` → items; `state varchar(16)`; `title varchar(255)` with `title title`; `body text` with `markdown body` and `body body`; `assignee varchar(32) null` → people; `start_on date null`, `start_at datetime(0) null`, `due_on date null`, `due_at datetime(0) null`; `priority i16 null`; `position i32` with `order position`; `evidence text null`, `cause text null`, `retry text null`, each `markdown` |
| `people` | `store document table`, file `people.flowmark.md` | `id varchar(32)`; `name varchar(255)` with `title name` |
| `item_links` | `store block fk_item_links_from table`, in the file of its `from_item` | `id i64 identity`; `from_item varchar(32)` → items; `to_item varchar(32)` → items; `kind varchar(16)` |
| `views` | `store document table`, file `views.flowmark.md` | `id i64 identity`; `title varchar(255)` with `title title`; `body text` with `markdown body` and `body body`; `kind varchar(16)`; `entity varchar(63)`; `filter text` and `sort text` with `codec filter ordered_json` and `codec sort ordered_json`; `group_by varchar(63) null` |

Every foreign key has its leading index. The checks are
`ck_items_state (state in ('waiting', 'doing', 'done', 'bypassed', 'impossible'))`,
`start_on is null or start_at is null`,
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

- An item is its own file `items/<key>.flowmark.md`: its title as the H1, its fields, its body,
  its `markdown` columns, then the block `## item_links` of the links it starts, as the [document model](20-document-model.md#storage)
  stores a `files` row and its child blocks. The rows of `people` and of `views` are the rows of
  one document file each, `people.flowmark.md` and `views.flowmark.md`. The keys follow the key
  rules of the document model: `T1` and `P1` are keys of the `items` and `people` tables.
- **No file grows with the life of the project.** A write to an item, a link the item starts and
  a move of the item write that item's file alone, so links of different items touch different
  files, and an item's outgoing links are read with the item. The two document files are bounded
  by the team and by its saved views, which do not grow with the work done. Grounding: a document
  file holding every item and its links would be parsed and rewritten whole on every write
  ([cost model](40-ops.md#cost-model)), so its cost and its git merge conflicts would grow with the
  project. The size of each file is observed, never assumed:
  the envelope a file is measured against and the diagnostic past it are `T12.5-3`'s.
- `item_links` is placed by `from_item` (`store block fk_item_links_from table`): a link belongs to
  the item it starts from, its `to_item` and `kind` are the cells of the block, and the
  `from_item` of a block row is its placement, never a cell. The incoming links of an item are
  the rows whose `to_item` names it, read by a query over the table as any foreign key is.
- `position` orders the items with the same `parent`: they stand by ascending value, those that
  share one in key order ([document model](20-document-model.md#order)). The application writes it
  as any integer, and a move writes the moved item's file alone; an insert gives it, as it gives
  the item's key.
- A foreign key is a link, as the [canonical format](10-canonical-format.md) states. The field
  of an item with the assignee `P1` is written `- assignee: [P1](../people.flowmark.md)`: the
  person row has no detail section, so the target has no anchor. A `to_item` cell of a link block
  links the item's file, `[T3](T3.flowmark.md)`, and a `parent` field links its parent's file the
  same way.
- A multi-line evidence is a section: `items/T1.flowmark.md` with the item's title as its H1,
  `## evidence`. `evidence`, `cause` and `retry` are `markdown` columns of `items`, stored as sections
  of the item's file; adding a column never changes the storage of `items`.

## The states

`state` holds one of five values — `waiting`, `doing`, `done`, `bypassed` and `impossible` — and the check
`ck_items_state` constrains it to them as any database constrains a column; another value is refused as any check refuses
it ([constraints](50-constraints.md#state-columns)).

- The standard storage writes the state as the field `- state: doing` of the item's file. The schema declares no
  `task_marker`: `items` is stored `files`, where a task marker shows nothing, and its state is a word.
- A done item is not reopened, by convention: a new issue related to it is a new item with an `item_links` row of kind
  `supersedes`. The engine enforces nothing of it: the transitions of a state and their trail are polyspec fsm's.
- **No WIP limit.** The owner's rule — completion before starting, with independent items allowed in parallel — is a
  working rule that no count expresses; the engine enforces no WIP rule.

## Views

- The view table is the table `views` of the catalog document named `flowmark` — this application
  schema — and no manifest setting declares it. Its view columns are `kind` and `entity`, each a
  non-null `varchar` or `text` without a codec; `filter` and `sort`, each a non-null `varchar` or
  `text` with exactly `codec <column> ordered_json`; and `group_by`, a `varchar` or `text` without a
  codec. Another shape is a catalog error at the table name; a catalog without a document named
  `flowmark`, or whose `flowmark` document declares no `views`, has no view table. Grounding: a view
  means nothing to the SQL databases, and orm adopts a setting only when it renders with the same
  meaning on all three, so orm declares no view table; a view is application data that the boards
  read, so the application that defines views names their table, as an application binds to the
  tables its own schema declares. The schema's rule that a column's name selects no behaviour
  ([schema](30-schema.md#markdown-settings)) governs the storage settings, which apply to every
  catalog; this declaration names one table of the one document this specification owns.
- A view is a row of `views`. `filter` holds `{"where": <IR Group>, "params": [<value>, …]}`, no
  filter being `{"where":{"items":[]},"params":[]}`, and `sort` an IR `Order` list, `[]` for no sort;
  `group_by` names the field the view groups by, NULL for none. They are validated with
  `ir.Validate` against the runtime model on write and on validate, as the
  [constraints](50-constraints.md#views) state; a view made invalid by a catalog change is a located
  diagnostic. A filter saved in a GUI never edits the catalog.
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
