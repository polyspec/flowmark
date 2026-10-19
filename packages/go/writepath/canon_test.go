package writepath

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
)

// crlf writes text with CRLF line endings, as a Windows autocrlf checkout does.
func crlf(text string) string { return strings.ReplaceAll(text, "\n", "\r\n") }

// noncanonicalDocs is the base corpus with three noncanonical documents, one of each target kind: a document list in
// CRLF (items), a files row whose link target is not the derived one (notes/N2), and a document table with a trailing
// blank line (tags). Their canonical forms are the base documents.
func noncanonicalDocs() map[string]string {
	docs := baseDocs()
	docs["items.flowmark.md"] = crlf(docs["items.flowmark.md"])
	docs["notes/N2.flowmark.md"] = strings.Replace(docs["notes/N2.flowmark.md"], "[N1](N1.flowmark.md)", "[N1](./N1.flowmark.md)", 1)
	docs["tags.flowmark.md"] += "\n"
	return docs
}

// with returns the documents docs with the files of edits replaced.
func with(docs map[string]string, edits map[string]string) map[string]string {
	for p, text := range edits {
		docs[p] = text
	}
	return docs
}

// canonOf runs flowmark canon in a new session and fails the test on an error.
func (f *fixture) canonOf(files ...string) *CanonResult {
	f.t.Helper()
	r, err := f.session("canon").Canon(context.Background(), files)
	if err != nil {
		f.t.Fatalf("Canon(%v) = %v", files, err)
	}
	return r
}

// wantReports requires the reports of r to be want, each "<path> <rule> <file of the diagnostic> <needed table>".
func wantReports(t testing.TB, r *CanonResult, want ...string) {
	t.Helper()
	got := []string{}
	for _, rp := range r.Reported {
		got = append(got, fmt.Sprintf("%s %s %s %s", rp.Path, rp.Diagnostic.Rule, rp.Diagnostic.File, rp.Needs))
	}
	if want == nil {
		want = []string{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reports = %q, want %q", got, want)
	}
}

// wantRewritten requires the files r rewrote to be want.
func wantRewritten(t testing.TB, r *CanonResult, want ...string) {
	t.Helper()
	got := append([]string{}, r.Rewritten...)
	if want == nil {
		want = []string{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rewritten = %q, want %q", got, want)
	}
}

// TestCanon pins flowmark canon (spec 40 The canon operation): one transaction under the lock that rewrites its targets
// to canonical form, exempt from the commit refusal for them — the named files, or with no file named every corpus
// document whose only format diagnostics are format.noncanonical — refusing a named file it cannot read or whose links
// need an unavailable block or document table, and reporting such a file when none is named.
func TestCanon(t *testing.T) {
	ctx := context.Background()
	base := baseDocs()
	t.Run("40#the-canon-operation: with no file named, every noncanonical document of each kind is rewritten", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), noncanonicalDocs())
		r := f.canonOf()
		wantRewritten(t, r, "items.flowmark.md", "notes/N2.flowmark.md", "tags.flowmark.md")
		wantReports(t, r)
		f.rec.wantKinds(t, committed...)
		if got, want := snapshot(t, f.root), snapshot(t, baseFixture(t).root); !reflect.DeepEqual(got, want) {
			t.Fatalf("corpus = %v, want the canonical corpus %v", got, want)
		}
		f.clean()
		// Canon is idempotent: a canonical corpus gives no target, and nothing is written.
		wantRewritten(t, f.canonOf())
		f.rec.wantKinds(t, opened...)
	})
	t.Run("40#the-canon-operation: the named files are the only targets, a file named twice one target", func(t *testing.T) {
		docs := noncanonicalDocs()
		f := newFixture(t, executorCatalog(t), docs)
		wantRewritten(t, f.canonOf("tags.flowmark.md", "notes/N2.flowmark.md", "tags.flowmark.md"), "notes/N2.flowmark.md", "tags.flowmark.md")
		f.rec.wantKinds(t, committed...)
		if f.file("tags.flowmark.md") != base["tags.flowmark.md"] || f.file("notes/N2.flowmark.md") != base["notes/N2.flowmark.md"] {
			t.Fatal("a named file was not rewritten")
		}
		if f.file("items.flowmark.md") != docs["items.flowmark.md"] {
			t.Fatal("a file not named was rewritten")
		}
	})
	t.Run("40#the-canon-operation: a named canonical file is no change", func(t *testing.T) {
		f := baseFixture(t)
		wantRewritten(t, f.canonOf("people.flowmark.md"))
		f.rec.wantKinds(t, opened...)
	})
	t.Run("40#the-canon-operation: a value with a validator diagnostic keeps its written text", func(t *testing.T) {
		typed := strings.Replace(base["people.flowmark.md"], "| P2 | Max |  |  |", "| P2 | Max |  | old |", 1)
		f := newFixture(t, executorCatalog(t), with(baseDocs(), map[string]string{"people.flowmark.md": crlf(typed)}))
		wantRewritten(t, f.canonOf(), "people.flowmark.md")
		if got := f.file("people.flowmark.md"); got != typed {
			t.Fatalf("people:\n%s\nwant:\n%s", got, typed)
		}
	})
	t.Run("40#the-canon-operation: byte-equal keys in two blocks of a file are the validator's, never a refusal", func(t *testing.T) {
		// A write refuses such keys, since no request can name one of the rows; the read does not, and canon renders
		// as the read does. The keys make item_history unavailable, which the links of items do not need.
		shared := strings.Replace(base["items.flowmark.md"], "| 3 | waiting | doing | 2026-10-02 08:00:00.000000 |", "| 1 | waiting | doing | 2026-10-02 08:00:00.000000 |", 1)
		f := newFixture(t, executorCatalog(t), with(baseDocs(), map[string]string{"items.flowmark.md": crlf(shared)}))
		wantRewritten(t, f.canonOf(), "items.flowmark.md")
		if got := f.file("items.flowmark.md"); got != shared {
			t.Fatalf("items:\n%s\nwant:\n%s", got, shared)
		}
	})
	t.Run("40#the-canon-operation: a file of an unavailable table is rewritten, its link to a files row needing no table", func(t *testing.T) {
		docs := with(noncanonicalDocs(), map[string]string{"notes/N3.flowmark.md": "# Second root\n- position: \xff2\n"})
		f := newFixture(t, executorCatalog(t), docs)
		wantRewritten(t, f.canonOf("notes/N2.flowmark.md"), "notes/N2.flowmark.md")
		if f.file("notes/N2.flowmark.md") != base["notes/N2.flowmark.md"] {
			t.Fatalf("notes/N2:\n%s", f.file("notes/N2.flowmark.md"))
		}
		f2 := newFixture(t, executorCatalog(t), docs)
		r := f2.canonOf()
		wantRewritten(t, r, "items.flowmark.md", "notes/N2.flowmark.md", "tags.flowmark.md")
		wantReports(t, r, "notes/N3.flowmark.md format.encoding notes/N3.flowmark.md ")
	})
	t.Run("40#the-canon-operation: inside a transaction canon is CONFIG", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), noncanonicalDocs())
		s := f.session("server")
		must(t, s.Begin(ctx, false))
		_, err := s.Canon(ctx, nil)
		fails(t, err, "CONFIG", "its own transaction")
		must(t, s.Rollback())
	})
}

