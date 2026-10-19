package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/polyspec/flowmark/packages/go/ops"
)

// install runs `flowmark install [--lock-wait-ms <n>] <corpus>` (spec 30 Install, spec 40 The faces): it prints each
// file or folder it created, the .gitattributes line when it added it, and a count.
func install(out, errOut io.Writer, args []string) (code int) {
	cmd := newCommand("install", errOut, true)
	pos, ok := cmd.parse(args)
	if !ok {
		return 2
	}
	if len(pos) != 1 {
		return usage(errOut, "install: the command line takes the corpus alone")
	}
	ctx := context.Background()
	c, err := ops.Open(ctx, pos[0], cmd.lockWait())
	if err != nil {
		return fail(errOut, "install", err)
	}
	defer closeCorpus(errOut, "install", c, &code)
	s, err := c.Session("cli")
	if err != nil {
		return fail(errOut, "install", err)
	}
	res, err := s.Install(ctx)
	if cerr := s.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(errOut, "install", err)
	}
	for _, p := range res.Created {
		fmt.Fprintf(out, "[install] created %s\n", p)
	}
	if res.Attributes {
		fmt.Fprintln(out, "[install] added the line *.flowmark.md text eol=lf to .gitattributes")
	}
	fmt.Fprintf(out, "[install] %d created\n", len(res.Created))
	return 0
}

// history runs `flowmark history repair [--lock-wait-ms <n>] <corpus> <table> <key> [--from <state>]` (spec 50 A state
// changed outside the engine): it prints the answer of the repair as one line of JSON.
func history(out, errOut io.Writer, args []string) (code int) {
	if len(args) == 0 || args[0] != "repair" {
		return usage(errOut, "history: the only history command is history repair")
	}
	cmd := newCommand("history repair", errOut, true)
	from := cmd.fs.String("from", "", "the initial state a row without history rows is repaired from, when the machine has several")
	pos, ok := cmd.parse(args[1:])
	if !ok {
		return 2
	}
	if len(pos) != 3 {
		return usage(errOut, "history repair: the command line takes the corpus, the table and the key")
	}
	ctx := context.Background()
	c, err := ops.Open(ctx, pos[0], cmd.lockWait())
	if err != nil {
		return fail(errOut, "history repair", err)
	}
	defer closeCorpus(errOut, "history repair", c, &code)
	s, err := c.Session("cli")
	if err != nil {
		return fail(errOut, "history repair", err)
	}
	a, err := s.Repair(ctx, pos[1], pos[2], *from)
	if cerr := s.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(errOut, "history repair", err)
	}
	b, err := ops.JSON(a)
	if err != nil {
		return fail(errOut, "history repair", err)
	}
	fmt.Fprintf(out, "%s\n", b)
	return 0
}

// unlock runs `flowmark unlock <corpus> [--holder <pid>@<host>/<session>]` (spec 40 A stale lock): it removes the lock
// of a holder that has ended and prints the holder. A refused unlock is CONFIG; a failure of the file system under it
// is DRIVER.
func unlock(out, errOut io.Writer, args []string) int {
	cmd := newCommand("unlock", errOut, false)
	holder := cmd.fs.String("holder", "", "the holder of another host whose lock is removed, as <pid>@<host>/<session>")
	pos, ok := cmd.parse(args)
	if !ok {
		return 2
	}
	if len(pos) != 1 {
		return usage(errOut, "unlock: the command line takes the corpus alone")
	}
	j, err := ops.Journal(pos[0])
	if err != nil {
		return fail(errOut, "unlock", err)
	}
	h, err := j.Unlock(*holder)
	if err != nil {
		code := "CONFIG"
		var pe *fs.PathError
		if errors.As(err, &pe) {
			code = "DRIVER"
		}
		return fail(errOut, "unlock", &ops.Error{Code: code, Msg: err.Error()})
	}
	fmt.Fprintf(out, "[unlock] removed the lock of %s\n", h)
	return 0
}
