//go:build linux || darwin

package journal

// flushWant is the flushes of each case of TestFlushes on Linux and macOS: a name is made durable by the sync of its
// folder, and a created folder by the sync of its parent.
var flushWant = map[string]int64{
	"the lock alone":                           0,
	"an update of one file":                    6,
	"a create in a folder that exists":         6,
	"a delete of one file":                     5,
	"a create in a folder that does not exist": 7,
	"an update of two files":                   7,
}
