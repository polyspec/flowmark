package catalog_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

const fixtureRoot = "testdata/app"

// writeCorpus writes files under a fresh corpus root's dbspec/ directory and
// returns the root.
func writeCorpus(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "dbspec")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// fixtureText returns the application schema with each edit applied; every
// edit's old text must occur exactly once, so an edit never silently misses.
func fixtureText(t *testing.T, edits ...[2]string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureRoot, "dbspec", "flowmark.dbs"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, e := range edits {
		if n := strings.Count(text, e[0]); n != 1 {
			t.Fatalf("edit anchor %q occurs %d times, want 1", e[0], n)
		}
		text = strings.Replace(text, e[0], e[1], 1)
	}
	return text
}

// editedCorpus writes the application schema with the edits applied, plus
// any extra documents, and returns the root and the schema text.
func editedCorpus(t *testing.T, edits ...[2]string) (string, string) {
	t.Helper()
	text := fixtureText(t, edits...)
	return writeCorpus(t, map[string]string{"flowmark.dbs": text}), text
}

// tableLine is the 1-based line of the `table <name> {` declaration.
func tableLine(t *testing.T, text, name string) int {
	t.Helper()
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "table "+name+" ") || strings.HasPrefix(line, "table "+name+"{") {
			return i + 1
		}
	}
	t.Fatalf("table %s is not declared in the test text", name)
	return 0
}

// expectTableDiag requires a catalog diagnostic located at the table name of
// the schema text, whose message contains msg.
func expectTableDiag(t *testing.T, diags []catalog.Diagnostic, text, table, msg string) {
	t.Helper()
	line := tableLine(t, text, table)
	for _, d := range diags {
		if strings.Contains(d.Message, msg) {
			if d.File != "dbspec/flowmark.dbs" || d.Rule != "catalog" || d.Line != line || d.Column != len("table ")+1 {
				t.Fatalf("diagnostic %+v: want file dbspec/flowmark.dbs, rule catalog, line %d, column %d", d, line, len("table ")+1)
			}
			return
		}
	}
	t.Fatalf("no diagnostic containing %q in %+v", msg, diags)
}

// expectOnlyRejection requires the load to fail and the diagnostics to hold
// the expected rejection.
func expectRejected(t *testing.T, cat *catalog.Catalog, diags []catalog.Diagnostic) {
	t.Helper()
	if cat != nil {
		t.Fatalf("catalog returned for a rejected corpus")
	}
	if len(diags) == 0 {
		t.Fatalf("no diagnostics for a rejected corpus")
	}
}

