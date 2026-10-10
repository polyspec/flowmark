package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/codec"
)

// WIPHeadroom is the number of doing items allowed beyond the done count: parallel work is
// welcome, an endless growth of unfinished items is not (spec 100-checklist).
const WIPHeadroom = 2

// Transition applies one state transition to an item of a document: the state machine the
// document declares in its Mermaid block holds the edges, the required fields and the glyphs —
// no built-in table exists. The evidence carries the fields the edge label names.
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
	schema, err := SchemaFromMermaidBlock(root, relativePath)
	if err != nil {
		return fmt.Errorf("the document declares no state machine: %v", err)
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
	fromState := schema.StateByGlyph(item.State)
	if fromState == nil {
		return fmt.Errorf("the item %q carries the state %q, which the state machine of %s does not declare", id, item.State, relativePath)
	}
	// the caller may pass the glyph ([~]) or the name (doing)
	targetState := schema.StateByName(target)
	if targetState == nil {
		targetState = schema.StateByGlyph(target)
	}
	if targetState == nil {
		return fmt.Errorf("the state %q is not in the state machine of %s", target, relativePath)
	}
	edge := schema.Edge(fromState.Name, targetState.Name)
	if edge == nil {
		return fmt.Errorf("the state machine of %s holds no transition %s -> %s", relativePath, fromState.Name, targetState.Name)
	}
	if !requiredMet(edge.Requires, evidence) {
		return fmt.Errorf("the transition %s -> %s requires %s on the item", fromState.Name, targetState.Name, strings.Join(edge.Requires, ", "))
	}
	// the WIP gate holds the kit glyph [~] count within the headroom over the [o] count
	if targetState.Glyph == "[~]" && !wipHolds(document) {
		return fmt.Errorf("the WIP gate refuses another [~] item: %d in progress against %d done", countGlyph(document, "[~]"), countGlyph(document, "[o]"))
	}

	lines := strings.Split(string(text), "\n")
	at := item.Line - 1
	replace := strings.Replace(lines[at], item.State, targetState.Glyph, 1)
	if replace == lines[at] {
		return fmt.Errorf("the state %q is not on line %d", item.State, item.Line)
	}
	lines[at] = replace
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// requiredMet holds the required-field rules of a transition against the evidence text.
func requiredMet(requires []string, evidence string) bool {
	for _, field := range requires {
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

// wipHolds keeps the in-progress glyph count within the headroom over the done count.
func wipHolds(document *codec.Document) bool {
	return countGlyph(document, "[~]") < countGlyph(document, "[o]")+WIPHeadroom
}

func countGlyph(document *codec.Document, glyph string) int {
	count := 0
	for _, item := range document.Items {
		if item.State == glyph {
			count++
		}
	}
	return count
}

// AddItem appends one item in the initial state of the document — the state the Mermaid
// block's `[*] -->` line names — through the write path: the ID is unique in the document,
// the line lands after the last item's continuations, or at the end when the document holds
// no item yet.
func AddItem(root, relativePath, id, title string) error {
	if id == "" {
		id = NextID(root, relativePath)
	}
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	schema, err := SchemaFromMermaidBlock(root, relativePath)
	if err != nil {
		return fmt.Errorf("the document declares no state machine: %v", err)
	}
	glyph := schema.GlyphOfInitial()
	if glyph == "" {
		return fmt.Errorf("the state machine of %s declares no initial state", relativePath)
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
	newItem := "- " + glyph + " " + id + " " + title
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

// RenameTitle rewrites the H1 title of a document.
func RenameTitle(root, relativePath, title string) error {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(text), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			lines[i] = "# " + title
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
		}
	}
	return fmt.Errorf("the document holds no H1 title")
}
