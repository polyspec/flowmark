package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// patience bounds every wait of a test for something the code under test must publish; it is a guard against a hung
// test, never the condition a test checks.
const patience = 60 * time.Second

// testNow is the request clock of every request of the tests.
const testNow = "2026-10-11 09:00:00.123456"

// tagsDBS is the catalog of the tests: one document table with an identity key, a unique key and a check.
const tagsDBS = `dbspec 1 board

table tags {
  id i64 identity
  label varchar(16)
  kind varchar(16)
  primary key (id)
  unique uq_tags_label (label)
  check ck_tags_kind (kind in ('topic', 'area'))
  settings {
    store document table
  }
}
`

const tagsDoc = "# tags\n| id | label | kind |\n| --- | --- | --- |\n| 1 | go | topic |\n| 2 | ops | area |\n"

// recorder is the journal observer of a test: it keeps every event in order and wakes the waits for one.
type recorder struct {
	mu     sync.Mutex
	events []journal.Event
	notify chan struct{}
}

func (r *recorder) Observe(e journal.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	if r.notify != nil {
		close(r.notify)
		r.notify = nil
	}
}

// waitFor blocks until an event that match accepts has been published, and returns it.
func (r *recorder) waitFor(t testing.TB, what string, match func(journal.Event) bool) journal.Event {
	t.Helper()
	deadline := time.After(patience)
	for {
		r.mu.Lock()
		for _, e := range r.events {
			if match(e) {
				r.mu.Unlock()
				return e
			}
		}
		if r.notify == nil {
			r.notify = make(chan struct{})
		}
		ch := r.notify
		r.mu.Unlock()
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("waited %s for %s", patience, what)
		}
	}
}

// count returns how many events match accepts.
func (r *recorder) count(match func(journal.Event) bool) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if match(e) {
			n++
		}
	}
	return n
}

// of matches the events of kind whose holder is the session.
func of(kind journal.Kind, session string) func(journal.Event) bool {
	return func(e journal.Event) bool { return e.Kind == kind && strings.HasSuffix(e.Holder, "/"+session) }
}

// fixture is a server on a corpus in a temporary root.
type fixture struct {
	t      testing.TB
	root   string
	name   string
	token  string
	hash   string
	rec    *recorder
	srv    *Server
	addr   string
	served chan error
	mu     sync.Mutex
	ended  map[string]chan struct{}
	logs   []string
}

// start writes the tags corpus into a temporary root and serves it with the idle bound idle and the lock wait wait.
func start(t testing.TB, idle, wait time.Duration) *fixture {
	t.Helper()
	root := t.TempDir()
	write(t, root, "dbspec/test.dbs", tagsDBS)
	write(t, root, "tags.flowmark.md", tagsDoc)
	cat, diags := catalog.Load(root)
	if len(diags) > 0 {
		t.Fatalf("catalog: %+v", diags)
	}
	e, err := irexec.New(cat)
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	j, err := journal.New(root, rec)
	if err != nil {
		t.Fatal(err)
	}
	c, err := writepath.Open(context.Background(), cat, j, wait)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("closing the corpus: %v", err)
		}
	})
	token, err := Token(root, false)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, root: root, name: filepath.Base(root), token: token, hash: e.Model().ManifestHash, rec: rec,
		served: make(chan error, 1), ended: map[string]chan struct{}{}}
	srv, err := New(c, f.name, token, idle, func(format string, a ...any) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.logs = append(f.logs, format)
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.ended = func(session string) { close(f.endedCh(session)) }
	f.srv = srv
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	f.addr = ln.Addr().String()
	go func() { f.served <- srv.Serve(ln) }()
	t.Cleanup(func() {
		if err := srv.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		select {
		case err := <-f.served:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(patience):
			t.Errorf("Serve did not return after Close")
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.logs) > 0 {
			t.Errorf("the server logged failures: %q", f.logs)
		}
	})
	return f
}

// endedCh returns the channel closed when the connection of session ends.
func (f *fixture) endedCh(session string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch, ok := f.ended[session]
	if !ok {
		ch = make(chan struct{})
		f.ended[session] = ch
	}
	return ch
}

