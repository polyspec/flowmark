package codec

import "strings"

// Render writes the document in its canonical form. It owns the skeleton — the header
// comments, the headings, the fences and the blank-line placement — and leaves the payload
// lines byte for byte (spec 10-canonical-format).
func Render(document *Document) string {
	var header strings.Builder
	header.WriteString("<!-- doc-id: " + document.DocID + " -->")
	if document.TwinSHA != "" {
		header.WriteString("\n<!-- source-sha256: " + document.TwinSHA + " -->")
	}
	if document.Owner != "" {
		header.WriteString("\n<!-- owner: " + document.Owner + " -->")
	}
	blocks := []string{header.String()}
	if document.Title != "" {
		blocks = append(blocks, "# "+document.Title)
	}
	if len(document.Preamble) > 0 {
		blocks = append(blocks, strings.Join(document.Preamble, "\n"))
	}
	if len(document.Frontmatter) > 0 {
		blocks = append(blocks, "---\n"+strings.Join(document.Frontmatter, "\n")+"\n---")
	}
	for _, section := range document.Sections {
		block := "## " + section.Heading
		if len(section.Lines) > 0 {
			block += "\n\n" + strings.Join(section.Lines, "\n")
		}
		blocks = append(blocks, block)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
