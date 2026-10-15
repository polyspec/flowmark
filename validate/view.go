package validate

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/engine/ir"
	"github.com/polyspec/orm/packages/go/engine/runtimemodel"
	"github.com/polyspec/orm/packages/go/orm"
)

// The rule names of the codec and view rules (spec 50 Codec values, Views).
const (
	RuleCodec = "codec"
	RuleView  = "view"
)

// HostStages are the codec stages that orm decodes on the host and does not
// export for a decode of its own (orm T74): orm.Decode refuses every stage list
// that holds one.
var HostStages = []string{"hex", "ip"}

// RowFault is a fault of the codec or view rule in one row: the column whose
// element it is at, the rule and the message.
type RowFault struct {
	Column, Rule, Message string
}

// JudgeRow returns the codec faults of the row r of table tb, by column in byte
// order, then its view faults when tb is the view table (spec 50 Codec values,
// Views). The validator reports them over the corpus, and the commit judges every
// row it writes with them, so the two judge a row one way. m is orm's runtime
// model of cat, needed only when tb is the view table.
func JudgeRow(cat *catalog.Catalog, m *runtimemodel.Model, tb *catalog.Table, r *store.Row) []RowFault {
	out := codecFaults(tb, r)
	faulty := map[string]bool{}
	for _, f := range out {
		faulty[f.Column] = true
	}
	return append(out, viewFaults(cat, m, tb, r, faulty)...)
}

// judgeRow reports the codec and view faults of the row p of tb.
func (c *corpus) judgeRow(tb *catalog.Table, p placed) error {
	var m *runtimemodel.Model
	if tb.View != nil {
		var err error
		if m, err = c.runtimeModel(); err != nil {
			return err
		}
	}
	for _, f := range JudgeRow(c.cat, m, tb, p.row) {
		c.report(p, f.Column, f.Rule, f.Message)
	}
	return nil
}

// codecFaults returns a fault for each non-NULL value of its type in a codec
// column of r that orm's codec package cannot decode, at the value's element. A
// column whose stages hold a host stage is not judged until orm exports its host
// decode.
func codecFaults(tb *catalog.Table, r *store.Row) []RowFault {
	var out []RowFault
	for _, col := range sortedKeys(tb.Codecs) {
		stages := tb.Codecs[col]
		if slices.ContainsFunc(stages, func(s string) bool { return slices.Contains(HostStages, s) }) {
			continue
		}
		if _, raw := r.Raw[col]; raw {
			continue
		}
		v := r.Scalars[col]
		if v == nil {
			v = r.Sections[col]
		}
		if v == nil {
			continue
		}
		cell := []byte(*v)
		if columnKind(tb, col) == dbspec.TypeBytes {
			b, err := base64.StdEncoding.DecodeString(*v)
			if err != nil { // a bytes value of its type is padded base64
				continue
			}
			cell = b
		}
		if _, err := orm.Decode(stages, cell); err != nil {
			out = append(out, RowFault{col, RuleCodec, fmt.Sprintf("%s holds a value its codec %v cannot decode: %v", col, stages, err)})
		}
	}
	return out
}

// sortedKeys returns the keys of m in byte order.
func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// listKinds are the view kinds whose entity declares a title and a body (spec 20
// Title and body).
var listKinds = []string{"kanban", "list", "gantt"}

// viewFaults returns the view faults of the row r of the view table tb (spec 50
// Views), none when tb is another table, judged by JudgeView over the columns of
// r it can judge: a column that is NULL or has a type or codec fault is that
// fault alone. faulty holds the columns of r with a codec fault.
func viewFaults(cat *catalog.Catalog, m *runtimemodel.Model, tb *catalog.Table, r *store.Row, faulty map[string]bool) []RowFault {
	v := tb.View
	if v == nil {
		return nil
	}
	usable := func(col string) (string, bool) {
		if _, raw := r.Raw[col]; raw || faulty[col] {
			return "", false
		}
		s := r.Scalars[col]
		if s == nil {
			return "", false
		}
		return *s, true
	}
	vals := map[string]string{}
	for _, col := range []string{v.Kind, v.Entity, v.Filter, v.Sort, v.Group} {
		if s, ok := usable(col); ok {
			vals[col] = s
		}
	}
	var out []RowFault
	for _, f := range JudgeView(cat, m, v, vals) {
		out = append(out, RowFault{f.Column, RuleView, f.Message})
	}
	return out
}

// ViewFault is a fault of a view (spec 50 Views): the view column it stands at and its message.
type ViewFault struct {
	Column  string
	Message string
}

// JudgeView returns the faults of a view of the view table whose columns v names, over the catalog cat and its runtime
// model m (spec 50 Views): vals holds the text of each view column that can be judged, a column being absent when it is
// NULL or has a fault of its own. A faulty or absent entity leaves the other columns unjudged; the faces judge a view
// with it before they run it.
func JudgeView(cat *catalog.Catalog, m *runtimemodel.Model, v *catalog.View, vals map[string]string) []ViewFault {
	name, ok := vals[v.Entity]
	if !ok {
		return nil
	}
	ent := m.Entities[name]
	if ent == nil {
		return []ViewFault{{v.Entity, fmt.Sprintf("the entity %s is no entity of the catalog", name)}}
	}
	if kind, ok := vals[v.Kind]; ok && slices.Contains(listKinds, kind) {
		et, _ := cat.Table(ent.Table)
		if et == nil || et.Title == "" || et.Body == "" {
			return []ViewFault{{v.Entity, fmt.Sprintf("a %s view needs an entity whose table declares title and body, and %s declares no title or no body", kind, name)}}
		}
	}
	var out []ViewFault
	if text, ok := vals[v.Filter]; ok {
		if msg := judgeFilter(m, name, text); msg != "" {
			out = append(out, ViewFault{v.Filter, "the filter " + msg})
		}
	}
	if text, ok := vals[v.Sort]; ok {
		if msg := judgeSort(m, name, text); msg != "" {
			out = append(out, ViewFault{v.Sort, "the sort " + msg})
		}
	}
	if g, ok := vals[v.Group]; ok {
		f := ent.Field(g)
		switch {
		case f == nil:
			out = append(out, ViewFault{v.Group, fmt.Sprintf("the group %s is no field of %s", g, name)})
		case !ir.OpAllowed(f, "eq"):
			out = append(out, ViewFault{v.Group, fmt.Sprintf("the group %s of %s has no equality, so its rows have no groups", g, name)})
		}
	}
	return out
}

