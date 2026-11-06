package journal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
)

// txRel is the journal folder of spec 40: a commit stages its files in `.flowmark/tx/`, each named after the stage, and
// it holds the files of at most one stage.
const txRel = ".flowmark/tx"

// The files of a stage `<id>`, 32 lowercase hex digits, are `<id>.<name>`: the prepare record `<id>.prepare`, written to
// `<id>.prepare.part` and renamed into place; the source of rename n, `<id>.<n>.<hash>`, a copy of its post-image, so
// two identical targets are two files and no target shares a file with the stage; a delete moves its target to
// `<id>.<n>.deleted`; a folder created in the stage is `<id>.<n>.dir.<k>`; and a recovery refusal writes each recorded
// state as `<id>.<hash>`.
const (
	prepareName = "prepare"
	partSuffix  = ".part"
)

// stageFileName matches the name of a file of a stage, its stage the first 32 hex digits.
var stageFileName = regexp.MustCompile(`^[0-9a-f]{32}\.(prepare|prepare\.part|[0-9]+\.[0-9a-f]{64}|[0-9]+\.deleted|[0-9]+\.dir\.[0-9]+|[0-9a-f]{64})$`)

func copyName(n int, hash string) string { return strconv.Itoa(n) + "." + hash }
func deletedName(n int) string           { return strconv.Itoa(n) + ".deleted" }
func folderName(n, k int) string         { return fmt.Sprintf("%d.dir.%d", n, k) }

// stageFile is the file name of the stage at stage, `.flowmark/tx/<id>`.
func stageFile(stage, name string) string { return stage + "." + name }

// Change is one target write of a commit: the full bytes of Path, or its delete. A target that does not exist is
// created, with any folder of it that does not exist.
type Change struct {
	Path   string
	Data   []byte
	Delete bool
}

// rename is one entry of the prepare record; an empty hash is an absent file: a create has an empty pre-hash, and a
// delete an empty post-hash.
type rename struct {
	Path string `json:"path"`
	Pre  string `json:"pre"`
	Post string `json:"post"`
}

// prepareRecord decides a commit: the generation it takes, its renames, and the bytes of every recorded state by its
// hash, so the record alone completes the commit and resolves a refusal.
type prepareRecord struct {
	Generation uint64            `json:"generation"`
	Renames    []rename          `json:"renames"`
	Blobs      map[string][]byte `json:"blobs"`
}

// RecoveryRefusal: a target's current hash matches neither recorded hash — a change landed while the commit was
// interrupted. Nothing of the stage is applied and the stage stays, with both recorded states written beside its
// record; every open refuses the same way until the target holds one of its recorded states.
type RecoveryRefusal struct {
	Stage, Path, Pre, Post, Current string
}

// state names a recorded state and the stage file that holds it.
func (e *RecoveryRefusal) state(h string) string {
	if h == "" {
		return "absent (remove the file)"
	}
	return h + " in " + stageFile(e.Stage, h)
}

func (e *RecoveryRefusal) Error() string {
	current := e.Current
	if current == "" {
		current = "absent"
	}
	return fmt.Sprintf("journal: recovery refuses %s: its content %s matches neither recorded state — before: %s; after: %s. A change made while the commit was interrupted is never overwritten: copy it aside, copy one recorded state over %s, and open the corpus again, which completes the commit; then make the change again",
		e.Path, current, e.state(e.Pre), e.state(e.Post), e.Path)
}

// CommitError is a commit that stopped. Prepared says whether its prepare record is in the journal: before it the
// commit wrote no target and recovery discards its stage; from it on the commit is decided, and recovery completes it
// (spec 40 The commit protocol). Err is the cause.
type CommitError struct {
	Prepared bool
	Err      error
}

func (e *CommitError) Error() string {
	if e.Prepared {
		return e.Err.Error() + " (the prepare record is written: the commit is decided, and recovery completes it)"
	}
	return e.Err.Error() + " (before the prepare record: no target was written, and recovery discards the stage)"
}

func (e *CommitError) Unwrap() error { return e.Err }

func (c *Corpus) txDir() string { return c.abs(txRel) }

