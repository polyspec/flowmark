package store

import "testing"

func TestRelLink(t *testing.T) {
	cases := []struct {
		from, to, anchor, want string
	}{
		{"tasks/T1.flowmark.md", "tasks/T2.flowmark.md", "", "T2.flowmark.md"},
		{"tasks/T2.flowmark.md", "tasks/T1.flowmark.md", "", "T1.flowmark.md"},
		{"comments/C5.flowmark.md", "posts/P3.flowmark.md", "", "../posts/P3.flowmark.md"},
		{"posts/P3.flowmark.md", "people.flowmark.md", "", "../people.flowmark.md"},
		{"a/b/c.flowmark.md", "x.flowmark.md", "", "../../x.flowmark.md"},
		{"items.flowmark.md", "items.flowmark.md", "items-row-t2", "#items-row-t2"},
		{"items.flowmark.md", "items.flowmark.md", "", "items.flowmark.md"},
		{"posts/P3.flowmark.md", "posts/P4.flowmark.md", "", "P4.flowmark.md"},
		{"posts/P3.flowmark.md", "posts/P4.flowmark.md", "x", "P4.flowmark.md#x"},
		{"items.flowmark.md", "posts/P3.flowmark.md", "posts-row-p3", "posts/P3.flowmark.md#posts-row-p3"},
		{"tasks/T1.flowmark.md", "tasks/T1.flowmark.md", "", "T1.flowmark.md"},
	}
	for _, c := range cases {
		got, err := RelLink(c.from, c.to, c.anchor)
		if err != nil || got != c.want {
			t.Fatalf("RelLink(%q, %q, %q) = %q, %v; want %q", c.from, c.to, c.anchor, got, err, c.want)
		}
	}
}

func TestDetailAnchor(t *testing.T) {
	cases := []struct{ table, key, want string }{
		{"items", "T2", "items-row-t2"},
		{"items", "Ab-1", "items-row-ab-1"},
		{"people", "7", "people-row-7"},
	}
	for _, c := range cases {
		if got := DetailAnchor(c.table, c.key); got != c.want {
			t.Fatalf("DetailAnchor(%q, %q) = %q, want %q", c.table, c.key, got, c.want)
		}
	}
}
