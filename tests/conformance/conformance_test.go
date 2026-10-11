// Package conformance runs the conformance vectors of spec 90: a corpus before — a shared catalog and documents —, an
// IR request run through the write path, and the expected answer, corpus after and repeat.
package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/irexec"
	"github.com/polyspec/flowmark/store/journal"
	"github.com/polyspec/flowmark/validate"
	"github.com/polyspec/flowmark/writepath"
)

// irVersion is the IR version of the request messages the runner builds (orm protocol 1).
const irVersion = 1

// storages are the storages a vector names, each with a read and a write vector at least.
var storages = []string{"files", "document list", "document table", "block list", "block table"}

// text is a file's text: a JSON string of its exact bytes, or an array of its lines, each ending with a line feed.
type text string

func (t *text) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = text(s)
		return nil
	}
	var lines []string
	if err := json.Unmarshal(b, &lines); err != nil {
		return fmt.Errorf("a text is a string or an array of lines: %s", b)
	}
	var sb strings.Builder
	for _, l := range lines {
		if strings.Contains(l, "\n") {
			return fmt.Errorf("the line %q of a text holds a line feed", l)
		}
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	*t = text(sb.String())
	return nil
}

// expectation is an expected answer: an error, or the members of a successful answer and the files it changed.
type expectation struct {
	Error      *string             `json:"error"`
	Diagnostic *string             `json:"diagnostic"`
	Rows       *[]map[string]any   `json:"rows"`
	Count      json.RawMessage     `json:"count"`
	Value      json.RawMessage     `json:"value"`
	Affected   json.RawMessage     `json:"affected"`
	Keys       json.RawMessage     `json:"keys"`
	Corpus     map[string]*text    `json:"corpus"`
	members    map[string]struct{} // the answer members the expectation names
}

func (e *expectation) UnmarshalJSON(b []byte) error {
	type plain expectation
	var p plain
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&p); err != nil {
		return err
	}
	*e = expectation(p)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	e.members = map[string]struct{}{}
	for _, m := range []string{"rows", "count", "value", "affected", "keys"} {
		if _, ok := raw[m]; ok {
			e.members[m] = struct{}{}
		}
	}
	switch {
	case e.Error != nil && (len(e.members) > 0 || e.Corpus != nil):
		return errors.New("an expectation of an error names no answer member and no corpus")
	case e.Error == nil && e.Diagnostic != nil:
		return errors.New("an expectation names a diagnostic only with an error")
	case e.Error == nil && len(e.members) == 0:
		return errors.New("an expectation names an error or the members of an answer")
	}
	return nil
}

// vector is one conformance vector (spec 90 The vector shape).
type vector struct {
	Name        string              `json:"name"`
	Storage     string              `json:"storage"`
	Catalog     string              `json:"catalog"`
	Corpus      map[string]text     `json:"corpus"`
	Diagnostics map[string][]string `json:"diagnostics"`
	Equivalent  string              `json:"equivalent"`
	Request     json.RawMessage     `json:"request"`
	Params      []json.RawMessage   `json:"params"`
	Now         string              `json:"now"`
	Expect      *expectation        `json:"expect"`
	Repeat      *expectation        `json:"repeat"`
}

// decodeVector reads the vector of the file named file: every member it does not name is refused, and its name is the
// file's name.
func decodeVector(file string, b []byte) (*vector, error) {
	var v vector
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s: %v", file, err)
	}
	if want := strings.TrimSuffix(filepath.Base(file), ".json"); v.Name != want {
		return nil, fmt.Errorf("%s: the name %q is not the file name %q", file, v.Name, want)
	}
	switch {
	case !slices.Contains(storages, v.Storage):
		return nil, fmt.Errorf("%s: the storage %q is none of %q", file, v.Storage, storages)
	case v.Catalog == "" || len(v.Request) == 0 || v.Now == "" || v.Expect == nil:
		return nil, fmt.Errorf("%s: a vector names its catalog, request, now and expect", file)
	}
	if v.Params == nil {
		v.Params = []json.RawMessage{}
	}
	return &v, nil
}

