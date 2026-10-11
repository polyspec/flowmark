package irexec

import (
	"errors"
	"fmt"
	"strings"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store"
	"github.com/polyspec/orm/engine/ir"
)

// The error codes of orm errors.yaml that the executor answers with.
const (
	codeVersionMismatch = "VERSION_MISMATCH"
	codeHashMismatch    = "SCHEMA_HASH_MISMATCH"
	codeIRInvalid       = "IR_INVALID"
	codeCapability      = "CAPABILITY_UNSUPPORTED"
	codeColumnUnknown   = "COLUMN_UNKNOWN"
	codeNotJoined       = "ENTITY_NOT_JOINED"
	codeOpNotAllowed    = "OPERATOR_NOT_ALLOWED"
	codeOptimistic      = "OPTIMISTIC_LOCK"
	codeConfig          = "CONFIG"
	codeNoRows          = "NO_ROWS"
	codeEntityUnknown   = "ENTITY_UNKNOWN"
	codeDuplicateKey    = "DUPLICATE_KEY"
	codeForeignKey      = "FOREIGN_KEY"
	codeConstraint      = "CONSTRAINT"
	codeInternal        = "INTERNAL"
)

// Error is a failed request: one code of orm errors.yaml and a message that names
// the cause. A write that the key, reference or catalog rules refuse carries its
// faults, located in the request (spec 10 Diagnostics, "Write faults"); a request
// refused for a diagnostic of a file (an unavailable table, a value not of its
// type) carries that diagnostic.
type Error struct {
	Code       string
	Msg        string
	Faults     []store.Refusal
	Diagnostic *catalog.Diagnostic
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

// errf returns an error of code with a formatted message.
func errf(code, format string, a ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, a...)}
}

// fromIR converts an error of orm's ir package to an executor error with its code.
func fromIR(err error) *Error {
	var e *ir.Error
	if errors.As(err, &e) {
		return &Error{Code: e.Code, Msg: e.Msg}
	}
	return &Error{Code: codeInternal, Msg: err.Error()}
}

// diagError is the CONSTRAINT error of a request that needs what diagnostic d
// stands for, naming the file and the diagnostic.
func diagError(why string, d catalog.Diagnostic) *Error {
	return &Error{Code: codeConstraint, Msg: fmt.Sprintf("%s: %s:%d:%d %s: %s", why, d.File, d.Line, d.Column, d.Rule, d.Message), Diagnostic: &d}
}

// unspecified is the error of a request whose rule spec 60 states as orm's but
// that orm's validator does not refuse yet. It names the case and the orm item it
// waits on; the executor never supplies orm's rule in its place (spec 60
// Surfaces). CAPABILITY_UNSUPPORTED is the code of a surface the executor does not
// run.
func unspecified(question string) *Error {
	return &Error{Code: codeCapability, Msg: "the executor does not run this until orm's rule lands: " + question}
}

// faultError is the error of a write refused by faults, whose code is the code of
// its first fault's rule: DUPLICATE_KEY, FOREIGN_KEY or CONSTRAINT.
func faultError(faults []store.Refusal) *Error {
	msgs := make([]string, len(faults))
	for i, f := range faults {
		at := f.Table + " " + f.Key
		if f.Index >= 0 {
			at += fmt.Sprintf(" (index %d)", f.Index)
		}
		if f.Column != "" {
			at += ", column " + f.Column
		}
		msgs[i] = f.Rule + " at " + at + ": " + f.Message
	}
	return &Error{Code: faults[0].Rule, Msg: strings.Join(msgs, "; "), Faults: faults}
}
