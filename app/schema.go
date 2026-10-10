package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SchemaMermaid writes the state machine as a Mermaid stateDiagram-v2 with a left-to-right
// direction: the states as nodes, the transitions as edges, the requires as edge labels,
// and the initial and terminal states as entry and exit points.
func SchemaMermaid(schema StateSchema) string {
	var out strings.Builder
	out.WriteString("stateDiagram-v2\n    direction LR\n")
	hasIncoming := map[string]bool{}
	hasOutgoing := map[string]bool{}
	for _, t := range schema.Transitions {
		hasIncoming[t.To] = true
		hasOutgoing[t.From] = true
	}
	// state descriptions: the name is the identifier, the glyph+label is the description
	for _, s := range schema.States {
		if s.Glyph != "" {
			fmt.Fprintf(&out, "    %s : %s %s\n", s.Name, s.Glyph, s.Label)
		} else {
			fmt.Fprintf(&out, "    %s : %s\n", s.Name, s.Label)
		}
	}
	// the initial state comes first; a schema without one falls back to the first state
	initial := schema.Initial
	if initial == "" && len(schema.States) > 0 {
		initial = schema.States[0].Name
	}
	fmt.Fprintf(&out, "    [*] --> %s\n", initial)
	for _, t := range schema.Transitions {
		if len(t.Requires) > 0 {
			fmt.Fprintf(&out, "    %s --> %s: %s\n", t.From, t.To, strings.Join(t.Requires, ", "))
		} else {
			fmt.Fprintf(&out, "    %s --> %s\n", t.From, t.To)
		}
	}
	for _, s := range schema.States {
		if !hasOutgoing[s.Name] && s.Name != initial {
			fmt.Fprintf(&out, "    %s --> [*]\n", s.Name)
		}
	}
	return out.String()
}

// ApplySchema rewrites the checklist after a schema change and cascades the effects on
// the items: a renamed glyph rewrites every item that carries it, a removed state removes
// every item in it (with its children), and a new state or a transition change touches
// nothing — it only governs the future. The new schema lands in the Mermaid block, the
// only place a document declares its state machine.
func ApplySchema(root, relativePath string, old, new StateSchema) error {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(text), "\n")
	oldGlyphs := map[string]bool{}
	newGlyphs := map[string]bool{}
	for _, s := range old.States {
		oldGlyphs[s.Glyph] = true
	}
	for _, s := range new.States {
		newGlyphs[s.Glyph] = true
	}
	// Pass 1: rename — an old glyph that maps to a new name with the same position.
	renames := map[string]string{} // old glyph → new glyph
	for i, os := range old.States {
		if i < len(new.States) && new.States[i].Name == os.Name && new.States[i].Glyph != os.Glyph {
			renames[os.Glyph] = new.States[i].Glyph
		}
	}
	if len(renames) > 0 {
		for i, line := range lines {
			for from, to := range renames {
				if strings.Contains(line, from) && strings.HasPrefix(strings.TrimSpace(line), "- ") {
					lines[i] = strings.Replace(line, from, to, 1)
				}
			}
		}
	}
	// Pass 2: remove — a glyph that was in old but not in new.
	removed := map[string]bool{}
	for glyph := range oldGlyphs {
		if !newGlyphs[glyph] && renames[glyph] == "" {
			removed[glyph] = true
		}
	}
	if len(removed) > 0 {
		var kept []string
		skip := 0
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if skip > 0 {
				if strings.HasPrefix(trimmed, "  ") {
					continue // a continuation of a removed item
				}
				skip = 0
			}
			isItem := strings.HasPrefix(trimmed, "- ")
			matched := false
			for glyph := range removed {
				if isItem && strings.HasPrefix(trimmed[2:], glyph) {
					matched = true
					break
				}
			}
			if matched {
				skip = 1 // skip the next continuation lines
				continue
			}
			kept = append(kept, line)
		}
		lines = kept
	}
	// Pass 3: write the new schema into the Mermaid block.
	newText, err := writeSchema(strings.Join(lines, "\n"), new)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(newText), 0o644)
}

