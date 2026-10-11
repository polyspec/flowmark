package main

import (
	"flag"
	"io"
	"time"
)

// command is the flag set of one command with the lock wait every command that takes the lock has.
type command struct {
	fs   *flag.FlagSet
	wait *int
}

// newCommand returns the flags of the command name; withWait adds --lock-wait-ms, 5000 by default (spec 40 Acquiring
// and waiting).
func newCommand(name string, errOut io.Writer, withWait bool) *command {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	c := &command{fs: fs}
	if withWait {
		c.wait = fs.Int("lock-wait-ms", 5000, "how long to wait for the corpus lock, in milliseconds; 0 refuses at once")
	}
	return c
}

// lockWait returns the lock wait the command line gave.
func (c *command) lockWait() time.Duration { return time.Duration(*c.wait) * time.Millisecond }

// parse parses args, whose flags may stand before or after the positional arguments; "--" ends the flags. It returns
// the positional arguments, or false for a wrong command line, which the flag set has reported.
func (c *command) parse(args []string) ([]string, bool) {
	var pos []string
	for {
		if err := c.fs.Parse(args); err != nil {
			return nil, false
		}
		rest := c.fs.Args()
		if len(rest) == 0 {
			return pos, true
		}
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			return append(pos, rest...), true
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}
