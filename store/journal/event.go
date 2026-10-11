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
)

// Event is one published event. Path is the absolute path of the lock file or of the stage directory; Holder is the
// lock holder's record for the lock events and empty otherwise.
type Event struct {
	Kind   Kind
	Path   string
	Holder string
}

// Observer receives every event of a Corpus, synchronously, in the goroutine that caused it.
type Observer interface {
	Observe(Event)
}
