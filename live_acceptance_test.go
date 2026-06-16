//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/zouxiaoliang/pocketbase-mcp-go/tests/testutil"
)

const liveTestCollection = "mcp_acceptance_posts"

func liveConfig() (config, bool) {
	pbURL := strings.TrimRight(os.Getenv("POCKETBASE_URL"), "/")
	if pbURL == "" {
		return config{}, false
	}
	email := os.Getenv("POCKETBASE_ADMIN_EMAIL")
	password := os.Getenv("POCKETBASE_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return config{}, false
	}
	return config{
		PocketBaseURL: pbURL,
		AdminEmail:    email,
		AdminPassword: password,
		Transport:     "http",
		HTTPAddr:      ":8080",
	}, true
}

func waitForPocketBase(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("PocketBase not reachable at %s", url)
}

func TestLiveAcceptanceP1(t *testing.T) {
	cfg, ok := liveConfig()
	if !ok {
		t.Skip("set POCKETBASE_URL, POCKETBASE_ADMIN_EMAIL, POCKETBASE_ADMIN_PASSWORD for live acceptance")
	}
	waitForPocketBase(t, cfg.PocketBaseURL)

	a := newApp(cfg)
	cleanupLiveCollection(t, a)

	// list_collections
	result := callTool(t, a, "list_collections", map[string]any{})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "users") {
		t.Fatalf("expected users collection, got %s", testutil.ResultText(t, result))
	}

	// get_collection
	result = callTool(t, a, "get_collection", map[string]any{"collectionIdOrName": "users"})
	testutil.AssertNoToolError(t, result)

	// create_collection (open rules for record CRUD without user token)
	result = callTool(t, a, "create_collection", map[string]any{
		"name":       liveTestCollection,
		"listRule":   "",
		"createRule": "",
		"updateRule": "",
		"deleteRule": "",
		"fields": []any{
			map[string]any{"name": "title", "type": "text", "required": true},
		},
	})
	testutil.AssertNoToolError(t, result)
	createText := testutil.ResultText(t, result)
	if !strings.Contains(createText, liveTestCollection) {
		t.Fatalf("expected created collection, got %s", createText)
	}

	// create_record
	result = callTool(t, a, "create_record", map[string]any{
		"collection": liveTestCollection,
		"data":       map[string]any{"title": "acceptance draft"},
	})
	testutil.AssertNoToolError(t, result)
	recordID := extractJSONField(t, testutil.ResultText(t, result), "id")
	if recordID == "" {
		t.Fatal("expected record id from create_record")
	}

	// list_records
	result = callTool(t, a, "list_records", map[string]any{
		"collection": liveTestCollection,
		"perPage":    float64(10),
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), recordID) {
		t.Fatalf("expected record %s in list_records", recordID)
	}

	// update_record
	result = callTool(t, a, "update_record", map[string]any{
		"collection": liveTestCollection,
		"id":         recordID,
		"data":       map[string]any{"title": "acceptance updated"},
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "acceptance updated") {
		t.Fatal("expected updated title")
	}

	// import_data
	result = callTool(t, a, "import_data", map[string]any{
		"collection": liveTestCollection,
		"mode":       "create",
		"data": []any{
			map[string]any{"title": "imported-1"},
			map[string]any{"title": "imported-2"},
		},
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), `"imported": 2`) {
		t.Fatalf("expected 2 imported records, got %s", testutil.ResultText(t, result))
	}

	// update_collection
	result = callTool(t, a, "update_collection", map[string]any{
		"collectionIdOrName": liveTestCollection,
		"listRule":           "",
	})
	testutil.AssertNoToolError(t, result)

	// backup_database
	result = callTool(t, a, "backup_database", map[string]any{"name": "mcp-live-acceptance"})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "mcp-live-acceptance") {
		t.Fatalf("expected backup name in response, got %s", testutil.ResultText(t, result))
	}

	// create_user + authenticate_user
	userEmail := fmt.Sprintf("mcp-user-%d@example.com", time.Now().Unix())
	userPass := "AcceptancePass123!"
	result = callTool(t, a, "create_user", map[string]any{
		"email":           userEmail,
		"password":        userPass,
		"passwordConfirm": userPass,
		"name":            "MCP Acceptance",
	})
	testutil.AssertNoToolError(t, result)
	userID := extractJSONField(t, testutil.ResultText(t, result), "id")

	result = callTool(t, a, "authenticate_user", map[string]any{
		"email":    userEmail,
		"password": userPass,
	})
	testutil.AssertNoToolError(t, result)
	userToken := extractNestedToken(t, testutil.ResultText(t, result))
	if userToken == "" {
		t.Fatal("expected user auth token")
	}

	// auth_refresh
	result = callTool(t, a, "auth_refresh", map[string]any{"token": userToken})
	testutil.AssertNoToolError(t, result)

	// impersonate_user
	result = callTool(t, a, "impersonate_user", map[string]any{"id": userID})
	testutil.AssertNoToolError(t, result)

	// admin authenticate
	result = callTool(t, a, "authenticate_user", map[string]any{"isAdmin": true})
	testutil.AssertNoToolError(t, result)

	// delete_record
	result = callTool(t, a, "delete_record", map[string]any{
		"collection": liveTestCollection,
		"id":         recordID,
	})
	testutil.AssertNoToolError(t, result)

	cleanupLiveCollection(t, a)
}