func TestLoadFixtureCatalog(t *testing.T) {
	cat, diags := catalog.Load(fixtureRoot)
	if len(diags) != 0 {
		t.Fatalf("diagnostics for the valid fixture: %+v", diags)
	}
	if cat == nil {
		t.Fatal("nil catalog for the valid fixture")
	}
	if got := len(cat.Documents); got != 1 || cat.Documents[0].Name != "flowmark" {
		t.Fatalf("documents = %d, want the flowmark document", got)
	}
	if !strings.HasPrefix(cat.ManifestHash, "sha256:") {
		t.Fatalf("manifest hash = %q", cat.ManifestHash)
	}
	var names []string
	for _, tb := range cat.Tables {
		names = append(names, tb.Name)
	}
	if want := []string{"items", "people", "item_links", "item_history", "views"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("tables = %v, want %v", names, want)
	}

	items, ok := cat.Table("items")
	if !ok {
		t.Fatal("items missing")
	}
	if items.Storage != (catalog.Storage{Kind: catalog.StorageDocument, Shape: catalog.ShapeList}) {
		t.Fatalf("items storage = %+v", items.Storage)
	}
	if items.Key != "id" || items.Title != "title" || items.Body != "body" || items.Order != "position" {
		t.Fatalf("items key/title/body/order = %q %q %q %q", items.Key, items.Title, items.Body, items.Order)
	}
	if want := []string{"body", "evidence", "cause", "retry"}; !reflect.DeepEqual(items.Markdown, want) {
		t.Fatalf("items markdown = %v, want %v", items.Markdown, want)
	}
	if items.RowScope != 3 {
		t.Fatalf("items row scope = %d, want 3", items.RowScope)
	}
	sm := items.StateMachine
	if sm == nil {
		t.Fatal("items has no state machine")
	}
	if sm.Column != "state" {
		t.Fatalf("state column = %q", sm.Column)
	}
	if want := []string{"waiting", "doing", "done", "bypassed", "impossible"}; !reflect.DeepEqual(sm.States, want) {
		t.Fatalf("states = %v, want %v", sm.States, want)
	}
	if want := []string{"waiting"}; !reflect.DeepEqual(sm.Initial, want) {
		t.Fatalf("initial = %v, want %v", sm.Initial, want)
	}
	wantTransitions := []catalog.Transition{
		{From: "waiting", To: "doing"},
		{From: "doing", To: "done", Require: []string{"evidence"}},
		{From: "doing", To: "bypassed", Require: []string{"cause", "retry"}},
		{From: "bypassed", To: "doing"},
		{From: "waiting", To: "impossible", Require: []string{"cause"}},
	}
	if !reflect.DeepEqual(sm.Transitions, wantTransitions) {
		t.Fatalf("transitions = %+v", sm.Transitions)
	}
	wantTerminals := []catalog.Terminal{{State: "done"}, {State: "impossible"}}
	if !reflect.DeepEqual(sm.Terminals, wantTerminals) {
		t.Fatalf("terminals = %+v", sm.Terminals)
	}
	wantHistory := catalog.History{Table: "item_history", Row: "item", From: "from_state", To: "to_state", At: "at"}
	if sm.History != wantHistory {
		t.Fatalf("history = %+v, want %+v", sm.History, wantHistory)
	}
	wantGlyphs := map[string]string{"waiting": " ", "doing": "~", "done": "o", "bypassed": "!", "impossible": "-"}
	if !reflect.DeepEqual(sm.Checkboxes, wantGlyphs) {
		t.Fatalf("checkboxes = %v, want %v", sm.Checkboxes, wantGlyphs)
	}

	history, ok := cat.Table("item_history")
	if !ok {
		t.Fatal("item_history missing")
	}
	if history.Storage != (catalog.Storage{Kind: catalog.StorageBlock, Shape: catalog.ShapeTable, ParentForeignKey: "fk_history_item", Parent: "items"}) {
		t.Fatalf("item_history storage = %+v", history.Storage)
	}
	if history.RowScope != 5 {
		t.Fatalf("item_history row scope = %d, want 5", history.RowScope)
	}
	if want := []string{"evidence", "cause", "retry"}; !reflect.DeepEqual(history.Markdown, want) {
		t.Fatalf("item_history markdown = %v, want %v", history.Markdown, want)
	}

	people, ok := cat.Table("people")
	if !ok {
		t.Fatal("people missing")
	}
	if people.Storage != (catalog.Storage{Kind: catalog.StorageDocument, Shape: catalog.ShapeTable}) || people.Key != "id" || people.Title != "name" {
		t.Fatalf("people = %+v", people)
	}
	links, ok := cat.Table("item_links")
	if !ok {
		t.Fatal("item_links missing")
	}
	if links.Storage.Kind != catalog.StorageDocument || links.Key != "id" || links.StateMachine != nil {
		t.Fatalf("item_links = %+v", links)
	}
	views, ok := cat.Table("views")
	if !ok {
		t.Fatal("views missing")
	}
	if views.Title != "title" || views.Body != "body" || !reflect.DeepEqual(views.Markdown, []string{"body"}) {
		t.Fatalf("views = %+v", views)
	}
	if _, ok := cat.Table("missing"); ok {
		t.Fatal("Table reports a missing table")
	}
}

