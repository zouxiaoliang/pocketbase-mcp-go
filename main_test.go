package main

import (
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/zouxiaoliang/pocketbase-mcp-go/tests/testutil"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	os.Exit(m.Run())
}

func testConfig(pbURL string) config {
	return config{
		PocketBaseURL: pbURL,
		AdminEmail:    "admin@example.com",
		AdminPassword: "secret",
		Transport:     "http",
		HTTPAddr:      ":8080",
	}
}

func newTestApp(t *testing.T, mock *testutil.MockPocketBase) *app {
	t.Helper()
	return newApp(testConfig(mock.URL))
}

func callTool(t *testing.T, a *app, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := testutil.NewToolRequest(name, args)
	ctx := context.Background()

	var (
		result *mcp.CallToolResult
		err    error
	)
	switch name {
	case "list_collections":
		result, err = a.handleListCollections(ctx, req)
	case "list_records":
		result, err = a.handleListRecords(ctx, req)
	case "create_record":
		result, err = a.handleCreateRecord(ctx, req)
	case "update_record":
		result, err = a.handleUpdateRecord(ctx, req)
	case "delete_record":
		result, err = a.handleDeleteRecord(ctx, req)
	case "create_collection":
		result, err = a.handleCreateCollection(ctx, req)
	case "get_collection":
		result, err = a.handleGetCollection(ctx, req)
	case "update_collection":
		result, err = a.handleUpdateCollection(ctx, req)
	case "delete_collection":
		result, err = a.handleDeleteCollection(ctx, req)
	case "authenticate_user":
		result, err = a.handleAuthenticateUser(ctx, req)
	case "create_user":
		result, err = a.handleCreateUser(ctx, req)
	case "backup_database":
		result, err = a.handleBackupDatabase(ctx, req)
	case "list_auth_methods":
		result, err = a.handleListAuthMethods(ctx, req)
	case "authenticate_with_oauth2":
		result, err = a.handleAuthenticateWithOAuth2(ctx, req)
	case "authenticate_with_otp":
		result, err = a.handleAuthenticateWithOTP(ctx, req)
	case "auth_refresh":
		result, err = a.handleAuthRefresh(ctx, req)
	case "request_verification":
		result, err = a.handleRequestVerification(ctx, req)
	case "confirm_verification":
		result, err = a.handleConfirmVerification(ctx, req)
	case "request_password_reset":
		result, err = a.handleRequestPasswordReset(ctx, req)
	case "confirm_password_reset":
		result, err = a.handleConfirmPasswordReset(ctx, req)
	case "request_email_change":
		result, err = a.handleRequestEmailChange(ctx, req)
	case "confirm_email_change":
		result, err = a.handleConfirmEmailChange(ctx, req)
	case "impersonate_user":
		result, err = a.handleImpersonateUser(ctx, req)
	case "import_data":
		result, err = a.handleImportData(ctx, req)
	default:
		t.Fatalf("unknown tool %q", name)
	}
	if err != nil {
		t.Fatalf("tool %s error: %v", name, err)
	}
	return result
}

func TestFlattenErrors(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{
			name: "nested message",
			in: map[string]any{
				"message": "top",
				"data": map[string]any{
					"email": map[string]any{"message": "invalid email"},
				},
			},
			want: "top\ninvalid email",
		},
		{name: "string", in: "plain error", want: "plain error"},
		{name: "empty", in: 123, want: "No errors found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pocketbaseErrorMessage(tt.in)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestLoadConfigMissingURL(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	t.Setenv("POCKETBASE_URL", "")
	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "POCKETBASE_URL") {
		t.Fatalf("expected POCKETBASE_URL error, got %v", err)
	}
}

func TestStdioTransportConfig(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	t.Setenv("POCKETBASE_URL", "http://127.0.0.1:8090")
	t.Setenv("MCP_TRANSPORT", "stdio")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Transport != "stdio" {
		t.Fatalf("expected stdio transport, got %q", cfg.Transport)
	}
}

func TestLoadConfigAdminMode(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	t.Setenv("POCKETBASE_URL", "http://127.0.0.1:8090")
	t.Setenv("POCKETBASE_ADMIN_MODE", "true")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UseAdminMode {
		t.Fatal("expected admin mode enabled")
	}

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	t.Setenv("POCKETBASE_ADMIN_MODE", "false")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UseAdminMode {
		t.Fatal("expected admin mode disabled")
	}
}

