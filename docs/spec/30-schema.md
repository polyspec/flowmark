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
  actions; an external `use` document; a table without a `store` setting; a `codec` whose last stage
  is `yaml` — orm's encoder (go-yaml's `Marshal`) ends every value with a line feed, which no place of
  the format holds, while a later stage encodes it away. A view table is declared by the
  [application schema](100-checklist.md#views), not by a setting.
- **Allowed**: `identity` only on an `i64` primary key column of a markdown table, allocated `max+1`
  under the lock. dbspec refuses `identity` on any other type (`an identity column has type i64`), so an
  `i16` or `i32` key cannot take it either. Grounding: `max+1` is an integer sequence, and a `bool`,
  `varchar`, `uuid` or `date` key has no maximum to add one to.
- **Required**: a table with a `state_machine` declares its `history` and at least one `initial`
  state; a `list` or `files` storage declares `title`; a `list` storage of a state machine
  declares at least one `checkbox`, and dbspec then requires one for every state; the catalog rule is
  stated here because dbspec accepts a state machine with no checkbox at all.
- **Key types.** The one-column primary key of a markdown table is an integer type (`i16`, `i32`,
  `i64`), `bool`, `varchar`, `uuid` or `date`; any other type is a schema error, located at the table
  name. Grounding: a key is a file name and an anchor, so its text must hold only the key shape
  ([document model](20-document-model.md#keys)). A `date` is written `YYYY-MM-DD` with a year from 0001 to 9999, the range orm's date literal rule accepts, so its text has four digits and no sign, and its `-` only separates. The ordinary values of `time` and `datetime` hold `:`. The non-integral values of a `decimal` of scale above 0 and of an `f64` hold `.`, and a negative value of any of these types begins with `-`, so they can never meet the shape. A `decimal` of scale 0 holds only integers, which an integer type keys, so it is refused with the other decimals; an `f64`'s whole values are likewise integers that an integer type keys, so an `f64` key adds only values the shape refuses. `text` and `bytes` are refused by dbspec's own
  key rule. A negative integer key is refused per value, not per type ([document model](20-document-model.md#keys)).
- **Foreign keys.** Every foreign key of a table is one column that references the one-column
  primary key of a table. A composite foreign key, or one that references a unique non-key
  column, is a schema error, located at the foreign key. Grounding: a foreign key is written as a
  link to the referenced row, and a row has a link only through its one-column key.
- **Block placement.** The column of a block table's placement foreign key — the foreign key its
  `store block <fk>` setting names — is non-null; a nullable one is a schema error, located at the
  block table's name. Grounding: a block row stands inside the scope of the row its placement
  foreign key references ([document model](20-document-model.md#storage)), and NULL references
  none, so a row holding NULL there would have no place to be written. dbspec accepts a nullable
  foreign key column, since an SQL row needs no place; the catalog rule is stated here. It holds for
  every block table, history tables included ([The history table](#the-history-table)).

## Name rules

- **Reserved table names.** A table whose name equals, without regard to case, `CON`, `PRN`, `AUX`,
  `NUL`, `COM0`–`COM9` or `LPT0`–`LPT9` is a schema error: these are reserved file names on Windows.
- **A child table named like a markdown column** of its parent is a schema error.
- **Heading depth.** A layout that needs a heading past H6 is a schema error, decided when the
  catalog loads. A table's rows write their sections at their row scope level when the table can
  have a markdown column other than the body or a child block, and their detail heading one level
  above that scope when it can have a body; a table with neither writes no heading below its block
  heading, so its row scope may lie past H6. Grounding: a heading the layout never writes is not one
  the layout needs; a state machine's history table without a markdown column, as when its machine
  requires no column or only columns that neither table declares `markdown`, has no detail section, and it stands at level 7 under a block row.

## The state set

- A machine's states are the states its `initial`, transition and `terminal` lines name.
  When a table declares a `checkbox`, `checkbox` covers that set exactly with distinct glyphs; `checkbox`, `limit` or `initial` naming
  another state is a setting error.
- A `default` of the state column is an initial state, otherwise a setting error. An initial state
  that is also terminal is a setting error.
- The state column of a table is the column its `state_machine` setting names. It is non-null, in every
  storage: dbspec refuses a nullable state column of a state machine. Grounding: a row's state is its place in
  the machine, and NULL names no place; dbspec refuses it.

## The history table

The history setting names the history table's columns: the `<fk column>` has a foreign key to the
state table with its leading index; `from` and `to` have the state column's type; `at` is
`datetime(6)`; and one nullable column per column that a `require` list names has that column's
name and type. The table has no other column besides its `identity` key and declares no `title`
or `body`; any mismatch is a setting error at the history table's name. It is stored
`block <fk> table` under its state table, or `document table`; another storage of a history table is a setting error at the history table's name. Grounding: the history rules — one row per transition, the gap and chain diagnostics, the repair and the restrict delete ([constraints](50-constraints.md#the-state-machine)) — are stated over the rows and their foreign key, never over their place, so the storage is the schema's choice as for any table; the other storages are excluded: `files` and a `list` declare a `title`, which the history table does not; and a block row stands in the scope of the row its placement foreign key names ([document model](20-document-model.md#storage)), so a history block stands under its own state row only when that key is its row column. A block placed by any other column — `from` or `to` under a table keyed by the states, the identity key under a table keyed like it, a copied column, under the state table or under another — stands in the scope of a row other than the one whose trail it records, a row chosen by a state, by a key the insert allocates or by a copied value, never by the transition's row. Such a block pays the depth of a block and loses its locality: the executor writes a transition's history row into its state row's block and the validator reads a row's trail — its gap, chain, transition and required rules — from that block ([constraints](50-constraints.md#a-state-changed-outside-the-engine)), and a trail spread over the scopes of other rows is read with none of them; the `document table` storage already keeps a trail apart from its rows, in one place. So any other placement is the setting error just stated, whatever the nullability of its key; a copied column, nullable and NULL on every transition whose line does not require it, is refused by the block placement rule too ([Block placement](#corpus-rules-on-dbspec-constructs)), and so is a block under the state table whose row column is nullable. The two trade locality against depth. A block keeps a row's trail in the row's detail section — in the row's own file for a `files` table — so the row and its trail are read together and transitions of different rows touch different places; but every row with history has a detail section, and the block stands two levels below the state rows. A document table keeps the table's trail in `<history>.flowmark.md`, whose rows have row scope 3 at any depth of the state table, so a required `markdown` column always has a section; but every transition of the table appends to that one file. Neither cost holds for every corpus, so neither storage is mandated.

A column that a `require` list names is declared `markdown` on the history table exactly when it is declared `markdown` on the state table; a mismatch is a schema error at the history table's name. Grounding: a history row holds a copy of the required columns ([constraints](50-constraints.md#the-state-machine)), and a section and a single-line place accept different values — a multi-line value or raw HTML is a section's text but is refused in a cell, and a line `# x` is a cell's text but is refused in a section ([canonical format](10-canonical-format.md#single-line-places)) — so a mismatch would refuse a declared transition for a value the state table holds. dbspec accepts the mismatch, since the SQL backends ignore `markdown`; the catalog rule is stated here. When the history table is stored as a block, its rows stand two levels below the state table's rows, so its row scope is the state table's row scope plus two. A required `markdown` column is a section of the history rows at that scope; when the state table's row scope is 5 or deeper, that section would stand past H6, so the layout is a schema error at the history table's name ([Heading depth](#name-rules)); stored `document table`, the history rows have row scope 3 at any depth of the state table, so such a machine loads; a required column that neither table declares `markdown` is a cell, needs no heading, and is allowed at any depth. Grounding: a `markdown` value is a section in every storage ([canonical format](10-canonical-format.md#sections-and-their-values)), and CommonMark has no heading past H6, so such a value has no place to be written; writing it as a cell would change the column's form with its table's depth, and a cell cannot hold a line feed; writing its section at H6 would put it at the level of the history rows' detail headings, where a heading other than `<table> row <key>` ends the row scope and its content belongs to no row ([canonical format](10-canonical-format.md#detail-headings-and-anchors)).

## Install

`flowmark install <corpus>` generates the storage skeleton from the catalog — the folder of each
`files` table and the file `<table>.flowmark.md` of each `document` table, and no block section,
since an empty block is not written — as `CREATE TABLE` creates storage. `flowmark install` adds the line `*.flowmark.md text eol=lf` to `<corpus>/.gitattributes`, creating the file when it is missing and keeping its other lines, so a git checkout keeps LF on every platform ([canonical format](10-canonical-format.md#encoding-and-line-structure)).
Schema install through orm
over `markdown://` is `CAPABILITY_UNSUPPORTED`.

## Introspection

Introspection validates every document against the catalog and returns the catalog; a document
that fails is a located diagnostic. A corpus without a catalog has no schema to return; a tool
that drafts one is not part of this specification.

- The catalog returned is the `dbspec/` document set: each document in dbspec's canonical form
  (orm `docs/dbspec.md`, Canonical form), with its corpus path and its document name, in file name
  order, and the set's manifest hash. Grounding: the catalog is the only origin of the schema, so
  introspection returns it and nothing derived from the rows; orm's introspection of a database
  returns a dbspec document, and canonical form is dbspec's own form of one — it keeps every
  declaration, the markdown settings and the comments, so the returned set reads back to the same
  manifest hash; the manifest hash is what a client's generated code is checked against
  (`SCHEMA_HASH_MISMATCH`), so a tool tells from it whether its code matches the corpus. The
  catalog is read with orm's `engine/dbspec` and written with its `Emit`, never by a second
  parser or emitter.
- The diagnostics are every diagnostic of the validator ([constraints](50-constraints.md#diagnostics)),
  sorted by file, line, column, rule and message, and the catalog is returned with them whenever it
  loads, whether or not a document fails. Grounding: rows never change the schema, so a document
  that fails says nothing about the catalog, as a database's introspection returns its schema
  whatever its rows hold; withholding the catalog would hide the schema from the person who repairs
  the document. The corpus is valid exactly when there is no diagnostic.
- A catalog that does not load returns no catalog and only its `catalog` diagnostics: the validator
  reads the documents through the catalog and cannot run without it. A corpus without `dbspec/` is
  the `catalog` diagnostic `no dbspec/ directory` at `dbspec`.
- The command line is `flowmark introspect <corpus>`, `<corpus>` being the corpus root. It prints
  one JSON object on standard output, `{"catalog": {"manifest_hash", "documents": [{"file",
  "name", "text"}]}, "diagnostics": [{"file", "line", "column", "rule", "message"}]}`, with
  `"catalog": null` when the catalog does not load and `"diagnostics": []` when there is none. It
  exits 0 when there is no diagnostic; 1 when there is one, the object printed all the same, or
  when the command failed, with the failure on standard error and nothing on standard output; and
  2 for a wrong command line, as [canon](40-ops.md#the-canon-operation) does. Grounding: a
  document's text spans lines and the answer is read by tools, so it is one JSON value, as the
  messages of the IR-executor protocol are; introspection validates, so a diagnostic fails the
  command as a failed validation does.
