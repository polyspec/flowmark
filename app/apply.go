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

// resolveTransition holds the document's own state machine against one move: the state the
// item carries, the requested target, the edge between them and the required evidence.
func resolveTransition(schema StateSchema, relativePath, id, fromGlyph, target, evidence string, document *codec.Document) (StateDef, TransitionDef, error) {
	fromState := schema.StateByGlyph(fromGlyph)
	if fromState == nil {
		return StateDef{}, TransitionDef{}, fmt.Errorf("the item %q carries the state %q, which the state machine of %s does not declare", id, fromGlyph, relativePath)
	}
	// the caller may pass the glyph ([~]) or the name (doing)
	targetState := schema.StateByName(target)
	if targetState == nil {
		targetState = schema.StateByGlyph(target)
	}
	if targetState == nil {
		return StateDef{}, TransitionDef{}, fmt.Errorf("the state %q is not in the state machine of %s", target, relativePath)
	}
	edge := schema.Edge(fromState.Name, targetState.Name)
	if edge == nil {
		return StateDef{}, TransitionDef{}, fmt.Errorf("the state machine of %s holds no transition %s -> %s", relativePath, fromState.Name, targetState.Name)
	}
	if !requiredMet(edge.Requires, evidence) {
		return StateDef{}, TransitionDef{}, fmt.Errorf("the transition %s -> %s requires %s on the item", fromState.Name, targetState.Name, strings.Join(edge.Requires, ", "))
	}
	// the WIP gate holds the kit glyph [~] count within the headroom over the [o] count
	if targetState.Glyph == "[~]" && !wipHolds(document) {
		return StateDef{}, TransitionDef{}, fmt.Errorf("the WIP gate refuses another [~] item: %d in progress against %d done", countGlyph(document, "[~]"), countGlyph(document, "[o]"))
	}
	return *targetState, *edge, nil
}

