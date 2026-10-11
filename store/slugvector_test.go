package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// The slug vectors of spec 10 (Detail headings and anchors): the heading ids that
// GitHub, VS Code and GitLab render, recorded from the viewers by
// scripts/record-slugs.mjs (make slug-vectors) into testdata/slug. S_GH, S_VS and S_GL
// must reproduce every one.

// cpVector is a line of a code point file: the code points lo to hi, the surrogates
// skipped, and the treatment of each in the reference form and in the raw form.
type cpVector struct {
	lo, hi   rune
	ref, raw string
}

// readCodePoints reads a code point file.
func readCodePoints(t *testing.T, file string) []cpVector {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []cpVector
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, " ; ")
		r := strings.Split(f[0], "..")
		if len(f) != 3 || len(r) != 2 {
			t.Fatalf("%s: malformed line %q", file, l)
		}
		lo, err1 := strconv.ParseUint(r[0], 16, 32)
		hi, err2 := strconv.ParseUint(r[1], 16, 32)
		if err1 != nil || err2 != nil {
			t.Fatalf("%s: malformed range %q", file, f[0])
		}
		out = append(out, cpVector{rune(lo), rune(hi), f[1], f[2]})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// cpID returns the id that the treatment spec gives the code point cp.
func cpID(t *testing.T, cp rune, spec string) string {
	switch {
	case spec == "drop":
		return ""
	case spec == "keep":
		return string(cp)
	case spec[0] == '+' || spec[0] == '-':
		d, err := strconv.ParseInt(spec[1:], 16, 32)
		if err != nil {
			t.Fatalf("malformed treatment %q", spec)
		}
		if spec[0] == '-' {
			d = -d
		}
		return string(cp + rune(d))
	}
	var b strings.Builder
	for _, h := range strings.Fields(spec) {
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			t.Fatalf("malformed treatment %q", spec)
		}
		b.WriteRune(rune(v))
	}
	return b.String()
}

