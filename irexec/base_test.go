package irexec

import (
	"testing"

	"github.com/polyspec/flowmark/validate"
)

// baseDocs is the corpus of the test catalog that most cases start from: items in
// every state with their history, people with a soft-deleted row, notes in files
// with a block of steps, and tags.
func baseDocs() map[string]string {
	return map[string]string{
		"items.flowmark.md": lines(
			"# items",
			"- [x] T1 Ship the parser owner:[P1](people.flowmark.md)",
			"  - [~] T2 Write the tests due:2026-10-20",
			"- [ ] T3 Review the design",
			"- [-] T4 Drop the old path",
			"",
			"## items row T1",
			"",
			"Parser body.",
			"",
			"### evidence",
			"",
			"Golden tests pass.",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"| 2 | doing | done | 2026-10-01 09:00:00.000000 |",
			"",
			"#### item_history row 2",
			"",
			"##### evidence",
			"",
			"Golden tests pass.",
			"",
			"## items row T2",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 3 | waiting | doing | 2026-10-02 08:00:00.000000 |",
			"",
			"## items row T4",
			"",
			"### cause",
			"",
			"Superseded.",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 4 | waiting | dropped | 2026-10-03 08:00:00.000000 |",
			"",
			"#### item_history row 4",
			"",
			"##### cause",
			"",
			"Superseded.",
		),
		"people.flowmark.md": lines(
			"# people",
			"| id | name | email | age | active | score | joined | deleted_at | updated_at |",
			"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
			"| P1 | Ada | ada@x.io | 36 | true | 1.50 | 2026-10-01 09:00:00 |  |  |",
			"| P2 | Max |  |  | false | 0.00 | 2026-10-02 10:00:00 |  |  |",
			"| P3 | Gone | gone@x.io |  | true | 0.00 | 2026-10-03 00:00:00 | 2026-10-05 00:00:00.000000 |  |",
		),
		"notes/N1.flowmark.md": lines(
			"# First note",
			"- item: [T1](../items.flowmark.md#items-row-t1)",
			"- position: 1",
			"- weight: 0.5",
			"",
			"Note body.",
			"",
			"## steps",
			"- S1 Draft done:false",
			"- S2 Review done:true",
		),
		"notes/N2.flowmark.md": lines(
			"# Child note",
			"- parent: [N1](N1.flowmark.md)",
			"- position: 1",
		),
		"notes/N3.flowmark.md": lines(
			"# Second root",
			"- position: 2",
		),
		"tags.flowmark.md": lines(
			"# tags",
			"| id | label | kind |",
			"| --- | --- | --- |",
			"| 1 | go | topic |",
			"| 2 | ops | area |",
		),
	}
}

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
