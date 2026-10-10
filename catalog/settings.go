package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// loadTranslations reads dbspec/flowmark.json. The file is optional: when it
// is absent the translations are empty. A file that is present holds only the
// translations key, an array of language names; a broken file stops at its
// first error.
func (l *loader) loadTranslations(root string) []string {
	file := dbspecDir + "/" + settingsFile
	b, err := os.ReadFile(filepath.Join(root, dbspecDir, settingsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		l.add(fileDiagnostic(file, "cannot read "+file+": "+err.Error()))
		return nil
	}
	out, ds := parseSettings(file, b)
	l.diags = append(l.diags, ds...)
	return out
}

// parseSettings parses the settings object with the JSON token reader, so that
// each diagnostic is located at the token it names.
func parseSettings(file string, b []byte) ([]string, []Diagnostic) {
	dec := json.NewDecoder(bytes.NewReader(b))
	fail := func(off int, msg string) ([]string, []Diagnostic) {
		line, column := offsetPosition(b, off)
		return nil, []Diagnostic{{File: file, Line: line, Column: column, Rule: rule, Message: msg}}
	}
	at := tokenStart(dec, b)
	tok, err := dec.Token()
	if err != nil {
		return fail(at, "invalid JSON: "+err.Error())
	}
	if tok != json.Delim('{') {
		return fail(at, "settings must be a JSON object")
	}
	var out []string
	seen := false
	for dec.More() {
		at = tokenStart(dec, b)
		tok, err = dec.Token()
		if err != nil {
			return fail(at, "invalid JSON: "+err.Error())
		}
		key, _ := tok.(string)
		if key != "translations" {
			return fail(at, fmt.Sprintf("unknown key %q", key))
		}
		if seen {
			return fail(at, `key "translations" repeats`)
		}
		seen = true
		at = tokenStart(dec, b)
		tok, err = dec.Token()
		if err != nil {
			return fail(at, "invalid JSON: "+err.Error())
		}
		if tok != json.Delim('[') {
			return fail(at, "translations must be an array of strings")
		}
		out = []string{}
		for dec.More() {
			at = tokenStart(dec, b)
			tok, err = dec.Token()
			if err != nil {
				return fail(at, "invalid JSON: "+err.Error())
			}
			lang, ok := tok.(string)
			if !ok {
				return fail(at, "translations entries must be strings")
			}
			out = append(out, lang)
		}
		if _, err = dec.Token(); err != nil {
			return fail(at, "invalid JSON: "+err.Error())
		}
	}
	at = tokenStart(dec, b)
	if _, err = dec.Token(); err != nil {
		return fail(at, "invalid JSON: "+err.Error())
	}
	at = tokenStart(dec, b)
	if _, err = dec.Token(); err != io.EOF {
		return fail(at, "trailing data after the settings object")
	}
	return out, nil
}

// tokenStart is the byte offset of the next token: the decoder's offset with
// the JSON white space before it skipped.
func tokenStart(dec *json.Decoder, b []byte) int {
	off := int(dec.InputOffset())
	for off < len(b) && (b[off] == ' ' || b[off] == '\t' || b[off] == '\r' || b[off] == '\n') {
		off++
	}
	return off
}

// offsetPosition is the 1-based line and column, in code points, of a byte offset.
func offsetPosition(b []byte, off int) (int, int) {
	line := 1 + bytes.Count(b[:off], []byte("\n"))
	lineStart := bytes.LastIndexByte(b[:off], '\n') + 1
	return line, utf8.RuneCount(b[lineStart:off]) + 1
}