func TestLiveAcceptanceP2AuthHelpers(t *testing.T) {
	cfg, ok := liveConfig()
	if !ok {
		t.Skip("set POCKETBASE_URL, POCKETBASE_ADMIN_EMAIL, POCKETBASE_ADMIN_PASSWORD for live acceptance")
	}
	waitForPocketBase(t, cfg.PocketBaseURL)
	a := newApp(cfg)

	result := callTool(t, a, "list_auth_methods", map[string]any{})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "password") {
		t.Fatalf("expected password auth method, got %s", testutil.ResultText(t, result))
	}

	result = callTool(t, a, "request_verification", map[string]any{"email": "nobody@example.com"})
	testutil.AssertNoToolError(t, result)

	result = callTool(t, a, "request_password_reset", map[string]any{"email": "nobody@example.com"})
	testutil.AssertNoToolError(t, result)

	// OAuth2/OTP/confirm-* need external setup or valid tokens; verify error paths only.
	result = callTool(t, a, "authenticate_with_oauth2", map[string]any{
		"provider": "google", "code": "invalid", "codeVerifier": "x", "redirectUrl": "http://localhost/cb",
	})
	if result == nil || !result.IsError {
		t.Fatal("expected oauth2 error for invalid code")
	}

	result = callTool(t, a, "confirm_verification", map[string]any{"token": "invalid-token"})
	if result == nil || !result.IsError {
		t.Fatal("expected confirm_verification error for invalid token")
	}
}

func TestLiveHTTPHealth(t *testing.T) {
	cfg, ok := liveConfig()
	if !ok {
		t.Skip("set POCKETBASE_URL, POCKETBASE_ADMIN_EMAIL, POCKETBASE_ADMIN_PASSWORD for live acceptance")
	}
	waitForPocketBase(t, cfg.PocketBaseURL)

	a := newApp(cfg)
	mcpServer := newMCPServer(a)
	handler := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath("/mcp"))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/mcp", handler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status %d", resp.StatusCode)
	}
}

func cleanupLiveCollection(t *testing.T, a *app) {
	t.Helper()
	result := callTool(t, a, "delete_collection", map[string]any{
		"collectionIdOrName": liveTestCollection,
	})
	if result != nil && result.IsError {
		// collection may not exist on first run
		return
	}
}

func extractJSONField(t *testing.T, raw, key string) string {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}

func extractNestedToken(t *testing.T, raw string) string {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if token, ok := obj["token"].(string); ok {
		return token
	}
	return ""
}
