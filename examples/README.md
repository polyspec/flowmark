<!-- doc-id: examples -->
# Examples

[한국어](README.ko.md)

Example corpora of the engine. Each folder is a corpus root: its `dbspec/` catalog and the
`*.flowmark.md` documents the engine wrote. Every row was written through the engine, and the
tests of this folder (`go test ./examples/`, part of `make test`) read and change copies of the
corpora through the operations of the faces only, so the corpora stay valid and canonical.

Open one with any face, for example `flowmark rows examples/tracker/aggregate-list items`.

## The tracker in three storages

The issue tracker of the application schema (`flowmark.dbs`: `items`, `people`, `item_links`,
`item_history`, `views`) holds one dataset — six items in every state, their history, two
links, two people and three views — in three corpora that differ in the store setting of
`items` alone:

| Corpus | `items` storage | Where an item lives |
| --- | --- | --- |
| `tracker/aggregate-list` | `store document list` | one list item of `items.flowmark.md` |
| `tracker/aggregate-table` | `store document table` | one table row of `items.flowmark.md` |
| `tracker/split-file` | `store files` | its own file `items/<key>.flowmark.md` |

An item's body, its markdown columns and its history block are its detail section in the two
aggregate corpora and the rest of its own file in the split one. The three corpora read back
the same rows of every table, which a test checks.

## The board

A board of `posts` (title, body, author, created) with `comments` (body, author, created, and a
`parent` for a reply) proves a one-to-many relation in markdown, in two corpora that hold the
same rows (`board.dbs`):

| Corpus | `comments` storage | Where a comment lives |
| --- | --- | --- |
| `board/block` | `store block fk_comments_post list` | a list item under `## comments` in its post's file, a reply indented under its comment |
| `board/files` | `store files` | its own file `comments/<key>.flowmark.md`, whose field list links to its post |

A post is its own file `posts/<key>.flowmark.md` in both, and every document reads as an
ordinary board page in a markdown viewer: the post, its fields as links, its comments. A test
creates a post, comments on it, replies, edits and deletes through the engine on both corpora
and requires the same rows after each step.
