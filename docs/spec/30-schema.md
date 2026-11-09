<!-- doc-id: spec-schema -->
# Schema

[한국어](30-schema.ko.md)

This specification pins the schema of a corpus: the catalog, the markdown settings, the corpus
rules on dbspec constructs, the name rules, and the catalog's two operations — install and
introspection.

## The catalog

- The document set that `dbspec/dbspec-set.json`, orm's set file (orm `docs/dbspec.md`, Document sets), declares
  is the catalog of a corpus root: the only origin of the schema,
  as a database catalog is. Documents carry no schema copy and no stamp — a database keeps its
  schema once and never per row, and a copy per document makes the drift of the copies a bug
  class of its own.
- flowmark reads the catalog with orm's Go `engine/dbspec` package and uses its manifest hash; it
  never reimplements the grammar. `use` lines resolve within the `dbspec/` set; an external
  document is a schema error.
- `dbspec/` holds the set file and the `.dbs` documents it lists, each directly in `dbspec/`. flowmark reads the set
  file with orm's set reader, so its rules are orm's and its errors are `catalog` diagnostics at the set file. Every
  declaration of the set is checked, and none is dropped:
  - a corpus without `dbspec/dbspec-set.json` is the `catalog` diagnostic `no dbspec/dbspec-set.json`;
  - a `.dbs` file in `dbspec/` that the set file does not list is a `catalog` diagnostic naming it;
  - a listed document outside `dbspec/`, or one in a folder of it, is a `catalog` diagnostic naming the path;
  - an `external` entry is a `catalog` diagnostic: the catalog uses no document outside its set;
  - another file in `dbspec/` is a `catalog` diagnostic.
  Grounding: orm's generators and language server read the same set file, so one declaration names the set for every
  tool; a directory form beside it would be a second declaration that drifts.
- The corpus setting file `dbspec/flowmark.json` and its `translations` language list are
  withdrawn: the catalog holds every declaration, and no file outside the set file and its documents is read.
- Standalone documents with an embedded schema fragment do not exist: a corpus is a root with a
  catalog.

## Markdown settings

Every markdown-specific declaration is a dbspec **manifest setting**, after the precedent of the
`markdown <column>` setting: the SQL backends ignore it, introspection of a database returns a
plain column, and the setting names the columns it applies to — a column's name never selects a
behaviour.

| Setting | Meaning |
| --- | --- |
| `store files` / `store document list\|table` / `store block <fk> list\|table` | the table's storage ([document model](20-document-model.md)); exactly one per table on a corpus. A `files` or `document` store may end in `shard <column> <year\|month\|day>`, which stores each period apart ([time shards](20-document-model.md#time-shards)); orm checks the clause (a non-null `date` or `datetime` column, no `order`, no block) |
| `title <column>` | the row's title: non-null, never empty |
| `body <column>` | the row's body: a `markdown` column, never NULL |
| `order <column>` | the order of the row among its siblings, by the column's value ([document model](20-document-model.md#order)); nothing assigns it, so an insert gives it (orm DBS-79) |
| `task_marker <column>` | the column a list row writes between the brackets of its task marker (orm DBS-80): a `varchar(1)` or `bool` column, nullable or not, as stated below |
| `markdown <column>` | the column holds markdown prose and is stored as a section |