// milestonesDBS is a catalog of a document table whose rows have parents within their file: a cycle of them is the
// file's parent.cycle.
const milestonesDBS = `dbspec 1 milestones

table milestones {
  id varchar(16)
  parent varchar(16) null
  title varchar(255)
  primary key (id)
  index ix_milestones_parent (parent)
  foreign key fk_milestones_parent (parent) references milestones (id) on delete restrict on update restrict
  settings {
    store document table
    title title
  }
}

table tags {
  id i64 identity
  label varchar(16)
  primary key (id)
  settings {
    store document table
  }
}
`

// TestCanonUnreadable pins a file canon cannot read (spec 40 The canon operation): a named file with format.block,
// format.section, format.encoding or a parent.cycle among its rows refuses the command as CONSTRAINT, naming the file
// and the diagnostic, and nothing is written; with no file named, it is reported and the other targets are rewritten.
func TestCanonUnreadable(t *testing.T) {
	tags := lines("# tags", "| id | label |", "| --- | --- |", "| 1 | go |")
	cases := []struct {
		name, dbs, path, text, rule string
		// also are the other reports of a canon with no file named: the files whose links need the table made
		// unavailable.
		also []string
	}{
		{"40#the-canon-operation: a files row with format.block", executorCatalog(t), "notes/N3.flowmark.md",
			lines("# Second root", "- position: 2", "Body right after the fields."), "format.block", nil},
		{"40#the-canon-operation: a files row with format.section", executorCatalog(t), "notes/N3.flowmark.md",
			lines("# Second root", "- position: 2", "", "## nope", "", "x"), "format.section", nil},
		{"40#the-canon-operation: a files row with format.encoding", executorCatalog(t), "notes/N3.flowmark.md",
			"# Second root\n- position: \xff2\n", "format.encoding", nil},
		{"40#the-canon-operation: a document list with format.encoding", executorCatalog(t), "items.flowmark.md",
			strings.Replace(baseDocs()["items.flowmark.md"], "Parser body.", "Parser \x00body.", 1), "format.encoding",
			[]string{"notes/N1.flowmark.md format.encoding items.flowmark.md items"}},
		{"40#the-canon-operation: a document table with a parent.cycle among its rows", milestonesDBS, "milestones.flowmark.md",
			lines("# milestones", "| id | parent | title |", "| --- | --- | --- |",
				"| M1 | [M2](milestones.flowmark.md) | A |", "| M2 | [M1](milestones.flowmark.md) | B |"), "parent.cycle", nil},
	}
	for _, c := range cases {
		docs := map[string]string{c.path: c.text, "tags.flowmark.md": tags + "\n"}
		if c.dbs != milestonesDBS {
			docs = with(baseDocs(), docs)
			docs["tags.flowmark.md"] = baseDocs()["tags.flowmark.md"] + "\n"
		}
		t.Run(c.name+", named, refuses", func(t *testing.T) {
			f := newFixture(t, c.dbs, docs)
			before := snapshot(t, f.root)
			_, err := f.session("canon").Canon(context.Background(), []string{"tags.flowmark.md", c.path})
			fails(t, err, "CONSTRAINT", c.path, c.rule, "nothing was written")
			var ee *irexec.Error
			if !errors.As(err, &ee) || ee.Diagnostic == nil || ee.Diagnostic.Rule != c.rule || ee.Diagnostic.File != c.path {
				t.Fatalf("the refusal %v does not carry the %s of %s", err, c.rule, c.path)
			}
			f.rec.wantKinds(t, opened...)
			if got := snapshot(t, f.root); !reflect.DeepEqual(got, before) {
				t.Fatalf("corpus = %v, want it untouched", got)
			}
		})
		t.Run(c.name+", not named, is reported", func(t *testing.T) {
			f := newFixture(t, c.dbs, docs)
			r := f.canonOf()
			wantRewritten(t, r, "tags.flowmark.md")
			wantReports(t, r, append([]string{c.path + " " + c.rule + " " + c.path + " "}, c.also...)...)
			if f.file(c.path) != c.text {
				t.Fatalf("%s was rewritten", c.path)
			}
		})
	}
}

