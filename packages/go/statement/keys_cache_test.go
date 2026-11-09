package statement

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/min-0-max/sqlite"
)

// TestKeysCacheCurrent pins that what capture.prepare holds of the schema — the primary key of each table without a
// rowid, and the name of the rowid of each table with one and whether it keys the table — is that of the schema the
// database holds. A schema left as it was keeps what prepare read, and a schema changed is read again, a schema that a
// rollback gave the schema_version of the schema the cache was read from included, so nothing of another definition is
// kept.
func TestKeysCacheCurrent(t *testing.T) {
	cases := []struct {
		name string
		// first is the schema the cache is read from inside a transaction, which a rollback takes back when undone is
		// set; then is the schema that follows.
		first, then []string
		undone      bool
		// sameVersion says that then reaches the schema_version first was read at, which the case needs to hold.
		sameVersion bool
		want        string
	}{
		{"a schema left as it was", []string{"CREATE TABLE w (b TEXT, a TEXT, PRIMARY KEY (b, a)) WITHOUT ROWID"}, nil, false, true, "w b,a"},
		{"a table added", []string{"CREATE TABLE w (a TEXT PRIMARY KEY, b TEXT) WITHOUT ROWID"},
			[]string{"CREATE TABLE r (rowid TEXT PRIMARY KEY NOT NULL, b TEXT)"}, false, false, "r _rowid_ keyed false;w a"},
		{"a key a rollback took back", []string{"CREATE TABLE w (a TEXT PRIMARY KEY, b TEXT) WITHOUT ROWID"},
			[]string{"CREATE TABLE w (b TEXT, a TEXT, PRIMARY KEY (b, a)) WITHOUT ROWID"}, true, true, "w b,a"},
		{"a rowid a rollback took back", []string{"CREATE TABLE r (a TEXT, b TEXT)"},
			[]string{"CREATE TABLE r (rowid TEXT PRIMARY KEY NOT NULL, b TEXT)"}, true, true, "r _rowid_ keyed false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			pool, err := sql.Open(sqlite.DriverName, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			pool.SetMaxOpenConns(1)
			defer closeDB(t, pool)
			db, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			}()
			exec := func(text string) {
				t.Helper()
				if _, err := db.ExecContext(ctx, text); err != nil {
					t.Fatalf("%s: %v", text, err)
				}
			}
			version := func() int64 {
				t.Helper()
				var v int64
				if err := db.QueryRowContext(ctx, "PRAGMA main.schema_version").Scan(&v); err != nil {
					t.Fatal(err)
				}
				return v
			}
			cp := &capture{}
			prepared := func() string {
				t.Helper()
				if err := cp.prepare(ctx, db); err != nil {
					t.Fatal(err)
				}
				var out []string
				for name, k := range cp.keys {
					out = append(out, name+" "+strings.Join(k.names, ","))
				}
				for name, rowid := range cp.names {
					_, keyed := cp.ids[name]
					if keyed {
						out = append(out, name+" "+rowid+" keyed true")
					} else {
						out = append(out, name+" "+rowid+" keyed false")
					}
				}
				slices.Sort(out)
				return strings.Join(out, ";")
			}
			exec("BEGIN")
			for _, text := range tc.first {
				exec(text)
			}
			before := prepared()
			keys := cp.keys
			at := version()
			if tc.undone {
				exec("ROLLBACK")
			} else {
				exec("COMMIT")
			}
			for _, text := range tc.then {
				exec(text)
			}
			if v := version(); (v == at) != tc.sameVersion {
				t.Fatalf("the schema_version is %d after %d, so the case does not hold", v, at)
			}
			if got := prepared(); got != tc.want {
				t.Fatalf("prepare holds %q, want %q (it read %q before)", got, tc.want, before)
			}
			if len(tc.then) == 0 && !tc.undone && !sameMap(keys, cp.keys) {
				t.Fatal("prepare read the keys of a schema left as it was again")
			}
		})
	}
}

// sameMap says whether a and b are one map, as a read again makes another.
func sameMap(a, b map[string]*primaryKey) bool {
	for name, k := range a {
		if b[name] != k {
			return false
		}
	}
	return len(a) == len(b)
}
