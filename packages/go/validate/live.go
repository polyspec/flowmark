package validate

import (
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/internal/pmap"
	"github.com/polyspec/flowmark/packages/go/store"
)

// Live is a corpus read kept across the reads of a long-lived corpus, so that a read reads again only the paths that
// changed since the read before it (plan, Ruling: an operation costs what it touches). Its result is the result of
// Read from nothing, path for path: each folder and document is a unit whose result depends only on what it reads — a
// folder its entries, a document its bytes — and on the link targets of the keys its parse asked for. A read takes
// the paths the corpus tree dropped since the last one: a folder whose entries changed builds the units of its new and
// changed entries and removes those of the entries gone; a document whose bytes changed is parsed again in both
// passes. The first pass indexes the keys of every row; a key whose target changes parses again, in the second pass,
// the documents whose links asked for it, which a reverse index names. Duplicate keys and the unreadable files that
// make a table unavailable are kept by table and by diagnostic, so the availability is read without the rows.
//
// A Live is used by one read at a time, under the corpus lock. A Snapshot it returns is never changed by a later read.
type Live struct {
	cat  *catalog.Catalog
	cost *cost.Counts
	// built is whether the units hold a read of the corpus; a read that fails leaves it false, and the next read
	// reads the whole corpus.
	built bool
	units pmap.Map[string, *unit]
	// idx holds the first-pass targets of each key (indexKey), by the path that holds them; refs the paths whose
	// second-pass parse asked for each key.
	idx  map[string]map[string][]target
	refs map[string]map[string]bool
	// keys holds the second-pass rows of each table by key, the duplicates of each table, and blocks the diagnostics
	// of unreadable files and walk entries, ordered as sortDiags orders them.
	keys   map[string]map[string][]occurrence
	dups   map[string]map[string]bool
	blocks pmap.Map[string, catalog.Diagnostic]
	files  pmap.Map[string, *File]
	diags  pmap.Map[string, []catalog.Diagnostic]
	// pending holds the documents to parse in the first pass, and touched the keys whose targets a read changed, with
	// their answer before it, and changed the paths whose file or diagnostics the read changed.
	pending map[string]bool
	touched map[string]answer
	changed map[string]bool
	// fresh holds the units this read built.
	fresh map[string]bool
}

// unitKind is what a unit of a Live read is.
type unitKind int

const (
	unitRoot     unitKind = iota // the corpus root
	unitFiles                    // a folder of a files table
	unitShard                    // the folder of a sharded document table
	unitOutside                  // a folder outside every table place
	unitRow                      // a document of a files table
	unitDocument                 // a document of a document table
	unitLeaf                     // an entry whose read is a walk diagnostic
)

// spec is what the entry of a unit in its folder makes of it: its kind, table and depth, the type of the entry, and
// the reason of a leaf. Two equal specs build the same unit from the same disk.
type spec struct {
	kind   unitKind
	table  string
	depth  int
	mode   fs.FileMode
	reason string
}

// unit is a folder or a document of the corpus as the read made it.
type unit struct {
	spec
	// children holds the spec of each entry of a folder that is a unit.
	children map[string]spec
	data     []byte
	read     bool
	first    []*store.Row
	rows     []*store.Row
	diags    []catalog.Diagnostic
	asked    []string
}

// occurrence is a second-pass row of a key, where corpus order places it.
type occurrence struct {
	file string
	row  *store.Row
}

// answer is what the second pass's linker answers for a key: the target of its one row, shared, or none.
type answer struct {
	t      target
	shared bool
	found  bool
}

// NewLive returns a Live of the catalog cat that has read nothing, counting its work in k (nil counts nothing).
func NewLive(cat *catalog.Catalog, k *cost.Counts) *Live {
	return &Live{cat: cat, cost: k}
}

// Reset makes the next read read the whole corpus: the caller holds no corpus of the last read to read on from.
func (l *Live) Reset() { l.built = false }