// TestCanonLinkDerivation pins the link-derivation refusal of canon (spec 40 The canon operation): a target holding a
// link to a row of an unavailable block or document table refuses a named-file canon as CONSTRAINT, naming the target
// and the file that makes the table unavailable, and with no file named is reported with both files; a link to a
// files row needs no table.
func TestCanonLinkDerivation(t *testing.T) {
	// items is unavailable through its format.encoding; notes/N1 links T1 of items, and is noncanonical.
	docs := with(noncanonicalDocs(), map[string]string{
		"items.flowmark.md":    strings.Replace(baseDocs()["items.flowmark.md"], "Parser body.", "Parser \x00body.", 1),
		"notes/N1.flowmark.md": baseDocs()["notes/N1.flowmark.md"] + "\n",
	})
	t.Run("40#the-canon-operation: a named target whose link needs an unavailable document table refuses", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		before := snapshot(t, f.root)
		_, err := f.session("canon").Canon(context.Background(), []string{"notes/N1.flowmark.md", "tags.flowmark.md"})
		fails(t, err, "CONSTRAINT", "notes/N1.flowmark.md", "items.flowmark.md", "format.encoding", "nothing was written")
		f.rec.wantKinds(t, opened...)
		if got := snapshot(t, f.root); !reflect.DeepEqual(got, before) {
			t.Fatalf("corpus = %v, want it untouched", got)
		}
	})
	t.Run("40#the-canon-operation: with no file named, such a target is reported with both files", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		r := f.canonOf()
		wantRewritten(t, r, "notes/N2.flowmark.md", "tags.flowmark.md")
		wantReports(t, r,
			"items.flowmark.md format.encoding items.flowmark.md ",
			"notes/N1.flowmark.md format.encoding items.flowmark.md items")
		if f.file("notes/N1.flowmark.md") != docs["notes/N1.flowmark.md"] {
			t.Fatal("notes/N1 was rewritten")
		}
	})
	t.Run("40#the-canon-operation: a link to an unavailable files table needs no table", func(t *testing.T) {
		// notes is unavailable through notes/N3; notes/N2's parent links the files row N1.
		f := newFixture(t, executorCatalog(t), with(noncanonicalDocs(), map[string]string{
			"notes/N3.flowmark.md": lines("# Second root", "- position: 2", "Body right after the fields."),
		}))
		wantRewritten(t, f.canonOf("notes/N2.flowmark.md"), "notes/N2.flowmark.md")
	})
}

