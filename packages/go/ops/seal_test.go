package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/orm/packages/go/orm"
)

const sealedDBS = `dbspec 1 flowmark

table people {
  id i64 identity
  name varchar(64)
  mail varchar(255) null
  mail_index varchar(64) null
  phone bytes null
  mail_version i32
  primary key (id)
  index ix_people_mail (mail_index)
  settings {
    store document table
    title name
    codec mail aes hex
    codec phone aes
    aes_version mail_version
    blind_index mail mail_index
  }
}
`

// sealedCorpus opens a corpus of the sealed catalog in a process with the keyring text, none when it is empty.
func sealedCorpus(t *testing.T, keyring string) (*Corpus, string) {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{"dbspec/flowmark.dbs": sealedDBS})
	t.Setenv(catalog.KeyringVariable, "")
	if keyring != "" {
		path := filepath.Join(t.TempDir(), "keyring.json")
		if err := os.WriteFile(path, []byte(keyring), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(catalog.KeyringVariable, path)
	}
	return openCorpus(t, root), root
}

func runOp(t *testing.T, c *Corpus, op, members string) (map[string]any, error) {
	t.Helper()
	raw, err := c.RunText(context.Background(), "test", op, members)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out, nil
}

// TestSealedWrites holds the writes a face originates on an aes column (spec 30 Keys).
func TestSealedWrites(t *testing.T) {
	const two = `{"current":2,"versions":{"1":"one","2":"two"},"blind_index":"index"}`
	const insert = `{"table":"people","values":{"name":"Ann","mail":"ann@example.com","phone":"010"}}`
	t.Run("an insert stores the ciphertext, the current version and the blind index", func(t *testing.T) {
		c, root := sealedCorpus(t, two)
		if _, err := runOp(t, c, "row.insert", insert); err != nil {
			t.Fatal(err)
		}
		text, err := os.ReadFile(filepath.Join(root, "people.flowmark.md"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(text), "ann@example.com") || strings.Contains(string(text), "| 010 |") {
			t.Fatalf("the corpus holds the plain text:\n%s", text)
		}
		row := oneRow(t, c, 1)
		plain, err := orm.HostDecode(row["mail"], []string{"aes", "hex"}, "two")
		if err != nil || plain != "ann@example.com" {
			t.Fatalf("mail decodes to %v, %v", plain, err)
		}
		want, err := orm.BlindIndex("ann@example.com", "index")
		if err != nil {
			t.Fatal(err)
		}
		if row["mail_index"] != want || row["mail_version"] != float64(2) {
			t.Fatalf("row %v, want the index %s and version 2", row, want)
		}
	})
	t.Run("an update of every aes column takes the current version", func(t *testing.T) {
		c, _ := sealedCorpus(t, two)
		if _, err := runOp(t, c, "row.insert", insert); err != nil {
			t.Fatal(err)
		}
		if _, err := runOp(t, c, "row.update", `{"table":"people","key":1,"values":{"mail":"new@example.com","phone":null}}`); err != nil {
			t.Fatal(err)
		}
		row := oneRow(t, c, 1)
		if plain, err := orm.HostDecode(row["mail"], []string{"aes", "hex"}, "two"); err != nil || plain != "new@example.com" {
			t.Fatalf("mail decodes to %v, %v", plain, err)
		}
		if row["mail_version"] != float64(2) || row["phone"] != nil {
			t.Fatalf("row %v", row)
		}
	})
	t.Run("a null clears the column and its blind index", func(t *testing.T) {
		c, _ := sealedCorpus(t, two)
		if _, err := runOp(t, c, "row.insert", insert); err != nil {
			t.Fatal(err)
		}
		if _, err := runOp(t, c, "row.update", `{"table":"people","key":1,"values":{"mail":null,"phone":null}}`); err != nil {
			t.Fatal(err)
		}
		if row := oneRow(t, c, 1); row["mail"] != nil || row["mail_index"] != nil {
			t.Fatalf("row %v", row)
		}
	})
	for _, f := range []struct{ name, keyring, op, members, code, text string }{
		{"a write of an aes column without a keyring", "", "row.insert", insert, codeConfig, catalog.KeyringVariable},
		{"a value that is no text", two, "row.insert", `{"table":"people","values":{"name":"Ann","mail":7}}`, codeConfig, "not text"},
		{"a blind index without its key", `{"current":1,"versions":{"1":"one"}}`, "row.insert", insert, codeConfig, "blind_index key"},
		{"an update of a row that does not exist", two, "row.update", `{"table":"people","key":9,"values":{"mail":"a","phone":"b"}}`, codeNoRows, "9"},
		{"an update of one aes column of two", two, "row.update", `{"table":"people","key":1,"values":{"mail":"a"}}`, codeIRInvalid, "must assign every AES column"},
		{"a write that assigns the version", two, "row.insert", `{"table":"people","values":{"name":"Ann","mail_version":1}}`, codeIRInvalid, "managed by the AES writer"},
	} {
		t.Run(f.name, func(t *testing.T) {
			c, _ := sealedCorpus(t, f.keyring)
			_, err := runOp(t, c, f.op, f.members)
			code, msg := Failure(err)
			if code != f.code || !strings.Contains(msg, f.text) {
				t.Fatalf("got %s: %s, want %s naming %q", code, msg, f.code, f.text)
			}
		})
	}
	t.Run("an update that assigns no aes column needs no keyring", func(t *testing.T) {
		c, root := sealedCorpus(t, "")
		corpustest.WriteTree(t, root, map[string]string{"people.flowmark.md": "# people\n| id | name | mail | mail_index | phone | mail_version |\n| --- | --- | --- | --- | --- | --- |\n| 1 | Ann |  |  |  | 2 |\n"})
		if _, err := runOp(t, c, "row.update", `{"table":"people","key":1,"values":{"name":"Bea"}}`); err != nil {
			t.Fatal(err)
		}
	})
}

func oneRow(t *testing.T, c *Corpus, key int) map[string]any {
	t.Helper()
	out, err := runOp(t, c, "row.get", fmt.Sprintf(`{"table":"people","key":%d}`, key))
	if err != nil {
		t.Fatal(err)
	}
	row, ok := out["row"].(map[string]any)
	if !ok {
		t.Fatalf("answer %v", out)
	}
	return row
}
