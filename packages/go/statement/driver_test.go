package statement_test

import (
	"database/sql"
	"slices"
	"testing"

	"github.com/min-0-max/sqlite"
	upstream "modernc.org/sqlite"
)

// TestDriverBesideUpstream pins that the engine's SQLite driver registers beside the one of the module it is forked from
// (T59): a program that imports the engine and that module, as the orm module does, initializes, and a database the engine
// opens by its own name is served by the fork, which carries what the engine relies on, never by the module it comes from.
func TestDriverBesideUpstream(t *testing.T) {
	drivers := sql.Drivers()
	for _, name := range []string{"sqlite", sqlite.DriverName} {
		if !slices.Contains(drivers, name) {
			t.Fatalf("driver %q is not registered; registered: %v", name, drivers)
		}
	}
	if sqlite.DriverName == "sqlite" {
		t.Fatalf("the fork registers %q, the name the module it comes from registers", sqlite.DriverName)
	}
	db, err := sql.Open(sqlite.DriverName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, ok := db.Driver().(*sqlite.Driver); !ok {
		t.Fatalf("a database opened as %q is served by %T, not the fork's driver", sqlite.DriverName, db.Driver())
	}
	other, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := other.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, ok := other.Driver().(*upstream.Driver); !ok {
		t.Fatalf("a database opened as %q is served by %T, not the driver of the module the fork comes from", "sqlite", other.Driver())
	}
}