// TestCanonPaths pins how canon reads the files named (spec 40 The canon operation): a named corpus.walk entry refuses
// as CONSTRAINT naming its diagnostic, a path that names no corpus document as CONFIG, and with no file named a
// corpus.walk entry is reported and never a target.
func TestCanonPaths(t *testing.T) {
	docs := with(noncanonicalDocs(), map[string]string{"notes/sub/N9.flowmark.md": lines("# Nested", "- position: 1")})
	t.Run("40#the-canon-operation: a named corpus.walk entry refuses as CONSTRAINT", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		_, err := f.session("canon").Canon(context.Background(), []string{"notes/sub/N9.flowmark.md"})
		fails(t, err, "CONSTRAINT", "notes/sub/N9.flowmark.md", "corpus.walk")
		f.rec.wantKinds(t, opened...)
	})
	for _, p := range []string{"nope.flowmark.md", "notes/N9.flowmark.md", "./tags.flowmark.md", "dbspec/test.dbs", ""} {
		t.Run(fmt.Sprintf("40#the-canon-operation: the named path %q is CONFIG", p), func(t *testing.T) {
			f := newFixture(t, executorCatalog(t), docs)
			_, err := f.session("canon").Canon(context.Background(), []string{"tags.flowmark.md", p})
			fails(t, err, "CONFIG", fmt.Sprintf("%q", p), "no corpus document")
			f.rec.wantKinds(t, opened...)
		})
	}
	t.Run("40#the-canon-operation: with no file named, a corpus.walk entry is reported", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		r := f.canonOf()
		wantRewritten(t, r, "items.flowmark.md", "notes/N2.flowmark.md", "tags.flowmark.md")
		wantReports(t, r, "notes/sub/N9.flowmark.md corpus.walk notes/sub/N9.flowmark.md ")
	})
}

// TestCanonCrash pins an interrupted canon (spec 40 The commit protocol): stopped before its prepare record it writes
// nothing; from the prepare record on it is decided, and recovery under the same lock completes it. A canon whose
// recovery fails too is replayed at the next acquisition of the lock.
func TestCanonCrash(t *testing.T) {
	canonical := snapshot(t, baseFixture(t).root)
	for _, cp := range crashPoints {
		t.Run(fmt.Sprintf("40#the-commit-protocol: canon stopped at %s#%d", cp.step, cp.index), func(t *testing.T) {
			f := newFixture(t, executorCatalog(t), noncanonicalDocs())
			before := snapshot(t, f.root)
			f.j.SetFault(crashOnce(cp.step, cp.index))
			_, err := f.session("canon").Canon(context.Background(), nil)
			f.j.SetFault(nil)
			want := before
			if cp.new {
				if err != nil {
					t.Fatalf("Canon = %v, want the canon completed under its lock", err)
				}
				want = canonical
			} else {
				fails(t, err, "DRIVER", "simulated crash", "nothing was written")
			}
			if kinds := f.rec.kinds(); kinds[len(kinds)-2] != cp.event {
				t.Fatalf("events = %v, want %s before the release", kinds, cp.event)
			}
			if got := snapshot(t, f.root); !reflect.DeepEqual(got, want) {
				t.Fatalf("corpus = %v, want %v", got, want)
			}
		})
	}
	t.Run("40#opening-a-corpus: a decided canon whose recovery fails is replayed at the next acquisition", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), noncanonicalDocs())
		f.j.SetFault(crashAt("renames", 0))
		_, err := f.session("canon").Canon(context.Background(), nil)
		fails(t, err, "DRIVER", "prepared", "do not repeat")
		if !errors.Is(err, errCrash) {
			t.Fatalf("Canon = %v, want the crash as its cause", err)
		}
		f.j.SetFault(nil)
		f.rec.reset()
		// The next canon recovers the stage under the lock before it reads: the corpus is canonical, so it writes nothing.
		r := f.canonOf()
		wantRewritten(t, r)
		f.rec.wantKinds(t, journal.LockAcquired, journal.Applied, journal.Collected, journal.Replayed, journal.LockReleased)
		if got := snapshot(t, f.root); !reflect.DeepEqual(got, canonical) {
			t.Fatalf("corpus = %v, want the canonical corpus", got)
		}
		f.clean()
	})
}
