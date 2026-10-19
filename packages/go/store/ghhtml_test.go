package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// The vectors of GitHub's reading of HTML headings (spec 10, Detail headings and
// anchors), recorded by scripts/record-slugs.mts into testdata/slug.

// ghHTMLFile is the part of github-headings.json that records how GitHub reads HTML.
type ghHTMLFile struct {
	RawHeadings []struct {
		Name     string    `json:"name"`
		Markdown string    `json:"markdown"`
		IDs      []*string `json:"ids"`
	} `json:"rawHeadings"`
	HTMLEntities map[string]string `json:"htmlEntities"`
	Closers      map[string]struct {
		Open []*string `json:"open"`
		End  []*string `json:"end"`
	} `json:"closers"`
	Nested map[string]map[string][]*string `json:"nested"`
}

func readGHHTML(t *testing.T) ghHTMLFile {
	t.Helper()
	src, err := os.ReadFile("testdata/slug/github-headings.json")
	if err != nil {
		t.Fatal(err)
	}
	var f ghHTMLFile
	if err := json.Unmarshal(src, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// ghReadingsOf returns GitHub's reading of the heading elements of a document.
func ghReadingsOf(markdown string) []ghRead {
	lines := strings.Split(markdown, "\n")
	v := readViewers(lines, scanHeadings(lines, 0))
	return ghReadings(lines, v.gh.sc, v.gh.refs)
}

// checkGitHubIDs requires the readings of a document to reproduce the id GitHub gives
// each of its heading elements: a heading surely inside another has none; a heading
// that the engine reads to its end has one slug, which is GitHub's id (or the id
// numbered from it, when an earlier heading had that slug or id; or none, when the slug
// is empty); a heading that continues past what the engine reads has an id that its slug
// so far begins. A heading that may end at several places is no exact reading, and
// fails here.
func checkGitHubIDs(t *testing.T, markdown string, ids []*string) {
	t.Helper()
	reads := ghReadingsOf(markdown)
	if len(reads) != len(ids) {
		t.Fatalf("%d heading elements; GitHub rendered %d", len(reads), len(ids))
	}
	seen := map[string]bool{}
	for i, r := range reads {
		id := ids[i]
		if !r.anchored {
			if id != nil {
				t.Fatalf("element %d is inside another heading; GitHub gives it %q", i+1, *id)
			}
			continue
		}
		ends, so, cont := r.slugs()
		distinct := map[string]bool{}
		for _, s := range ends {
			distinct[s] = true
		}
		switch {
		case cont:
			if len(ends) > 0 {
				t.Fatalf("element %d may end at %q and continues with %q", i+1, ends, so)
			}
			if id == nil || !strings.HasPrefix(*id, so) {
				t.Fatalf("element %d continues with the slug %q; GitHub renders %v", i+1, so, show(id))
			}
		case len(distinct) != 1:
			t.Fatalf("element %d may end with the slugs %q; GitHub renders %v", i+1, ends, show(id))
		default:
			s := ends[0]
			switch {
			case id == nil:
				if s != "" {
					t.Fatalf("element %d: slug %q; GitHub gives no anchor", i+1, s)
				}
			case !seen[s] && *id != s:
				t.Fatalf("element %d: slug %q; GitHub renders %q", i+1, s, *id)
			case !takesAnchor(s, *id):
				t.Fatalf("element %d: slug %q does not take GitHub's id %q", i+1, s, *id)
			}
			seen[s] = true
		}
		if id != nil {
			seen[*id] = true
		}
	}
}

func show(id *string) string {
	if id == nil {
		return "no anchor"
	}
	return fmt.Sprintf("%q", *id)
}

// TestGitHubRawHeadings requires GitHub's reading to reproduce its ids of the raw HTML
// heading cases.
func TestGitHubRawHeadings(t *testing.T) {
	for _, c := range readGHHTML(t).RawHeadings {
		t.Run(c.Name, func(t *testing.T) { checkGitHubIDs(t, c.Markdown, c.IDs) })
	}
}

// TestGitHubClosers requires GitHub's reading to reproduce, for every element name the
// vectors record, its ids of a raw heading holding the start tag or the end tag of
// that name, and holding that element open before each tag that ends a heading.
func TestGitHubClosers(t *testing.T) {
	f := readGHHTML(t)
	for name, c := range f.Closers {
		t.Run("<"+name+">", func(t *testing.T) { checkGitHubIDs(t, "<h2>a<"+name+">b</h2>", c.Open) })
		t.Run("</"+name+">", func(t *testing.T) { checkGitHubIDs(t, "<h2>a</"+name+">b</h2>", c.End) })
	}
	for o, byTag := range f.Nested {
		for x, ids := range byTag {
			t.Run("<"+o+"> then <"+x+">", func(t *testing.T) { checkGitHubIDs(t, "<h2><"+o+">a<"+x+">b</h2>", ids) })
		}
	}
}

// TestGitHubNestedIsRecorded requires ghThrough and ghNoPush to name only elements whose
// behaviour the vector nested records, so the engine reads no element by a rule it
// assumed.
func TestGitHubNestedIsRecorded(t *testing.T) {
	f := readGHHTML(t)
	var bad []string
	for o := range ghThrough {
		if f.Nested[o] == nil {
			bad = append(bad, o)
		}
	}
	for o := range ghNoPush {
		if f.Nested[o] == nil {
			bad = append(bad, o)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("names read without a vector: %v", bad)
	}
}

// TestGitHubHTMLEntities requires ghRawRef to decode exactly the named references that
// GitHub decodes in raw HTML, to the text GitHub renders, of every name of the entities
// package.
func TestGitHubHTMLEntities(t *testing.T) {
	want := readGHHTML(t).HTMLEntities
	src, err := os.ReadFile("../internal/htmlentity/testdata/entities-3.0.1.json")
	if err != nil {
		t.Fatal(err)
	}
	var names map[string]string
	if err := json.Unmarshal(src, &names); err != nil {
		t.Fatal(err)
	}
	for n := range names {
		ref := "&" + n + ";"
		got, k := ghRawRef(ref)
		w, decoded := want[n]
		switch {
		case decoded && (k != len(ref) || got != w):
			t.Fatalf("%s reads %q (%d bytes); GitHub renders %q", ref, got, k, w)
		case !decoded && k != 0:
			t.Fatalf("%s reads %q; GitHub renders it as text", ref, got)
		}
	}
}

// TestGitHubHTMLCodePoints requires S_GH of a raw HTML heading's text, read by
// htmlEvents, to reproduce GitHub's id of every code point in a raw <h2>.
func TestGitHubHTMLCodePoints(t *testing.T) {
	var bad []string
	n := 0
	for _, v := range readCodePoints(t, "testdata/slug/github-html-codepoints.txt") {
		for cp := v.lo; cp <= v.hi; cp++ {
			if cp >= 0xD800 && cp <= 0xDFFF {
				continue
			}
			for _, form := range []struct {
				spec, text string
			}{{v.ref, fmt.Sprintf("&#x%X;", cp)}, {v.raw, string(cp)}} {
				if form.spec == "none" {
					continue
				}
				var b strings.Builder
				for _, ev := range htmlEvents("-" + form.text + "-") {
					b.WriteString(ev.text)
				}
				want := "-" + cpID(t, cp, form.spec) + "-"
				if got := slugGH(b.String()); got != want {
					n++
					if len(bad) < 12 {
						bad = append(bad, fmt.Sprintf("%U %q: %+q, want %+q", cp, form.text, got, want))
					}
				}
			}
		}
	}
	if n > 0 {
		t.Fatalf("%d code point forms differ, first:\n%s", n, strings.Join(bad, "\n"))
	}
}

// TestRawHeadingsHaveNoAnchorInVSCodeOrGitLab requires the raw HTML heading cases to
// show that VS Code and GitLab give a raw HTML heading no anchor of its text: each raw
// heading element they render carries the id it is written with, or none.
func TestRawHeadingsHaveNoAnchorInVSCodeOrGitLab(t *testing.T) {
	for _, file := range []string{"testdata/slug/vscode-headings.json", "testdata/slug/gitlab-headings.json"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var f ghHTMLFile
		if err := json.Unmarshal(src, &f); err != nil {
			t.Fatal(err)
		}
		for _, c := range f.RawHeadings {
			t.Run(file+"/"+c.Name, func(t *testing.T) {
				reads := ghReadingsOf(c.Markdown)
				if len(reads) != len(c.IDs) {
					t.Fatalf("%d heading elements; the viewer rendered %d", len(reads), len(c.IDs))
				}
				for i, r := range reads {
					if r.head != nil {
						continue
					}
					var written *string
					if _, attrs, open := htmlMarkupAt(r.e.text[r.at:]); open {
						for _, a := range attrs {
							if a.name == "id" {
								v := a.value
								written = &v
							}
						}
					}
					if show(c.IDs[i]) != show(written) {
						t.Fatalf("raw element %d renders %s; written %s", i+1, show(c.IDs[i]), show(written))
					}
				}
			})
		}
	}
}
