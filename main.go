package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// --- Config ---

type config struct {
	PocketBaseURL string
	AdminEmail    string
	AdminPassword string
	UseAdminMode  bool
	Transport     string
	HTTPAddr      string
}

func parseBoolEnv(key string, defaultValue bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultValue
	}
	return b
}

func loadConfig() (config, error) {
	fs := flag.NewFlagSet("pocketbase-mcp-go", flag.ContinueOnError)
	transport := fs.String("transport", "", "MCP transport: http or stdio")
	httpAddr := fs.String("http-addr", "", "HTTP listen address for Gin MCP server")
	adminMode := fs.String("admin-mode", "", "Use superuser for record CRUD: true or false")
	_ = fs.Parse(programArgs())

	cfg := config{
		PocketBaseURL: strings.TrimRight(os.Getenv("POCKETBASE_URL"), "/"),
		AdminEmail:    os.Getenv("POCKETBASE_ADMIN_EMAIL"),
		AdminPassword: os.Getenv("POCKETBASE_ADMIN_PASSWORD"),
		UseAdminMode:  parseBoolEnv("POCKETBASE_ADMIN_MODE", false),
		Transport:     strings.ToLower(strings.TrimSpace(os.Getenv("MCP_TRANSPORT"))),
		HTTPAddr:      strings.TrimSpace(os.Getenv("MCP_HTTP_ADDR")),
	}

	if *adminMode != "" {
		if b, err := strconv.ParseBool(*adminMode); err == nil {
			cfg.UseAdminMode = b
		}
	}

	if *transport != "" {
		cfg.Transport = strings.ToLower(strings.TrimSpace(*transport))
	}
	if cfg.Transport == "" {
		cfg.Transport = "http"
	}
	if *httpAddr != "" {
		cfg.HTTPAddr = strings.TrimSpace(*httpAddr)
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}

	if cfg.PocketBaseURL == "" {
		return cfg, errors.New("POCKETBASE_URL environment variable is required")
	}
	return cfg, nil
}

func programArgs() []string {
	var args []string
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test") {
			continue
		}
		args = append(args, arg)
	}
	return args
}

// --- PocketBase Client ---

type pbClient struct {
	baseURL       string
	httpClient    *http.Client
	adminEmail    string
	adminPassword string
	useAdminMode  bool
	adminToken    string
}

func NewPBClient(cfg config) *pbClient {
	return &pbClient{
		baseURL:       cfg.PocketBaseURL,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		adminEmail:    cfg.AdminEmail,
		adminPassword: cfg.AdminPassword,
		useAdminMode:  cfg.UseAdminMode,
	}
}

