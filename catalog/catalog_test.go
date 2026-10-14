package catalog_test

import (
	"fmt"
	"github.com/polyspec/flowmark/internal/corpustest"
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
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := corpustest.WriteSetFile(dir); err != nil {
		t.Fatal(err)
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

// itemsList stores the fixture's items as a document list, rows at row scope 3,
// in place of the application schema's files storage (row scope 2), for the
// tests whose list rules or heading arithmetic stand on a list of items.
var itemsList = [2]string{"    store files\n", "    store document list\n"}

// listCorpus is editedCorpus with items stored as a document list.
func listCorpus(t *testing.T, edits ...[2]string) (string, string) {
	t.Helper()
	return editedCorpus(t, append([][2]string{itemsList}, edits...)...)
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
	// Spec 100 Tables: one file an item, its history and its outgoing links
	// blocks of that file, so no file grows with the life of the project.
	if items.Storage != (catalog.Storage{Kind: catalog.StorageFiles}) {
		t.Fatalf("items storage = %+v", items.Storage)
	}
	if items.Key != "id" || items.Title != "title" || items.Body != "body" || items.Order != "position" {
		t.Fatalf("items key/title/body/order = %q %q %q %q", items.Key, items.Title, items.Body, items.Order)
	}
	if want := []string{"body", "evidence", "cause", "retry"}; !reflect.DeepEqual(items.Markdown, want) {
		t.Fatalf("items markdown = %v, want %v", items.Markdown, want)
	}
	if items.RowScope != 2 {
		t.Fatalf("items row scope = %d, want 2", items.RowScope)
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
	if history.RowScope != 4 {
		t.Fatalf("item_history row scope = %d, want 4", history.RowScope)
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
	if links.Storage != (catalog.Storage{Kind: catalog.StorageBlock, Shape: catalog.ShapeTable, ParentForeignKey: "fk_item_links_from", Parent: "items"}) || links.RowScope != 4 || links.Key != "id" || links.StateMachine != nil {
		t.Fatalf("item_links = %+v", links)
	}
	views, ok := cat.Table("views")
	if !ok {
		t.Fatal("views missing")
	}
	if views.Storage != (catalog.Storage{Kind: catalog.StorageDocument, Shape: catalog.ShapeTable}) || views.Title != "title" || views.Body != "body" || !reflect.DeepEqual(views.Markdown, []string{"body"}) {
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

// TestLoadNamesDocumentsByTheParser checks that a document is named by the header that the parser reads: a header the
// parser refuses names no document, so it repeats no name of the set and reports only its header diagnostic.
func TestLoadNamesDocumentsByTheParser(t *testing.T) {
	good := "dbspec 1 a\n\ntable a_rows {\n  id varchar(32)\n  primary key (id)\n  settings {\n    store files\n    title id\n  }\n}\n"
	bad := "dbspec 1 a extra\n\ntable b_rows {\n  id varchar(32)\n  primary key (id)\n  settings {\n    store files\n    title id\n  }\n}\n"
	_, diags := catalog.Load(writeCorpus(t, map[string]string{"a.dbs": good, "b.dbs": bad}))
	if len(diags) != 1 || diags[0].File != "dbspec/b.dbs" || !strings.Contains(diags[0].Message, "dbspec header") {
		t.Fatalf("diagnostics %+v, want only the header diagnostic of dbspec/b.dbs", diags)
	}
}

// TestLoadSetFile checks the rules of the set file dbspec/dbspec-set.json (docs/spec/30-schema.md, The catalog): each
// rejecting case names its file and its cause, and the accepting cases load.
func TestLoadSetFile(t *testing.T) {
	doc := func(name string) string {
		return "dbspec 1 " + name + "\n\ntable " + name + "_rows {\n  id varchar(32)\n  primary key (id)\n  settings {\n    store files\n    title id\n  }\n}\n"
	}
	for _, c := range []struct {
		name  string
		files map[string]string
		file  string
		want  string
	}{
		{"no set file", map[string]string{"notes.md": "# notes\n"}, "dbspec/dbspec-set.json", "no dbspec/dbspec-set.json"},
		{"a document the set does not list", map[string]string{"a.dbs": doc("a"), "b.dbs": doc("b"), "dbspec-set.json": `{"schema":1,"documents":["a.dbs"]}`}, "dbspec/b.dbs", "dbspec/b.dbs is not listed in dbspec/dbspec-set.json"},
		{"a listed document outside dbspec", map[string]string{"a.dbs": doc("a"), "dbspec-set.json": `{"schema":1,"documents":["a.dbs","../outside.dbs"]}`}, "dbspec/dbspec-set.json", "set.path"},
		{"a listed document in a folder", map[string]string{"a.dbs": doc("a"), "sub/": "", "sub/c.dbs": doc("c"), "dbspec-set.json": `{"schema":1,"documents":["a.dbs","sub/c.dbs"]}`}, "dbspec/dbspec-set.json", "listed document dbspec/sub/c.dbs is not directly in dbspec/"},
		{"an external document", map[string]string{"a.dbs": doc("a"), "dbspec-set.json": `{"schema":1,"documents":["a.dbs"],"external":["../ext/e.dbs"]}`}, "dbspec/dbspec-set.json", ""},
		{"a set file that is not JSON", map[string]string{"a.dbs": doc("a"), "dbspec-set.json": `{`}, "dbspec/dbspec-set.json", "set.json"},
		{"a missing listed document", map[string]string{"a.dbs": doc("a"), "dbspec-set.json": `{"schema":1,"documents":["a.dbs","gone.dbs"]}`}, "dbspec/dbspec-set.json", "gone.dbs"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := writeCorpus(t, c.files)
			if c.name == "an external document" {
				if err := os.MkdirAll(filepath.Join(root, "ext"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "ext", "e.dbs"), []byte(doc("e")), 0o644); err != nil {
					t.Fatal(err)
				}
				c.want = "external document ext/e.dbs: the catalog uses no document outside its set"
			}
			cat, diags := catalog.Load(root)
			expectRejected(t, cat, diags)
			for _, d := range diags {
				if d.File == c.file && strings.Contains(d.Message, c.want) {
					return
				}
			}
			t.Fatalf("no diagnostic at %s containing %q in %+v", c.file, c.want, diags)
		})
	}
	for _, c := range []struct {
		name  string
		files map[string]string
	}{
		{"every document listed", map[string]string{"a.dbs": doc("a"), "b.dbs": doc("b"), "dbspec-set.json": `{"schema":1,"documents":["b.dbs","a.dbs"]}`}},
		{"a hidden entry beside the set", map[string]string{"a.dbs": doc("a"), ".keep": "", "dbspec-set.json": `{"schema":1,"documents":["a.dbs"]}`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			cat, diags := catalog.Load(writeCorpus(t, c.files))
			if len(diags) > 0 {
				t.Fatalf("diagnostics: %+v", diags)
			}
			if got := strings.Join(cat.Files, " "); got != "dbspec/a.dbs dbspec/b.dbs" && got != "dbspec/a.dbs" {
				t.Fatalf("files %s, want the listed documents in file name order", got)
			}
		})
	}
}

func TestLoadRejectsTableWithoutStore(t *testing.T) {
	root, text := editedCorpus(t, [2]string{"    store files\n", ""})
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

// checkboxLines are the five checkbox lines of the fixture's items table, a
// state machine whose glyphs a list storage of items writes.
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
		{name: "accepts a list with checkboxes", edits: [][2]string{itemsList}},
		{name: "rejects a list without a checkbox", edits: [][2]string{itemsList, {checkboxLines, ""}}, msg: "declares no checkbox"},
		{name: "accepts a table without a checkbox", edits: [][2]string{{checkboxLines, ""}, {"    store files\n", "    store document table\n"}}},
		{name: "accepts files without a checkbox", edits: [][2]string{{checkboxLines, ""}}},
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
	root, text := listCorpus(t, [2]string{"    store document list\n    title title\n", "    store document list\n"})
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
	// > item_note (block, rows at 7). The markdown column note is a section of
	// the rows at level 7.
	note := `table item_note {
  id i64 identity
  history i64
  note text null
  primary key (id)
  index ix_item_note_history (history)
  foreign key fk_note_history (history) references item_history (id) on delete restrict on update restrict
  settings {
    store block fk_note_history table
    markdown note
  }
}

table views {`
	root, text := listCorpus(t, [2]string{"table views {", note})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "item_note", "level 7")
}

// TestLoadHeadingPastH6IsNeeded pins that the heading-depth rule refuses a layout
// only when its rows write a heading past H6. A row writes its detail heading one
// level above its scope when it can have a body, a markdown column or a child
// block, and its sections at its scope when it can have a markdown column or a
// child block. A block at level 7 with neither writes no heading past H6, but a
// child block under it writes its section at level 7, so the parent is refused.
const leafChild = "table item_leaf_child {\n  id i64 identity\n  leaf i64\n  primary key (id)\n  index ix_item_leaf_child_leaf (leaf)\n  foreign key fk_leaf_child_leaf (leaf) references item_leaf (id) on delete restrict on update restrict\n  settings {\n    store block fk_leaf_child_leaf table\n  }\n}\n\n"

func TestLoadHeadingPastH6IsNeeded(t *testing.T) {
	cases := []struct {
		name     string
		columns  string
		settings string
		child    string // a block table declared under item_leaf, or empty
		refused  bool
	}{
		{"no body, markdown column or child block", "", "", "", false},
		{"a body alone, whose detail heading is at level 6", "  body text\n", "    body body\n    markdown body\n", "", false},
		{"a markdown column, whose section is at level 7", "  note text null\n", "    markdown note\n", "", true},
		{"a child block alone, whose section is at level 7", "", "", leafChild, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			leaf := "table item_leaf {\n  id i64 identity\n  history i64\n" + c.columns +
				"  primary key (id)\n  index ix_item_leaf_history (history)\n" +
				"  foreign key fk_leaf_history (history) references item_history (id) on delete restrict on update restrict\n" +
				"  settings {\n    store block fk_leaf_history table\n" + c.settings + "  }\n}\n\n" + c.child + "table views {"
			root, text := listCorpus(t, [2]string{"table views {", leaf})
			cat, diags := catalog.Load(root)
			if !c.refused {
				if cat == nil || len(diags) > 0 {
					t.Fatalf("a layout that writes no heading past H6 is refused: %+v", diags)
				}
				return
			}
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, "item_leaf", "level 7")
			for _, d := range diags {
				if strings.Contains(d.Message, "item_leaf_child") {
					t.Fatalf("diagnostic %+v on the child block, want none", d)
				}
			}
		})
	}
}

// TestLoadRejectsBodyAtScopeEight pins that a block table at scope 8 with a body
// is refused at level 7, its detail heading. The files root notes (scope 2) and
// the block tables note_a (4) and note_b (6) put note_c at scope 8; each of the
// three has a child block, so none writes a heading of its own past H6. The same
// body shape at scope 7 is accepted, as the body-alone case above pins.
func TestLoadRejectsBodyAtScopeEight(t *testing.T) {
	chain := `table notes {
  id varchar(32)
  title varchar(255)
  primary key (id)
  settings {
    key_prefix 'N'
    store files
    title title
  }
}

table note_a {
  id i64 identity
  note varchar(32)
  primary key (id)
  index ix_note_a_note (note)
  foreign key fk_note_a_note (note) references notes (id) on delete restrict on update restrict
  settings {
    store block fk_note_a_note table
  }
}

table note_b {
  id i64 identity
  note_a i64
  primary key (id)
  index ix_note_b_note_a (note_a)
  foreign key fk_note_b_note_a (note_a) references note_a (id) on delete restrict on update restrict
  settings {
    store block fk_note_b_note_a table
  }
}

table note_c {
  id i64 identity
  note_b i64
  body text
  primary key (id)
  index ix_note_c_note_b (note_b)
  foreign key fk_note_c_note_b (note_b) references note_b (id) on delete restrict on update restrict
  settings {
    store block fk_note_c_note_b table
    body body
    markdown body
  }
}

table views {`
	root, text := editedCorpus(t, [2]string{"table views {", chain})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "note_c", "level 7")
}

// requireSchema is the state table subtasks, whose machine requires column x,
// and its history table subtask_history; both declare x with the type typ. A
// block table stands under items, stored a document list (listCorpus), so its row scope is 5 and its history table's
// is 7; a document table has row scope 3 and its history table scope 5. stateMark
// and historyMark declare x markdown on the state table and on the history table.
func requireSchema(block bool, typ string, stateMark, historyMark bool) string {
	state := "table subtasks {\n  id i64 identity\n"
	store := "    store document table\n"
	if block {
		state += "  item varchar(32)\n"
		store = "    store block fk_subtasks_item table\n"
	}
	state += "  state varchar(16)\n  x " + typ + " null\n  primary key (id)\n"
	if block {
		state += "  index ix_subtasks_item (item)\n  foreign key fk_subtasks_item (item) references items (id) on delete restrict on update restrict\n"
	}
	state += "  settings {\n" + store +
		"    state_machine state initial open\n" +
		"    state_machine state open -> closed require (x)\n" +
		"    state_machine state history subtask_history row subtask from from_state to to_state at at\n"
	if stateMark {
		state += "    markdown x\n"
	}
	state += "  }\n}\n\n"

	history := "table subtask_history {\n  id i64 identity\n  subtask i64\n  from_state varchar(16)\n  to_state varchar(16)\n  at datetime(6)\n  x " + typ + " null\n  primary key (id)\n  index ix_subtask_history_subtask (subtask)\n  foreign key fk_subtask_history_subtask (subtask) references subtasks (id) on delete restrict on update restrict\n  settings {\n    store block fk_subtask_history_subtask table\n"
	if historyMark {
		history += "    markdown x\n"
	}
	history += "  }\n}\n\ntable views {"
	return state + history
}

// TestLoadHistoryStorage pins the ruling of spec 30 on the storage of a history
// table: stored `document table`, the history rows have row scope 3 at any depth
// of the state table, so a block machine at row scope 5 that requires a markdown
// column loads; the same history stored as a block under its state table is
// refused past H6.
func TestLoadHistoryStorage(t *testing.T) {
	const blockStore = "    store block fk_subtask_history_subtask table\n"
	cases := []struct {
		name     string
		document bool // the history table is stored document table
	}{
		{"30#the-history-table: a history stored document table under a block machine at row scope 5 loads at row scope 3", true},
		{"30#the-history-table: the same history stored as a block is refused past H6", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			schema := requireSchema(true, "text", true, true)
			if strings.Count(schema, blockStore) != 1 {
				t.Fatalf("the history store line %q is not in the schema once", blockStore)
			}
			if c.document {
				schema = strings.Replace(schema, blockStore, "    store document table\n", 1)
			}
			root, text := listCorpus(t, [2]string{"table views {", schema})
			cat, diags := catalog.Load(root)
			if !c.document {
				expectRejected(t, cat, diags)
				expectTableDiag(t, diags, text, "subtask_history", "level 7")
				return
			}
			if cat == nil || len(diags) > 0 {
				t.Fatalf("a history stored document table is refused: %+v", diags)
			}
			state, _ := cat.Table("subtasks")
			history, _ := cat.Table("subtask_history")
			if state.RowScope != 5 || history.RowScope != 3 || history.Storage.Kind != catalog.StorageDocument {
				t.Fatalf("state row scope %d, history row scope %d storage %s; want 5, 3 and document", state.RowScope, history.RowScope, history.Storage.Kind)
			}
		})
	}
}