// cpForm returns the heading content that holds cp in the form, as the recorder
// writes it: the numeric character reference, or the character, an ASCII punctuation
// character backslash-escaped.
func cpForm(cp rune, raw bool) string {
	if !raw {
		return fmt.Sprintf("&#x%X;", cp)
	}
	if cp < utf8.RuneSelf && isASCIIPunct(byte(cp)) {
		return `\` + string(cp)
	}
	return string(cp)
}

// viewerRefs are the definitions of a file in the reading of each viewer.
type viewerRefs struct {
	gh, vs, gl linkRefs
}

// viewerSlug is a viewer's slug of a heading's inline content s, as the collision rule
// reads it with the definitions refs: ghSlug, vsSlug or glSlug.
type viewerSlug func(s string, refs viewerRefs) string

// ghSlug is S_GH of a heading's content when nothing ends its element early: without
// the text of images, a line break read as nothing, a tag of the tagfilter as text.
func ghSlug(s string, refs viewerRefs) string {
	return slugGH(strings.Join(inlineText(s, refs.gh).text, ""))
}

// vsSlug is S_VS of a heading's content: with the alt text of images and VS Code's math,
// a line break read as nothing.
func vsSlug(s string, refs viewerRefs) string { return slugVS(strings.Join(vsText(s, refs.vs), "")) }

// glSlug is S_GL of a heading's content: with the alt text of images and GitLab's math,
// footnotes and wikilinks, a line break read as a space.
func glSlug(s string, refs viewerRefs) string { return slugGL(strings.Join(glText(s, refs.gl), " ")) }

// checkCodePoints requires slug to give, for every code point of the file in each of
// its forms, the heading -c- the id -t-, t the code point's recorded id.
func checkCodePoints(t *testing.T, file string, slug viewerSlug) {
	var bad []string
	n := 0
	refs := testRefs()
	for _, v := range readCodePoints(t, file) {
		for cp := v.lo; cp <= v.hi; cp++ {
			if cp >= 0xD800 && cp <= 0xDFFF {
				continue
			}
			for _, form := range []struct {
				spec string
				raw  bool
			}{{v.ref, false}, {v.raw, true}} {
				if form.spec == "none" {
					continue
				}
				_, content := atxContent("-" + cpForm(cp, form.raw) + "-")
				want := "-" + cpID(t, cp, form.spec) + "-"
				if got := slug(content, refs); got != want {
					n++
					if len(bad) < 12 {
						bad = append(bad, fmt.Sprintf("%U raw=%v: %+q, want %+q", cp, form.raw, got, want))
					}
				}
			}
		}
	}
	if n > 0 {
		t.Fatalf("%d code point forms differ from %s, first:\n%s", n, file, strings.Join(bad, "\n"))
	}
}

// TestGitHubCodePoints requires S_GH to reproduce GitHub's id of every code point.
func TestGitHubCodePoints(t *testing.T) {
	checkCodePoints(t, "testdata/slug/github-codepoints.txt", ghSlug)
}

// TestVSCodeCodePoints requires S_VS to reproduce VS Code's id of every code point.
func TestVSCodeCodePoints(t *testing.T) {
	checkCodePoints(t, "testdata/slug/vscode-codepoints.txt", vsSlug)
}

// TestGitLabCodePoints requires S_GL to reproduce GitLab's id of every code point.
func TestGitLabCodePoints(t *testing.T) {
	checkCodePoints(t, "testdata/slug/gitlab-codepoints.txt", glSlug)
}

// headingCase is a structural case: a document and the id of each of its headings in the
// order the viewer renders them, null where the viewer renders no anchor.
type headingCase struct {
	Name     string    `json:"name"`
	Markdown string    `json:"markdown"`
	IDs      []*string `json:"ids"`
}

// headingFile is a heading file: the structural cases; for GitHub, the id and name
// attributes it renders for raw HTML; for GitLab, the cases of a snippet, rendered with
// the heading slug prefix GitLab gives a snippet's file.
type headingFile struct {
	Viewer   string        `json:"viewer"`
	Recorded string        `json:"recorded"`
	Cases    []headingCase `json:"cases"`
	Snippet  struct {
		File   string        `json:"file"`
		Prefix string        `json:"prefix"`
		Cases  []headingCase `json:"cases"`
	} `json:"snippet"`
	Attributes []struct {
		Name     string `json:"name"`
		Markdown string `json:"markdown"`
		Values   []struct {
			Attr  string `json:"attr"`
			Value string `json:"value"`
		} `json:"values"`
	} `json:"attributes"`
}

func readHeadingFile(t *testing.T, file string) headingFile {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var h headingFile
	if err := json.Unmarshal(src, &h); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return h
}

// viewer names the viewer whose reading a test reads: its heading file's viewer field.
type viewer string

const (
	gitHub viewer = "GitHub"
	vsCode viewer = "VS Code"
	gitLab viewer = "GitLab"
)

// viewerSlugs returns the slug of every heading that the viewer anchors in a document,
// as the collision rule reads it, in the order the viewer renders them: the viewer's
// reading of the blocks (readViewers), without the headings of a footnote definition it
// drops, and those of one it renders at the end after the others, by the definitions'
// numbers.
func viewerSlugs(lines []string, v viewer) []string {
	r := readViewers(lines, scanHeadings(lines, 0))
	rd := map[viewer]viewerReading{gitHub: r.gh, vsCode: r.vs, gitLab: r.gl}[v]
	setext := setextStarts(rd.sc.blocks)
	type placed struct {
		moved, at int
		slug      string
	}
	var out []placed
	for _, h := range rd.sc.all {
		moved, shown := 0, true
		if h.note != nil && rd.refs.notes != nil {
			moved, shown = rd.refs.notes.home(h.note)
		}
		if !shown {
			continue
		}
		text := headingElement(lines, h, setext).text
		refs := viewerRefs{gh: rd.refs, vs: rd.refs, gl: rd.refs}
		slug := map[viewer]viewerSlug{gitHub: ghSlug, vsCode: vsSlug, gitLab: glSlug}[v]
		out = append(out, placed{moved, h.at, slug(text, refs)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].moved > 0) != (out[j].moved > 0) {
			return out[j].moved > 0
		}
		return out[i].moved < out[j].moved
	})
	slugs := make([]string, len(out))
	for i, p := range out {
		slugs[i] = p.slug
	}
	return slugs
}

// checkIDs requires the slugs to reproduce the recorded ids, each without prefix: a
// heading that has no anchor has the empty slug; an id is the slug or the slug numbered
// by the numbering rule (takesAnchor); and the id of a heading whose slug no earlier
// heading had as its slug or its id is the slug.
func checkIDs(t *testing.T, v viewer, slugs []string, ids []*string, prefix string) {
	t.Helper()
	if len(slugs) != len(ids) {
		t.Fatalf("%d headings; %s recorded %d", len(slugs), v, len(ids))
	}
	seen := map[string]bool{}
	for i, s := range slugs {
		id := ids[i]
		if id != nil {
			rest, ok := strings.CutPrefix(*id, prefix)
			if !ok {
				t.Fatalf("heading %d: %s renders %q, without the prefix %q", i+1, v, *id, prefix)
			}
			id = &rest
		}
		switch {
		case id == nil:
			if s != "" {
				t.Fatalf("heading %d: slug %q; %s renders no anchor", i+1, s, v)
			}
		case !seen[s] && *id != s:
			t.Fatalf("heading %d: slug %q; %s renders %q", i+1, s, v, *id)
		case !takesAnchor(s, *id):
			t.Fatalf("heading %d: slug %q does not take %s's id %q", i+1, s, v, *id)
		}
		seen[s] = true
		if id != nil {
			seen[*id] = true
		}
	}
}

// checkHeadings requires the viewer's slug to reproduce the recorded id of every heading
// of the structural cases (checkIDs).
func checkHeadings(t *testing.T, file string) {
	h := readHeadingFile(t, file)
	for _, c := range h.Cases {
		t.Run(c.Name, func(t *testing.T) {
			checkIDs(t, viewer(h.Viewer), viewerSlugs(strings.Split(c.Markdown, "\n"), viewer(h.Viewer)), c.IDs, "")
		})
	}
}

// TestGitHubHeadings requires S_GH to reproduce GitHub's structural cases.
func TestGitHubHeadings(t *testing.T) {
	checkHeadings(t, "testdata/slug/github-headings.json")
}

// TestVSCodeHeadings requires S_VS to reproduce VS Code's structural cases.
func TestVSCodeHeadings(t *testing.T) {
	checkHeadings(t, "testdata/slug/vscode-headings.json")
}

// TestGitLabHeadings requires S_GL to reproduce GitLab's structural cases.
func TestGitLabHeadings(t *testing.T) {
	checkHeadings(t, "testdata/slug/gitlab-headings.json")
}

// TestGitLabSnippetPrefix requires GitLab's heading slug prefix of a snippet to stand
// before the id of every heading alike, numbered by the slug without it, so that headings
// take one another's ids in a snippet exactly as in a repository file: the collision rule
// reads a corpus file, which GitLab renders without the prefix (spec 10).
func TestGitLabSnippetPrefix(t *testing.T) {
	h := readHeadingFile(t, "testdata/slug/gitlab-headings.json")
	if h.Snippet.Prefix == "" || len(h.Snippet.Cases) == 0 {
		t.Fatal("no snippet cases recorded")
	}
	for _, c := range h.Snippet.Cases {
		t.Run(c.Name, func(t *testing.T) {
			checkIDs(t, gitLab, viewerSlugs(strings.Split(c.Markdown, "\n"), gitLab), c.IDs, h.Snippet.Prefix)
		})
	}
}

// TestGitHubAttributes requires the collision rule to refuse every raw HTML id or name
// that GitHub renders as the id of the detail heading items row T1: GitHub writes
// user-content- before the value of each, unless the value already begins with it, as
// it writes it before the id of a heading.
func TestGitHubAttributes(t *testing.T) {
	h := readHeadingFile(t, "testdata/slug/github-headings.json")
	const prefix = "user-content-"
	for _, c := range h.Attributes {
		t.Run(c.Name, func(t *testing.T) {
			if len(c.Values) != 1 {
				t.Fatalf("GitHub rendered %d attributes", len(c.Values))
			}
			v := c.Values[0]
			tags := attributeValues(strings.Split(c.Markdown, "\n"))
			if len(tags) != 1 || tags[0].name != v.Attr {
				t.Fatalf("the engine reads %v; GitHub renders %s", tags, v.Attr)
			}
			rendered := tags[0].value
			if !strings.HasPrefix(rendered, prefix) {
				rendered = prefix + rendered
			}
			if rendered != v.Value {
				t.Fatalf("%s=%q renders %q; GitHub renders %q", v.Attr, tags[0].value, rendered, v.Value)
			}
			lines := append([]string{"## items row T1", ""}, strings.Split(c.Markdown, "\n")...)
			clashes := anchorClashes(lines, anchorTables)
			if v.Value == prefix+"items-row-t1" && len(clashes) == 0 {
				t.Fatalf("GitHub renders %q, the id of the detail heading, and the rule refuses nothing", v.Value)
			}
		})
	}
}

// attributeValues returns the id and name attributes of the raw HTML of a document, as
// the collision rule reads them.
func attributeValues(lines []string) []htmlAttr {
	sc := scanHeadings(lines, 0)
	g := linkDefs(sc.blocks)
	setext := setextStarts(sc.blocks)
	var tags [][]htmlAttr
	for _, h := range sc.all {
		tags = append(tags, inlineText(headingElement(lines, h, setext).text, g).tags...)
	}
	for _, u := range sc.units {
		if u.html {
			ts, _ := rawTags(htmlElement(lines, u).text)
			tags = append(tags, ts...)
			continue
		}
		_, cm := cmDefLines(u.lines)
		tags = append(tags, inlineText(linesElement(lines, u, cm).text, g).tags...)
	}
	var out []htmlAttr
	for _, attrs := range tags {
		for _, a := range attrs {
			if a.name == "id" || a.name == "name" {
				out = append(out, a)
			}
		}
	}
	return out
}
