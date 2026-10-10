package store

import (
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/polyspec/orm/engine/dbspec"
)

// maxKeyBytes bounds a files key so its file name stays within 255 bytes.
const maxKeyBytes = 243

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

// singleLineFault names the single-line rule that v breaks, or "" when none: a
// line break, leading or trailing white space, or raw HTML, which a viewer
// would hide or alter.
func singleLineFault(v string) string {
	switch {
	case strings.ContainsAny(v, "\r\n"):
		return "a line break"
	case strings.TrimSpace(v) != v:
		return "leading or trailing white space"
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

// keyFault names the key rule that key breaks, or "" when none.
func keyFault(key string) string {
	switch {
	case key == "":
		return "the key is empty"
	case len(key) > maxKeyBytes:
		return "the key is longer than 243 bytes"
	case !isAlnum(key[0]):
		return "the key does not begin with a letter or a digit"
	case strings.Contains(key, "--"):
		return "the key holds a double dash"
	case key[len(key)-1] == '-':
		return "the key ends with a dash"
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

// escapeTitle writes a title as its H1 text: when the title ends in a space and
// one or more hashes, the last hash is written \# so that CommonMark does not
// read it as a closing sequence.
func escapeTitle(title string) string {
	n := len(title)
	j := n
	for j > 0 && title[j-1] == '#' {
		j--
	}
	if j < n && j > 0 && title[j-1] == ' ' {
		return title[:n-1] + `\#`
	}
	return title
}

// unescapeTitle reads the title of an H1 text: a text that ends in \# after a
// space and hashes reads its \# as a hash.
func unescapeTitle(text string) string {
	if !strings.HasSuffix(text, `\#`) {
		return text
	}
	pre := text[:len(text)-2]
	j := len(pre)
	for j > 0 && pre[j-1] == '#' {
		j--
	}
	if j > 0 && pre[j-1] == ' ' {
		return pre + "#"
	}
	return text
}