func TestLoadMultiDocumentSet(t *testing.T) {
	text := fixtureText(t,
		[2]string{"dbspec 1 flowmark\n", "dbspec 1 flowmark\n\nuse core { users }\n"},
		[2]string{"table views {", `table owner {
  id i64 identity
  user_id varchar(32)
  primary key (id)
  index ix_owner_user (user_id)
  foreign key fk_owner_user (user_id) references users (id) on delete restrict on update restrict
  settings {
    store document table
  }
}

table views {`},
	)
	core := `dbspec 1 core

table users {
  id varchar(32)
  name varchar(255)
  primary key (id)
  settings {
    key_prefix 'U'
    store document table
  }
}
`
	root := writeCorpus(t, map[string]string{"flowmark.dbs": text, "core.dbs": core})
	cat, diags := catalog.Load(root)
	if len(diags) != 0 || cat == nil {
		t.Fatalf("diagnostics for a valid two-document set: %+v", diags)
	}
	if len(cat.Documents) != 2 {
		t.Fatalf("documents = %d, want 2", len(cat.Documents))
	}
	if _, ok := cat.Table("users"); !ok {
		t.Fatal("the table of the other owned document is not in the catalog")
	}
	if _, ok := cat.Table("owner"); !ok {
		t.Fatal("owner is not in the catalog")
	}
}

