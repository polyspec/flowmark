package irexec

import "testing"

// TestIsWrite pins the write-kind classification of a request message: the write
// path refuses a write in a read-only transaction before it runs, by this one
// classification of the executor's.
func TestIsWrite(t *testing.T) {
	f := baseFixture(t)
	for kind, write := range map[string]bool{
		"insert": true, "update": true, "delete": true, "restore": true,
		"one": false, "all": false, "count": false, "group_count": false, "sum": false, "avg": false, "paginate": false,
	} {
		if got := IsWrite(f.msg(kind, "tags", nil)); got != write {
			t.Errorf("IsWrite(%s) = %v, want %v", kind, got, write)
		}
	}
	for _, bad := range []string{"", "{", `{"request":null}`, `[]`} {
		if IsWrite([]byte(bad)) {
			t.Errorf("IsWrite(%q) = true for a message that holds no write", bad)
		}
	}
}
