// Package irexec is the IR executor of a corpus under the orm IR-executor protocol
// (orm docs/protocol.md section 5 and flowmark spec 60): it decodes one request
// message, reads the corpus through the store and the validator's read, and
// answers a read with the rows assembled to the request's shape, or a write with
// {affected, keys} and the post-image of the write.
//
// The executor never writes a file, takes no lock and keeps no journal. A write is
// computed against one Corpus and answered with its PostImage: the files to write
// and to remove, each with the bytes it was read with, and the Corpus as it reads
// after the write. The write path commits a PostImage under the lock and the journal
// (spec 40 The commit protocol); a request inside a transaction reads the staged
// Corpus of the PostImage before it.
package irexec

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/orm/engine/dbspec"
	"github.com/polyspec/orm/engine/ir"
	"github.com/polyspec/orm/engine/runtimemodel"
)

// Executor runs the requests of one catalog. Its runtime model is built from the
// catalog's dbspec set, so a request's manifest hash is checked against the set the
// corpus declares.
type Executor struct {
	cat   *catalog.Catalog
	model *runtimemodel.Model
}

// New returns the executor of cat. The catalog was loaded valid, so its set builds
// a runtime model; an error says that it did not.
func New(cat *catalog.Catalog) (*Executor, error) {
	m, diags := runtimemodel.Build(cat.Documents)
	if len(diags) > 0 {
		return nil, &Error{Code: "SCHEMA_INVALID", Msg: runtimemodel.DiagnosticsError(diags)}
	}
	return &Executor{cat: cat, model: m}, nil
}

// Model returns the runtime model of the executor's catalog.
func (e *Executor) Model() *runtimemodel.Model { return e.model }

// message is one request message of the protocol: {ir_version, manifest_hash,
// request, params, now}.
type message struct {
	IRVersion    int               `json:"ir_version"`
	ManifestHash string            `json:"manifest_hash"`
	Request      json.RawMessage   `json:"request"`
	Params       []json.RawMessage `json:"params"`
	Now          string            `json:"now"`
}

// call is a decoded and validated request with its parameters and clock.
type call struct {
	req    *ir.Request
	params []json.RawMessage
	now    time.Time
}

// decode reads one request message. The message and its request each carry the
// IR version and the manifest hash; both are checked as the protocol checks a
// request (VERSION_MISMATCH, SCHEMA_HASH_MISMATCH), and the request is validated
// with ir.Validate. params must hold n_params values, and now is the request clock
// in the datetime wire form.
func (e *Executor) decode(b []byte, orm bool) (*call, *Error) {
	var m message
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, errf(codeIRInvalid, "the request message: %v", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, errf(codeIRInvalid, "the request message holds more than one object")
	}
	if m.IRVersion != ir.Version {
		return nil, errf(codeVersionMismatch, "ir_version %d, executor %d", m.IRVersion, ir.Version)
	}
	if m.ManifestHash != e.model.ManifestHash {
		return nil, errf(codeHashMismatch, "client %s, corpus %s", m.ManifestHash, e.model.ManifestHash)
	}
	if len(m.Request) == 0 || string(bytes.TrimSpace(m.Request)) == "null" {
		return nil, errf(codeIRInvalid, "the request message has no request")
	}
	req, err := ir.Decode(e.model, m.Request)
	if err != nil {
		return nil, fromIR(err)
	}
	if len(m.Params) != req.NParams {
		return nil, errf(codeIRInvalid, "params holds %d values for n_params %d", len(m.Params), req.NParams)
	}
	// The parameters of an orm message are checked against their slots; the values of a face operation are the
	// engine's own write, which the catalog's rules judge (spec 50 Types).
	if orm {
		if err := e.checkParams(req, m.Params); err != nil {
			return nil, err
		}
	}
	now, ok := datetimeOfWire(m.Now, 6)
	if !ok || now.null {
		return nil, errf(codeIRInvalid, "now %q is not a datetime in the form YYYY-MM-DD HH:MM:SS.ffffff", m.Now)
	}
	tm, _ := parseDatetime(now.text)
	return &call{req: req, params: m.Params, now: tm}, nil
}

// IsWrite reports whether the request message msg holds a write — insert, update,
// delete or restore — by the kind of its request, as the executor runs it. A message
// whose request kind does not decode holds no write; executing it answers its error.
// The write path reads it to refuse a write in a read-only transaction before it runs.
func IsWrite(msg []byte) bool {
	var m struct {
		Request struct {
			Kind string `json:"kind"`
		} `json:"request"`
	}
	if json.Unmarshal(msg, &m) != nil {
		return false
	}
	return isWriteKind(m.Request.Kind)
}

// isWriteKind reports whether a request kind is a write.
func isWriteKind(kind string) bool {
	switch kind {
	case "insert", "update", "delete", "restore":
		return true
	}
	return false
}

// Result is the answer of one request: the answer sent to the client, and, for a
// write, its post-image.
type Result struct {
	Answer *Answer
	Post   *PostImage
}

// Execute runs the request message msg against the corpus c. A read answers its
// rows; a write answers {affected, keys} with the post-image of the write, which
// the write path commits. A failed request returns an *Error.
func (e *Executor) Execute(c *Corpus, msg []byte) (*Result, error) { return e.execute(c, msg, true) }

// ExecuteFace runs the request message msg of a face operation, whose values are the engine's own write: the
// parameters are not checked against their slots as an orm message's are, and the catalog's rules judge them
// (spec 50 Types).
func (e *Executor) ExecuteFace(c *Corpus, msg []byte) (*Result, error) {
	return e.execute(c, msg, false)
}

func (e *Executor) execute(c *Corpus, msg []byte, orm bool) (*Result, error) {
	cl, err := e.decode(msg, orm)
	if err != nil {
		return nil, err
	}
	res, err := e.run(c, cl)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// run executes a decoded request.
func (e *Executor) run(c *Corpus, cl *call) (*Result, *Error) {
	if c.cat != e.cat {
		return nil, errf(codeInternal, "the corpus was read with another catalog than the executor's")
	}
	if isWriteKind(cl.req.Kind) {
		return e.write(c, cl)
	}
	a, err := e.read(c, cl)
	if err != nil {
		return nil, err
	}
	return &Result{Answer: a}, nil
}

// fieldType returns the dbspec type of a field of the runtime model.
func fieldType(f *runtimemodel.Field) dbspec.Type {
	return dbspec.Type{Kind: dbspec.TypeKind(f.Type), Length: f.Length, Precision: f.Precision, Scale: f.Scale}
}