// TestLoadHistoryStorageKind pins spec 30's storage of a history table: `block <row
// fk> table` under its state table or `document table`; any other storage is a
// setting error at the history table's name.
func TestLoadHistoryStorageKind(t *testing.T) {
	const blockStore = "    store block fk_history_item table\n"
	// statesTable is a table keyed by the state column's type, which a history
	// row's from or to can reference.
	const statesTable = "table states {\n  id varchar(16)\n  primary key (id)\n  settings {\n    store document table\n  }\n}\n\n"
	const msg = "a history table is stored `block <row fk> table` under its state table or `document table`"
	cases := []struct {
		name    string
		edits   [][2]string
		refused bool
	}{
		{"30#the-history-table: block of its row foreign key under its state table", nil, false},
		{"30#the-history-table: document table", [][2]string{{blockStore, "    store document table\n"}}, false},
		{"30#the-history-table: files", [][2]string{{blockStore, "    store files\n"}}, true},
		{"30#the-history-table: document table sharded by time", [][2]string{{blockStore, "    store document table shard at month\n"}}, true},
		{"30#the-history-table: document list", [][2]string{{blockStore, "    store document list\n"}}, true},
		{"30#the-history-table: block of another table", [][2]string{
			{"  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict\n",
				"  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict\n  foreign key fk_history_person (item) references people (id) on delete restrict on update restrict\n"},
			{blockStore, "    store block fk_history_person table\n"},
		}, true},
		// T12.3-5: a non-null placement key other than the row column places a
		// history row in the scope of a row other than the one whose trail it
		// records: by from or to under a table keyed by the states, or by the
		// identity key under a table keyed like it.
		{"30#the-history-table: block of another table placed by from", [][2]string{
			{"  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict\n",
				"  index ix_history_from (from_state)\n  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict\n  foreign key fk_history_from (from_state) references states (id) on delete restrict on update restrict\n"},
			{blockStore, "    store block fk_history_from table\n"},
			{"table views {", statesTable + "table views {"},
		}, true},
		{"30#the-history-table: block of another table placed by to", [][2]string{
			{"  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict\n",
				"  index ix_history_to (to_state)\n  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict\n  foreign key fk_history_to (to_state) references states (id) on delete restrict on update restrict\n"},
			{blockStore, "    store block fk_history_to table\n"},
			{"table views {", statesTable + "table views {"},
		}, true},
		{"30#the-history-table: block of another table placed by the identity key", [][2]string{
			{"  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict\n",
				"  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict\n  foreign key fk_history_link (id) references item_links (id) on delete restrict on update restrict\n"},
			{blockStore, "    store block fk_history_link table\n"},
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, text := editedCorpus(t, c.edits...)
			cat, diags := catalog.Load(root)
			if !c.refused {
				if cat == nil || len(diags) > 0 {
					t.Fatalf("refused: %+v", diags)
				}
				return
			}
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, "item_history", msg)
			// A block placement breaks no other rule, so the storage error stands
			// alone; files and a list also lack the title their storage needs.
			if strings.Contains(c.name, "block") && len(diags) != 1 {
				t.Fatalf("%d diagnostics, want the storage error alone: %+v", len(diags), diags)
			}
		})
	}
}