`task_marker <column>` is one line per table, and orm refuses a second as a repeat, a column of another type
(`task_marker needs a varchar(1) or bool column, not <type>`) and the per-state form `task_marker <column> <state>
'<glyph>'` as a syntax error. In a table stored as a list, the column is written between the brackets of the list row,
`- [<value>] <key> <title> …` ([canonical format](10-canonical-format.md#list-rows)): a `varchar(1)` value as itself,
the empty text as `[]`; a `bool` as the unchecked and the checked task box of GFM, `[ ]` for false and `[x]` for true,
where `[X]`, which GFM reads as checked too, reads as true and is `format.noncanonical`; and NULL as a list row without a
box, `- <key> <title> …`. Its values are those its type and the schema's constraints allow, as on every database — a
check `in (…)` or a foreign key to a table of states — never a list of glyphs. In `table` and `files` storage the column
is a cell or a field item, as any scalar is. The settings `checkbox`, its former name, and `state_machine` are not
settings of the grammar, each refused as unknown (`setting "checkbox" is unknown`): the engine holds no state machine
([constraints](50-constraints.md#state-columns)). Grounding: a GFM task list item holds one character between its
brackets, its task list item marker, so a column of one character shows its whole value there, and a boolean shows as
GFM's own two boxes.

## Corpus rules on dbspec constructs

- **Composite keys**: a table in `document` or `block` storage may have a primary key of several columns. Its key cell holds the
  values joined by `/` and its heading the column names joined by `/`, as `| tenant/account | name |` and `| 900/1 | a |`; no value
  of a key type holds `/`, so the cell has one reading. Each value follows the key rules of its column type; a read and a write see the
  columns as columns. A composite foreign key holds plain values, not a link, and names the whole key of its table; the validator's
  `reference.dangling` applies to it.
- **Default storage**: a table without a `store` setting is stored `document table`, in one document of
  its own, so a dbspec document written for a SQL database installs on a corpus as it is.
- **Rejected** on a corpus, each a schema error: a composite primary key of a table in `files` storage, which names a file by one key column; an external `use` document; a `codec` whose last stage
  is `yaml` — orm's encoder (go-yaml's `Marshal`) ends every value with a line feed, which no place of
  the format holds, while a later stage encodes it away. A view table is declared by the
  [application schema](100-checklist.md#views), not by a setting.
- **Allowed**: `identity` only on an `i64` primary key column of a markdown table, allocated one more than the larger of
  the largest key and the table's counter ([document model](20-document-model.md#keys)) under the lock. dbspec refuses `identity` on any other type (`an identity column has type i64`), so an
  `i16` or `i32` key cannot take it either. Grounding: `max+1` is an integer sequence, and a `bool`,
  `varchar`, `uuid` or `date` key has no maximum to add one to.
- **Required**: a `list` or `files` storage declares `title`.
- **Key types.** The one-column primary key of a markdown table is an integer type (`i16`, `i32`,
  `i64`), `bool`, `varchar`, `uuid` or `date`; any other type is a schema error, located at the table
  name. Grounding: a key is a file name and an anchor, so its text must hold only the key shape
  ([document model](20-document-model.md#keys)). A `date` is written `YYYY-MM-DD` with a year from 0001 to 9999, the range orm's date literal rule accepts, so its text has four digits and no sign, and its `-` only separates. The ordinary values of `time` and `datetime` hold `:`. The non-integral values of a `decimal` of scale above 0 and of an `f64` hold `.`, and a negative value of any of these types begins with `-`, so they can never meet the shape. A `decimal` of scale 0 holds only integers, which an integer type keys, so it is refused with the other decimals; an `f64`'s whole values are likewise integers that an integer type keys, so an `f64` key adds only values the shape refuses. `text` and `bytes` are refused by dbspec's own
  key rule. A negative integer key is refused per value, not per type ([document model](20-document-model.md#keys)).
- **Foreign keys.** Every foreign key of a table references the primary key of a table. One that
  references a unique non-key column is a schema error, located at the foreign key. Grounding: a
  one-column foreign key is written as a link to the referenced row, and a row has a link through its key.
- **Block placement.** The column of a block table's placement foreign key — the foreign key its
  `store block <fk>` setting names — is non-null; a nullable one is a schema error, located at the
  block table's name. Grounding: a block row stands inside the scope of the row its placement
  foreign key references ([document model](20-document-model.md#storage)), and NULL references
  none, so a row holding NULL there would have no place to be written. dbspec accepts a nullable
  foreign key column, since an SQL row needs no place; the catalog rule is stated here. It holds for
  every block table.

## Keys

A corpus declares `aes`, `aes_version` and `blind_index`, and it stores what orm stores: the ciphertext, the key version
of the row and the blind index.

- **The client holds its keys.** On an orm request the client encrypts and hashes, as on SQL: its connection configuration
  gives the keys (orm `COD-8`), and the executor receives stored values only. It needs no key.
- **The engine takes its keys by injection.** The environment variable `FLOWMARK_KEYRING` names a keyring file, a JSON
  object `{"current": <version>, "versions": {"<version>": "<key>"}, "blind_index": "<key>"}`, in the shape of the
  configuration of orm; `blind_index` is optional. Every command reads it when it loads the catalog. A file that does not
  read as this object, or holds an undeclared current version or an empty key, makes the catalog fail to load, naming the
  variable. So does a keyring inside the corpus, which a corpus publishes, or one that is a symbolic link.
- **A write the engine originates** (CLI, MCP, sidecar) takes the plain text of an aes column and follows orm's rules: it
  encrypts with the current key, records that version, and writes the blind index; assigning the version column is
  `IR_INVALID`, and an update of encrypted data assigns every aes column, since one version describes the row. Without a
  keyring such a write is `CONFIG`, since it would store the plain text; `rows.load`
  ([operations](40-ops.md#the-faces)) takes stored values and is not such a write.
- **The validator and the commit** decode an aes column with the key of the version its row records
  ([constraints](50-constraints.md#codec-values)); in a process without a keyring that column is not judged, and the rest
  of the corpus is.
- **The export** ([sync](70-sync.md)) keeps the stored ciphertext, the version and the blind index, and needs no key.

Grounding: the ciphertext is opaque to every read and write of the executor, so a corpus takes these settings as SQL does;
a key held in a corpus would be published with it, so it is injected from outside; and a value the engine writes must be
readable by a client with the same key, so the engine holds orm's rules and reimplements no stage.

## Name rules

- **Reserved table names.** A table whose name equals, without regard to case, `CON`, `PRN`, `AUX`,
  `NUL`, `COM0`–`COM9` or `LPT0`–`LPT9` is a schema error: these are reserved file names on Windows.
- **A child table named like a markdown column** of its parent is a schema error.
- **Heading depth.** A layout that needs a heading past H6 is a schema error, decided when the
  catalog loads. A table's rows write their sections at their row scope level when the table can
  have a markdown column other than the body or a child block, and their detail heading one level
  above that scope when it can have a body; a table with neither writes no heading below its block
  heading, so its row scope may lie past H6. Grounding: a heading the layout never writes is not one
  the layout needs; a block table with no `markdown` column and no child block has no detail section, and it may
  stand at level 7 under a block row.

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

- The catalog returned is the document set of `dbspec/dbspec-set.json`: each document in dbspec's canonical form
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
  the `catalog` diagnostic `no dbspec/ directory` at `dbspec`, and one without its set file the `catalog`
  diagnostic `no dbspec/dbspec-set.json` at `dbspec/dbspec-set.json`.
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
