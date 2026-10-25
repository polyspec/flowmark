package journal

import (
	"sync"
	"time"
)

// Recorder is an Observer that keeps every event in order and wakes the callers that wait for one: the observer of a
// host that reads the events of a corpus as they are published, such as a test of the code that publishes them.
type Recorder struct {
	mu     sync.Mutex
	events []Event
	notify chan struct{}
}

// Observe records e and wakes the callers of Wait.
func (r *Recorder) Observe(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	if r.notify != nil {
		close(r.notify)
		r.notify = nil
	}
}

// Events returns the events recorded since the last Reset, in order.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

// Reset forgets the events recorded so far.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

// Wait blocks until n of the events recorded since the last Reset satisfy match, and returns them with true; when
// timeout passes first, it returns those found with false.
func (r *Recorder) Wait(n int, match func(Event) bool, timeout time.Duration) ([]Event, bool) {
	deadline := time.After(timeout)
	for {
		r.mu.Lock()
		var found []Event
		for _, e := range r.events {
			if match(e) {
				found = append(found, e)
			}
		}
		if len(found) >= n {
			r.mu.Unlock()
			return found, true
		}
		if r.notify == nil {
			r.notify = make(chan struct{})
		}
		ch := r.notify
		r.mu.Unlock()
		select {
		case <-ch:
		case <-deadline:
			return found, false
		}
	}
}
