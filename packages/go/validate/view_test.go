package validate

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// viewRow is one row of the views document: its key, title, kind, entity,
// filter, sort and group_by cells, the group "" being NULL.
type viewRow struct {
	key, title, kind, entity, filter, sort, group string
}

// noFilter is the stored filter of a view without one.
const noFilter = `{"where":{"items":[]},"params":[]}`

// views returns the views document holding rows; its first row is on line 4.
func views(rows ...viewRow) string {
	var b strings.Builder
	b.WriteString("# views\n| id | title | kind | entity | filter | sort | group_by |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rows {
		b.WriteString("| " + strings.Join([]string{r.key, r.title, r.kind, r.entity, r.filter, r.sort, r.group}, " | ") + " |\n")
	}
	return b.String()
}

// view returns a list view of memos with the filter, sort and group.
func view(filter, sort, group string) viewRow {
	return viewRow{"1", "Memos", "list", "memos", filter, sort, group}
}

// noteRow is one row of the memos document: its key and its doc, packed,
// wrapped and hexed cells, "" being NULL.
type noteRow struct {
	key, doc, packed, wrapped, hexed string
}

// memos returns the memos document holding rows; its first row is on line 4.
func memos(rows ...noteRow) string {
	var b strings.Builder
	b.WriteString("# memos\n| id | title | rank | doc | packed | wrapped | hexed |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rows {
		b.WriteString("| " + strings.Join([]string{r.key, "Note " + r.key, "", r.doc, r.packed, r.wrapped, r.hexed}, " | ") + " |\n")
	}
	return b.String()
}

// gzipped returns the padded base64 text of the zlib stream of text, the
// readable form of a bytes value that the stages ordered_json gz encode.
func gzipped(t *testing.T, text string) string {
	t.Helper()
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	if _, err := w.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

// FuzzViewDecoder feeds any text to the stored filter and sort decoders: each
// returns a fault or accepts, and never panics; an accepted filter uses every
// param it holds.
func FuzzViewDecoder(f *testing.F) {
	for _, s := range []string{noFilter, `[]`, `[{"column":"title","desc":true}]`,
		`{"where":{"items":[{"pred":{"column":"rank","op":"gt","p":0}}]},"params":[1]}`,
		`{"where":{"items":[{"pred":{"column":"title","op":"in","sub":{"query":{"entity":"memos","where":{"items":[{"pred":{"column":"rank","op":"eq","p":0}}]}},"column":"title"}}}]},"params":[2]}`,
		`{"where":{"items":[]},"where":{},"params":[]}`, `{`, `null`} {
		f.Add(s)
	}
	cat, diags := catalog.Load(catalogRoot)
	if cat == nil || len(diags) > 0 {
		f.Fatalf("test catalog: %+v", diags)
	}
	c := &corpus{cat: cat}
	m, err := c.runtimeModel()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, text string) {
		_ = judgeSort(m, "memos", text)
		if judgeFilter(m, "memos", text) == "" {
			var sf storedFilter
			if err := strictJSON(text, &sf); err != nil {
				t.Fatalf("an accepted filter does not decode: %v", err)
			}
			used := map[int]bool{}
			usesOfGroup(sf.Where, used)
			if len(used) != len(sf.Params) {
				t.Fatalf("an accepted filter uses %d of its %d params", len(used), len(sf.Params))
			}
		}
	})
}

