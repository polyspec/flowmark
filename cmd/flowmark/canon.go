package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/polyspec/flowmark/ops"
)

// canonTo runs `flowmark canon [--lock-wait-ms <n>] <corpus> [file…]` (spec 40 The canon operation) with the
// arguments after the command name: each file is a corpus path, relative to the corpus root. It writes each file
// rewritten and each file left as it is to out, and a failure to errOut. It returns 0 when canon ran, 1 when it was
// refused or failed, and 2 for a wrong command line.
func canonTo(out, errOut io.Writer, args []string) int {
	cmd := newCommand("canon", errOut, true)
	pos, ok := cmd.parse(args)
	if !ok {
		return 2
	}
	if len(pos) < 1 {
		return usage(errOut, "canon: the command line lacks the corpus")
	}
	files := make([]string, 0, len(pos)-1)
	for _, f := range pos[1:] {
		files = append(files, filepath.ToSlash(f))
	}
	ctx := context.Background()
	c, err := ops.Open(ctx, pos[0], cmd.lockWait())
	if err != nil {
		return fail(errOut, "canon", err)
	}
	s, err := c.Session("cli")
	if err != nil {
		return fail(errOut, "canon", err)
	}
	res, err := s.Canon(ctx, files)
	if cerr := s.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(errOut, "canon", err)
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
