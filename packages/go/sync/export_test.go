// Tests of the md→db export (spec 70 The export): the schema statements and the row
// inserts orm renders from the catalog's runtime model, applied to a SQLite database
// and read back.
package sync

import (
	"bytes"
	"compress/zlib"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/engine/plan"
	"github.com/polyspec/orm/packages/go/engine/runtimemodel"
	"github.com/polyspec/orm/packages/go/orm"
	_ "github.com/polyspec/orm/packages/go/orm/sqlite"
	_ "modernc.org/sqlite"
)

// equivalenceRoots are the store goldens whose catalogs hold no identity column.
var equivalenceRoots = []string{
	"equivalence/block",
	"equivalence/block_table",
	"equivalence/document",
	"equivalence/document_table",
	"equivalence/files",
}

// corpus copies the store golden name into a temporary root, writes the files of
// docs (corpus path to text) over it, and returns the root.
func corpus(t *testing.T, name string, docs map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := corpustest.CopyTree(corpustest.GoldenDir(t, name), root); err != nil {
		t.Fatal(err)
	}
	corpustest.WriteFiles(t, root, docs)
	return root
}

// open reads the corpus at root with its catalog.
func open(t *testing.T, root string) *irexec.Corpus {
	t.Helper()
	cat, diags := catalog.Load(root)
	if len(diags) > 0 {
		t.Fatalf("catalog: %+v", diags)
	}
	c, err := irexec.Open(cat, store.DirFS(root), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// typesDocs turns the files golden into a catalog without an identity column, whose
// memos table has a soft-deleted row: step_refs takes a varchar key, and memos a
// soft_delete column.
func typesDocs(t *testing.T) map[string]string {
	t.Helper()
	src := filepath.Join(corpustest.ModuleRoot(t), "store", "testdata", "files", "dbspec", "flowmark.dbs")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, r := range [][2]string{
		{"  id i64 identity\n  step varchar(16)", "  id varchar(16)\n  step varchar(16)"},
		{"  summary text null\n  primary key (id)\n  settings {\n    store files", "  summary text null\n  deleted_at datetime(6) null\n  primary key (id)\n  settings {\n    soft_delete deleted_at\n    store files"},
	} {
		if !strings.Contains(text, r[0]) {
			t.Fatalf("the files golden catalog no longer holds %q", r[0])
		}
		text = strings.Replace(text, r[0], r[1], 1)
	}
	return map[string]string{
		"dbspec/flowmark.dbs":  text,
		"memos/M2.flowmark.md": "# Memo two\n- deleted_at: 2026-10-11 09:00:00.000000\n",
	}
}

// codecDocs turns the files golden into a catalog whose memos table holds four codec
// columns: doc, ordered_json text; packed, ordered_json compressed by gz, which is
// bytes; hexed, ordered_json then the host stage hex; and addr, the host stage ip, which
// is bytes. Its first memo stores all four.
func codecDocs(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(corpustest.ModuleRoot(t), "store", "testdata", "files", "dbspec", "flowmark.dbs"))
	if err != nil {
		t.Fatal(err)
	}
	old := "  summary text null\n  primary key (id)\n  settings {\n    store files\n    title title\n    body body\n    markdown body\n    markdown summary\n  }"
	if !strings.Contains(string(data), old) {
		t.Fatal("the files golden catalog no longer holds the memos table")
	}
	text := strings.Replace(string(data), old, "  summary text null\n  doc text null\n  packed bytes null\n  hexed varchar(255) null\n  addr bytes null\n  primary key (id)\n  settings {\n    store files\n    title title\n    body body\n    markdown body\n    markdown summary\n    codec doc ordered_json\n    codec packed ordered_json gz\n    codec hexed ordered_json hex\n    codec addr ip\n  }", 1)
	var packed bytes.Buffer
	w := zlib.NewWriter(&packed)
	if _, err := w.Write([]byte(`{"b":2}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"dbspec/flowmark.dbs":  text,
		"memos/M1.flowmark.md": "# Memo one\n- doc: {\"a\":1}\n- packed: " + base64.StdEncoding.EncodeToString(packed.Bytes()) + "\n- hexed: " + strings.ToUpper(hex.EncodeToString([]byte(`{"h":3}`))) + "\n- addr: AQIDBA==\n\nBody.\n\n## summary\n\nSummary text.\n",
	}
}

// hostFirstDocs is codecDocs with a hexed column whose codec puts the host stage hex
// before a client stage.
func hostFirstDocs(t *testing.T) map[string]string {
	t.Helper()
	docs := codecDocs(t)
	text := docs["dbspec/flowmark.dbs"]
	if !strings.Contains(text, "    codec hexed ordered_json hex\n") {
		t.Fatal("codecDocs no longer declares the hexed codec")
	}
	docs["dbspec/flowmark.dbs"] = strings.Replace(text, "    codec hexed ordered_json hex\n", "    codec hexed hex base64\n", 1)
	return docs
}

// apply runs the statements of the export of c on a new SQLite database with its
// foreign keys enforced: the schema statements through the driver and every insert
// through orm's DB.Execute, which binds the parameters in the wire form (spec 70 The
// export).
func apply(t *testing.T, c *irexec.Corpus, stmts []Statement) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	model, diags := runtimemodel.Build(c.Catalog().Documents)
	if len(diags) > 0 {
		t.Fatalf("runtime model: %v", runtimemodel.DiagnosticsError(diags))
	}
	var odb *orm.DB
	for i, s := range stmts {
		if len(s.Slots) == 0 && len(s.Params) == 0 {
			if _, err := db.Exec(s.SQL); err != nil {
				t.Fatalf("statement %d %s: %v", i, s.SQL, err)
			}
			continue
		}
		if odb == nil {
			odb, err = orm.ConnectSchema((&url.URL{Scheme: "sqlite", Path: sqliteURLPath(path)}).String(), &orm.Schema{Hash: model.ManifestHash, Text: model.ManifestText}, orm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := odb.Close(); err != nil {
					t.Error(err)
				}
			})
		}
		params := make([]any, len(s.Params))
		copy(params, s.Params)
		for _, slot := range s.Slots {
			params[slot.Param] = native(t, slot.ColType, s.Params[slot.Param])
		}
		if err := odb.Execute(&plan.Step{Role: "main", SQL: s.SQL, BindSlots: s.Slots}, params); err != nil {
			t.Fatalf("statement %d %s %v: %v", i, s.SQL, s.Params, err)
		}
	}
	return db
}

// as is v as a T; the test fails when v is another type.
func as[T any](t *testing.T, v any) T {
	t.Helper()
	x, ok := v.(T)
	if !ok {
		t.Fatalf("the value %v is a %T, not a %T", v, v, x)
	}
	return x
}

// native turns a parameter in the wire form into the value orm's DB.Execute takes for
// a slot of the dbspec type col: an integer as int64, a number as float64, bytes decoded
// from base64. Decimals, datetimes, times and text stay strings, which Execute binds by
// the slot's type. orm offers no call that takes the wire form itself (spec 70, open
// question 6).
func native(t *testing.T, col string, v any) any {
	t.Helper()
	if v == nil {
		return nil
	}
	switch col {
	case "i16", "i32", "i64":
		n, err := as[json.Number](t, v).Int64()
		if err != nil {
			t.Fatal(err)
		}
		return n
	case "f64":
		f, err := as[json.Number](t, v).Float64()
		if err != nil {
			t.Fatal(err)
		}
		return f
	case "bytes":
		b, err := base64.StdEncoding.DecodeString(as[string](t, v))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	return v
}

// readBack returns the rows of table in insertion order, each value written as its
// wire form, so that it compares with the parameters the export gave.
func readBack(t *testing.T, db *sql.DB, c *irexec.Corpus, table string) [][]any {
	t.Helper()
	tb, _ := c.Catalog().Table(table)
	names := make([]string, len(tb.Columns))
	for i, col := range tb.Columns {
		names[i] = `"` + col.Name + `"`
		switch col.Type.Kind {
		case dbspec.TypeDate, dbspec.TypeTime, dbspec.TypeDatetime:
			// The driver reads a DATE or DATETIME column as a time; the text is the
			// stored value.
			names[i] = "CAST(" + names[i] + " AS TEXT)"
		}
	}
	rows, err := db.Query("SELECT " + strings.Join(names, ", ") + ` FROM "` + table + `" ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var out [][]any
	for rows.Next() {
		vals := make([]any, len(tb.Columns))
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		row := make([]any, len(vals))
		for i, col := range tb.Columns {
			row[i] = wire(t, col.Type, vals[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// wire writes a SQLite value of a column of type typ in the wire form.
func wire(t *testing.T, typ dbspec.Type, v any) any {
	t.Helper()
	if v == nil {
		return nil
	}
	switch typ.Kind {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64:
		return json.Number(strconv.FormatInt(as[int64](t, v), 10))
	case dbspec.TypeF64:
		return json.Number(strconv.FormatFloat(as[float64](t, v), 'f', -1, 64))
	case dbspec.TypeBool:
		return as[int64](t, v) == 1
	case dbspec.TypeDecimal:
		s := strconv.FormatInt(as[int64](t, v), 10)
		neg := strings.HasPrefix(s, "-")
		s = strings.TrimPrefix(s, "-")
		for len(s) <= typ.Scale {
			s = "0" + s
		}
		s = s[:len(s)-typ.Scale] + "." + s[len(s)-typ.Scale:]
		if neg {
			s = "-" + s
		}
		return s
	case dbspec.TypeBytes:
		return base64.StdEncoding.EncodeToString(as[[]byte](t, v))
	case dbspec.TypeDatetime:
		s := as[string](t, v)
		if len(s) == 19 {
			s += "."
		}
		return s + strings.Repeat("0", 26-len(s))
	case dbspec.TypeTime:
		s := as[string](t, v)
		if len(s) == 8 {
			s += "."
		}
		return s + strings.Repeat("0", 15-len(s))
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}
	t.Fatalf("a %s value %T", typ.String(), v)
	return nil
}

// TestExportRebuildsTheCorpus pins that the export's statements create the catalog's
// tables in a database and insert every row of the corpus, soft-deleted rows
// included, each table's rows in storage order, with the values the engine reads.
// Each case has its own corpus and database, so the cases run in parallel and the
// test lasts as long as its slowest case, not as long as their sum (T15-45).
func TestExportRebuildsTheCorpus(t *testing.T) {
	cases := map[string]string{}
	for _, name := range equivalenceRoots {
		cases[name] = corpus(t, name, nil)
	}
	cases["files (every type)"] = corpus(t, "files", typesDocs(t))
	cases["files (identity column)"] = corpus(t, "files", nil)
	cases["files (codec columns)"] = corpus(t, "files", codecDocs(t))
	for name, root := range cases {
		t.Run("70#the-export: "+name+" rebuilds in SQLite", func(t *testing.T) {
			t.Parallel()
			c := open(t, root)
			stmts, err := Export(c, dbspec.DialectSQLite)
			if err != nil {
				t.Fatal(err)
			}
			db := apply(t, c, stmts)
			total := 0
			for _, tb := range c.Catalog().Tables {
				want, err := c.Rows(tb.Name)
				if err != nil {
					t.Fatal(err)
				}
				got := readBack(t, db, c, tb.Name)
				if len(want) == 0 {
					want = nil
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s:\n got %v\nwant %v", tb.Name, got, want)
				}
				total += len(want)
			}
			if total == 0 {
				t.Fatal("the corpus exported no row")
			}
		})
	}
}

// TestExportSoftDeletedRow pins that a soft-deleted row is exported: the database is
// rebuilt whole, and a read of it excludes the row as the corpus does.
func TestExportSoftDeletedRow(t *testing.T) {
	c := open(t, corpus(t, "files", typesDocs(t)))
	rows, err := c.Rows("memos")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1][0] != "M2" || rows[1][len(rows[1])-1] != "2026-10-11 09:00:00.000000" {
		t.Fatalf("70#the-export: the soft-deleted memo M2 is not among the rows: %v", rows)
	}
}

// TestRowsOfAnUnknownTable pins that the corpus reads no rows of a table its catalog
// does not declare.
func TestRowsOfAnUnknownTable(t *testing.T) {
	_, err := open(t, corpus(t, "equivalence/document", nil)).Rows("nothing")
	var e *irexec.Error
	if !errors.As(err, &e) || e.Code != "ENTITY_UNKNOWN" {
		t.Fatalf("70#the-export: the rows of an unknown table: %v", err)
	}
}

// TestExportStatements pins the form of the statements: the schema statements are
// those orm's renderer writes for the catalog, in its order, and every other
// statement inserts one row; the export is the same on every run, in every dialect.
func TestExportStatements(t *testing.T) {
	root := corpus(t, "equivalence/block", nil)
	cat, _ := catalog.Load(root)
	for _, d := range []dbspec.Dialect{dbspec.DialectMySQL, dbspec.DialectPostgres, dbspec.DialectSQLite} {
		t.Run("70#the-export: "+string(d)+" statements", func(t *testing.T) {
			stmts, err := Export(open(t, root), d)
			if err != nil {
				t.Fatal(err)
			}
			schema, diags := dbspec.Render(cat.Documents, d)
			if len(diags) > 0 {
				t.Fatal(diags)
			}
			if len(stmts) < len(schema) {
				t.Fatalf("%d statements, %d schema statements", len(stmts), len(schema))
			}
			for i, s := range schema {
				if stmts[i].SQL != s || len(stmts[i].Slots) != 0 {
					t.Fatalf("statement %d is %q, the renderer writes %q", i, stmts[i].SQL, s)
				}
			}
			rows := 0
			for _, tb := range cat.Tables {
				n, err := open(t, root).Rows(tb.Name)
				if err != nil {
					t.Fatal(err)
				}
				rows += len(n)
			}
			inserts := stmts[len(schema):]
			if len(inserts) != rows {
				t.Fatalf("%d inserts for %d rows", len(inserts), rows)
			}
			for _, s := range inserts {
				if !strings.HasPrefix(s.SQL, "INSERT INTO ") || len(s.Slots) == 0 {
					t.Fatalf("not a row insert: %q", s.SQL)
				}
			}
			again, err := Export(open(t, root), d)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stmts, again) {
				t.Fatal("the export differs between two runs")
			}
		})
	}
}

// TestExportOrder pins the order of the rows: a table's rows follow the rows of every
// table its foreign keys reference, and a row follows its parent.
func TestExportOrder(t *testing.T) {
	c := open(t, corpus(t, "equivalence/block", nil))
	stmts, err := Export(c, dbspec.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for _, s := range stmts {
		if strings.HasPrefix(s.SQL, "INSERT INTO ") && (len(tables) == 0 || tables[len(tables)-1] != s.Table) {
			tables = append(tables, s.Table)
		}
	}
	if fmt.Sprint(tables) != "[plans entries]" {
		t.Fatalf("70#the-export: the tables insert in the order %v, want [plans entries]", tables)
	}
}

// TestExportRefusals pins the exports that fail and write nothing: one that needs an
// unavailable table or a value not of its type (CONSTRAINT, naming the diagnostic),
// and one of a table with an identity column (CAPABILITY_UNSUPPORTED, naming the
// table).
func TestExportRefusals(t *testing.T) {
	cases := []struct {
		name, golden string
		docs         map[string]string
		code, rule   string
		file, table  string
	}{
		{name: "70#scope: an unavailable table fails naming its file", golden: "files", docs: mergeDocs(typesDocs(t), map[string]string{"memos/M9.flowmark.md": "no heading\n"}),
			code: "CONSTRAINT", rule: "format.", file: "memos/M9.flowmark.md"},
		{name: "70#scope: a value not of its type fails naming its diagnostic", golden: "equivalence/document",
			docs: map[string]string{"entries.flowmark.md": strings.Replace(readGolden(t, "equivalence/document/entries.flowmark.md"), "count:10", "count:ten", 1)},
			code: "CONSTRAINT", rule: "type", file: "entries.flowmark.md"},
		{name: "70#the-export: a codec column with a host stage before a client stage is unsupported", golden: "files", docs: hostFirstDocs(t),
			code: "CAPABILITY_UNSUPPORTED", table: "hexed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmts, err := Export(open(t, corpus(t, tc.golden, tc.docs)), dbspec.DialectSQLite)
			if stmts != nil {
				t.Fatalf("the failed export gave %d statements", len(stmts))
			}
			var e *irexec.Error
			if !errors.As(err, &e) || e.Code != tc.code {
				t.Fatalf("error %v, want %s", err, tc.code)
			}
			if tc.rule != "" && (e.Diagnostic == nil || !strings.HasPrefix(e.Diagnostic.Rule, tc.rule) || e.Diagnostic.File != tc.file) {
				t.Fatalf("diagnostic %+v", e.Diagnostic)
			}
			if tc.table != "" && !strings.Contains(e.Msg, tc.table) {
				t.Fatalf("the error does not name %s: %s", tc.table, e.Msg)
			}
		})
	}
}

// readGolden returns the text of the store golden file name.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(corpustest.ModuleRoot(t), "store", "testdata", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mergeDocs returns the files of a and b, b's winning.
func mergeDocs(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// TestExportRestartsTheIdentity pins that a table with an identity column keeps its
// keys and that the next key a database generates follows the largest one loaded.
func TestExportRestartsTheIdentity(t *testing.T) {
	c := open(t, corpus(t, "files", nil))
	stmts, err := Export(c, dbspec.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	db := apply(t, c, stmts)
	var seq, max int64
	if err := db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'step_refs'`).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT MAX(id) FROM step_refs`).Scan(&max); err != nil {
		t.Fatal(err)
	}
	if max == 0 || seq != max {
		t.Fatalf("70#the-export: step_refs holds the largest key %d and its identity restarts past %d", max, seq)
	}
}

// TestExportShardedTables pins that a database export ignores a time shard (spec 20 Time shards): a sharded
// table is one table with the rows of every period, read back as the corpus reads them.
func TestExportShardedTables(t *testing.T) {
	docs := map[string]string{
		"events/2026-10/E1.flowmark.md": "# First\n- at: 2026-10-11 09:30:00.000000\n",
		"events/2026-11/E2.flowmark.md": "# Second\n- at: 2026-11-02 08:00:00.000000\n",
	}
	c := open(t, corpus(t, "shard", docs))
	stmts, err := Export(c, dbspec.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	creates := 0
	for _, s := range stmts {
		if strings.HasPrefix(s.SQL, `CREATE TABLE "events"`) {
			creates++
			if strings.Contains(strings.ToLower(s.SQL), "shard") || strings.Contains(strings.ToLower(s.SQL), "partition") {
				t.Fatalf("the statement renders the shard: %s", s.SQL)
			}
		}
	}
	if creates != 1 {
		t.Fatalf("%d statements create events; want one", creates)
	}
	db := apply(t, c, stmts)
	want, err := c.Rows("events")
	if err != nil {
		t.Fatal(err)
	}
	if got := readBack(t, db, c, "events"); !reflect.DeepEqual(got, want) || len(got) != 2 {
		t.Fatalf("events read back %v; want the two rows %v", got, want)
	}
}

// sqliteURLPath is the path of a SQLite DSN for the file path: absolute, with slashes, so that a Windows path
// C:\dir\file becomes /C:/dir/file.
func sqliteURLPath(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
