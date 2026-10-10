// Package app is the checklist application: the local issue model of spec 100-checklist —
// the state machine, the transition over a corpus and the commands.
package app

import (
	"fmt"
	"strconv"
	"strings"
)

// The five states and their kit glyphs.
var glyphs = map[string]string{
	"waiting":    "[ ]",
	"doing":      "[~]",
	"done":       "[o]",
	"bypassed":   "[!]",
	"impossible": "[-]",
}

var states = map[string]string{
	"[ ]": "waiting",
	"[~]": "doing",
	"[o]": "done",
	"[!]": "bypassed",
	"[-]": "impossible",
}

// StateOf maps a kit glyph to its state name; an unknown glyph maps to none.
func StateOf(glyph string) (string, bool) {
	state, ok := states[glyph]
	return state, ok
}

// GlyphOf maps a state name to its kit glyph.
func GlyphOf(state string) string {
	return glyphs[state]
}

// Rule is one allowed transition with the fields it requires on the item.
type Rule struct {
	From, To string
	Requires []string
}

// The transition table: done is terminal — a related issue spawns a superseded item instead
// (spec 100-checklist).
var transitions = []Rule{
	{From: "waiting", To: "doing"},
	{From: "doing", To: "done", Requires: []string{"evidence"}},
	{From: "doing", To: "bypassed", Requires: []string{"cause", "retry"}},
	{From: "bypassed", To: "doing"},
	{From: "waiting", To: "impossible", Requires: []string{"cause"}},
}

// CanTransition names the transition between two states, or none when the table refuses it.
func CanTransition(from, to string) (Rule, bool) {
	for _, transition := range transitions {
		if transition.From == from && transition.To == to {
			return transition, true
		}
	}
	return Rule{}, false
}

// SupersededID returns the next suffixed ID of a base for a related issue of a done item:
// T8.0.14 spawns T8.0.14-1, then T8.0.14-2.
func SupersededID(base string, existing []string) string {
	highest := 0
	prefix := base + "-"
	for _, id := range existing {
		if strings.HasPrefix(id, prefix) {
			if number, err := strconv.Atoi(strings.TrimPrefix(id, prefix)); err == nil && number > highest {
				highest = number
			}
		}
	}
	return fmt.Sprintf("%s-%d", base, highest+1)
}

// StateSchema is the schema of a checklist: the states with their glyphs and labels, and
// the transitions with their evidence rules — what a board needs to build itself.
type StateSchema struct {
	States      []StateDef      `json:"states"`
	Transitions []TransitionDef `json:"transitions"`
}

// StateDef is one column of a board.
type StateDef struct {
	Glyph string `json:"glyph"`
	Name  string `json:"name"`
	Label string `json:"label"`
}

// TransitionDef is one allowed transition with its required evidence fields.
type TransitionDef struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Requires []string `json:"requires,omitempty"`
}

// Schema returns the state machine as a board schema. The labels are the defaults; a
// config/board.json in the corpus overrides them.
func Schema() StateSchema {
	labels := map[string]string{
		"waiting":    "대기",
		"doing":      "진행 중",
		"done":       "완료",
		"bypassed":   "우회",
		"impossible": "불가",
	}
	states := make([]StateDef, 0, len(glyphs))
	for _, name := range []string{"waiting", "doing", "done", "bypassed", "impossible"} {
		states = append(states, StateDef{Glyph: glyphs[name], Name: name, Label: labels[name]})
	}
	seen := map[string]bool{}
	transitionDefs := []TransitionDef{}
	for _, t := range transitions {
		key := t.From + "->" + t.To
		if seen[key] {
			continue
		}
		seen[key] = true
		transitionDefs = append(transitionDefs, TransitionDef{From: t.From, To: t.To, Requires: t.Requires})
	}
	return StateSchema{States: states, Transitions: transitionDefs}
}
