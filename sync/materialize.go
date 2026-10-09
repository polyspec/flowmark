package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Materialize writes one adapter-owned document: the provenance header `owner: db:<table>` —
// the header names the owning side; the body beneath it carries its own hash, so the drift
// check is self-verifying and clone-safe (spec 70-sync). The rows arrive ordered by their
// primary key from the owning side.
func Materialize(path, table, title string, rows []map[string]string) error {
	body := &strings.Builder{}
	fmt.Fprintf(body, "# %s\n\n## Rows\n\n", title)
	ids := make([]string, 0, len(rows))
	byID := map[string]map[string]string{}
	for _, row := range rows {
		ids = append(ids, row["id"])
		byID[row["id"]] = row
	}
	sort.Strings(ids)
	for _, id := range ids {
		row := byID[id]
		fmt.Fprintf(body, "- %s %s %s\n", row["state"], id, row["title"])
	}
	// The hash covers the body beneath the header line, so the file verifies itself.
	sum := sha256.Sum256([]byte(body.String()))
	text := fmt.Sprintf("<!-- owner: db:%s@%s -->\n", table, hex.EncodeToString(sum[:8])) + body.String()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// Drift names the drift of an adapter-owned document: the empty string when the body matches
// its recorded hash, a sentence when a human edited it or the header is gone.
func Drift(path string) (string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.SplitN(string(text), "\n", 2)
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "<!-- owner: db:") {
		return fmt.Sprintf("%s: the provenance header is missing", path), nil
	}
	at := strings.Index(lines[0], "@")
	if at < 0 {
		return fmt.Sprintf("%s: the provenance header holds no hash", path), nil
	}
	recorded := strings.TrimSpace(strings.TrimSuffix(lines[0][at+1:], "-->"))
	sum := sha256.Sum256([]byte(lines[1]))
	if hex.EncodeToString(sum[:8]) != recorded {
		return fmt.Sprintf("%s: the body drifted from its recorded hash; the owning side materializes it again", path), nil
	}
	return "", nil
}
