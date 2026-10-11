package server

import (
	"bufio"
	"io"
	"sync"
	"time"
)

// inbox holds the lines a connection has sent and not yet answered. It is filled by the connection's reader, which
// runs while a request does, so the end of the connection is learned at once and not after the request.
type inbox struct {
	mu    sync.Mutex
	lines [][]byte
	done  bool
	// ready holds one wake of next after a line or the end arrives.
	ready chan struct{}
}

// The states of next.
const (
	lineRead = iota
	lineEnded
	lineIdle
)

// read reads the lines of r until it ends; a last line without a line feed is part of a connection that ended, and is
// not a message.
func (in *inbox) read(r io.Reader) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			break
		}
		in.mu.Lock()
		in.lines = append(in.lines, line)
		in.mu.Unlock()
		in.wake()
	}
	in.mu.Lock()
	in.done = true
	in.mu.Unlock()
	in.wake()
}

func (in *inbox) wake() {
	select {
	case in.ready <- struct{}{}:
	default:
	}
}

// next returns the next line; lineEnded when the connection has ended, its unanswered lines discarded with its
// session; and lineIdle when idle delivers before a line arrives.
func (in *inbox) next(idle <-chan time.Time) ([]byte, int) {
	for {
		in.mu.Lock()
		switch {
		case in.done:
			in.mu.Unlock()
			return nil, lineEnded
		case len(in.lines) > 0:
			line := in.lines[0]
			in.lines = in.lines[1:]
			in.mu.Unlock()
			return line, lineRead
		}
		in.mu.Unlock()
		select {
		case <-in.ready:
		case <-idle:
			return nil, lineIdle
		}
	}
}
