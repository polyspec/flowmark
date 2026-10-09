// Package sidecar is the soksak sidecar face: the protocol v1 message loop of the soksak
// core specification — one JSON object per line over standard input and output, a page
// request as {surface, root, body}, the answers as {surface, body} or a closed handshake.
package sidecar

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/polyspec/flowmark/app"
	"github.com/polyspec/flowmark/corpus"
	"github.com/polyspec/flowmark/executor"
)

type message struct {
	Surface int             `json:"surface"`
	Root    string          `json:"root"`
	Closed  bool            `json:"closed"`
	Body    json.RawMessage `json:"body"`
}

type operation struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Evidence  string `json:"evidence"`
}

// Serve reads the host messages and answers each: a closed handshake answers closed, a page
// request runs its operation over the corpus of the surface root. A line that is not a
// message of the protocol produces no answer; the host treats a protocol breach by killing
// the process, so the loop stops reading.
func Serve(in io.Reader, out io.Writer) {
	writer := bufio.NewWriter(out)
	defer writer.Flush()
	watcher := &corpusWatcher{surfaces: map[int]string{}, finger: map[string]string{}}
	defer watcher.stop()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var received message
		if err := json.Unmarshal([]byte(line), &received); err != nil {
			return // an invalid line ends the loop; the host reports the breach
		}
		if received.Closed {
			watcher.forget(received.Surface)
			write(writer, map[string]any{"surface": received.Surface, "closed": true})
			continue
		}
		watcher.watch(received.Surface, received.Root, writer)
		var operation operation
		if err := json.Unmarshal(received.Body, &operation); err != nil {
			return
		}
		answer, err := run(received.Root, &operation)
		if err != nil {
			write(writer, map[string]any{"surface": received.Surface, "body": err.Error()})
			continue
		}
		write(writer, map[string]any{"surface": received.Surface, "body": answer})
	}
}

func run(root string, operation *operation) (string, error) {
	switch operation.Operation {
	case "items":
		opened, err := executor.Open(root)
		if err != nil {
			return "", err
		}
		documents, err := opened.All("documents", executor.Query{})
		if err != nil {
			return "", err
		}
		pathOf := map[string]string{}
		for _, row := range documents {
			pathOf[row["doc_id"]] = row["path"]
		}
		rows, err := opened.All("items", executor.Query{})
		if err != nil {
			return "", err
		}
		// state, id, title, path — the page addresses its operations by the path.
		lines := make([]string, 0, len(rows))
		for _, row := range rows {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s", row["state"], row["id"], row["title"], pathOf[row["doc_id"]]))
		}
		return strings.Join(lines, "\n"), nil
	case "validate":
		findings, err := corpus.Validate(root)
		if err != nil {
			return "", err
		}
		lines := make([]string, 0, len(findings))
		for _, finding := range findings {
			lines = append(lines, fmt.Sprintf("%s: line %d: %s: %s", finding.File, finding.Line, finding.Rule, finding.Message))
		}
		if len(lines) == 0 {
			lines = append(lines, "the corpus passes")
		}
		return strings.Join(lines, "\n"), nil
	case "add":
		if err := app.AddItem(root, operation.Path, operation.ID, operation.Title); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s added", operation.ID), nil
	case "remove":
		if err := app.RemoveItem(root, operation.Path, operation.ID); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s removed", operation.ID), nil
	case "transition":
		if err := app.Transition(root, operation.Path, operation.ID, operation.State, operation.Evidence); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s is %s", operation.ID, operation.State), nil
	}
	return "", fmt.Errorf("the operation %q is unknown", operation.Operation)
}

func write(writer *bufio.Writer, message map[string]any) {
	text, err := json.Marshal(message)
	if err != nil {
		return
	}
	writer.Write(text)
	writer.WriteByte('\n')
	// The host reads each answer while the process runs; a message flushes on its own.
	writer.Flush()
}

// corpusWatcher polls the markdown files of each open surface root and pushes a changed
// event when one of them changes: the page refreshes while the host session runs.
type corpusWatcher struct {
	mu       sync.Mutex
	surfaces map[int]string
	finger   map[string]string
	stopped  bool
}

// watch remembers the surface root and starts one poll loop per distinct root.
func (w *corpusWatcher) watch(surface int, root string, writer *bufio.Writer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	firstRoot := len(w.surfaces) == 0
	w.surfaces[surface] = root
	if firstRoot {
		go w.poll(writer)
	}
}

// forget drops a closed surface; the poll loop ends when no surface remains.
func (w *corpusWatcher) forget(surface int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.surfaces, surface)
}

// stop ends every poll loop with the session.
func (w *corpusWatcher) stop() {
	w.mu.Lock()
	w.stopped = true
	w.mu.Unlock()
}

// poll compares the fingerprint of the watched roots and pushes changed events.
func (w *corpusWatcher) poll(writer *bufio.Writer) {
	for {
		w.mu.Lock()
		if w.stopped || len(w.surfaces) == 0 {
			w.mu.Unlock()
			return
		}
		roots := map[string]bool{}
		for _, root := range w.surfaces {
			roots[root] = true
		}
		w.mu.Unlock()
		for root := range roots {
			finger, err := fingerprint(root)
			if err != nil {
				continue
			}
			w.mu.Lock()
			known := w.finger[root]
			w.finger[root] = finger
			w.mu.Unlock()
			if known != "" && known != finger {
				w.push(writer)
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
}

// push sends one changed event to every open surface.
func (w *corpusWatcher) push(writer *bufio.Writer) {
	w.mu.Lock()
	surfaces := make([]int, 0, len(w.surfaces))
	for surface := range w.surfaces {
		surfaces = append(surfaces, surface)
	}
	w.mu.Unlock()
	for _, surface := range surfaces {
		write(writer, map[string]any{"surface": surface, "body": map[string]any{"event": "changed"}})
	}
}

// fingerprint joins the path, size and mtime of every markdown file of the root.
func fingerprint(root string) (string, error) {
	var parts []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		parts = append(parts, fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	return strings.Join(parts, "|"), err
}
