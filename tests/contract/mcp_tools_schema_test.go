package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zouxiaoliang/pocketbase-mcp-go/tests/testutil"
)

func TestP1ToolNamesMatchContract(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	contractPath := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../specs/001-pocketbase-mcp-go-port/contracts/mcp-tools.json"))
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}

	testutil.RunRootTest(t, "TestMCPServerRegistersAllTools")

	if len(contract.Tools) != 24 {
		t.Fatalf("expected 24 contract tools, got %d", len(contract.Tools))
	}
}
