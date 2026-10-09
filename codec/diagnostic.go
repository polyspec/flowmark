// Package codec: the strict parser and the canonical renderer of a document (spec 10-canonical-format).
package codec

// Diagnostic reports a parse or render rule violation; the shape follows spec 50-constraints.
type Diagnostic struct {
	File    string
	Line    int
	Column  int
	Rule    string
	Message string
}

// The rule names are fixed by the Diagnostics section of spec 10-canonical-format.
const (
	RuleEncoding     = "format.encoding"     // a byte order mark or a wrong encoding
	RuleNonCanonical = "format.noncanonical" // non-canonical input such as CRLF (P2 holds, P1 does not)
	RuleHeader       = "format.header"       // a missing or duplicate doc-id, a wrong header order
	RuleFrontmatter  = "format.frontmatter"  // a malformed frontmatter fence
	RuleSection      = "format.section"      // a malformed heading or a second H1
	RuleBlock        = "format.block"        // a malformed checklist item or table block
)