// Read reads the corpus fsys again: the paths changed, which the corpus tree dropped since the read before, or the
// whole corpus when all is true or no read built the units. It returns the read as a Snapshot, which names the paths
// whose file or diagnostics changed since the Snapshot before. An error is returned only when the corpus root cannot
// be read, or a document's parse fails as Read fails; the next read then reads the whole corpus.
func (l *Live) Read(fsys store.FS, changed []string, all bool) (*Snapshot, error) {
	l.pending, l.touched, l.changed, l.fresh = map[string]bool{}, map[string]answer{}, map[string]bool{}, map[string]bool{}
	full := all || !l.built
	l.built = false
	if full {
		l.units = pmap.Map[string, *unit]{}
		l.idx, l.refs = map[string]map[string][]target{}, map[string]map[string]bool{}
		l.keys, l.dups = map[string]map[string][]occurrence{}, map[string]map[string]bool{}
		l.blocks, l.files, l.diags = pmap.Map[string, catalog.Diagnostic]{}, pmap.Map[string, *File]{}, pmap.Map[string, []catalog.Diagnostic]{}
		if err := l.build(fsys, ".", spec{kind: unitRoot}); err != nil {
			return nil, err
		}
	} else if err := l.update(fsys, changed); err != nil {
		return nil, err
	}
	if err := l.parse(); err != nil {
		return nil, err
	}
	l.built = true
	s := &Snapshot{files: l.files, diags: l.diags, unavailable: map[string]catalog.Diagnostic{}, full: full, changed: slices.Sorted(maps.Keys(l.changed))}
	for _, tb := range l.cat.Tables {
		if d, ok := l.unavailableBy(tb.Name); ok {
			s.unavailable[tb.Name] = d
		}
	}
	return s, nil
}

// update reads again the units of the changed paths; a unit that this read built is already read.
func (l *Live) update(fsys store.FS, changed []string) error {
	for _, p := range changed {
		u, ok := l.units.Get(p)
		if !ok || l.fresh[p] {
			continue
		}
		switch u.kind {
		case unitRoot, unitFiles, unitShard, unitOutside:
			if err := l.relist(fsys, p, u); err != nil {
				return err
			}
		case unitRow, unitDocument:
			l.reread(fsys, p, u)
		}
	}
	return nil
}

// child returns the corpus path of the entry name of the folder dir.
func child(dir, name string) string {
	if dir == "." {
		return name
	}
	return dir + "/" + name
}

// build builds the unit of path from its spec, and the units under it, reading the disk through fsys.
func (l *Live) build(fsys store.FS, path string, sp spec) error {
	u := &unit{spec: sp}
	l.units = l.units.Set(path, u)
	l.fresh[path] = true
	switch sp.kind {
	case unitLeaf:
		l.setDiags(path, u, []catalog.Diagnostic{store.Diag(path, store.RuleWalk, sp.reason)})
	case unitRow, unitDocument:
		l.reread(fsys, path, u)
	default:
		entries, err := fsys.ReadDir(path)
		if err != nil {
			if sp.kind == unitRoot {
				return fmt.Errorf("cannot read the corpus root: %w", err)
			}
			u.children = map[string]spec{}
			l.setDiags(path, u, []catalog.Diagnostic{store.Diag(path, store.RuleWalk, store.FolderFailure(err))})
			return nil
		}
		u.children = l.specs(path, sp, entries)
		for _, name := range slices.Sorted(maps.Keys(u.children)) {
			if err := l.build(fsys, child(path, name), u.children[name]); err != nil {
				return err
			}
		}
	}
	return nil
}

// specs returns the spec of each entry of the folder path, of spec sp, that is a unit.
func (l *Live) specs(path string, sp spec, entries []fs.DirEntry) map[string]spec {
	out := map[string]spec{}
	for _, e := range entries {
		if c, ok := l.entrySpec(path, sp, e); ok {
			c.mode = e.Type()
			out[e.Name()] = c
		}
	}
	return out
}

// entrySpec returns the spec of the entry e of the folder path, of spec sp, and false when e is no unit: the same
// classification as the store's walk of a table folder and the validator's walk of the root and of the folders outside
// the places.
func (l *Live) entrySpec(path string, sp spec, e fs.DirEntry) (spec, bool) {
	var en store.Entry
	switch sp.kind {
	case unitRoot:
		return l.rootSpec(e)
	case unitFiles:
		tb, _ := l.cat.Table(sp.table)
		en = store.FilesEntry(tb.Storage.Shard, e, sp.depth)
		if en.Kind == store.EntryRow {
			return spec{kind: unitRow, table: sp.table}, true
		}
		if en.Kind == store.EntryFolder {
			return spec{kind: unitFiles, table: sp.table, depth: sp.depth + 1}, true
		}
	case unitShard:
		tb, _ := l.cat.Table(sp.table)
		en = store.DocumentEntry(tb.Storage.Shard, e)
		if en.Kind == store.EntryRow {
			return spec{kind: unitDocument, table: sp.table}, true
		}
	case unitOutside:
		en = outsideEntry(e)
		if en.Kind == store.EntryFolder {
			return spec{kind: unitOutside}, true
		}
	}
	if en.Kind == store.EntryDiag {
		return spec{kind: unitLeaf, reason: en.Reason}, true
	}
	return spec{}, false
}

