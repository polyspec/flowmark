package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// patience bounds every wait of a test for something the command must publish; it is a guard against a hung test,
// never the condition a test checks.
const patience = 60 * time.Second

const serveDBS = `dbspec 1 board

table tags {
  id i64 identity
  label varchar(16)
  primary key (id)
  settings {
    store document table
  }
}
`

// serveCorpus writes a corpus of one tags table into a temporary root and returns the root.
func serveCorpus(t *testing.T, dbs string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "board")
	for rel, text := range map[string]string{
		"dbspec/test.dbs":  dbs,
		"tags.flowmark.md": "# tags\n| id | label |\n| --- | --- |\n| 1 | go |\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := corpustest.WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
		t.Fatal(err)
	}
	return root
}

// syncBuffer is a buffer written by the command's goroutines and read by the test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestServeUsage pins the refusals of the command line (spec 40 The server): --port is required, a flag value must be
// of its kind, and the corpus is named once.
func TestServeUsage(t *testing.T) {
	root := serveCorpus(t, serveDBS)
	for name, args := range map[string][]string{
		"no corpus":                 {},
		"no --port":                 {root},
		"a --port that is no port":  {root, "--port", "http"},
		"a --port out of range":     {root, "--port", "70000"},
		"a negative --lock-wait-ms": {root, "--port", "0", "--lock-wait-ms", "-1"},
		"a --tx-idle-ms of 0":       {root, "--port", "0", "--tx-idle-ms", "0"},
		"an unknown flag":           {root, "--port", "0", "--remote"},
		"a second corpus":           {root, "--port", "0", root},
		"the corpus after the flag": {"--port", "0", root},
	} {
		t.Run("40#the-server: "+name+" is a usage error", func(t *testing.T) {
			var out, errOut syncBuffer
			if code := serve(args, &out, &errOut, make(chan os.Signal)); code != 2 {
				t.Fatalf("exit %d, want 2; stderr %s", code, errOut.String())
			}
			if !strings.Contains(errOut.String(), "usage: flowmark serve <corpus> --port <port>") {
				t.Fatalf("stderr %q names no usage", errOut.String())
			}
			if _, err := os.Stat(filepath.Join(root, ".flowmark", "server.json")); err == nil {
				t.Fatal("a refused start wrote the token file")
			}
		})
	}
	t.Run("40#the-server: a catalog with diagnostics refuses the start", func(t *testing.T) {
		bad := serveCorpus(t, "dbspec 1 board\n\ntable tags {\n")
		var out, errOut syncBuffer
		if code := serve([]string{bad, "--port", "0"}, &out, &errOut, make(chan os.Signal)); code != 1 {
			t.Fatalf("exit %d, want 1; stderr %s", code, errOut.String())
		}
		if !strings.Contains(errOut.String(), "dbspec/test.dbs") {
			t.Fatalf("stderr %q names no diagnostic", errOut.String())
		}
	})
}

var listening = regexp.MustCompile(`^\[serve\] listening on 127\.0\.0\.1:(\d+) for the corpus board\n$`)

// TestServe runs the command: it prints the address it bound, serves a session with the token of server.json, prints
// the lock events, keeps the token across starts, renews it with --new-token, and stops on a signal.
func TestServe(t *testing.T) {
	root := serveCorpus(t, serveDBS)
	run := func(t *testing.T, args ...string) (addr string, stopped func() (int, string)) {
		t.Helper()
		pr, pw := io.Pipe()
		var errOut syncBuffer
		stop := make(chan os.Signal, 1)
		done := make(chan int, 1)
		go func() {
			done <- serve(append([]string{root, "--port", "0"}, args...), pw, &errOut, stop)
			pw.Close()
		}()
		line, err := bufio.NewReader(pr).ReadString('\n')
		m := listening.FindStringSubmatch(line)
		if err != nil || m == nil {
			t.Fatalf("stdout %q, %v; stderr %s", line, err, errOut.String())
		}
		go io.Copy(io.Discard, pr)
		return "127.0.0.1:" + m[1], func() (int, string) {
			stop <- syscall.SIGTERM
			select {
			case code := <-done:
				return code, errOut.String()
			case <-time.After(patience):
				t.Fatalf("serve did not stop on SIGTERM")
			}
			return 0, ""
		}
	}
	token := func(t *testing.T) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, ".flowmark", "server.json"))
		if err != nil {
			t.Fatal(err)
		}
		var v struct{ Token string }
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		return v.Token
	}
	ask := func(t *testing.T, addr string, lines ...string) []string {
		t.Helper()
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := conn.SetDeadline(time.Now().Add(patience)); err != nil {
			t.Fatal(err)
		}
		r := bufio.NewReader(conn)
		var out []string
		for _, l := range lines {
			if _, err := io.WriteString(conn, l+"\n"); err != nil {
				t.Fatal(err)
			}
			a, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("answer to %s: %v", l, err)
			}
			out = append(out, strings.TrimSuffix(a, "\n"))
		}
		return out
	}
	connect := func(tok string) string {
		return `{"op":"connect","corpus":"board","token":"` + tok + `"}`
	}

	addr, stopped := run(t)
	first := token(t)
	got := ask(t, addr, connect(first), `{"op":"begin","read_only":true}`, `{"op":"rollback"}`)
	if got[0] != `{"ok":true}` || got[1] != `{"ok":true}` || got[2] != `{"ok":true}` {
		t.Fatalf("answers %q", got)
	}
	code, errOut := stopped()
	if code != 0 {
		t.Fatalf("exit %d; stderr %s", code, errOut)
	}
	if !strings.Contains(errOut, "lock.acquired") || !strings.Contains(errOut, "/serve-1") || !strings.Contains(errOut, "lock.released") {
		t.Fatalf("stderr %q shows no lock events of serve-1", errOut)
	}

	addr, stopped = run(t)
	if token(t) != first {
		t.Fatalf("the token changed across starts: %s, then %s", first, token(t))
	}
	if got := ask(t, addr, connect(first)); got[0] != `{"ok":true}` {
		t.Fatalf("answers %q", got)
	}
	if code, errOut := stopped(); code != 0 {
		t.Fatalf("exit %d; stderr %s", code, errOut)
	}

	addr, stopped = run(t, "--new-token")
	renewed := token(t)
	if renewed == first {
		t.Fatal("--new-token kept the token")
	}
	if got := ask(t, addr, connect(first)); !strings.Contains(got[0], `"code":"CONFIG"`) {
		t.Fatalf("the old token answered %q", got)
	}
	if got := ask(t, addr, connect(renewed)); got[0] != `{"ok":true}` {
		t.Fatalf("answers %q", got)
	}
	if code, errOut := stopped(); code != 0 {
		t.Fatalf("exit %d; stderr %s", code, errOut)
	}
}
