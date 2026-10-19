// Package server is `flowmark serve` (spec 40 The server): the resident face that orm clients reach over loopback
// TCP. A connection is a session of the write path; its JSON lines are request messages of the IR-executor protocol
// and control messages, each answered in order by one line. The first message connects the session with the DSN's
// corpus and token; an open transaction idle longer than the idle bound is rolled back and its connection closed.
package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// The orm codes (orm errors.yaml) of the server's own answers.
const (
	codeConfig         = "CONFIG"
	codeIRInvalid      = "IR_INVALID"
	codeConnectionLost = "CONNECTION_LOST"
	codeInternal       = "INTERNAL"
)

// Server serves one corpus through the write path.
type Server struct {
	c      *writepath.Corpus
	corpus string
	token  string
	idle   time.Duration
	logf   func(format string, a ...any)
	// ended is the seam of the tests that learns that the connection of a session has ended; nil outside the tests.
	ended func(session string)

	mu     sync.Mutex
	ln     net.Listener
	conns  map[net.Conn]struct{}
	n      int
	closed bool
	wg     sync.WaitGroup
}

// New returns the server of the corpus c, whose DSN name is corpus and whose token is token. idle is the bound of an
// open transaction's wait for its next message (--tx-idle-ms), positive. logf receives each failure the server cannot
// answer on a connection: a session that fails to end, a connection it cannot serve.
func New(c *writepath.Corpus, corpus, token string, idle time.Duration, logf func(format string, a ...any)) (*Server, error) {
	switch {
	case idle <= 0:
		return nil, fmt.Errorf("--tx-idle-ms %d is not positive; give the milliseconds an open transaction may wait for its next message", idle.Milliseconds())
	case c == nil:
		return nil, errors.New("the server has no corpus")
	case corpus == "":
		return nil, errors.New("the server has no corpus name")
	case !tokenForm(token):
		return nil, errors.New("the server token is not 32 lowercase hex digits")
	case logf == nil:
		return nil, errors.New("the server has no log for the failures it cannot answer")
	}
	return &Server{c: c, corpus: corpus, token: token, idle: idle, logf: logf, conns: map[net.Conn]struct{}{}}, nil
}

func tokenForm(s string) bool {
	return tokenFile.MatchString(`{"token":"` + s + `"}` + "\n")
}

// Listen binds 127.0.0.1:port, loopback TCP only; port 0 lets the operating system choose.
func Listen(port int) (net.Listener, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("--port %d is not a TCP port; give 1 to 65535, or 0 to let the operating system choose", port)
	}
	return net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

// Serve serves the connections of ln until Close, and returns nil then; it returns the failure of an accept that is
// not Close's, after closing the server. It waits for every connection to end.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	switch {
	case s.closed:
		s.mu.Unlock()
		return ln.Close()
	case s.ln != nil:
		s.mu.Unlock()
		return errors.New("the server is already serving")
	}
	s.ln = ln
	s.mu.Unlock()
	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				s.wg.Wait()
				return nil
			}
			return errors.Join(fmt.Errorf("accepting a connection: %w", err), s.Close())
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			s.closeConn(conn)
			continue
		}
		s.n++
		n := s.n
		s.conns[conn] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go s.handle(conn, "serve-"+strconv.Itoa(n))
	}
}

// Close stops listening and closes every connection, which rolls its open transaction back and ends its request's lock
// wait; it returns when every connection has ended. Closing again does nothing.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	var err error
	if s.ln != nil {
		if cerr := s.ln.Close(); cerr != nil && !errors.Is(cerr, net.ErrClosed) {
			err = fmt.Errorf("closing the listener: %w", cerr)
		}
	}
	for conn := range s.conns {
		s.closeConn(conn)
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}

// closeConn closes a connection the server is done with. A connection that is already closed is no fault; any other
// failure to close is logged, since no caller can answer for it.
func (s *Server) closeConn(conn net.Conn) {
	if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		s.logf("[serve] closing a connection: %v", err)
	}
}

// handle serves the connection conn as the session name.
func (s *Server) handle(conn net.Conn, name string) {
	defer func() {
		s.closeConn(conn)
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		if s.ended != nil {
			s.ended(name)
		}
		s.wg.Done()
	}()
	sess, err := s.c.Session(name)
	if err != nil {
		s.logf("[serve] %s: %v", name, err)
		return
	}
	end := func() {
		if err := sess.Close(); err != nil {
			s.logf("[serve] %s: ending the session: %v", name, err)
		}
	}
	defer end()
	// The connection is read while a request runs, so a closed connection ends the session — its transaction and its
	// request's lock wait — at once.
	in := &inbox{ready: make(chan struct{}, 1)}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		in.read(conn)
		end()
	}()
	w := bufio.NewWriter(conn)
	answer := func(line []byte) bool {
		if _, err := w.Write(line); err != nil {
			return false
		}
		if err := w.WriteByte('\n'); err != nil {
			return false
		}
		return w.Flush() == nil
	}
	connected := false
	for {
		var idle <-chan time.Time
		var timer *time.Timer
		if connected && sess.InTransaction() {
			timer = time.NewTimer(s.idle)
			idle = timer.C
		}
		line, state := in.next(idle)
		if timer != nil {
			timer.Stop()
		}
		switch state {
		case lineEnded:
			return
		case lineIdle:
			end()
			answer(errorLine(codeConnectionLost, fmt.Sprintf("the transaction was idle longer than --tx-idle-ms %d; it was rolled back, the lock released and the connection closed", s.idle.Milliseconds())))
			return
		}
		if !connected {
			if err := s.connect(line); err != nil {
				answer(errorLine(codeConfig, err.Error()))
				return
			}
			connected = true
			if !answer(okLine) {
				return
			}
			continue
		}
		if !answer(s.message(sess, line)) {
			return
		}
	}
}