// rootSpec returns the spec of the entry e of the corpus root: the folder of a files or sharded table, the document of
// a document table, or what the walk outside the places makes of it.
func (l *Live) rootSpec(e fs.DirEntry) (spec, bool) {
	name := e.Name()
	for _, tb := range l.cat.Tables {
		switch {
		case tb.Name == name && tb.Storage.Kind == catalog.StorageFiles:
			if err := store.CheckFilesTable(l.cat, tb.Name); err != nil {
				return spec{kind: unitLeaf, reason: err.Error()}, true
			}
			if reason := store.TableFolderFault(e, store.FilesTableWhat); reason != "" {
				return spec{kind: unitLeaf, reason: reason}, true
			}
			return spec{kind: unitFiles, table: tb.Name}, true
		case tb.Name == name && folderTable(tb):
			if reason := store.TableFolderFault(e, store.DocumentTableWhat); reason != "" {
				return spec{kind: unitLeaf, reason: reason}, true
			}
			return spec{kind: unitShard, table: tb.Name}, true
		case fileTable(tb) && name == tb.Name+docSuffix && e.Type().IsRegular():
			return spec{kind: unitDocument, table: tb.Name}, true
		}
	}
	switch en := rootEntry(l.cat, e); en.Kind {
	case store.EntryDiag:
		return spec{kind: unitLeaf, reason: en.Reason}, true
	case store.EntryFolder:
		return spec{kind: unitOutside}, true
	}
	return spec{}, false
}

