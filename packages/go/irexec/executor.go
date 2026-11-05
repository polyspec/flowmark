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
	"slices"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/engine/ir"
	"github.com/polyspec/orm/packages/go/engine/runtimemodel"
)

// Executor runs the requests of one catalog. Its runtime model is built from the
// catalog's dbspec set, so a request's manifest hash is checked against the set the
// corpus declares.
type Executor struct {
	cat   *catalog.Catalog
	model *runtimemodel.Model
	// loading is the model a face's load reads: an external table takes rows there, since a corpus holds its rows and
	// the set that owns the document does not reach them (spec 30 External documents).
	loading *runtimemodel.Model
	// stated is the model of the difference of a SQL statement, which SQLite has judged: a column's NOT NULL is its own.
	stated *runtimemodel.Model
}

// New returns the executor of cat. The catalog was loaded valid, so its set builds
// a runtime model; an error says that it did not.
func New(cat *catalog.Catalog) (*Executor, error) {
	m := &runtimemodel.Model{Entities: map[string]*runtimemodel.Entity{}}
	if len(cat.Documents) > 0 {
		var diags []dbspec.Diagnostic
		if m, diags = runtimemodel.Build(cat.Documents); len(diags) > 0 {
			return nil, &Error{Code: "SCHEMA_INVALID", Msg: runtimemodel.DiagnosticsError(diags)}
		}
	}
	loading := *m
	loading.Entities = map[string]*runtimemodel.Entity{}
	for name, ent := range m.Entities {
		owned := *ent
		owned.External = false
		loading.Entities[name] = &owned
	}
	// The difference of a SQL statement also writes the tables that schema statements added.
	if cat.Added != nil && len(cat.Added.Tables) > 0 {
		all, diags := runtimemodel.Build(append(slices.Clone(cat.Documents), cat.Added))
		if len(diags) > 0 {
			return nil, &Error{Code: "SCHEMA_INVALID", Msg: runtimemodel.DiagnosticsError(diags)}
		}
		for name, ent := range all.Entities {
			if _, set := m.Entities[name]; !set {
				loading.Entities[name] = ent
			}
		}
	}
	stated, err := statedModel(cat)
	if err != nil {
		return nil, err
	}
	stated.ManifestHash = m.ManifestHash
	return &Executor{cat: cat, model: m, loading: &loading, stated: stated}, nil
}

// statedModel is the model of the difference of a SQL statement, over every table of the corpus: SQLite has judged the
// statement, so a column's NOT NULL is not the model's to refuse, and an external table takes rows like any other.
func statedModel(cat *catalog.Catalog) (*runtimemodel.Model, error) {
	open := make([]*dbspec.Document, len(cat.Stated))
	for i, d := range cat.Stated {
		c := *d
		c.External = false
		c.Tables = slices.Clone(d.Tables)
		for j := range c.Tables {
			t := &c.Tables[j]
			t.Columns = slices.Clone(t.Columns)
			for k := range t.Columns {
				if !slices.Contains(t.PrimaryKey.Columns, t.Columns[k].Name) {
					t.Columns[k].Null = true
				}
			}
		}
		open[i] = &c
	}
	if len(open) == 0 {
		return &runtimemodel.Model{Entities: map[string]*runtimemodel.Entity{}}, nil
	}
	m, diags := runtimemodel.Build(open)
	if len(diags) > 0 {
		return nil, &Error{Code: "SCHEMA_INVALID", Msg: runtimemodel.DiagnosticsError(diags)}
	}
	return m, nil
}

// Model returns the runtime model of the executor's catalog.
func (e *Executor) Model() *runtimemodel.Model { return e.model }

// message is one request message of the protocol: {ir_version, manifest_hash,
// request, params, now}.
type message struct {
	IRVersion    int    `json:"ir_version"`
	ManifestHash string `json:"manifest_hash"`
	// Audit is the key of the audit record of the transaction, for a write of an audited table (spec 50 Audit).
	Audit json.RawMessage `json:"audit,omitempty"`
	// Seed marks the insert of rows.load, which only a face sends.
	Seed bool `json:"seed,omitempty"`
	// ForeignKeysOff marks a seed of a connection whose foreign_keys is off: no foreign key judges its rows.
	ForeignKeysOff *bool `json:"foreign_keys_off,omitempty"`
	// Keys and Updates name the rows of a difference of a SQL statement: a delete takes the rows of Keys, and an update
	// gives each row of Updates its own values. Each key holds the parameters of its key columns.
	Keys    [][]int           `json:"keys,omitempty"`
	Updates []StatedUpdate    `json:"updates,omitempty"`
	Request json.RawMessage   `json:"request"`
	Params  []json.RawMessage `json:"params"`
	Now     string            `json:"now"`
}

// StatedUpdate is the update of one row of a difference of a SQL statement.
type StatedUpdate struct {
	Key []int       `json:"key"`
	Set []ir.Assign `json:"set"`
}

// call is a decoded and validated request with its parameters and clock.
type call struct {
	req   *ir.Request
	audit *cell
	// seed is a write a face forms from stored rows: rows.load, or the difference of a SQL statement.
	seed bool
	// noForeignKeys is a seed of a connection whose foreign_keys is off.
	noForeignKeys bool
	// stated is the difference of a SQL statement.
	stated bool
	// keys and updates are the rows the difference names.
	keys    [][]int
	updates []StatedUpdate
	params  []json.RawMessage
	page    *pageLimit
	now     time.Time
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
	decodeModel := e.model
	if !orm && m.Seed {
		decodeModel = e.loading
		if m.ForeignKeysOff != nil {
			decodeModel = e.stated
		}
	}
	req, err := ir.Decode(decodeModel, m.Request)
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
	page, pageErr := parameterizedPageLimit(req, m.Params)
	if pageErr != nil {
		return nil, pageErr
	}
	now, ok := datetimeOfWire(m.Now, 6)
	if !ok || now.null {
		return nil, errf(codeIRInvalid, "now %q is not a datetime in the form YYYY-MM-DD HH:MM:SS.ffffff", m.Now)
	}
	tm, err := parseDatetime(now.text)
	if err != nil {
		return nil, errf(codeIRInvalid, "now %q is not a datetime: %v", m.Now, err)
	}
	var audit *cell
	if len(m.Audit) > 0 && string(m.Audit) != "null" {
		v := cell{text: string(m.Audit)}
		var text string
		if json.Unmarshal(m.Audit, &text) == nil {
			v.text = text
		}
		audit = &v
	}
	return &call{keys: m.Keys, updates: m.Updates, req: req, audit: audit, seed: !orm && m.Seed, stated: !orm && m.Seed && m.ForeignKeysOff != nil, noForeignKeys: !orm && m.Seed && m.ForeignKeysOff != nil && *m.ForeignKeysOff, params: m.Params, page: page, now: tm}, nil
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
	entities := e.model.Entities
	if cl.seed {
		entities = e.loading.Entities
		if cl.stated {
			entities = e.stated.Entities
		}
	}
	if ent := entities[cl.req.Entity]; ent != nil {
		if _, ok := c.cat.Table(ent.Table); !ok {
			return nil, errf("DRIVER", "no such table: %s", ent.Table)
		}
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