// writeDocument writes the lines back to the document file.
func writeDocument(path string, lines []string) error {
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// itemForm names how a document lists its items: as checklist lines or as a table. A
// document that holds both refuses writes — one grammar per document.
func itemForm(document *codec.Document) (*codec.Table, error) {
	table := codec.ItemsTable(document)
	if table != nil && len(document.Items) > 0 {
		return nil, fmt.Errorf("the document mixes checklist lines and an items table")
	}
	return table, nil
}

// findRow finds a row of an items table by ID; -1 when the table holds none.
func findRow(table *codec.Table, id string) int {
	column := table.Column("id")
	if column < 0 {
		return -1
	}
	for i, row := range table.Rows {
		if row[column] == id {
			return i
		}
	}
	return -1
}

// replaceCell rewrites one cell of a table line, keeping the column layout.
func replaceCell(line string, column int, value string) string {
	parts := strings.Split(line, "|")
	if column+1 >= len(parts)-1 {
		return line
	}
	parts[column+1] = " " + value + " "
	return strings.Join(parts, "|")
}

// rowLine builds one table row line for the values of an item, in the column order the
// header declares.
func rowLine(table *codec.Table, values map[string]string) string {
	cells := make([]string, len(table.Headers))
	for i, header := range table.Headers {
		cells[i] = values[header]
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

// Transition applies one state transition to an item of a document: the state machine the
// document declares in its Mermaid block holds the edges, the required fields and the glyphs —
// no built-in table exists. The evidence carries the fields the edge label names. The item
// lives in the checklist lines or in the items table — both carry the same grammar.
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
	table, err := itemForm(document)
	if err != nil {
		return err
	}
	lines := strings.Split(string(text), "\n")
	if table != nil {
		row := findRow(table, id)
		if row < 0 {
			return fmt.Errorf("the item %q is not in %s", id, relativePath)
		}
		stateColumn := table.Column("state")
		targetState, _, err := resolveTransition(schema, relativePath, id, table.Rows[row][stateColumn], target, evidence, document)
		if err != nil {
			return err
		}
		at := table.RowLines[row] - 1
		lines[at] = replaceCell(lines[at], stateColumn, targetState.Glyph)
		return writeDocument(path, lines)
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
	targetState, _, err := resolveTransition(schema, relativePath, id, item.State, target, evidence, document)
	if err != nil {
		return err
	}
	at := item.Line - 1
	replace := strings.Replace(lines[at], item.State, targetState.Glyph, 1)
	if replace == lines[at] {
		return fmt.Errorf("the state %q is not on line %d", item.State, item.Line)
	}
	lines[at] = replace
	return writeDocument(path, lines)
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
	table, err := itemForm(document)
	if err != nil {
		return err
	}
	lines := strings.Split(string(text), "\n")
	if table != nil {
		if findRow(table, id) >= 0 {
			return fmt.Errorf("the item %q is already in %s", id, relativePath)
		}
		newRow := rowLine(table, map[string]string{"state": glyph, "id": id, "title": title})
		insert := len(lines)
		if len(table.RowLines) > 0 {
			insert = table.RowLines[len(table.RowLines)-1] // after the last data row
		}
		lines = append(lines[:insert], append([]string{newRow}, lines[insert:]...)...)
		return writeDocument(path, lines)
	}
	for _, item := range document.Items {
		if item.ID == id {
			return fmt.Errorf("the item %q is already in %s", id, relativePath)
		}
	}
	insert := len(lines)
	if len(document.Items) > 0 {
		last := document.Items[len(document.Items)-1]
		insert = last.Line + len(last.Continuations)
	}
	newItem := "- " + glyph + " " + id + " " + title
	lines = append(lines[:insert], append([]string{newItem}, lines[insert:]...)...)
	return writeDocument(path, lines)
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
	table, err := itemForm(document)
	if err != nil {
		return err
	}
	if table != nil {
		row := findRow(table, id)
		if row < 0 {
			return fmt.Errorf("the item %q is not in %s", id, relativePath)
		}
		lines := strings.Split(string(text), "\n")
		at := table.RowLines[row] - 1
		rest := append(append([]string{}, lines[:at]...), lines[at+1:]...)
		return writeDocument(path, rest)
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
	ids := make([]string, 0, len(document.Items))
	for _, item := range document.Items {
		ids = append(ids, item.ID)
	}
	if table := codec.ItemsTable(document); table != nil {
		column := table.Column("id")
		if column >= 0 {
			for _, row := range table.Rows {
				ids = append(ids, row[column])
			}
		}
	}
	highest := 0
	for _, item := range ids {
		id := item
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
	table, err := itemForm(document)
	if err != nil {
		return err
	}
	if table != nil {
		moving := findRow(table, id)
		if moving < 0 {
			return fmt.Errorf("the item %q is not in %s", id, relativePath)
		}
		anchor := -1
		if afterID != "" {
			anchor = findRow(table, afterID)
			if anchor < 0 {
				return fmt.Errorf("the anchor %q is not in %s", afterID, relativePath)
			}
		}
		lines := strings.Split(string(text), "\n")
		rowText := lines[table.RowLines[moving]-1]
		without := append(append([]string{}, lines[:table.RowLines[moving]-1]...), lines[table.RowLines[moving]:]...)
		insert := table.RowLines[len(table.RowLines)-1] - 1 // after the last row by default
		if anchor >= 0 {
			insert = table.RowLines[anchor] - 1
			if anchor > moving {
				insert-- // the cut row shifts the anchor up
			}
		}
		out := append(append([]string{}, without[:insert]...), rowText)
		out = append(out, without[insert:]...)
		return writeDocument(path, out)
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
	itemLines := append([]string{}, lines[block:blockEnd]...)
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
	table, err := itemForm(document)
	if err != nil {
		return err
	}
	if table != nil {
		row := findRow(table, id)
		if row < 0 {
			return fmt.Errorf("the item %q is not in %s", id, relativePath)
		}
		lines := strings.Split(string(text), "\n")
		at := table.RowLines[row] - 1
		lines[at] = replaceCell(lines[at], table.Column("title"), title)
		return writeDocument(path, lines)
	}
	for _, item := range document.Items {
		if item.ID == id {
			lines := strings.Split(string(text), "\n")
			at := item.Line - 1
			prefix := "- " + item.State + " " + id + " "
			if strings.HasPrefix(lines[at], prefix) {
				lines[at] = prefix + title
				return writeDocument(path, lines)
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
