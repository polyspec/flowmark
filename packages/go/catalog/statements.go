package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/min-0-max/sqlite" // the database that a schema statement of a corpus runs in
	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// StatementsFile is the file of the schema statements a corpus took in SQL, relative to its root, a JSON array of their
// texts in order (spec 40 SQL statements). The document set declares what the set owns, and this file what SQL added.
const StatementsFile = dbspecDir + "/statements.json"

// SequencesFile is the file of the AUTOINCREMENT sequences of a corpus, relative to its root: a JSON object, by table,
// of the sequence SQLite keeps in sqlite_sequence for each AUTOINCREMENT table whose sequence is not the one its rows
// imply, an integer, or null when sqlite_sequence holds no row of a table that holds rows (spec 40 SQL statements). A
// table the file does not name has the sequence of its largest key, and no row of sqlite_sequence when it holds no row.
// The next key of such a table is one more than the larger of its sequence and its largest key, so no key is given twice.
const SequencesFile = dbspecDir + "/sequences.json"

// ParseSequences reads the sequences of the bytes of SequencesFile; nil bytes, a corpus without the file, hold none. A
// sequence is an integer of int64, nil for null, a table of no row of sqlite_sequence.
func ParseSequences(data []byte) (map[string]*int64, error) {
	if data == nil {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("%s: not a JSON object of sequences", SequencesFile)
	}
	out := map[string]*int64{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", SequencesFile, err)
		}
		table, named := tok.(string)
		if !named {
			return nil, fmt.Errorf("%s: a name of no string", SequencesFile)
		}
		if _, twice := out[table]; twice {
			return nil, fmt.Errorf("%s: the sequence of %s is named twice", SequencesFile, table)
		}
		var n *int64
		if err := dec.Decode(&n); err != nil {
			return nil, fmt.Errorf("%s: the sequence of %s: %w", SequencesFile, table, err)
		}
		out[table] = n
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("%s: %w", SequencesFile, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: data after the object of sequences", SequencesFile)
	}
	return out, nil
}

// RenderSequences writes the sequences as the bytes of SequencesFile, by table name, nil as null; no sequence is no
// file (nil).
func RenderSequences(sequences map[string]*int64) []byte {
	if len(sequences) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(sequences, "", "  ")
	if err != nil {
		panic("catalog: the sequences do not marshal: " + err.Error())
	}
	return append(data, '\n')
}

// AddedDocument is the name of the document of the tables that schema statements added.
const AddedDocument = "statements"

// ReadStatements reads the schema statements of the corpus root; a corpus without the file has none.
func ReadStatements(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(StatementsFile)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", StatementsFile, err)
	}
	return out, nil
}

// SchemaStatements is the SQLite schema of the documents with the schema statements after them, in order.
func SchemaStatements(documents []*dbspec.Document, added []string) ([]string, error) {
	// The set renders its own tables; the database that owns an external document created the others, so here the set does.
	own := make([]*dbspec.Document, len(documents))
	for i, d := range documents {
		c := *d
		c.External = false
		own[i] = &c
	}
	rendered, diags := dbspec.RenderStatements(own, dbspec.DialectSQLite)
	if len(diags) > 0 {
		return nil, fmt.Errorf("orm renders no schema for the set: %v", diags)
	}
	schema := make([]string, 0, len(rendered)+len(added))
	for _, s := range rendered {
		schema = append(schema, s.SQL)
	}
	return append(schema, added...), nil
}