type pbErrorBody struct {
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

func flattenErrors(errs any) []string {
	switch v := errs.(type) {
	case []any:
		var out []string
		for _, item := range v {
			out = append(out, flattenErrors(item)...)
		}
		return out
	case map[string]any:
		if msg, ok := v["message"].(string); ok && msg != "" {
			out := []string{msg}
			if data, ok := v["data"]; ok {
				out = append(out, flattenErrors(data)...)
			}
			return out
		}
		var out []string
		for _, val := range v {
			out = append(out, flattenErrors(val)...)
		}
		return out
	case string:
		if v != "" {
			return []string{v}
		}
		return nil
	case error:
		if v != nil {
			return []string{v.Error()}
		}
		return nil
	default:
		return nil
	}
}

func pocketbaseErrorMessage(errs any) string {
	msgs := flattenErrors(errs)
	if len(msgs) == 0 {
		return "No errors found"
	}
	return strings.Join(msgs, "\n")
}

func (c *pbClient) doRequest(ctx context.Context, method, path string, body any, authToken string) (json.RawMessage, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode >= 400 {
		var pbErr pbErrorBody
		if json.Unmarshal(raw, &pbErr) == nil && (pbErr.Message != "" || len(pbErr.Data) > 0) {
			return nil, resp.StatusCode, errors.New(pocketbaseErrorMessage(map[string]any{
				"message": pbErr.Message,
				"data":    pbErr.Data,
			}))
		}
		return nil, resp.StatusCode, errors.New(string(raw))
	}

	if len(raw) == 0 {
		return json.RawMessage("{}"), resp.StatusCode, nil
	}
	return json.RawMessage(raw), resp.StatusCode, nil
}

func (c *pbClient) authWithPassword(ctx context.Context, collection, email, password string) (json.RawMessage, error) {
	payload := map[string]string{
		"identity": email,
		"password": password,
	}
	raw, _, err := c.doRequest(ctx, http.MethodPost, "/api/collections/"+url.PathEscape(collection)+"/auth-with-password", payload, "")
	return raw, err
}

func (c *pbClient) authAdmin(ctx context.Context) error {
	if c.adminEmail == "" || c.adminPassword == "" {
		return errors.New("admin credentials missing: set POCKETBASE_ADMIN_EMAIL and POCKETBASE_ADMIN_PASSWORD")
	}
	raw, err := c.authWithPassword(ctx, "_superusers", c.adminEmail, c.adminPassword)
	if err != nil {
		return err
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.Token == "" {
		return errors.New("admin authentication failed: empty token")
	}
	c.adminToken = result.Token
	return nil
}

func (c *pbClient) adminAuthHeader() string {
	if c.adminToken == "" {
		return ""
	}
	return c.adminToken
}

// recordAuthHeader returns a superuser token when admin mode is enabled and credentials are configured.
func (c *pbClient) recordAuthHeader(ctx context.Context) (string, error) {
	if !c.useAdminMode {
		return "", nil
	}
	if c.adminEmail == "" || c.adminPassword == "" {
		return "", errors.New("admin mode enabled: set POCKETBASE_ADMIN_EMAIL and POCKETBASE_ADMIN_PASSWORD")
	}
	if err := c.authAdmin(ctx); err != nil {
		return "", err
	}
	return c.adminAuthHeader(), nil
}

func (c *pbClient) listCollections(ctx context.Context, filter, sort string) (json.RawMessage, error) {
	if err := c.authAdmin(ctx); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("page", "1")
	q.Set("perPage", "100")
	if filter != "" {
		q.Set("filter", filter)
	}
	if sort != "" {
		q.Set("sort", sort)
	}
	path := "/api/collections?" + q.Encode()
	raw, _, err := c.doRequest(ctx, http.MethodGet, path, nil, c.adminAuthHeader())
	return raw, err
}

func (c *pbClient) getCollection(ctx context.Context, idOrName, fields string) (json.RawMessage, error) {
	if err := c.authAdmin(ctx); err != nil {
		return nil, err
	}
	path := "/api/collections/" + url.PathEscape(idOrName)
	if fields != "" {
		path += "?fields=" + url.QueryEscape(fields)
	}
	raw, _, err := c.doRequest(ctx, http.MethodGet, path, nil, c.adminAuthHeader())
	return raw, err
}

func (c *pbClient) createCollection(ctx context.Context, body map[string]any) (json.RawMessage, error) {
	if err := c.authAdmin(ctx); err != nil {
		return nil, err
	}
	fields, _ := body["fields"].([]any)
	defaultFields := []map[string]any{
		{"hidden": false, "id": "autodate_created", "name": "created", "onCreate": true, "onUpdate": false, "presentable": false, "system": false, "type": "autodate"},
		{"hidden": false, "id": "autodate_updated", "name": "updated", "onCreate": true, "onUpdate": true, "presentable": false, "system": false, "type": "autodate"},
	}
	if fields == nil {
		fields = []any{}
	}
	for _, f := range defaultFields {
		fields = append(fields, f)
	}
	body["fields"] = fields
	raw, _, err := c.doRequest(ctx, http.MethodPost, "/api/collections", body, c.adminAuthHeader())
	return raw, err
}

func (c *pbClient) updateCollection(ctx context.Context, idOrName string, body map[string]any) (json.RawMessage, error) {
	if err := c.authAdmin(ctx); err != nil {
		return nil, err
	}
	path := "/api/collections/" + url.PathEscape(idOrName)
	raw, _, err := c.doRequest(ctx, http.MethodPatch, path, body, c.adminAuthHeader())
	return raw, err
}

func (c *pbClient) deleteCollection(ctx context.Context, idOrName string) error {
	if err := c.authAdmin(ctx); err != nil {
		return err
	}
	path := "/api/collections/" + url.PathEscape(idOrName)
	_, _, err := c.doRequest(ctx, http.MethodDelete, path, nil, c.adminAuthHeader())
	return err
}

func (c *pbClient) listRecords(ctx context.Context, collection, filter, sort string, page, perPage int) (json.RawMessage, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 50
	}
	auth, err := c.recordAuthHeader(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("perPage", strconv.Itoa(perPage))
	if filter != "" {
		q.Set("filter", filter)
	}
	if sort != "" {
		q.Set("sort", sort)
	}
	path := "/api/collections/" + url.PathEscape(collection) + "/records?" + q.Encode()
	raw, _, err := c.doRequest(ctx, http.MethodGet, path, nil, auth)
	return raw, err
}

func (c *pbClient) createRecord(ctx context.Context, collection string, data map[string]any) (json.RawMessage, error) {
	auth, err := c.recordAuthHeader(ctx)
	if err != nil {
		return nil, err
	}
	path := "/api/collections/" + url.PathEscape(collection) + "/records"
	raw, _, err := c.doRequest(ctx, http.MethodPost, path, data, auth)
	return raw, err
}

func (c *pbClient) updateRecord(ctx context.Context, collection, id string, data map[string]any) (json.RawMessage, error) {
	auth, err := c.recordAuthHeader(ctx)
	if err != nil {
		return nil, err
	}
	path := "/api/collections/" + url.PathEscape(collection) + "/records/" + url.PathEscape(id)
	raw, _, err := c.doRequest(ctx, http.MethodPatch, path, data, auth)
	return raw, err
}

func (c *pbClient) deleteRecord(ctx context.Context, collection, id string) error {
	auth, err := c.recordAuthHeader(ctx)
	if err != nil {
		return err
	}
	path := "/api/collections/" + url.PathEscape(collection) + "/records/" + url.PathEscape(id)
	_, _, err = c.doRequest(ctx, http.MethodDelete, path, nil, auth)
	return err
}

func (c *pbClient) createUser(ctx context.Context, collection string, data map[string]any) (json.RawMessage, error) {
	return c.createRecord(ctx, collection, data)
}

func (c *pbClient) createBackup(ctx context.Context, name string) (json.RawMessage, error) {
	if err := c.authAdmin(ctx); err != nil {
		return nil, err
	}
	name = normalizeBackupName(name)
	body := map[string]string{"name": name}
	raw, status, err := c.doRequest(ctx, http.MethodPost, "/api/backups", body, c.adminAuthHeader())
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent || len(raw) == 0 || string(raw) == "{}" {
		out, _ := json.Marshal(map[string]any{"name": name, "success": true})
		return json.RawMessage(out), nil
	}
	return raw, nil
}

func normalizeBackupName(name string) string {
	if name == "" {
		name = fmt.Sprintf("backup-%d", time.Now().Unix())
	}
	if !strings.HasSuffix(strings.ToLower(name), ".zip") {
		name += ".zip"
	}
	return name
}

func (c *pbClient) collectionBase(collection string) string {
	return "/api/collections/" + url.PathEscape(collection)
}

func (c *pbClient) listAuthMethods(ctx context.Context, collection string) (json.RawMessage, error) {
	raw, _, err := c.doRequest(ctx, http.MethodGet, c.collectionBase(collection)+"/auth-methods", nil, "")
	return raw, err
}

func (c *pbClient) authWithOAuth2(ctx context.Context, collection, provider, code, codeVerifier, redirectURL string) (json.RawMessage, error) {
	body := map[string]string{
		"provider":      provider,
		"code":            code,
		"codeVerifier":    codeVerifier,
		"redirectURL":   redirectURL,
	}
	raw, _, err := c.doRequest(ctx, http.MethodPost, c.collectionBase(collection)+"/auth-with-oauth2", body, "")
	return raw, err
}

func (c *pbClient) requestOTP(ctx context.Context, collection, email string) (json.RawMessage, error) {
	body := map[string]string{"email": email}
	raw, _, err := c.doRequest(ctx, http.MethodPost, c.collectionBase(collection)+"/request-otp", body, "")
	return raw, err
}

func (c *pbClient) authRefresh(ctx context.Context, collection, token string) (json.RawMessage, error) {
	raw, _, err := c.doRequest(ctx, http.MethodPost, c.collectionBase(collection)+"/auth-refresh", map[string]any{}, token)
	return raw, err
}

func (c *pbClient) postAuthAction(ctx context.Context, collection, action string, body any, token string) (json.RawMessage, error) {
	raw, status, err := c.doRequest(ctx, http.MethodPost, c.collectionBase(collection)+"/"+action, body, token)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "{}" {
		return json.RawMessage(`{"success":true}`), nil
	}
	_ = status
	return raw, nil
}

func (c *pbClient) impersonate(ctx context.Context, collection, id string, duration int) (json.RawMessage, error) {
	if err := c.authAdmin(ctx); err != nil {
		return nil, err
	}
	body := map[string]any{"duration": duration}
	path := c.collectionBase(collection) + "/impersonate/" + url.PathEscape(id)
	raw, _, err := c.doRequest(ctx, http.MethodPost, path, body, c.adminAuthHeader())
	return raw, err
}

func (c *pbClient) importData(ctx context.Context, collection, mode string, records []map[string]any) (json.RawMessage, error) {
	if mode == "" {
		mode = "create"
	}
	results := make([]map[string]any, 0, len(records))
	for i, record := range records {
		var (
			raw json.RawMessage
			err error
		)
		switch mode {
		case "update":
			id, _ := record["id"].(string)
			if id == "" {
				return nil, fmt.Errorf("record at index %d missing id for update mode", i)
			}
			data := map[string]any{}
			for k, v := range record {
				if k != "id" {
					data[k] = v
				}
			}
			raw, err = c.updateRecord(ctx, collection, id, data)
		case "upsert":
			id, _ := record["id"].(string)
			if id == "" {
				raw, err = c.createRecord(ctx, collection, record)
			} else {
				data := map[string]any{}
				for k, v := range record {
					if k != "id" {
						data[k] = v
					}
				}
				raw, err = c.updateRecord(ctx, collection, id, data)
				if err != nil {
					raw, err = c.createRecord(ctx, collection, record)
				}
			}
		default:
			raw, err = c.createRecord(ctx, collection, record)
		}
		if err != nil {
			return nil, fmt.Errorf("import record %d failed: %w", i, err)
		}
		var item map[string]any
		_ = json.Unmarshal(raw, &item)
		results = append(results, item)
	}
	return json.Marshal(map[string]any{
		"imported": len(results),
		"mode":     mode,
		"items":    results,
	})
}

func authCollection(args map[string]any) string {
	if v, ok := args["collection"].(string); ok && v != "" {
		return v
	}
	return "users"
}

// --- MCP Tool Handlers ---

type app struct {
	cfg config
	pb  *pbClient
}

func newApp(cfg config) *app {
	return &app{cfg: cfg, pb: NewPBClient(cfg)}
}

func toolError(err error) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(pocketbaseErrorMessage(err)), nil
}

func jsonResult(raw json.RawMessage) (*mcp.CallToolResult, error) {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return mcp.NewToolResultText(string(raw)), nil
	}
	return mcp.NewToolResultText(pretty.String()), nil
}

