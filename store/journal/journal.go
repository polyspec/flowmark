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
	"sort"
	"strconv"
	"strings"
)

// txRel is the journal folder of spec 40: a commit stages in `.flowmark/tx/<id>/`, and it holds at most one stage.
const txRel = ".flowmark/tx"

// The files of a stage. A post-image or pre-image is a blob named by its hash, written once; the source of rename n
// is `<n>.<hash>`, a copy of the blob, so two identical targets are two files and no target shares a file with the
// stage; a delete moves its target to `<n>.deleted`; a folder created in the stage is `<n>.dir.<k>`. Every stage file
// is written to `<name>.part` and renamed into place.
const (
	prepareName   = "prepare"
	commitName    = "commit"
	collectedName = "collected"
	partSuffix    = ".part"
)

func copyName(n int, hash string) string { return strconv.Itoa(n) + "." + hash }
func deletedName(n int) string           { return strconv.Itoa(n) + ".deleted" }
func folderName(n, k int) string         { return fmt.Sprintf("%d.dir.%d", n, k) }

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

type prepareRecord struct {
	Renames []rename `json:"renames"`
}

// RecoveryRefusal: a target's current hash matches neither recorded hash — a change landed while the commit was
// interrupted. Nothing of the stage is applied and the stage stays; every open refuses the same way until the target
// holds one of its recorded states.
type RecoveryRefusal struct {
	Stage, Path, Pre, Post, Current string
}

// state names a recorded state and the stage file that holds it.
func (e *RecoveryRefusal) state(h string) string {
	if h == "" {
		return "absent (remove the file)"
	}
	return h + " in " + filepath.Join(e.Stage, h)
}

