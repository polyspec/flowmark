package statement

import (
	"reflect"
	"testing"
)

// tokenCases are texts and the tokens SQLite's tokenizer reads from them, a case of each token kind and of each
// comment form, with the edges of an unclosed comment or quote.
var tokenCases = []struct {
	name, text string
	want       []token
}{
	{"an empty text", "", nil},
	{"whitespace alone", " \t\n\f\r", nil},
	{"a line comment alone", "-- note", nil},
	{"a line comment and a word", "-- note\nSELECT", []token{{tokenWord, "SELECT"}}},
	{"a block comment and a word", "/* note */SELECT", []token{{tokenWord, "SELECT"}}},
	{"a block comment of lines", "/* a\nb */ SELECT", []token{{tokenWord, "SELECT"}}},
	{"an unclosed block comment", "SELECT /* note", []token{{tokenWord, "SELECT"}}},
	{"a minus that opens no comment", "1 - 2", []token{{tokenOther, "1"}, {tokenOther, "-"}, {tokenOther, "2"}}},
	{"a slash that opens no comment", "4/2", []token{{tokenOther, "4"}, {tokenOther, "/"}, {tokenOther, "2"}}},
	{"a comment inside a string", "'-- no'", []token{{tokenString, "-- no"}}},
	{"a string with a doubled quote", "'it''s'", []token{{tokenString, "it's"}}},
	{"an unclosed string", "'open", []token{{tokenString, "open"}}},
	{"a double-quoted identifier", `"a ""b"""`, []token{{tokenQuoted, `a "b"`}}},
	{"a backquoted identifier", "`a``b`", []token{{tokenQuoted, "a`b"}}},
	{"a bracketed identifier", "[a b]", []token{{tokenQuoted, "a b"}}},
	{"an unclosed bracket", "[a", []token{{tokenQuoted, "a"}}},
	{"a blob literal", "x'00ff' X''", []token{{tokenOther, "x'00ff'"}, {tokenOther, "X''"}}},
	{"a word named x", "x = 1", []token{{tokenWord, "x"}, {tokenOther, "="}, {tokenOther, "1"}}},
	{"a word of digits, underscores and dollars", "_a1$b", []token{{tokenWord, "_a1$b"}}},
	{"a word outside ASCII", "é1", []token{{tokenWord, "é1"}}},
	{"numbers", "1.5e+3 .5 0x1F", []token{{tokenOther, "1.5e+3"}, {tokenOther, ".5"}, {tokenOther, "0x1F"}}},
	{"parameters", "? ?1 :a @b $c", []token{{tokenOther, "?"}, {tokenOther, "?1"}, {tokenOther, ":a"}, {tokenOther, "@b"}, {tokenOther, "$c"}}},
	{"a call with no space", "SELECT(1);", []token{{tokenWord, "SELECT"}, {tokenOther, "("}, {tokenOther, "1"}, {tokenOther, ")"}, {tokenOther, ";"}}},
}