func TestLoadDbspecDiagnosticsCarriedThrough(t *testing.T) {
	root, _ := editedCorpus(t, [2]string{"  primary key (id)\n  index ix_items_parent", "  primary key (idx)\n  index ix_items_parent"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	for _, d := range diags {
		if d.File == "dbspec/flowmark.dbs" && d.Rule == "catalog" && d.Line > 0 && d.Column > 0 && strings.HasPrefix(d.Message, "dbspec key:") {
			return
		}
	}
	t.Fatalf("no located dbspec key diagnostic in %+v", diags)
}

func TestLoadRejectsMissingCatalogDirectory(t *testing.T) {
	root := t.TempDir()
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	if diags[0].File != "dbspec" || !strings.Contains(diags[0].Message, "no dbspec/ directory") {
		t.Fatalf("diagnostics = %+v", diags)
	}
}

func TestLoadRejectsCorpusWithoutDocument(t *testing.T) {
	root := writeCorpus(t, map[string]string{"notes.md": "# notes\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	for _, d := range diags {
		if d.File == "dbspec" && strings.Contains(d.Message, "no .dbs document") {
			return
		}
	}
	t.Fatalf("no diagnostic for a corpus without a document: %+v", diags)
}

func TestLoadRejectsTableWithoutStore(t *testing.T) {
	root, text := editedCorpus(t, [2]string{"    store document list\n", ""})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "items", "no store setting")
}

func TestLoadRejectsCompositePrimaryKey(t *testing.T) {
	// identity must be the only key column, so the composite key drops it.
	root, text := editedCorpus(t,
		[2]string{"  id i64 identity\n  from_item", "  id i64\n  from_item"},
		[2]string{"  kind varchar(16)\n  primary key (id)", "  kind varchar(16)\n  primary key (id, kind)"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "item_links", "composite primary key")
}

// aesColumns adds a bytes column encrypted with aes and its key version.
var aesColumns = [2]string{"  name varchar(255)\n", "  name varchar(255)\n  secret bytes null\n  secret_version i32\n"}

func TestLoadRejectsAESCodec(t *testing.T) {
	root, text := editedCorpus(t, aesColumns,
		[2]string{"    title name\n", "    title name\n    codec secret aes\n    aes_version secret_version\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "people", "uses aes")
}

func TestLoadRejectsAESVersionSetting(t *testing.T) {
	root, text := editedCorpus(t, aesColumns,
		[2]string{"    title name\n", "    title name\n    codec secret aes\n    aes_version secret_version\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "people", "aes_version setting")
}

func TestLoadRejectsBlindIndexSetting(t *testing.T) {
	root, text := editedCorpus(t,
		[2]string{"  name varchar(255)\n", "  name varchar(255)\n  secret bytes null\n  secret_index varchar(64) null\n  secret_version i32\n  index ix_people_secret (secret_index)\n"},
		[2]string{"    title name\n", "    title name\n    codec secret aes\n    aes_version secret_version\n    blind_index secret secret_index\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "people", "blind_index setting")
}

func TestLoadRejectsAuditSetting(t *testing.T) {
	audited := `table audit_record {
  seq i64 identity
  request_id varchar(64)
  primary key (seq)
  settings {
    store document table
  }
}

table service {
  id i64 identity
  name varchar(191)
  audit_seq i64
  primary key (id)
  index ix_service_audit (audit_seq)
  foreign key fk_service_audit (audit_seq) references audit_record (seq) on delete restrict on update restrict
  settings {
    store document table
    audit into service_history column audit_seq references audit_record action change previous previous_audit_seq
  }
}

table service_history {
  history_id i64 identity
  change varchar(8)
  previous_audit_seq i64 null
  id i64
  name varchar(191)
  audit_seq i64
  primary key (history_id)
  index ix_service_history_row (id)
  settings {
    store document table
  }
}

table views {`
	root, text := editedCorpus(t, [2]string{"table views {", audited})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "service", "audit setting")
}

func TestLoadRejectsImmutableSetting(t *testing.T) {
	root, text := editedCorpus(t, [2]string{"    title name\n", "    title name\n    immutable\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "people", "immutable setting")
}

func TestLoadRejectsCascadeAction(t *testing.T) {
	root, text := editedCorpus(t, [2]string{
		"foreign key fk_item_links_from (from_item) references items (id) on delete restrict",
		"foreign key fk_item_links_from (from_item) references items (id) on delete cascade",
	})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "item_links", "cascade")
}

func TestLoadRejectsSetNullAction(t *testing.T) {
	root, text := editedCorpus(t, [2]string{
		"foreign key fk_items_parent (parent) references items (id) on delete restrict",
		"foreign key fk_items_parent (parent) references items (id) on delete set_null",
	})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "items", "set_null")
}

func TestLoadRejectsExternalUse(t *testing.T) {
	root, _ := editedCorpus(t, [2]string{"dbspec 1 flowmark\n", "dbspec 1 flowmark\n\nuse core { users }\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	for _, d := range diags {
		if d.File == "dbspec/flowmark.dbs" && d.Rule == "catalog" && d.Line == 3 && strings.Contains(d.Message, "not in the declared document set") {
			return
		}
	}
	t.Fatalf("no located use diagnostic at line 3 in %+v", diags)
}

func TestLoadRejectsStateMachineWithoutHistory(t *testing.T) {
	root, text := editedCorpus(t, [2]string{"    state_machine state history item_history row item from from_state to to_state at at\n", ""})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "items", "no history")
}

func TestLoadRejectsStateMachineWithoutInitial(t *testing.T) {
	root, text := editedCorpus(t, [2]string{"    state_machine state initial waiting\n", ""})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "items", "no initial state")
}

// checkboxLines are the five checkbox lines of the fixture's items table, which
// is a list storage with a state machine.
const checkboxLines = "    checkbox state waiting ' '\n    checkbox state doing '~'\n    checkbox state done 'o'\n    checkbox state bypassed '!'\n    checkbox state impossible '-'\n"

// TestLoadListStateRules pins that a list storage of a state machine declares
// at least one checkbox. dbspec then covers every state with a checkbox, but
// it accepts a state machine with none at all, so the rule is flowmark's.
func TestLoadListStateRules(t *testing.T) {
	cases := []struct {
		name  string
		edits [][2]string
		msg   string // the rejection's message; empty for an accepted catalog
	}{
		{name: "accepts a list with checkboxes", edits: nil},
		{name: "rejects a list without a checkbox", edits: [][2]string{{checkboxLines, ""}}, msg: "declares no checkbox"},
		{name: "accepts a table without a checkbox", edits: [][2]string{{checkboxLines, ""}, {"    store document list\n", "    store document table\n"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, text := editedCorpus(t, c.edits...)
			cat, diags := catalog.Load(root)
			if c.msg == "" {
				if len(diags) != 0 || cat == nil {
					t.Fatalf("diagnostics = %+v, want an accepted catalog", diags)
				}
				return
			}
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, "items", c.msg)
		})
	}
}

// TestLoadRejectsNullableStateColumn pins that the state column of a state
// machine is non-null. dbspec refuses a nullable one in every storage, so the
// rejection is its own, located at the state line.
func TestLoadRejectsNullableStateColumn(t *testing.T) {
	root, _ := editedCorpus(t, [2]string{"  state varchar(16)\n", "  state varchar(16) null\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	if !slices.ContainsFunc(diags, func(d catalog.Diagnostic) bool {
		return d.File == "dbspec/flowmark.dbs" && d.Rule == "catalog" && strings.Contains(d.Message, "needs a non-null")
	}) {
		t.Fatalf("diagnostics = %+v, want the non-null state rule", diags)
	}
}

func TestLoadRejectsListWithoutTitle(t *testing.T) {
	root, text := editedCorpus(t, [2]string{"    store document list\n    title title\n", "    store document list\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "items", "list storage needs a title")
}

func TestLoadRejectsFilesWithoutTitle(t *testing.T) {
	notes := `table notes {
  id varchar(32)
  body text
  primary key (id)
  settings {
    key_prefix 'N'
    store files
    body body
    markdown body
  }
}

table views {`
	root, text := editedCorpus(t, [2]string{"table views {", notes})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "notes", "files storage needs a title")
}

func TestLoadRejectsHeadingPastH6(t *testing.T) {
	// items (document list, rows at level 3) > item_history (block, rows at 5)
	// > item_note (block, rows at 7).
	note := `table item_note {
  id i64 identity
  history i64
  body text
  primary key (id)
  index ix_item_note_history (history)
  foreign key fk_note_history (history) references item_history (id) on delete restrict on update restrict
  settings {
    store block fk_note_history table
    body body
    markdown body
  }
}

table views {`
	root, text := editedCorpus(t, [2]string{"table views {", note})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "item_note", "level 7")
}

func TestLoadRejectsBlockCycle(t *testing.T) {
	node := `table node {
  id varchar(32)
  parent varchar(32) null
  primary key (id)
  index ix_node_parent (parent)
  foreign key fk_node_parent (parent) references node (id) on delete restrict on update restrict
  settings {
    key_prefix 'N'
    store block fk_node_parent table
  }
}

table views {`
	root, text := editedCorpus(t, [2]string{"table views {", node})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "node", "cycle")
}

func TestCatalogHasNoTranslations(t *testing.T) {
	if _, ok := reflect.TypeOf(catalog.Catalog{}).FieldByName("Translations"); ok {
		t.Fatal("Catalog still has a Translations field")
	}
}

// TestLoadDbspecEntries pins what dbspec/ may hold: the .dbs set only; a
// hidden entry is skipped; any other file or folder is a catalog diagnostic.
func TestLoadDbspecEntries(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]string
		diag  string // the file of the diagnostic, relative to the root; "" accepts
		msg   string
	}{
		{name: "hidden file is skipped", extra: map[string]string{".DS_Store": "x"}},
		{name: "hidden folder is skipped", extra: map[string]string{".cache/": ""}},
		{name: "settings file is removed", extra: map[string]string{"flowmark.json": `{"translations": ["ko"]}`}, diag: "dbspec/flowmark.json", msg: "unexpected file in dbspec/: flowmark.json"},
		{name: "text file is a diagnostic", extra: map[string]string{"notes.txt": "x"}, diag: "dbspec/notes.txt", msg: "unexpected file in dbspec/: notes.txt"},
		{name: "upper suffix is a diagnostic", extra: map[string]string{"extra.DBS": "x"}, diag: "dbspec/extra.DBS", msg: "unexpected file in dbspec/: extra.DBS"},
		{name: "folder is a diagnostic", extra: map[string]string{"sub/": ""}, diag: "dbspec/sub", msg: "unexpected directory in dbspec/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := map[string]string{"flowmark.dbs": fixtureText(t)}
			for k, v := range c.extra {
				files[k] = v
			}
			root := writeCorpus(t, files)
			cat, diags := catalog.Load(root)
			if c.diag == "" {
				if len(diags) != 0 || cat == nil {
					t.Fatalf("diagnostics = %+v, want an accepted catalog", diags)
				}
				return
			}
			expectRejected(t, cat, diags)
			for _, d := range diags {
				if d.File == c.diag && d.Rule == "catalog" && d.Line == 1 && d.Column == 1 && strings.Contains(d.Message, c.msg) {
					return
				}
			}
			t.Fatalf("no diagnostic %q at %s in %+v", c.msg, c.diag, diags)
		})
	}
}

// TestLoadRejectsReservedTableName pins the Windows device names as table
// names. orm's name grammar admits lowercase names only, so the names are
// lowercase; a name that only contains a device name is accepted.
func TestLoadRejectsReservedTableName(t *testing.T) {
	reject := []string{"con", "prn", "aux", "nul", "com0", "com9", "lpt0", "lpt9"}
	accept := []string{"cons", "console", "com10", "lpt", "con_1", "prnt"}
	for _, name := range reject {
		t.Run("reject "+name, func(t *testing.T) {
			root, text := editedCorpus(t, [2]string{"table views {", "table " + name + " {"})
			cat, diags := catalog.Load(root)
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, name, "reserved file name")
		})
	}
	for _, name := range accept {
		t.Run("accept "+name, func(t *testing.T) {
			root, _ := editedCorpus(t, [2]string{"table views {", "table " + name + " {"})
			cat, diags := catalog.Load(root)
			if len(diags) != 0 || cat == nil {
				t.Fatalf("diagnostics = %+v, want an accepted catalog", diags)
			}
		})
	}
}

// childBlock is a block table under items, named name, with one row per item.
func childBlock(name string) string {
	return fmt.Sprintf(`table %[1]s {
  id i64 identity
  item varchar(32)
  label varchar(64)
  primary key (id)
  index ix_%[1]s_item (item)
  foreign key fk_%[1]s_item (item) references items (id) on delete restrict on update restrict
  settings {
    store block fk_%[1]s_item table
  }
}

table views {`, name)
}

// TestLoadRejectsChildNamedLikeMarkdownColumn pins that a child block table
// may not be named like a markdown column of its parent.
func TestLoadRejectsChildNamedLikeMarkdownColumn(t *testing.T) {
	cases := []struct {
		name   string
		table  string
		accept bool
	}{
		{name: "child named like a column", table: "evidence"},
		{name: "child named like another column", table: "cause"},
		{name: "child named like no column", table: "notes", accept: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, text := editedCorpus(t, [2]string{"table views {", childBlock(c.table)})
			cat, diags := catalog.Load(root)
			if c.accept {
				if len(diags) != 0 || cat == nil {
					t.Fatalf("diagnostics = %+v, want an accepted catalog", diags)
				}
				return
			}
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, c.table, "named like markdown column")
		})
	}
}

