package journal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// The test binary runs as a child process when FLOWMARK_JOURNAL_CHILD names a mode; the child reports on stdout.
func TestMain(m *testing.M) {
	if mode := os.Getenv("FLOWMARK_JOURNAL_CHILD"); mode != "" {
		os.Exit(child(mode))
	}
	os.Exit(m.Run())
}

type nobody struct{}

func (nobody) Observe(Event) {}

// child runs one mode:
//   - exit: exits at once;
//   - record: prints its own holder record, then exits when stdin closes;
//   - hold: takes the lock without waiting, prints "acquired <record>" or "held <state> <holder>", and holds until
//     stdin closes;
//   - abandon: takes the lock and exits without releasing it;
//   - crash: takes the lock and commits the fixture's changes, exiting with 3 at FLOWMARK_JOURNAL_STEP.
func child(mode string) int {
	if mode == "exit" {
		return 0
	}
	c, err := New(os.Getenv("FLOWMARK_JOURNAL_ROOT"), nobody{})
	if err != nil {
		fmt.Println("error", err)
		return 1
	}
	if mode == "record" {
		h, err := c.self(os.Getenv("FLOWMARK_JOURNAL_SESSION"))
		if err != nil {
			fmt.Println("error", err)
			return 1
		}
		fmt.Print(string(h.record()))
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			fmt.Println("error", err)
			return 1
		}
		return 0
	}
	l, err := c.Acquire(context.Background(), os.Getenv("FLOWMARK_JOURNAL_SESSION"), 0)
	var timeout *LockTimeout
	switch {
	case errors.As(err, &timeout):
		fmt.Printf("held %d %s\n", timeout.State, timeout.Holder)
		return 0
	case err != nil:
		fmt.Println("error", err)
		return 1
	}
	fmt.Printf("acquired %s", l.Holder().record())
	switch mode {
	case "hold":
		// Hold until the parent closes stdin: a blocking read, woken by the close.
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			fmt.Println("error", err)
			return 1
		}
		if err := l.Release(); err != nil {
			fmt.Println("error", err)
			return 1
		}
		return 0
	case "abandon":
		return 0
	case "crash":
		index, err := strconv.Atoi(os.Getenv("FLOWMARK_JOURNAL_INDEX"))
		if err != nil {
			fmt.Println("error", err)
			return 1
		}
		step := os.Getenv("FLOWMARK_JOURNAL_STEP")
		c.fault = func(s string, i int) error {
			if s == step && i == index {
				os.Exit(3)
			}
			return nil
		}
		if err := c.Commit(l, fixtureChanges()); err != nil {
			fmt.Println("error", err)
			return 1
		}
		return 0
	}
	fmt.Println("error unknown mode", mode)
	return 1
}

// fixtureChanges are the changes of fixture, for a child that does not build the corpus.
func fixtureChanges() []Change {
	return []Change{
		{Path: "a.md", Data: []byte("new a\n")},
		{Path: "sub/b.md", Data: []byte("same\n")},
		{Path: "d.md", Data: []byte("same\n")},
		{Path: "c.md", Delete: true},
	}
}

func childCommand(t *testing.T, mode, root, session string, env ...string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "FLOWMARK_JOURNAL_CHILD="+mode, "FLOWMARK_JOURNAL_ROOT="+root, "FLOWMARK_JOURNAL_SESSION="+session)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stderr = os.Stderr
	return cmd
}

// endedPID returns the process ID of a child that has exited and been reaped.
func endedPID(t *testing.T) int {
	t.Helper()
	cmd := childCommand(t, "exit", "", "")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

// Several processes contend for one lock at once without waiting: exactly one acquires it, every other one is
// refused naming that holder as running.
func TestLockContentionAcrossProcesses(t *testing.T) {
	root := t.TempDir()
	const n = 6
	type proc struct {
		cmd   *exec.Cmd
		stdin io.WriteCloser
		out   *bufio.Reader
	}
	procs := make([]proc, n)
	for i := range procs {
		cmd := childCommand(t, "hold", root, fmt.Sprintf("child-%d", i))
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		procs[i] = proc{cmd, stdin, bufio.NewReader(stdout)}
	}
	for _, p := range procs {
		if err := p.cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	var winners, lines []string
	for _, p := range procs {
		line, err := p.out.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, strings.TrimSuffix(line, "\n"))
		if record, ok := strings.CutPrefix(line, "acquired "); ok {
			h, err := parseRecord([]byte(record))
			if err != nil {
				t.Fatal(err)
			}
			winners = append(winners, h.String())
		}
	}
	for _, p := range procs {
		if err := p.stdin.Close(); err != nil {
			t.Fatal(err)
		}
		if err := p.cmd.Wait(); err != nil {
			t.Fatalf("child: %v (lines %v)", err, lines)
		}
	}
	if len(winners) != 1 {
		t.Fatalf("winners = %v, want exactly one (lines %v)", winners, lines)
	}
	refused := 0
	for _, line := range lines {
		if line == fmt.Sprintf("held %d %s", HolderRunning, winners[0]) {
			refused++
		}
	}
	if refused != n-1 {
		t.Fatalf("lines %v: want %d refusals naming %s", lines, n-1, winners[0])
	}
	if _, err := os.Lstat(lockFile(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock after every release: %v", err)
	}
}

// A lock held by this process refuses a child, naming this process.
func TestLockHeldByParentRefusesChild(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	l := acquire(t, c, "parent")
	out, err := childCommand(t, "hold", root, "child").Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("held %d %s\n", HolderRunning, l.Holder()); string(out) != want {
		t.Fatalf("child = %q, want %q", out, want)
	}
}

// A process that ends while holding the lock leaves a stale lock: it is reported as ended and never broken.
func TestStaleLockOfEndedProcess(t *testing.T) {
	root := t.TempDir()
	cmd := childCommand(t, "abandon", root, "child")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	c, _ := newCorpus(t, root)
	_, err = c.Acquire(context.Background(), "parent", 0)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) || timeout.State != HolderEnded || timeout.Holder.PID != cmd.Process.Pid {
		t.Fatalf("Acquire = %v, want the ended child %d (child said %q)", err, cmd.Process.Pid, out)
	}
}

// A process killed at each step of a commit: after `flowmark unlock` clears its stale lock, recovery gives the old
// state or the new one, never a mix.
func TestCrashedProcessRecovers(t *testing.T) {
	for _, p := range crashPoints {
		t.Run(fmt.Sprintf("%s#%d", p.step, p.index), func(t *testing.T) {
			root, _, before, after := fixture(t)
			corpustest.WantExit(t, childCommand(t, "crash", root, "child", "FLOWMARK_JOURNAL_STEP="+p.step, "FLOWMARK_JOURNAL_INDEX="+strconv.Itoa(p.index)), 3)
			c, rec := newCorpus(t, root)
			_, err := c.Acquire(context.Background(), "parent", 0)
			var timeout *LockTimeout
			if !errors.As(err, &timeout) || timeout.State != HolderEnded {
				t.Fatalf("Acquire = %v, want the stale lock of the crashed child", err)
			}
			if _, err := c.Unlock(""); err != nil {
				t.Fatal(err)
			}
			l := acquire(t, c, "parent")
			want := before
			if p.new {
				want = after
			}
			wantRecovered(t, c, l, root, want)
			if kinds := rec.kinds(); kinds[len(kinds)-1] != p.event {
				t.Fatalf("events = %v, want the last %s", kinds, p.event)
			}
		})
	}
}
