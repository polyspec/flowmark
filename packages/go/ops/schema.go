package ops

import "github.com/polyspec/flowmark/packages/go/catalog"

// The answer of the schema operation: the catalog, each table in declaration order with its storage, key, settings,
// columns, keys and checkbox (spec 40 The faces). A setting the table does not declare is absent.
type schemaAnswer struct {
	ManifestHash string        `json:"manifest_hash"`
	Tables       []schemaTable `json:"tables"`
}

type schemaTable struct {
	Name        string          `json:"name"`
	Storage     schemaStorage   `json:"storage"`
	Key         string          `json:"key"`
	Title       string          `json:"title,omitempty"`
	Body        string          `json:"body,omitempty"`
	Order       string          `json:"order,omitempty"`
	Parent      string          `json:"parent,omitempty"`
	Markdown    []string        `json:"markdown"`
	Columns     []schemaColumn  `json:"columns"`
	ForeignKeys []schemaFK      `json:"foreign_keys"`
	Uniques     [][]string      `json:"uniques"`
	Checkbox    *schemaCheckbox `json:"checkbox"`
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

// schemaCheckbox is the checkbox of a table: the column a list row writes as its glyph and the glyph of each value
// (spec 30 Markdown settings).
type schemaCheckbox struct {
	Column string            `json:"column"`
	Glyphs map[string]string `json:"glyphs"`
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
		if cb := tb.Checkbox; cb != nil {
			t.Checkbox = &schemaCheckbox{Column: cb.Column, Glyphs: cb.Glyphs}
		}
		out.Tables = append(out.Tables, t)
	}
	return out
}