// waitEnded blocks until the connection of session has ended.
func (f *fixture) waitEnded(session string) {
	f.t.Helper()
	select {
	case <-f.endedCh(session):
	case <-time.After(patience):
		f.t.Fatalf("waited %s for the end of %s", patience, session)
	}
}

func write(t testing.TB, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(rel, "dbspec/") {
		if err := corpustest.WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
			t.Fatal(err)
		}
	}
}

// client is one connection to the server.
type client struct {
	t    testing.TB
	conn net.Conn
	r    *bufio.Reader
}

// dial opens a connection without connecting the session.
func (f *fixture) dial() *client {
	f.t.Helper()
	conn, err := net.Dial("tcp", f.addr)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			f.t.Error(err)
		}
	})
	return &client{t: f.t, conn: conn, r: bufio.NewReader(conn)}
}

// connect opens a connection and connects its session.
func (f *fixture) connect() *client {
	f.t.Helper()
	c := f.dial()
	c.want(f.connectMsg(f.name, f.token), `{"ok":true}`)
	return c
}

func (f *fixture) connectMsg(corpus, token string) string {
	return marshal(f.t, map[string]any{"op": "connect", "corpus": corpus, "token": token})
}

// send writes one line.
func (c *client) send(line string) {
	c.t.Helper()
	if _, err := io.WriteString(c.conn, line+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

// recv reads one answer line.
func (c *client) recv() string {
	c.t.Helper()
	if err := c.conn.SetReadDeadline(time.Now().Add(patience)); err != nil {
		c.t.Fatal(err)
	}
	line, err := c.r.ReadString('\n')
	if err != nil {
		c.t.Fatalf("reading an answer: %v (read %q)", err, line)
	}
	return strings.TrimSuffix(line, "\n")
}

// want sends line and requires the answer want.
func (c *client) want(line, want string) {
	c.t.Helper()
	c.send(line)
	if got := c.recv(); got != want {
		c.t.Fatalf("%s\nanswered %s\nwant      %s", line, got, want)
	}
}

// fails sends line and requires an error answer of code whose message holds each of parts.
func (c *client) fails(line, code string, parts ...string) {
	c.t.Helper()
	c.send(line)
	c.wantError(code, parts...)
}

// wantError reads an answer and requires it to be an error of code whose message holds each of parts.
func (c *client) wantError(code string, parts ...string) {
	c.t.Helper()
	got := c.recv()
	var a struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	dec := json.NewDecoder(strings.NewReader(got))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil || a.Error == nil {
		c.t.Fatalf("answered %s, want an error %s (%v)", got, code, err)
	}
	if a.Error.Code != code || a.Error.Message == "" {
		c.t.Fatalf("answered %s, want the code %s and a message", got, code)
	}
	for _, p := range parts {
		if !strings.Contains(a.Error.Message, p) {
			c.t.Fatalf("answered %s, want a message naming %q", got, p)
		}
	}
}

// closed requires the server to have closed the connection: the next read ends without a line.
func (c *client) closed() {
	c.t.Helper()
	if err := c.conn.SetReadDeadline(time.Now().Add(patience)); err != nil {
		c.t.Fatal(err)
	}
	line, err := c.r.ReadString('\n')
	if err != io.EOF || line != "" {
		c.t.Fatalf("read %q, %v; want the connection closed", line, err)
	}
}

func marshal(t testing.TB, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// msg is a request message of kind on tags with the fields of body and the parameters params.
func (f *fixture) msg(kind string, body map[string]any, params ...any) string {
	req := map[string]any{"ir_version": 1, "manifest_hash": f.hash, "kind": kind, "entity": "tags", "n_params": len(params)}
	for k, v := range body {
		req[k] = v
	}
	if params == nil {
		params = []any{}
	}
	return marshal(f.t, map[string]any{"ir_version": 1, "manifest_hash": f.hash, "request": req, "params": params, "now": testNow})
}

func (f *fixture) insertTag(label, kind string) string {
	return f.msg("insert", map[string]any{"set": []any{map[string]any{"column": "label", "p": 0}, map[string]any{"column": "kind", "p": 1}}}, label, kind)
}

func (f *fixture) countTags() string { return f.msg("count", nil) }

// tags returns the text of the tags document.
func (f *fixture) tags() string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, "tags.flowmark.md"))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

const (
	ok           = `{"ok":true}`
	begin        = `{"op":"begin"}`
	beginRO      = `{"op":"begin","read_only":true}`
	commit       = `{"op":"commit"}`
	rollback     = `{"op":"rollback"}`
	noIdle       = time.Hour
	waitLong     = patience
	count2       = `{"count":2}`
	count3       = `{"count":3}`
	insertedWeb3 = `{"affected":1,"keys":[3]}`
)

var tokenLine = regexp.MustCompile(`^\{"token":"[0-9a-f]{32}"\}\n$`)

// TestToken pins the token file (spec 40 The server): written once on the first start, kept across starts, replaced
// only by --new-token, one token for servers starting at once, and a malformed file or a symlink refused.
func TestToken(t *testing.T) {
	file := func(root string) string { return filepath.Join(root, ".flowmark", "server.json") }
	read := func(t *testing.T, root string) string {
		t.Helper()
		b, err := os.ReadFile(file(root))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	t.Run("40#the-server: the first start writes a random token; later starts keep it", func(t *testing.T) {
		root := t.TempDir()
		tok, err := Token(root, false)
		if err != nil {
			t.Fatal(err)
		}
		text := read(t, root)
		if !tokenLine.MatchString(text) || !strings.Contains(text, `"`+tok+`"`) {
			t.Fatalf("server.json = %q, token %q", text, tok)
		}
		fi, err := os.Stat(file(root))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("server.json mode %v, want 0600", fi.Mode().Perm())
		}
		again, err := Token(root, false)
		if err != nil || again != tok {
			t.Fatalf("second start: %q, %v; want %q", again, err, tok)
		}
		entries, err := os.ReadDir(filepath.Join(root, ".flowmark"))
		if err != nil || len(entries) != 1 {
			t.Fatalf(".flowmark holds %v (%v), want server.json alone", entries, err)
		}
	})
	t.Run("40#the-server: --new-token replaces the token", func(t *testing.T) {
		root := t.TempDir()
		tok, err := Token(root, false)
		if err != nil {
			t.Fatal(err)
		}
		renewed, err := Token(root, true)
		if err != nil || renewed == tok || !tokenLine.MatchString(read(t, root)) || !strings.Contains(read(t, root), renewed) {
			t.Fatalf("renewed %q (was %q), %v; file %q", renewed, tok, err, read(t, root))
		}
		kept, err := Token(root, false)
		if err != nil || kept != renewed {
			t.Fatalf("after renewal: %q, %v; want %q", kept, err, renewed)
		}
	})
	t.Run("40#the-server: servers starting at once keep one token", func(t *testing.T) {
		root := t.TempDir()
		const n = 8
		toks := make([]string, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() { toks[i], errs[i] = Token(root, false) })
		}
		wg.Wait()
		for i := range n {
			if errs[i] != nil || toks[i] != toks[0] {
				t.Fatalf("start %d: %q, %v; start 0: %q", i, toks[i], errs[i], toks[0])
			}
		}
		if !strings.Contains(read(t, root), toks[0]) {
			t.Fatalf("server.json %q, token %q", read(t, root), toks[0])
		}
	})
	for _, bad := range []string{"", "{}\n", `{"token":"abc"}` + "\n", `{"token":"0123456789ABCDEF0123456789abcdef"}` + "\n",
		`{"token":"0123456789abcdef0123456789abcdef"}`, `{"token":"0123456789abcdef0123456789abcdef","x":1}` + "\n"} {
		t.Run("40#the-server: a malformed token file refuses the start: "+bad, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, ".flowmark/server.json", bad)
			_, err := Token(root, false)
			if err == nil || !strings.Contains(err.Error(), "--new-token") || !strings.Contains(err.Error(), "server.json") {
				t.Fatalf("err = %v, want a refusal naming server.json and --new-token", err)
			}
			if tok, err := Token(root, true); err != nil || !tokenLine.MatchString(read(t, root)) || !strings.Contains(read(t, root), tok) {
				t.Fatalf("--new-token over a malformed file: %q, %v", tok, err)
			}
		})
	}
	t.Run("40#the-server: a symlink at server.json refuses the start", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "elsewhere.json", `{"token":"0123456789abcdef0123456789abcdef"}`+"\n")
		if err := os.MkdirAll(filepath.Join(root, ".flowmark"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "elsewhere.json"), file(root)); err != nil {
			t.Fatalf("no symlink: %v", err)
		}
		for _, renew := range []bool{false, true} {
			if _, err := Token(root, renew); err == nil || !strings.Contains(err.Error(), "symbolic link") {
				t.Fatalf("renew %v: err = %v, want a symlink refusal", renew, err)
			}
		}
	})
}