// TestLoadRequireOfMarkdownPastH6 pins the ruling of spec 30 on a required column
// of a block machine: a required column that the state table and the history
// table both declare markdown is a section of the history rows, which is refused
// past H6; a column that the two tables declare alike without markdown is a cell
// and is accepted at any depth; a required column that only one of the two tables
// declares markdown is refused at the history table's name, since the history row
// holds a copy of a value that the two places accept differently.
func TestLoadRequireOfMarkdownPastH6(t *testing.T) {
	cases := []struct {
		name        string
		block       bool   // the state table is a block under items, else a document table
		typ         string // the type of column x in both tables
		stateMark   bool
		historyMark bool
		refusedAt   string // the table the one diagnostic names, or empty when accepted
		msg         string
	}{
		{"both markdown under a block at scope 5", true, "text", true, true, "subtask_history", "level 7"},
		{"both varchar(255) without markdown", true, "varchar(255)", false, false, "", ""},
		{"markdown on the state table only", true, "text", true, false, "subtask_history", "markdown setting"},
		{"markdown on the history only, state table a document", false, "text", false, true, "subtask_history", "markdown setting"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, text := listCorpus(t, [2]string{"table views {", requireSchema(c.block, c.typ, c.stateMark, c.historyMark)})
			cat, diags := catalog.Load(root)
			if c.refusedAt == "" {
				if cat == nil || len(diags) > 0 {
					t.Fatalf("a required column declared alike without markdown is refused: %+v", diags)
				}
				return
			}
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, c.refusedAt, c.msg)
			if len(diags) != 1 {
				t.Fatalf("%d diagnostics, want exactly one: %+v", len(diags), diags)
			}
			for _, d := range diags {
				if d.Line == tableLine(t, text, "subtasks") {
					t.Fatalf("diagnostic %+v on table subtasks, want none", d)
				}
			}
		})
	}
}

