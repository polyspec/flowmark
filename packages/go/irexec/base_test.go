package irexec

import (
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/validate"
)

// baseDocs is corpustest.BaseDocs, the corpus of the test catalog that most cases start from.
func baseDocs() map[string]string { return corpustest.BaseDocs() }

// baseFixture is a corpus of the test catalog holding baseDocs.
func baseFixture(t testing.TB) *fixture {
	t.Helper()
	return catalogFixture(t, baseDocs())
}

// TestBaseCorpusValidatesClean requires the base corpus to be an accepting case:
// a corpus the validator reports nothing on.
func TestBaseCorpusValidatesClean(t *testing.T) {
	f := baseFixture(t)
	diags, err := validate.Corpus(f.cat, f.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diags {
		t.Errorf("%s:%d:%d %s: %s", d.File, d.Line, d.Column, d.Rule, d.Message)
	}
}
