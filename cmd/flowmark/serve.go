package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/server"
	"github.com/polyspec/flowmark/store/journal"
	"github.com/polyspec/flowmark/writepath"
)

const serveUsage = "usage: flowmark serve <corpus> --port <port> [--new-token] [--lock-wait-ms <n>] [--tx-idle-ms <n>]"

// serve runs `flowmark serve` with args, the words after serve (spec 40 The server): it serves the corpus on
// 127.0.0.1:<port> until stop delivers, printing the address it bound on stdout and each lock and journal event and
// each failure on stderr. It returns 0 after a stop, 1 when the server cannot start or fails, and 2 on a usage error.
func serve(args []string, stdout, stderr io.Writer, stop <-chan os.Signal) int {
	usage := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "flowmark serve: "+format+"\n%s\n", append(a, serveUsage)...)
		return 2
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return usage("name the corpus root first")
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	port := fs.Int("port", -1, "")
	newToken := fs.Bool("new-token", false, "")
	waitMS := fs.Int("lock-wait-ms", 5000, "")
	idleMS := fs.Int("tx-idle-ms", 5000, "")
	if err := fs.Parse(args[1:]); err != nil {
		return usage("%v", err)
	}
	switch {
	case fs.NArg() > 0:
		return usage("one corpus root is served; %q follows the flags", fs.Arg(0))
	case *port == -1:
		return usage("--port is required; give 0 to let the operating system choose")
	case *port < 0 || *port > 65535:
		return usage("--port %d is not a TCP port; give 1 to 65535, or 0 to let the operating system choose", *port)
	case *waitMS < 0:
		return usage("--lock-wait-ms %d is negative; give 0 to refuse at once or a positive wait", *waitMS)
	case *idleMS <= 0:
		return usage("--tx-idle-ms %d is not positive", *idleMS)
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "flowmark serve: "+format+"\n", a...)
		return 1
	}
	root, err := filepath.Abs(args[0])
	if err != nil {
		return fail("the corpus root: %v", err)
	}
	cat, diags := catalog.Load(root)
	if len(diags) > 0 {
		for _, d := range diags {
			fmt.Fprintf(stderr, "flowmark serve: %s: line %d: %s: %s\n", d.File, d.Line, d.Rule, d.Message)
		}
		return fail("the catalog has %d diagnostics; fix them to serve the corpus", len(diags))
	}
	log := &lineLog{w: stderr}
	j, err := journal.New(root, log)
	if err != nil {
		return fail("%v", err)
	}
	c, err := writepath.Open(context.Background(), cat, j, time.Duration(*waitMS)*time.Millisecond)
	if err != nil {
		return fail("opening the corpus: %v", err)
	}
	token, err := server.Token(root, *newToken)
	if err != nil {
		return fail("%v", err)
	}
	name := filepath.Base(root)
	srv, err := server.New(c, name, token, time.Duration(*idleMS)*time.Millisecond, log.printf)
	if err != nil {
		return fail("%v", err)
	}
	ln, err := server.Listen(*port)
	if err != nil {
		return fail("%v", err)
	}
	fmt.Fprintf(stdout, "[serve] listening on %s for the corpus %s\n", ln.Addr(), name)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case <-stop:
		if err := srv.Close(); err != nil {
			return fail("%v", err)
		}
		if err := <-served; err != nil {
			return fail("%v", err)
		}
		return 0
	case err := <-served:
		return fail("%v", err)
	}
}

// lineLog writes the server's lines to one writer: each lock and journal event, and each failure the server cannot
// answer on a connection. Its writes come from every session's goroutine.
type lineLog struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lineLog) printf(format string, a ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, format+"\n", a...)
}

// Observe prints a lock or journal event.
func (l *lineLog) Observe(e journal.Event) {
	l.printf("[serve] %s %s %s", e.Kind, e.Holder, e.Path)
}
