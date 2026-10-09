package codec

// Document is the skeleton of one markdown document (spec 20-document-model).
// The preamble and the section lines keep the preserved payload bytes as they are.
type Document struct {
	DocID       string
	TwinSHA     string
	Owner       string // the provenance comment (`owner: …`), present on adapter-owned documents only
	Title       string
	Preamble    []string
	Frontmatter []string // the lines inside the fence; T2.2 parses them as the YAML 1.2 core subset
	Sections    []Section
}

// Section is one H2 or H3 heading with the payload lines under it.
type Section struct {
	Level   int // 2 or 3
	Heading string
	Lines   []string
}
