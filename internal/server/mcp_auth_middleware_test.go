package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

func TestMCPAuthorization_protectsHandler(t *testing.T) {
	// Given: pgpeek trusts a Descope issuer for one audience and scope.
	descope := newFakeDescopeServer(t)
	resourceURL := "https://pgpeek.example.com/mcp"
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config(resourceURL, "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	validToken := descope.token(t, resourceURL, nil)

	tests := []struct {
		name       string
		token      string
		wantStatus int
		wantUserID string
	}{
		{"missing token", "", http.StatusUnauthorized, ""},
		{"malformed token", "not-a-jwt", http.StatusUnauthorized, ""},
		{"wrong issuer", descope.token(t, resourceURL, map[string]any{"iss": "https://issuer.example.com"}), http.StatusUnauthorized, ""},
		{"wrong audience", descope.token(t, resourceURL, map[string]any{"aud": []string{"https://other.example.com/mcp"}}), http.StatusUnauthorized, ""},
		{"additional audience", descope.token(t, resourceURL, map[string]any{"aud": []string{"client-id", "project-id", resourceURL}, "azp": "client-id"}), http.StatusNoContent, "user-123"},
		{"expired", descope.token(t, resourceURL, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}), http.StatusUnauthorized, ""},
		{"malformed scope", descope.token(t, resourceURL, map[string]any{"scope": []string{"mcp:pgpeek.read"}}), http.StatusUnauthorized, ""},
		{"missing scope", descope.token(t, resourceURL, map[string]any{"scope": "openid"}), http.StatusForbidden, ""},
		{"valid", validToken, http.StatusNoContent, "user-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: a request reaches the bearer-token middleware.
			var gotUserID string
			handler := authz.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				info := auth.TokenInfoFromContext(r.Context())
				if info != nil {
					gotUserID = info.UserID
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, bearerRequest(t, http.MethodPost, resourceURL, tt.token))

			// Then: only a valid, scoped token reaches the handler.
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if gotUserID != tt.wantUserID {
				t.Errorf("userID = %q, want %q", gotUserID, tt.wantUserID)
			}
			if tt.wantStatus == http.StatusUnauthorized || tt.wantStatus == http.StatusForbidden {
				challenge := recorder.Header().Get("WWW-Authenticate")
				if !strings.Contains(challenge, authz.resourceMetadataURL) || !strings.Contains(challenge, "mcp:pgpeek.read") {
					t.Errorf("WWW-Authenticate = %q", challenge)
				}
			}
		})
	}
}

func TestMCPAuthorization_metadataIsPublicWithCloudflareAccess(t *testing.T) {
	// Given: both Descope MCP auth and Cloudflare Access are enabled.
	descope := newFakeDescopeServer(t)
	resourceURL := "https://pgpeek.example.com/mcp"
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config(resourceURL, "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	ts := newMCPAuthTestServer(t, authz, RequireCloudflareAccess(true))

	// When: an unauthenticated OAuth client requests protected-resource metadata.
	resp, err := ts.Client().Get(ts.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatalf("Get metadata: %v", err)
	}
	defer resp.Body.Close()

	// Then: discovery remains public while ordinary application routes stay protected.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metadata status = %d", resp.StatusCode)
	}
	var metadata oauthex.ProtectedResourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		t.Fatalf("Decode metadata: %v", err)
	}
	if metadata.Resource != resourceURL || len(metadata.BearerMethodsSupported) != 1 || metadata.BearerMethodsSupported[0] != "header" {
		t.Fatalf("metadata = %+v", metadata)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	optionsReq, err := http.NewRequest(http.MethodOptions, ts.URL+protectedResourceMetadataPath, nil)
	if err != nil {
		t.Fatalf("NewRequest OPTIONS: %v", err)
	}
	optionsResp, err := ts.Client().Do(optionsReq)
	if err != nil {
		t.Fatalf("OPTIONS metadata: %v", err)
	}
	defer optionsResp.Body.Close()
	if optionsResp.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204", optionsResp.StatusCode)
	}
	postReq, err := http.NewRequest(http.MethodPost, ts.URL+protectedResourceMetadataPath, nil)
	if err != nil {
		t.Fatalf("NewRequest POST: %v", err)
	}
	postResp, err := ts.Client().Do(postReq)
	if err != nil {
		t.Fatalf("POST metadata: %v", err)
	}
	defer postResp.Body.Close()
	if postResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", postResp.StatusCode)
	}
	protectedResp, err := ts.Client().Get(ts.URL + "/api/meta")
	if err != nil {
		t.Fatalf("Get API: %v", err)
	}
	defer protectedResp.Body.Close()
	if protectedResp.StatusCode != http.StatusForbidden {
		t.Fatalf("API status = %d, want 403", protectedResp.StatusCode)
	}
}

func TestMCPRoutes_requireDescopeBearerToken(t *testing.T) {
	// Given: Descope authorization is enabled without Cloudflare Access.
	descope := newFakeDescopeServer(t)
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config("https://pgpeek.example.com/mcp", "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	ts := newMCPAuthTestServer(t, authz)

	// When: a client calls MCP without a bearer token.
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	// Then: auth rejects it before MCP reads the body.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestMCPRoutes_allowAuthenticatedBrowserPreflight(t *testing.T) {
	descope := newFakeDescopeServer(t)
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config("https://pgpeek.example.com/mcp", "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	ts := newMCPAuthTestServer(t, authz)
	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/mcp", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Origin", "https://www.mcpjam.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,mcp-protocol-version")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "https://www.mcpjam.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("Access-Control-Allow-Headers = %q", resp.Header.Get("Access-Control-Allow-Headers"))
	}
}

func TestMCPRoutes_exposeOAuthChallengeToBrowserOrigin(t *testing.T) {
	descope := newFakeDescopeServer(t)
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config("https://pgpeek.example.com/mcp", "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	ts := newMCPAuthTestServer(t, authz)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Origin", "https://www.mcpjam.com")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "https://www.mcpjam.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(resp.Header.Get("WWW-Authenticate"), authz.resourceMetadataURL) {
		t.Fatalf("WWW-Authenticate = %q", resp.Header.Get("WWW-Authenticate"))
	}
}

func TestMCPRoutes_allowBrowserPreflightWithCloudflareAccess(t *testing.T) {
	descope := newFakeDescopeServer(t)
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config("https://pgpeek.example.com/mcp", "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	ts := newMCPAuthTestServer(t, authz, RequireCloudflareAccess(true))
	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/mcp", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Origin", "https://www.mcpjam.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestMCPRoutes_allowConfiguredResourceHostBehindLocalProxy(t *testing.T) {
	descope := newFakeDescopeServer(t)
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config("https://pgpeek.example.com/mcp", "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	ts := newMCPAuthTestServer(t, authz)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = "pgpeek.example.com"
	req.Header.Set("Authorization", "Bearer "+descope.token(t, "https://pgpeek.example.com/mcp", nil))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestMCPRoutes_rejectUnexpectedResourceHost(t *testing.T) {
	descope := newFakeDescopeServer(t)
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config("https://pgpeek.example.com/mcp", "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	ts := newMCPAuthTestServer(t, authz)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = "attacker.example"
	req.Header.Set("Authorization", "Bearer "+descope.token(t, "https://pgpeek.example.com/mcp", nil))

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}