// runtimeModel returns orm's runtime model of the catalog, built once.
func (c *corpus) runtimeModel() (*runtimemodel.Model, error) {
	if c.model != nil {
		return c.model, nil
	}
	m, diags := runtimemodel.Build(c.cat.Documents)
	if len(diags) > 0 {
		return nil, fmt.Errorf("the runtime model of a loaded catalog: %s", runtimemodel.DiagnosticsError(diags))
	}
	c.model = m
	return m, nil
}

// storedFilter is the stored form of a view's filter.
type storedFilter struct {
	Where  *ir.Group         `json:"where"`
	Params []json.RawMessage `json:"params"`
}

// judgeFilter returns the fault of the stored filter text of a view of entity,
// "" when it holds a valid filter.
func judgeFilter(m *runtimemodel.Model, entity, text string) string {
	var f storedFilter
	if err := strictJSON(text, &f); err != nil {
		return "is malformed: " + err.Error()
	}
	switch {
	case f.Where == nil:
		return "has no where"
	case f.Where.Items == nil:
		return "has a where without items"
	case f.Params == nil:
		return "has no params"
	}
	for i, p := range f.Params {
		if p[0] == '{' || p[0] == '[' {
			return fmt.Sprintf("holds param %d, which is not null, a boolean, a number or a string", i)
		}
	}
	r := &ir.Request{IRVersion: ir.Version, ManifestHash: m.ManifestHash, Kind: "all",
		Query: ir.Query{Entity: entity, Where: f.Where}, NParams: len(f.Params)}
	if err := ir.Validate(m, r); err != nil {
		return "is refused by ir.Validate: " + err.Error()
	}
	used := map[int]bool{}
	usesOfQuery(&r.Query, used)
	for i := range f.Params {
		if !used[i] {
			return fmt.Sprintf("holds param %d, which no index uses", i)
		}
	}
	return ""
}

// judgeSort returns the fault of the stored sort text of a view of entity, ""
// when it holds a valid sort.
func judgeSort(m *runtimemodel.Model, entity, text string) string {
	var order []ir.Order
	if err := strictJSON(text, &order); err != nil {
		return "is malformed: " + err.Error()
	}
	if order == nil {
		return "is not an array"
	}
	r := &ir.Request{IRVersion: ir.Version, ManifestHash: m.ManifestHash, Kind: "all",
		Query: ir.Query{Entity: entity, Order: order}}
	if err := ir.Validate(m, r); err != nil {
		return "is refused by ir.Validate: " + err.Error()
	}
	return ""
}

// strictJSON decodes the one JSON value of text into out: a repeated object
// member at any depth, a member out does not have and any text after the value
// are faults.
func strictJSON(text string, out any) error {
	if err := noRepeatedMembers(json.NewDecoder(bytes.NewReader([]byte(text)))); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(text)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("text follows the value")
	}
	return nil
}

// noRepeatedMembers reads one JSON value from dec and refuses an object that
// names a member twice.
func noRepeatedMembers(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch tok {
	case json.Delim('{'):
		seen := map[string]bool{}
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return err
			}
			name, _ := k.(string)
			if seen[name] {
				return fmt.Errorf("the member %s repeats", strconv.Quote(name))
			}
			seen[name] = true
			if err := noRepeatedMembers(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case json.Delim('['):
		for dec.More() {
			if err := noRepeatedMembers(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	}
	return nil
}

// usesOfQuery records in used every parameter index the query q refers to.
func usesOfQuery(q *ir.Query, used map[int]bool) {
	if q == nil {
		return
	}
	usesOfGroup(q.Where, used)
	usesOfGroup(q.On, used)
	for _, j := range q.Joins {
		usesOfQuery(j.Query, used)
	}
	for _, r := range q.Relations {
		usesOfQuery(r.Query, used)
	}
	for _, o := range q.Order {
		usesOfFunc(o.Fn, used)
	}
	if q.Columns != nil {
		for _, f := range q.Columns.Fn {
			usesOfFunc(&f.Fn, used)
		}
		for _, s := range q.Columns.Sub {
			usesOfQuery(s.Query, used)
		}
	}
	if q.IfParent != nil {
		used[q.IfParent.P] = true
	}
}

// usesOfGroup records in used every parameter index the group g refers to.
func usesOfGroup(g *ir.Group, used map[int]bool) {
	if g == nil {
		return
	}
	for _, it := range g.Items {
		usesOfGroup(it.Group, used)
		if pr := it.Pred; pr != nil {
			if pr.P != nil {
				used[*pr.P] = true
			}
			for _, i := range pr.Ps {
				used[i] = true
			}
			usesOfFunc(pr.Fn, used)
			usesOfFunc(pr.Value, used)
			if pr.Sub != nil {
				usesOfQuery(pr.Sub.Query, used)
			}
		}
	}
}

// usesOfFunc records in used the parameter indexes of the function f.
func usesOfFunc(f *ir.Func, used map[int]bool) {
	if f == nil {
		return
	}
	for _, i := range f.Ps {
		used[i] = true
	}
}
