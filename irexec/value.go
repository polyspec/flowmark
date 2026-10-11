package irexec

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/polyspec/flowmark/store"
	"github.com/polyspec/orm/engine/dbspec"
)

// The runtime value model JSON of the protocol (orm docs/protocol.md section 5): a
// decimal is a string, bytes are base64 and a datetime is YYYY-MM-DD HH:MM:SS.ffffff
// in UTC. An integer and a float are JSON numbers, a bool a JSON bool, and every
// other type a JSON string in its readable form.
const datetimeWire = "2006-01-02 15:04:05.000000"

// timeWire is the wire form of every time: HH:MM:SS.ffffff, whose digits past the column's precision are zeros.
const timeWire = "15:04:05.000000"

// decodeParam reads the parameter raw as a value of type t and returns its
// canonical readable text, or null. ok is false when raw is not a value of t in
// the runtime value model JSON.
func decodeParam(raw json.RawMessage, t dbspec.Type) (cell, bool) {
	raw = bytes.TrimSpace(raw)
	if string(raw) == "null" {
		return cell{null: true}, true
	}
	switch t.Kind {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64, dbspec.TypeF64:
		var n json.Number
		if !isJSONNumber(raw) || json.Unmarshal(raw, &n) != nil {
			return cell{}, false
		}
		text := n.String()
		if isInteger(t.Kind) {
			// A JSON number is an integer of the column when it has no fraction
			// and no exponent; dbspec's literal reads only that form.
			if strings.ContainsAny(text, ".eE") {
				return cell{}, false
			}
		} else if strings.ContainsAny(text, "eE") {
			f, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsInf(f, 0) {
				return cell{}, false
			}
			text = strconv.FormatFloat(f, 'f', -1, 64)
		}
		v, ok := store.Readable(t, text)
		return cell{text: v}, ok
	case dbspec.TypeBool:
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return cell{}, false
		}
		return cell{text: strconv.FormatBool(b)}, true
	}
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return cell{}, false
	}
	switch t.Kind {
	case dbspec.TypeDatetime:
		return datetimeOfWire(s, t.Precision)
	case dbspec.TypeTime:
		return timeOfWire(s, t.Precision)
	case dbspec.TypeBytes:
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return cell{}, false
		}
		return cell{text: base64.StdEncoding.EncodeToString(b)}, true
	}
	v, ok := store.Readable(t, s)
	return cell{text: v}, ok
}

// datetimeOfWire reads a datetime in the wire form, whose six fraction digits are
// the form of every datetime on the wire, as a value of datetime(p): the digits past
// p must be zeros, which the column does not hold.
func datetimeOfWire(s string, p int) (cell, bool) {
	tm, err := time.Parse(datetimeWire, s)
	if err != nil || tm.Format(datetimeWire) != s {
		return cell{}, false
	}
	if !tm.Equal(tm.Truncate(precisionUnit(p))) {
		return cell{}, false
	}
	return cell{text: formatDatetime(tm, p)}, true
}

// timeOfWire reads a time in the wire form, whose six fraction digits are the form of every time on the wire, as a
// value of time(p): the digits past p must be zeros, which the column does not hold.
func timeOfWire(s string, p int) (cell, bool) {
	tm, err := time.Parse(timeWire, s)
	if err != nil || tm.Format(timeWire) != s {
		return cell{}, false
	}
	if !tm.Equal(tm.Truncate(precisionUnit(p))) {
		return cell{}, false
	}
	return cell{text: formatTime(tm, p)}, true
}

// formatTime writes tm as a readable time(p): HH:MM:SS with exactly p fraction digits.
func formatTime(tm time.Time, p int) string {
	layout := "15:04:05"
	if p > 0 {
		layout += "." + strings.Repeat("0", p)
	}
	return tm.Format(layout)
}

// precisionUnit is the duration of the last fraction digit of precision p.
func precisionUnit(p int) time.Duration {
	d := time.Second
	for i := 0; i < p; i++ {
		d /= 10
	}
	return d
}

