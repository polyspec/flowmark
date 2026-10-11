package writepath

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store/journal"
)

// The test binary runs as a child process when FLOWMARK_WRITEPATH_CHILD names a mode; the child reports on stdout.
func TestMain(m *testing.M) {
	if mode := os.Getenv("FLOWMARK_WRITEPATH_CHILD"); mode != "" {
		os.Exit(child(mode))
	}
	os.Exit(m.Run())
}

// announcer prints the lock.waiting event of a child, so its parent learns that it waits.
type announcer struct{}

func (announcer) Observe(e journal.Event) {
	if e.Kind == journal.LockWaiting {
		fmt.Println("waiting")
	}
}

// child runs the messages of FLOWMARK_WRITEPATH_MSGS on the corpus FLOWMARK_WRITEPATH_ROOT, waiting for the lock up to
// FLOWMARK_WRITEPATH_WAIT_MS, and prints "ok" and each answer, or "error <code> <face> <message>":
//   - write: each message is one request outside a transaction;
//   - tx: the messages are one transaction;
//   - crash: the messages are one transaction whose commit exits with 3 at FLOWMARK_WRITEPATH_STEP and _INDEX.
func child(mode string) int {
	root := os.Getenv("FLOWMARK_WRITEPATH_ROOT")
	var msgs []json.RawMessage
	if err := json.Unmarshal([]byte(os.Getenv("FLOWMARK_WRITEPATH_MSGS")), &msgs); err != nil {
		fmt.Println("error", err)
		return 1
	}
	wait, _ := strconv.Atoi(os.Getenv("FLOWMARK_WRITEPATH_WAIT_MS"))
	cat, diags := catalog.Load(root)
	if len(diags) > 0 {
		fmt.Println("error", diags)
		return 1
	}
	j, err := journal.New(root, announcer{})
	if err != nil {
		fmt.Println("error", err)
		return 1
	}
	c, err := Open(context.Background(), cat, j, time.Duration(wait)*time.Millisecond)
	if err != nil {
		report(err)
		return 0
	}
	s, err := c.Session("child")
	if err != nil {
		fmt.Println("error", err)
		return 1
	}
	if mode == "crash" {
		index, _ := strconv.Atoi(os.Getenv("FLOWMARK_WRITEPATH_INDEX"))
		step := os.Getenv("FLOWMARK_WRITEPATH_STEP")
		j.SetFault(func(st string, i int) error {
			if st == step && i == index {
				os.Exit(3)
			}
			return nil
		})
	}
	ctx := context.Background()
	if mode != "write" {
		if err := s.Begin(ctx, false); err != nil {
			report(err)
			return 0
		}
	}
	var answers []string
	for _, m := range msgs {
		a, err := s.Exec(ctx, m)
		if err != nil {
			report(err)
			return 0
		}
		b, _ := json.Marshal(a)
		answers = append(answers, string(b))
	}
	if mode != "write" {
		if err := s.Commit(); err != nil {
			report(err)
			return 0
		}
	}
	fmt.Println("ok " + strings.Join(answers, " "))
	return 0
}

// report prints a failure of the write path.
func report(err error) {
	var we *Error
	if errors.As(err, &we) {
		fmt.Printf("error %s %s %s\n", we.Code, we.Face, strings.ReplaceAll(we.Msg, "\n", " "))
		return
	}
	fmt.Printf("error %s - %s\n", codeOf(err), strings.ReplaceAll(err.Error(), "\n", " "))
}

// childCommand returns the command of a child of mode on root running msgs.
func childCommand(t *testing.T, mode, root string, wait time.Duration, msgs [][]byte, env ...string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]json.RawMessage, len(msgs))
	for i, m := range msgs {
		raw[i] = m
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "FLOWMARK_WRITEPATH_CHILD="+mode, "FLOWMARK_WRITEPATH_ROOT="+root,
		"FLOWMARK_WRITEPATH_MSGS="+string(b), "FLOWMARK_WRITEPATH_WAIT_MS="+strconv.Itoa(int(wait/time.Millisecond)))
	cmd.Env = append(cmd.Env, env...)
	cmd.Stderr = os.Stderr
	return cmd
}