// sum is the SHA-256 of raw bytes as 64 lowercase hex digits (raw-file-bytes hashing).
func sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hashFile(p string) (string, error) {
	f, err := openShared(p, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, cerr := io.Copy(h, f)
	if err := errors.Join(cerr, f.Close()); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// isHash accepts an empty hash (an absent file) or 64 lowercase hex digits.
func isHash(h string) bool { return h == "" || isHex(h, 64) }

// checkTarget accepts a target path: clean, inside the root and outside `.flowmark/`.
func checkTarget(rel string) error {
	if err := checkRel(rel); err != nil {
		return err
	}
	if rel == ".flowmark" || strings.HasPrefix(rel, ".flowmark/") {
		return fmt.Errorf("the path %q is inside .flowmark/, which holds no corpus file", rel)
	}
	return nil
}

// readTarget returns the current bytes of a target and their hash, or an empty hash when it or a folder of it is
// absent.
func (c *Corpus) readTarget(rel string) (string, []byte, error) {
	info, err := c.lstat(rel)
	if err != nil || info == nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("%s is not a regular file", rel)
	}
	data, _, err := readShared(c.abs(rel))
	if err != nil {
		return "", nil, err
	}
	return sum(data), data, nil
}

// currentHash returns the hash of a target, or an empty hash when it is absent.
func (c *Corpus) currentHash(rel string) (string, error) {
	info, err := c.lstat(rel)
	if err != nil || info == nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", rel)
	}
	return hashFile(c.abs(rel))
}

// writeDurable writes a stage file to its partial name, syncs it and renames it into place.
func writeDurable(k *cost.Counts, dir, name string, data []byte) error {
	part := filepath.Join(dir, name+partSuffix)
	if err := writeSynced(k, part, data, os.O_TRUNC); err != nil {
		return fmt.Errorf("journal: writing %s: %w", name, err)
	}
	if err := moveDurable(k, part, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("journal: placing %s: %w", name, err)
	}
	return nil
}

// Commit runs steps 3 to 7 of the spec 40 commit protocol for changes, under l: a synced copy of each post-image; the
// prepare record, synced with its folder, which decides the commit; the renames, each folder of a target synced once;
// the generation; and the collection of the stage. The caller has run steps 1 and 2. A target folder, or a folder to
// create, on another file system than the journal refuses before anything is staged. A stage left by an interrupted
// commit refuses a new commit until Recover has run. A commit that stops returns a *CommitError saying whether its
// prepare record is written.
func (c *Corpus) Commit(l *Lock, changes []Change) error {
	prepared := false
	if err := c.commit(l, changes, &prepared); err != nil {
		return &CommitError{Prepared: prepared, Err: err}
	}
	return nil
}

func (c *Corpus) commit(l *Lock, changes []Change, prepared *bool) error {
	if err := c.checkHeld(l); err != nil {
		return err
	}
	renames := make([]rename, len(changes))
	blobs := map[string][]byte{}
	seen := map[string]bool{}
	for i, ch := range changes {
		if err := checkTarget(ch.Path); err != nil {
			return fmt.Errorf("journal: change %d: %w", i, err)
		}
		if seen[ch.Path] {
			return fmt.Errorf("journal: %s is changed twice in one commit", ch.Path)
		}
		seen[ch.Path] = true
		if ch.Delete && ch.Data != nil {
			return fmt.Errorf("journal: the delete of %s carries bytes", ch.Path)
		}
		pre, data, err := c.readTarget(ch.Path)
		if err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		if ch.Delete && pre == "" {
			return fmt.Errorf("journal: %s is deleted but does not exist", ch.Path)
		}
		r := rename{Path: ch.Path, Pre: pre}
		if !ch.Delete {
			r.Post = sum(ch.Data)
			blobs[r.Post] = ch.Data
		}
		renames[i] = r
		if pre != "" {
			blobs[pre] = data
		}
	}
	if err := c.ensureDir(txRel); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	// The generation the commit takes is the one after the corpus's, read before anything is staged; one that cannot
	// be read refuses the commit here.
	g, err := c.Generation()
	if err != nil {
		return err
	}
	if err := c.checkDevices(renames); err != nil {
		return err
	}
	left, err := os.ReadDir(c.txDir())
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if len(left) > 0 {
		return fmt.Errorf("journal: %s holds an interrupted commit; recover the corpus before a new commit", c.txDir())
	}
	stage := filepath.Join(c.txDir(), randomHex(16))
	// Step 3: the copy that is the source of each rename, synced; the folder sync of the record makes its name durable,
	// and a recovery writes a missing one again from the record.
	for n, r := range renames {
		if r.Post == "" {
			continue
		}
		if err := writeSynced(c.cost, stageFile(stage, copyName(n, r.Post)), blobs[r.Post], os.O_TRUNC); err != nil {
			return fmt.Errorf("journal: writing the copy of %s: %w", r.Path, err)
		}
	}
	if err := c.step("post-images", 0); err != nil {
		return err
	}
	// Step 4: the prepare record, which decides the commit.
	rec := prepareRecord{Generation: g.N + 1, Renames: renames, Blobs: blobs}
	if err := c.writeRecord(stage, rec); err != nil {
		// A record renamed into place before its folder sync failed is in the journal, and recovery completes it.
		exists, serr := regularExists(stageFile(stage, prepareName))
		*prepared = exists
		return errors.Join(err, serr)
	}
	*prepared = true
	c.obs.Observe(Event{Kind: Prepared, Path: stage})
	if err := c.step("prepare", 0); err != nil {
		return err
	}
	return c.complete(stage, rec)
}