// applyStatements reads the tables of the corpus's schema statements. It returns the document of the tables they added
// and sets the columns of an owned table they changed; a table they dropped leaves the set's tables.
func (l *loader) applyStatements(docs []*document, added []string) (_ *dbspec.Document, err error) {
	var parsed []*dbspec.Document
	for _, d := range docs {
		if d.parsed != nil {
			parsed = append(parsed, d.parsed)
		}
	}
	schema, err := SchemaStatements(parsed, added)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	db, err := sql.Open(sqlite.DriverName, ":memory:")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	db.SetMaxOpenConns(1)
	// What the documents declare, before the statements: a column they re-declare takes the type they give it.
	declared := map[string]map[string]string{}
	for i, s := range schema {
		if i == len(schema)-len(added) {
			names, err := liveTables(ctx, db)
			if err != nil {
				return nil, err
			}
			for _, name := range names {
				if declared[name], err = liveDeclarations(ctx, db, name); err != nil {
					return nil, err
				}
			}
		}
		if _, err := db.ExecContext(ctx, s); err != nil {
			return nil, fmt.Errorf("the schema statement %s: %w", s, err)
		}
	}
	names, err := liveTables(ctx, db)
	if err != nil {
		return nil, err
	}
	if l.autoincrement, err = autoincrementTables(ctx, db, names); err != nil {
		return nil, err
	}
	l.affinities = map[string]map[string]Affinity{}
	for _, name := range names {
		decls, err := liveDeclarations(ctx, db, name)
		if err != nil {
			return nil, err
		}
		l.affinities[name] = map[string]Affinity{}
		for col, decl := range decls {
			l.affinities[name][col] = DeclaredAffinity(decl)
		}
	}
	out := &dbspec.Document{Name: AddedDocument}
	held := map[string]bool{}
	for _, d := range docs {
		if d.parsed == nil {
			continue
		}
		// The document keeps its text and its manifest; only the tables of the corpus follow the statements.
		c := *d.parsed
		c.Tables = nil
		for _, t := range d.parsed.Tables {
			if !slices.Contains(names, t.Name) {
				continue
			}
			held[t.Name] = true
			live, err := liveTable(ctx, db, t.Name)
			if err != nil {
				return nil, err
			}
			if !sameColumnNames(t.Columns, live.Columns) {
				// The statements made another table of this name: what the document says of its columns is gone with them.
				t.Columns, t.PrimaryKey = live.Columns, live.PrimaryKey
				t.Indexes, t.Uniques, t.ForeignKeys, t.Checks, t.Settings = nil, nil, nil, nil, nil
			} else {
				// A rebuild keeps the names and may change what SQLite requires of a column and the type it declares.
				now, err := liveDeclarations(ctx, db, t.Name)
				if err != nil {
					return nil, err
				}
				t.Columns = slices.Clone(t.Columns)
				for i := range t.Columns {
					t.Columns[i].Null = live.Columns[i].Null
					if now[t.Columns[i].Name] != declared[t.Name][t.Columns[i].Name] {
						t.Columns[i].Type = live.Columns[i].Type
					}
				}
			}
			c.Tables = append(c.Tables, t)
		}
		d.parsed = &c
	}
	for _, name := range names {
		if held[name] {
			continue
		}
		t, err := liveTable(ctx, db, name)
		if err != nil {
			return nil, err
		}
		out.Tables = append(out.Tables, *t)
	}
	return out, nil
}

func sameColumnNames(a, b []dbspec.Column) bool {
	return slices.EqualFunc(a, b, func(x, y dbspec.Column) bool { return x.Name == y.Name })
}

func liveTables(ctx context.Context, db *sql.DB) (_ []string, err error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY rowid")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// autoincrementTables returns the tables of names in db whose schema statement declares AUTOINCREMENT. SQLite keeps that
// flag to itself and creates its sqlite_sequence table with the first such table, so each statement, as db holds it,
// runs alone in a new database, and SQLite says whether it made one: no text of the statement is read for it.
func autoincrementTables(ctx context.Context, db *sql.DB, names []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, name := range names {
		var text string
		if err := db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&text); err != nil {
			return nil, err
		}
		on, err := declaresAutoincrement(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("the schema statement of %s: %w", name, err)
		}
		if on {
			out[name] = true
		}
	}
	return out, nil
}

