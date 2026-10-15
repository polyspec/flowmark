<!-- doc-id: changelog -->
# Changelog

## Unreleased

- flowmark takes orm 0.0.18: go.mod requires the Go module github.com/polyspec/orm/packages/go, and the engine imports take its packages/go/engine paths. (#T12-6)
- T12-3-1-1: go.mod pins the orm release v0.0.15, which carries the per-document set parse (T166), instead of a main commit.
- T12-3-2: the catalog copies orm's per-document results of ParseSet, a repeated name on its own document, and reconstructs nothing.
- T12-3-1: the catalog names its documents by orm's dbspec.ParseSet instead of splitting the header line, so a refused header names no document and raises no false repeat.
- T12-4-1: the catalog keeps the messages of orm's set reader unchanged and makes corpus paths with filepath.Rel.
- T12-5: the slug recorder's retry tests pass a node:test mock to githubRender instead of a gh stub on PATH, which Windows did not run.
- T12-4: the catalog reads the document set that dbspec/dbspec-set.json declares, with orm's set reader; a .dbs file the set does not list, a listed document outside dbspec/, an external entry and the set file's errors are catalog diagnostics, and every corpus carries its set file.
- T12-3: the catalog locates a table or foreign key diagnostic by the declaration positions of dbspec.Parse instead of a regular expression that fell back to line 1, column 1; the audit of the regular expressions of non-test code found the GFM delimiter cell, the token file, the lock name and the cited markdown-it-katex guard each the one reading of its grammar.
- T14-1: flowmark pins orm v0.0.11.
- T14: a restore picks its key after the request hook through orm's ir.RestoreKey (orm protocol 1.5).
- T12-1: flowmark vendors kit from its release tag v0.0.35 instead of a branch, with the shared PRINCIPLES.md; AGENTS.md links it and keeps only flowmark's rules, and the full test suite runs once when every item of the active checklist is done, the owning checks per item.
- T12.1-38-2-1 (fix): the slug recorder's tests remove the temporary directory of their stub gh, which they left behind.
- T12-2: an abandoned worktree is reviewed at once, what is worth keeping becomes a checklist item with its removal condition, and the worktree is deleted with its branch (AGENTS.md); the two abandoned agent worktrees are removed.
- T12.1-38-1: the engine reads GFM tables as each viewer does, checked against 54 more table vectors recorded from GitHub, GitLab and VS Code: a table cell is inline content, so its footnote references number the footnotes and its raw HTML headings and ids are read, with each `\|` read as `|` and the cells past the header's columns dropped; the paragraph before a header row unescapes its pipes and defines no link label on GitHub and GitLab; the body rows stop at each viewer's cell limit; and VS Code's markdown-it table rule is tried first at every block start and on every paragraph line, so a heading, fence, HTML block, math block, list item or block quote line is a header row when a delimiter row follows it.
- T12.1-38-2 (fix): `make slug-vectors` sends a GitHub request again, up to three attempts, when its connection fails before any response, so one timed-out connection no longer ends a recording of thousands of requests; an HTTP error still ends it.
- T12: the markdown database is complete: every item of its checklist is done — the grammar, the validator, the IR executor behind `markdown://` on the released orm v0.0.9, the scale-tested write path, the faces, the crudui-rendered board at a thousand posts, the three-platform gate, and the complex foreign key corpus.
- T12.12: flowmark requires the released orm v0.0.9 instead of the pseudo-version: the tag carries the IR executor protocol items T61 and T62 and the validator series T82–T102, and the whole suite passes on it.
- T12.14: the corpus of complex foreign keys: a golden declares a five-table chain, a self reference, a two-directional pair, a table five others reference, a link into a block row and every storage kind; the engine writes the chain, joins across it, assembles the relation of a relation, keeps references on a move, and refuses a dangling reference, a self-referenced delete, a chain-edge delete and one side of a two-directional pair alone — the composite keys the model cannot hold stay schema errors the tests pin.
- T12.11.2-2 (fix): the board's sort works on both faces — a wrong call to the query API left the PHP sort always empty, and the sort now orders the whole list before the page is cut (a title as a column order in the request, an author's name by reading the list, for a request order item cannot name a joined column), so the pages follow the sort.
- T12.11.2-2 (fix): the board's pager windows its links — the first page, the last and five around the current with gap markers, as crudui's pagination model does — and keeps the sort in its links; the list itself always shows the ten posts of the requested page.
- T12.11.2-2: the board's form, list and validation run by crudui on both faces: the same field and list specification in `src/PostForm.php` and `node/postForm.ts`, the create form rendered with its errors and the hidden CSRF token, the list rendered server-side, the submission validated by the crudui validator (422 with the field errors, 303 to the list), and the crudui stylesheet served with the app.
- T12.13: the platform gate: `make test` and the writepath benchmarks run on Linux, Windows and macOS at every push (`.github/workflows/test.yml`), green with no test skipped; the Windows defects it found are fixed, the benchmark artifacts are kept per run, and the numbers are compared with the scale envelope of spec 40.
- T12.4-10: orm's table "Markdown IR executor" is spec 60's capability table row by row, held by orm's `make markdown-table-check` against the flowmark revision it pins (`444f27f`), and the Korean capability table of spec 60 is worded within orm's wording rules.
- T12.4-6: a `time` has one form in an IR executor message, `HH:MM:SS.ffffff` whatever its column's precision: the executor reads a parameter in that form (zeros past p) and answers a time in it, and refuses the p-digit form and a non-zero digit past p with `CODEC_ENCODE`, where it read and answered p digits.
- T12.4-5 (fix): the parameter check applies to the messages of orm clients only; the values of a face operation are the engine's own write, which the catalog's rules judge, so a face write with a value of the wrong type is `CONSTRAINT` again, as spec 50 says.
- T12.4-5: the IR executor checks every parameter against the type of its slot before it reads a row: a value outside the slot's type (a JSON kind, an integer range, a decimal's scale and precision, a date outside 0001 to 9999, a varchar's length, U+0000 in text, a non-string `contains` value) and NULL assigned to a non-null column answer `CODEC_ENCODE`, which replaces a capability error and a write fault for the same inputs.
- T12.4-2: the IR executor follows orm's scalar column subquery rule: a scalar column subquery without a limit of one row and an order that includes the primary key answers `IR_INVALID` from orm's validator, and the executor takes the first row of the subquery's order, NULL when none matches, instead of its own earlier answer for several rows.
- T12.4-4: the IR executor follows orm's upsert rules: an insert with `on_duplicate` that more than one key can meet, or whose conflict key it does not assign in full, answers `IR_INVALID` from orm's validator, and the executor's own earlier answer is removed; `affected` stays 1 for an upsert.
- T12.4-8: the IR executor follows orm's column-pair rules: two columns that do not meet, and a column-comparison operator that the column's type or codec does not allow, answer `OPERATOR_NOT_ALLOWED` from orm's validator, and the executor's own earlier answer is removed.
- T12.4-7: the IR executor follows orm's key update rule: an `update` or `restore` that assigns a primary key or identity column (or the soft delete column in a restore) answers `IR_INVALID` from orm's validator, and the executor's own earlier answers for it are removed.
- T12.4-9: the IR executor follows orm's rule that an insert assigns by value or NULL: `plus_p` and `minus_p` in an insert's `set` answer `IR_INVALID` from orm's validator, and the executor's own earlier answer is removed.
- T12.4-3: the IR executor follows orm's `key_by` rule: a relation keyed by a nullable column answers `IR_INVALID` from orm's validator, and the executor's own earlier answer for it is removed.
- T12.4-1: the IR executor follows orm's `group_by` rule: a grouped row read, a grouped sum, a grouped subquery and a group order outside the group answer `IR_INVALID` from orm's validator, and the executor's own earlier answers for them are removed.
- T12.1-38: the engine reads GFM tables as GitHub, GitLab and VS Code do: a header row, a delimiter row of as many cells and the body rows after it take in the lines that CommonMark reads as a paragraph, so a heading that no viewer reads is no heading of the engine and a setext underline after a body row underlines nothing; VS Code's markdown-it also takes a heading line as a header row and a pipeless delimiter row of one cell.
- T12.5-3: the engine states its scale envelope — 1 MiB for a document, 20,000 documents for a folder and for a corpus — from benchmarks of the size of a file, the documents of a folder and a corpus, and the sessions under the lock; a corpus past it is reported by the validator (`scale.file`, `scale.folder`, `scale.corpus`) and a commit that writes a document past 1 MiB publishes `scale.exceeded`, and both are written all the same.
- T12.5-6: opening one corpus from several processes at once no longer fails when two of them make the lock folder together: the folder is made first and judged after.
- T12.5-5: a corpus that fails to open leaves no lock: the watch is closed before the lock is released, since releasing the lock opens its file, which a macOS watch holding a descriptor for every document had left no descriptor for.
- T12.11.2-1: the board application runs on hyper's Node.js server too, over orm's TypeScript client and `markdown://`, in `examples/board/web/node`; the PHP and the Node.js server give the same rows and the same page for a post.
- T12.11.2: the board application runs on hyper's PHP server over orm and `markdown://`, in `examples/board/web`, with 1000 posts spread over the months January to October, 100 each: `posts` is sharded by the month of `created` in both board corpora, and `examples/board/seed` writes the posts through the engine.
- T12.1-35: a `files` or `document` table can declare a time shard (`store ... shard <column> <year|month|day>`, orm T119): its rows stand in `<table>/<period>/<key>.flowmark.md` or its period document `<table>/<period>.flowmark.md`, an insert places a row in the file of its period, an update of the shard column moves the row and rewrites the links to it in the same write, a period without rows has no file, and the validator reports a row in another period's file as `shard.period`.
- T12.7: the export to a database is done: a codec column keeps its stored cell, the host stages hex and ip at the end of its list are undone and orm applies them again, and only a host stage before a client stage is refused; with the identity keys and the `datetime(p)` bind of the earlier entries every table of the catalog exports.
- T12.4: the IR executor is done: it answers every IR kind under the orm protocol with the rulings that need nothing from orm applied; the ten rulings that wait on an orm item are the sub-items T12.4-1 to T12.4-10, each applied after its orm bump.
- T12.7 (in part): the export test applies the statements through orm's `DB.Execute` instead of a binder of its own, so SQLite `datetime(p)` and `time(p)` columns of every precision bind as the database requires; the orm pin moves to the commit that carries it.
- T12.7 (in part): the export to a database keeps an identity table's keys: its rows load through orm's `load` insert and orm's identity restart follows, so the next generated key follows the largest loaded one; only codec tables are still refused.
- T12.1-37: the detail-anchor collision rule reads each viewer's blocks: GitHub's and GitLab's footnote definitions, VS Code's math blocks, GitHub's footnote numbers and GitLab's wikilinks; GitLab's snippet prefix is stated as never applying to a corpus file.
- T12.5-4: the codec's parse and render cost is linear in a file's bytes: about 48 and 35 ms per MB, allocating 50× and 39×, down from 2.1 s and 456× for parsing 1 MB.
- T12.1-33: the collision rule reads raw HTML `<h1>`–`<h6>` headings, which GitHub anchors, and every heading element as GitHub's parser does, by recorded vectors; an element continuing into content the engine does not render takes an anchor its slug so far begins.
- T12.1-31: S_GL reads math spans as GitLab renders them (their literal source), its final Σ as ς, and its footnote references as comrak does; S_VS reads math spans as no text, as VS Code's default markdown-math does. The slug vectors record GitLab, built from the pinned gitlab-glfm-markdown with GitLab's options.
- T12.1-32: the vendored Unicode data folders ship the Unicode License v3 text, which the package documentation names and a test requires.
- T12.1-34: the standard application schema stores one file per item: `items` is `store files`, its history and the links it starts are blocks of its file, and `people` and `views` are one document each, so a transition, a link or a move of an item writes that item's file alone.
- T12.1-36: `files` storage stores `order` as a sparse rank: a move, an insert or a parent change rewrites only the placed row's file unless no rank is free, then renumbers the group once, and a delete rewrites no sibling; reads still see 1-based positions, and the validator's `order` reports only a shared rank. An insert at position 1 among 1,000 siblings writes 1 file instead of 1,001.
- T12.5-2: an operation no longer reads unchanged files: an opened corpus is kept current by the operating system's file notifications (kqueue, inotify, ReadDirectoryChangesW), re-reads everything on an overflow, always re-reads its commit targets under the lock, and reuses each document's parse while its bytes and link targets are unchanged; a read of 10k documents drops from 3.9 s to 68 ms of CPU, and every face closes the corpus it opens.
- T12.11: the issue tracker as one dataset in three storages (`examples/tracker`: document list, document table, one file per row), which read back the same rows through the engine, and the hardcoding inventory grep as the test `tests/inventory`, at zero.
- T12.11.1: the board example (`examples/board`): posts with comments and replies, comments as a block in the post's file or as their own files, where creating, commenting, replying, editing and deleting through the engine give the same rows in both.
- T12.3-6: the write path's commit judges the `codec` and `view` rules on the rows it writes, with the validator's one view judgement: an undecodable codec value the write assigns, or a view row left invalid, is `CONSTRAINT` at its column naming the rule.
- T12.10: the CLI, MCP server and sidecar run schema, rows, row.get, row.insert, row.update, row.move, row.delete, transition and views over any table through the write path, answering wire JSON and `{code, error}` failures; views answers the view table, or a view's rows through its stored filter and sort; `flowmark install` creates the document files, the files-table folders and the `.gitattributes` line `*.flowmark.md text eol=lf`; history repair and unlock are commands; every command exits 0, 1 or 2; the old codec, executor, corpus and app paths and `make validate` are removed; answers no longer HTML-escape.
- T12.3-1: the application schema declares its view table (`views` of the document `flowmark`); the validator reports `view` for a view whose entity, filter, sort or group is invalid, checked with orm's `ir.Validate`, and `codec` for a value its stages cannot decode; a codec ending in `yaml` is refused.
- T12.3-5: spec 30 grounds why a history block stands only under its own state row, with a catalog case for each placement key.
- T12.9: `flowmark serve <corpus> --port <port>` is the server orm clients reach over loopback TCP, with a token in `.flowmark/server.json`, a connect message that checks the corpus and token, transactions and savepoints as control messages, and `--tx-idle-ms`, which rolls back an idle transaction and closes its connection.
- T12.8: the conformance vectors are rewritten for the storage grammar: spec 90 fixes the vector format, and 69 vectors run through the write path over files, document list and table, and block list and table, with equivalence groups, repeats and the write path's own answers.
- T12.7 (in part): the export to a database renders through orm: orm's schema statements, then one planner insert per row in foreign-key and storage order, soft-deleted rows included; identity and codec tables are refused until orm offers what they need; the provenance header writer is removed.
- T12.6: introspection validates every document against the catalog and returns the catalog, each `dbspec/` document in canonical form with its path, name and manifest hash, with every diagnostic; `flowmark introspect` prints it as JSON and exits 0, 1 or 2.
- T12.5-1: `flowmark canon <corpus> [file…]` rewrites the named files, or every corpus document it can, to canonical form in one journaled transaction under the lock. A named file with `format.block`, `format.section`, `format.encoding`, a `parent.cycle` among its rows, or a link into an unavailable block or document table is refused as `CONSTRAINT` and nothing is written; with no file named, such files are reported and left as they are. The command exits 0 when canon ran, 1 on a refusal or failure and 2 on a wrong command line.
- T12.3-4: the catalog refuses a block table whose placement foreign key column is nullable, at the table's name: a block row stands inside the scope of the row that key references, and NULL references none. The history table's storage rule relies on this one rule, and spec 30 names the setting error that places a history row by a column other than its row column.
- T12.4: the IR executor applies the T12.4 rulings: contains folds ASCII case and contains_binary compares code points; NULL sorts first ascending; a files table's storage order is total; the clock is truncated when written and compared at six digits; an out-of-range relative time is NULL; joins exclude soft-deleted children; flatten is answered as a one relation; an unreadable answer value is INTERNAL; restrict holds against the rows before the write; a refused write reports its faults in one order, with at most one duplicate fault a row; an insert rewrites a dangling link it resolves; every key check, on_duplicate included, folds A–Z alone; a write to a document holding two keys equal without regard to case commits; the catalog refuses any other history storage. Specs 10, 20, 30, 40, 50, 60, 80 and the plans state every ruling, and each site that waits on orm names its orm item. `scripts/go-test.mjs` leaves a mount it reused mounted.
- T12.5: the write path joins the IR executor and the journal: every operation takes the corpus lock and first recovers an interrupted commit; a request outside a transaction is one transaction under the lock; transactions and savepoints hold the lock and read their post-image; a commit checks its targets, runs its writes again on the corpus re-read under the lock when it changed, and writes through the journal, the prepare record being its commit point; a lock record carries a token per acquisition; a panic releases the lock; a write that must rewrite a file with a format diagnostic is refused at the request, with the exemptions of canon, the add-column migration and the whole-file delete; flowmark history repair is an executor write; the commit judges checks with the validator's evaluator.
- T12.1-30: the detail-anchor collision rule follows each viewer's exact slug instead of a folded form: S_GH reproduces GitHub's heading ids (Unicode 17.0.0 word characters, no alt text) and S_VS VS Code's (its slugify.ts, Unicode 13.0.0 set, Final_Sigma, alt text), both checked against ids recorded from the viewers for every code point by `make slug-vectors`, and S_GL comrak 0.56.0's as GitLab pins it; the slug input is the CommonMark 0.31.2 rendered text; a numeric character reference in text is its code point; an HTML id or name compares once lowercased, also in GitHub's `user-content-` form.
- T13: Git runs the `commit-msg` hook, which refuses a message that breaks `config/commits.json`, and the `pre-push` push gate: `config/checklist.json` lists both and `make hooks` installs them.
- T12.3-2: the validator reports `check` when a check's predicate is false over a row's logical values whatever a value not of its type is (each such atom evaluated true, then false), and exports the same verdict for the commit; `history.chain` reports continuity only, `state.transition` an undeclared history pair and `state.required` a declared pair without a required value, and a state, `from` or `to` not of its type is `type` alone, a row's own `from` still judged against the row before it.
- T12.3-3: a unique key stays byte for byte, as orm's binary collation compares, the primary key comparing without regard to case only because it is a file name and an anchor; `key.duplicate` says byte for byte when the keys are byte-equal, naming that row, and otherwise names the first key equal without regard to case.
- T12.4 (in part): add the IR executor (`irexec`): it decodes the orm request message, answers reads with the IR Where, joins, relations, aggregates and pagination over the store's rows, refuses requests that need an unavailable table, runs the key checks on the request against the corpus before rendering, and answers writes with `{affected, keys}` and a post-image that the write path commits. Questions the specifications do not yet rule answer `CAPABILITY_UNSUPPORTED` until they are decided.
- T12.3: the validator finds a row's history wherever its history table is stored — through the history foreign key, in corpus order — so a `document table` history no longer reports a false `history.gap`. `scripts/go-test.mjs` names a missing `go` on PATH instead of failing with `spawnSync go ENOENT`.
- T12.1-4-1: the readability criterion is the specification. Structure is judged by CommonMark 0.31.2, and by GFM 0.29 for tables and task-list items; fragment targets — heading slugs, and the ids and names of raw HTML that CommonMark reads outside code — by GitHub, GitLab and VS Code, since the specifications define none and a link `[k](file#anchor)` must resolve in them. MkDocs and Python-Markdown are withdrawn as criteria: MkDocs was named in T12.1-2 only to ground the `--` key rule and made a claimed viewer in T12.1-4 without any requirement naming it, and Python-Markdown implements no specification, so following it chased one implementation's deviations. Withdrawn with them: the `--` key rule of T12.1-2 and T12.1-4 (no criterion viewer collapses hyphen runs, so `a--b` is a valid key); the F_M fold and MkDocs' `_<n>` numbering of T12.1-4; the Python-Markdown reference definitions and the attribute-list collision rule of T12.1-14 (a `{#…}` is literal text in CommonMark and GFM); and their code from T12.2.2, T12.2.2-1 and T12.2.2-6 (`store/attr.go`, `foldM`, the F_M definitions) with the tests that pinned them. Kept with corrected grounding: the `-_` key rule (CommonMark §6.2), the `-<n>` repeat numbering, and the HTML `id`/`name` rule for raw HTML outside code. T12.1-22, T12.1-26 and T12.1-27 are withdrawn as impossible with their cause.
- T12.5 (in part): the write path has the portable lock and the journal of spec 40 (`store/journal`). The lock `.flowmark/lock` is a hard-linked JSON holder record (process start and machine identifier). A held lock is waited for, driven by the operating system's change notification (inotify, kqueue, ReadDirectoryChangesW) and the holder's exit, up to a deadline, and fails as `LOCK_TIMEOUT` naming the holder and its state. A stale lock is removed only by the claim procedure of `flowmark unlock`. A commit stages content-addressed post-images with a copy per rename and pre-images, writes the prepare record, renames (a delete into the stage, missing folders created first), writes the commit record and collects the stage, making each step durable per platform. Recovery completes or discards the one stage and refuses a target edited mid-crash, naming the stage files that resolve it. FAT32, exFAT and mount points inside the corpus are refused. Lock and journal events are published to an observer. Spec 40 states every ruling, among them that durability needs folder syncs on Unix and write-through renames on Windows. The Linux and Windows paths compile and vet; they run in T12.13.
- T12.1-29: a link reference definition at the start of a setext heading's paragraph is no longer read as part of the heading text, as CommonMark 0.31.2 reads it: `[r]: /u` over a setext `people row P1` is a definition followed by a noncanonical detail heading, not `format.block`, and the same holds for section headings; a paragraph of definitions alone over an underline makes no heading.
- T12.1-28: detail, section and block headings report their diagnostics at the first character of their text — column 1 for a setext heading, a document's sections at any level instead of a fixed column 5, a `files` row's instead of column 1 — and a setext section or block heading is reported as a non-canonical heading spelling, not only through the generic canonical-form check.
- T12.3: add the validator (`validate`): the corpus walk, the format rules and the validator rules `type`, `reference.dangling`, `key.duplicate`, `order`, `state.limit`, `parent.cycle`, `history.gap` and `history.chain`, reported per row it can read, with the rules that need a table's complete rows withheld while it is unavailable. `state.limit` is reported at each row past the limit and `key.duplicate` at each later occurrence, of the primary key and of every unique key, in corpus order over every block and file of the table; a unique key compares its values byte for byte, NULLs and values not of their type taking no part. An order value not of its column's type is the type fault alone, counted toward the sibling count, on write and in the validator, and a row whose parent is not of its type joins no sibling group. A foreign key whose key is not of its column's type is `type` alone on read, never `reference.dangling`, and a write that assigns one is `CONSTRAINT`, never `FOREIGN_KEY`. The store goldens are repaired so that every golden validates clean, and a test holds it; `make test` provisions the case-sensitive folder the Go tests need, on macOS by a Case-sensitive APFS image, on Windows by `fsutil`. The views wait on T12.3-1.
- T12.1-25: a test now fails on any test case name of the form `<spec>#<anchor>: …` whose anchor is not a heading of the named spec, with anchors computed as GitHub computes them. Thirteen case names that cited missing anchors (`10#order`, `10#storage-order`, `20#document-order`, `20#readable-values`) now cite the rule they pin.
- T12.1-20: the specifications state that a `markdown` column's section stands at most once in a row scope: a second heading named by the same column, whatever its spelling and even with an equal value, is `format.section`, since two values have no certain meaning and canon would have to drop the bytes of one. The codec already did so; tests pin it for `files` and document rows.
- T12.1-21: a link to a key that rows of two blocks share byte for byte derives its target by a rule. A `parent` derives it from the row of its own block. Any other foreign key whose key two or more rows of the referenced table share has no derived target: it is read without a target check and written as read, so a file's diagnostics no longer depend on the order of its rows. When the referenced table's placement chain reaches a `files` table, the codec asks the linker, whose new `ErrKeyShared` reports a key shared across files. A write that keeps no target for such a link is `CONSTRAINT` at its column.
- T12.1-23: write refusals name a child block alike: a `files` row's undeclared block is refused at the block's name, as a document row's is. A row has at most one `DUPLICATE_KEY`, at its key column: it names the first earlier row with its key byte for byte, or else the case-equal row with the least key in byte order, the first given among equal keys.
- T12.1-24: the specifications ground the storage of a history table: it is stored `block <fk> table` under its state table, or `document table`, since the history rules are stated over its rows and their foreign key, never over their place. A block keeps a row's trail beside the row; a document table has row scope 3 at any depth, so a block machine at row scope 5 that requires a `markdown` column loads when its history is a document table.
- T12.1-16: the table header row is read as GFM reads it: without either outer pipe (a one-column header with none), indented by up to three spaces with no tab, with spaces, tabs, vertical tabs or form feeds at its end, it is `format.noncanonical`; four columns of indent is `format.block`. A row without its last pipe keeps its last cell whole, a final `\|` included, and is `format.noncanonical`; it was silently cut by one byte before. A delimiter row with a tab in its indent is `format.block`.
- T12.1-17: the run after a list marker is measured in columns, a tab advancing to the next multiple of four, and is ruled as spaces of that width: two to four columns are `format.noncanonical`, five or more `format.block`. A row two spaces under a wider row is `format.block`, since CommonMark reads it as a sibling; a row at the wider row's content indent, where the two-space grammar gives it no place, is that row's child and `format.noncanonical`. This supersedes the T12.1-12 ruling that a tab after the marker is `format.block` because it moves the content indent: that reason does not hold at every column, since a tab at column 2 mod 4 is one column wide and moves nothing, and measured in columns a tab is ruled exactly as the spaces CommonMark reads it as.
- T12.1-18: exactly one blank line separates a list or table from the element after it, and none ends the file. A line of only spaces and tabs there is a blank line, so it is no longer `format.block`; any other count, or a blank line holding white space, is `format.noncanonical`.
- T12.1-19: an H1 that names the table in another case, such as `# Items`, is `format.noncanonical`. The H1 and a detail heading's table name are lowercased as the viewers lowercase them, so `İ` becomes `i` followed by U+0307 and names another table.
- T12.1-14: the specifications state what the anchor-collision codec needed, and the codec follows them. A heading's text is read with the link reference definitions of the whole file: under F_G as CommonMark reads them, under F_M as Python-Markdown does, where one white space may stand before a label. An image counts by its alt text under F_G, and by no text under both folds. Every attribute list `{…}` that sets an id on one line, code included, unless a backslash escapes its `{`, is compared with the detail anchors, whatever element Python-Markdown would attach it to; a tab counts as a space; the keys `id` and `name` compare in any case, and the last token of each key spelling wins. A read locates a collision at the `{`, the `<` or the first character of a heading's text, and a write names the cell, field or title whose text holds it. A detail heading's table name matches after lowercasing, and only space, tab and line feed separate its words. A detail heading spelled with a setext underline, or with a run of spaces or tabs other than the single canonical space before, between or after its words, is `format.noncanonical`; another white space character between its words, such as U+00A0, makes it `format.block`. The codec's tests pin each ruling; those of a ruling the codec did not yet follow were red before the change; the writer keeps each cell of a table row, and the key, title, state and each field of a list row, under its column, with the same bytes. Spec 10 and spec 50 state the rulings, with their grounding, in English and Korean.
- T12.1-15: the specifications decide whether a state machine of a block table may `require` a column. A required column that the state table and the history table both declare `markdown` is a section of the history rows, so the depth rule refuses a layout that puts it past H6, at the history table's name. A required column that neither table declares `markdown` is a cell and is allowed at any depth. A required column that only one of the two tables declares `markdown` is a schema error at the history table's name: a history row holds a copy of the value, and a section and a single-line place accept different values. The catalog checks the mismatch in `checkHistoryMarks`; `TestLoadRequireOfMarkdownPastH6` pins the rulings and was red before the check. Spec 30 and spec 100 state them, with their grounding, in English and Korean.
- T12.1-12: the specifications state the calls the `document` codec made without a rule, and the codec follows them. The rows of a document file stand directly under the H1 with no blank line between them, and any blank line there is `format.noncanonical`; the delimiter row is `| --- | --- |`, a GFM delimiter form that differs is `format.noncanonical`, and anything else is `format.block`; a table name in a detail heading may differ in case (`format.noncanonical`), another table's name is `format.block`, and the key stays case-sensitive; a detail section takes the row whose key it names byte for byte; a title token matches a declared column by prefix, a matching token that another token follows is `format.noncanonical` and one that ends the title is `format.block`, the title escapes every matching token and keeps the white space between its tokens, a tab included; a tab that ends a list title before a field is `format.noncanonical`; a tab after the list marker is `format.block`, since it moves the item's content indent. A refused write is located in the request, not in a file: each refusal names its table, key, index among the rows given, column and rule, with `store.Refusal` replacing the line-located diagnostics of `RenderFile` and `RenderDocument`. A key shared byte for byte carries its index, of a pair equal only without regard to case, the spec refuses the row the write introduces (the executor's check, T12.4); the codec's backstop refuses the greater key in byte order; sibling-order and cycle faults are per row, and a dangling parent has its own fault only. `RenderFile` also returns an error for a table that is not a files table. The read side locates a writer refusal at the element the parser read for it (the title, a field, a section or the body of its row), with its row and column in the message; a whole-row fault is at the row's first line. A table or `row` name in a detail heading is compared after lowercasing, as the viewers lowercase it. `catalog.Diagnostic` is unchanged. The spec states the commit-level refusal of a rewrite that has a read diagnostic, and the key checks that the executor runs before rendering (T12.4); neither is implemented here.
- T12.1-13: the specifications state the calls the block codec made without a rule, and no longer disagree. A `parent` of a block or document row names a row of its own block, not of its file: a parent in another block is `CONSTRAINT` on write and `format.block` on read, and the read no longer adds a false `parent.cycle`. A blank line after a block heading is `format.noncanonical` in any count, and a line of only spaces or tabs counts as one; a child block stands once in its row scope, a second one being `format.section`; a detail key shared by blocks of two rows is assigned by its block; a block row links its own `files` row by the file name, derived from that file's key. The codec follows each ruling: the blank-line rule skips every blank line under one diagnostic, and the link to a file's own row is derived without the linker, so a render no longer refuses it as `FOREIGN_KEY`. The writer refuses a parent that names no row once, as `CONSTRAINT`, without the false `parent.cycle` it used to add beside it.

- T12.2.5: a storage-equivalence dataset under `store/testdata/equivalence`: the `entries` table of five rows is stored in five corpora, each with its own catalog: as files (one file per row), as a document list and a document table (`entries.flowmark.md`), and as a block list and a block table under the `plans` of a files table. The five catalogs declare the same columns, the block corpora adding only their placement foreign key `plan`. The rows cover a title, a non-empty and an empty body, a nullable summary section that is absent, present and empty, scalars of text, int, bool and date with NULL values, and a self foreign key `parent` hierarchy, which is the indentation in the list corpora and an explicit `parent` column in the table corpora, with a row that has no detail section. The test requires the five corpora to parse to the same rows on every shared column, NULL included, and each storage to round-trip its rows and its golden bytes. The file and document fuzz targets take the goldens of the dataset as seeds, each corpus selected by a byte.
- T12.2.4-1: the store fixture has a block of `list` storage whose table has a state machine: `item_checks` under each items row, with a checkbox glyph for each of its three states, nested rows, an order column, and a `history` block (`item_check_history`) inside the detail section of its rows. Accepting and rejecting cases pin each rule it exercises, the items golden holds its P1 and P2 round trip, and the fuzz targets are seeded with that golden. The heading-depth rule of the catalog refused the history block at level 7, although its rows write no heading there; the rule now refuses a layout only when its rows write a heading past H6, as spec 30 states. A level-7 row with a body alone is valid, and a markdown column at level 7 stays refused.

- T12.2.2-6: the `files` and `document` codecs apply the detail-anchor collision rule of spec 10. A heading that is not a detail heading is refused when its fold (F_G or F_M, with a final run of digits removed) equals the fold of a detail anchor. An attribute list that sets the id of an anchor, and an HTML `id` or `name` equal to an anchor, are refused too, compared without regard to case. A write that introduces a collision is `CONSTRAINT`, naming the other heading; a duplicate key stays `DUPLICATE_KEY`. A read of a collision is `format.block`. Headings come from the CommonMark block structure, nested ones included, and the inline text from a minimal reader of links, emphasis, code spans, entities and raw HTML, checked against commonmark.js 0.31.2. The module adds `golang.org/x/text` v0.42.0 as a direct dependency for `norm.NFKD`: the folds need the decomposition of Unicode 17.0.0, which the standard library lacks, and a test pins `norm.Version` to 17.0.0. `internal/htmlentity` holds the named references of the `entities` 3.0.1 data, generated from it and checked fresh by a test.

- T12.2.2-5: the heading scanner reads an ATX heading whose `#` run is followed by a tab, as CommonMark 0.31.2 does; the end of the line was already read. A tab after the hashes is a noncanonical spelling of a title, section, block or detail heading (`format.noncanonical`), and the scanner, the section boundaries and the detail-heading parsing all follow the same reading. `##x` and a run of seven `#` stay text. Spec 10 names the tab in the detail heading rule.

- T12.2.4: the `block` storage codec parses and writes the child blocks of a row. A block is a section at the level of its parent's sections: a `files` row holds its blocks at level 2 after its markdown sections, and a `document` or block row holds them in its detail section, in table name order. A block is its list or table, followed by the detail sections of its rows, recursively. A block with no rows is not written, and an empty block heading is `format.noncanonical`. A block row carries no placement foreign key, which its nesting implies; `Row.Blocks` holds each block's rows in storage order. A write refuses an undeclared block, rows of another table, and a placement key assigned to a block row. The T12.2.3 write guard and read refusal are removed, and `sp`, which the production codec used, moves from a test file into `store/row.go`.

- T12.2.3: the `document` storage codec parses and writes `<table>.flowmark.md` for the list and the table shapes: the H1, the rows under it (a list row with its glyph, key, title and fields parsed from the right with their escapes, and indentation as `parent`; a table row with its key first, its cells with the pipe escape, NULL and the empty text), and the detail sections `## <table> row <key>` with their body and sections in storage order. `order` is filled with the 1-based sibling position on read and taken as exactly 1..m per sibling group on write; the writer orders the rows itself and never refuses unsorted input. Foreign keys are links through the linker, and a dangling link keeps its written target. Until T12.2.4 a write refuses a table whose catalog declares a child block, with an error that names the table, the block and T12.2.4, so that no write drops the block's rows; a read refuses a detail section that holds a child block section with the same naming. A temporal value written `now` is not of its type: it is kept as written on read and refused on write, while a text value `now` is text. The rules have accepting and rejecting cases, the three goldens are P1 and P2, and the parse and render fuzz targets are seeded with them.

- T12.2.2-4: the `files` codec applies the invisible-character rule of spec 10 to the H1 title and to a field-list value. An invisible character at either end is refused on write as `CONSTRAINT`; on read a space or a tab that CommonMark strips is `format.noncanonical`, and any other invisible character at an end is `format.block`. The invisible characters are the White_Space characters other than U+1680, the controls, the format characters other than the prepended concatenation marks, and the Default_Ignorable_Code_Point characters; the last come from a table generated from the Unicode 17.0.0 data file and checked fresh by a test. List rows and table cells follow in T12.2.3.

- T12.1-11: the specifications and the plan state the read side of an open block as it is. The lines that a fence or an HTML block left open takes in are the value's text on read, since under the CommonMark block structure they are no boundary and a viewer shows them inside the block; only the write that places an element after such a block is `CONSTRAINT`. The `format.block` read clause of an open block is withdrawn, as no reader can observe that fault, and the open top-level fence sentence reads consistently with this rule.

- T12.2.2-3: the `files` codec applies the open-block rule of spec 10 on write: a value that leaves a fence or an HTML block open before the element that follows it, which that element would be taken into, is `CONSTRAINT`. The block scanner reads HTML blocks of types 6 and 7 as CommonMark 0.31.2 does, a type 7 block not interrupting a paragraph, and a block left open inside a list item that a top-level heading closes is no fault.

- T12.1-9: the specifications and the plan refuse an invisible character at either end of a single-line value: one with the Unicode White_Space property other than U+1680, a control (Cc), a format character (Cf, including U+FEFF) other than a Prepended_Concatenation_Mark, or a Default_Ignorable_Code_Point, under Unicode 17.0.0. Such a character renders with no glyph, so a reader cannot see it; U+1680 and the prepended concatenation marks render with a glyph and are text. The viewers strip different sets of them at the edges, so refusing every one keeps each viewer's display the text of the value, whatever the viewer's version. The canonical separators stay the space. On read, a space or a tab at an end, which CommonMark strips, is `format.noncanonical` (a tab ending a list title before a following token is `format.block`), and any other invisible character at an end is `format.block`, since the write rule cannot produce it again. This replaces the space-and-tab definition of T12.1-8, and the framing blank line is spelled out as a line of only spaces and tabs.

- T12.1-7: the specifications ground the setext rule. A setext heading is of level 1 or 2 and every row scope's level L is at least 2, so no setext underline lies deeper than L; the setext refusal of spec 10 therefore covers every top-level setext heading in a value, and the plan states the same grounding.

- T12.1-10: the specifications state one rule for section boundaries and value ends, the CommonMark 0.31.2 block structure. A top-level heading is an ATX heading or a setext heading closing a top-level paragraph; fences, HTML blocks of every type (1 to 7), containers, lazy continuation lines and the interruption rules count as CommonMark states them. A value that leaves a fence or an HTML block open is refused only when the following element would be taken into it, and a block left open inside a container that a following top-level element closes is no fault. This replaces the narrower sentences of T12.1-6 and T12.1-8.

- T12.2.2-2: the block scanner reads CommonMark containers. A heading inside a list item (by the content indent of
  the item, ordered and tab-indented ones included) or a block quote, a line inside a fence, and a line inside an HTML
  block of types 1 to 5 are not section boundaries; only a top-level heading ends a body. A setext underline is a
  heading only when it closes a top-level paragraph, and a lazy continuation line is paragraph text, as CommonMark
  0.31.2 reads it.

- T12.1-8: the specifications state the read rules the `files` codec needed. A value that leaves an HTML block of
  CommonMark types 1–5 open before a following element is `format.block` on read, as it is `CONSTRAINT` on
  write; an open fence cannot precede an element on read, since a heading inside it is no boundary. A title whose decoded text is empty or ends in `\#` is `format.block` on read, since the write rule
  cannot produce its H1 again. A line ending is LF, CRLF or a bare CR as CommonMark reads it, and a CR in any of
  them is `format.noncanonical`. White space in the single-line rules is the space and the tab.

- T12.2.2-1: the `files` codec and store follow the T12.1-3 and T12.1-4 rulings. An H1 title reads a final `\#`
  as `#`, is written with its last `#` escaped when its final hash run follows a space or a tab or is the whole
  title, and a title ending in `\#` is `CONSTRAINT`. A markdown value that begins or ends with a blank line, or
  holds a CR or a U+0000, is refused; a value that leaves a fence or an HTML block open before an element, or holds
  a setext heading, is refused; a `files` body after a field list does not begin with an indented line. A BOM or a CR
  is `format.noncanonical`, a U+0000 is `format.encoding`, and a typed scalar is written in its canonical form. A
  scalar not of its column type, a dangling foreign key and a key that differs from another only in case are kept as
  read (a raw text, a written target) for the validator's `type`, `reference.dangling` and `key.duplicate`. A key may
  end with `-` and holds no `-_`. White space that rendering drops around a single-line place is
  `format.noncanonical`. A body line directly after the field list is `format.block`.

- T12.1-6: the specifications answer the questions of T12.1-3. A regular file or a symlink at a `files` table's
  place is `corpus.walk`, and the table is unavailable; a missing folder stays an empty table. The first body or
  section of a `files` row without a field list follows the H1 after exactly one blank line. A value that leaves a
  fence or an HTML block open is `CONSTRAINT` whichever write places the element after it. The setext rule covers
  top-level headings only, as section boundaries do. A `format.encoding` fault at an invalid UTF-8 byte is at that
  byte, a whole-file or `corpus.walk` fault is at line 1, column 1, and `catalog` columns count code points, as
  orm's dbspec does. A whole-file delete is exempt from the rewrite refusal, since removing a file rewrites none of
  its bytes.

- T12.1-5: the specifications state the rulings the `document` codec needed. `now` is dbspec's keyword for the
  current time in a `datetime` default and is never stored: a temporal value written `now` is not of its type, and a
  text `now` is text. A table row out of storage order is `format.noncanonical`; a `parent` cycle among a file's rows
  is `parent.cycle` on read and leaves the file unreadable; a second detail section of a row is `format.block`. A
  title token `<column>\:` is `CONSTRAINT` on write. In a table cell each `|` is written `\|` and nothing more, and
  on read a `|` after a `\` is never a cell boundary. `order` is a logical column that a read fills with the sibling
  position, and a write takes each sibling group's values as exactly 1..m, m being the group's size after the write. The codec computes the storage order from the rows and
  never refuses an unsorted set. A diagnostic's column is the 1-based code-point column of the element its rule names.
  A dangling reference keeps its written target, and a `parent` cell naming a key that two rows share is
  `format.block`. A value holds no setext heading, and a value that leaves a fence or an HTML block open is
  `CONSTRAINT` when an element follows. The catalog rejects a list storage of a state machine without a checkbox;
  dbspec already rejects a nullable state column.

- T12.1-4: the specifications state the detail-anchor, key and identity rulings. A detail heading's
  anchor is checked against the headings of its file under two folds of the heading's rendered inline
  text. F_G, for GitHub, GitLab and VS Code, removes every character that is not a letter or a decimal
  digit by Unicode category, lowercases, decomposes by NFKD, drops the combining marks, and keeps the
  ASCII letters and digits. F_M, for MkDocs, lowercases, decomposes by NFKD, drops the combining marks,
  and keeps the ASCII letters and digits, so `café` folds to `cafe` under both. A heading other than a
  detail heading takes a detail anchor when the two folds are equal once a final run of decimal digits
  is removed from each, since every named viewer numbers a taken or repeated id by appending a number or
  raising a final number. Two detail headings collide only by equal anchors, so keys `P1` and `P10`
  never collide. An attribute list that sets the id to the anchor, and a payload HTML `id` or `name`
  equal to the anchor, are refused too. A write that introduces a collision is `CONSTRAINT`, except a
  duplicate key, which is `DUPLICATE_KEY`; a read of a collision is `format.block`, except a pair of keys
  that differ only in case, which is `key.duplicate` alone. The claimed viewers are GitHub, GitLab, VS
  Code and MkDocs; Pandoc is not claimed. A key holds no `--` or `-_` and is ASCII; a negative integer key
  is refused. The primary key of a markdown table is an integer, `bool`, `varchar`, `uuid` or `date`
  column, and the catalog rejects any other type at the table; `identity` is allowed only on an `i64` key,
  which dbspec already enforces and the catalog tests pin. A key's date range is 0001 to 9999.

- T8.4-4: `TestAddWithoutPathOnEmptyCorpus` passes on every run. The test assumed the add answer
  comes before the watcher's change event, but the order is not defined: the watcher reports the
  add's own write, and a sidecar may push change events at any time. The test now asserts that the
  one string answer is `T1 added` and that every other message is a changed event of the surface.
  `Serve` also no longer returns while a poll goroutine can still write to the output: `stop` waits
  for the poll loops, which removes a data race that `go test -race` reported on the output buffer,
  and a poll loop ends at once when stopped instead of finishing its wait between two polls.

- T12.1-3: the specifications state the rulings the `files` codec needed. On read, a missing non-null field or section, a single-line value that renders differently from its text beyond its inline markdown, a `files` body opening with a `-` item, and a `files` body whose first line is indented by two or more columns (spaces, or a tab) after a field list are `format.block`; white space that rendering drops around a single-line place is `format.noncanonical`; a BOM or a CR is `format.noncanonical`, and invalid UTF-8 or a U+0000 is `format.encoding`. The same read faults are `CONSTRAINT` on write. A value not of its column type, a dangling foreign key and a key equal to another without regard to case are validator rules (`type`, `reference.dangling`, `key.duplicate`); the writes that introduce a dangling key or a duplicate are `FOREIGN_KEY` and `DUPLICATE_KEY`. A non-null `markdown` column other than the body is always a section. A title whose final `#` run follows a space or a tab, or that is only `#`, writes its last `#` as `\#`, and a title ending in `\#` is refused. A markdown value never begins or ends with a line feed. The blank line after a field list stands only before a non-empty body or a section, and a document ends with one LF. A logical write that rewrites a file with a `format.*` diagnostic is refused as `CONSTRAINT`, naming `flowmark canon` when the diagnostic is `format.noncanonical`; spec 40 adds `flowmark canon`, the one operation that normalizes document bytes, and refuses a commit that must rewrite a file with a `format.*` diagnostic, exempting `flowmark canon` for its targets, the add-column migration for the diagnostics the added column causes, and a delete of a whole `files` row file; a table holding such a file is unavailable to the requests and checks that need its rows; `flowmark install` adds `*.flowmark.md text eol=lf` to `.gitattributes`; a missing table folder is an empty table; a typed scalar is written in its canonical form. A table holds every file its rows may be in, a block table including its placement parent's files; two rows with byte-for-byte equal keys make the table unavailable; a request that evaluates a value of the wrong type fails as `CONSTRAINT`; an export that needs an unavailable table fails and writes nothing. A `corpus.walk` entry at a table's place, or a suffix in another case, makes that table unavailable until a human repairs the corpus, and the table is then available again on the next read; a `type` diagnostic fails a commit only where a constraint check must evaluate the value, and an export that reads it fails. The `files` codec test cases name the spec section they pin.

- T1.3-1: the reference check reads the files of the checkout that Git does not ignore, through
  kit's `trackedFiles`, as the kit checks do. Ignored directories such as the agent worktrees under
  `.claude/worktrees/` no longer change its result.

- T12.2.1-2: every foreign key of a table is one column that references the one-column primary key
  of a table. A composite foreign key, or one referencing a unique non-key column, is a located
  `catalog` diagnostic at the foreign key. A link to a row has only the row's one-column key.

- T12.2.2: the `files` storage parses and renders one `<table>/<key>.flowmark.md` per row: the H1
  title with its `\#` escape, the field list of the stored scalar columns in their readable value
  forms, the body, the level-2 sections of the nullable markdown columns, and foreign keys as links
  whose targets come from a `Linker`. Literals are read through orm's `CanonicalLiteral` at the
  pinned commit `72e2b535`. A noncanonical spelling is `format.noncanonical`: its row is returned,
  and emitting it gives the canonical file. A byte order mark is a noncanonical
  `format.noncanonical`; invalid UTF-8 is a fatal `format.encoding`. A file outside its place, a symbolic link, a folder
  named as a document and a suffix in another case are `corpus.walk`; a key that breaks the key
  rules is `format.block`; a write the rules refuse is `CONSTRAINT`. A child block section is
  `format.section` until T12.2.4 reads it. The golden fixtures round-trip byte for byte, and parse
  and emit are fuzzed.

- T12.1-2: a detail heading is `<h> <table> row <key>` with the anchor `<table>-row-<lowercased key>`,
  so a block or markdown-column heading repeated under several parents can no longer take a detail
  anchor (`items-1` is not `items row 1`). A key holds no `--` and does not end with `-`. A detail
  anchor must be free under both GitHub slug de-duplications (github-slugger and github.com); a
  heading that takes it is `CONSTRAINT` on write and `format.block` on read, and a non-canonical
  spelling is `format.noncanonical`. Links are stated to resolve in viewers with GitHub-style heading
  slugs. The catalog's same-file name rule is withdrawn: a markdown column or a block table may be
  named like another table of its file; the child-named-like-markdown-column rule stays.

- T12.2.1-1: the catalog reads only the `.dbs` set. `dbspec/flowmark.json` and its `translations`
  field are removed, with the settings reader and the `Translations` field of the Catalog API. Any
  other entry in `dbspec/`, a file of another name or a folder, is a `catalog` diagnostic; hidden
  entries are skipped. The name rules of spec 30 are enforced: a table named like a Windows device
  name (`con`, `prn`, `aux`, `nul`, `com0`-`com9`, `lpt0`-`lpt9`) is a schema error; a child block
  table named like a markdown column of its parent is a schema error; a markdown column or a block
  table named like another table whose detail sections stand in the same file (a document table
  with its block descendants, or the block descendants of a files table) is a schema error. The
  name comparison ignores case. The fixture catalog no longer carries `dbspec/flowmark.json`.

- T12.1-1: the specifications state the readable-document ruling. A corpus document is a
  `*.flowmark.md` file that reads as an ordinary markdown document, with no comment, frontmatter,
  doc-id, provenance or twin marker. A `files` row is the file `<table>/<key>.flowmark.md` and its
  key is the file name; keys are portable file names and anchors. Spec 10 states the field list,
  the readable value forms, the single-line places, the detail headings `<h> <table> <key>` with
  their anchors, the order of sections, and foreign keys as links with derived targets; spec 20
  and spec 50 state the walk, the key rules and `corpus.walk`; spec 30 withdraws
  `dbspec/flowmark.json`; the sync provenance moves out of the document (open question 5). The
  design plan, spec 00 and spec 80 drop their withdrawn leftovers, and the plan adds the board
  app on the polyspec stack and keeps the Wails example host.

- T2.5-1: the corpus golden test reads the documents `config/documents.json` selects, through the same selection as `make documents-check`, instead of every markdown file outside four hard-coded directories. A markdown fixture under a package's `testdata` directory no longer breaks `go test ./codec/`; `config/documents.json` excludes `**/testdata/**` from the documents.

- T12.2.1: the `catalog` package loads the catalog of a corpus root. It reads the `dbspec/*.dbs`
  set with orm's `engine/dbspec` and the `dbspec/flowmark.json` translations, and rejects each
  case the spec names as a located diagnostic: a missing catalog, a table without `store`, a
  composite key, `aes`/`aes_version`/`blind_index`, `audit`/`immutable`, `cascade`/`set_null`, an
  external `use`, a state machine without `history` or `initial`, a list or files storage without
  `title`, a heading past H6, and a block cycle. The loaded catalog holds, per table, the storage,
  key, title, body, order, markdown columns and state machine. The glyph notation is corrected:
  spec 100, spec 30 and the markdown database plan write each checkbox glyph as the one character
  between the brackets (`waiting ' '`, `doing '~'`, `done 'o'`, `bypassed '!'`, `impossible '-'`)
  instead of `'[ ]'` and its siblings, since orm's `checkbox` setting takes exactly one character.

- T12.1: the rulings and specifications are rewritten on the markdown database plan. flowmark is
  the engine and orm reaches it as an IR executor by `markdown://`; the `dbspec/` catalog is the
  only schema; each table declares a `files`, `document` or `block` storage; keys are opaque; the
  canonical format pins the row scopes, sections, list fields and table cells; operations pin the
  server, sessions, the commit order and the failure codes; constraints pin the state machine
  with its history, gap, chain and repair; `flowmark.dbs` is the application schema. The design
  plan records revision 3 and the in-document Mermaid authority (T8.5) is withdrawn.

- T8.4-3: `go vet` passes over the old write path. `MoveItem` copies the moved lines with one
  `append`; the second `append` with no values that T8.4 added is removed.

- T8.4-2: the repository tracks no built executable. The root `flowmark`, written by a
  `go build` in the repository root, leaves the index and is ignored; `make build` writes
  `build/flowmark`.

- T10-5: the plugin repository tracks no built executable. The 16 MB example host binary is
  ignored; `go build -o example .` regenerates it. The change lives in the plugin repository.

- T10-4: the new-issue row is pinned to the bottom of its column. The card list fills the
  column and scrolls inside itself, so the row stays visible however many cards the column
  holds. The change lives in the plugin repository.

- T10-3: every board column takes the full height of the board instead of the height of the
  busiest column, and scrolls inside itself when its cards overflow. The change lives in the
  plugin repository.

- T10-2: the new-issue row follows the last card of its column and the whole column takes a
  drop; the card list no longer fills the column, so a stretched column leaves no gap. The
  change lives in the plugin repository.

- T10-1: the board never shows a state the document does not hold. A refused drop (done →
  doing, or an edge that needs evidence) left the card in the column it was dropped on while
  the markdown kept its state; the plugin now judges a drop before moving anything and redraws
  from the document after every operation, clears the columns on an empty answer and re-reads
  the state machine on a changed event. The change lives in the plugin repository.

- T7.3-3: the sidecar answers stay whole lines. The watcher's poll goroutine and the request
  loop wrote to one buffered stream without a lock, so a pushed changed event could tear an
  answer line — the page then lost an answer and its view of the corpus. One lock now guards
  the stream, the session end flushes under it, and a root's fingerprint baseline is taken when
  the surface is watched, before its first answer, so an edit right after that answer is seen.

- T7.1-1: `flowmark items` prints the same item the sidecar serves: six tab-separated columns
  ending in the path, assignee and due. The executor rows carry the path directly, so the
  sidecar drops its documents join.

- T8.6: the write path serves the table form. A document whose items live in a table gets the
  same commands as the list form: a transition rewrites the state cell by the document's own
  edges, an add appends a row with the auto ID (NextID reads table rows), an edit rewrites the
  title cell, a move reorders the row lines and a remove deletes the row. One grammar per
  document — checklist lines and an items table together are refused.

- T11: items carry people and dates. An item title may close with `@assignee` and a
  `YYYY-MM-DD` due date — the codec splits the closing tokens from the title — and a document
  holds a people table by naming `id` and `name` columns. The first row is the owner: an item
  without an assignee is the owner's. `All("items")` gains assignee and due for list and table
  items alike, `All("people")` returns the people rows, and the sidecar items answer carries
  the two new columns with a `people` answer of its own.
- T11 correction: the people table is the registry of the declared people, not an owner
  declaration — a blank assignee reads as self and the board never guesses a name; the
  assignee shows as its id (`@max`) in one consistent form, and a due may carry a time
  (`YYYY-MM-DDTHH:MM[:SS]` with an optional zone).

- T4.3: items also live in table form. A markdown table whose header row names the state, id and
  title columns is the document's item list — the header the document writes is the definition —
  and `All("items")` returns its rows in row order; the codec records the absolute line of every
  data row so a write path can address them.

- T8.5: the Mermaid block of the document is the only transition authority. Transition reads the
  edges, the required evidence and the glyphs from the document's own state machine — an
  undeclared edge is refused — AddItem lands a new item in the state the `[*]` line names, the
  WIP gate counts the kit glyphs `[~]` against `[o]` regardless of the state names, ApplySchema
  rewrites the Mermaid block itself, and the built-in five-state table, the frontmatter schema
  reader, the unused document-driven wrappers and the dead corpus mermaid walker are gone.

- T1.0: the repository bootstraps with its document pairs (README, AGENTS, CHANGELOG), the
  `.flowmark/` ignore rule and this execution checklist pair.
- T1.1: the design plan folds the three-lane adversarial audit into rulings — the executor seam
  in orm, two-clause idempotency, dbspec-driven frontmatter embedding, the prepare-record commit
  protocol with one portable per-operation lock, restrict-only foreign keys, the rejected
  identity column, type-directed ordering, adapter-based sync.
- T1.2: the repository vendors polyspec/kit whole-directory with its lock (`make kit-sync`,
  `make kit-check`) and configures the trackers: config/checklist.json reads the
  execution-checklist pair with the five states and config/documents.json selects the document
  pairs and the changelog; `make documents-check` passes.
- T1.3: `make refs-check` reads the checklists of config/checklist.json and the prose of every
  selected document (fenced blocks and inline code excluded) and reports each task ID that
  resolves to no item as a finding with its file, line and column; `make test` covers it.
- T1.1-1: one Go binary carries three thin faces — the CLI, the MCP server and the soksak sidecar
  (protocol v1 of the soksak core specification) — and the GUI ships as a soksak plugin in its own
  repository; the repository stays single-language Go with a `package.json` and a `sidecar.json`
  manifest, and per-language packages appear only when a requirement names them.
- T1.4.2: 10-canonical-format states the two idempotency propositions, the skeleton with the preservation boundary, the document grammar, the frontmatter profile, the task-ID grammar with type-directed ordering, and the encoding rules.
- T1.4.3: 20-document-model states folder as table, file as row, the column mappings, child tables, the translation pair as one row, and the frontmatter bindings.
- T1.4.4: 30-schema states the dbspec-authority derivation, the literal-block embedding, the standalone provisional authority, the schema stamp, and the human-write rulings.
- T1.4.5: 40-ops states recovery on open, the per-operation portable lock, the normative commit protocol with its prepare record, the corpus revision token, and the cost model.
- T1.4.6: 50-constraints states strict parse, types, keys and references, the state-machine setting, the human-write rulings, and the key-required codec validation.
- T1.5.1: 60-dialect writes the capability table in full — the IR surface, the strengthened update, the on_duplicate ruling, the rejected surfaces, and the Go-only marking.
- T1.5.2: 70-sync states the adapter contract scope and the settled rulings — truth ownership, provenance, git-aware drift handling, and rebuildable projections.
- T1.5.3: 80-migration states the additive-first scope, the codemod transaction, the reference-rewrite exception, and the checklist cutover.
- T1.5.4: 90-conformance states the corpus-vector contract, the required round-trip and idempotency properties, and the Go-first runner.
- T1.5.5: 100-checklist writes the application in full — the tables, the five-state machine with terminals and required fields, the WIP gate, the board as one more consumer, and the cutover.
- T1.5.6: 110-open-questions registers the seven open decisions with recommendations and deciding conditions.
- T1.4.1: 00-overview writes the scope, the enforcement summary, the glossary, the non-goals, and the document map.
- T1.4.7: the Korean documents follow the dry style — action verbs, no metaphor, English technical terms; T1.4 completes with its seven sub-items.
- T2.1: the Go module starts with the document skeleton — the strict parse and the canonical render of the header comments, the title, the preamble, the frontmatter block and the H2 sections — with the P1 and P2 round-trip tests and the header diagnostics.
- T2.5: the codec proves P1 and P2 over the repository corpus — every selected document parses without a diagnostic and re-renders byte for byte, and a CRLF copy re-renders stably; the grammar takes the H2 and H3 headings and the header-title adjacency of the real documents.
- T2.4: every parse rule reports a located diagnostic — the fixed rule names with line and column — and a fenced block is payload: the structural rules read unfenced lines only, and an unclosed frontmatter fence names its opening line.
- T2.2: the frontmatter parses as the YAML 1.2 core subset — scalars, quoted scalars, sequences and literal blocks — and re-emits canonically; comments, the flow style, anchors, tab indents and duplicate keys are located rejections.
- T2.3: the child blocks extract as rows — the checklist items of the kit grammar with their states, IDs, titles and continuation lines, and the markdown tables with headers and cells — from the unfenced payload only; a malformed item is a located rejection. T2 completes.
- T3.1: the corpus validator walks a directory of documents — the strict parse findings, the unique doc-ids and item IDs across the corpus, and every task-ID token in prose (fenced blocks and inline code excluded) resolving to an item.
- T3.2: `make validate` runs `flowmark validate` over the corpus directory with the selection
  of config/documents.json, and the corpus of this repository passes. T3 completes.
- T4.1: the executor runs one, all and count over the corpus rows — documents and checklist items as the first tables — with where predicates, ordering and limit; the task-ID ordering is segment-wise numeric, so `T8.0.9` sorts before `T8.0.10` and the suffix follows its base.
- T4.2: the executor runs sum, avg, group_count and paginate — integer aggregates with a non-digit rejection, per-value counts, and a page with its total. T4 completes.
- T8.1: the application encodes the transition table — done is terminal, a bypass carries its cause and retry condition, an impossible item its cause — with the five states mapped to the kit glyphs and the superseded spawn numbering `T8.0.14-1`, `T8.0.14-2`.
- T8.2: applying a transition holds the table, the required fields and the WIP gate — at most two doing items beyond the done count — and rewrites the state glyph of the item line in place, leaving every other byte of the document alone.
- T8.3: `flowmark items` lists the corpus items and `flowmark transition` applies one through
  the state machine; the walk now enters a dot root, a Korean twin registers no second
  keys, and the changelog examples carry their IDs in code spans. T8 completes.
- T1.4.8: 20-document-model states the aggregate and the split shapes — one file, one-to-many linked files, one-to-many-to-one — all built from the file-as-row, child-block and foreign-key-column primitives, with restrict-only keys.
- T9.1: the conformance vectors run — corpus before, request, expected rows as projections — with four vectors pinning the document order, the where count, the type-directed item order and the paginate total; spec 90 states the vector shape. T9 completes its first stage.
- T7: the three faces run from one binary — the CLI (validate, items, transition), the MCP server (initialize, tools/list, tools/call over stdio, one JSON-RPC message per line) and the soksak sidecar (the protocol v1 loop with surface routing and the closed handshake). T7 completes.
- T6: the sync engine lands its v1 — the md-to-db export as a derived, deterministic SQL dump of the rows, and the materialization of a rowset into an adapter-owned document whose provenance header records the hash of its body, so the drift check is self-verifying and clone-safe. T6 completes.
- T5: the orm repository lands the manifest settings and the state-machine sub-grammar as the state_machine and markdown settings of dbspec (orm `T59` b74d8890 and `T60` fb218c7c): parse, validation, canonical emission and the settings model, with the engine suite green. The executor registration in the orm engine remains the engine roadmap; the capability contract stays spec 60-dialect.
- T10: the plugin starts in its own repository (soksak/plugins/flowmark, commit ae07ec9): the kanban board over the flowmark sidecar — five state columns, one card per item, the board.refresh command, and no write path of its own, so the board cannot break the state machine.
- T7.3-1: every sidecar and MCP message flushes on its own write. Cause: the deferred flush held each answer until the input ended, so a host that keeps the session open received nothing while it ran — the example Wails host of the plugin repository found it over a live round trip. A regression test reads one answer while the pipe stays open.
- T8.4: issues enter through the write path — `flowmark add` appends one waiting item with a unique-ID check and `flowmark rm` removes one, refusing a parent that carries children; the MCP tools flowmark_add and flowmark_remove and the sidecar operations add and remove carry the same surface.
- T7.3-2: the sidecar watches the corpus — it polls the fingerprint of every open surface root and pushes a changed event, so the page refreshes while the session runs instead of needing a restart.
