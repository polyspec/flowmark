package store

import (
	"bytes"
	"strings"
	"testing"

	"github.com/polyspec/orm/engine/dbspec"
)

// documentTables are the fuzzed tables, one per seed golden: a list, a table with
// a parent link and sections, and a table with an order column.
var documentTables = []string{"items", "people", "milestones"}

// fuzzDocumentSeeds are the golden documents and their noncanonical spellings.
func fuzzDocumentSeeds(f *testing.F) {
	for i, table := range documentTables {
		b := readDocumentGolden(f, table)
		f.Add(uint8(i), b)
		f.Add(uint8(i), bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n")))
		f.Add(uint8(i), bytes.TrimSuffix(b, []byte("\n")))
	}
	f.Add(uint8(0), []byte(itemsOf()))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan done:false note:\"a b\"", "  - [~] T2 Child done:false", "", "## items row T2", "", "Body.")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Set owner\\: review done:false", "- [x] T2 Plan done:true", "", "## items row T2", "", "### evidence", "", "a|b")))
	f.Add(uint8(0), []byte(itemsOf(`- [ ] T1 Plan done:false note:"a\"b" weight:-3`)))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### cause")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan done:false  note:\"x\"")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan done:false\tnote:\"x\"")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan\tdone:false")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 ​Plan done:false")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan done:false note:a b")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan done:false note:\" a\"")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "A.", "", "## items row T1", "", "B.")))
	f.Add(uint8(0), []byte(itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history", "", "x")))
	f.Add(uint8(1), []byte(peopleOf(peopleHeader, peopleDelim, row("P1", "", "Ada", "", "true", "", "", "", "", "", "", "", "", ""), "", "## people row P1", "", "### bio", "", "Notes.")))
	f.Add(uint8(1), []byte(peopleOf(peopleHeader, peopleDelim, row("P1", "", `"\q"`, "", "true", "", "", "", "", "", "", "", "", ""))))
	f.Add(uint8(1), []byte(peopleOf(peopleHeader, peopleDelim, row("P1", "", "a \\| b", "", "true", "", "", "", "", "", "", "", "", ""), "", "## people row P1", "", "### bio", "", "x")))
	f.Add(uint8(2), []byte(milestonesOf("| id | parent | title |", "| --- | --- | --- |", row("M1", "[M2](milestones.flowmark.md)", "A"), row("M2", "[M1](milestones.flowmark.md)", "B"))))
}

// FuzzParseDocument requires that a parse never panics, that a document with no
// diagnostic writes back as itself (P1), and that a parsed document writes
// bytes that parse back without a diagnostic and write the same bytes again (P2).
func FuzzParseDocument(f *testing.F) {
	fuzzDocumentSeeds(f)
	cat := loadDocumentFixture(f)
	lk := documentLinker()
	f.Fuzz(func(t *testing.T, sel uint8, data []byte) {
		table := documentTables[int(sel)%len(documentTables)]
		rel := table + ".flowmark.md"
		lay, err := docLayoutOf(cat, table)
		if err != nil {
			t.Fatal(err)
		}
		rows, diags, err := ParseDocument(cat, table, rel, data, lk)
		if err != nil {
			if !strings.Contains(err.Error(), "T12.2.4") {
				t.Fatalf("parse refused with an unexpected error: %v\ninput: %q", err, data)
			}
			return
		}
		if rows == nil {
			if len(diags) == 0 {
				t.Fatal("no rows and no diagnostic")
			}
			return
		}
		// Two rows with byte-for-byte equal keys are readable, but their table is
		// unavailable to a write (DUPLICATE_KEY), so the emission is the read-side one.
		out, rd, err := writeDocument(cat, table, rows, lk)
		if err != nil || len(rd) > 0 {
			for _, d := range rd {
				if d.Rule != RuleDuplicateKey {
					t.Fatalf("a parsed document is refused on write: %v %+v\ninput: %q", err, rd, data)
				}
			}
			out, rd = renderDocument(lay, rows, lk, false)
			if len(rd) > 0 {
				t.Fatalf("a parsed document is refused on the read-side emission: %+v\ninput: %q", rd, data)
			}
		}
		back, pd, err := ParseDocument(cat, table, rel, out, lk)
		if err != nil || len(pd) > 0 || back == nil {
			t.Fatalf("emitted document does not parse cleanly: %v %+v\ninput: %q\nemit: %q", err, pd, data, out)
		}
		again, rd := renderDocument(lay, back, lk, false)
		if len(rd) > 0 || !bytes.Equal(again, out) {
			t.Fatalf("P2 failed\ninput: %q\nemit: %q\nagain: %q", data, out, again)
		}
		if len(diags) == 0 && !bytes.Equal(out, data) {
			t.Fatalf("P1 failed\ninput: %q\nemit: %q", data, out)
		}
	})
}

// FuzzRenderDocument requires that rows the writer accepts write a canonical
// document that parses back to the same rows and writes the same bytes.
func FuzzRenderDocument(f *testing.F) {
	f.Add("Plan the parser", "Intro line.\n", `it's: "quoted" ünï`, "1", "P1", "true", "Proof.\n")
	f.Add("Set owner: review", "", "", "2", "", "false", "")
	f.Add("a \\#", "```\n## x\n```\n", "a\\\"b", "7", "", "true", "")
	f.Add("Title", "", "a​", "1", "", "true", "")
	f.Add("Title", "", "ㅤa͏", "1", "", "true", "")
	f.Add(" a", "", "", "1", "", "true", "")
	cat := loadDocumentFixture(f)
	lk := documentLinker()
	f.Fuzz(func(t *testing.T, title, body, note, position, owner, done, evidence string) {
		doneText := strings.ToLower(done)
		if doneText != "true" && doneText != "false" {
			return
		}
		sc := map[string]*string{"state": sp("waiting"), "position": &position, "done": &doneText, "note": &note}
		if owner != "" {
			sc["owner"] = &owner
		}
		r := &Row{
			Table: "items", Key: "T1", Title: title, Body: body, Scalars: sc,
			Sections: map[string]*string{"evidence": &evidence}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{},
		}
		out, diags, err := writeDocument(cat, "items", []*Row{r}, lk)
		if err != nil || len(diags) > 0 {
			return
		}
		back, pd, err := ParseDocument(cat, "items", "items.flowmark.md", out, lk)
		if err != nil || len(pd) > 0 || len(back) != 1 {
			t.Fatalf("written document does not parse cleanly: %v %+v\ntitle %q body %q\nout %q", err, pd, title, body, out)
		}
		if back[0].Title != title || back[0].Body != body || *back[0].Scalars["note"] != note {
			t.Fatalf("row changed on the round trip: %+v", back[0])
		}
		if want, ok := readable(dbspec.Type{Kind: dbspec.TypeI32}, position); !ok || *back[0].Scalars["position"] != want {
			t.Fatalf("position changed on the round trip: %q", position)
		}
		again, rd, err := writeDocument(cat, "items", back, lk)
		if err != nil || len(rd) > 0 || !bytes.Equal(again, out) {
			t.Fatalf("P2 failed: %q vs %q", again, out)
		}
	})
}
