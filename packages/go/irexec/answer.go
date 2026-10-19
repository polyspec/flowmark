package irexec

import (
	"bytes"
	"encoding/json"
)

// Answer is the answer of a request. A row read (one, all, group_count) answers
// its rows; count its number; sum and avg their value; paginate its page rows and
// the count of every matching row; a write {affected, keys}.
type Answer struct {
	Kind     string
	Rows     []*Row
	Count    int64
	Value    any
	Affected int64
	Keys     []any
}

// MarshalJSON writes the answer as the protocol's JSON: {"rows": [...]} for a row
// read, {"count": n} for count, {"value": v} for sum and avg, {"rows": [...],
// "count": n} for paginate, and {"affected": n, "keys": [...]} for a write.
func (a *Answer) MarshalJSON() ([]byte, error) {
	rows := a.Rows
	if rows == nil {
		rows = []*Row{}
	}
	keys := a.Keys
	if keys == nil {
		keys = []any{}
	}
	switch a.Kind {
	case "count":
		return marshalPlain(struct {
			Count int64 `json:"count"`
		}{a.Count})
	case "sum", "avg":
		return marshalPlain(struct {
			Value any `json:"value"`
		}{a.Value})
	case "paginate":
		return marshalPlain(struct {
			Rows  []*Row `json:"rows"`
			Count int64  `json:"count"`
		}{rows, a.Count})
	case "insert", "update", "delete", "restore":
		return marshalPlain(struct {
			Affected int64 `json:"affected"`
			Keys     []any `json:"keys"`
		}{a.Affected, keys})
	}
	return marshalPlain(struct {
		Rows []*Row `json:"rows"`
	}{rows})
}

// marshalPlain returns the JSON of v without HTML escaping and without a line feed: json.Marshal escapes <, > and &
// also in the output of a Marshaler, and an encoder that marshals the answer does not undo them, so an answer would no
// longer read as its values are stored.
func marshalPlain(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// Row is one assembled row: its outputs in projection order, then its joins and
// relations. A join child or a one relation is a *Row or nil, a many relation a
// []*Row, or a *Row keyed by key_by.
type Row struct {
	names  []string
	values map[string]any
}

func newRow() *Row { return &Row{values: map[string]any{}} }

// set gives name the value v, appending name when the row does not hold it.
func (r *Row) set(name string, v any) {
	if _, ok := r.values[name]; !ok {
		r.names = append(r.names, name)
	}
	r.values[name] = v
}

// Get returns the value of name and whether the row holds it.
func (r *Row) Get(name string) (any, bool) {
	v, ok := r.values[name]
	return v, ok
}

// Names returns the names of the row in their order.
func (r *Row) Names() []string { return append([]string(nil), r.names...) }

// MarshalJSON writes the row as a JSON object in its order, its strings without HTML escaping (see marshalPlain).
func (r *Row) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, n := range r.names {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := marshalPlain(n)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		v, err := marshalPlain(r.values[n])
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
