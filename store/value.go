package store

import (
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/polyspec/flowmark/internal/unicodedcp"
	"github.com/polyspec/orm/engine/dbspec"
)

// maxKeyBytes bounds a files key so its file name stays within 255 bytes.
const maxKeyBytes = 243

// Readable returns the readable canonical text of v as a value of type t, or
// false when v is not a value of t: the form a stored scalar takes in a Row
// (see the canonical format, "Readable values").
func Readable(t dbspec.Type, v string) (string, bool) { return readable(t, v) }

// SingleLineFault names the single-line rule that a value written in a single-line
// place breaks, or "" when none (see the canonical format, "Single-line places"):
// the rule a writer checks of a value before it renders a file.
func SingleLineFault(v string) string { return singleLineFault(v) }

// KeyFault names the key shape rule that key breaks, or "" when none (see the
// document model, "Keys"): the rule a writer checks of a key before it renders a file.
func KeyFault(key string) string { return keyFault(key) }

// FoldKey returns key with A-Z lower-cased and every other byte kept: the fold under
// which two keys are equal without regard to case. The key shape holds A-Z and a-z
// alone of the letters, so no other character folds (see the document model, "Keys").
func FoldKey(key string) string {
	b := []byte(key)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// readable returns the readable canonical text of v as a value of type t, or
// false when v is not a value of t. Numbers and booleans take orm's canonical
// literal; a date, time, datetime or uuid takes the literal of its quoted text,
// without the quotes; text is written as it is, and bytes as padded base64.
func readable(t dbspec.Type, v string) (string, bool) {
	switch t.Kind {
	case dbspec.TypeText:
		return v, true
	case dbspec.TypeVarchar:
		if utf8.RuneCountInString(v) > t.Length {
			return "", false
		}
		return v, true
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64, dbspec.TypeDecimal, dbspec.TypeF64, dbspec.TypeBool:
		return dbspec.CanonicalLiteral(t, v)
	case dbspec.TypeDate, dbspec.TypeTime, dbspec.TypeDatetime, dbspec.TypeUUID:
		if strings.ContainsRune(v, '\'') {
			return "", false
		}
		lit, ok := dbspec.CanonicalLiteral(t, "'"+v+"'")
		if !ok || len(lit) < 2 || lit[0] != '\'' || lit[len(lit)-1] != '\'' {
			return "", false
		}
		return lit[1 : len(lit)-1], true
	case dbspec.TypeBytes:
		b, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(v)
			if err != nil {
				return "", false
			}
		}
		return base64.StdEncoding.EncodeToString(b), true
	}
	return "", false
}

// trimWhite removes the spaces and tabs at both ends of s: the white space that
// CommonMark drops around a heading and a paragraph.
func trimWhite(s string) string { return strings.Trim(s, " \t") }

// invisible reports whether r is an invisible character of the single-line places
// under Unicode 17.0.0: a White_Space character other than U+1680, a control (Cc),
// a format character (Cf) other than a Prepended_Concatenation_Mark, or a
// Default_Ignorable_Code_Point. Such a character renders with no glyph. The
// space and the tab are White_Space, so they are invisible too.
func invisible(r rune) bool {
	switch {
	case r == 0x1680:
		return false
	case unicode.Is(unicode.White_Space, r), unicode.Is(unicode.Cc, r):
		return true
	case unicode.Is(unicode.Cf, r):
		return !unicode.Is(unicode.Prepended_Concatenation_Mark, r)
	}
	return unicodedcp.DefaultIgnorable(r)
}

// invisibleAtEnd reports whether v is non-empty and its first or its last
// character is invisible. A character inside v is not an end.
func invisibleAtEnd(v string) bool {
	if v == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(v)
	last, _ := utf8.DecodeLastRuneInString(v)
	return invisible(first) || invisible(last)
}

// singleLineFault names the single-line rule that v breaks, or "" when none: a
// line break, a U+0000, an invisible character at either end, or raw HTML, which
// a viewer would hide or alter.
func singleLineFault(v string) string {
	switch {
	case strings.ContainsAny(v, "\r\n"):
		return "a line break"
	case strings.ContainsRune(v, 0):
		return "a U+0000"
	case invisibleAtEnd(v):
		return "an invisible character at an end"
	case strings.Contains(v, "<!") || strings.Contains(v, "</"):
		return "raw HTML"
	}
	for i := 0; i+1 < len(v); i++ {
		if v[i] == '<' {
			if r, _ := utf8.DecodeRuneInString(v[i+1:]); unicode.IsLetter(r) {
				return "raw HTML"
			}
		}
	}
	return ""
}

// keyFault names the key rule that key breaks, or "" when none. A key holds no
// "-_", since an underscore after punctuation is left-flanking and can open
// emphasis (CommonMark 0.31.2, 6.2), which would alter the shown key and its
// anchor; a trailing dash is accepted, as no viewer strips it from an anchor.
func keyFault(key string) string {
	switch {
	case key == "":
		return "the key is empty"
	case len(key) > maxKeyBytes:
		return "the key is longer than 243 bytes"
	case !isAlnum(key[0]):
		return "the key does not begin with a letter or a digit"
	case strings.Contains(key, "-_"):
		return "the key holds a dash before an underscore"
	case reservedName(key):
		return "the key is a reserved Windows file name"
	}
	for i := 0; i < len(key); i++ {
		if c := key[i]; !isAlnum(c) && c != '_' && c != '-' {
			return "the key holds a character outside A-Z, a-z, 0-9, _ and -"
		}
	}
	return ""
}

func isAlnum(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

// reservedName reports whether name equals CON, PRN, AUX, NUL, COM0-COM9 or
// LPT0-LPT9 without regard to case.
func reservedName(name string) bool {
	u := strings.ToUpper(name)
	switch u {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(u) == 4 && (strings.HasPrefix(u, "COM") || strings.HasPrefix(u, "LPT")) && '0' <= u[3] && u[3] <= '9'
}

// splitLink reads a link [key](target). The key holds no brackets.
func splitLink(raw string) (key, target string, ok bool) {
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, ")") {
		return "", "", false
	}
	i := strings.Index(raw, "](")
	if i < 1 {
		return "", "", false
	}
	key = raw[1:i]
	if key == "" || strings.ContainsAny(key, "[]") {
		return "", "", false
	}
	return key, raw[i+2 : len(raw)-1], true
}

// escapeTitle writes the text of a title for its H1. A final run of hashes that
// follows a space or a tab, or that is the whole title, has its last hash written
// \#, since CommonMark would read the run as a closing sequence.
func escapeTitle(title string) string {
	n := len(title)
	j := n
	for j > 0 && title[j-1] == '#' {
		j--
	}
	if j < n && (j == 0 || title[j-1] == ' ' || title[j-1] == '\t') {
		return title[:n-1] + `\#`
	}
	return title
}

// unescapeTitle reads the title of an H1 from the text after its "# " marker:
// CommonMark's leading and trailing spaces and tabs and its closing sequence are
// removed, and a final \# reads as #. No other escape is decoded.
func unescapeTitle(content string) string {
	s := trimWhite(content)
	j := len(s)
	for j > 0 && s[j-1] == '#' {
		j--
	}
	if j < len(s) && (j == 0 || s[j-1] == ' ' || s[j-1] == '\t') {
		s = strings.TrimRight(s[:j], " \t")
	}
	if strings.HasSuffix(s, `\#`) {
		s = s[:len(s)-2] + "#"
	}
	return s
}
