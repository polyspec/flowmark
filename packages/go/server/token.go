package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// TokenPath is the corpus path of the token file (spec 40 The server).
const TokenPath = ".flowmark/server.json"

// tokenFile is the one form of the token file: one line of JSON holding 32 lowercase hex digits, and a line feed.
var tokenFile = regexp.MustCompile(`^\{"token":"([0-9a-f]{32})"\}\n$`)

// Token returns the server token of the corpus at root, an absolute path. The first start writes a new token; later
// starts read it; renew (--new-token) replaces it. The file is written under a partial name and linked into place, so
// servers starting at once keep the one token the first link placed; a renewal renames over it. A file that does not
// hold the one form, or a symlink where it or `.flowmark/` must be, refuses.
func Token(root string, renew bool) (tok string, err error) {
	dir := filepath.Join(root, ".flowmark")
	if err := ensureFolder(dir); err != nil {
		return "", err
	}
	p := filepath.Join(root, filepath.FromSlash(TokenPath))
	fi, err := os.Lstat(p)
	switch {
	case err == nil && fi.Mode()&fs.ModeSymlink != 0:
		return "", fmt.Errorf("the token file %s is a symbolic link, which flowmark never follows; remove it and start the server with --new-token", p)
	case err == nil && !renew:
		return readToken(p)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("the token file: %w", err)
	}
	tok = randomHex(16)
	part := p + "." + randomHex(16) + ".part"
	if err := writeSynced(part, []byte(`{"token":"`+tok+`"}`+"\n")); err != nil {
		return "", fmt.Errorf("writing the token file: %w", err)
	}
	defer func() {
		if rerr := os.Remove(part); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			err = errors.Join(err, rerr)
		}
	}()
	if renew {
		if err := os.Rename(part, p); err != nil {
			return "", fmt.Errorf("placing the token file: %w", err)
		}
		return tok, nil
	}
	if err := os.Link(part, p); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return readToken(p)
		}
		return "", fmt.Errorf("placing the token file: %w", err)
	}
	return tok, nil
}

// readToken reads the token file p.
func readToken(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("the token file: %w", err)
	}
	m := tokenFile.FindSubmatch(b)
	if m == nil {
		return "", fmt.Errorf(`the token file %s does not hold one line {"token":"<32 lowercase hex digits>"} and a line feed; replace it by starting the server with --new-token`, p)
	}
	return string(m[1]), nil
}

// ensureFolder creates the folder dir when it is absent, following no symlink.
func ensureFolder(dir string) error {
	if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	fi, err := os.Lstat(dir)
	switch {
	case err != nil:
		return err
	case fi.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%s is a symbolic link, which flowmark never follows; declare a corpus whose .flowmark is a folder", dir)
	case !fi.IsDir():
		return fmt.Errorf("%s is not a folder", dir)
	}
	return nil
}

// writeSynced creates the file p, readable and writable by its owner only, with data, and syncs it.
func writeSynced(p string, data []byte) error {
	f, err := createOwnerOnly(p)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

// randomHex returns 2n lowercase hex digits of n random bytes from the OS random source.
func randomHex(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read never returns an error; it aborts the program when the system source fails.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
