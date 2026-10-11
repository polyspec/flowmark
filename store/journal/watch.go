package journal

// wakeKind is what a lock watch reports.
type wakeKind int

const (
	// wakeLock: the lock file was removed or renamed, the notification overflowed, or the watched folder itself
	// changed; it starts exactly one acquisition attempt.
	wakeLock wakeKind = iota
	// wakeHolderExit: the watched holder's process exited; it judges the holder again and starts no attempt.
	wakeHolderExit
	// wakeError: the watch failed; the wait ends with err.
	wakeError
)

type wake struct {
	kind wakeKind
	err  error
}

// fileID identifies a file by device and inode; the macOS watch compares the lock it watches with the lock a try
// read.
type fileID struct{ dev, ino uint64 }

// lockWatch is the OS change notification that drives a lock wait (spec 40, the lock wait): inotify on Linux, kqueue
// on macOS, ReadDirectoryChangesW on Windows.
type lockWatch interface {
	// armLock is called after every try that found the lock held, with the identity of the lock that try read; it
	// reports whether the lock changed since that try, which starts one more attempt at once.
	armLock(judged fileID) (retry bool, err error)
	// armHolder watches the exit of a holder of this host, replacing the holder watched before.
	armHolder(h Holder) error
	// wakes delivers the wakes in the order they happen.
	wakes() <-chan wake
	close() error
}

// wakeBuffer is the capacity of a watch's wake channel. A wake that finds it full is dropped: the wakes already
// queued start the attempt it would start, as an overflow starts one attempt for any number of lost events.
const wakeBuffer = 16

func send(ch chan wake, w wake) {
	select {
	case ch <- w:
	default:
	}
}
