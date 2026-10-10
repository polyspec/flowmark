package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/polyspec/flowmark/codec"
)

// SchemaFromDocument reads the state machine that the checklist document declares in its
// frontmatter — the literal block under the state_machine key holds the JSON. A document
// without a declaration gives the default five-state machine.
func SchemaFromDocument(root, relativePath string) (StateSchema, error) {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	text, err := os.ReadFile(path)
	if err != nil {
		return StateSchema{}, err
	}
	document, _ := codec.Parse(string(text))
	for _, entry := range document.FM {
		if entry.Key != "state_machine" || len(entry.Literal) == 0 {
			continue
		}
		var schema StateSchema
		if err := json.Unmarshal([]byte(strings.Join(entry.Literal, "\n")), &schema); err == nil && len(schema.States) > 0 {
			return schema, nil
		}
	}
	return Schema(), nil
}

// SchemaMermaid writes the state machine as a Mermaid stateDiagram-v2: the states as nodes,
// the transitions as edges, the requires as edge labels, and the initial and terminal
// states as entry and exit points.
func SchemaMermaid(schema StateSchema) string {
	var out strings.Builder
	out.WriteString("stateDiagram-v2\n")
	hasIncoming := map[string]bool{}
	hasOutgoing := map[string]bool{}
	for _, t := range schema.Transitions {
		hasIncoming[t.To] = true
		hasOutgoing[t.From] = true
	}
	for i, s := range schema.States {
		if i == 0 || !hasIncoming[s.Name] {
			fmt.Fprintf(&out, "    [*] --> %s\n", s.Label)
		}
	}
	for _, t := range schema.Transitions {
		from := labelOf(schema, t.From)
		to := labelOf(schema, t.To)
		if len(t.Requires) > 0 {
			fmt.Fprintf(&out, "    %s --> %s: %s\n", from, to, strings.Join(t.Requires, ", "))
		} else {
			fmt.Fprintf(&out, "    %s --> %s\n", from, to)
		}
	}
	for _, s := range schema.States {
		if !hasOutgoing[s.Name] {
			fmt.Fprintf(&out, "    %s --> [*]\n", s.Label)
		}
	}
	return out.String()
}

func labelOf(schema StateSchema, name string) string {
	for _, s := range schema.States {
		if s.Name == name {
			return s.Label
		}
	}
	return name
}

// ApplySchema rewrites the checklist after a schema change and cascades the effects on
// the items: a renamed glyph rewrites every item that carries it, a removed state removes
// every item in it (with its children), and a new state or a transition change touches
// nothing — it only governs the future.
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
	// Pass 3: write the new schema into the frontmatter literal block.
	newText, err := writeSchema(strings.Join(lines, "\n"), new)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(newText), 0o644)
}

// writeSchema replaces the state_machine literal block in the frontmatter with the new
// JSON, or inserts one when the document declares none.
func writeSchema(text string, schema StateSchema) (string, error) {
	jsonText, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", err
	}
	document, _ := codec.Parse(text)
	hasBlock := false
	for _, entry := range document.FM {
		if entry.Key == "state_machine" {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		// insert a frontmatter block after the header comments and title
		lines := strings.Split(text, "\n")
		insert := 0
		for i, line := range lines {
			if strings.HasPrefix(line, "# ") {
				insert = i + 1
				break
			}
		}
		block := []string{"---", "state_machine: |"}
		for _, line := range strings.Split(string(jsonText), "\n") {
			block = append(block, "  "+line)
		}
		block = append(block, "---", "")
		lines = append(lines[:insert], append(block, lines[insert:]...)...)
		return strings.Join(lines, "\n"), nil
	}
	// replace the existing literal block
	lines := strings.Split(text, "\n")
	inBlock := false
	replaced := false
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "state_machine: |") {
			inBlock = true
			out = append(out, line)
			for _, jl := range strings.Split(string(jsonText), "\n") {
				out = append(out, "  "+jl)
			}
			replaced = true
			continue
		}
		if inBlock {
			if strings.HasPrefix(line, "  ") {
				continue // skip the old block content
			}
			inBlock = false
		}
		out = append(out, line)
	}
	if !replaced {
		return "", fmt.Errorf("the state_machine block was not found for replacement")
	}
	return strings.Join(out, "\n"), nil
}

// ParseMermaid reads a Mermaid stateDiagram-v2 text into a schema: `[*] --> X` names the
// initial state, `X : glyph label` describes a state, `A --> B: requires` is a transition,
// and `B --> [*]` marks a terminal.
func ParseMermaid(text string) (StateSchema, error) {
	lines := strings.Split(text, "\n")
	var states []StateDef
	var transitions []TransitionDef
	stateOrder := []string{}
	seen := map[string]bool{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "stateDiagram-v2") {
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
				// initial state — record the order
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
	return StateSchema{States: ordered, Transitions: transitions}, nil
}

// SchemaFromMermaidBlock reads the schema from the first fenced mermaid block of the
// document that holds a stateDiagram-v2.
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
	return Schema(), nil
}