// declaresAutoincrement reports whether the schema statement text of a table, run alone in a new database, makes SQLite
// keep an AUTOINCREMENT counter for its table.
func declaresAutoincrement(ctx context.Context, text string) (_ bool, err error) {
	db, err := sql.Open(sqlite.DriverName, ":memory:")
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, text); err != nil {
		return false, err
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_sequence'").Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// RowIDColumn is the key column of a table that SQL created without a primary key: the rowid SQLite gives its rows.
const RowIDColumn = "rowid"

// liveDeclarations reads the declared type of each column of a table.
func liveDeclarations(ctx context.Context, db *sql.DB, name string) (_ map[string]string, err error) {
	rows, err := db.QueryContext(ctx, "SELECT name, type FROM pragma_table_info(?)", name)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	out := map[string]string{}
	for rows.Next() {
		var col, decl string
		if err := rows.Scan(&col, &decl); err != nil {
			return nil, err
		}
		out[col] = decl
	}
	return out, rows.Err()
}

// liveTable reads a table of a SQLite database as a dbspec table.
func liveTable(ctx context.Context, db *sql.DB, name string) (_ *dbspec.Table, err error) {
	rows, err := db.QueryContext(ctx, "SELECT name, type, \"notnull\", pk FROM pragma_table_info(?) ORDER BY cid", name)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	t := &dbspec.Table{Name: name}
	key := map[int]string{}
	nullable := map[string]bool{}
	rowAlias := false
	for rows.Next() {
		var col, decl string
		var notNull, pk int
		if err := rows.Scan(&col, &decl, &notNull, &pk); err != nil {
			return nil, err
		}
		typ, err := liveType(decl)
		if err != nil {
			return nil, err
		}
		c := dbspec.Column{Name: col, Type: typ, Null: notNull == 0 && pk == 0}
		t.Columns = append(t.Columns, c)
		if pk > 0 {
			key[pk] = col
			nullable[col] = notNull == 0
			rowAlias = strings.EqualFold(decl, "INTEGER")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// SQLite keeps NULL in a primary key column it does not require to be NOT NULL, unless that column is the rowid. Such a
	// key names no row, and the rowid does.
	if len(key) != 1 || !rowAlias {
		for i := range t.Columns {
			if nullable[t.Columns[i].Name] {
				t.Columns[i].Null = true
				key = map[int]string{}
			}
		}
	}
	for i := 1; i <= len(key); i++ {
		t.PrimaryKey.Columns = append(t.PrimaryKey.Columns, key[i])
	}
	if len(key) == 1 {
		for i := range t.Columns {
			if t.Columns[i].Name == key[1] && t.Columns[i].Type.Kind == dbspec.TypeI64 {
				t.Columns[i].Identity = true
			}
		}
	}
	if err := namedRowid(ctx, db, name, t.Columns, len(key) == 1 && rowAlias); err != nil {
		return nil, err
	}
	if len(key) == 0 {
		// The corpus holds the rowid of a table keyed by it as its column rowid, so a column of that name, as SQLite matches
		// a name without regard to ASCII case, would stand in the place of the rowid and the rows it gives would be lost.
		for _, c := range t.Columns {
			if strings.EqualFold(c.Name, RowIDColumn) {
				return nil, fmt.Errorf("the table %s is keyed by its rowid, which the corpus holds as the column %s, and its column %s takes that name: give the table a primary key that holds no NULL, or name the column otherwise", name, RowIDColumn, c.Name)
			}
		}
		t.Columns = append([]dbspec.Column{{Name: RowIDColumn, Type: dbspec.Type{Kind: dbspec.TypeI64}, Identity: true}}, t.Columns...)
		t.PrimaryKey.Columns = []string{RowIDColumn}
	}
	return t, nil
}

// namedRowid refuses a table with a rowid that no statement can name: SQLite names the rowid by the first of rowid,
// _rowid_ and oid that names no column, so a table whose columns take all three and whose key, alias says, is no
// INTEGER PRIMARY KEY has a rowid that neither the engine nor a statement reads.
func namedRowid(ctx context.Context, db *sql.DB, name string, cols []dbspec.Column, alias bool) error {
	if alias {
		return nil
	}
	taken := map[string]bool{}
	for _, c := range cols {
		taken[strings.ToLower(c.Name)] = true
	}
	if !taken["rowid"] || !taken["_rowid_"] || !taken["oid"] {
		return nil
	}
	var without bool
	if err := db.QueryRowContext(ctx, "SELECT wr FROM pragma_table_list WHERE schema = 'main' AND name = ?", name).Scan(&without); err != nil {
		return err
	}
	if without {
		return nil
	}
	return fmt.Errorf("the table %s has a rowid that no statement names, as its columns take rowid, _rowid_ and oid and its key is no INTEGER PRIMARY KEY: name a column otherwise", name)
}

var decimalInt = regexp.MustCompile(`^DECIMALINT\((\d+),\s*(\d+)\)$`)

// liveType is the dbspec type of a declared SQLite type, by its affinity (the rules of SQLite's datatype page). SQLite has
// no date or time storage, so such a column holds the text it was given, byte for byte.
func liveType(decl string) (dbspec.Type, error) {
	upper := strings.ToUpper(strings.TrimSpace(decl))
	if m := decimalInt.FindStringSubmatch(upper); m != nil {
		p, err := strconv.Atoi(m[1])
		if err != nil {
			return dbspec.Type{}, fmt.Errorf("the declared type %s: its precision: %w", decl, err)
		}
		s, err := strconv.Atoi(m[2])
		if err != nil {
			return dbspec.Type{}, fmt.Errorf("the declared type %s: its scale: %w", decl, err)
		}
		return dbspec.Type{Kind: dbspec.TypeDecimal, Precision: p, Scale: s}, nil
	}
	if upper == "BOOLEAN" {
		return dbspec.Type{Kind: dbspec.TypeBool}, nil
	}
	switch DeclaredAffinity(decl) {
	case AffinityInteger:
		return dbspec.Type{Kind: dbspec.TypeI64}, nil
	case AffinityReal:
		return dbspec.Type{Kind: dbspec.TypeF64}, nil
	case AffinityBlob:
		// A column of no declared type holds any value, most often a text: a value of another storage class is a stored
		// literal there (spec 10 Stored literals).
		if upper != "" {
			return dbspec.Type{Kind: dbspec.TypeBytes}, nil
		}
	}
	return dbspec.Type{Kind: dbspec.TypeText}, nil
}
