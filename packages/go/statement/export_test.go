package statement

import "github.com/polyspec/flowmark/packages/go/writepath"

// InsertMessage is insertMessage for the tests of the package, which form an IR request of a session as a face does.
var InsertMessage = insertMessage

// SetFullDiff installs the seam of s that forms the difference of each write statement from every row of the database
// as well; the function it returns removes it.
func SetFullDiff(s *writepath.Session, f func(text string, captured, full [][]byte)) func() {
	fullDiffs.Store(s, fullDiff(f))
	return func() { fullDiffs.Delete(s) }
}
