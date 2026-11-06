// Package cost counts the work of the engine's operations: the files a read reads from the disk, visits and parses,
// the rows it places, indexes and walks, the rows a write copies, the files a comparison compares, the stored rows a request
// forms, the rows a statement reads and writes in its SQLite database, the acquisitions of the corpus lock, and the flushes that wait for the disk (plan,
// Ruling: an operation costs what it touches). An operation that costs what it touches counts the same on a corpus of any size; the counts are how a test
// says so. A nil *Counts counts nothing.
package cost

import "sync/atomic"

// Counter names one count.
type Counter int

const (
	// FilesRead is a folder or a file the corpus tree read from the disk.
	FilesRead Counter = iota
	// FilesVisited is a document a pass of the corpus read took, from the disk or from its memo.
	FilesVisited
	// FilesParsed is a document a pass of the corpus read parsed, its memo holding no parse of the same bytes.
	FilesParsed
	// RowsPlaced is a row a pass of the corpus read placed in its table.
	RowsPlaced
	// RowsIndexed is a row the executor's corpus entered in its index.
	RowsIndexed
	// RowsWalked is a record the executor's corpus placed in the walk of a document: each row of a document it walks
	// again, and each row a write appended to a document.
	RowsWalked
	// IndexEntries is an entry the executor's corpus wrote to its index: each locator of a key and a document that a
	// change of the index wrote, the locators it kept and those it added.
	IndexEntries
	// RowsCopied is a row a write copied into its post-image.
	RowsCopied
	// RowsScanned is a record of the executor's corpus that a write or its checks read: each record a lookup of the index
	// returned, and each record of a table a write walked. The walks of a table's storage order that fill a database are
	// its rows-stored.
	RowsScanned
	// FilesCompared is a file a comparison of two corpora compared.
	FilesCompared
	// RowsStored is a stored row a request formed from the corpus.
	RowsStored
	// DatabaseRowsRead is a row a statement's code read from its SQLite database.
	DatabaseRowsRead
	// DatabaseRowsWritten is a row a fill or a refresh wrote into the SQLite database.
	DatabaseRowsWritten
	// LocksTaken is an acquisition of the corpus lock: a lock record written and removed.
	LocksTaken
	// Flushes is a wait of the journal for the disk: the sync of a file or a folder, and on Windows a write-through
	// move. A corpus that does not wait for its writes (`flowmark serve --synchronous off`) makes none.
	Flushes
	counters
)

var names = [counters]string{
	FilesRead:           "files-read",
	FilesVisited:        "files-visited",
	FilesParsed:         "files-parsed",
	RowsPlaced:          "rows-placed",
	RowsIndexed:         "rows-indexed",
	RowsWalked:          "rows-walked",
	IndexEntries:        "index-entries",
	RowsCopied:          "rows-copied",
	RowsScanned:         "rows-scanned",
	FilesCompared:       "files-compared",
	RowsStored:          "rows-stored",
	DatabaseRowsRead:    "database-rows-read",
	DatabaseRowsWritten: "database-rows-written",
	LocksTaken:          "locks-taken",
	Flushes:             "flushes",
}

// String is the name of the counter.
func (c Counter) String() string { return names[c] }

// Counters are every counter, in order.
func Counters() []Counter {
	out := make([]Counter, counters)
	for i := range out {
		out[i] = Counter(i)
	}
	return out
}

// Counts are the counts of one corpus, which its operations add to at once.
type Counts struct {
	n [counters]atomic.Int64
}

// Add adds n to the counter c.
func (k *Counts) Add(c Counter, n int) {
	if k != nil {
		k.n[c].Add(int64(n))
	}
}

// Snapshot returns the counts at this moment.
func (k *Counts) Snapshot() Snapshot {
	var s Snapshot
	if k != nil {
		for i := range s {
			s[i] = k.n[i].Load()
		}
	}
	return s
}

// Snapshot is the value of every counter at one moment.
type Snapshot [counters]int64

// Get is the value of the counter c.
func (s Snapshot) Get(c Counter) int64 { return s[c] }

// Since is what each counter added after the snapshot before.
func (s Snapshot) Since(before Snapshot) Snapshot {
	var out Snapshot
	for i := range s {
		out[i] = s[i] - before[i]
	}
	return out
}
