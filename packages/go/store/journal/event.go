package journal

// Kind names an event of the lock or the journal.
type Kind string

// The events, published in the order they happen.
const (
	// LockAcquired: the lock file appeared with this holder's record.
	LockAcquired Kind = "lock.acquired"
	// LockReleased: the holder removed its lock file.
	LockReleased Kind = "lock.released"
	// LockWaiting: an acquisition found the lock held by a running holder and waits for its release; published once
	// per holder it waits for, after its watch is armed.
	LockWaiting Kind = "lock.waiting"
	// LockCleared: `flowmark unlock` removed the lock of an ended holder; Holder names it.
	LockCleared Kind = "lock.cleared"
	// Prepared: the stage holds every post-image, every pre-image and the prepare record (spec 40, steps 3–5).
	Prepared Kind = "journal.prepared"
	// Applied: every rename is performed and the commit record is written (steps 6–7).
	Applied Kind = "journal.applied"
	// Collected: the stage is removed (step 8).
	Collected Kind = "journal.collected"
	// Replayed: recovery completed a stage that holds a prepare or a commit record.
	Replayed Kind = "journal.replayed"
	// Discarded: recovery removed a stage without a prepare record.
	Discarded Kind = "journal.discarded"
	// CorpusRead: the corpus tree read a folder or a file from the disk; Path names it.
	CorpusRead Kind = "corpus.read"
	// CorpusChanged: a notification, or the write path's Forget, dropped what the corpus tree held of Path; the next
	// read reads it from the disk again.
	CorpusChanged Kind = "corpus.changed"
	// CorpusOverflow: the change notifications overflowed, so the corpus tree dropped everything and the next read
	// reads the whole corpus again; Path is the corpus root.
	CorpusOverflow Kind = "corpus.overflow"
	// ScaleExceeded: a commit wrote a file past the scale envelope (spec 40 The scale envelope); Path is the file and
	// Detail names its size and the limit. The commit is written all the same.
	ScaleExceeded Kind = "scale.exceeded"
)

// Event is one published event. Path is the absolute path of the lock file or of the stage directory; Holder is the
// lock holder's record for the lock events and empty otherwise; Detail is the text a kind adds to its path, empty for
// the kinds that add none.
type Event struct {
	Kind   Kind
	Path   string
	Holder string
	Detail string
}

// Observer receives every event of a Corpus, synchronously, in the goroutine that caused it.
type Observer interface {
	Observe(Event)
}
