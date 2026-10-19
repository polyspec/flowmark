package main

import (
	"io"

	"github.com/polyspec/flowmark/packages/go/mcp"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/sidecar"
)

// mcpFace runs `flowmark mcp [--lock-wait-ms <n>] <corpus>` (spec 40 The faces) until standard input ends.
func mcpFace(in io.Reader, out, errOut io.Writer, args []string) int {
	cmd := newCommand("mcp", errOut, true)
	pos, ok := cmd.parse(args)
	if !ok {
		return 2
	}
	if len(pos) != 1 {
		return usage(errOut, "mcp: the command line takes the corpus alone")
	}
	if err := mcp.Serve(pos[0], cmd.lockWait(), in, out); err != nil {
		return fail(errOut, "mcp", &ops.Error{Code: "DRIVER", Msg: err.Error()})
	}
	return 0
}

// sidecarFace runs `flowmark sidecar [--lock-wait-ms <n>]` (spec 40 The faces) until standard input ends.
func sidecarFace(in io.Reader, out, errOut io.Writer, args []string) int {
	cmd := newCommand("sidecar", errOut, true)
	pos, ok := cmd.parse(args)
	if !ok {
		return 2
	}
	if len(pos) != 0 {
		return usage(errOut, "sidecar: the command line takes no argument; each request names its corpus root")
	}
	if err := sidecar.Serve(cmd.lockWait(), in, out, errOut); err != nil {
		return fail(errOut, "sidecar", &ops.Error{Code: "DRIVER", Msg: err.Error()})
	}
	return 0
}