// connect checks the first message of a connection: {"op": "connect", "corpus", "token"} with this server's corpus
// and token, the token compared in constant time.
func (s *Server) connect(line []byte) error {
	const form = `the first message of a connection is {"op":"connect","corpus":"<corpus>","token":"<token>"} with the DSN's corpus and token`
	var m struct {
		Op     string  `json:"op"`
		Corpus *string `json:"corpus"`
		Token  *string `json:"token"`
	}
	if err := strictDecode(line, &m); err != nil || m.Op != "connect" || m.Corpus == nil || m.Token == nil {
		return errors.New(form)
	}
	if *m.Corpus != s.corpus {
		return fmt.Errorf("connect: the corpus %q is not this server's corpus %q; give the corpus root's directory name in the DSN", *m.Corpus, s.corpus)
	}
	if subtle.ConstantTimeCompare([]byte(*m.Token), []byte(s.token)) != 1 {
		return fmt.Errorf("connect: the token is not the corpus's token; give the token of %s in the DSN", TokenPath)
	}
	return nil
}

// The fields each control message takes besides op.
var controlFields = map[string]map[string]bool{
	"connect":     {"corpus": true, "token": true},
	"begin":       {"read_only": true},
	"commit":      {},
	"rollback":    {},
	"savepoint":   {"name": true},
	"release":     {"name": true},
	"rollback_to": {"name": true},
}

// message answers one message of a connected session: a request message, or a control message when it has op.
func (s *Server) message(sess *writepath.Session, line []byte) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil || fields == nil {
		return errorLine(codeIRInvalid, "a message is one JSON object on one line")
	}
	rawOp, control := fields["op"]
	if !control {
		a, err := sess.Exec(context.Background(), line)
		if err != nil {
			return failureLine(err)
		}
		b, err := json.Marshal(a)
		if err != nil {
			return errorLine(codeInternal, "writing the answer: "+err.Error())
		}
		return b
	}
	var op string
	if err := json.Unmarshal(rawOp, &op); err != nil {
		return errorLine(codeIRInvalid, "op is not a string")
	}
	takes, ok := controlFields[op]
	if !ok {
		return errorLine(codeIRInvalid, fmt.Sprintf("op %q names no control message; give begin, commit, rollback, savepoint, release or rollback_to", op))
	}
	if op == "connect" {
		return errorLine(codeConfig, "connect: the session is already connected")
	}
	for f := range fields {
		if f != "op" && !takes[f] {
			return errorLine(codeIRInvalid, fmt.Sprintf("the control message %s takes no field %s", op, f))
		}
	}
	var c struct {
		ReadOnly bool   `json:"read_only"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(line, &c); err != nil {
		return errorLine(codeIRInvalid, fmt.Sprintf("the control message %s: %v", op, err))
	}
	var err error
	switch op {
	case "begin":
		err = sess.Begin(context.Background(), c.ReadOnly)
	case "commit":
		err = sess.Commit()
	case "rollback":
		err = sess.Rollback()
	case "savepoint":
		err = sess.Savepoint(c.Name)
	case "release":
		err = sess.Release(c.Name)
	case "rollback_to":
		err = sess.RollbackTo(c.Name)
	}
	if err != nil {
		return failureLine(err)
	}
	return okLine
}

// strictDecode decodes the one JSON object of b into v, refusing a field v does not name.
func strictDecode(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("more than one JSON value")
	}
	return nil
}

var okLine = []byte(`{"ok":true}`)

// errorLine is the answer of a failure: {"error": {"code", "message"}}.
func errorLine(code, msg string) []byte {
	b, err := json.Marshal(map[string]map[string]string{"error": {"code": code, "message": msg}})
	if err != nil {
		panic(err) // a map of strings always marshals
	}
	return b
}

// failureLine is the answer of a failure of the write path or the executor, with its orm code: the code of the first
// coded error in the error's tree, INTERNAL when none has one.
func failureLine(err error) []byte {
	switch e := err.(type) {
	case *writepath.Error:
		return errorLine(e.Code, e.Msg)
	case *irexec.Error:
		return errorLine(e.Code, e.Msg)
	}
	code := codeOf(err)
	if code == "" {
		code = codeInternal
	}
	return errorLine(code, err.Error())
}

func codeOf(err error) string {
	switch e := err.(type) {
	case *writepath.Error:
		return e.Code
	case *irexec.Error:
		return e.Code
	case interface{ Unwrap() []error }:
		for _, x := range e.Unwrap() {
			if c := codeOf(x); c != "" {
				return c
			}
		}
	case interface{ Unwrap() error }:
		return codeOf(e.Unwrap())
	}
	return ""
}
