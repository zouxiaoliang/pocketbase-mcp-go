package testutil_test

import (
	"net/http"
	"testing"

	"github.com/zouxiaoliang/pocketbase-mcp-go/tests/testutil"
)

func TestMockPocketBaseSmoke(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()

	resp, err := http.Get(mock.URL + "/api/collections")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}

	resp2, err := http.Post(mock.URL+"/api/collections/_superusers/auth-with-password", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 auth, got %d", resp2.StatusCode)
	}
}
