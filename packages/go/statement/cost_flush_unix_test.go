//go:build linux || darwin

package statement_test

// lockFlushes is the flushes of an acquisition of the lock, and commitFlushes those of a commit that writes one file of a
// folder that exists, on Linux and macOS (T55; plan, Ruling: a commit waits for the disk as few times as its atomicity needs).
// TestFlushes of the journal pins the flushes of every kind of commit.
const lockFlushes, commitFlushes = 0, 17
