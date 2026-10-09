package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
