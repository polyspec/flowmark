// Command flowmark is the one binary of the markdown database engine and its faces (spec 40 The faces): the
// operations over any table, canon, install, introspect, history repair, unlock, the server, the MCP server and the
// soksak sidecar. Every command exits 0 when it ran, 1 when it was refused or failed, with the failure on standard
// error, and 2 for a wrong command line.
package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/polyspec/flowmark/packages/go/ops"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run runs the command line args and returns its exit code.
func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		return usage(errOut, "")
	}
	name, rest := args[0], args[1:]
	switch name {
	case "canon":
		return canonTo(out, errOut, rest)
	case "install":
		return install(out, errOut, rest)
	case "introspect":
		return introspectTo(out, errOut, rest)
	case "history":
		return history(out, errOut, rest)
	case "unlock":
		return unlock(out, errOut, rest)
	case "serve":
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stop)
		return serve(rest, out, errOut, stop)
	case "mcp":
		return mcpFace(in, out, errOut, rest)
	case "sidecar":
		return sidecarFace(in, out, errOut, rest)
	}
	if _, ok := faceCommands[name]; ok {
		return face(out, errOut, name, rest)
	}
	return usage(errOut, fmt.Sprintf("the command %q is unknown", name))
}

// usage writes why the command line is wrong and the command lines, and returns 2.
func usage(errOut io.Writer, why string) int {
	if why != "" {
		fmt.Fprintf(errOut, "flowmark: %s\n", why)
	}
	lines := []string{"usage:"}
	for _, n := range ops.Operations() {
		lines = append(lines, "  flowmark "+n+" [--lock-wait-ms <n>] "+faceCommands[n].usage)
	}
	lines = append(lines,
		"  flowmark canon [--lock-wait-ms <n>] <corpus> [file…]",
		"  flowmark install [--lock-wait-ms <n>] <corpus>",
		"  flowmark introspect <corpus>",
		"  flowmark history repair [--lock-wait-ms <n>] <corpus> <table> <key> [--from <state>]",
		"  flowmark unlock <corpus> [--holder <pid>@<host>/<session>]",
		"  flowmark serve <corpus> --port <port> [--new-token] [--lock-wait-ms <n>] [--tx-idle-ms <n>]",
		"  flowmark mcp [--lock-wait-ms <n>] <corpus>",
		"  flowmark sidecar [--lock-wait-ms <n>]",
		"an assignment is <column>=<text> or <column>:=<json>; a value is <text> or :=<json>",
	)
	fmt.Fprintln(errOut, strings.Join(lines, "\n"))
	return 2
}

// fail writes the failure err of the command to standard error as `flowmark <command>: <code>: <message>` and returns 1.
func fail(errOut io.Writer, command string, err error) int {
	code, msg := ops.Failure(err)
	fmt.Fprintf(errOut, "flowmark %s: %s: %s\n", command, code, msg)
	return 1
}

// closeCorpus closes the corpus a command opened, on every way out of the command: a failure to close it is written
// and makes the command's exit code 1.
func closeCorpus(errOut io.Writer, command string, c *ops.Corpus, code *int) {
	if err := c.Close(); err != nil {
		*code = fail(errOut, command, err)
	}
}