// noteGroup is a files table note with a block tag and a grandchild block
// review; its markdown column is named column.
func noteGroup(column string) string {
	return fmt.Sprintf(`table note {
  id varchar(32)
  title varchar(255)
  body text
  %[1]s text null
  primary key (id)
  settings {
    key_prefix 'N'
    store files
    title title
    body body
    markdown body
    markdown %[1]s
  }
}

table tag {
  id i64 identity
  note varchar(32)
  label varchar(64)
  primary key (id)
  index ix_tag_note (note)
  foreign key fk_tag_note (note) references note (id) on delete restrict on update restrict
  settings {
    store block fk_tag_note table
  }
}

table review {
  id i64 identity
  tag_id i64
  primary key (id)
  index ix_review_tag (tag_id)
  foreign key fk_review_tag (tag_id) references tag (id) on delete restrict on update restrict
  settings {
    store block fk_review_tag table
  }
}

table views {`, column)
}

// itemNotes is a block table under items with one nullable markdown column
// named column.
func itemNotes(column string) string {
	return fmt.Sprintf(`table item_notes {
  id i64 identity
  item varchar(32)
  body text
  %[1]s text null
  primary key (id)
  index ix_item_notes_item (item)
  foreign key fk_item_notes_item (item) references items (id) on delete restrict on update restrict
  settings {
    store block fk_item_notes_item table
    body body
    markdown body
    markdown %[1]s
  }
}

table views {`, column)
}