func TestRecordAuthHeaderAdminMode(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		pb := NewPBClient(config{
			PocketBaseURL: "http://127.0.0.1:8090",
			AdminEmail:    "admin@example.com",
			AdminPassword: "secret",
			UseAdminMode:  false,
		})
		auth, err := pb.recordAuthHeader(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if auth != "" {
			t.Fatalf("expected empty auth header, got %q", auth)
		}
	})

	t.Run("enabled without credentials", func(t *testing.T) {
		pb := NewPBClient(config{
			PocketBaseURL: "http://127.0.0.1:8090",
			UseAdminMode:  true,
		})
		_, err := pb.recordAuthHeader(context.Background())
		if err == nil || !strings.Contains(err.Error(), "admin mode enabled") {
			t.Fatalf("expected admin mode credentials error, got %v", err)
		}
	})
}

func TestListCollections(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "list_collections", map[string]any{})
	testutil.AssertNoToolError(t, result)
	text := testutil.ResultText(t, result)
	if !strings.Contains(text, "posts") || !strings.Contains(text, "users") {
		t.Fatalf("expected collections in response, got %s", text)
	}

	mock.SetAuthFailures(true)
	result = callTool(t, a, "list_collections", map[string]any{})
	if result == nil || !result.IsError {
		t.Fatal("expected auth error result")
	}
}

func TestListRecords(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "list_records", map[string]any{"collection": "posts"})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "rec1") {
		t.Fatal("expected record in response")
	}

	result = callTool(t, a, "list_records", map[string]any{
		"collection": "missing",
		"page":       float64(1),
		"perPage":    float64(10),
	})
	if result == nil || !result.IsError {
		t.Fatal("expected error for missing collection")
	}
}

func TestCreateRecord(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "create_record", map[string]any{
		"collection": "posts",
		"data":       map[string]any{"title": "new post"},
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "rec-new") {
		t.Fatal("expected created record id")
	}
}

func TestUpdateRecord(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "update_record", map[string]any{
		"collection": "posts",
		"id":         "rec1",
		"data":       map[string]any{"title": "updated"},
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "updated") {
		t.Fatal("expected updated title")
	}

	result = callTool(t, a, "update_record", map[string]any{
		"collection": "posts",
		"id":         "missing",
		"data":       map[string]any{"title": "x"},
	})
	if result == nil || !result.IsError {
		t.Fatal("expected not found error")
	}
}

func TestDeleteRecord(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "delete_record", map[string]any{
		"collection": "posts",
		"id":         "rec1",
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "Successfully deleted") {
		t.Fatal("expected delete confirmation")
	}
}

func TestCreateCollection(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "create_collection", map[string]any{
		"name": "test_mcp",
		"fields": []any{
			map[string]any{"name": "title", "type": "text", "required": true},
		},
	})
	testutil.AssertNoToolError(t, result)
	text := testutil.ResultText(t, result)
	if !strings.Contains(text, "created") || !strings.Contains(text, "updated") {
		t.Fatal("expected autodate fields in collection")
	}
}

func TestGetCollection(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "get_collection", map[string]any{
		"collectionIdOrName": "posts",
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "posts") {
		t.Fatal("expected posts collection")
	}
}

func TestUpdateCollection(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "update_collection", map[string]any{
		"collectionIdOrName": "posts",
		"listRule":           "",
	})
	testutil.AssertNoToolError(t, result)
}

func TestDeleteCollection(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "delete_collection", map[string]any{
		"collectionIdOrName": "posts",
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "Successfully deleted") {
		t.Fatal("expected delete confirmation")
	}
}

func TestAuthenticateUser(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "authenticate_user", map[string]any{
		"email":    "user@example.com",
		"password": "secret",
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "user-token") {
		t.Fatal("expected auth token")
	}

	result = callTool(t, a, "authenticate_user", map[string]any{
		"email":    "user@example.com",
		"password": "wrong",
	})
	if result == nil || !result.IsError {
		t.Fatal("expected auth failure")
	}

	result = callTool(t, a, "authenticate_user", map[string]any{
		"isAdmin":  true,
		"email":    "admin@example.com",
		"password": "secret",
	})
	testutil.AssertNoToolError(t, result)
}