// TestConcurrentProcesses pins one write path across processes (spec 40 The lock): several processes write at once,
// each waiting for the lock, and every write lands once, serialized, with its own key.
func TestConcurrentProcesses(t *testing.T) {
	f := baseFixture(t)
	const n = 6
	cmds := make([]*exec.Cmd, n)
	outs := make([]io.Reader, n)
	for i := range cmds {
		cmds[i] = childCommand(t, "write", f.root, patience, [][]byte{f.insertTag(fmt.Sprintf("t%d", i), "area")})
		out, err := cmds[i].StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		outs[i] = out
	}
	for _, c := range cmds {
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
	}
	var keys []string
	for i, c := range cmds {
		b, err := io.ReadAll(outs[i])
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Wait(); err != nil {
			t.Fatalf("child %d: %v (%s)", i, err, b)
		}
		var last string
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			last = line
		}
		a, ok := strings.CutPrefix(last, "ok ")
		if !ok {
			t.Fatalf("child %d said %q", i, b)
		}
		var ans struct{ Keys []int }
		if err := json.Unmarshal([]byte(a), &ans); err != nil || len(ans.Keys) != 1 {
			t.Fatalf("child %d answered %q", i, a)
		}
		keys = append(keys, strconv.Itoa(ans.Keys[0]))
	}
	sort.Strings(keys)
	if want := []string{"3", "4", "5", "6", "7", "8"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	want(t, f.session("cli"), f.countTags(), `{"count":8}`)
	f.clean()
}

// TestTransactionHoldsProcesses pins the lock of a transaction across processes (spec 40 The lock): a write of another
// process attempted mid-transaction fails LOCK_TIMEOUT naming the transaction's session when it does not wait, and,
// waiting, lands after the commit.
func TestTransactionHoldsProcesses(t *testing.T) {
	f := baseFixture(t)
	s := f.session("server")
	must(t, s.Begin(context.Background(), false))
	want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
	out, err := childCommand(t, "write", f.root, 0, [][]byte{f.insertTag("db", "area")}).Output()
	if err != nil {
		t.Fatal(err)
	}
	if line := string(out); !strings.HasPrefix(line, "error CANCELED LOCK_TIMEOUT ") || !strings.Contains(line, s.Name()) {
		t.Fatalf("child = %q, want LOCK_TIMEOUT naming %s", line, s.Name())
	}
	cmd := childCommand(t, "write", f.root, patience, [][]byte{f.insertTag("db", "area")})
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(stdout)
	line, err := r.ReadString('\n')
	if err != nil || line != "waiting\n" {
		t.Fatalf("child = %q, %v; want it waiting", line, err)
	}
	must(t, s.Commit())
	line, err = r.ReadString('\n')
	if err != nil || line != "ok {\"affected\":1,\"keys\":[4]}\n" {
		t.Fatalf("child = %q, %v; want the write after the commit", line, err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	f.clean()
}

// TestCrashedProcessRecovers pins a process killed at each step of a commit (spec 40 Opening a corpus): its lock is
// stale until `flowmark unlock`, and the next open recovers the old state or the new one, never a mix.
func TestCrashedProcessRecovers(t *testing.T) {
	after := afterCrashTx(t)
	for _, cp := range crashPoints {
		t.Run(fmt.Sprintf("40#opening-a-corpus: %s#%d", cp.step, cp.index), func(t *testing.T) {
			f := baseFixture(t)
			before := snapshot(t, f.root)
			cmd := childCommand(t, "crash", f.root, 0, crashTx(f), "FLOWMARK_WRITEPATH_STEP="+cp.step, "FLOWMARK_WRITEPATH_INDEX="+strconv.Itoa(cp.index))
			out, err := cmd.Output()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 3 {
				t.Fatalf("child = %v %q, want the exit 3 of the crash", err, out)
			}
			_, err = f.session("cli").Exec(context.Background(), f.countTags())
			fails(t, err, "CANCELED", "LOCK_TIMEOUT", "flowmark unlock")
			if _, err := f.j.Unlock(""); err != nil {
				t.Fatal(err)
			}
			f.rec.reset()
			g := openFixtureEvents(t, f.root)
			kinds := g.rec.kinds()
			if kinds[len(kinds)-2] != cp.event {
				t.Fatalf("events = %v, want %s before the release", kinds, cp.event)
			}
			wantSnap := before
			if cp.new {
				wantSnap = after
			}
			if got := snapshot(t, f.root); !reflect.DeepEqual(got, wantSnap) {
				t.Fatalf("corpus = %v, want %v", got, wantSnap)
			}
			g.clean()
		})
	}
}

// openFixtureEvents opens the corpus at root and keeps the events of the opening.
func openFixtureEvents(t *testing.T, root string) *fixture {
	t.Helper()
	cat, diags := catalog.Load(root)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	rec := &recorder{}
	j, err := journal.New(root, rec)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Open(context.Background(), cat, j, 0)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, root: root, cat: cat, j: j, c: c, rec: rec}
}