// notesDoc is a document table of its own file, named notes.
const notesDoc = `table notes {
  id varchar(32)
  title varchar(255)
  primary key (id)
  settings {
    key_prefix 'M'
    store document table
    title title
  }
}

table views {`

// TestLoadAcceptsNameOfSameFileTable pins that a markdown column or a block
// table may be named like another table of its file. The same-file name rule
// is withdrawn: a detail anchor is `<table>-row-<key>`, which no block or
// markdown-column section heading slugs to, so such a name cannot collide.
func TestLoadAcceptsNameOfSameFileTable(t *testing.T) {
	cases := []struct {
		name  string
		edits [][2]string
	}{
		{
			name:  "column of items named like a document of another file",
			edits: [][2]string{{"  retry text null\n  primary key (id)\n  index ix_items_parent", "  retry text null\n  notes text null\n  primary key (id)\n  index ix_items_parent"}, {"    markdown retry\n    checkbox", "    markdown retry\n    markdown notes\n    checkbox"}, {"table views {", notesDoc}},
		},
		{
			name:  "column of a block named like its document",
			edits: [][2]string{{"table views {", itemNotes("items")}},
		},
		{
			name:  "column of a block named like no table of its file",
			edits: [][2]string{{"table views {", itemNotes("people")}},
		},
		{
			name:  "column of a files row named like its grandchild block",
			edits: [][2]string{{"table views {", noteGroup("review")}},
		},
		{
			name:  "column of a files row named like no table of its file",
			edits: [][2]string{{"table views {", noteGroup("summary")}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, _ := editedCorpus(t, c.edits...)
			cat, diags := catalog.Load(root)
			if len(diags) != 0 || cat == nil {
				t.Fatalf("diagnostics = %+v, want an accepted catalog", diags)
			}
		})
	}
}

