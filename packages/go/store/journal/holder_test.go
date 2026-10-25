package journal

import (
	"bufio"
	"os"
	"strings"
	"testing"
	"time"
)

func judge(t *testing.T, c *Corpus, h Holder) HolderState {
	t.Helper()
	state, err := c.judge(h)
	if err != nil {
		t.Fatalf("judge(%v): %v", h, err)
	}
	return state
}

// A running process of this host whose start matches is running.
func TestOwnProcessReadsRunning(t *testing.T) {
	c, _ := newCorpus(t, t.TempDir())
	if got := judge(t, c, selfRecord(t, c, "s")); got != HolderRunning {
		t.Fatalf("own process = %v, want running", got)
	}
}

// A process ID whose process has another start was reused: its holder has ended.
func TestReusedPIDReadsEnded(t *testing.T) {
	c, _ := newCorpus(t, t.TempDir())
	h := selfRecord(t, c, "s")
	h.Start = "0"
	if got := judge(t, c, h); got != HolderEnded {
		t.Fatalf("reused PID = %v, want ended", got)
	}
	h = selfRecord(t, c, "s")
	h.PID = endedPID(t)
	if got := judge(t, c, h); got != HolderEnded {
		t.Fatalf("absent PID = %v, want ended", got)
	}
}

// A process that has exited but is not reaped (a zombie) has ended. The exit is learned from the holder-exit watch.
func TestZombieHolderReadsEnded(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	if err := os.Mkdir(root+string(os.PathSeparator)+".flowmark", 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := childCommand(t, "record", root, "zombie")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	h, err := parseRecord([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if got := judge(t, c, h); got != HolderRunning {
		t.Fatalf("live child = %v, want running", got)
	}
	w, err := c.newWatch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.close(); err != nil {
			t.Error(err)
		}
	})
	if err := w.armHolder(h); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case wk := <-w.wakes():
		if wk.kind != wakeHolderExit {
			t.Fatalf("wake = %+v, want the holder's exit", wk)
		}
	case <-time.After(patience):
		t.Fatal("no exit event")
	}
	// The child is a zombie until it is reaped below.
	if got := judge(t, c, h); got != HolderEnded {
		t.Fatalf("zombie = %v, want ended", got)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

// otherUserPID is the process of another user that the host has: process 1, launchd of root on macOS.
func otherUserPID() int {
	return 1
}

// A process of another user exists (kill answers EPERM); with its start matching it is running.
func TestOtherUserProcessReadsRunning(t *testing.T) {
	c, _ := newCorpus(t, t.TempDir())
	h := selfRecord(t, c, "s")
	pid := otherUserPID()
	h.PID = pid
	start, err := processStart(pid)
	if err != nil {
		t.Fatalf("the start of process %d: %v", pid, err)
	}
	h.Start = start
	if got := judge(t, c, h); got != HolderRunning {
		t.Fatalf("process %d = %v, want running", pid, got)
	}
}

// The host is the machine identifier (with the PID namespace on Linux); another one is unknown, never ended.
func TestMachineIDDistinguishesHosts(t *testing.T) {
	a, err := hostID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := hostID()
	if err != nil || a != b || a == "" || strings.ContainsAny(a, "/\n") {
		t.Fatalf("hostID = %q then %q, %v", a, b, err)
	}
	c, _ := newCorpus(t, t.TempDir())
	h := selfRecord(t, c, "s")
	if h.Host != a {
		t.Fatalf("record host = %q, want %q", h.Host, a)
	}
	h.Host = a + "-other"
	h.PID = endedPID(t)
	if got := judge(t, c, h); got != HolderUnknown {
		t.Fatalf("another host = %v, want unknown", got)
	}
}

// Every hash is the SHA-256 of the raw bytes as 64 lowercase hex digits.
func TestHashVector(t *testing.T) {
	for in, want := range map[string]string{
		"":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	} {
		if got := sum([]byte(in)); got != want {
			t.Errorf("sum(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestIsHash(t *testing.T) {
	good := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	for h, want := range map[string]bool{
		"":                                 true,
		good:                               true,
		strings.ToUpper(good):              false,
		good[:63]:                          false,
		good + "0":                         false,
		strings.Replace(good, "b", "g", 1): false,
	} {
		if got := isHash(h); got != want {
			t.Errorf("isHash(%q) = %v, want %v", h, got, want)
		}
	}
}
