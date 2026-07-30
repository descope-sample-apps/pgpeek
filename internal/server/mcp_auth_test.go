package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/descope-sample-apps/pgpeek/internal/mcpauth"
)

func TestNewDescopeMCPAuthorization_discoversDCR(t *testing.T) {
	// Given: Descope publishes OAuth metadata with DCR and the configured scope.
	descope := newFakeDescopeServer(t)
	resourceURL := "https://pgpeek.example.com/mcp"

	// When: pgpeek initializes optional MCP authorization.
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config(resourceURL, "mcp:pgpeek.read"))
	// Then: it uses Descope as the authorization server and advertises the MCP resource.
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	if authz.resourceMetadataURL != "https://pgpeek.example.com/.well-known/oauth-protected-resource" {
		t.Errorf("resourceMetadataURL = %q", authz.resourceMetadataURL)
	}
	if authz.metadata.Resource != resourceURL || len(authz.metadata.AuthorizationServers) != 1 || authz.metadata.AuthorizationServers[0] != descope.metadata.Issuer {
		t.Fatalf("metadata = %+v", authz.metadata)
	}
	if len(authz.metadata.ScopesSupported) != 1 || authz.metadata.ScopesSupported[0] != "mcp:pgpeek.read" {
		t.Errorf("ScopesSupported = %q", authz.metadata.ScopesSupported)
	}
}

func TestNewDescopeMCPAuthorization_rejectsInvalidDiscovery(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeDescopeServer, *DescopeMCPAuthConfig)
	}{
		{"discovery failure", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.discoveryStatus = http.StatusInternalServerError
		}},
		{"discovery not found", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) { d.discoveryStatus = http.StatusNotFound }},
		{"issuer mismatch", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) { d.metadata.Issuer = d.server.URL + "/other" }},
		{"authorization endpoint missing", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.metadata.AuthorizationEndpoint = ""
		}},
		{"authorization endpoint not absolute", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.metadata.AuthorizationEndpoint = "https:authorize"
		}},
		{"token endpoint missing", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) { d.metadata.TokenEndpoint = "" }},
		{"token endpoint not absolute", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.metadata.TokenEndpoint = "https:token"
		}},
		{"DCR disabled", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) { d.metadata.RegistrationEndpoint = "" }},
		{"DCR endpoint not absolute", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.metadata.RegistrationEndpoint = "https:register"
		}},
		{"insecure DCR endpoint", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.metadata.RegistrationEndpoint = "http://register.example.com"
		}},
		{"scope missing", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) { d.metadata.ScopesSupported = []string{"openid"} }},
		{"insecure JWKS", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.metadata.JWKSURI = "http://keys.example.com/jwks"
		}},
		{"missing signing algorithms", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) { d.signingAlgs = nil }},
		{"unsupported signing algorithm", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) { d.signingAlgs = []string{"HS256"} }},
		{"OIDC discovery failure", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.secondDiscoveryStatus = http.StatusInternalServerError
		}},
		{"oversized discovery", func(d *fakeDescopeServer, _ *DescopeMCPAuthConfig) {
			d.discoveryPadding = maxMCPAuthResponseBytes
		}},
		{"invalid well-known path", func(_ *fakeDescopeServer, c *DescopeMCPAuthConfig) { c.WellKnownURL = "https://api.descope.com/config" }},
		{"invalid resource URL", func(_ *fakeDescopeServer, c *DescopeMCPAuthConfig) { c.ResourceURL = "://bad" }},
		{"resource path prefix", func(_ *fakeDescopeServer, c *DescopeMCPAuthConfig) {
			c.ResourceURL = "https://pgpeek.example.com/tenant/mcp"
		}},
		{"empty scopes", func(_ *fakeDescopeServer, c *DescopeMCPAuthConfig) { c.RequiredScopes = nil }},
		{"invalid scope", func(_ *fakeDescopeServer, c *DescopeMCPAuthConfig) {
			c.RequiredScopes = []string{"mcp:read all"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a malformed or incomplete Descope discovery contract.
			descope := newFakeDescopeServer(t)
			cfg := descope.config("https://pgpeek.example.com/mcp", "mcp:pgpeek.read")
			tt.mutate(descope, &cfg)

			// When: pgpeek initializes MCP authorization.
			_, err := NewDescopeMCPAuthorization(context.Background(), cfg)

			// Then: startup fails closed.
			if err == nil {
				t.Fatal("expected authorization configuration error")
			}
		})
	}
}

