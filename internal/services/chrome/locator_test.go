package chrome

import (
	"testing"
)

func TestFindChromeExecutable(t *testing.T) {
	path, err := FindChromeExecutable()
	if err != nil {
		t.Fatalf("FindChromeExecutable failed: %v", err)
	}
	t.Logf("Found Chrome executable at: %s", path)
	if !fileExists(path) {
		t.Errorf("Path reported by FindChromeExecutable does not exist: %s", path)
	}
}
