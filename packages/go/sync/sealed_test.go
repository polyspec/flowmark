package sync

import (
	"reflect"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/orm"
)

// TestExportKeepsSealedCells pins that the export of an aes column needs no key and keeps the ciphertext, the version and
// the blind index as they are stored (spec 30 Keys).
func TestExportKeepsSealedCells(t *testing.T) {
	cipher, err := orm.HostEncode("ann@example.com", []string{"aes", "hex"}, "old")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/flowmark.dbs": `dbspec 1 flowmark

table people {
  id i64 identity
  name varchar(64)
  mail varchar(255) null
  mail_index varchar(64) null
  mail_version i32
  primary key (id)
  index ix_people_mail (mail_index)
  settings {
    store document table
    title name
    codec mail aes hex
    aes_version mail_version
    blind_index mail mail_index
  }
}
`,
		"people.flowmark.md": "# people\n| id | name | mail | mail_index | mail_version |\n| --- | --- | --- | --- | --- |\n| 1 | Ann | " + cipher.(string) + " | abc | 1 |\n",
	})
	c := open(t, root)
	stmts, err := Export(c, dbspec.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	db := apply(t, c, stmts)
	want, err := c.Rows("people")
	if err != nil {
		t.Fatal(err)
	}
	if got := readBack(t, db, c, "people"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
