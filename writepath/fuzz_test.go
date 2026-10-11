package writepath

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// FuzzSession runs a sequence of write requests and control messages, read from the fuzzer's bytes as pairs of an
// operation and its argument, through one session. Every failure carries a code the rules name; a commit writes
// exactly the post-image the transaction read before it; and at the end the corpus validates clean, with no stage and
// no lock left.
func FuzzSession(f *testing.F) {
	for _, seed := range [][]byte{
		{0, 1, 9, 0},
		{2, 1, 0, 2, 5, 0, 0, 3, 6, 0, 3, 0},
		{2, 1, 0, 2, 4, 0},
		{2, 0, 0, 2, 3, 0},
		{0, 2, 0, 2, 1, 1, 8, 3},
		{2, 1, 5, 0, 5, 1, 7, 0, 6, 1, 0, 4, 3, 0},
		{3, 0, 6, 0, 2, 1, 2, 1, 4, 0},
	} {
		f.Add(seed)
	}
	allowed := map[string]bool{"": true, "CONFIG": true, "CONSTRAINT": true, "DUPLICATE_KEY": true, "READ_ONLY": true, "FOREIGN_KEY": true}
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 32 {
			ops = ops[:32]
		}
		fx := baseFixture(t)
		s := fx.session("fuzz")
		other := fx.session("check")
		ctx := context.Background()
		count := func(s *Session) string {
			a, err := s.Exec(ctx, fx.countTags())
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			return answerJSON(t, a)
		}
		for i := 0; i+1 < len(ops); i += 2 {
			op, arg := ops[i]%10, int(ops[i+1])
			var err error
			switch op {
			case 0:
				_, err = s.Exec(ctx, fx.insertTag(fmt.Sprintf("l%d", arg%8), []string{"topic", "area", "other"}[arg%3]))
			case 1:
				_, err = s.Exec(ctx, fx.msg("delete", "tags", map[string]any{"where": whereID(0)}, arg%6+1))
			case 2:
				err = s.Begin(ctx, arg%4 == 0)
			case 3:
				inside, rerr := s.Exec(ctx, fx.countTags())
				err = s.Commit()
				if err == nil && rerr == nil {
					if got, want := count(other), answerJSON(t, inside); got != want {
						t.Fatalf("after the commit the corpus counts %s; the transaction read %s", got, want)
					}
				}
			case 4:
				err = s.Rollback()
			case 5:
				err = s.Savepoint(fmt.Sprintf("s%d", arg%3))
			case 6:
				err = s.RollbackTo(fmt.Sprintf("s%d", arg%3))
			case 7:
				err = s.Release(fmt.Sprintf("s%d", arg%3))
			case 8:
				_, err = s.Exec(ctx, fx.msg("update", "tags", map[string]any{"set": sets(0, "kind"), "where": whereID(1)}, []string{"topic", "area", "other"}[arg%3], arg%6+1))
			case 9:
				_, err = s.Exec(ctx, fx.countTags())
			}
			if code := codeOf(err); !allowed[code] {
				t.Fatalf("op %d(%d) failed with %v", op, arg, err)
			}
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		fx.clean()
		if _, err := os.Lstat(filepath.Join(fx.root, ".flowmark", "lock")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("the lock is left: %v", err)
		}
	})
}