// TestConnect pins the connect message (spec 40 The server): the first message names the corpus and the token; a
// mismatch, or another message first, is CONFIG and closes the connection.
func TestConnect(t *testing.T) {
	f := start(t, noIdle, 0)
	t.Run("40#the-server: the corpus and the token connect the session", func(t *testing.T) {
		c := f.connect()
		c.want(f.countTags(), count2)
	})
	for name, line := range map[string]string{
		"another token":             f.connectMsg(f.name, strings.Repeat("0", 32)),
		"another corpus":            f.connectMsg(f.name+"x", f.token),
		"no token":                  `{"op":"connect","corpus":"` + f.name + `"}`,
		"a request first":           f.countTags(),
		"a control message first":   begin,
		"a line that is not JSON":   "hello",
		"a connect with more field": `{"op":"connect","corpus":"` + f.name + `","token":"` + f.token + `","x":1}`,
	} {
		t.Run("40#the-server: "+name+" is CONFIG and closes the connection", func(t *testing.T) {
			c := f.dial()
			c.fails(line, "CONFIG", "connect")
			c.closed()
		})
	}
	t.Run("40#the-server: a second connect is CONFIG and keeps the session", func(t *testing.T) {
		c := f.connect()
		c.fails(f.connectMsg(f.name, f.token), "CONFIG", "connect")
		c.want(f.countTags(), count2)
	})
}

