// Package ucd reads the data files of the Unicode Character Database from which the Unicode tables of the internal
// packages are generated.
package ucd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ReadRanges reads the lines of the Unicode data file name of version, which must begin with its version header
// "# <name>-<version>.txt", and calls each with the 1-based line number, the range of its first field and its second
// field. A comment or a blank line is skipped; a line without a second field, a field that names no code point, a
// reversed range and an empty file are refused.
func ReadRanges(r io.Reader, name, version string, each func(line int, lo, hi rune, value string) error) error {
	sc := bufio.NewScanner(r)
	header := "# " + name + "-" + version + ".txt"
	headerSeen := false
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if !headerSeen {
			if text != header {
				return fmt.Errorf("line 1 is %q; the file must begin with %q", text, header)
			}
			headerSeen = true
		}
		body, _, _ := strings.Cut(text, "#")
		if strings.TrimSpace(body) == "" {
			continue
		}
		span, value, ok := strings.Cut(body, ";")
		if !ok {
			return fmt.Errorf("line %d: no ';' in %q", line, text)
		}
		loText, hiText, isRange := strings.Cut(strings.TrimSpace(span), "..")
		if !isRange {
			hiText = loText
		}
		lo, err := parseCodePoint(loText)
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		hi, err := parseCodePoint(hiText)
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if lo > hi {
			return fmt.Errorf("line %d: the range %s is reversed", line, strings.TrimSpace(span))
		}
		if err := each(line, lo, hi, strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if !headerSeen {
		return fmt.Errorf("the file is empty")
	}
	return nil
}

// parseCodePoint reads a code point written as hexadecimal digits.
func parseCodePoint(s string) (rune, error) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 16, 32)
	if err != nil || v > 0x10FFFF {
		return 0, fmt.Errorf("%q is not a code point", s)
	}
	return rune(v), nil
}

// ParseFile opens the data file path and returns what parse reads from it; an error of parse names path.
func ParseFile[T any](path string, parse func(io.Reader) (T, error)) (_ T, err error) {
	var zero T
	f, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	v, err := parse(f)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

// Generate writes the table file path from render, the main of the gen command of a table package: go generate runs it
// in the package directory. A failure is written to standard error and exits with status 1.
func Generate(path string, render func() ([]byte, error)) {
	src, err := render()
	if err == nil {
		err = os.WriteFile(path, src, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
