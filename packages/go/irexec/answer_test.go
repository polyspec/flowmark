package irexec

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestRowJSONReadsAsStored pins that a row's strings are written without HTML escaping, also through an encoder that
// does not escape: the faces answer a value as it is stored (spec 40 The faces).
func TestRowJSONReadsAsStored(t *testing.T) {
	r := newRow()
	r.set("na<me", "Max & Co <x>")
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		t.Fatal(err)
	}
	if got, want := b.String(), `{"na<me":"Max & Co <x>"}`+"\n"; got != want {
		t.Fatalf("row = %s, want %s", got, want)
	}
	for _, c := range []struct {
		a    *Answer
		want string
	}{
		{&Answer{Kind: "all", Rows: []*Row{r}}, `{"rows":[{"na<me":"Max & Co <x>"}]}`},
		{&Answer{Kind: "paginate", Rows: []*Row{r}, Count: 1}, `{"rows":[{"na<me":"Max & Co <x>"}],"count":1}`},
		{&Answer{Kind: "sum", Value: "a&b"}, `{"value":"a&b"}`},
		{&Answer{Kind: "insert", Affected: 1, Keys: []any{"K&1"}}, `{"affected":1,"keys":["K&1"]}`},
	} {
		b.Reset()
		if err := enc.Encode(c.a); err != nil {
			t.Fatal(err)
		}
		if got := b.String(); got != c.want+"\n" {
			t.Fatalf("%s answer = %s, want %s", c.a.Kind, got, c.want)
		}
	}
}
