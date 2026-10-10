package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/polyspec/orm/engine/dbspec"
)

// fuzzSeeds are the golden tasks files and the noncanonical spellings of them.
func fuzzSeeds(f *testing.F) {
	for _, rel := range []string{"tasks/T1.flowmark.md", "tasks/T2.flowmark.md", "tasks/T3.flowmark.md", "tasks/T4.flowmark.md"} {
		b, err := os.ReadFile(filepath.Join(fixtureRoot, rel))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
		f.Add(bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n")))
		f.Add(bytes.TrimSuffix(b, []byte("\n")))
	}
	f.Add([]byte(withBase()))
	f.Add([]byte(withBase("## evidence", "", "a", "```", "## x", "```")))
	f.Add([]byte(withBase("## cause", "", "## evidence", "")))
	f.Add([]byte(withBase("## task_steps")))
	f.Add([]byte(withBase("## task_steps", "- S1 Draft done:true", "", "### task_steps row S1", "", "#### step_refs", "| id | label |", "| --- | --- |", "| 1 | spec |", "", "##### step_refs row 1", "", "###### note", "", "Ref.")))
	f.Add([]byte(withBase("## cause", "", "Cause.", "", "## task_steps", "- S1 Draft done:true", "  - S2 Child done:false")))
	// Invisible characters at the ends of the single-line places, the text that is not one, and the interior.
	f.Add([]byte(doc("#  Title", "- position: 1", "- done: true")))
	f.Add([]byte(doc("# Title​", "- position: 1", "- done: true")))
	f.Add([]byte(doc("# Title", "- position: 1", "- done: true", "- note: ㅤa", "")))
	f.Add([]byte(doc("# Title", "- position: 1", "- done: true", "- note: a͏", "")))
	f.Add([]byte(doc("# Title", "- position: 1", "- done: true", "- note:  a۝", "")))
	f.Add([]byte(doc("# Title", "- position: 1", "- done: true", "- note: a\u001c", "")))
	f.Add([]byte(doc("# Title", "- position: 1", "- done: true", "- note: a b", "")))
}

// FuzzParseFile requires that a parse never panics, that a parsed row emits
// without refusal and that the emitted file parses back to the same bytes (P2),
// and that an input without diagnostic is emitted as itself (P1).
func FuzzParseFile(f *testing.F) {
	fuzzSeeds(f)
	cat := loadFixture(f)
	lk := fixtureLinker()
	f.Fuzz(func(t *testing.T, data []byte) {
		row, diags := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", data, lk)
		if row == nil {
			if len(diags) == 0 {
				t.Fatal("no row and no diagnostic")
			}
			return
		}
		out, rd := RenderFile(cat, "tasks", row, lk)
		if len(rd) > 0 {
			t.Fatalf("a parsed row is refused on write: %+v\ninput: %q", rd, data)
		}
		back, pd := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", out, lk)
		if len(pd) > 0 || back == nil {
			t.Fatalf("emitted file does not parse cleanly: %+v\ninput: %q\nemit: %q", pd, data, out)
		}
		again, rd := RenderFile(cat, "tasks", back, lk)
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
		out, diags := RenderFile(cat, "tasks", row, lk)
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
		again, rd := RenderFile(cat, "tasks", back, lk)
		if len(rd) > 0 || !bytes.Equal(again, out) {
			t.Fatalf("P2 failed: %q vs %q", again, out)
		}
	})
}