// tagTables is a document table tag with a unique non-key column code, and a
// table tag_use whose foreign key references the columns refs of tag.
func tagTables(refs string) string {
	return fmt.Sprintf(`table tag {
  id varchar(32)
  code varchar(32)
  primary key (id)
  unique uq_tag_code (code)
  settings {
    key_prefix 'G'
    store document table
    title code
  }
}

table tag_use {
  id i64 identity
  tag varchar(32)
  primary key (id)
  index ix_tag_use_tag (tag)
  foreign key fk_tag_use_tag (tag) references tag (%[1]s) on delete restrict on update restrict
  settings {
    store document table
  }
}

table views {`, refs)
}

// gridTables is a document table grid with a composite unique key on x and y,
// and a table slot whose composite foreign key references that key.
const gridTables = `table grid {
  id varchar(32)
  x varchar(32)
  y varchar(32)
  primary key (id)
  unique uq_grid_xy (x, y)
  settings {
    key_prefix 'G'
    store document table
    title x
  }
}

table slot {
  id i64 identity
  gx varchar(32)
  gy varchar(32)
  primary key (id)
  index ix_slot_grid (gx, gy)
  foreign key fk_slot_grid (gx, gy) references grid (x, y) on delete restrict on update restrict
  settings {
    store document table
  }
}

table views {`

// TestLoadAcceptsOneColumnPrimaryKeyForeignKey pins that a foreign key of a
// table references the one-column primary key of a table, the self foreign
// key parent of items included.
func TestLoadAcceptsOneColumnPrimaryKeyForeignKey(t *testing.T) {
	cases := []struct {
		name  string
		edits [][2]string
	}{
		{name: "self foreign key parent of items"},
		{name: "foreign key to the primary key of another table", edits: [][2]string{{"table views {", tagTables("id")}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, _ := editedCorpus(t, c.edits...)
			cat, diags := catalog.Load(root)
			if len(diags) != 0 || cat == nil {
				t.Fatalf("diagnostics = %+v, want an accepted catalog", diags)
			}
		})
	}
}

// expectForeignKeyDiag requires a catalog diagnostic located at the name of
// the foreign key declaration in the schema text, whose message contains msg.
func expectForeignKeyDiag(t *testing.T, diags []catalog.Diagnostic, text, fk, msg string) {
	t.Helper()
	idx := strings.Index(text, "foreign key "+fk+" (")
	if idx < 0 {
		t.Fatalf("foreign key %s is not declared in the test text", fk)
	}
	line := 1 + strings.Count(text[:idx], "\n")
	column := idx - (strings.LastIndex(text[:idx], "\n") + 1) + len("foreign key ") + 1
	for _, d := range diags {
		if strings.Contains(d.Message, msg) {
			if d.File != "dbspec/flowmark.dbs" || d.Rule != "catalog" || d.Line != line || d.Column != column {
				t.Fatalf("diagnostic %+v: want file dbspec/flowmark.dbs, rule catalog, line %d, column %d", d, line, column)
			}
			return
		}
	}
	t.Fatalf("no diagnostic containing %q in %+v", msg, diags)
}

