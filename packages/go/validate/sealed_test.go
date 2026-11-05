package validate

import (
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
  id i64
  name varchar(64)
  mail varchar(255) null
  mail_version i32
  primary key (id)
  settings {
    store document table
    title name
    codec mail aes hex
    aes_version mail_version
  }
}
`

// TestSealedValues holds the codec rule of an aes column (spec 50 Codec values, spec 30 Keys).
func TestSealedValues(t *testing.T) {
	cipher := func(key string) string {
		v, err := orm.HostEncode("ann@example.com", []string{"aes", "hex"}, key)
		if err != nil {
			t.Fatal(err)
		}
		text, ok := v.(string)
		if !ok {
			t.Fatalf("the sealed value is %T, not a text", v)
		}
		return text
	}
	const keyring = `{"current":2,"versions":{"1":"one","2":"two"}}`
	for _, c := range []struct{ name, keyring, mail, version, want string }{
		{"a value of the version its row records", keyring, cipher("one"), "1", ""},
		{"a value of the current version", keyring, cipher("two"), "2", ""},
		{"a null value", keyring, "", "2", ""},
		{"a value of another key", keyring, cipher("two"), "1", "codec ~ mail"},
		{"plain text", keyring, "ann@example.com", "2", "codec ~ mail"},
		{"a version the keyring does not declare", keyring, cipher("two"), "3", "codec ~ no key version 3"},
		{"no keyring leaves the value unjudged", "", "ann@example.com", "2", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			corpustest.WriteTree(t, root, map[string]string{
				"dbspec/flowmark.dbs": sealedDBS,
				"people.flowmark.md":  "# people\n| id | name | mail | mail_version |\n| --- | --- | --- | --- |\n| 1 | Ann | " + c.mail + " | " + c.version + " |\n",
			})
			t.Setenv(catalog.KeyringVariable, "")
			if c.keyring != "" {
				path := filepath.Join(t.TempDir(), "keyring.json")
				if err := os.WriteFile(path, []byte(c.keyring), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv(catalog.KeyringVariable, path)
			}
			cat, diags := catalog.Load(root)
			if len(diags) > 0 {
				t.Fatalf("catalog: %+v", diags)
			}
			got, err := Corpus(cat, root)
			if err != nil {
				t.Fatal(err)
			}
			if c.want == "" {
				if len(got) > 0 {
					t.Fatalf("diagnostics: %+v", got)
				}
				return
			}
			rule, text, _ := strings.Cut(c.want, " ~ ")
			if len(got) != 1 || got[0].Rule != rule || !strings.Contains(got[0].Message, text) {
				t.Fatalf("diagnostics %+v, want one %s", got, c.want)
			}
		})
	}
}
