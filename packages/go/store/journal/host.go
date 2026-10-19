package journal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Holder is the record a lock file holds (spec 40, the lock): one line of JSON and a line feed. Host is the machine
// identifier (with the PID namespace on Linux), Start the start of the process, and Hostname the host name for the
// messages only. Acquisition is the token of one acquisition — the 32 hex digits of its pending record's name — so two
// acquisitions never write the same record, even of one session in one process. The holder is named
// `<pid>@<host>/<session>`.
type Holder struct {
	PID         int    `json:"pid"`
	Start       string `json:"start"`
	Host        string `json:"host"`
	Hostname    string `json:"hostname"`
	Session     string `json:"session"`
	Acquisition string `json:"acquisition"`
}

func (h Holder) String() string {
	return fmt.Sprintf("%d@%s/%s", h.PID, h.Host, h.Session)
}

func (h Holder) describe() string {
	return fmt.Sprintf("%s (process %d on %s, started %s)", h, h.PID, h.Hostname, h.Start)
}

// record is the lock file's bytes for h.
func (h Holder) record() []byte {
	data, err := json.Marshal(h)
	if err != nil {
		// A struct of an int and strings always marshals.
		panic(err)
	}
	return append(data, '\n')
}

// checkPart accepts a session or a host: not empty, and neither a separator of the holder name nor a line break.
func checkPart(what, s string) error {
	if s == "" || strings.ContainsAny(s, "/@\r\n") {
		return fmt.Errorf("the %s %q is empty or holds '/', '@' or a line break", what, s)
	}
	return nil
}

// parseRecord reads a lock or pending record; anything but the canonical one-line record is refused.
func parseRecord(data []byte) (Holder, error) {
	var h Holder
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&h); err != nil {
		return Holder{}, fmt.Errorf("holds no lock record: %q", data)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Holder{}, fmt.Errorf("holds more than one lock record: %q", data)
	}
	if h.PID <= 0 || h.Start == "" || h.Hostname == "" || checkPart("session", h.Session) != nil || checkPart("host", h.Host) != nil || !isHex(h.Acquisition, 32) {
		return Holder{}, fmt.Errorf("holds an incomplete lock record: %q", data)
	}
	if !bytes.Equal(h.record(), data) {
		return Holder{}, fmt.Errorf("holds a lock record that is not one canonical line: %q", data)
	}
	return h, nil
}

// self returns this process's holder for a new acquisition of session, with a fresh acquisition token.
func (c *Corpus) self(session string) (Holder, error) {
	if err := checkPart("session", session); err != nil {
		return Holder{}, fmt.Errorf("journal: %w", err)
	}
	host, err := hostID()
	if err != nil {
		return Holder{}, fmt.Errorf("journal: the machine identifier, which names a lock holder, cannot be read: %w", err)
	}
	name, err := os.Hostname()
	if err != nil {
		return Holder{}, fmt.Errorf("journal: the host name: %w", err)
	}
	start, err := processStart(os.Getpid())
	if err != nil {
		return Holder{}, fmt.Errorf("journal: the start of this process: %w", err)
	}
	return Holder{PID: os.Getpid(), Start: start, Host: host, Hostname: name, Session: session, Acquisition: randomHex(16)}, nil
}

// HolderState says whether the holder of a lock still runs.
type HolderState int

const (
	// HolderRunning: the holder's process runs on this host, or exists and its start cannot be read.
	HolderRunning HolderState = iota
	// HolderEnded: no process has its ID, the process with its ID started at another time (a reused ID), or that
	// process is a zombie — a stale lock.
	HolderEnded
	// HolderUnknown: the holder is on another host, whose processes cannot be checked from here; never judged ended.
	HolderUnknown
)

// judge decides whether h still runs.
func (c *Corpus) judge(h Holder) (HolderState, error) {
	host, err := hostID()
	if err != nil {
		return 0, fmt.Errorf("journal: the machine identifier: %w", err)
	}
	if h.Host != host {
		return HolderUnknown, nil
	}
	state, err := processState(h.PID, h.Start)
	if err != nil {
		return 0, fmt.Errorf("journal: judging the holder %s: %w", h, err)
	}
	return state, nil
}