// TestViewsAndCodecs holds the accepting and rejecting cases of the view and
// codec rules (spec 50 Codec values, Views).
func TestViewsAndCodecs(t *testing.T) {
	cat := loadCatalog(t)
	const where = `{"where":{"items":[{"pred":{"column":"rank","op":"gt","p":0}}]},"params":[1]}`
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		// view: accepted.
		{name: "50#views: a filter with a param, a sort and a group are accepted",
			files: map[string]string{"views.flowmark.md": views(view(where, `[{"column":"title","desc":true}]`, "title"))}},
		{name: "50#views: no filter, no sort and no group are accepted",
			files: map[string]string{"views.flowmark.md": views(view(noFilter, `[]`, ""))}},
		{name: "50#views: a null param and a nested group are accepted",
			files: map[string]string{"views.flowmark.md": views(view(
				`{"where":{"items":[{"pred":{"column":"rank","op":"is_null"}},{"group":{"conn":"or","items":[{"pred":{"column":"title","op":"in","ps":[0,1]}}]}}]},"params":[null,"a"]}`, `[]`, ""))}},

		// view: the entity.
		{name: "50#views: an entity the catalog does not declare",
			files: map[string]string{"views.flowmark.md": views(viewRow{"1", "Ghosts", "list", "ghosts", noFilter, `[]`, ""})},
			want:  []string{"views.flowmark.md:4:23 view ~ ghosts"}},
		{name: "50#views: a list view of an entity without a body",
			files: map[string]string{"views.flowmark.md": views(viewRow{"1", "Tickets", "list", "tickets", noFilter, `[]`, ""})},
			want:  []string{"views.flowmark.md:4 view ~ body"}},
		{name: "50#views: a kind outside the check is check alone and needs no body",
			files: map[string]string{"views.flowmark.md": views(viewRow{"1", "Tickets", "board", "tickets", noFilter, `[]`, ""})},
			want:  []string{"views.flowmark.md:4 check ~ ck_views_kind"}},
		{name: "50#views: a kind not of its type is type alone and the entity is judged only as named",
			files: map[string]string{"views.flowmark.md": views(viewRow{"1", "Tickets", "listlistlistlistlist", "tickets", noFilter, `[]`, ""})},
			want:  []string{"views.flowmark.md:4 type ~ kind"}},
		{name: "50#views: a faulty entity leaves the filter, sort and group unjudged",
			files: map[string]string{"views.flowmark.md": views(viewRow{"1", "Ghosts", "list", "ghosts", `{}`, `{}`, "nothing"})},
			want:  []string{"views.flowmark.md:4 view ~ ghosts"}},

		// view: the filter.
		{name: "50#views: a column the entity does not have",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[{"pred":{"column":"color","op":"eq","p":0}}]},"params":[1]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ COLUMN_UNKNOWN"}},
		{name: "50#views: contains on an integer column",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[{"pred":{"column":"rank","op":"contains","p":0}}]},"params":["a"]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ OPERATOR_NOT_ALLOWED"}},
		{name: "50#views: an index past the params",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[{"pred":{"column":"rank","op":"gt","p":1}}]},"params":[1]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ IR_INVALID"}},
		{name: "50#views: a param no index uses",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[{"pred":{"column":"rank","op":"gt","p":0}}]},"params":[1,2]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ param 1"}},
		{name: "50#views: a param that is an object",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[{"pred":{"column":"rank","op":"gt","p":0}}]},"params":[{"a":1}]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ param 0"}},
		{name: "50#views: a member the filter does not have",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[]},"params":[],"limit":1}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ limit"}},
		{name: "50#views: a missing member",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[]}}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ params"}},
		{name: "50#views: a where without items",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{},"params":[]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ items"}},
		{name: "50#views: a repeated member",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[]},"where":{"items":[]},"params":[]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ repeats"}},
		{name: "50#views: a repeated member deep in the where",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[{"pred":{"column":"rank","column":"title","op":"is_null"}}]},"params":[]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ repeats"}},
		{name: "50#views: an index that is not an integer",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[{"pred":{"column":"rank","op":"gt","p":1.5}}]},"params":[1]}`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 view"}},

		// view: the sort and the group.
		{name: "50#views: a sort that is not an array",
			files: map[string]string{"views.flowmark.md": views(view(noFilter, `{}`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ sort"}},
		{name: "50#views: a sort by a column the entity does not have",
			files: map[string]string{"views.flowmark.md": views(view(noFilter, `[{"column":"color"}]`, ""))},
			want:  []string{"views.flowmark.md:4 view ~ COLUMN_UNKNOWN"}},
		{name: "50#views: a group by a field the entity does not have",
			files: map[string]string{"views.flowmark.md": views(view(noFilter, `[]`, "color"))},
			want:  []string{"views.flowmark.md:4 view ~ color"}},
		{name: "50#views: a group by a codec field, which has no equality",
			files: map[string]string{"views.flowmark.md": views(view(noFilter, `[]`, "doc"))},
			want:  []string{"views.flowmark.md:4 view ~ doc"}},
		{name: "50#views: a faulty filter and a faulty sort are two reports",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":{"items":[{"pred":{"column":"color","op":"is_null"}}]},"params":[]}`, `[{"column":"color"}]`, ""))},
			want: []string{
				"views.flowmark.md:4:30 view ~ filter",
				"views.flowmark.md:4 view ~ sort",
			}},

		// codec.
		{name: "50#codec-values: values its stages decode are accepted",
			files: map[string]string{"memos.flowmark.md": memos(noteRow{"N1", `{"a":[1,2]}`, gzipped(t, `{"a":1}`), base64.StdEncoding.EncodeToString([]byte(`[true]`)), ""})}},
		{name: "50#codec-values: text its ordered_json stage cannot decode",
			files: map[string]string{"memos.flowmark.md": memos(noteRow{"N1", `{"a":`, "", "", ""})},
			want:  []string{"memos.flowmark.md:4 codec ~ doc"}},
		{name: "50#codec-values: bytes its gz stage cannot decode",
			files: map[string]string{"memos.flowmark.md": memos(noteRow{"N1", "", base64.StdEncoding.EncodeToString([]byte("plain")), "", ""})},
			want:  []string{"memos.flowmark.md:4 codec ~ gz"}},
		{name: "50#codec-values: text its base64 stage cannot decode",
			files: map[string]string{"memos.flowmark.md": memos(noteRow{"N1", "", "", "not base64!", ""})},
			want:  []string{"memos.flowmark.md:4 codec ~ base64"}},
		{name: "50#codec-values: a host stage is not judged until orm exports its decode",
			files: map[string]string{"memos.flowmark.md": memos(noteRow{"N1", "", "", "", "not hex"})}},
		{name: "50#codec-values: a filter its codec cannot decode is codec alone",
			files: map[string]string{"views.flowmark.md": views(view(`{"where":`, `[]`, ""))},
			want:  []string{"views.flowmark.md:4 codec ~ filter"}},
		{name: "50#codec-values: an entity not of its type is type alone",
			files: map[string]string{"views.flowmark.md": views(viewRow{"1", "Long", "list", strings.Repeat("e", 64), noFilter, `[]`, ""})},
			want:  []string{"views.flowmark.md:4 type ~ entity"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkCorpus(t, cat, corpustest.NewTree(t, tc.files), tc.want)
		})
	}
}
