//go:build windows

package journal

// flushWant is the flushes of each case of TestFlushes on Windows: a name is made durable by a write-through move, and
// a folder is never synced.
var flushWant = map[string]int64{
	"the lock alone":                           0,
	"an update of one file":                    14,
	"a create in a folder that exists":         12,
	"a delete of one file":                     10,
	"a create in a folder that does not exist": 13,
	"an update of two files":                   21,
}
