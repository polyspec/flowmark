package statement

import (
	"encoding/json"

	"github.com/polyspec/flowmark/packages/go/writepath"
)

// InsertMessage is insertMessage for the tests of the package, which form an IR request of a session as a face does.
var InsertMessage = insertMessage

// SetFullDiff installs the seam of s that forms the difference of each write statement from every row of the database
// as well; the function it returns removes it.
func SetFullDiff(s *writepath.Session, f func(text string, captured, full [][]byte)) func() {
	fullDiffs.Store(s, fullDiff(f))
	return func() { fullDiffs.Delete(s) }
}

// AnswerJSON is the JSON that an answer writes for the value v a row read from SQLite holds.
func AnswerJSON(v any) ([]byte, error) { return json.Marshal(answerValue(v)) }