func TestMCPAuthorization_rejectsOversizedJWKS(t *testing.T) {
	// Given: discovery is valid but the advertised key set exceeds the auth-response limit.
	descope := newFakeDescopeServer(t)
	authz, err := NewDescopeMCPAuthorization(context.Background(), descope.config("https://pgpeek.example.com/mcp", "mcp:pgpeek.read"))
	if err != nil {
		t.Fatalf("NewDescopeMCPAuthorization: %v", err)
	}
	descope.jwksPadding = maxMCPAuthResponseBytes
	recorder := httptest.NewRecorder()

	// When: token verification fetches the oversized key set.
	authz.protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("oversized JWKS must not reach the protected handler")
	})).ServeHTTP(recorder, bearerRequest(t, http.MethodPost, "https://pgpeek.example.com/mcp", descope.token(t, "https://pgpeek.example.com/mcp", nil)))

	// Then: authentication fails closed.
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestMCPAuthHTTPClient_redirectPolicy(t *testing.T) {
	client := newMCPAuthHTTPClient()
	if client.Timeout != mcpAuthHTTPTimeout {
		t.Fatalf("Timeout = %v, want %v", client.Timeout, mcpAuthHTTPTimeout)
	}

	tests := []struct {
		name    string
		target  string
		via     int
		wantErr bool
	}{
		{name: "secure", target: "https://api.descope.com/config"},
		{name: "loopback development", target: "http://127.0.0.1:9000/config"},
		{name: "HTTP downgrade", target: "http://api.descope.com/config", wantErr: true},
		{name: "redirect loop", target: "https://api.descope.com/config", via: 3, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			via := make([]*http.Request, tt.via)
			err := client.CheckRedirect(req, via)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeMCPAuthScopes_deduplicatesAndRejectsInvalidValues(t *testing.T) {
	got, err := normalizeMCPAuthScopes([]string{"mcp:read", "mcp:read", "mcp:schema"})
	if err != nil {
		t.Fatalf("normalizeMCPAuthScopes: %v", err)
	}
	if strings.Join(got, ",") != "mcp:read,mcp:schema" {
		t.Fatalf("scopes = %q", got)
	}
	for _, scopes := range [][]string{nil, {""}, {"mcp:read all"}, {"mcp:rëad"}} {
		if _, err := normalizeMCPAuthScopes(scopes); err == nil {
			t.Fatalf("scopes %q should fail", scopes)
		}
	}
}

func TestMCPAuthURLParsers_rejectUnsafeValues(t *testing.T) {
	tests := []struct {
		name  string
		parse func(string) error
		value string
	}{
		{
			name: "malformed well-known URL",
			parse: func(value string) error {
				_, err := issuerFromWellKnownURL(value)
				return err
			},
			value: "://bad",
		},
		{
			name: "URL user information",
			parse: func(value string) error {
				_, err := mcpauth.ParseSecureURL(value)
				return err
			},
			value: "https://user@example.com/mcp",
		},
		{
			name: "URL query",
			parse: func(value string) error {
				_, err := mcpauth.ParseSecureURL(value)
				return err
			},
			value: "https://example.com/mcp?tenant=one",
		},
		{
			name: "URL fragment",
			parse: func(value string) error {
				_, err := mcpauth.ParseSecureURL(value)
				return err
			},
			value: "https://example.com/mcp#fragment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.parse(tt.value); err == nil {
				t.Fatal("expected URL validation error")
			}
		})
	}
}

func TestMCPAuthSigningAlgorithms_rejectsProviderWithoutClaims(t *testing.T) {
	provider := (&oidc.ProviderConfig{IssuerURL: "https://issuer.example.com"}).NewProvider(context.Background())

	if _, err := mcpAuthSigningAlgorithms(provider); err == nil {
		t.Fatal("expected provider claims error")
	}
}

func TestNormalizeResourceAuthority(t *testing.T) {
	tests := []struct {
		scheme    string
		authority string
		want      string
	}{
		{"https", "PGPEEK.EXAMPLE.COM", "pgpeek.example.com"},
		{"https", "pgpeek.example.com:443", "pgpeek.example.com"},
		{"http", "pgpeek.example.com:80", "pgpeek.example.com"},
		{"https", "pgpeek.example.com:8443", "pgpeek.example.com:8443"},
		{"https", "[2001:db8::1]:443", "[2001:db8::1]"},
	}
	for _, tt := range tests {
		t.Run(tt.scheme+"_"+tt.authority, func(t *testing.T) {
			if got := normalizeResourceAuthority(tt.scheme, tt.authority); got != tt.want {
				t.Fatalf("normalizeResourceAuthority(%q, %q) = %q, want %q", tt.scheme, tt.authority, got, tt.want)
			}
		})
	}
}