// writeRecord writes the prepare record of the stage at stage to its partial name, syncs it, renames it into place and
// syncs the folder of the stages, so the copies and the record are durable.
func (c *Corpus) writeRecord(stage string, rec prepareRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	part := stageFile(stage, prepareName+partSuffix)
	if err := writeSynced(c.cost, part, append(data, '\n'), os.O_TRUNC); err != nil {
		return fmt.Errorf("journal: writing the prepare record: %w", err)
	}
	if err := moveName(c.cost, part, stageFile(stage, prepareName)); err != nil {
		return fmt.Errorf("journal: placing the prepare record: %w", err)
	}
	if err := syncFolder(c.cost, c.txDir()); err != nil {
		return fmt.Errorf("journal: the folder of the prepare record: %w", err)
	}
	return nil
}

// checkDevices refuses a target folder, or the folder a missing one would be created in, on another file system than
// `.flowmark/tx/` (REVIEW 12): a mount point inside the corpus is unsupported.
func (c *Corpus) checkDevices(renames []rename) error {
	tx, err := deviceOf(c.txDir())
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	for _, r := range renames {
		folder, err := c.nearestFolder(path.Dir(r.Path))
		if err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		dev, err := deviceOf(folder)
		if err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		if dev != tx {
			return fmt.Errorf("journal: the folder %s of %s is on another file system than %s: a mount point inside the corpus is unsupported", folder, r.Path, c.txDir())
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// complete runs steps 5 to 7 for a prepared stage; it is the commit's own path and the replay's, and running it again
// on the same stage is idempotent. Every target is checked before any is touched: one that holds its post-image is
// done, one that holds its pre-image is pending, and any other refuses the whole stage, its recorded states written
// beside the record for the person who resolves it.
func (c *Corpus) complete(stage string, rec prepareRecord) error {
	var pending []int
	for n, r := range rec.Renames {
		cur, err := c.currentHash(r.Path)
		if err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		switch cur {
		case r.Post:
		case r.Pre:
			pending = append(pending, n)
		default:
			refusal := &RecoveryRefusal{Stage: stage, Path: r.Path, Pre: r.Pre, Post: r.Post, Current: cur}
			return errors.Join(refusal, writeStates(stage, rec, r))
		}
	}
	// Every rename source is checked against its hash before any target is touched; a missing copy is written again
	// from the record.
	for _, n := range pending {
		if r := rec.Renames[n]; r.Post != "" {
			if err := c.ensureCopy(stage, n, r, rec.Blobs[r.Post]); err != nil {
				return err
			}
		}
	}
	// Step 5: the renames; a delete is a rename into the stage. Each folder that gained or lost a name is synced once,
	// after all of them.
	folders := map[string]bool{}
	for _, n := range pending {
		r := rec.Renames[n]
		target := c.abs(r.Path)
		var err error
		if r.Post == "" {
			err = moveName(c.cost, target, stageFile(stage, deletedName(n)))
		} else {
			if r.Pre == "" {
				if err := c.makeFolders(stage, n, path.Dir(r.Path), folders); err != nil {
					return err
				}
			}
			err = moveName(c.cost, stageFile(stage, copyName(n, r.Post)), target)
		}
		if err != nil {
			return fmt.Errorf("journal: applying %s: %w", r.Path, err)
		}
		folders[filepath.Dir(target)] = true
		if err := c.step("rename", n); err != nil {
			return err
		}
	}
	for _, dir := range sortedKeys(folders) {
		if err := syncFolder(c.cost, dir); err != nil {
			return fmt.Errorf("journal: %w", err)
		}
	}
	c.obs.Observe(Event{Kind: Applied, Path: stage})
	if err := c.step("renames", 0); err != nil {
		return err
	}
	// Step 6: the generation, which says the renames are done.
	if err := c.publishGeneration(rec.Generation, rec.Renames); err != nil {
		return err
	}
	if err := c.step("generation", 0); err != nil {
		return err
	}
	return c.collect(stage)
}

// writeStates writes the recorded states of the rename r beside the prepare record of the stage, as `<id>.<hash>`, for
// the refusal that names them.
func writeStates(stage string, rec prepareRecord, r rename) error {
	var errs []error
	for _, h := range []string{r.Pre, r.Post} {
		if h == "" {
			continue
		}
		if err := os.WriteFile(stageFile(stage, h), rec.Blobs[h], 0o644); err != nil {
			errs = append(errs, fmt.Errorf("journal: writing the recorded state %s of %s: %w", h, r.Path, err))
		}
	}
	return errors.Join(errs...)
}

// ensureCopy makes sure the source of rename n holds its post-image blob, writing it again when absent.
func (c *Corpus) ensureCopy(stage string, n int, r rename, blob []byte) error {
	src := stageFile(stage, copyName(n, r.Post))
	_, err := os.Lstat(src)
	if errors.Is(err, fs.ErrNotExist) {
		if err := writeSynced(c.cost, src, blob, os.O_TRUNC); err != nil {
			return fmt.Errorf("journal: writing the copy of %s again: %w", r.Path, err)
		}
	} else if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	got, err := hashFile(src)
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if got != r.Post {
		return fmt.Errorf("journal: the stage file %s for %s holds bytes of hash %s, not %s; the stage %s is damaged and nothing of it is applied", filepath.Base(src), r.Path, got, r.Post, stage)
	}
	return nil
}

// makeFolders creates the missing folders of dir for the create of rename n, from the corpus root, following no
// symlink, and adds the folder each is created in to folders, which the commit syncs. Where folders are staged
// (Windows), each is made in the stage as `<id>.<n>.dir.<k>` and moved into place; a replay skips a folder in place and
// moves one it finds staged.
func (c *Corpus) makeFolders(stage string, n int, dir string, folders map[string]bool) error {
	if dir == "." {
		return nil
	}
	cur := c.root
	made := false
	for k, part := range strings.Split(dir, "/") {
		parent := cur
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		switch {
		case err == nil && fi.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("journal: %w", symlinkError(cur))
		case err == nil && !fi.IsDir():
			return fmt.Errorf("journal: %s is not a folder", cur)
		case err == nil:
			continue
		case !errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("journal: %w", err)
		}
		if c.stageFolders {
			staged := stageFile(stage, folderName(n, k))
			_, err := os.Lstat(staged)
			if errors.Is(err, fs.ErrNotExist) {
				if err := os.Mkdir(staged, 0o755); err != nil {
					return fmt.Errorf("journal: %w", err)
				}
				if err := c.step("folder-staged", n); err != nil {
					return err
				}
			} else if err != nil {
				return fmt.Errorf("journal: %w", err)
			}
			if err := moveFolderName(c.cost, staged, cur); err != nil {
				return fmt.Errorf("journal: creating the folder %s: %w", cur, err)
			}
		} else if err := os.Mkdir(cur, 0o755); err != nil {
			return fmt.Errorf("journal: creating the folder %s: %w", cur, err)
		}
		folders[parent] = true
		made = true
	}
	if made {
		return c.step("folder", n)
	}
	return nil
}

// collect runs step 7: it removes the files of the stage and then its prepare record, without a flush. A record that a
// power loss brings back names a generation that is written, and the recovery collects it again.
func (c *Corpus) collect(stage string) error {
	if err := c.removeFiles(stage, false); err != nil {
		return err
	}
	if err := c.step("collect", 0); err != nil {
		return err
	}
	if err := c.removeFiles(stage, true); err != nil {
		return err
	}
	c.obs.Observe(Event{Kind: Collected, Path: stage})
	return nil
}

// removeFiles removes the files of the stage at stage: those other than its prepare record, or with record also that
// record.
func (c *Corpus) removeFiles(stage string, record bool) error {
	entries, err := os.ReadDir(c.txDir())
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	prefix := filepath.Base(stage) + "."
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || name == prefix+prepareName && !record {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.txDir(), name)); err != nil {
			return fmt.Errorf("journal: removing the stage: %w", err)
		}
	}
	return nil
}

// Staged reports whether the folder of the stages holds an entry: a commit in progress, or one a process left, which
// the recovery completes or discards under the lock. A read without the lock answers from the corpus it last read only
// when no stage is present (spec 40 Reads), so the folder is read and nothing in it is judged here.
func (c *Corpus) Staged() (bool, error) {
	present, folder, err := c.stageFolder()
	if err != nil || !present {
		return false, err
	}
	if !folder {
		// The recovery refuses it under the lock, so the read takes the lock to report it.
		return true, nil
	}
	f, err := os.Open(c.txDir())
	if err != nil {
		return false, fmt.Errorf("journal: %w", err)
	}
	names, err := f.Readdirnames(1)
	if err = errors.Join(ignoreEOF(err), f.Close()); err != nil {
		return false, fmt.Errorf("journal: %w", err)
	}
	return len(names) > 0, nil
}

// Prepared reports whether the journal holds a prepare record: a commit that may be renaming its targets, so a read
// without the lock may find some of them written and others not (spec 40 Reads). A stage before its prepare record has
// written no target. An entry that is no file of a stage, a stage folder of an earlier engine, or a file in the
// folder's place is reported too: the recovery judges it under the lock.
func (c *Corpus) Prepared() (bool, error) {
	present, folder, err := c.stageFolder()
	if err != nil || !present {
		return false, err
	}
	if !folder {
		return true, nil
	}
	entries, err := os.ReadDir(c.txDir())
	if err != nil {
		return false, fmt.Errorf("journal: %w", err)
	}
	for _, e := range entries {
		m := stageFileName.FindStringSubmatch(e.Name())
		if m == nil || m[1] == prepareName || e.IsDir() != strings.Contains(m[1], ".dir.") {
			return true, nil
		}
	}
	return false, nil
}

// stageFolder reads the place of the folder of the stages: whether an entry is there, and whether it is a folder.
func (c *Corpus) stageFolder() (present, folder bool, err error) {
	info, err := c.lstat(txRel)
	if err != nil {
		return false, false, fmt.Errorf("journal: %w", err)
	}
	if info == nil {
		return false, false, nil
	}
	return true, info.IsDir(), nil
}

// ignoreEOF is err without the io.EOF of a folder read to its end.
func ignoreEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// Recover runs the recovery of spec 40, opening a corpus, under l. It removes the pending records of ended lock
// holders and the databases of ended openings (removeEndedDatabases); then the one stage of `.flowmark/tx/` is
// collected when its prepare record names a generation the corpus has, completed when it holds a prepare record, and
// discarded otherwise. A target whose current hash matches neither recorded hash refuses with *RecoveryRefusal and
// leaves the stage. A second stage, an entry that is no file of a stage and a stage folder of an earlier engine refuse
// opening. Recover is idempotent.
func (c *Corpus) Recover(l *Lock) error {
	if err := c.checkHeld(l); err != nil {
		return err
	}
	if err := c.removeEndedPendings(); err != nil {
		return err
	}
	if err := c.removeEndedDatabases(); err != nil {
		return err
	}
	info, err := c.lstat(txRel)
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if info == nil {
		return nil
	}
	if !info.IsDir() {
		return fmt.Errorf("journal: %s is not a folder", c.txDir())
	}
	entries, err := os.ReadDir(c.txDir())
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	ids := map[string]bool{}
	for _, e := range entries {
		p := filepath.Join(c.txDir(), e.Name())
		if e.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("journal: %w", symlinkError(p))
		}
		if e.IsDir() && isHex(e.Name(), 32) {
			return fmt.Errorf("journal: %s is the stage folder of an earlier engine of flowmark, whose protocol this engine does not complete; open the corpus with the engine that wrote it, which completes or discards the stage, then open it with this one", p)
		}
		m := stageFileName.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() != strings.Contains(m[1], ".dir.") {
			return fmt.Errorf("journal: %s is no file of a stage (`<id>.<name>`, the id 32 lowercase hex digits); only commits write into %s, so remove it after checking what wrote it", p, c.txDir())
		}
		ids[e.Name()[:32]] = true
	}
	switch len(ids) {
	case 0:
		return nil
	case 1:
	default:
		return fmt.Errorf("journal: %s holds the stages %s, but a commit leaves at most one; check what wrote them before removing any", c.txDir(), strings.Join(sortedKeys(ids), ", "))
	}
	stage := filepath.Join(c.txDir(), sortedKeys(ids)[0])
	prepared, err := regularExists(stageFile(stage, prepareName))
	if err != nil {
		return err
	}
	if !prepared {
		if err := c.removeFiles(stage, true); err != nil {
			return err
		}
		c.obs.Observe(Event{Kind: Discarded, Path: stage})
		return nil
	}
	rec, err := readRecord(stage)
	if err != nil {
		return err
	}
	g, err := c.Generation()
	if err != nil {
		return err
	}
	if g.N >= rec.Generation {
		// The generation is written, so the renames are done: the stage is only collected.
		err = c.collect(stage)
	} else {
		err = c.complete(stage, rec)
	}
	if err != nil {
		return err
	}
	c.obs.Observe(Event{Kind: Replayed, Path: stage})
	return nil
}

