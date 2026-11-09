package main

import (
	"context"
	"errors"
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
	pr := &printer{w: out}
	for _, p := range res.Created {
		pr.printf("[install] created %s\n", p)
	}
	if res.Attributes {
		pr.println("[install] added the line *.flowmark.md text eol=lf to .gitattributes")
	}
	pr.printf("[install] %d created\n", len(res.Created))
	return pr.status(errOut, "install")
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
	pr := &printer{w: out}
	pr.printf("[unlock] removed the lock of %s\n", h)
	return pr.status(errOut, "unlock")
}
