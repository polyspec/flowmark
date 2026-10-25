package ops

import (
	"bufio"
	"bytes"
	"io"
)

// MaxLine is the longest message line a line face reads: the sidecar protocol's message limit, 64 MiB before the line
// feed, which holds the answer of any operation the faces share.
const MaxLine = 64 << 20

// ServeLines answers the message lines of in on out until in ends, the protocol of the MCP and sidecar faces: each
// line that is not blank goes to answer, and an answer that is not nil is written as JSON on its own line and flushed,
// since the client reads each one while the process runs. It returns an error when in, out or answer fails, or a line
// passes MaxLine.
func ServeLines(in io.Reader, out io.Writer, answer func(line []byte) (any, error)) error {
	w := bufio.NewWriter(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), MaxLine+1)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		a, err := answer(line)
		if err != nil {
			return err
		}
		if a == nil {
			continue
		}
		b, err := JSON(a)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	return sc.Err()
}
