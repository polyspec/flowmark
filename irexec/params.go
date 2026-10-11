package irexec

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/polyspec/orm/engine/dbspec"
	"github.com/polyspec/orm/engine/ir"
)

// checkParams checks every parameter of a request against each slot it fills before any
// row is read (spec 60 The wire): a parameter that is not a value of the type of its slot
// — the column it is compared with or assigned to, a column function's result, text for
// contains and contains_binary, the interval of a relative time function — is
// CODEC_ENCODE, naming the parameter index and the type. NULL is a value of every slot
// except the assignment of a non-null column. The check is made before reading, so the
// answer does not depend on the rows, as a bind failure does not.
func (e *Executor) checkParams(req *ir.Request, params []json.RawMessage) *Error {
	for i, us := range ir.ParamUses(e.model, req) {
		raw := bytes.TrimSpace(params[i])
		if string(raw) == "null" {
			continue
		}
		for _, u := range us {
			t := slotType(u)
			if t.Kind == dbspec.TypeTime {
				// The time wire form waits on orm T91 (T12.4-6): the write path judges a time until then.
				continue
			}
			if _, ok := decodeParam(raw, t); !ok || textual(t) && hasNUL(raw) {
				return errf(codeCodecEncode, "parameter %d is not a value of %s", i, t.String())
			}
		}
	}
	ent := e.model.Entities[req.Entity]
	for _, a := range assignments(req) {
		if string(bytes.TrimSpace(params[a.p])) != "null" {
			continue
		}
		if f := ent.Field(a.column); f != nil && !f.Null {
			return errf(codeCodecEncode, "parameter %d assigns NULL to the non-null %s.%s", a.p, req.Entity, a.column)
		}
	}
	return nil
}

// slotType is the logical type of a parameter slot.
func slotType(u ir.ParamUse) dbspec.Type {
	if u.Column != nil {
		return fieldType(u.Column)
	}
	return dbspec.Type{Kind: dbspec.TypeKind(u.Type)}
}

// textual reports whether t is a type whose value is text.
func textual(t dbspec.Type) bool { return t.Kind == dbspec.TypeText || t.Kind == dbspec.TypeVarchar }

// hasNUL reports whether the JSON string raw holds U+0000, which a text value may not.
func hasNUL(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil && strings.ContainsRune(s, 0)
}

type assignment struct {
	column string
	p      int
}

// assignments lists the value parameters the request assigns to columns: set, the rows of
// a multi-row insert, and on_duplicate. A plus or minus assignment adds to the column and
// so assigns no NULL.
func assignments(req *ir.Request) []assignment {
	var out []assignment
	for _, a := range req.Set {
		if a.P != nil {
			out = append(out, assignment{a.Column, *a.P})
		}
	}
	for _, row := range req.Rows {
		for j, i := range row {
			out = append(out, assignment{req.Set[j].Column, i})
		}
	}
	for _, a := range req.OnDuplicate {
		if a.P != nil {
			out = append(out, assignment{a.Column, *a.P})
		}
	}
	return out
}
