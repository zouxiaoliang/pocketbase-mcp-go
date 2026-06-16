package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// RunRootTest runs a test function in the repository root package main by name.
// Used by tests/tools to delegate to handler tests in main_test.go without duplicating logic.
func RunRootTest(t *testing.T, testName string) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve testutil path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
	cmd := exec.Command("go", "test", "-run", "^"+testName+"$", ".")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIN_MODE=test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("root test %s failed: %v\n%s", testName, err, out)
	}
}