// formatDatetime writes tm as a readable datetime(p): YYYY-MM-DD HH:MM:SS with
// exactly p fraction digits.
func formatDatetime(tm time.Time, p int) string {
	layout := "2006-01-02 15:04:05"
	if p > 0 {
		layout += "." + strings.Repeat("0", p)
	}
	return tm.Format(layout)
}

// isJSONNumber reports whether raw is a JSON number token.
func isJSONNumber(raw []byte) bool {
	return len(raw) > 0 && (raw[0] == '-' || ('0' <= raw[0] && raw[0] <= '9'))
}

// output returns the JSON value of a readable value of type t in the runtime value
// model JSON. A datetime that does not read is INTERNAL: the corpus holds readable
// values only, and an answer never carries a stored text in place of the wire form.
// A time is answered in the wire form HH:MM:SS.ffffff (spec 60 The wire).
func output(t dbspec.Type, v cell) (any, *Error) {
	if v.null {
		return nil, nil
	}
	switch t.Kind {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64, dbspec.TypeF64:
		return json.Number(v.text), nil
	case dbspec.TypeBool:
		return v.text == "true", nil
	case dbspec.TypeDatetime:
		tm, err := parseDatetime(v.text)
		if err != nil {
			return nil, errf(codeInternal, "%q is not a readable %s", v.text, t.String())
		}
		return tm.Format(datetimeWire), nil
	case dbspec.TypeTime:
		tm, err := time.Parse("15:04:05.999999999", v.text)
		if err != nil {
			return nil, errf(codeInternal, "%q is not a readable %s", v.text, t.String())
		}
		return tm.Format(timeWire), nil
	}
	return v.text, nil
}

// parseDatetime reads a readable datetime, with or without fraction digits.
func parseDatetime(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05.999999999", s)
}

// scalar is a value read for a comparison: a number, a time, a bool or a text.
type scalar struct {
	kind  scalarKind
	num   *big.Rat  // integers, decimals and floats
	tm    time.Time // dates and datetimes, a date at midnight
	nanos int64     // a time of day
	text  string    // texts, uuids, bytes and bools as their canonical text
}

type scalarKind int

const (
	kindNumber scalarKind = iota
	kindTime
	kindTimeOfDay
	kindText
	kindBool
)

// scalarOf reads the readable value text of type t for a comparison; ok is false
// when text is not of the type.
func scalarOf(t dbspec.Type, text string) (scalar, bool) {
	switch t.Kind {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64, dbspec.TypeDecimal, dbspec.TypeF64:
		r, ok := new(big.Rat).SetString(text)
		return scalar{kind: kindNumber, num: r}, ok
	case dbspec.TypeDate:
		tm, err := time.Parse("2006-01-02", text)
		return scalar{kind: kindTime, tm: tm}, err == nil
	case dbspec.TypeDatetime:
		tm, err := parseDatetime(text)
		return scalar{kind: kindTime, tm: tm}, err == nil
	case dbspec.TypeTime:
		tm, err := time.Parse("15:04:05.999999999", text)
		if err != nil {
			return scalar{}, false
		}
		return scalar{kind: kindTimeOfDay, nanos: int64(tm.Hour())*int64(time.Hour) + int64(tm.Minute())*int64(time.Minute) + int64(tm.Second())*int64(time.Second) + int64(tm.Nanosecond())}, true
	case dbspec.TypeBool:
		return scalar{kind: kindBool, text: text}, text == "true" || text == "false"
	}
	return scalar{kind: kindText, text: text}, true
}

// compare orders two scalars of one kind: numbers by value, times by instant, bools
// false before true, and texts by code point, which is byte order in UTF-8 — the one
// collation of orm (docs/dialects.md, "Collation": binary code point order).
func compare(a, b scalar) int {
	switch a.kind {
	case kindNumber:
		return a.num.Cmp(b.num)
	case kindTime:
		return a.tm.Compare(b.tm)
	case kindTimeOfDay:
		switch {
		case a.nanos < b.nanos:
			return -1
		case a.nanos > b.nanos:
			return 1
		}
		return 0
	case kindBool:
		switch {
		case a.text == b.text:
			return 0
		case a.text == "false":
			return -1
		}
		return 1
	}
	return strings.Compare(a.text, b.text)
}
