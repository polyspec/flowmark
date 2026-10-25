// Package unicode13 carries the General_Category and the Alphabetic property of
// Unicode 13.0.0, from which the set of characters that VS Code's slug removes is
// stated: the expression githubSlugReplaceRegex of its slugify.ts matches exactly the
// characters this data names (spec 10, Detail headings and anchors, and store's
// TestVSRemovalIsVSCodeRegex). The Go unicode package holds only the version Go is
// built with, so the table is generated from the Unicode data files by go generate
// (gen/main.go), and a test requires the committed table to be that generation. The
// data files are Unicode's own, distributed under the Unicode License v3, whose text
// stands beside them in testdata/UNICODE-LICENSE.txt.
package unicode13

//go:generate go run ./gen

import (
	"bytes"
	"fmt"
	"github.com/polyspec/flowmark/packages/go/internal/ucd"
	"go/format"
	"io"
	"sort"
)

// UnicodeVersion is the version of the Unicode data that the table is generated from.
const UnicodeVersion = "13.0.0"

// CategorySource is the public URL of the General_Category data file of UnicodeVersion.
const CategorySource = "https://www.unicode.org/Public/" + UnicodeVersion + "/ucd/extracted/DerivedGeneralCategory.txt"

// AlphabeticSource is the public URL of the data file of UnicodeVersion that holds the
// Alphabetic property.
const AlphabeticSource = "https://www.unicode.org/Public/" + UnicodeVersion + "/ucd/DerivedCoreProperties.txt"

// Range is an inclusive range of code points.
type Range struct{ Lo, Hi rune }

// CategoryRange is an inclusive range of code points of one general category, named by
// its two-letter alias, such as Lu or Cn.
type CategoryRange struct {
	Lo, Hi   rune
	Category string
}

// Category returns the general category of r, by its two-letter alias; every code
// point has one, Cn for a code point that is not assigned. The table covers every code
// point (ParseCategories refuses a gap), so only a value outside U+0000..U+10FFFF, which
// is no code point and which no Go string decodes to, finds no range; it is reported as
// Cn, the category of a value that is not assigned.
func Category(r rune) string {
	i := sort.Search(len(categories), func(i int) bool { return categories[i].Hi >= r })
	if i < len(categories) && categories[i].Lo <= r {
		return categories[i].Category
	}
	return "Cn"
}

// Alphabetic reports whether r has the Alphabetic property.
func Alphabetic(r rune) bool {
	i := sort.Search(len(alphabetic), func(i int) bool { return alphabetic[i].Hi >= r })
	return i < len(alphabetic) && alphabetic[i].Lo <= r
}

// categoryNames are the two-letter aliases of the general categories.
var categoryNames = map[string]bool{
	"Lu": true, "Ll": true, "Lt": true, "Lm": true, "Lo": true,
	"Mn": true, "Mc": true, "Me": true,
	"Nd": true, "Nl": true, "No": true,
	"Pc": true, "Pd": true, "Ps": true, "Pe": true, "Pi": true, "Pf": true, "Po": true,
	"Sm": true, "Sc": true, "Sk": true, "So": true,
	"Zs": true, "Zl": true, "Zp": true,
	"Cc": true, "Cf": true, "Cs": true, "Co": true, "Cn": true,
}

// ParseCategories reads a DerivedGeneralCategory file of UnicodeVersion and returns its
// ranges in ascending order, adjacent ranges of one category joined. The file must
// begin with its own version header, and its ranges must cover every code point once:
// a range that is malformed, names an unknown category or overlaps another, and a code
// point that no range covers, are refused.
func ParseCategories(r io.Reader) ([]CategoryRange, error) {
	var ranges []CategoryRange
	err := ucd.ReadRanges(r, "DerivedGeneralCategory", UnicodeVersion, func(line int, lo, hi rune, value string) error {
		if !categoryNames[value] {
			return fmt.Errorf("line %d: %q is no general category", line, value)
		}
		ranges = append(ranges, CategoryRange{Lo: lo, Hi: hi, Category: value})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Lo < ranges[j].Lo })
	var out []CategoryRange
	next := rune(0)
	for _, c := range ranges {
		switch {
		case c.Lo < next:
			return nil, fmt.Errorf("the range %04X..%04X overlaps another", c.Lo, c.Hi)
		case c.Lo > next:
			return nil, fmt.Errorf("no range covers %04X", next)
		}
		if n := len(out); n > 0 && out[n-1].Category == c.Category {
			out[n-1].Hi = c.Hi
		} else {
			out = append(out, c)
		}
		next = c.Hi + 1
	}
	if next != 0x110000 {
		return nil, fmt.Errorf("no range covers %04X", next)
	}
	return out, nil
}

