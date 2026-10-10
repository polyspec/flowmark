package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/polyspec/flowmark/codec"
)

// WIPHeadroom is the number of doing items allowed beyond the done count: parallel work is
// welcome, an endless growth of unfinished items is not (spec 100-checklist).
const WIPHeadroom = 2

// Transition applies one state transition to an item of a document: it holds the transition
// table, the required fields and the WIP gate, then rewrites the state glyph of the item
// line. The evidence carries the required fields — `cause:` and `retry:` on a bypass,
// `cause:` on an impossible item, the verification on a done item.
func Transition(root, relativePath, id, target, evidence string) error {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	document, diagnostics := codec.Parse(string(text))
	if len(diagnostics) > 0 {
		return fmt.Errorf("%s holds %d parse findings; fix them first", relativePath, len(diagnostics))
	}
	var item *codec.ChecklistItem
	for i := range document.Items {
		if document.Items[i].ID == id {
			item = &document.Items[i]
			break
		}
	}
	if item == nil {
		return fmt.Errorf("the item %q is not in %s", id, relativePath)
	}
	// the caller may pass the glyph ([~]) or the name (doing)
	if state, ok := StateOf(target); ok {
		target = state
	}
	from, ok := StateOf(item.State)
	if !ok {
		return fmt.Errorf("the item %q carries the unknown state %q", id, item.State)
	}
	rule, ok := CanTransition(from, target)
	if !ok {
		return fmt.Errorf("the transition %s -> %s is not in the table; done is terminal and a related issue spawns a superseded item", from, target)
	}
	if !requiredMet(rule, evidence) {
		return fmt.Errorf("the transition %s -> %s requires %s on the item", from, target, strings.Join(rule.Requires, ", "))
	}
	if target == "doing" && !wipHolds(document) {
		return fmt.Errorf("the WIP gate refuses another doing item: %d doing against %d done", countState(document, "doing"), countState(document, "done"))
	}

	lines := strings.Split(string(text), "\n")
	at := item.Line - 1
	glyph := GlyphOf(target)
	replace := strings.Replace(lines[at], item.State, glyph, 1)
	if replace == lines[at] {
		return fmt.Errorf("the state %q is not on line %d", item.State, item.Line)
	}
	lines[at] = replace
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// requiredMet holds the required-field rules of a transition against the evidence text.
func requiredMet(transition Rule, evidence string) bool {
	for _, field := range transition.Requires {
		switch field {
		case "evidence":
			if strings.TrimSpace(evidence) == "" {
				return false
			}
		case "cause":
			if !strings.Contains(evidence, "cause:") {
				return false
			}
		case "retry":
			if !strings.Contains(evidence, "retry:") {
				return false
			}
		}
	}
	return true
}

// wipHolds keeps the doing count within the headroom over the done count.
func wipHolds(document *codec.Document) bool {
	return countState(document, "doing") < countState(document, "done")+WIPHeadroom
}

func countState(document *codec.Document, state string) int {
	count := 0
	for _, item := range document.Items {
		if current, ok := StateOf(item.State); ok && current == state {
			count++
		}
	}
	return count
}

// AddItem appends one waiting item to a document through the write path: the ID is unique
// in the document, the line lands after the last item's continuations, or at the end when
// the document holds no item yet.
func AddItem(root, relativePath, id, title string) error {
	if id == "" {
		id = NextID(root, relativePath)
	}
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	document, diagnostics := codec.Parse(string(text))
	if len(diagnostics) > 0 {
		return fmt.Errorf("%s holds %d parse findings; fix them first", relativePath, len(diagnostics))
	}
	for _, item := range document.Items {
		if item.ID == id {
			return fmt.Errorf("the item %q is already in %s", id, relativePath)
		}
	}
	lines := strings.Split(string(text), "\n")
	insert := len(lines)
	if len(document.Items) > 0 {
		last := document.Items[len(document.Items)-1]
		insert = last.Line + len(last.Continuations)
	}
	newItem := "- " + GlyphOf("waiting") + " " + id + " " + title
	lines = append(lines[:insert], append([]string{newItem}, lines[insert:]...)...)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// RemoveItem deletes one item with its continuation lines. A parent whose ID carries
// children (a dotted or suffixed child) is refused — a delete may not orphan rows.
func RemoveItem(root, relativePath, id string) error {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	document, diagnostics := codec.Parse(string(text))
	if len(diagnostics) > 0 {
		return fmt.Errorf("%s holds %d parse findings; fix them first", relativePath, len(diagnostics))
	}
	var found *codec.ChecklistItem
	for i := range document.Items {
		if document.Items[i].ID == id {
			found = &document.Items[i]
		}
		if strings.HasPrefix(document.Items[i].ID, id+".") || strings.HasPrefix(document.Items[i].ID, id+"-") {
			return fmt.Errorf("the item %q carries the child %q; remove the children first", id, document.Items[i].ID)
		}
	}
	if found == nil {
		return fmt.Errorf("the item %q is not in %s", id, relativePath)
	}
	lines := strings.Split(string(text), "\n")
	block := len(found.Continuations) + 1
	rest := append(append([]string{}, lines[:found.Line-1]...), lines[found.Line-1+block:]...)
	return os.WriteFile(path, []byte(strings.Join(rest, "\n")), 0o644)
}

// NextID names the next top-level item ID of a document: one past the highest T<number>.
func NextID(root, relativePath string) string {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return "T1"
	}
	document, _ := codec.Parse(string(text))
	highest := 0
	for _, item := range document.Items {
		id := item.ID
		if !strings.HasPrefix(id, "T") {
			continue
		}
		segment := id[1:]
		if at := strings.IndexAny(segment, ".-"); at >= 0 {
			segment = segment[:at] // a child raises the top level of its first segment
		}
		number, err := strconv.Atoi(segment)
		if err != nil {
			continue
		}
		if number > highest {
			highest = number
		}
	}
	return fmt.Sprintf("T%d", highest+1)
}

// MoveItem repositions one item (with its continuation lines) after another item, or to the
// end of the item list when afterID is empty. The state does not change.
func MoveItem(root, relativePath, id, afterID string) error {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	document, diagnostics := codec.Parse(string(text))
	if len(diagnostics) > 0 {
		return fmt.Errorf("%s holds %d parse findings; fix them first", relativePath, len(diagnostics))
	}
	var moving, anchor *codec.ChecklistItem
	for i := range document.Items {
		if document.Items[i].ID == id {
			moving = &document.Items[i]
		}
		if afterID != "" && document.Items[i].ID == afterID {
			anchor = &document.Items[i]
		}
	}
	if moving == nil {
		return fmt.Errorf("the item %q is not in %s", id, relativePath)
	}
	if afterID != "" && anchor == nil {
		return fmt.Errorf("the anchor %q is not in %s", afterID, relativePath)
	}

	lines := strings.Split(string(text), "\n")
	block := moving.Line - 1
	blockEnd := block + 1 + len(moving.Continuations)
	itemLines := append(append([]string{}, lines[block:blockEnd]...))
	rest := append(append([]string{}, lines[:block]...), lines[blockEnd:]...)

	var insert int
	if anchor == nil {
		// no anchor: the item lands before the first item of the list
		if len(document.Items) > 0 {
			insert = document.Items[0].Line - 1
			if insert > block {
				insert -= blockEnd - block
			}
		} else {
			insert = 0
		}
	} else {
		anchorBlock := anchor.Line - 1
		if anchorBlock >= block {
			anchorBlock -= blockEnd - block
		}
		insert = anchorBlock + 1 + len(anchor.Continuations)
	}
	out := append(append(append([]string{}, rest[:insert]...), itemLines...), rest[insert:]...)
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}

// EditTitle rewrites the title of one item in place, leaving the state, ID and
// continuations alone.
func EditTitle(root, relativePath, id, title string) error {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	document, diagnostics := codec.Parse(string(text))
	if len(diagnostics) > 0 {
		return fmt.Errorf("%s holds %d parse findings; fix them first", relativePath, len(diagnostics))
	}
	for _, item := range document.Items {
		if item.ID == id {
			lines := strings.Split(string(text), "\n")
			at := item.Line - 1
			prefix := "- " + item.State + " " + id + " "
			if strings.HasPrefix(lines[at], prefix) {
				lines[at] = prefix + title
				return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
			}
		}
	}
	return fmt.Errorf("the item %q is not in %s", id, relativePath)
}

// stamp writes one history continuation line under the item: `  - at <RFC3339> <event>`.
func stamp(lines []string, itemLine int, event string) []string {
	at := time.Now().UTC().Format(time.RFC3339)
	record := "  - at " + at + " " + event
	insert := itemLine + 1
	return append(lines[:insert], append([]string{record}, lines[insert:]...)...)
}

// findItemEnd returns the line just past the item's continuation lines (0-indexed).
func findItemEnd(document *codec.Document, id string) int {
	for _, item := range document.Items {
		if item.ID == id {
			return item.Line - 1 + 1 + len(item.Continuations)
		}
	}
	return -1
}
