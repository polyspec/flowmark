package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store/journal"
	"github.com/polyspec/flowmark/writepath"
)

// noSubscriber is the observer of the lock and journal events of a CLI command: the command ends with its answer, and
// nothing in its process subscribes to the events, so they reach no one.
type noSubscriber struct{}

func (noSubscriber) Observe(journal.Event) {}

// canonTo runs `flowmark canon [--lock-wait-ms <n>] <corpus> [file…]` (spec 40 The canon operation) with the
// arguments after the command name: each file is a corpus path, relative to the corpus root. It writes each file
// rewritten and each file left as it is to out, and a failure to errOut. It returns 0 when canon ran, 1 when it was
// refused or failed, and 2 for a wrong command line.
func canonTo(out, errOut io.Writer, args []string) int {
	fs := flag.NewFlagSet("canon", flag.ContinueOnError)
	fs.SetOutput(errOut)
	wait := fs.Int("lock-wait-ms", 5000, "how long to wait for the corpus lock, in milliseconds; 0 refuses at once")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(errOut, "usage: flowmark canon [--lock-wait-ms <n>] <corpus> [file…]")
		return 2
	}
	root, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(errOut, "flowmark canon: the corpus root %s: %v\n", fs.Arg(0), err)
		return 1
	}
	files := make([]string, 0, fs.NArg()-1)
	for _, f := range fs.Args()[1:] {
		files = append(files, filepath.ToSlash(f))
	}
	cat, diags := catalog.Load(root)
	if cat == nil || len(diags) > 0 {
		for _, d := range diags {
			fmt.Fprintf(errOut, "flowmark canon: %s:%d:%d %s: %s\n", d.File, d.Line, d.Column, d.Rule, d.Message)
		}
		fmt.Fprintln(errOut, "flowmark canon: the catalog does not load; repair dbspec/ first")
		return 1
	}
	j, err := journal.New(root, noSubscriber{})
	if err != nil {
		fmt.Fprintf(errOut, "flowmark canon: %v\n", err)
		return 1
	}
	ctx := context.Background()
	c, err := writepath.Open(ctx, cat, j, time.Duration(*wait)*time.Millisecond)
	if err != nil {
		fmt.Fprintf(errOut, "flowmark canon: %v\n", err)
		return 1
	}
	s, err := c.Session("cli")
	if err != nil {
		fmt.Fprintf(errOut, "flowmark canon: %v\n", err)
		return 1
	}
	res, err := s.Canon(ctx, files)
	if cerr := s.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		fmt.Fprintf(errOut, "flowmark canon: %v\n", err)
		return 1
	}
	for _, p := range res.Rewritten {
		fmt.Fprintf(out, "[canon] rewrote %s\n", p)
	}
	for _, r := range res.Reported {
		d := r.Diagnostic
		why := fmt.Sprintf("%s:%d:%d %s: %s", d.File, d.Line, d.Column, d.Rule, d.Message)
		if r.Needs != "" {
			why = fmt.Sprintf("a link needs the unavailable table %s (%s)", r.Needs, why)
		}
		fmt.Fprintf(out, "[canon] left %s: %s\n", r.Path, why)
	}
	fmt.Fprintf(out, "[canon] %d rewritten, %d left\n", len(res.Rewritten), len(res.Reported))
	return 0
}