func TestCreateUser(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "create_user", map[string]any{
		"email":           "new@example.com",
		"password":        "secret123",
		"passwordConfirm": "secret123",
		"name":            "New User",
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "new@example.com") {
		t.Fatal("expected new user email")
	}
}

func TestBackupDatabase(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "backup_database", map[string]any{
		"name": "quickstart-backup",
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "quickstart-backup") {
		t.Fatal("expected backup name in response")
	}
}

func TestGinHealthEndpoint(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	cfg := testConfig(mock.URL)

	a := newApp(cfg)
	mcpServer := newMCPServer(a)
	r := gin.New()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	httpHandler := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath("/mcp"))
	r.Any("/mcp", gin.WrapH(httpHandler))

	ts := httptest.NewServer(r)
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

func TestMCPServerRegistersAllTools(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newApp(testConfig(mock.URL))
	s := newMCPServer(a)

	expected := []string{
		"list_collections", "list_records", "create_record", "update_record", "delete_record",
		"create_collection", "get_collection", "update_collection", "delete_collection",
		"authenticate_user", "create_user", "backup_database",
		"list_auth_methods", "authenticate_with_oauth2", "authenticate_with_otp", "auth_refresh",
		"request_verification", "confirm_verification", "request_password_reset", "confirm_password_reset",
		"request_email_change", "confirm_email_change", "impersonate_user", "import_data",
	}
	tools := s.ListTools()
	if len(tools) != len(expected) {
		t.Fatalf("expected %d tools, got %d", len(expected), len(tools))
	}
	for _, name := range expected {
		if _, ok := tools[name]; !ok {
			t.Fatalf("missing tool %q", name)
		}
	}
}

func TestListAuthMethods(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "list_auth_methods", map[string]any{})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "password") {
		t.Fatal("expected auth methods")
	}
}

func TestAuthenticateWithOAuth2(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "authenticate_with_oauth2", map[string]any{
		"provider": "google", "code": "code", "codeVerifier": "verifier", "redirectUrl": "http://localhost/cb",
	})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "oauth-token") {
		t.Fatal("expected oauth token")
	}
}

func TestAuthenticateWithOTP(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "authenticate_with_otp", map[string]any{"email": "user@example.com"})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "otp-123") {
		t.Fatal("expected otp id")
	}
}

func TestAuthRefresh(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "auth_refresh", map[string]any{"token": "user-token"})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "refreshed-token") {
		t.Fatal("expected refreshed token")
	}
}

func TestRequestVerification(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "request_verification", map[string]any{"email": "user@example.com"})
	testutil.AssertNoToolError(t, result)
}

func TestConfirmVerification(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "confirm_verification", map[string]any{"token": "verify-token"})
	testutil.AssertNoToolError(t, result)
}

func TestRequestPasswordReset(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "request_password_reset", map[string]any{"email": "user@example.com"})
	testutil.AssertNoToolError(t, result)
}

func TestConfirmPasswordReset(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "confirm_password_reset", map[string]any{
		"token": "reset-token", "password": "newpass", "passwordConfirm": "newpass",
	})
	testutil.AssertNoToolError(t, result)
}

func TestRequestEmailChange(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "request_email_change", map[string]any{
		"newEmail": "new@example.com", "token": "user-token",
	})
	testutil.AssertNoToolError(t, result)
}

func TestConfirmEmailChange(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "confirm_email_change", map[string]any{
		"token": "change-token", "password": "secret",
	})
	testutil.AssertNoToolError(t, result)
}

func TestImpersonateUser(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)
	result := callTool(t, a, "impersonate_user", map[string]any{"id": "user1"})
	testutil.AssertNoToolError(t, result)
	if !strings.Contains(testutil.ResultText(t, result), "impersonate-token") {
		t.Fatal("expected impersonate token")
	}
}

func TestImportData(t *testing.T) {
	mock := testutil.NewMockPocketBase()
	defer mock.Close()
	a := newTestApp(t, mock)

	result := callTool(t, a, "import_data", map[string]any{
		"collection": "posts",
		"mode":       "create",
		"data": []any{
			map[string]any{"title": "imported-1"},
			map[string]any{"title": "imported-2"},
		},
	})
	testutil.AssertNoToolError(t, result)
	text := testutil.ResultText(t, result)
	if !strings.Contains(text, `"imported": 2`) {
		t.Fatalf("expected 2 imported records, got %s", text)
	}
}
