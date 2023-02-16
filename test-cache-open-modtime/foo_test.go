package foo_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFoo(t *testing.T) {
	got, err := os.ReadFile(filepath.Join("testdata", "foo.txt"))
	if err != nil {
		t.Fatal(err)
	}
	exp := "foo bar\n"
	if string(got) != exp {
		t.Fatalf("got %q, expected %q", string(got), exp)
	}
}
