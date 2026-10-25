// Package tabletest holds the checks that the tests of each generated table share.
package tabletest

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// License requires the Unicode License v3, under which the data files are distributed, to stand beside them in full in
// the file path: its copyright and permission notice must appear with every copy.
func License(t *testing.T, path string) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.HasPrefix(text, "UNICODE LICENSE V3\n") {
		t.Fatalf("%s does not begin with the license title", path)
	}
	for _, want := range []string{"COPYRIGHT AND PERMISSION NOTICE", "Permission is hereby granted, free of charge,"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s lacks %q", path, want)
		}
	}
}

// Fresh requires table.go of the test's package to be want, the rendering of its data files, so a table edited by
// hand, or left behind by a changed data file, fails; source names the data files in the message.
func Fresh(t *testing.T, want []byte, source string) {
	t.Helper()
	got, err := os.ReadFile("table.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("table.go is not the rendering of %s: run go generate in the package directory", source)
	}
}
