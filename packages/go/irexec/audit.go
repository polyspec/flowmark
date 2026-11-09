package irexec

import (
	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
)

// auditKey is the key of the audit record of the request, which every write of an audited table holds in its audit
// column (spec 50 Audit).
func (w *writer) auditKey() (cell, *Error) {
	if a := w.r.cl.audit; a != nil {
		return *a, nil
	}
	return cell{}, errf(codeConfig, "a write of an audited table needs an audit: table %s", w.tb.Name)
}

// auditRow writes what a database's audit triggers write for the row just written: one row of its history table with the
// action, the previous audit record and each recorded column, and with `tables` one row naming the table for the audit
// record, unless it stands (dbspec DBS-93). A load writes rows in their stored form, so it records nothing.
func (w *writer) auditRow(row *store.Row, action string, previous cell) *Error {
	a := w.tb.Audit
	if a == nil || w.r.cl.req.Load {
		return nil
	}
	x := w.next.recOf(row)
	ht, _ := w.c.cat.Table(a.History)
	if err := w.next.need(ht.Name); err != nil {
		return err
	}
	out := &store.Row{Table: ht.Name, Scalars: map[string]*string{}, Sections: map[string]*string{}}
	hk, err := w.nextIdentity(ht, ht.Key)
	if err != nil {
		return err
	}
	setValue(ht, out, ht.Key, hk)
	setValue(ht, out, a.Action, cell{text: action})
	setValue(ht, out, a.Previous, previous)
	for _, c := range w.tb.Columns {
		if !a.Records(c.Name) {
			continue
		}
		v, err := w.next.value(x, c.Name)
		if err != nil {
			return err
		}
		setValue(ht, out, c.Name, v)
	}
	w.appendRow(ht, out)
	if a.Tables == "" {
		return nil
	}
	tt, _ := w.c.cat.Table(a.Tables)
	if err := w.next.need(tt.Name); err != nil {
		return err
	}
	record, err := w.next.value(x, a.Column)
	if err != nil {
		return err
	}
	key := joinedKey(tt, map[string]string{a.TablesRecord: record.text, a.TablesName: w.tb.Name})
	if w.next.lookupOne(keyIndex(tt.Name, key)) != nil {
		return nil
	}
	named := &store.Row{Table: tt.Name, Key: key, Scalars: map[string]*string{}, Sections: map[string]*string{}}
	w.appendRow(tt, named)
	return nil
}

// joinedKey is the key cell of a row of a table with a composite key, from the value of each of its columns.
func joinedKey(tb *catalog.Table, values map[string]string) string {
	parts := make([]string, len(tb.KeyParts))
	for i, name := range tb.KeyParts {
		parts[i] = values[name]
	}
	return catalog.JoinKey(parts)
}

// appendRow adds a row the write originates to its table, stored as a document.
func (w *writer) appendRow(tb *catalog.Table, row *store.Row) {
	path := tb.Name + docSuffix
	f := w.ownFile(path)
	if f == nil {
		f = &file{table: tb.Name}
		w.next.setFile(path, f)
	}
	f.rows = append(f.rows, row)
	w.touched[path] = true
}