// TestLoadRejectsForeignKeyNotToPrimaryKey pins that a composite foreign key,
// or a foreign key to a column that is not the one-column primary key of its
// referenced table, is a located catalog diagnostic at the foreign key.
func TestLoadRejectsForeignKeyNotToPrimaryKey(t *testing.T) {
	cases := []struct {
		name  string
		edits [][2]string
		fk    string
		msg   string
	}{
		{
			name:  "composite foreign key",
			edits: [][2]string{{"table views {", gridTables}},
			fk:    "fk_slot_grid",
			msg:   "is composite",
		},
		{
			name:  "foreign key to a unique non-key column",
			edits: [][2]string{{"table views {", tagTables("code")}},
			fk:    "fk_tag_use_tag",
			msg:   "not the one-column primary key",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, text := editedCorpus(t, c.edits...)
			cat, diags := catalog.Load(root)
			expectRejected(t, cat, diags)
			expectForeignKeyDiag(t, diags, text, c.fk, c.msg)
		})
	}
}

// keyedTable is a document table keyed whose one-column primary key has the
// type key, inserted before the table views of the fixture.
func keyedTable(key string) string {
	return fmt.Sprintf(`table keyed {
  id %s
  name varchar(255)
  primary key (id)
  settings {
    store document table
  }
}

table views {`, key)
}

// TestLoadKeyColumnTypes pins that the primary key of a markdown table is an
// integer, bool, varchar, uuid or date column: a key of any other type is a
// located catalog diagnostic, since the key shape cannot carry its text.
func TestLoadKeyColumnTypes(t *testing.T) {
	accepted := []string{"i16", "i32", "i64", "bool", "varchar(32)", "uuid", "date"}
	for _, key := range accepted {
		t.Run("accepts "+key, func(t *testing.T) {
			root, _ := editedCorpus(t, [2]string{"table views {", keyedTable(key)})
			cat, diags := catalog.Load(root)
			if len(diags) != 0 || cat == nil {
				t.Fatalf("diagnostics = %+v, want an accepted catalog", diags)
			}
		})
	}
	rejected := []string{"time(0)", "datetime(0)", "decimal(4,2)", "f64"}
	for _, key := range rejected {
		t.Run("rejects "+key, func(t *testing.T) {
			root, text := editedCorpus(t, [2]string{"table views {", keyedTable(key)})
			cat, diags := catalog.Load(root)
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, "keyed", "is not a key type")
		})
	}
	// bytes and text keys are refused by dbspec's own key rule, located at
	// the key column, before this rule runs; the cases pin that they stay
	// refused.
	for _, key := range []string{"bytes", "text"} {
		t.Run("rejects "+key, func(t *testing.T) {
			root, _ := editedCorpus(t, [2]string{"table views {", keyedTable(key)})
			cat, diags := catalog.Load(root)
			expectRejected(t, cat, diags)
			if !slices.ContainsFunc(diags, func(d catalog.Diagnostic) bool {
				return strings.Contains(d.Message, "cannot be part of a key")
			}) {
				t.Fatalf("diagnostics = %+v, want the dbspec key rule", diags)
			}
		})
	}
}

// TestLoadIdentityKeyType pins that identity is allowed only on an i64 primary
// key column: max+1 is an integer sequence, and dbspec allows identity only on
// an i64 column, so any other key type is a located catalog diagnostic at the
// identity column.
func TestLoadIdentityKeyType(t *testing.T) {
	root, _ := editedCorpus(t, [2]string{"table views {", keyedTable("i64 identity")})
	cat, diags := catalog.Load(root)
	if len(diags) != 0 || cat == nil {
		t.Fatalf("diagnostics = %+v, want an accepted catalog", diags)
	}
	rejected := []string{"i32 identity", "varchar(32) identity", "uuid identity", "date identity", "bool identity"}
	for _, key := range rejected {
		t.Run("rejects "+key, func(t *testing.T) {
			root, _ := editedCorpus(t, [2]string{"table views {", keyedTable(key)})
			cat, diags := catalog.Load(root)
			expectRejected(t, cat, diags)
			if !slices.ContainsFunc(diags, func(d catalog.Diagnostic) bool {
				return d.File == "dbspec/flowmark.dbs" && d.Rule == "catalog" && d.Line > 0 && strings.Contains(d.Message, "an identity column has type i64")
			}) {
				t.Fatalf("diagnostics = %+v, want the located identity diagnostic", diags)
			}
		})
	}
}
