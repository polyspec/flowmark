package main

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestModuleInstallable pins that the module of the command holds no replace directive (T59): `go install
// github.com/polyspec/flowmark/packages/go/cmd/flowmark@<version>` refuses a module whose go.mod holds one, and a module
// that imports this one does not see it, so a fork is required by its own module path.
func TestModuleInstallable(t *testing.T) {
	f, err := os.Open("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	s := bufio.NewScanner(f)
	for n := 1; s.Scan(); n++ {
		if line := strings.TrimSpace(s.Text()); line == "replace" || strings.HasPrefix(line, "replace ") || strings.HasPrefix(line, "replace(") {
			t.Errorf("go.mod:%d holds a replace directive, which go install of the command refuses: %s", n, line)
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
}