// loadVectors reads every vector of the folder vectors.
func loadVectors(t *testing.T) []*vector {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("vectors", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("vectors: %v (%d)", err, len(files))
	}
	var out []*vector
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		v, err := decodeVector(file, b)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

// TestVectors runs every vector, then checks the equivalence groups and the coverage of every storage.
func TestVectors(t *testing.T) {
	vectors := loadVectors(t)
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			if err := runVector(t.TempDir(), v); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := checkGroups(vectors); err != nil {
		t.Error(err)
	}
	if err := checkCoverage(vectors); err != nil {
		t.Error(err)
	}
}

// ignore is the observer of the runner's corpora: a vector compares the answer and the files, not the events, which
// the write path's own tests pin.
type ignore struct{}

func (ignore) Observe(journal.Event) {}

// outcome is what one run of a request answers: the answer's members, or the error's code and diagnostic rule.
type outcome struct {
	members    map[string]json.RawMessage
	code       string
	diagnostic string
	random     bool // the request orders by random, so its rows compare as a set
}

// runVector runs the vector v in the empty folder root and returns the first way its run departs from it.
func runVector(root string, v *vector) (err error) {
	dbs, err := os.ReadFile(filepath.Join("catalogs", v.Catalog+".dbs"))
	if err != nil {
		return fmt.Errorf("the catalog %q: %v", v.Catalog, err)
	}
	before := map[string]string{"dbspec/" + v.Catalog + ".dbs": string(dbs)}
	for p, t := range v.Corpus {
		if _, ok := before[p]; ok {
			return fmt.Errorf("the corpus names the catalog file %s", p)
		}
		before[p] = string(t)
	}
	for p, t := range before {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(t), 0o644); err != nil {
			return err
		}
	}
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		return fmt.Errorf("the catalog %q does not load: %+v", v.Catalog, diags)
	}
	if err := checkStorage(cat, v); err != nil {
		return err
	}
	declared := map[string][]string{}
	for p, rules := range v.Diagnostics {
		declared[p] = uniqueSorted(rules)
	}
	got, err := diagnostics(cat, root)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(got, declared) {
		return fmt.Errorf("the diagnostics of the corpus before are %v, the vector declares %v", got, declared)
	}
	exec, err := irexec.New(cat)
	if err != nil {
		return err
	}
	msg, random, err := message(v, exec.Model().ManifestHash)
	if err != nil {
		return err
	}
	j, err := journal.New(root, ignore{})
	if err != nil {
		return err
	}
	c, err := writepath.Open(context.Background(), cat, j, 0)
	if err != nil {
		return fmt.Errorf("opening the corpus: %v", err)
	}
	defer func() { err = errors.Join(err, c.Close()) }()

	first, err := run(c, msg)
	if err != nil {
		return err
	}
	first.random = random
	after, err := expectOutcome(root, v.Expect, first, before)
	if err != nil {
		return err
	}
	if err := checkAfter(cat, root, before, after, declared); err != nil {
		return err
	}

	// The repeat (spec 90 Required properties, idempotency).
	written := first.code == "" && irexec.IsWrite(msg)
	if written != (v.Repeat != nil) {
		if written {
			return errors.New("the write succeeds, and its vector states no repeat")
		}
		return errors.New("the vector states a repeat, and its request is no write that succeeds")
	}
	second, err := run(c, msg)
	if err != nil {
		return err
	}
	second.random = random
	if !written {
		if _, err := expectOutcome(root, v.Expect, second, after); err != nil {
			return fmt.Errorf("the repeat: %v", err)
		}
		if !random && !reflect.DeepEqual(first, second) {
			return fmt.Errorf("the repeat answers %v, the first run %v", second, first)
		}
		return nil
	}
	again, err := expectOutcome(root, v.Repeat, second, after)
	if err != nil {
		return fmt.Errorf("the repeat: %v", err)
	}
	if err := checkAfter(cat, root, before, again, declared); err != nil {
		return fmt.Errorf("the repeat: %v", err)
	}
	return nil
}

