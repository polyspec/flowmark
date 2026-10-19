package ops

import "github.com/polyspec/flowmark/packages/go/catalog"

// The answer of the schema operation: the catalog, each table in declaration order with its storage, key, settings,
// columns, keys and state machine (spec 40 The faces). A setting the table does not declare is absent.
type schemaAnswer struct {
	ManifestHash string        `json:"manifest_hash"`
	Tables       []schemaTable `json:"tables"`
}

type schemaTable struct {
	Name         string         `json:"name"`
	Storage      schemaStorage  `json:"storage"`
	Key          string         `json:"key"`
	Title        string         `json:"title,omitempty"`
	Body         string         `json:"body,omitempty"`
	Order        string         `json:"order,omitempty"`
	Parent       string         `json:"parent,omitempty"`
	Markdown     []string       `json:"markdown"`
	Columns      []schemaColumn `json:"columns"`
	ForeignKeys  []schemaFK     `json:"foreign_keys"`
	Uniques      [][]string     `json:"uniques"`
	StateMachine *schemaMachine `json:"state_machine"`
}

type schemaStorage struct {
	Kind       string `json:"kind"`
	Shape      string `json:"shape,omitempty"`
	Parent     string `json:"parent,omitempty"`
	ForeignKey string `json:"foreign_key,omitempty"`
}

type schemaColumn struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Null     bool    `json:"null"`
	Identity bool    `json:"identity"`
	Default  *string `json:"default"`
}

type schemaFK struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns"`
	Table      string   `json:"table"`
	References []string `json:"references"`
}

type schemaMachine struct {
	Column      string             `json:"column"`
	States      []string           `json:"states"`
	Initial     []string           `json:"initial"`
	Transitions []schemaTransition `json:"transitions"`
	Terminals   []schemaTerminal   `json:"terminals"`
	History     schemaHistory      `json:"history"`
	Limits      []schemaLimit      `json:"limits"`
	Checkboxes  map[string]string  `json:"checkboxes"`
}

type schemaTransition struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Require []string `json:"require"`
}

type schemaTerminal struct {
	State   string   `json:"state"`
	Require []string `json:"require"`
}

type schemaHistory struct {
	Table string `json:"table"`
	Row   string `json:"row"`
	From  string `json:"from"`
	To    string `json:"to"`
	At    string `json:"at"`
}

type schemaLimit struct {
	State string `json:"state"`
	Count int64  `json:"count"`
}

// orEmpty returns xs, or an empty slice for nil, so the answer writes [] rather than null.
func orEmpty[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

// schemaOf returns the schema answer of the catalog cat.
func schemaOf(cat *catalog.Catalog) schemaAnswer {
	out := schemaAnswer{ManifestHash: cat.ManifestHash, Tables: []schemaTable{}}
	for _, tb := range cat.Tables {
		t := schemaTable{
			Name:        tb.Name,
			Storage:     schemaStorage{Kind: string(tb.Storage.Kind), Shape: string(tb.Storage.Shape), Parent: tb.Storage.Parent, ForeignKey: tb.Storage.ParentForeignKey},
			Key:         tb.Key,
			Title:       tb.Title,
			Body:        tb.Body,
			Order:       tb.Order,
			Parent:      tb.Hierarchy(),
			Markdown:    orEmpty(tb.Markdown),
			Columns:     []schemaColumn{},
			ForeignKeys: []schemaFK{},
			Uniques:     orEmpty(tb.Uniques),
		}
		for _, c := range tb.Columns {
			col := schemaColumn{Name: c.Name, Type: c.Type.String(), Null: c.Null, Identity: c.Identity}
			if d := c.Default; d != nil {
				v := d.Literal
				if d.Now {
					v = "now"
				}
				col.Default = &v
			}
			t.Columns = append(t.Columns, col)
		}
		for _, fk := range tb.ForeignKeys {
			t.ForeignKeys = append(t.ForeignKeys, schemaFK{Name: fk.Name, Columns: orEmpty(fk.Columns), Table: fk.Table, References: orEmpty(fk.References)})
		}
		if sm := tb.StateMachine; sm != nil {
			m := &schemaMachine{
				Column:      sm.Column,
				States:      orEmpty(sm.States),
				Initial:     orEmpty(sm.Initial),
				Transitions: []schemaTransition{},
				Terminals:   []schemaTerminal{},
				History:     schemaHistory(sm.History),
				Limits:      []schemaLimit{},
				Checkboxes:  sm.Checkboxes,
			}
			if m.Checkboxes == nil {
				m.Checkboxes = map[string]string{}
			}
			for _, tr := range sm.Transitions {
				m.Transitions = append(m.Transitions, schemaTransition{From: tr.From, To: tr.To, Require: orEmpty(tr.Require)})
			}
			for _, tm := range sm.Terminals {
				m.Terminals = append(m.Terminals, schemaTerminal{State: tm.State, Require: orEmpty(tm.Require)})
			}
			for _, l := range sm.Limits {
				m.Limits = append(m.Limits, schemaLimit(l))
			}
			t.StateMachine = m
		}
		out.Tables = append(out.Tables, t)
	}
	return out
}
