package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// MockPocketBase is a minimal PocketBase REST API mock for integration tests.
type MockPocketBase struct {
	Server *httptest.Server
	URL    string

	mu           sync.Mutex
	adminToken   string
	collections  []map[string]any
	records      map[string][]map[string]any
	authFailures bool
}

// NewMockPocketBase starts an httptest server mimicking PocketBase admin and record APIs.
func NewMockPocketBase() *MockPocketBase {
	m := &MockPocketBase{
		adminToken:  "mock-admin-token",
		collections: defaultCollections(),
		records: map[string][]map[string]any{
			"posts": {
				{"id": "rec1", "title": "hello", "collectionId": "posts", "collectionName": "posts"},
			},
			"users": {},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/collections/_superusers/auth-with-password", m.handleAdminAuth)
	mux.HandleFunc("/api/collections/", m.handleCollections)
	mux.HandleFunc("/api/collections", m.handleCollectionsRoot)
	mux.HandleFunc("/api/backups", m.handleBackups)

	m.Server = httptest.NewServer(mux)
	m.URL = m.Server.URL
	return m
}

func defaultCollections() []map[string]any {
	return []map[string]any{
		{"id": "users", "name": "users", "type": "auth"},
		{"id": "posts", "name": "posts", "type": "base"},
	}
}

func (m *MockPocketBase) Close() {
	m.Server.Close()
}

func (m *MockPocketBase) SetAuthFailures(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authFailures = fail
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writePBError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"message": msg,
		"data":    map[string]any{},
	})
}

func (m *MockPocketBase) handleAdminAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	m.mu.Lock()
	fail := m.authFailures
	m.mu.Unlock()
	if fail {
		writePBError(w, http.StatusBadRequest, "Invalid login credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": m.adminToken,
		"record": map[string]any{
			"id": "admin1", "email": "admin@example.com",
		},
	})
}

func (m *MockPocketBase) handleBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !m.checkAdmin(w, r) {
		return
	}
	var body map[string]string
	_ = json.NewDecoder(r.Body).Decode(&body)
	name := body["name"]
	if name == "" {
		name = "backup"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key":  "backup-" + name,
		"name": name,
	})
}

func (m *MockPocketBase) checkAdmin(w http.ResponseWriter, r *http.Request) bool {
	m.mu.Lock()
	fail := m.authFailures
	m.mu.Unlock()
	if fail || r.Header.Get("Authorization") == "" {
		writePBError(w, http.StatusUnauthorized, "The request requires valid admin authorization token")
		return false
	}
	return true
}

