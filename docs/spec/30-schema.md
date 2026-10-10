<!-- doc-id: spec-schema -->
# Schema

[한국어](30-schema.ko.md)

This specification pins the schema of a corpus: the catalog, the markdown settings, the corpus
rules on dbspec constructs, the name rules, and the catalog's two operations — install and
introspection.

## The catalog

- The `dbspec/*.dbs` document set of a corpus root is the catalog: the only origin of the schema,
  as a database catalog is. Documents carry no schema copy and no stamp — a database keeps its
  schema once and never per row, and a copy per document makes the drift of the copies a bug
  class of its own.
- flowmark reads the catalog with orm's Go `engine/dbspec` package and uses its manifest hash; it
  never reimplements the grammar. `use` lines resolve within the `dbspec/` set; an external
  document is a schema error.
- `dbspec/` holds only the `.dbs` set. Another file in `dbspec/` is a `catalog` diagnostic.
- The corpus setting file `dbspec/flowmark.json` and its `translations` language list are
  withdrawn: the catalog holds every declaration, and no file outside the `.dbs` set is read.
- Standalone documents with an embedded schema fragment do not exist: a corpus is a root with a
  catalog.

## Markdown settings

Every markdown-specific declaration is a dbspec **manifest setting**, after the precedent of the
`markdown <column>` setting: the SQL backends ignore it, introspection of a database returns a
plain column, and the setting names the columns it applies to — a column's name never selects a
behaviour.

| Setting | Meaning |
| --- | --- |
| `store files` / `store document list\|table` / `store block <fk> list\|table` | the table's storage ([document model](20-document-model.md)); exactly one per table on a corpus |
| `key_prefix <p>` | the `varchar` primary key is allocated `<p><n + 1>`; orm treats the column as executor-assigned — a `varchar`, as dbspec keys no `text` column |
| `title <column>` | the row's title: non-null, never empty |
| `body <column>` | the row's body: a `markdown` column, never NULL |
| `order <column>` | the row's position among its siblings; orm treats the column as executor-assigned |
| `checkbox <column> <state> '<glyph>'` | the list glyph of a state: exactly one character, written between the brackets of a list row (`- [<glyph>] …`) |
| `markdown <column>` | the column holds markdown prose and is stored as a section |
| `state_machine <column> initial <state>` | an initial state; may repeat |
| `state_machine <column> <from> -> <to> [require (…)]`, `state_machine <column> terminal <state> [require (…)]` | the transitions and terminals, as dbspec defines them |
| `state_machine <column> history <table> row <fk column> from <column> to <column> at <column>` | the history table and its named columns |
| `state_machine <column> limit <state> <n>` | at most n rows in the state |

## Corpus rules on dbspec constructs

- **Rejected** on a corpus, each a schema error: a composite primary key; `aes`, `aes_version` and
  `blind_index` (the key never leaves the client); `audit` and `immutable` (markdown has no
  triggers; `state_machine … history` records transitions instead); `cascade` and `set_null`
  actions; an external `use` document; a table without a `store` setting.
- **Allowed**: `identity` on every markdown table, allocated `max+1` under the lock.
- **Required**: a table with a `state_machine` declares its `history` and at least one `initial`
  state; a `list` or `files` storage declares `title`; a `list` storage with a state column
  declares a `checkbox` for every state.
- **Foreign keys.** Every foreign key of a table is one column that references the one-column
  primary key of a table. A composite foreign key, or one that references a unique non-key
  column, is a schema error, located at the foreign key. Grounding: a foreign key is written as a
  link to the referenced row, and a row has a link only through its one-column key.

## Name rules

- **Reserved table names.** A table whose name equals, without regard to case, `CON`, `PRN`, `AUX`,
  `NUL`, `COM0`–`COM9` or `LPT0`–`LPT9` is a schema error: these are reserved file names on Windows.
- **A child table named like a markdown column** of its parent is a schema error.
- **Heading depth.** A layout that needs a heading past H6 is a schema error, decided when the
  catalog loads.

## The state set

- A machine's states are the states its `initial`, transition and `terminal` lines name.
  `checkbox` covers that set exactly with distinct glyphs; `checkbox`, `limit` or `initial` naming
  another state is a setting error.
- A `default` of the state column is an initial state, otherwise a setting error. An initial state
  that is also terminal is a setting error.
- The state column of a table is the column its `state_machine` setting names.

## The history table

The history setting names the history table's columns: the `<fk column>` has a foreign key to the
state table with its leading index; `from` and `to` have the state column's type; `at` is
`datetime(6)`; and one nullable column per column that a `require` list names has that column's
name and type. The table has no other column besides its `identity` key and declares no `title`
or `body`; any mismatch is a setting error at the history table's name. It is stored
`block <fk> table`.

## Install

`flowmark install <corpus>` generates the storage skeleton from the catalog — the folder of each
`files` table and the file `<table>.flowmark.md` of each `document` table, and no block section,
since an empty block is not written — as `CREATE TABLE` creates storage. Schema install through orm
over `markdown://` is `CAPABILITY_UNSUPPORTED`.

## Introspection

Introspection validates every document against the catalog and returns the catalog; a document
that fails is a located diagnostic. A corpus without a catalog has no schema to return; a tool
that drafts one is not part of this specification.
