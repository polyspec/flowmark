// Package app is the checklist application: the local issue model of spec 100-checklist —
// the state machine a document declares in its Mermaid block, the transition over a corpus
// and the commands.
package app

import (
	"fmt"
	"strconv"
	"strings"
)

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

// StateSchema is the schema a document declares: the states with their glyphs and labels,
// the transitions with their evidence rules, and the initial state — what a board needs to
// build itself. The Mermaid block of the document is the only authority.
type StateSchema struct {
	States      []StateDef      `json:"states"`
	Transitions []TransitionDef `json:"transitions"`
	Initial     string          `json:"initial,omitempty"`
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

// StateByName finds a state of the schema by name; nil when the schema holds none.
func (s StateSchema) StateByName(name string) *StateDef {
	for i := range s.States {
		if s.States[i].Name == name {
			return &s.States[i]
		}
	}
	return nil
}

// StateByGlyph finds a state of the schema by glyph; nil when the schema holds none.
func (s StateSchema) StateByGlyph(glyph string) *StateDef {
	for i := range s.States {
		if s.States[i].Glyph == glyph {
			return &s.States[i]
		}
	}
	return nil
}

// Edge finds the declared transition between two state names; nil when the schema holds none.
func (s StateSchema) Edge(from, to string) *TransitionDef {
	for i := range s.Transitions {
		if s.Transitions[i].From == from && s.Transitions[i].To == to {
			return &s.Transitions[i]
		}
	}
	return nil
}

// GlyphOfInitial returns the glyph of the declared initial state; the schema holds none
// without an initial state.
func (s StateSchema) GlyphOfInitial() string {
	if s.Initial == "" {
		return ""
	}
	if state := s.StateByName(s.Initial); state != nil {
		return state.Glyph
	}
	return ""
}
