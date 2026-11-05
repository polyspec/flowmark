package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/polyspec/orm/packages/go/orm"
)

// KeyringVariable is the environment variable that names the keyring file of a process (spec 30 Keys).
const KeyringVariable = "FLOWMARK_KEYRING"

// Keys are the keys injected into a process: the AES key of each declared version, the current version and the key of
// the blind indexes. The file holds the shape of orm's connection configuration (orm COD-8).
type Keys struct {
	Current    int32
	Versions   map[int32]string
	BlindIndex string
}

// Key returns the AES key of version, and whether the keyring declares it.
func (k *Keys) Key(version int32) (string, bool) {
	key, ok := k.Versions[version]
	return key, ok
}

// keyringFile is the JSON form of a keyring file.
type keyringFile struct {
	Current    int32             `json:"current"`
	Versions   map[string]string `json:"versions"`
	BlindIndex string            `json:"blind_index"`
}

// readKeys reads the keyring that KeyringVariable names, and nil when it names none. A key held in the corpus would be
// published with the corpus, so a keyring inside root is refused, as a symbolic link is (spec 30 Keys).
func readKeys(root string) (*Keys, error) {
	path := os.Getenv(KeyringVariable)
	if path == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	corpus, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if rel, err := filepath.Rel(corpus, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("the keyring %s is inside the corpus, which publishes it", path)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("the keyring %s is not a regular file", path)
	}
	text, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	var f keyringFile
	dec := json.NewDecoder(bytes.NewReader(text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("the keyring %s: %w", path, err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("the keyring %s holds more than one value", path)
	}
	versions := make(map[int32]string, len(f.Versions))
	for name, key := range f.Versions {
		v, err := strconv.ParseInt(name, 10, 32)
		if err != nil || strconv.FormatInt(v, 10) != name {
			return nil, fmt.Errorf("the keyring %s: the version %q is not an integer", path, name)
		}
		versions[int32(v)] = key
	}
	if _, err := orm.NewAESKeyring(versions, f.Current); err != nil {
		return nil, fmt.Errorf("the keyring %s: %w", path, err)
	}
	return &Keys{Current: f.Current, Versions: versions, BlindIndex: f.BlindIndex}, nil
}
