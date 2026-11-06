package statement

// InsertMessage is insertMessage for the tests of the package, which form an IR request of a session as a face does.
var InsertMessage = insertMessage

// SetFullDiff installs fullDiff, the seam that forms the difference of each write statement from every row of the
// database as well; the function it returns removes it.
func SetFullDiff(f func(text string, captured, full [][]byte)) func() {
	fullDiff = f
	return func() { fullDiff = nil }
}