// relist reads the entries of the folder unit u at path again: an entry gone, or whose spec changed, removes its units,
// and an entry new, or whose spec changed, builds them.
func (l *Live) relist(fsys store.FS, path string, u *unit) error {
	entries, err := fsys.ReadDir(path)
	next := map[string]spec{}
	var own []catalog.Diagnostic
	switch {
	case err != nil && u.kind == unitRoot:
		return fmt.Errorf("cannot read the corpus root: %w", err)
	case err != nil:
		own = []catalog.Diagnostic{store.Diag(path, store.RuleWalk, store.FolderFailure(err))}
	default:
		next = l.specs(path, u.spec, entries)
	}
	for _, name := range slices.Sorted(maps.Keys(u.children)) {
		if old := u.children[name]; next[name] != old {
			l.remove(child(path, name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(next)) {
		if old, ok := u.children[name]; !ok || old != next[name] {
			if err := l.build(fsys, child(path, name), next[name]); err != nil {
				return err
			}
		}
	}
	u.children = next
	l.setDiags(path, u, own)
	return nil
}

// remove removes the unit at path and every unit under it, with what each placed.
func (l *Live) remove(path string) {
	var gone []string
	if _, ok := l.units.Get(path); ok {
		gone = append(gone, path)
	}
	for p := range l.units.From(path + "/") {
		if !strings.HasPrefix(p, path+"/") {
			break
		}
		gone = append(gone, p)
	}
	for _, p := range gone {
		u, _ := l.units.Get(p)
		l.setFirst(p, u, nil)
		l.setSecond(p, u, nil, nil)
		l.setDiags(p, u, nil)
		l.setFile(p, nil)
		delete(l.pending, p)
		l.units = l.units.Delete(p)
	}
}

// reread reads the bytes of the document unit u at path again; bytes that differ from the ones read parse it again.
func (l *Live) reread(fsys store.FS, path string, u *unit) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		if u.read || u.diags == nil {
			u.read, u.data = false, nil
			l.setFirst(path, u, nil)
			l.setSecond(path, u, nil, nil)
			l.setFile(path, nil)
			delete(l.pending, path)
		}
		l.setDiags(path, u, []catalog.Diagnostic{store.Diag(path, store.RuleWalk, store.ReadFailure(err))})
		return
	}
	if u.read && string(u.data) == string(data) {
		return
	}
	u.read, u.data = true, data
	l.cost.Add(cost.FilesVisited, 1)
	l.pending[path] = true
}

// parse parses the pending documents in the first pass, updates the index of their keys, and parses in the second
// pass the pending documents and those whose links asked for a key whose target changed.
func (l *Live) parse() error {
	pending := slices.Sorted(maps.Keys(l.pending))
	for _, p := range pending {
		u, _ := l.units.Get(p)
		rows, err := l.parseOne(p, u, noTargets{})
		if err != nil {
			return err
		}
		l.setFirst(p, u, rows.rows)
	}
	second := map[string]bool{}
	for _, p := range pending {
		second[p] = true
	}
	for k, before := range l.touched {
		if l.answer(k) != before {
			for p := range l.refs[k] {
				second[p] = true
			}
		}
	}
	for _, p := range slices.Sorted(maps.Keys(second)) {
		u, ok := l.units.Get(p)
		if !ok || !u.read {
			continue
		}
		rec := &asking{l: l}
		res, err := l.parseOne(p, u, rec)
		if err != nil {
			return err
		}
		l.setSecond(p, u, res.rows, rec.asked)
		l.setDiags(p, u, res.diags)
		l.setFile(p, &File{Table: u.table, Data: u.data, Rows: res.rows})
	}
	return nil
}

// parsed is the parse of a document: its top-level rows and its diagnostics.
type parsed struct {
	rows  []*store.Row
	diags []catalog.Diagnostic
}

// parseOne parses the document unit u at path with the linker lk.
func (l *Live) parseOne(path string, u *unit, lk store.Linker) (parsed, error) {
	l.cost.Add(cost.FilesParsed, 1)
	if u.kind == unitRow {
		row, diags := store.ParseFile(l.cat, u.table, path, u.data, lk)
		var rows []*store.Row
		if row != nil {
			rows = []*store.Row{row}
		}
		return parsed{rows, diags}, nil
	}
	rows, diags, err := store.ParseDocument(l.cat, u.table, path, u.data, lk)
	return parsed{rows, diags}, err
}

// asking is the linker of the second pass: it answers from the index of the first and records each key it is asked.
type asking struct {
	l     *Live
	asked []string
}

func (a *asking) Target(fromFile, table, key string) (string, error) {
	k := indexKey(table, key)
	a.asked = append(a.asked, k)
	ans := a.l.answer(k)
	switch {
	case !ans.found:
		return "", fmt.Errorf("no %s row has the key %s", table, key)
	case ans.shared:
		return "", store.ErrKeyShared
	}
	return store.RelLink(fromFile, ans.t.file, ans.t.anchor)
}

// answer returns the target of the key k: its one row's, shared when two or more rows hold it, or none.
func (l *Live) answer(k string) answer {
	n := 0
	var t target
	for _, ts := range l.idx[k] {
		n += len(ts)
		if len(ts) > 0 {
			t = ts[0]
		}
	}
	switch n {
	case 0:
		return answer{}
	case 1:
		return answer{t: t, found: true}
	}
	return answer{shared: true, found: true}
}

// setFirst replaces the first-pass rows of the unit u at path by rows, updating the index of their keys and recording
// the answer of each key before its first change of the read.
func (l *Live) setFirst(path string, u *unit, rows []*store.Row) {
	change := func(r *store.Row, add bool) {
		k := indexKey(r.Table, r.Key)
		if _, seen := l.touched[k]; !seen {
			l.touched[k] = l.answer(k)
		}
		if !add {
			delete(l.idx[k], path)
			if len(l.idx[k]) == 0 {
				delete(l.idx, k)
			}
			return
		}
		t := target{file: path}
		if has, err := store.HasDetail(l.cat, r.Table, r); err == nil && has {
			t.anchor = store.DetailAnchor(r.Table, r.Key)
		}
		if l.idx[k] == nil {
			l.idx[k] = map[string][]target{}
		}
		l.idx[k][path] = append(l.idx[k][path], t)
	}
	walkRows(u.first, func(r *store.Row) { change(r, false) })
	u.first = rows
	walkRows(rows, func(r *store.Row) {
		l.cost.Add(cost.RowsPlaced, 1)
		change(r, true)
	})
}

// setSecond replaces the second-pass rows of the unit u at path by rows and the keys its parse asked for by asked,
// updating the rows of each table by key and the reverse index of the links.
func (l *Live) setSecond(path string, u *unit, rows []*store.Row, asked []string) {
	walkRows(u.rows, func(r *store.Row) { l.occur(path, r, false) })
	for _, k := range u.asked {
		delete(l.refs[k], path)
		if len(l.refs[k]) == 0 {
			delete(l.refs, k)
		}
	}
	u.rows, u.asked = rows, asked
	walkRows(rows, func(r *store.Row) {
		l.cost.Add(cost.RowsPlaced, 1)
		l.occur(path, r, true)
	})
	for _, k := range asked {
		if l.refs[k] == nil {
			l.refs[k] = map[string]bool{}
		}
		l.refs[k][path] = true
	}
}

// occur adds or removes the second-pass row r of the file path among the rows of its table by key, and keeps the
// duplicates of the table.
func (l *Live) occur(path string, r *store.Row, add bool) {
	byKey := l.keys[r.Table]
	if byKey == nil {
		byKey = map[string][]occurrence{}
		l.keys[r.Table] = byKey
	}
	if add {
		byKey[r.Key] = append(byKey[r.Key], occurrence{file: path, row: r})
	} else {
		byKey[r.Key] = slices.DeleteFunc(byKey[r.Key], func(o occurrence) bool { return o.row == r })
		if len(byKey[r.Key]) == 0 {
			delete(byKey, r.Key)
		}
	}
	if l.dups[r.Table] == nil {
		l.dups[r.Table] = map[string]bool{}
	}
	if len(byKey[r.Key]) > 1 {
		l.dups[r.Table][r.Key] = true
	} else {
		delete(l.dups[r.Table], r.Key)
	}
}

// setDiags replaces the diagnostics of the unit u at path, every one of which is at path, by diags.
func (l *Live) setDiags(path string, u *unit, diags []catalog.Diagnostic) {
	if slices.Equal(u.diags, diags) && (u.diags == nil) == (diags == nil) {
		return
	}
	for _, d := range u.diags {
		if blocksRule(d.Rule) {
			l.blocks = l.blocks.Delete(diagKey(d))
		}
	}
	u.diags = diags
	sorted := slices.Clone(diags)
	sortDiags(sorted)
	for _, d := range sorted {
		if d.File != path {
			panic(fmt.Sprintf("validate: the unit %s reports a diagnostic at %s", path, d.File))
		}
		if blocksRule(d.Rule) {
			l.blocks = l.blocks.Set(diagKey(d), d)
		}
	}
	if len(sorted) == 0 {
		l.diags = l.diags.Delete(path)
	} else {
		l.diags = l.diags.Set(path, sorted)
	}
	l.changed[path] = true
}

// setFile makes f the file of the corpus at path, nil for none.
func (l *Live) setFile(path string, f *File) {
	if f == nil {
		if _, ok := l.files.Get(path); !ok {
			return
		}
		l.files = l.files.Delete(path)
	} else {
		l.files = l.files.Set(path, f)
	}
	l.changed[path] = true
}

// diagKey orders diagnostics as sortDiags does: by file, line, column, rule and message.
func diagKey(d catalog.Diagnostic) string {
	return fmt.Sprintf("%s\x00%010d\x00%010d\x00%s\x00%s", d.File, d.Line, d.Column, d.Rule, d.Message)
}

// walkRows calls f with each row of rows and each of their block rows, in storage order.
func walkRows(rows []*store.Row, f func(*store.Row)) {
	for _, r := range rows {
		f(r)
		for _, name := range r.BlockNames() {
			walkRows(r.Blocks[name], f)
		}
	}
}

// unavailableBy returns the diagnostic that makes table unavailable, as corpus.unavailableBy does over a full read:
// the first unreadable file or walk entry at its place or its placement parent's, in diagnostic order, and otherwise
// the second row of a byte-for-byte equal key, in corpus order.
func (l *Live) unavailableBy(table string) (catalog.Diagnostic, bool) {
	for _, d := range l.blocks.All() {
		if t, ok := tableAt(l.cat, d.File); ok && holds(l.cat, table, t) {
			return d, true
		}
	}
	tb, ok := l.cat.Table(table)
	if !ok {
		return catalog.Diagnostic{}, false
	}
	var best, bestFirst occurrence
	found := false
	for key := range l.dups[table] {
		occ := l.keys[table][key]
		ordered := make([]placed, len(occ))
		for i, o := range occ {
			ordered[i] = placed{row: o.row, file: o.file}
		}
		ordered = corpusOrder(ordered)
		second := occurrence{file: ordered[1].file, row: ordered[1].row}
		if !found || before(second, best) {
			best, bestFirst, found = second, occurrence{file: ordered[0].file, row: ordered[0].row}, true
		}
	}
	if !found {
		return catalog.Diagnostic{}, false
	}
	line, col := best.row.At(tb.Key)
	return catalog.Diagnostic{File: best.file, Line: line, Column: col, Rule: ruleDuplicate,
		Message: fmt.Sprintf("the key %s equals the key %s of %s byte for byte", best.row.Key, bestFirst.row.Key, bestFirst.file)}, true
}

// before reports whether a stands before b in corpus order.
func before(a, b occurrence) bool {
	if a.file != b.file {
		return a.file < b.file
	}
	al, ac := a.row.At("")
	bl, bc := b.row.At("")
	if al != bl {
		return al < bl
	}
	return ac < bc
}
