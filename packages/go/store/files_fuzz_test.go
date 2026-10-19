package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// fileTarget is one table the file fuzzer reads: its catalog, its table, the file
// it reads and the linker of its corpus.
type fileTarget struct {
	cat   *catalog.Catalog
	table string
	rel   string
	lk    Linker
}

// fileTargets are the tables the file fuzzer reads, selected by a byte: the tasks of
// the fixture, the entries of the files corpus, and the plans of the block corpora,
// as a list (target 2) and as a table (target 3).
func fileTargets(f testing.TB) []fileTarget {
	return []fileTarget{
		{loadFixture(f), "tasks", "tasks/T1.flowmark.md", fixtureLinker()},
		{equivCatalog(f, equivFiles), "entries", "entries/E1.flowmark.md", equivLinker()},
		{equivCatalog(f, equivBlock), "plans", "plans/P1.flowmark.md", equivBlockLinker()},
		{equivCatalog(f, equivBlockTable), "plans", "plans/P1.flowmark.md", equivBlockLinker()},
	}
}

// fuzzSeeds are the golden documents of the file targets and their noncanonical
// spellings: the tasks files of the fixture (target 0), the entries of the files
// corpus (target 1), and the plans of the block corpora (targets 2 and 3).
func fuzzSeeds(f *testing.F) {
	seed := func(sel uint8, b []byte) {
		f.Add(sel, b)
		f.Add(sel, bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n")))
		f.Add(sel, bytes.TrimSuffix(b, []byte("\n")))
	}
	for _, rel := range []string{"tasks/T1.flowmark.md", "tasks/T2.flowmark.md", "tasks/T3.flowmark.md", "tasks/T4.flowmark.md"} {
		b, err := os.ReadFile(filepath.Join(fixtureRoot, rel))
		if err != nil {
			f.Fatal(err)
		}
		seed(0, b)
	}
	for _, s := range []struct {
		sel  uint8
		root string
	}{{1, equivFiles}, {2, equivBlock}, {3, equivBlockTable}} {
		for _, rel := range equivGoldens(equivCorpusAt(s.root)) {
			seed(s.sel, readEquiv(f, s.root, rel))
		}
	}
	f.Add(uint8(0), []byte(withBase()))
	f.Add(uint8(0), []byte(withBase("## evidence", "", "a", "```", "## x", "```")))
	f.Add(uint8(0), []byte(withBase("## cause", "", "## evidence", "")))
	f.Add(uint8(0), []byte(withBase("## task_steps")))
	f.Add(uint8(0), []byte(withBase("## task_steps", "- S1 Draft done:true", "", "### task_steps row S1", "", "#### step_refs", "| id | label |", "| --- | --- |", "| 1 | spec |", "", "##### step_refs row 1", "", "###### note", "", "Ref.")))
	f.Add(uint8(0), []byte(withBase("## cause", "", "Cause.", "", "## task_steps", "- S1 Draft done:true", "  - S2 Child done:false")))
	// Invisible characters at the ends of the single-line places, the text that is not one, and the interior.
	f.Add(uint8(0), []byte(doc("#  Title", "- position: 1", "- done: true")))
	f.Add(uint8(0), []byte(doc("# Title​", "- position: 1", "- done: true")))
	f.Add(uint8(0), []byte(doc("# Title", "- position: 1", "- done: true", "- note: ㅤa", "")))
	f.Add(uint8(0), []byte(doc("# Title", "- position: 1", "- done: true", "- note: a͏", "")))
	f.Add(uint8(0), []byte(doc("# Title", "- position: 1", "- done: true", "- note:  a۝", "")))
	f.Add(uint8(0), []byte(doc("# Title", "- position: 1", "- done: true", "- note: a\u001c", "")))
	f.Add(uint8(0), []byte(doc("# Title", "- position: 1", "- done: true", "- note: a b", "")))
}

// FuzzParseFile requires that a parse never panics, that a parsed row emits
// without refusal and that the emitted file parses back to the same bytes (P2),
// and that an input without diagnostic is emitted as itself (P1).
func FuzzParseFile(f *testing.F) {
	fuzzSeeds(f)
	targets := fileTargets(f)
	f.Fuzz(func(t *testing.T, sel uint8, data []byte) {
		tg := targets[int(sel)%len(targets)]
		cat, table, rel, lk := tg.cat, tg.table, tg.rel, tg.lk
		row, diags := ParseFile(cat, table, rel, data, lk)
		if row == nil {
			if len(diags) == 0 {
				t.Fatal("no row and no diagnostic")
			}
			return
		}
		out, rd := renderFileOf(t, cat, table, row, lk)
		if len(rd) > 0 {
			t.Fatalf("a parsed row is refused on write: %+v\ninput: %q", rd, data)
		}
		back, pd := ParseFile(cat, table, rel, out, lk)
		if len(pd) > 0 || back == nil {
			t.Fatalf("emitted file does not parse cleanly: %+v\ninput: %q\nemit: %q", pd, data, out)
		}
		again, rd := renderFileOf(t, cat, table, back, lk)
		if len(rd) > 0 || !bytes.Equal(again, out) {
			t.Fatalf("P2 failed\ninput: %q\nemit: %q\nagain: %q", data, out, again)
		}
		if len(diags) == 0 && !bytes.Equal(out, data) {
			t.Fatalf("P1 failed\ninput: %q\nemit: %q", data, out)
		}
	})
}

// FuzzRenderFile requires that a row the writer accepts is a canonical file
// that parses back to the same row and emits the same bytes.
func FuzzRenderFile(f *testing.F) {
	f.Add("Parse and render the files storage", "Intro line.\n", `it's: "quoted" ünï`, "1", "max", "Observed.\n", true)
	f.Add("Count the #", "", "", "7", "", "", false)
	f.Add("a \\#", "```\n## x\n```\n", "<3", "-3", "x", "", true)
	f.Add("a ", "", "۝", "1", "", "", true)
	f.Add(" a", "", "", "1", "", "", true)
	f.Add("Title", "", "a​", "1", "", "", true)
	f.Add("Title", "", "ㅤa͏", "1", "", "", true)
	cat := loadFixture(f)
	lk := fixtureLinker()
	f.Fuzz(func(t *testing.T, title, body, note, position, owner, evidence string, done bool) {
		doneText := strconv.FormatBool(done)
		row := &Row{
			Table: "tasks", Key: "T1", Title: title, Body: body,
			Scalars:  map[string]*string{"position": &position, "done": &doneText, "note": &note, "owner": &owner},
			Sections: map[string]*string{"evidence": &evidence},
		}
		out, diags := renderFileOf(t, cat, "tasks", row, lk)
		if len(diags) > 0 {
			return
		}
		back, pd := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", out, lk)
		if len(pd) > 0 || back == nil {
			t.Fatalf("written file does not parse cleanly: %+v\ntitle %q body %q\nout %q", pd, title, body, out)
		}
		// An integer is written in its canonical form, so the expected text is the canonical one.
		wantPosition, ok := readable(dbspec.Type{Kind: dbspec.TypeI32}, position)
		if !ok {
			t.Fatalf("an integer the writer accepted is not canonical: %q", position)
		}
		if back.Title != title || back.Body != body || *back.Scalars["position"] != wantPosition || *back.Scalars["note"] != note {
			t.Fatalf("row changed on the round trip: %+v", back)
		}
		if !strings.HasPrefix(string(out), "# ") {
			t.Fatalf("no H1: %q", out)
		}
		again, rd := renderFileOf(t, cat, "tasks", back, lk)
		if len(rd) > 0 || !bytes.Equal(again, out) {
			t.Fatalf("P2 failed: %q vs %q", again, out)
		}
	})
}