// ParseAlphabetic reads a DerivedCoreProperties file of UnicodeVersion and returns the
// ranges of Alphabetic in ascending order, adjacent ranges joined. The file must begin
// with its own version header; a line of another property is skipped. A range that is
// malformed or overlaps another is refused, and so is a file with no range of the
// property.
func ParseAlphabetic(r io.Reader) ([]Range, error) {
	var ranges []Range
	err := ucd.ReadRanges(r, "DerivedCoreProperties", UnicodeVersion, func(line int, lo, hi rune, value string) error {
		if value == "Alphabetic" {
			ranges = append(ranges, Range{Lo: lo, Hi: hi})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("the file has no range of Alphabetic")
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Lo < ranges[j].Lo })
	out := ranges[:1]
	for _, a := range ranges[1:] {
		last := &out[len(out)-1]
		switch {
		case a.Lo <= last.Hi:
			return nil, fmt.Errorf("the range %04X..%04X overlaps another", a.Lo, a.Hi)
		case a.Lo == last.Hi+1:
			last.Hi = a.Hi
		default:
			out = append(out, a)
		}
	}
	return out, nil
}

// The data files in testdata/ that table.go is rendered from.
const (
	categoryFile   = "testdata/DerivedGeneralCategory-" + UnicodeVersion + ".txt"
	alphabeticFile = "testdata/DerivedCoreProperties-" + UnicodeVersion + ".txt"
)

// Table returns the Go source of table.go rendered from the data files in testdata/, read from the package directory:
// gen writes it, and a test requires the committed table to be it.
func Table() ([]byte, error) {
	categories, err := ucd.ParseFile(categoryFile, ParseCategories)
	if err != nil {
		return nil, err
	}
	alphabetic, err := ucd.ParseFile(alphabeticFile, ParseAlphabetic)
	if err != nil {
		return nil, err
	}
	return Render(categories, alphabetic)
}

// Render returns the Go source of table.go for the category and Alphabetic ranges, in
// gofmt form.
func Render(categories []CategoryRange, alphabetic []Range) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by go generate; DO NOT EDIT.\n")
	b.WriteString("// Sources: " + CategorySource + "\n")
	b.WriteString("// and " + AlphabeticSource + ",\n")
	b.WriteString("// the files testdata/DerivedGeneralCategory-" + UnicodeVersion + ".txt and\n")
	b.WriteString("// testdata/DerivedCoreProperties-" + UnicodeVersion + ".txt.\n\n")
	b.WriteString("package unicode13\n\n")
	b.WriteString("// categories holds the general category of every code point of Unicode " + UnicodeVersion + ",\n")
	b.WriteString("// in ascending ranges.\n")
	b.WriteString("var categories = []CategoryRange{\n")
	for _, c := range categories {
		fmt.Fprintf(&b, "\t{0x%04X, 0x%04X, %q},\n", c.Lo, c.Hi, c.Category)
	}
	b.WriteString("}\n\n")
	b.WriteString("// alphabetic holds the ranges of Alphabetic of Unicode " + UnicodeVersion + " in ascending order.\n")
	b.WriteString("var alphabetic = []Range{\n")
	for _, a := range alphabetic {
		fmt.Fprintf(&b, "\t{0x%04X, 0x%04X},\n", a.Lo, a.Hi)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}