// TestLoadBlockPlacementNonNull pins spec 30's block placement rule: the column
// of a block table's placement foreign key is non-null, since a block row stands
// inside the scope of the row that key references and NULL references none; a
// nullable one is a schema error at the block table's name. A history table is a
// block table like any other, so the same rule refuses a nullable row column and
// a placement by a copied column, which the history line requires nullable.
func TestLoadBlockPlacementNonNull(t *testing.T) {
	const msg = "is placed by foreign key"
	copied := requireSchema(false, "varchar(32)", false, false)
	copiedEdits := [][2]string{
		{"  foreign key fk_subtask_history_subtask (subtask)", "  index ix_subtask_history_x (x)\n  foreign key fk_subtask_history_x (x) references items (id) on delete restrict on update restrict\n  foreign key fk_subtask_history_subtask (subtask)"},
		{"    store block fk_subtask_history_subtask table\n", "    store block fk_subtask_history_x table\n"},
	}
	for _, e := range copiedEdits {
		if strings.Count(copied, e[0]) != 1 {
			t.Fatalf("edit anchor %q is not in the history schema once", e[0])
		}
		copied = strings.Replace(copied, e[0], e[1], 1)
	}
	edit := func(text, old, new string) string {
		t.Helper()
		if strings.Count(text, old) != 1 {
			t.Fatalf("edit anchor %q is not in the text once", old)
		}
		return strings.Replace(text, old, new, 1)
	}
	nullableSteps := edit(childBlock("item_steps"), "  item varchar(32)\n", "  item varchar(32) null\n")
	// nested is item_steps under items, placed by a nullable item, and
	// item_step_notes under item_steps, placed by a non-null step.
	nested := edit(nullableSteps, "table views {", "table item_step_notes {\n  id i64 identity\n  step i64\n  primary key (id)\n  index ix_item_step_notes_step (step)\n  foreign key fk_item_step_notes_step (step) references item_steps (id) on delete restrict on update restrict\n  settings {\n    store block fk_item_step_notes_step table\n  }\n}\n\ntable views {")
	// composite is slot placed under grid by its composite foreign key, whose
	// column gy is nullable; the composite key is refused by its own rule too.
	composite := edit(edit(gridTables, "  gy varchar(32)\n", "  gy varchar(32) null\n"),
		"on update restrict\n  settings {\n    store document table\n", "on update restrict\n  settings {\n    store block fk_slot_grid table\n")
	// otherNullable is item_steps with a nullable foreign key that does not place it.
	otherNullable := edit(edit(childBlock("item_steps"), "  label varchar(64)\n", "  label varchar(64)\n  person varchar(32) null\n"),
		"  primary key (id)\n", "  primary key (id)\n  index ix_item_steps_person (person)\n  foreign key fk_item_steps_person (person) references people (id) on delete restrict on update restrict\n")
	cases := []struct {
		name    string
		edits   [][2]string
		table   string // the table the diagnostic names, or empty when accepted
		message string // the whole message, when it is pinned
	}{
		{"30#corpus-rules-on-dbspec-constructs: a non-null placement column loads", [][2]string{{"table views {", childBlock("item_steps")}}, "", ""},
		{"30#corpus-rules-on-dbspec-constructs: a nullable placement column is refused", [][2]string{{"table views {", nullableSteps}}, "item_steps",
			"block table item_steps is placed by foreign key fk_item_steps_item, whose column item is nullable"},
		{"30#corpus-rules-on-dbspec-constructs: a nullable middle placement of two nested blocks is refused at the middle table only", [][2]string{{"table views {", nested}}, "item_steps",
			"block table item_steps is placed by foreign key fk_item_steps_item, whose column item is nullable"},
		{"30#corpus-rules-on-dbspec-constructs: a composite placement key with one nullable column is refused naming that column", [][2]string{{"table views {", composite}}, "slot",
			"block table slot is placed by foreign key fk_slot_grid, whose column gy is nullable"},
		{"30#corpus-rules-on-dbspec-constructs: a nullable foreign key that does not place the block loads", [][2]string{{"table views {", otherNullable}}, "", ""},
		{"30#the-history-table: a nullable row column of a block history is refused", [][2]string{{"  item varchar(32)\n  from_state", "  item varchar(32) null\n  from_state"}}, "item_history",
			"block table item_history is placed by foreign key fk_history_item, whose column item is nullable"},
		{"30#the-history-table: a block history placed by a copied column is refused", [][2]string{{"table views {", copied}}, "subtask_history",
			"block table subtask_history is placed by foreign key fk_subtask_history_x, whose column x is nullable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, text := editedCorpus(t, c.edits...)
			cat, diags := catalog.Load(root)
			if c.table == "" {
				if cat == nil || len(diags) > 0 {
					t.Fatalf("refused: %+v", diags)
				}
				return
			}
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, c.table, msg)
			n := 0
			for _, d := range diags {
				if strings.Contains(d.Message, msg) {
					n++
					if d.Message != c.message {
						t.Fatalf("message %q, want %q", d.Message, c.message)
					}
				}
			}
			if n != 1 {
				t.Fatalf("%d placement diagnostics, want one: %+v", n, diags)
			}
		})
	}
}

