package transport_test

import (
	"testing"

	"github.com/zouxiaoliang/pocketbase-mcp-go/tests/testutil"
)

func TestGinHealthEndpoint(t *testing.T) {
	testutil.RunRootTest(t, "TestGinHealthEndpoint")
}