func (a *app) handleListCollections(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	filter, _ := args["filter"].(string)
	sort, _ := args["sort"].(string)
	raw, err := a.pb.listCollections(ctx, filter, sort)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleListRecords(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	collection, err := req.RequireString("collection")
	if err != nil {
		return toolError(err)
	}
	args := req.GetArguments()
	filter, _ := args["filter"].(string)
	sort, _ := args["sort"].(string)
	page, perPage := 1, 50
	if v, ok := args["page"].(float64); ok {
		page = int(v)
	}
	if v, ok := args["perPage"].(float64); ok {
		perPage = int(v)
	}
	raw, err := a.pb.listRecords(ctx, collection, filter, sort, page, perPage)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func requireMapArg(req mcp.CallToolRequest, key string) (map[string]any, error) {
	val, ok := req.GetArguments()[key]
	if !ok {
		return nil, fmt.Errorf("missing required argument %q", key)
	}
	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("argument %q must be an object", key)
	}
	return m, nil
}

func (a *app) handleCreateRecord(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	collection, err := req.RequireString("collection")
	if err != nil {
		return toolError(err)
	}
	data, err := requireMapArg(req, "data")
	if err != nil {
		return toolError(err)
	}
	raw, err := a.pb.createRecord(ctx, collection, data)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleUpdateRecord(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	collection, err := req.RequireString("collection")
	if err != nil {
		return toolError(err)
	}
	id, err := req.RequireString("id")
	if err != nil {
		return toolError(err)
	}
	data, err := requireMapArg(req, "data")
	if err != nil {
		return toolError(err)
	}
	raw, err := a.pb.updateRecord(ctx, collection, id, data)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleDeleteRecord(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	collection, err := req.RequireString("collection")
	if err != nil {
		return toolError(err)
	}
	id, err := req.RequireString("id")
	if err != nil {
		return toolError(err)
	}
	if err := a.pb.deleteRecord(ctx, collection, id); err != nil {
		return toolError(err)
	}
	msg := fmt.Sprintf("Successfully deleted record %s from collection %s", id, collection)
	return mcp.NewToolResultText(msg), nil
}

func (a *app) handleCreateCollection(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	body := map[string]any(req.GetArguments())
	raw, err := a.pb.createCollection(ctx, body)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleGetCollection(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idOrName, err := req.RequireString("collectionIdOrName")
	if err != nil {
		return toolError(err)
	}
	fields, _ := req.GetArguments()["fields"].(string)
	raw, err := a.pb.getCollection(ctx, idOrName, fields)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleUpdateCollection(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idOrName, err := req.RequireString("collectionIdOrName")
	if err != nil {
		return toolError(err)
	}
	body := map[string]any{}
	for k, v := range req.GetArguments() {
		if k != "collectionIdOrName" {
			body[k] = v
		}
	}
	raw, err := a.pb.updateCollection(ctx, idOrName, body)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleDeleteCollection(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idOrName, err := req.RequireString("collectionIdOrName")
	if err != nil {
		return toolError(err)
	}
	if err := a.pb.deleteCollection(ctx, idOrName); err != nil {
		return toolError(err)
	}
	return mcp.NewToolResultText(fmt.Sprintf("Successfully deleted collection %s", idOrName)), nil
}

func (a *app) handleAuthenticateUser(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	isAdmin, _ := args["isAdmin"].(bool)
	collection := "users"
	if v, ok := args["collection"].(string); ok && v != "" {
		collection = v
	}
	if isAdmin {
		collection = "_superusers"
	}
	email, _ := args["email"].(string)
	password, _ := args["password"].(string)
	if isAdmin {
		if email == "" {
			email = a.cfg.AdminEmail
		}
		if password == "" {
			password = a.cfg.AdminPassword
		}
	}
	if email == "" || password == "" {
		return toolError(errors.New("email and password are required for authentication"))
	}
	raw, err := a.pb.authWithPassword(ctx, collection, email, password)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleCreateUser(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	collection := "users"
	if v, ok := args["collection"].(string); ok && v != "" {
		collection = v
	}
	data := map[string]any{}
	for _, k := range []string{"email", "password", "passwordConfirm", "name"} {
		if v, ok := args[k]; ok {
			data[k] = v
		}
	}
	raw, err := a.pb.createUser(ctx, collection, data)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleBackupDatabase(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := req.GetArguments()["name"].(string)
	raw, err := a.pb.createBackup(ctx, name)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleListAuthMethods(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	collection := authCollection(req.GetArguments())
	raw, err := a.pb.listAuthMethods(ctx, collection)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleAuthenticateWithOAuth2(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	provider, _ := args["provider"].(string)
	code, _ := args["code"].(string)
	codeVerifier, _ := args["codeVerifier"].(string)
	redirectURL, _ := args["redirectUrl"].(string)
	if provider == "" || code == "" || codeVerifier == "" || redirectURL == "" {
		return toolError(errors.New("provider, code, codeVerifier, and redirectUrl are required"))
	}
	raw, err := a.pb.authWithOAuth2(ctx, authCollection(args), provider, code, codeVerifier, redirectURL)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleAuthenticateWithOTP(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	email, err := req.RequireString("email")
	if err != nil {
		return toolError(err)
	}
	raw, err := a.pb.requestOTP(ctx, authCollection(req.GetArguments()), email)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleAuthRefresh(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	token, _ := args["token"].(string)
	raw, err := a.pb.authRefresh(ctx, authCollection(args), token)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleRequestVerification(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	email, err := req.RequireString("email")
	if err != nil {
		return toolError(err)
	}
	raw, err := a.pb.postAuthAction(ctx, authCollection(req.GetArguments()), "request-verification", map[string]string{"email": email}, "")
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleConfirmVerification(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token, err := req.RequireString("token")
	if err != nil {
		return toolError(err)
	}
	raw, err := a.pb.postAuthAction(ctx, authCollection(req.GetArguments()), "confirm-verification", map[string]string{"token": token}, "")
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleRequestPasswordReset(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	email, err := req.RequireString("email")
	if err != nil {
		return toolError(err)
	}
	raw, err := a.pb.postAuthAction(ctx, authCollection(req.GetArguments()), "request-password-reset", map[string]string{"email": email}, "")
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleConfirmPasswordReset(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	token, _ := args["token"].(string)
	password, _ := args["password"].(string)
	passwordConfirm, _ := args["passwordConfirm"].(string)
	if token == "" || password == "" || passwordConfirm == "" {
		return toolError(errors.New("token, password, and passwordConfirm are required"))
	}
	body := map[string]string{"token": token, "password": password, "passwordConfirm": passwordConfirm}
	raw, err := a.pb.postAuthAction(ctx, authCollection(args), "confirm-password-reset", body, "")
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleRequestEmailChange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	newEmail, err := req.RequireString("newEmail")
	if err != nil {
		return toolError(err)
	}
	args := req.GetArguments()
	token, _ := args["token"].(string)
	raw, err := a.pb.postAuthAction(ctx, authCollection(args), "request-email-change", map[string]string{"newEmail": newEmail}, token)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleConfirmEmailChange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	token, _ := args["token"].(string)
	password, _ := args["password"].(string)
	if token == "" || password == "" {
		return toolError(errors.New("token and password are required"))
	}
	body := map[string]string{"token": token, "password": password}
	raw, err := a.pb.postAuthAction(ctx, authCollection(args), "confirm-email-change", body, "")
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleImpersonateUser(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	id, err := req.RequireString("id")
	if err != nil {
		return toolError(err)
	}
	collection := "users"
	if v, ok := args["collectionIdOrName"].(string); ok && v != "" {
		collection = v
	}
	duration := 3600
	if v, ok := args["duration"].(float64); ok && v > 0 {
		duration = int(v)
	}
	raw, err := a.pb.impersonate(ctx, collection, id, duration)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

func (a *app) handleImportData(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	collection, err := req.RequireString("collection")
	if err != nil {
		return toolError(err)
	}
	args := req.GetArguments()
	mode, _ := args["mode"].(string)
	rawItems, ok := args["data"].([]any)
	if !ok || len(rawItems) == 0 {
		return toolError(errors.New("data must be a non-empty array"))
	}
	records := make([]map[string]any, 0, len(rawItems))
	for i, item := range rawItems {
		m, ok := item.(map[string]any)
		if !ok {
			return toolError(fmt.Errorf("data[%d] must be an object", i))
		}
		records = append(records, m)
	}
	raw, err := a.pb.importData(ctx, collection, mode, records)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(raw)
}

// --- MCP Registration ---

func registerTools(s *server.MCPServer, a *app) {
	s.AddTool(mcp.NewTool("list_collections",
		mcp.WithDescription("List all collections in PocketBase"),
		mcp.WithString("filter", mcp.Description("Filter query for collections")),
		mcp.WithString("sort", mcp.Description("Sort order for collections")),
	), a.handleListCollections)

	s.AddTool(mcp.NewTool("list_records",
		mcp.WithDescription("List records from a collection with optional filters"),
		mcp.WithString("collection", mcp.Required(), mcp.Description("Collection name")),
		mcp.WithString("filter", mcp.Description("Filter query")),
		mcp.WithString("sort", mcp.Description("Sort field and direction")),
		mcp.WithNumber("page", mcp.Description("Page number")),
		mcp.WithNumber("perPage", mcp.Description("Items per page")),
	), a.handleListRecords)

	s.AddTool(mcp.NewTool("create_record",
		mcp.WithDescription("Create a new record in a collection"),
		mcp.WithString("collection", mcp.Required(), mcp.Description("Collection name")),
		mcp.WithObject("data", mcp.Required(), mcp.Description("Record data")),
	), a.handleCreateRecord)

	s.AddTool(mcp.NewTool("update_record",
		mcp.WithDescription("Update an existing record"),
		mcp.WithString("collection", mcp.Required(), mcp.Description("Collection name")),
		mcp.WithString("id", mcp.Required(), mcp.Description("Record ID")),
		mcp.WithObject("data", mcp.Required(), mcp.Description("Updated record data")),
	), a.handleUpdateRecord)

	s.AddTool(mcp.NewTool("delete_record",
		mcp.WithDescription("Delete a record"),
		mcp.WithString("collection", mcp.Required(), mcp.Description("Collection name")),
		mcp.WithString("id", mcp.Required(), mcp.Description("Record ID")),
	), a.handleDeleteRecord)

	s.AddTool(mcp.NewTool("create_collection",
		mcp.WithDescription("Create a new collection in PocketBase note never use created and updated because these are already created"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Unique collection name")),
		mcp.WithString("type", mcp.Description("Type of the collection"), mcp.Enum("base", "view", "auth")),
		mcp.WithArray("fields", mcp.Required(), mcp.Description("List with the collection fields")),
		mcp.WithString("createRule", mcp.Description("API rule for creating records")),
		mcp.WithString("updateRule", mcp.Description("API rule for updating records")),
		mcp.WithString("deleteRule", mcp.Description("API rule for deleting records")),
		mcp.WithString("listRule", mcp.Description("API rule for listing and viewing records")),
		mcp.WithString("viewRule", mcp.Description("API rule for viewing a single record")),
		mcp.WithString("viewQuery", mcp.Description("SQL query for view collections")),
	), a.handleCreateCollection)

	s.AddTool(mcp.NewTool("get_collection",
		mcp.WithDescription("Get details for a collection"),
		mcp.WithString("collectionIdOrName", mcp.Required(), mcp.Description("ID or name of the collection")),
		mcp.WithString("fields", mcp.Description("Comma separated string of the fields to return")),
	), a.handleGetCollection)

	s.AddTool(mcp.NewTool("update_collection",
		mcp.WithDescription("Update an existing collection in PocketBase (admin only)"),
		mcp.WithString("collectionIdOrName", mcp.Required(), mcp.Description("ID or name of the collection to update")),
	), a.handleUpdateCollection)

	s.AddTool(mcp.NewTool("delete_collection",
		mcp.WithDescription("Delete a collection from PocketBase (admin only)"),
		mcp.WithString("collectionIdOrName", mcp.Required(), mcp.Description("ID or name of the collection to delete")),
	), a.handleDeleteCollection)

	s.AddTool(mcp.NewTool("authenticate_user",
		mcp.WithDescription("Authenticate a user with email and password"),
		mcp.WithString("email", mcp.Required(), mcp.Description("User email")),
		mcp.WithString("password", mcp.Required(), mcp.Description("User password")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
		mcp.WithBoolean("isAdmin", mcp.Description("Whether to authenticate as an admin")),
	), a.handleAuthenticateUser)

	s.AddTool(mcp.NewTool("create_user",
		mcp.WithDescription("Create a new user account"),
		mcp.WithString("email", mcp.Required(), mcp.Description("User email")),
		mcp.WithString("password", mcp.Required(), mcp.Description("User password")),
		mcp.WithString("passwordConfirm", mcp.Required(), mcp.Description("Password confirmation")),
		mcp.WithString("name", mcp.Description("User name")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleCreateUser)

	s.AddTool(mcp.NewTool("backup_database",
		mcp.WithDescription("Create a backup of the PocketBase database"),
		mcp.WithString("name", mcp.Description("backup name")),
	), a.handleBackupDatabase)

	s.AddTool(mcp.NewTool("list_auth_methods",
		mcp.WithDescription("List all available authentication methods"),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleListAuthMethods)

	s.AddTool(mcp.NewTool("authenticate_with_oauth2",
		mcp.WithDescription("Authenticate a user with OAuth2"),
		mcp.WithString("provider", mcp.Required(), mcp.Description("OAuth2 provider name")),
		mcp.WithString("code", mcp.Required(), mcp.Description("Authorization code")),
		mcp.WithString("codeVerifier", mcp.Required(), mcp.Description("PKCE code verifier")),
		mcp.WithString("redirectUrl", mcp.Required(), mcp.Description("Redirect URL used in OAuth2 flow")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleAuthenticateWithOAuth2)

	s.AddTool(mcp.NewTool("authenticate_with_otp",
		mcp.WithDescription("Authenticate a user with one-time password"),
		mcp.WithString("email", mcp.Required(), mcp.Description("User email")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleAuthenticateWithOTP)

	s.AddTool(mcp.NewTool("auth_refresh",
		mcp.WithDescription("Refresh authentication token"),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
		mcp.WithString("token", mcp.Description("Existing auth token to refresh")),
	), a.handleAuthRefresh)

	s.AddTool(mcp.NewTool("request_verification",
		mcp.WithDescription("Request email verification"),
		mcp.WithString("email", mcp.Required(), mcp.Description("User email")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleRequestVerification)

	s.AddTool(mcp.NewTool("confirm_verification",
		mcp.WithDescription("Confirm email verification with token"),
		mcp.WithString("token", mcp.Required(), mcp.Description("Verification token")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleConfirmVerification)

	s.AddTool(mcp.NewTool("request_password_reset",
		mcp.WithDescription("Request password reset"),
		mcp.WithString("email", mcp.Required(), mcp.Description("User email")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleRequestPasswordReset)

	s.AddTool(mcp.NewTool("confirm_password_reset",
		mcp.WithDescription("Confirm password reset with token"),
		mcp.WithString("token", mcp.Required(), mcp.Description("Reset token")),
		mcp.WithString("password", mcp.Required(), mcp.Description("New password")),
		mcp.WithString("passwordConfirm", mcp.Required(), mcp.Description("Confirm new password")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleConfirmPasswordReset)

	s.AddTool(mcp.NewTool("request_email_change",
		mcp.WithDescription("Request email change"),
		mcp.WithString("newEmail", mcp.Required(), mcp.Description("New email address")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
		mcp.WithString("token", mcp.Description("Current user auth token")),
	), a.handleRequestEmailChange)

	s.AddTool(mcp.NewTool("confirm_email_change",
		mcp.WithDescription("Confirm email change with token"),
		mcp.WithString("token", mcp.Required(), mcp.Description("Email change token")),
		mcp.WithString("password", mcp.Required(), mcp.Description("Current password for confirmation")),
		mcp.WithString("collection", mcp.Description("Collection name (default: users)")),
	), a.handleConfirmEmailChange)

	s.AddTool(mcp.NewTool("impersonate_user",
		mcp.WithDescription("Impersonate another user (admin only)"),
		mcp.WithString("id", mcp.Required(), mcp.Description("ID of the user to impersonate")),
		mcp.WithString("collectionIdOrName", mcp.Description("Collection name or id (default: users)")),
		mcp.WithNumber("duration", mcp.Description("Token expiry time in seconds (default: 3600)")),
	), a.handleImpersonateUser)

	s.AddTool(mcp.NewTool("import_data",
		mcp.WithDescription("Import data into a collection"),
		mcp.WithString("collection", mcp.Required(), mcp.Description("Collection name")),
		mcp.WithArray("data", mcp.Required(), mcp.Description("Array of records to import")),
		mcp.WithString("mode", mcp.Description("Import mode: create, update, upsert"), mcp.Enum("create", "update", "upsert")),
	), a.handleImportData)
}

func newMCPServer(a *app) *server.MCPServer {
	s := server.NewMCPServer("pocketbase-server", "0.1.0", server.WithToolCapabilities(true))
	registerTools(s, a)
	return s
}

// --- Main ---

func run(cfg config) error {
	a := newApp(cfg)
	mcpServer := newMCPServer(a)

	switch cfg.Transport {
	case "stdio":
		fmt.Fprintln(os.Stderr, "PocketBase MCP server running on stdio")
		return server.ServeStdio(mcpServer)
	case "http":
		gin.SetMode(gin.ReleaseMode)
		r := gin.New()
		r.Use(gin.Recovery())
		r.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		httpHandler := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath("/mcp"))
		r.Any("/mcp", gin.WrapH(httpHandler))
		r.Any("/mcp/*path", gin.WrapH(httpHandler))
		fmt.Fprintf(os.Stderr, "PocketBase MCP server listening on %s (Gin HTTP, /mcp)\n", cfg.HTTPAddr)
		return r.Run(cfg.HTTPAddr)
	default:
		return fmt.Errorf("unsupported transport %q: use http or stdio", cfg.Transport)
	}
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