// checkStorage checks that the vector's storage is the storage of its request's entity.
func checkStorage(cat *catalog.Catalog, v *vector) error {
	var req struct {
		Entity string `json:"entity"`
	}
	if err := json.Unmarshal(v.Request, &req); err != nil {
		return fmt.Errorf("the request: %v", err)
	}
	for _, tb := range cat.Tables {
		if tb.Name != req.Entity {
			continue
		}
		storage := string(tb.Storage.Kind)
		if tb.Storage.Shape != "" {
			storage += " " + string(tb.Storage.Shape)
		}
		if storage != v.Storage {
			return fmt.Errorf("the vector names the storage %q, and the entity %s is stored as %q", v.Storage, req.Entity, storage)
		}
		return nil
	}
	return fmt.Errorf("the request's entity %q is no table of the catalog", req.Entity)
}

// message builds the request message of the vector with the manifest hash hash, and reports whether its request
// orders by random.
func message(v *vector, hash string) ([]byte, bool, error) {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(v.Request, &req); err != nil {
		return nil, false, fmt.Errorf("the request: %v", err)
	}
	for _, m := range []string{"ir_version", "manifest_hash", "n_params"} {
		if _, ok := req[m]; ok {
			return nil, false, fmt.Errorf("the request names %s, which the runner fills", m)
		}
	}
	var order struct {
		Order []struct {
			Random bool `json:"random"`
		} `json:"order"`
	}
	if err := json.Unmarshal(v.Request, &order); err != nil {
		return nil, false, fmt.Errorf("the request's order: %v", err)
	}
	random := slices.ContainsFunc(order.Order, func(o struct {
		Random bool `json:"random"`
	}) bool {
		return o.Random
	})
	full := map[string]any{"ir_version": irVersion, "manifest_hash": hash, "n_params": len(v.Params)}
	for k, x := range req {
		full[k] = x
	}
	b, err := json.Marshal(map[string]any{"ir_version": irVersion, "manifest_hash": hash, "request": full, "params": v.Params, "now": v.Now})
	return b, random, err
}

// run runs the request message msg in a new session of c, as a request outside a transaction.
func run(c *writepath.Corpus, msg []byte) (*outcome, error) {
	s, err := c.Session("conformance")
	if err != nil {
		return nil, err
	}
	a, err := s.Exec(context.Background(), msg)
	if cerr := s.Close(); cerr != nil {
		return nil, fmt.Errorf("closing the session: %v", cerr)
	}
	var we *writepath.Error
	var ee *irexec.Error
	switch {
	case err == nil:
	case errors.As(err, &we):
		o := &outcome{code: we.Code}
		if we.Diagnostic != nil {
			o.diagnostic = we.Diagnostic.Rule
		}
		return o, nil
	case errors.As(err, &ee):
		o := &outcome{code: ee.Code}
		if ee.Diagnostic != nil {
			o.diagnostic = ee.Diagnostic.Rule
		}
		return o, nil
	default:
		return nil, fmt.Errorf("the request failed with an error of no orm code: %v", err)
	}
	b, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	o := &outcome{members: map[string]json.RawMessage{}}
	if err := json.Unmarshal(b, &o.members); err != nil {
		return nil, err
	}
	return o, nil
}