func (m *MockPocketBase) handleCollectionsRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/collections" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !m.checkAdmin(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"page": 1, "perPage": 100, "totalItems": len(m.collections),
			"totalPages": 1, "items": m.collections,
		})
	case http.MethodPost:
		if !m.checkAdmin(w, r) {
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["id"] = body["name"]
		m.collections = append(m.collections, body)
		name, _ := body["name"].(string)
		m.records[name] = []map[string]any{}
		writeJSON(w, http.StatusOK, body)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *MockPocketBase) handleCollections(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/collections/")
	parts := strings.Split(path, "/")

	// auth-with-password for user collections
	if len(parts) >= 2 && parts[1] == "auth-with-password" {
		m.handleUserAuth(w, r, parts[0])
		return
	}

	// records sub-resource
	if len(parts) >= 2 && parts[1] == "records" {
		m.handleRecords(w, r, parts[0], parts[2:])
		return
	}

	// auth sub-resources
	if len(parts) >= 2 {
		switch parts[1] {
		case "auth-methods":
			if r.Method == http.MethodGet {
				writeJSON(w, http.StatusOK, map[string]any{
					"password": map[string]any{"enabled": true},
					"oauth2":   map[string]any{"enabled": true, "providers": []any{map[string]any{"name": "google"}}},
					"otp":      map[string]any{"enabled": true},
				})
				return
			}
		case "auth-with-oauth2":
			if r.Method == http.MethodPost {
				writeJSON(w, http.StatusOK, map[string]any{
					"token": "oauth-token",
					"record": map[string]any{"id": "oauth-user", "email": "oauth@example.com"},
				})
				return
			}
		case "request-otp":
			if r.Method == http.MethodPost {
				writeJSON(w, http.StatusOK, map[string]any{"otpId": "otp-123"})
				return
			}
		case "auth-refresh":
			if r.Method == http.MethodPost {
				if r.Header.Get("Authorization") == "" {
					writePBError(w, http.StatusUnauthorized, "Missing auth token")
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"token": "refreshed-token",
					"record": map[string]any{"id": "user1"},
				})
				return
			}
		case "impersonate":
			if r.Method == http.MethodPost && len(parts) >= 3 {
				if !m.checkAdmin(w, r) {
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"token": "impersonate-token",
					"record": map[string]any{"id": parts[2]},
				})
				return
			}
		case "request-verification", "confirm-verification",
			"request-password-reset", "confirm-password-reset",
			"request-email-change", "confirm-email-change":
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}

	switch r.Method {
	case http.MethodGet:
		if path == "" || !strings.Contains(path, "/") {
			if !m.checkAdmin(w, r) {
				return
			}
			if idOrName := strings.TrimSuffix(path, "/"); idOrName != "" && !strings.Contains(idOrName, "?") {
				for _, c := range m.collections {
					if c["id"] == idOrName || c["name"] == idOrName {
						writeJSON(w, http.StatusOK, c)
						return
					}
				}
				writePBError(w, http.StatusNotFound, "Collection not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"page":       1,
				"perPage":    100,
				"totalItems": len(m.collections),
				"totalPages": 1,
				"items":      m.collections,
			})
			return
		}
		idOrName := strings.Split(path, "/")[0]
		if !m.checkAdmin(w, r) {
			return
		}
		for _, c := range m.collections {
			if c["id"] == idOrName || c["name"] == idOrName {
				writeJSON(w, http.StatusOK, c)
				return
			}
		}
		writePBError(w, http.StatusNotFound, "Collection not found")

	case http.MethodPost:
		if !m.checkAdmin(w, r) {
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["id"] = body["name"]
		m.collections = append(m.collections, body)
		m.records[body["name"].(string)] = []map[string]any{}
		writeJSON(w, http.StatusOK, body)

	case http.MethodPatch:
		if !m.checkAdmin(w, r) {
			return
		}
		idOrName := parts[0]
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		for i, c := range m.collections {
			if c["id"] == idOrName || c["name"] == idOrName {
				for k, v := range body {
					c[k] = v
				}
				m.collections[i] = c
				writeJSON(w, http.StatusOK, c)
				return
			}
		}
		writePBError(w, http.StatusNotFound, "Collection not found")

	case http.MethodDelete:
		if !m.checkAdmin(w, r) {
			return
		}
		idOrName := parts[0]
		for i, c := range m.collections {
			if c["id"] == idOrName || c["name"] == idOrName {
				m.collections = append(m.collections[:i], m.collections[i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		writePBError(w, http.StatusNotFound, "Collection not found")

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *MockPocketBase) handleUserAuth(w http.ResponseWriter, r *http.Request, collection string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Identity string `json:"identity"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Password == "wrong" {
		writePBError(w, http.StatusBadRequest, "Invalid login credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": "user-token",
		"record": map[string]any{
			"id": "user1", "email": body.Identity, "collectionName": collection,
		},
	})
}

func (m *MockPocketBase) handleRecords(w http.ResponseWriter, r *http.Request, collection string, tail []string) {
	recs := m.records[collection]
	if recs == nil {
		writePBError(w, http.StatusNotFound, "Missing or invalid collection context")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if len(tail) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"page": 1, "perPage": 50, "totalItems": len(recs), "totalPages": 1, "items": recs,
			})
			return
		}
		id := tail[0]
		for _, rec := range recs {
			if rec["id"] == id {
				writeJSON(w, http.StatusOK, rec)
				return
			}
		}
		writePBError(w, http.StatusNotFound, "Record not found")

	case http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["id"] = "rec-new"
		body["collectionName"] = collection
		m.records[collection] = append(recs, body)
		writeJSON(w, http.StatusOK, body)

	case http.MethodPatch:
		if len(tail) == 0 {
			writePBError(w, http.StatusBadRequest, "Record id required")
			return
		}
		id := tail[0]
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		for i, rec := range recs {
			if rec["id"] == id {
				for k, v := range body {
					rec[k] = v
				}
				m.records[collection][i] = rec
				writeJSON(w, http.StatusOK, rec)
				return
			}
		}
		writePBError(w, http.StatusNotFound, "Record not found")

	case http.MethodDelete:
		if len(tail) == 0 {
			writePBError(w, http.StatusBadRequest, "Record id required")
			return
		}
		id := tail[0]
		for i, rec := range recs {
			if rec["id"] == id {
				m.records[collection] = append(recs[:i], recs[i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		writePBError(w, http.StatusNotFound, "Record not found")

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