func regularExists(p string) (bool, error) {
	info, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("journal: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("journal: %s is not a regular file", p)
	}
	return true, nil
}

// readRecord reads and checks the prepare record of the stage at stage; one that does not parse is an error, never an
// absent record. Every recorded state is checked against its hash, so a damaged record applies nothing.
func readRecord(stage string) (prepareRecord, error) {
	p := stageFile(stage, prepareName)
	data, _, err := readShared(p)
	if err != nil {
		return prepareRecord{}, fmt.Errorf("journal: %w", err)
	}
	var rec prepareRecord
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rec); err != nil {
		return prepareRecord{}, fmt.Errorf("journal: %s holds no prepare record: %w", p, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return prepareRecord{}, fmt.Errorf("journal: %s holds more than one prepare record", p)
	}
	if rec.Generation == 0 {
		return prepareRecord{}, fmt.Errorf("journal: %s names no generation", p)
	}
	seen := map[string]bool{}
	for _, r := range rec.Renames {
		if err := checkTarget(r.Path); err != nil {
			return prepareRecord{}, fmt.Errorf("journal: %s: %w", p, err)
		}
		if seen[r.Path] || !isHash(r.Pre) || !isHash(r.Post) || r.Pre == "" && r.Post == "" {
			return prepareRecord{}, fmt.Errorf("journal: %s holds a malformed rename of %s", p, r.Path)
		}
		seen[r.Path] = true
		for _, h := range []string{r.Pre, r.Post} {
			if h == "" {
				continue
			}
			blob, ok := rec.Blobs[h]
			if !ok {
				return prepareRecord{}, fmt.Errorf("journal: %s holds no bytes of the state %s of %s", p, h, r.Path)
			}
			if got := sum(blob); got != h {
				return prepareRecord{}, fmt.Errorf("journal: %s holds bytes of hash %s for the state %s of %s; the stage is damaged and nothing of it is applied", p, got, h, r.Path)
			}
		}
	}
	return rec, nil
}