// expectOutcome compares the outcome o with the expectation e and the files at root with the files from — the corpus
// before the run — changed as e states. It returns the files after the run.
func expectOutcome(root string, e *expectation, o *outcome, from map[string]string) (map[string]string, error) {
	want := map[string]string{}
	for p, t := range from {
		want[p] = t
	}
	if e.Error != nil {
		switch {
		case o.code != *e.Error:
			return nil, fmt.Errorf("the request answers %s, and the vector expects the error %s", o.describe(), *e.Error)
		case e.Diagnostic != nil && o.diagnostic != *e.Diagnostic:
			return nil, fmt.Errorf("the error %s carries the diagnostic %q, and the vector expects %q", o.code, o.diagnostic, *e.Diagnostic)
		}
	} else {
		if o.code != "" {
			return nil, fmt.Errorf("the request answers %s, and the vector expects an answer", o.describe())
		}
		if err := compareMembers(e, o); err != nil {
			return nil, err
		}
		for p, t := range e.Corpus {
			if t == nil {
				delete(want, p)
			} else {
				want[p] = string(*t)
			}
		}
	}
	got, err := tree(root)
	if err != nil {
		return nil, err
	}
	if err := compareTrees(got, want); err != nil {
		return nil, err
	}
	return got, nil
}

func (o *outcome) describe() string {
	if o.code != "" {
		return "the error " + o.code
	}
	return fmt.Sprintf("the answer %v", o.members)
}

// compareMembers compares the answer members of o with those e names: the same members, rows as projections, and the
// others whole.
func compareMembers(e *expectation, o *outcome) error {
	got := slices.Sorted(func(yield func(string) bool) {
		for m := range o.members {
			if !yield(m) {
				return
			}
		}
	})
	want := slices.Sorted(func(yield func(string) bool) {
		for m := range e.members {
			if !yield(m) {
				return
			}
		}
	})
	if !slices.Equal(got, want) {
		return fmt.Errorf("the answer holds the members %v, the vector names %v", got, want)
	}
	for m, raw := range map[string]json.RawMessage{"count": e.Count, "value": e.Value, "affected": e.Affected, "keys": e.Keys} {
		if _, ok := e.members[m]; !ok {
			continue
		}
		a, err := decodeValue(o.members[m])
		if err != nil {
			return err
		}
		b, err := decodeValue(raw)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("the answer's %s is %s, the vector expects %s", m, o.members[m], raw)
		}
	}
	if e.Rows == nil {
		return nil
	}
	var rows []map[string]any
	dec := json.NewDecoder(bytes.NewReader(o.members["rows"]))
	dec.UseNumber()
	if err := dec.Decode(&rows); err != nil {
		return fmt.Errorf("the answer's rows: %v", err)
	}
	expected := *e.Rows
	if len(rows) != len(expected) {
		return fmt.Errorf("the answer holds %d rows, the vector expects %d: %s", len(rows), len(expected), o.members["rows"])
	}
	if !o.random {
		for i, w := range expected {
			if !projects(rows[i], w) {
				return fmt.Errorf("row %d of the answer is %v, the vector expects %v", i, rows[i], w)
			}
		}
		return nil
	}
	used := make([]bool, len(rows))
	for _, w := range expected {
		found := false
		for i, r := range rows {
			if !used[i] && projects(r, w) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			return fmt.Errorf("no row of the answer is %v: %s", w, o.members["rows"])
		}
	}
	return nil
}

// projects reports whether the row r holds every column of want with its value.
func projects(r, want map[string]any) bool {
	for col, x := range want {
		y, ok := r[col]
		if !ok || !reflect.DeepEqual(x, y) {
			return false
		}
	}
	return true
}

func decodeValue(raw json.RawMessage) (any, error) {
	var x any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&x); err != nil {
		return nil, fmt.Errorf("the value %s: %v", raw, err)
	}
	return x, nil
}