func TestLoadRejectsBlockCycle(t *testing.T) {
	node := `table node {
  id varchar(32)
  parent varchar(32)
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

// TestLoadViewTable pins spec 100's declaration of the view table: the table
// views of the document named flowmark, the application schema, with its view
// columns; a views table of another document is an ordinary table.
func TestLoadViewTable(t *testing.T) {
	cat, diags := catalog.Load(fixtureRoot)
	if cat == nil || len(diags) > 0 {
		t.Fatalf("fixture refused: %+v", diags)
	}
	views, _ := cat.Table("views")
	want := &catalog.View{Kind: "kind", Entity: "entity", Filter: "filter", Sort: "sort", Group: "group_by"}
	if !reflect.DeepEqual(views.View, want) {
		t.Fatalf("views.View = %+v, want %+v", views.View, want)
	}
	if items, _ := cat.Table("items"); items.View != nil {
		t.Fatalf("items.View = %+v, want nil", items.View)
	}
	if !reflect.DeepEqual(views.Codecs, map[string][]string{"filter": {"ordered_json"}, "sort": {"ordered_json"}}) {
		t.Fatalf("views.Codecs = %v", views.Codecs)
	}

	root, _ := editedCorpus(t, [2]string{"dbspec 1 flowmark\n", "dbspec 1 tracker\n"})
	cat, diags = catalog.Load(root)
	if cat == nil || len(diags) > 0 {
		t.Fatalf("refused: %+v", diags)
	}
	if views, _ := cat.Table("views"); views.View != nil {
		t.Fatalf("a views table of the document tracker has View %+v, want nil", views.View)
	}
}

// TestLoadViewColumns pins the shape spec 100 gives the view columns of the
// application schema's views table; any other shape is a catalog error at the
// table name.
func TestLoadViewColumns(t *testing.T) {
	const msg = "the view table views"
	cases := []struct {
		name string
		edit [2]string
	}{
		{"an entity with a codec", [2]string{"    codec sort ordered_json\n", "    codec sort ordered_json\n    codec entity base64\n"}},
		{"a nullable entity", [2]string{"  entity varchar(63)\n", "  entity varchar(63) null\n"}},
		{"an entity of another type", [2]string{"  entity varchar(63)\n", "  entity i64\n"}},
		{"a filter without a codec", [2]string{"    codec filter ordered_json\n", ""}},
		{"a filter with a further stage", [2]string{"    codec filter ordered_json\n", "    codec filter ordered_json base64\n"}},
		{"a nullable sort", [2]string{"  sort text\n", "  sort text null\n"}},
		{"a group with a codec", [2]string{"    codec sort ordered_json\n", "    codec sort ordered_json\n    codec group_by base64\n"}},
		{"a group of another type", [2]string{"  group_by varchar(63) null\n", "  group_by i32 null\n"}},
		{"no group column", [2]string{"  group_by varchar(63) null\n", ""}},
		{"a nullable kind", [2]string{"  kind varchar(16)\n  entity", "  kind varchar(16) null\n  entity"}},
		{"a kind with a codec", [2]string{"    codec sort ordered_json\n", "    codec sort ordered_json\n    codec kind base64\n"}},
	}
	for _, c := range cases {
		t.Run("100#views: "+c.name, func(t *testing.T) {
			root, text := editedCorpus(t, c.edit)
			cat, diags := catalog.Load(root)
			expectRejected(t, cat, diags)
			expectTableDiag(t, diags, text, "views", msg)
			if len(diags) != 1 {
				t.Fatalf("%d diagnostics, want the view error alone: %+v", len(diags), diags)
			}
		})
	}
	t.Run("100#views: a group that is not null loads", func(t *testing.T) {
		root, _ := editedCorpus(t, [2]string{"  group_by varchar(63) null\n", "  group_by text\n"})
		if cat, diags := catalog.Load(root); cat == nil || len(diags) > 0 {
			t.Fatalf("refused: %+v", diags)
		}
	})
}

// TestLoadYAMLCodec pins spec 30's refusal of a yaml codec stage that is the
// last stage: its encoder ends every value with a line feed, which no place of
// the format holds; a later stage encodes the line feed away.
func TestLoadYAMLCodec(t *testing.T) {
	column := [2]string{"  name varchar(255)\n", "  name varchar(255)\n  meta text null\n"}
	root, text := editedCorpus(t, column, [2]string{"    title name\n", "    title name\n    codec meta yaml\n"})
	cat, diags := catalog.Load(root)
	expectRejected(t, cat, diags)
	expectTableDiag(t, diags, text, "people", "column meta ends its codec with the stage yaml")
	if len(diags) != 1 {
		t.Fatalf("%d diagnostics, want the yaml error alone: %+v", len(diags), diags)
	}
	root, _ = editedCorpus(t, column, [2]string{"    title name\n", "    title name\n    codec meta yaml base64\n"})
	if cat, diags := catalog.Load(root); cat == nil || len(diags) > 0 {
		t.Fatalf("yaml under base64 refused: %+v", diags)
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

// shardedPeople stores people as a document table sharded by the month of a joined time.
var shardedPeople = [][2]string{
	{"  name varchar(255)\n  primary key (id)\n  settings {\n    key_prefix 'P'\n    store document table\n",
		"  name varchar(255)\n  joined datetime(6)\n  primary key (id)\n  settings {\n    key_prefix 'P'\n    store document table shard joined month\n"},
}

func TestLoadReadsStoreShard(t *testing.T) {
	root, _ := editedCorpus(t, shardedPeople...)
	cat, diags := catalog.Load(root)
	if len(diags) != 0 || cat == nil {
		t.Fatalf("diagnostics for a sharded table: %+v", diags)
	}
	people, ok := cat.Table("people")
	if !ok {
		t.Fatal("no table people")
	}
	if want := (&catalog.Shard{Column: "joined", Period: catalog.PeriodMonth}); !reflect.DeepEqual(people.Storage.Shard, want) {
		t.Fatalf("people shard = %+v, want %+v", people.Storage.Shard, want)
	}
	items, _ := cat.Table("items")
	if items.Storage.Shard != nil {
		t.Fatalf("items has no shard clause, got %+v", items.Storage.Shard)
	}
}

// TestRowPath pins the corpus path of a row and of a document (spec 20 Time shards): the file of a files
// row or the document of a document table, with the folder or the file of its period when the storage is
// sharded; a datetime counts at its stored UTC value, and a value that is no time has no path.
func TestRowPath(t *testing.T) {
	cases := []struct {
		name    string
		storage catalog.Storage
		key     string
		value   string // the stored value of the shard column
		want    string
		fault   string // a text the error holds, "" for none
	}{
		{name: "files", storage: catalog.Storage{Kind: catalog.StorageFiles}, key: "T1", want: "items/T1.flowmark.md"},
		{name: "document", storage: catalog.Storage{Kind: catalog.StorageDocument}, want: "items.flowmark.md"},
		{name: "files by year", storage: sharded(catalog.StorageFiles, catalog.PeriodYear), key: "T1", value: "2026-10-11 09:30:00.000000", want: "items/2026/T1.flowmark.md"},
		{name: "files by month", storage: sharded(catalog.StorageFiles, catalog.PeriodMonth), key: "T1", value: "2026-10-11 09:30:00.000000", want: "items/2026-10/T1.flowmark.md"},
		{name: "files by day", storage: sharded(catalog.StorageFiles, catalog.PeriodDay), key: "T1", value: "2026-10-11 09:30:00.000000", want: "items/2026-10-11/T1.flowmark.md"},
		{name: "files by month of a date", storage: sharded(catalog.StorageFiles, catalog.PeriodMonth), key: "T1", value: "2026-10-11", want: "items/2026-10/T1.flowmark.md"},
		{name: "document by month", storage: sharded(catalog.StorageDocument, catalog.PeriodMonth), value: "2026-10-31 23:59:59", want: "items/2026-10.flowmark.md"},
		{name: "document by year", storage: sharded(catalog.StorageDocument, catalog.PeriodYear), value: "2026-01-01", want: "items/2026.flowmark.md"},
		{name: "a value that is no time", storage: sharded(catalog.StorageFiles, catalog.PeriodMonth), key: "T1", value: "soon", fault: "not a date or datetime"},
		{name: "a month past 12", storage: sharded(catalog.StorageFiles, catalog.PeriodMonth), key: "T1", value: "2026-13-01", fault: "not a date or datetime"},
		{name: "a day past the month", storage: sharded(catalog.StorageFiles, catalog.PeriodDay), key: "T1", value: "2026-02-30", fault: "not a date or datetime"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tb := &catalog.Table{Name: "items", Storage: tc.storage}
			got, err := tb.RowPath(tc.key, tc.value)
			if tc.fault != "" {
				if err == nil || !strings.Contains(err.Error(), tc.fault) {
					t.Fatalf("RowPath = %q, %v; want an error holding %q", got, err, tc.fault)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("RowPath = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func sharded(kind catalog.StorageKind, period catalog.Period) catalog.Storage {
	return catalog.Storage{Kind: kind, Shard: &catalog.Shard{Column: "at", Period: period}}
}