// TestMessages pins the lines of a session (spec 40 The server, spec 60 The wire): request answers, the ok line, the
// error line with the orm code, and IR_INVALID for a line of no message.
func TestMessages(t *testing.T) {
	f := start(t, noIdle, 0)
	c := f.connect()
	t.Run("60#the-wire: a request is answered by the executor's answer", func(t *testing.T) {
		c.want(f.insertTag("web", "area"), insertedWeb3)
		c.want(f.countTags(), count3)
		if !strings.HasSuffix(f.tags(), "| 3 | web | area |\n") {
			t.Fatalf("tags:\n%s", f.tags())
		}
	})
	t.Run("60#the-wire: a failed request is the error line with its orm code", func(t *testing.T) {
		c.fails(f.insertTag("new", "other"), "CONSTRAINT", "ck_tags_kind")
		bad := strings.Replace(f.countTags(), f.hash, "sha256:"+strings.Repeat("0", 64), 2)
		c.fails(bad, "SCHEMA_HASH_MISMATCH")
	})
	for name, line := range map[string]string{
		"a line that is not JSON":           "hello",
		"a JSON array":                      "[]",
		"an empty line":                     "",
		"an unknown op":                     `{"op":"vacuum"}`,
		"an op that is not a string":        `{"op":1}`,
		"a name on begin":                   `{"op":"begin","name":"a"}`,
		"read_only on commit":               `{"op":"commit","read_only":true}`,
		"read_only that is not a boolean":   `{"op":"begin","read_only":"yes"}`,
		"a field no control message takes":  `{"op":"rollback","x":1}`,
		"a name that is not a string":       `{"op":"savepoint","name":1}`,
		"a request message with more field": strings.Replace(f.countTags(), `"now"`, `"x":1,"now"`, 1),
	} {
		t.Run("40#the-server: "+name+" is IR_INVALID", func(t *testing.T) {
			c.fails(line, "IR_INVALID")
			c.want(f.countTags(), count3)
		})
	}
	t.Run("40#the-server: pipelined messages are answered in order", func(t *testing.T) {
		c.send(f.countTags() + "\n" + begin + "\n" + f.insertTag("db", "topic") + "\n" + rollback + "\n" + f.countTags())
		for _, want := range []string{count3, ok, `{"affected":1,"keys":[4]}`, ok, count3} {
			if got := c.recv(); got != want {
				t.Fatalf("answered %s, want %s", got, want)
			}
		}
	})
}

