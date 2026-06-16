package tools_test

import (
	"testing"

	"github.com/zouxiaoliang/pocketbase-mcp-go/tests/testutil"
)

func TestAuthenticateUser(t *testing.T) {
	testutil.RunRootTest(t, "TestAuthenticateUser")
}

func TestCreateUser(t *testing.T) {
	testutil.RunRootTest(t, "TestCreateUser")
}

func TestBackupDatabase(t *testing.T) {
	testutil.RunRootTest(t, "TestBackupDatabase")
}
