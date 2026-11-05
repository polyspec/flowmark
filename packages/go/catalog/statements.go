package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/polyspec/orm/packages/go/engine/dbspec"
	_ "modernc.org/sqlite" // the database that a schema statement of a corpus runs in
)

// StatementsFile is the file of the schema statements a corpus took in SQL, relative to its root, a JSON array of their
// texts in order (spec 40 SQL statements). The document set declares what the set owns, and this file what SQL added.
const StatementsFile = dbspecDir + "/statements.json"

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
func (l *loader) applyStatements(docs []*document, added []string) (*dbspec.Document, error) {
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
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
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

func liveTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY rowid")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

// RowIDColumn is the key column of a table that SQL created without a primary key: the rowid SQLite gives its rows.
const RowIDColumn = "rowid"

// liveDeclarations reads the declared type of each column of a table.
func liveDeclarations(ctx context.Context, db *sql.DB, name string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT name, type FROM pragma_table_info(?)", name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
func liveTable(ctx context.Context, db *sql.DB, name string) (*dbspec.Table, error) {
	rows, err := db.QueryContext(ctx, "SELECT name, type, \"notnull\", pk FROM pragma_table_info(?) ORDER BY cid", name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
		c := dbspec.Column{Name: col, Type: liveType(decl), Null: notNull == 0 && pk == 0}
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
	if !(len(key) == 1 && rowAlias) {
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
	if len(key) == 0 {
		t.Columns = append([]dbspec.Column{{Name: RowIDColumn, Type: dbspec.Type{Kind: dbspec.TypeI64}, Identity: true}}, t.Columns...)
		t.PrimaryKey.Columns = []string{RowIDColumn}
	}
	return t, nil
}

var decimalInt = regexp.MustCompile(`^DECIMALINT\((\d+),\s*(\d+)\)$`)

// liveType is the dbspec type of a declared SQLite type, by its affinity (the rules of SQLite's datatype page). SQLite has
// no date or time storage, so such a column holds the text it was given, byte for byte.
func liveType(decl string) dbspec.Type {
	upper := strings.ToUpper(strings.TrimSpace(decl))
	if m := decimalInt.FindStringSubmatch(upper); m != nil {
		p, _ := strconv.Atoi(m[1])
		s, _ := strconv.Atoi(m[2])
		return dbspec.Type{Kind: dbspec.TypeDecimal, Precision: p, Scale: s}
	}
	switch {
	case upper == "BOOLEAN":
		return dbspec.Type{Kind: dbspec.TypeBool}
	case strings.Contains(upper, "INT"):
		return dbspec.Type{Kind: dbspec.TypeI64}
	case strings.Contains(upper, "BLOB"):
		return dbspec.Type{Kind: dbspec.TypeBytes}
	case strings.Contains(upper, "REAL"), strings.Contains(upper, "FLOA"), strings.Contains(upper, "DOUB"):
		return dbspec.Type{Kind: dbspec.TypeF64}
	}
	return dbspec.Type{Kind: dbspec.TypeText}
}