// TestControl pins the control messages over the wire (spec 40 Transactions).
func TestControl(t *testing.T) {
	t.Run("40#transactions: a transaction commits its writes with its savepoints", func(t *testing.T) {
		f := start(t, noIdle, 0)
		c := f.connect()
		c.want(begin, ok)
		c.want(f.insertTag("web", "area"), insertedWeb3)
		c.want(`{"op":"savepoint","name":"a"}`, ok)
		c.want(f.insertTag("db", "topic"), `{"affected":1,"keys":[4]}`)
		c.want(f.countTags(), `{"count":4}`)
		c.want(`{"op":"rollback_to","name":"a"}`, ok)
		c.want(`{"op":"release","name":"a"}`, ok)
		c.want(f.countTags(), count3)
		if f.tags() != tagsDoc {
			t.Fatalf("the corpus changed before the commit:\n%s", f.tags())
		}
		c.want(commit, ok)
		if !strings.HasSuffix(f.tags(), "| 3 | web | area |\n") {
			t.Fatalf("tags:\n%s", f.tags())
		}
	})
	t.Run("40#transactions: a rollback discards the writes", func(t *testing.T) {
		f := start(t, noIdle, 0)
		c := f.connect()
		c.want(begin, ok)
		c.want(f.insertTag("web", "area"), insertedWeb3)
		c.want(rollback, ok)
		c.want(f.countTags(), count2)
		if f.tags() != tagsDoc {
			t.Fatalf("tags:\n%s", f.tags())
		}
	})
	t.Run("40#transactions: a read-only transaction refuses a write as READ_ONLY", func(t *testing.T) {
		f := start(t, noIdle, 0)
		c := f.connect()
		c.want(beginRO, ok)
		c.want(f.countTags(), count2)
		c.fails(f.insertTag("web", "area"), "READ_ONLY")
		c.want(commit, ok)
		c.want(`{"op":"begin","read_only":false}`, ok)
		c.want(f.insertTag("web", "area"), insertedWeb3)
		c.want(commit, ok)
	})
	t.Run("40#transactions: a control message its state does not allow is CONFIG", func(t *testing.T) {
		f := start(t, noIdle, 0)
		c := f.connect()
		for _, line := range []string{commit, rollback, `{"op":"savepoint","name":"a"}`, `{"op":"release","name":"a"}`, `{"op":"rollback_to","name":"a"}`} {
			c.fails(line, "CONFIG", "no transaction")
		}
		c.want(begin, ok)
		c.fails(begin, "CONFIG", "already open")
		c.fails(`{"op":"savepoint"}`, "CONFIG", "name")
		c.fails(`{"op":"release","name":"b"}`, "CONFIG", "b")
		c.fails(`{"op":"rollback_to"}`, "CONFIG")
		c.want(f.countTags(), count2)
		c.want(rollback, ok)
	})
	t.Run("40#failure-vocabulary: a lock wait that ends is CANCELED on the wire", func(t *testing.T) {
		f := start(t, noIdle, 0)
		a := f.connect()
		a.want(begin, ok)
		b := f.connect()
		b.fails(f.countTags(), "CANCELED", "serve-1")
		b.fails(begin, "CANCELED")
		a.want(rollback, ok)
		b.want(f.countTags(), count2)
	})
}