func TestTokens(t *testing.T) {
	for _, tc := range tokenCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokens(tc.text); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tokens(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestStatementKinds pins the classification of a statement by its words: readsOnly and schemaChange accept the
// statements of their first words and reject every other, a comment before the words included.
func TestStatementKinds(t *testing.T) {
	for _, tc := range []struct {
		text          string
		reads, schema bool
	}{
		{"SELECT 1", true, false},
		{"-- c\nSELECT 1", true, false},
		{"/* c */ select 1", true, false},
		{"SELECT(1)", true, false},
		{"VALUES (1)", true, false},
		{"EXPLAIN DELETE FROM t", true, false},
		{"PRAGMA table_info(t)", true, false},
		{"WITH n AS (SELECT 1) SELECT * FROM n", true, false},
		{"WITH n AS (SELECT 1) INSERT INTO t SELECT * FROM n", false, false},
		{"WITH n(v) AS (VALUES (1)) /* c */ DELETE FROM t", false, false},
		{"WITH \"select\" AS (SELECT 1) UPDATE t SET v = 1", false, false},
		{"-- only a comment", true, false},
		{"", true, false},
		{"INSERT INTO t VALUES (1)", false, false},
		{"-- SELECT\nDELETE FROM t", false, false},
		{"'SELECT'", false, false},
		{"REPLACE INTO t VALUES (1)", false, false},
		{"CREATE TABLE t (v)", false, true},
		{"/* c */ create index i on t (v)", false, true},
		{"ALTER TABLE t ADD v", false, true},
		{"-- c\nDROP TABLE t", false, true},
		{"\"CREATE\" TABLE t", false, false},
	} {
		if got := readsOnly(tc.text); got != tc.reads {
			t.Errorf("readsOnly(%q) = %v, want %v", tc.text, got, tc.reads)
		}
		if got := schemaChange(tc.text); got != tc.schema {
			t.Errorf("schemaChange(%q) = %v, want %v", tc.text, got, tc.schema)
		}
	}
}

func TestPragmaSetting(t *testing.T) {
	for _, tc := range []struct {
		text, name, value string
		ok                bool
	}{
		{"PRAGMA busy_timeout = 5", "busy_timeout", "5", true},
		{"-- c\nPRAGMA Locking_Mode=EXCLUSIVE;", "locking_mode", "EXCLUSIVE", true},
		{"PRAGMA cache_size = -2000", "cache_size", "-2000", true},
		{`PRAGMA journal_mode = "wal"`, "journal_mode", "wal", true},
		{"PRAGMA journal_mode = 'wal'", "journal_mode", "wal", true},
		{"PRAGMA table_info(t)", "", "", false},
		{"PRAGMA busy_timeout", "", "", false},
		{"PRAGMA busy_timeout = 5 6", "", "", false},
		{"PRAGMA busy_timeout = (", "", "", false},
		{"PRAGMA main.cache_size = 1", "", "", false},
	} {
		name, value, ok := pragmaSetting(statementTokens(tc.text))
		if name != tc.name || value != tc.value || ok != tc.ok {
			t.Errorf("pragmaSetting(%q) = %q, %q, %v, want %q, %q, %v", tc.text, name, value, ok, tc.name, tc.value, tc.ok)
		}
	}
}

func TestConditional(t *testing.T) {
	for _, tc := range []struct {
		text, name string
		not, ok    bool
	}{
		{"CREATE TABLE IF NOT EXISTS t (v)", "t", true, true},
		{"-- c\ncreate temp table if not exists \"a b\" (v)", "a b", true, true},
		{"CREATE UNIQUE INDEX IF NOT EXISTS [i] ON t (v)", "i", true, true},
		{"CREATE VIEW IF NOT EXISTS main.v AS SELECT 1", "v", true, true},
		{"DROP TRIGGER IF EXISTS g", "g", false, true},
		{"/* c */ DROP TABLE IF EXISTS `t`;", "t", false, true},
		{"CREATE TABLE t (v)", "", false, false},
		{"DROP TABLE t", "", false, false},
		{"CREATE TABLE IF NOT EXISTS other.t (v)", "", false, false},
		{"CREATE TABLE IF NOT EXISTS 't' (v)", "", false, false},
		{"CREATE TABLE IF NOT EXISTS", "", false, false},
		{"ALTER TABLE IF EXISTS t RENAME TO u", "", false, false},
		{"INSERT INTO t VALUES (1)", "", false, false},
	} {
		name, not, ok := conditional(statementTokens(tc.text))
		if name != tc.name || not != tc.not || ok != tc.ok {
			t.Errorf("conditional(%q) = %q, %v, %v, want %q, %v, %v", tc.text, name, not, ok, tc.name, tc.not, tc.ok)
		}
	}
}

// FuzzTokens holds that tokens ends on any text, that a token holds no byte its text does not, and that a comment or
// whitespace before a text changes neither its tokens nor its classification.
func FuzzTokens(f *testing.F) {
	for _, tc := range tokenCases {
		f.Add(tc.text)
	}
	f.Add("WITH n AS (SELECT 1) INSERT INTO t SELECT * FROM n")
	f.Fuzz(func(t *testing.T, text string) {
		got := tokens(text)
		for _, tok := range got {
			if len(tok.text) > len(text) {
				t.Fatalf("tokens(%q) holds %q, longer than the text", text, tok.text)
			}
		}
		for _, lead := range []string{" ", "-- c\n", "/* c */"} {
			if again := tokens(lead + text); !reflect.DeepEqual(again, got) {
				t.Fatalf("tokens(%q) = %v, but tokens(%q) = %v", text, got, lead+text, again)
			}
			if readsOnly(lead+text) != readsOnly(text) || schemaChange(lead+text) != schemaChange(text) {
				t.Fatalf("%q classifies as %q does not", lead+text, text)
			}
		}
	})
}
