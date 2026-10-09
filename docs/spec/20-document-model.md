<!-- doc-id: spec-document-model -->
# Document model

[한국어](20-document-model.ko.md)

This specification pins how a corpus directory becomes tables and rows. The mapping is total:
every byte of a document belongs to exactly one modeled place, or the parser reports it.

## Corpus and tables

- **Folder = table.** A directory that a dbspec table binds by its `table` declaration. The walk
  skips `.flowmark/` and hidden entries; a symlink or a hard link inside the corpus is a
  diagnostic; a nested corpus root is an error.
- **File = row.** One markdown document is one row; the primary key is the declared key column
  of the table, carried as `doc-id` in the header comment and, for task-keyed tables, the item
  grammar of the [canonical format](10-canonical-format.md).
- **A translation pair is one row.** The English document is the row; its Korean twin is a
  projection of that row — the twin carries the same `doc-id` and holds the `source-sha256` of
  the English file's raw bytes. The twin is never a second row and holds no column data of its
  own.

## Columns

- **Frontmatter = typed columns.** Every key of the frontmatter block is a column of the table;
  values follow the [frontmatter profile](10-canonical-format.md).
- **Preamble = the reserved long-text column `preamble`.**
- **H2 section = a declared long-text column.** One heading per declared column; an undeclared
  heading is a diagnostic; an absent section is NULL and an empty section is the empty text.
- **Body bytes outside the modeled places are preserved payload** — they belong to the row as
  the renderer's preservation boundary defines, and no query sees into them except as the row's
  preserved content.

## Child tables

- **Structured blocks are child tables.** A checklist block, a status table or a markdown table
  inside a document maps to rows of a declared child table whose foreign key targets the owning
  document; the layouts are declared per child table in the schema.
- **A child row lives inside its parent document** and has no file of its own; the parent's
  transaction covers it. Referential actions are restrict-only ([constraints](50-constraints.md)).
- **A markdown-table cell cannot hold a value that may contain a newline**; the schema places
  such columns in checklist or section layouts instead.

## Bindings

The frontmatter carries the row's data plus the bindings: `table` names the owning table,
`schema_version` stamps the schema generation the document was written for, and a standalone
document additionally embeds its [schema fragment](30-schema.md). A binding mismatch is the
`schema.drift` diagnostic, never a silent reparse.