// tree reads every file under root but the journal's folder, and requires that folder to hold no stage.
func tree(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".flowmark" {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	stages, err := os.ReadDir(filepath.Join(root, ".flowmark", "tx"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if len(stages) > 0 {
		return nil, fmt.Errorf("the journal holds a stage after the run: %v", stages)
	}
	return out, nil
}

// compareTrees compares the files of the corpus after, got, with the expected files want.
func compareTrees(got, want map[string]string) error {
	for _, p := range slices.Sorted(func(yield func(string) bool) {
		for p := range want {
			if !yield(p) {
				return
			}
		}
	}) {
		g, ok := got[p]
		switch {
		case !ok:
			return fmt.Errorf("the corpus after lacks %s", p)
		case g != want[p]:
			return fmt.Errorf("the corpus after holds %s as\n%s\nand the vector expects\n%s", p, g, want[p])
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			return fmt.Errorf("the corpus after holds %s, which the vector does not name:\n%s", p, got[p])
		}
	}
	return nil
}

// checkAfter validates the corpus after: it holds no diagnostic but the declared ones of the files left unchanged.
func checkAfter(cat *catalog.Catalog, root string, before, after map[string]string, declared map[string][]string) error {
	got, err := diagnostics(cat, root)
	if err != nil {
		return err
	}
	for p, rules := range got {
		allowed := declared[p]
		if after[p] != before[p] {
			allowed = nil
		}
		for _, r := range rules {
			if !slices.Contains(allowed, r) {
				return fmt.Errorf("the corpus after holds the diagnostic %s in %s", r, p)
			}
		}
	}
	return nil
}

// diagnostics validates the corpus at root and returns the rules of its diagnostics by file.
func diagnostics(cat *catalog.Catalog, root string) (map[string][]string, error) {
	diags, err := validate.Corpus(cat, root)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, d := range diags {
		out[d.File] = append(out[d.File], d.Rule)
	}
	for p, rules := range out {
		out[p] = uniqueSorted(rules)
	}
	return out, nil
}

func uniqueSorted(s []string) []string {
	out := slices.Clone(s)
	sort.Strings(out)
	return slices.Compact(out)
}

// family is the storage family a storage belongs to: files, document or block.
func family(storage string) string {
	f, _, _ := strings.Cut(storage, " ")
	return f
}

// checkGroups checks each equivalence group: a files, a document and a block vector, with one request and answer.
func checkGroups(vectors []*vector) error {
	groups := map[string][]*vector{}
	for _, v := range vectors {
		if v.Equivalent != "" {
			groups[v.Equivalent] = append(groups[v.Equivalent], v)
		}
	}
	var errs []error
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for g := range groups {
			if !yield(g) {
				return
			}
		}
	}) {
		g := groups[name]
		families := map[string]bool{}
		for _, v := range g {
			families[family(v.Storage)] = true
		}
		for _, f := range []string{"files", "document", "block"} {
			if !families[f] {
				errs = append(errs, fmt.Errorf("the equivalence group %s holds no %s vector", name, f))
			}
		}
		first := g[0]
		for _, v := range g[1:] {
			if !sameJSON(first.Request, v.Request) || !sameJSON(mustJSON(first.Params), mustJSON(v.Params)) || first.Now != v.Now {
				errs = append(errs, fmt.Errorf("the equivalence group %s: %s and %s differ in their request", name, first.Name, v.Name))
			}
			if !reflect.DeepEqual(first.Expect, v.Expect) {
				errs = append(errs, fmt.Errorf("the equivalence group %s: %s and %s differ in their expected answer", name, first.Name, v.Name))
			}
		}
	}
	return errors.Join(errs...)
}

func mustJSON(x any) []byte {
	b, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	return b
}

// sameJSON reports whether a and b are the same JSON value.
func sameJSON(a, b []byte) bool {
	x, errA := decodeValue(a)
	y, errB := decodeValue(b)
	return errA == nil && errB == nil && reflect.DeepEqual(x, y)
}

// checkCoverage checks that every storage has a read vector and a write vector.
func checkCoverage(vectors []*vector) error {
	reads, writes := map[string]bool{}, map[string]bool{}
	for _, v := range vectors {
		if irexec.IsWrite(mustJSON(map[string]json.RawMessage{"request": v.Request})) {
			writes[v.Storage] = true
		} else {
			reads[v.Storage] = true
		}
	}
	var errs []error
	for _, s := range storages {
		if !reads[s] {
			errs = append(errs, fmt.Errorf("no read vector covers the storage %s", s))
		}
		if !writes[s] {
			errs = append(errs, fmt.Errorf("no write vector covers the storage %s", s))
		}
	}
	return errors.Join(errs...)
}
