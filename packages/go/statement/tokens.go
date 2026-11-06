package statement

import "strings"

// tokenKind is the kind of a token of a statement.
type tokenKind int

const (
	// tokenWord is a keyword or a bare identifier.
	tokenWord tokenKind = iota
	// tokenQuoted is an identifier in double quotes, backquotes or brackets.
	tokenQuoted
	// tokenString is a string literal in single quotes.
	tokenString
	// tokenOther is a number, a blob literal, a parameter or a symbol.
	tokenOther
)

// token is one token of a statement: a word as written, the value of a quoted identifier or of a string literal, or the
// text of any other token.
type token struct {
	kind tokenKind
	text string
}

// keyword is the word of t in upper case, and "" for a token that is no word.
func (t token) keyword() string {
	if t.kind != tokenWord {
		return ""
	}
	return strings.ToUpper(t.text)
}

// is reports whether t is the symbol s.
func (t token) is(s string) bool { return t.kind == tokenOther && t.text == s }

// tokens reads text as SQLite's tokenizer does: whitespace and comments only separate tokens, a "--" comment running to
// the end of its line and a "/*" comment to its "*/" or the end of the text. A quoted token runs to its closing quote,
// which it holds doubled, or to the end of the text. The tokens that classify a statement are its words; a statement's
// own text still goes to SQLite as it was written.
func tokens(text string) []token {
	var out []token
	for i := 0; i < len(text); {
		c := text[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r':
			i++
		case c == '-' && i+1 < len(text) && text[i+1] == '-':
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				return out
			}
			i += end + 1
		case c == '/' && i+1 < len(text) && text[i+1] == '*':
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return out
			}
			i += 2 + end + 2
		case c == '\'':
			value, n := quotedToken(text[i:], '\'')
			out = append(out, token{tokenString, value})
			i += n
		case c == '"' || c == '`':
			value, n := quotedToken(text[i:], c)
			out = append(out, token{tokenQuoted, value})
			i += n
		case c == '[':
			end := strings.IndexByte(text[i:], ']')
			if end < 0 {
				return append(out, token{tokenQuoted, text[i+1:]})
			}
			out = append(out, token{tokenQuoted, text[i+1 : i+end]})
			i += end + 1
		case (c == 'x' || c == 'X') && i+1 < len(text) && text[i+1] == '\'':
			_, n := quotedToken(text[i+1:], '\'')
			out = append(out, token{tokenOther, text[i : i+1+n]})
			i += 1 + n
		case wordStart(c):
			j := i + 1
			for j < len(text) && wordPart(text[j]) {
				j++
			}
			out = append(out, token{tokenWord, text[i:j]})
			i = j
		case digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]):
			j := i + 1
			for j < len(text) && (wordPart(text[j]) || text[j] == '.' ||
				(text[j] == '+' || text[j] == '-') && (text[j-1] == 'e' || text[j-1] == 'E')) {
				j++
			}
			out = append(out, token{tokenOther, text[i:j]})
			i = j
		case c == '?' || c == ':' || c == '@' || c == '$':
			j := i + 1
			for j < len(text) && wordPart(text[j]) {
				j++
			}
			out = append(out, token{tokenOther, text[i:j]})
			i = j
		default:
			out = append(out, token{tokenOther, text[i : i+1]})
			i++
		}
	}
	return out
}

// quotedToken reads the token that the quote q opens at the start of s: its value, each doubled quote one quote, and the
// bytes it takes, to the end of s when it is not closed.
func quotedToken(s string, q byte) (string, int) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] != q {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == q {
			b.WriteByte(q)
			i++
			continue
		}
		return b.String(), i + 1
	}
	return b.String(), len(s)
}

// wordStart reports whether c begins a word: a letter, an underscore or a byte of a character outside ASCII.
func wordStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

// wordPart reports whether c continues a word: what begins one, a digit or a dollar sign.
func wordPart(c byte) bool { return wordStart(c) || digit(c) || c == '$' }

func digit(c byte) bool { return c >= '0' && c <= '9' }

// statementTokens is the tokens of text without the semicolons that end it.
func statementTokens(text string) []token {
	toks := tokens(text)
	for len(toks) > 0 && toks[len(toks)-1].is(";") {
		toks = toks[:len(toks)-1]
	}
	return toks
}

// firstKeyword is the first word of text in upper case, and "" when text begins with no word.
func firstKeyword(toks []token) string {
	if len(toks) == 0 {
		return ""
	}
	return toks[0].keyword()
}