// writeSchema replaces the diagram of the first Mermaid block with the new schema, or
// inserts one after the H1 title when the document declares none.
func writeSchema(text string, schema StateSchema) (string, error) {
	diagram := SchemaMermaid(schema)
	lines := strings.Split(text, "\n")
	inBlock := false
	replaced := false
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if trimmed == "```mermaid" {
				inBlock = true
			}
			out = append(out, line)
			continue
		}
		// inside the first mermaid block: stop at the fence, drop the old diagram
		if trimmed == "```" {
			for _, diagramLine := range strings.Split(strings.TrimRight(diagram, "\n"), "\n") {
				out = append(out, "    "+diagramLine)
			}
			out = append(out, "```")
			inBlock = false
			replaced = true
			continue
		}
	}
	if replaced {
		return strings.Join(out, "\n"), nil
	}
	if inBlock {
		// an unterminated block: refuse rather than guess
		return "", fmt.Errorf("the Mermaid block of the document is unterminated")
	}
	// no block: insert one after the H1 title
	fenced := []string{"", "## State machine", "", "```mermaid"}
	for _, diagramLine := range strings.Split(strings.TrimRight(diagram, "\n"), "\n") {
		fenced = append(fenced, "    "+diagramLine)
	}
	fenced = append(fenced, "```", "")
	insert := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			insert = i + 1
			break
		}
	}
	joined := append(append([]string{}, lines[:insert]...), fenced...)
	joined = append(joined, lines[insert:]...)
	return strings.Join(joined, "\n"), nil
}

// ParseMermaid reads a Mermaid stateDiagram-v2 text into a schema: `[*] --> X` names the
// initial state, `X : glyph label` describes a state, `A --> B: requires` is a transition,
// and `B --> [*]` marks a terminal. A `direction` line is ignored.
func ParseMermaid(text string) (StateSchema, error) {
	lines := strings.Split(text, "\n")
	var states []StateDef
	var transitions []TransitionDef
	stateOrder := []string{}
	seen := map[string]bool{}
	initial := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "stateDiagram-v2") || strings.HasPrefix(line, "direction") {
			continue
		}
		// `X : glyph label` — a state description
		if parts := strings.SplitN(line, " : ", 2); len(parts) == 2 && !strings.Contains(line, "-->") {
			name := strings.TrimSpace(parts[0])
			rest := strings.TrimSpace(parts[1])
			glyph := rest
			label := name
			if strings.HasPrefix(rest, "[") {
				if end := strings.Index(rest, "]"); end > 0 {
					glyph = rest[:end+1]
					label = strings.TrimSpace(rest[end+1:])
				}
			}
			if label == "" {
				label = name // no explicit label: the name is the label
			}
			if !seen[name] {
				seen[name] = true
				stateOrder = append(stateOrder, name)
			}
			states = append(states, StateDef{Glyph: glyph, Name: name, Label: label})
			continue
		}
		// `A --> B` or `A --> B: label` — a transition
		if arrow := strings.Index(line, "-->"); arrow >= 0 {
			from := strings.TrimSpace(line[:arrow])
			rest := strings.TrimSpace(line[arrow+3:])
			to := rest
			requires := ""
			if colon := strings.Index(rest, ":"); colon >= 0 {
				to = strings.TrimSpace(rest[:colon])
				requires = strings.TrimSpace(rest[colon+1:])
			}
			if from == "[*]" {
				// initial state — the first one wins
				if initial == "" {
					initial = to
				}
				if !seen[to] {
					seen[to] = true
					stateOrder = append(stateOrder, to)
				}
				continue
			}
			if to == "[*]" {
				// terminal — the state was already seen
				continue
			}
			if !seen[from] {
				seen[from] = true
				stateOrder = append(stateOrder, from)
			}
			if !seen[to] {
				seen[to] = true
				stateOrder = append(stateOrder, to)
			}
			t := TransitionDef{From: from, To: to}
			if requires != "" {
				t.Requires = strings.Split(requires, ", ")
			}
			transitions = append(transitions, t)
			continue
		}
	}
	if len(states) == 0 {
		return StateSchema{}, fmt.Errorf("the mermaid diagram holds no state")
	}
	// order states by first appearance
	ordered := make([]StateDef, 0, len(states))
	for _, name := range stateOrder {
		for _, s := range states {
			if s.Name == name {
				ordered = append(ordered, s)
				break
			}
		}
	}
	return StateSchema{States: ordered, Transitions: transitions, Initial: initial}, nil
}

// SchemaFromMermaidBlock reads the schema from the first fenced mermaid block of the
// document that holds a stateDiagram-v2. A document without one holds no state machine —
// there is no default.
func SchemaFromMermaidBlock(root, relativePath string) (StateSchema, error) {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return StateSchema{}, err
	}
	// find the fenced mermaid block
	inBlock := false
	var block []string
	for _, line := range strings.Split(string(text), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "```mermaid" {
			inBlock = true
			block = nil
			continue
		}
		if inBlock && trimmed == "```" {
			schema, err := ParseMermaid(strings.Join(block, "\n"))
			if err == nil {
				return schema, nil
			}
			inBlock = false
			continue
		}
		if inBlock {
			block = append(block, line)
		}
	}
	return StateSchema{}, fmt.Errorf("the document holds no Mermaid stateDiagram-v2 block")
}