func (e *RecoveryRefusal) Error() string {
	current := e.Current
	if current == "" {
		current = "absent"
	}
	return fmt.Sprintf("journal: recovery refuses %s: its content %s matches neither recorded state — before: %s; after: %s. A change made while the commit was interrupted is never overwritten: copy it aside, copy one recorded state over %s, and open the corpus again, which completes the commit; then make the change again",
		e.Path, current, e.state(e.Pre), e.state(e.Post), e.Path)
}

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
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'f') {
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
func writeDurable(dir, name string, data []byte) error {
	part := filepath.Join(dir, name+partSuffix)
	if err := writeSynced(part, data, os.O_TRUNC); err != nil {
		return fmt.Errorf("journal: writing %s: %w", name, err)
	}
	if err := moveDurable(part, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("journal: placing %s: %w", name, err)
	}
	return nil
}

// Commit runs steps 3 to 8 of the spec 40 commit protocol for changes, under l: every post-image is written into a
// new stage once, content-addressed, with a copy per rename; every pre-image content-addressed; the prepare record;
// the renames; the commit record; and the stage is collected. The caller has run steps 1 and 2. A target folder, or a
// folder to create, on another file system than the journal refuses before anything is staged. A stage left by an
// interrupted commit refuses a new commit until Recover has run.
func (c *Corpus) Commit(l *Lock, changes []Change) error {
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
	if err := mkdirDurable(stage); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	// Step 3: each post-image once, and the copy that is the source of each rename.
	posts := map[string]bool{}
	for n, r := range renames {
		if r.Post == "" {
			continue
		}
		if !posts[r.Post] {
			posts[r.Post] = true
			if err := writeDurable(stage, r.Post, blobs[r.Post]); err != nil {
				return err
			}
		}
		if err := writeDurable(stage, copyName(n, r.Post), blobs[r.Post]); err != nil {
			return err
		}
	}
	if err := c.step("post-images", 0); err != nil {
		return err
	}
	// Step 4: the pre-images not already written as a post-image of the same bytes.
	for _, h := range sortedKeys(blobs) {
		if !posts[h] {
			if err := writeDurable(stage, h, blobs[h]); err != nil {
				return err
			}
		}
	}
	if err := c.step("pre-images", 0); err != nil {
		return err
	}
	// Step 5: the prepare record.
	record, err := json.Marshal(prepareRecord{Renames: renames})
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if err := writeDurable(stage, prepareName, append(record, '\n')); err != nil {
		return err
	}
	c.obs.Observe(Event{Kind: Prepared, Path: stage})
	if err := c.step("prepare", 0); err != nil {
		return err
	}
	return c.complete(stage, renames)
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

// complete runs steps 6 to 8 for a prepared stage; it is the commit's own path and the replay's, and running it again
// on the same stage is idempotent. Every target is checked before any is touched: one that holds its post-image is
// done, one that holds its pre-image is pending, and any other refuses the whole stage.
func (c *Corpus) complete(stage string, renames []rename) error {
	var pending []int
	for n, r := range renames {
		cur, err := c.currentHash(r.Path)
		if err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		switch cur {
		case r.Post:
		case r.Pre:
			pending = append(pending, n)
		default:
			return &RecoveryRefusal{Stage: stage, Path: r.Path, Pre: r.Pre, Post: r.Post, Current: cur}
		}
	}
	// Every rename source is checked against its hash before any target is touched; a missing copy is written again
	// from its blob.
	for _, n := range pending {
		if r := renames[n]; r.Post != "" {
			if err := c.ensureCopy(stage, n, r); err != nil {
				return err
			}
		}
	}
	// Step 6: the renames; a delete is a rename into the stage.
	for _, n := range pending {
		r := renames[n]
		target := c.abs(r.Path)
		var err error
		if r.Post == "" {
			err = moveDurable(target, filepath.Join(stage, deletedName(n)))
		} else {
			if r.Pre == "" {
				if err := c.makeFolders(stage, n, path.Dir(r.Path)); err != nil {
					return err
				}
			}
			err = moveDurable(filepath.Join(stage, copyName(n, r.Post)), target)
		}
		if err != nil {
			return fmt.Errorf("journal: applying %s: %w", r.Path, err)
		}
		if err := c.step("rename", n); err != nil {
			return err
		}
	}
	if err := c.step("renames", 0); err != nil {
		return err
	}
	// Step 7: the commit record.
	if err := writeDurable(stage, commitName, []byte("{}\n")); err != nil {
		return err
	}
	c.obs.Observe(Event{Kind: Applied, Path: stage})
	if err := c.step("commit", 0); err != nil {
		return err
	}
	return c.collect(stage)
}

// ensureCopy makes sure the source of rename n holds its post-image, writing it again from the blob when absent.
func (c *Corpus) ensureCopy(stage string, n int, r rename) error {
	src := filepath.Join(stage, copyName(n, r.Post))
	_, err := os.Lstat(src)
	if errors.Is(err, fs.ErrNotExist) {
		blob := filepath.Join(stage, r.Post)
		data, _, err := readShared(blob)
		if err != nil {
			return fmt.Errorf("journal: the stage blob %s of %s: %w", r.Post, r.Path, err)
		}
		if got := sum(data); got != r.Post {
			return fmt.Errorf("journal: the stage blob %s of %s holds bytes of hash %s; the stage %s is damaged and nothing of it is applied", r.Post, r.Path, got, stage)
		}
		if err := writeDurable(stage, copyName(n, r.Post), data); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	got, err := hashFile(src)
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if got != r.Post {
		return fmt.Errorf("journal: the stage file %s for %s holds bytes of hash %s, not %s; the stage %s is damaged and nothing of it is applied", copyName(n, r.Post), r.Path, got, r.Post, stage)
	}
	return nil
}

// makeFolders creates the missing folders of dir for the create of rename n, from the corpus root, following no
// symlink. Where folders are staged (Windows), each is made in the stage as `<n>.dir.<k>` and moved into place; a
// replay skips a folder in place and moves one it finds staged.
func (c *Corpus) makeFolders(stage string, n int, dir string) error {
	if dir == "." {
		return nil
	}
	cur := c.root
	made := false
	for k, part := range strings.Split(dir, "/") {
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
			staged := filepath.Join(stage, folderName(n, k))
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
			if err := moveFolderDurable(staged, cur); err != nil {
				return fmt.Errorf("journal: creating the folder %s: %w", cur, err)
			}
		} else if err := mkdirDurable(cur); err != nil {
			return fmt.Errorf("journal: creating the folder %s: %w", cur, err)
		}
		made = true
	}
	if made {
		return c.step("folder", n)
	}
	return nil
}

// collect runs step 8. The prepare record is renamed aside first, so a stage interrupted while it is collected holds
// the commit record without it and is collected again.
func (c *Corpus) collect(stage string) error {
	_, err := os.Lstat(filepath.Join(stage, prepareName))
	switch {
	case err == nil:
		if err := moveDurable(filepath.Join(stage, prepareName), filepath.Join(stage, collectedName)); err != nil {
			return fmt.Errorf("journal: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("journal: %w", err)
	}
	if err := c.step("collect", 0); err != nil {
		return err
	}
	return c.remove(stage, Collected)
}

func (c *Corpus) remove(stage string, kind Kind) error {
	if err := os.RemoveAll(stage); err != nil {
		return fmt.Errorf("journal: removing the stage: %w", err)
	}
	if err := syncRemoved(c.txDir()); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	c.obs.Observe(Event{Kind: kind, Path: stage})
	return nil
}

// Recover runs the recovery of spec 40, opening a corpus, under l. It removes the pending records of ended lock
// holders; then the one stage of `.flowmark/tx/` is collected when it holds a commit record, completed when it holds a
// prepare record, and discarded otherwise. A target whose current hash matches neither recorded hash refuses with
// *RecoveryRefusal and leaves the stage. A second stage, or an entry that is no stage, refuses opening. Recover is
// idempotent.
func (c *Corpus) Recover(l *Lock) error {
	if err := c.checkHeld(l); err != nil {
		return err
	}
	if err := c.removeEndedPendings(); err != nil {
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
	var names []string
	for _, e := range entries {
		p := filepath.Join(c.txDir(), e.Name())
		if e.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("journal: %w", symlinkError(p))
		}
		if !e.IsDir() || !isHex(e.Name(), 32) {
			return fmt.Errorf("journal: %s is no stage (a folder of 32 lowercase hex digits); only commits write into %s, so remove it after checking what wrote it", p, c.txDir())
		}
		names = append(names, e.Name())
	}
	switch len(names) {
	case 0:
		return nil
	case 1:
	default:
		return fmt.Errorf("journal: %s holds the stages %s, but a commit leaves at most one; check what wrote them before removing any", c.txDir(), strings.Join(names, ", "))
	}
	stage := filepath.Join(c.txDir(), names[0])
	committed, err := regularExists(filepath.Join(stage, commitName))
	if err != nil {
		return err
	}
	prepared, err := regularExists(filepath.Join(stage, prepareName))
	if err != nil {
		return err
	}
	switch {
	case committed:
		err = c.collect(stage)
	case prepared:
		var rec prepareRecord
		if rec, err = readPrepare(stage); err == nil {
			err = c.complete(stage, rec.Renames)
		}
	default:
		return c.remove(stage, Discarded)
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

// readPrepare reads and checks a prepare record; one that does not parse is an error, never an absent record.
func readPrepare(stage string) (prepareRecord, error) {
	p := filepath.Join(stage, prepareName)
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
	seen := map[string]bool{}
	for _, r := range rec.Renames {
		if err := checkTarget(r.Path); err != nil {
			return prepareRecord{}, fmt.Errorf("journal: %s: %w", p, err)
		}
		if seen[r.Path] || !isHash(r.Pre) || !isHash(r.Post) || r.Pre == "" && r.Post == "" {
			return prepareRecord{}, fmt.Errorf("journal: %s holds a malformed rename of %s", p, r.Path)
		}
		seen[r.Path] = true
	}
	return rec, nil
}