// TestClosedConnection pins a connection that closes (spec 40 The server, The lock): its transaction is rolled back,
// the lock released, and a lock wait of its request ended.
func TestClosedConnection(t *testing.T) {
	t.Run("40#the-lock: a closed connection rolls its transaction back and releases the lock", func(t *testing.T) {
		f := start(t, noIdle, waitLong)
		a := f.connect()
		a.want(begin, ok)
		a.want(f.insertTag("web", "area"), insertedWeb3)
		if err := a.conn.Close(); err != nil {
			t.Fatal(err)
		}
		f.waitEnded("serve-1")
		f.rec.waitFor(t, "the release of serve-1", of(journal.LockReleased, "serve-1"))
		b := f.connect()
		b.want(f.countTags(), count2)
		if f.tags() != tagsDoc {
			t.Fatalf("tags:\n%s", f.tags())
		}
	})
	t.Run("40#the-lock: a closed connection ends its request's lock wait and runs no later message", func(t *testing.T) {
		f := start(t, noIdle, waitLong)
		a := f.connect()
		a.want(begin, ok)
		b := f.connect()
		b.send(begin + "\n" + f.insertTag("web", "area") + "\n" + commit)
		f.rec.waitFor(t, "the wait of serve-2", func(e journal.Event) bool {
			return e.Kind == journal.LockWaiting && strings.HasSuffix(e.Holder, "/serve-1")
		})
		if err := b.conn.Close(); err != nil {
			t.Fatal(err)
		}
		f.waitEnded("serve-2")
		a.want(f.countTags(), count2)
		a.want(rollback, ok)
		if n := f.rec.count(of(journal.LockAcquired, "serve-2")); n != 0 {
			t.Fatalf("serve-2 acquired the lock %d times after its connection closed", n)
		}
		if f.tags() != tagsDoc {
			t.Fatalf("a message after the close ran:\n%s", f.tags())
		}
	})
	t.Run("40#the-server: closing the server rolls every open transaction back", func(t *testing.T) {
		f := start(t, noIdle, waitLong)
		a := f.connect()
		a.want(begin, ok)
		a.want(f.insertTag("web", "area"), insertedWeb3)
		if err := f.srv.Close(); err != nil {
			t.Fatal(err)
		}
		a.closed()
		f.rec.waitFor(t, "the release of serve-1", of(journal.LockReleased, "serve-1"))
		if f.tags() != tagsDoc {
			t.Fatalf("tags:\n%s", f.tags())
		}
		if _, err := net.Dial("tcp", f.addr); err == nil {
			t.Fatal("the server still accepts connections after Close")
		}
	})
}

// TestIdle pins --tx-idle-ms (spec 40 The server): a transaction idle past it is rolled back, its lock released and its
// connection closed after a CONNECTION_LOST line; a session without a transaction is never idle.
func TestIdle(t *testing.T) {
	const idle = 50 * time.Millisecond
	t.Run("40#the-server: an idle transaction is rolled back and its connection closed", func(t *testing.T) {
		f := start(t, idle, waitLong)
		a := f.connect()
		a.want(begin, ok)
		a.want(f.insertTag("web", "area"), insertedWeb3)
		a.wantError("CONNECTION_LOST", "idle", "rolled back")
		a.closed()
		f.rec.waitFor(t, "the release of serve-1", of(journal.LockReleased, "serve-1"))
		b := f.connect()
		b.want(f.countTags(), count2)
		if f.tags() != tagsDoc {
			t.Fatalf("tags:\n%s", f.tags())
		}
	})
	t.Run("40#the-server: a session without a transaction is not idle", func(t *testing.T) {
		f := start(t, idle, waitLong)
		a := f.connect()
		a.want(begin, ok)
		a.want(rollback, ok)
		<-time.After(4 * idle)
		a.want(f.countTags(), count2)
	})
}

// TestNew pins the server's own refusals: an idle bound that is not positive, and a corpus name or token missing.
func TestNew(t *testing.T) {
	for name, idle := range map[string]time.Duration{"zero": 0, "negative": -time.Millisecond} {
		t.Run("40#the-server: an idle bound "+name+" is refused", func(t *testing.T) {
			if _, err := New(nil, "c", strings.Repeat("0", 32), idle, nil); err == nil || !strings.Contains(err.Error(), "--tx-idle-ms") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	t.Run("40#the-server: Listen binds loopback", func(t *testing.T) {
		ln, err := Listen(0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := ln.Close(); err != nil {
				t.Error(err)
			}
		}()
		a, ok := ln.Addr().(*net.TCPAddr)
		if !ok {
			t.Fatalf("the address %v is no TCP address", ln.Addr())
		}
		if !a.IP.Equal(net.IPv4(127, 0, 0, 1)) || a.Port == 0 {
			t.Fatalf("bound %v", a)
		}
	})
}
