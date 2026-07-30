package config

import (
	"slices"
	"testing"
)

func TestLoad_MCPAuth(t *testing.T) {
	// Given: Descope DCR discovery, the public MCP resource, and required scopes.
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://h/db")
	t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/v1/apps/agentic/project/server/.well-known/openid-configuration")
	t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp")
	t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "mcp:pgpeek.read, mcp:pgpeek.schema mcp:pgpeek.read")

	// When: application configuration is loaded.
	c, err := Load()
	// Then: MCP auth is enabled and scopes are normalized without duplicates.
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.MCPAuth.Enabled() {
		t.Fatal("MCPAuth should be enabled")
	}
	if c.MCPAuth.WellKnownURL != "https://api.descope.com/v1/apps/agentic/project/server/.well-known/openid-configuration" {
		t.Errorf("WellKnownURL = %q", c.MCPAuth.WellKnownURL)
	}
	if c.MCPAuth.ResourceURL != "https://pgpeek.example.com/mcp" {
		t.Errorf("ResourceURL = %q", c.MCPAuth.ResourceURL)
	}
	if want := []string{"mcp:pgpeek.read", "mcp:pgpeek.schema"}; !slices.Equal(c.MCPAuth.RequiredScopes, want) {
		t.Errorf("RequiredScopes = %q, want %q", c.MCPAuth.RequiredScopes, want)
	}
}

func TestLoad_MCPAuth_acceptsDescopeConfigURLAliasAndLoopback(t *testing.T) {
	// Given: the environment name used by Descope's B2B MCP example.
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://h/db")
	t.Setenv("DESCOPE_CONFIG_URL", "http://127.0.0.1:9000/issuer/.well-known/openid-configuration")
	t.Setenv("PGPEEK_MCP_SERVER_URL", "http://localhost:8080/mcp")
	t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")

	// When: application configuration is loaded.
	c, err := Load()
	// Then: the alias enables auth and loopback HTTP remains available for development.
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.MCPAuth.WellKnownURL != "http://127.0.0.1:9000/issuer/.well-known/openid-configuration" {
		t.Errorf("WellKnownURL = %q", c.MCPAuth.WellKnownURL)
	}
}

func TestLoad_MCPAuth_rejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "missing resource URL",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "missing well-known URL",
			setup: func(t *testing.T) {
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "missing scopes",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp")
			},
		},
		{
			name: "conflicting discovery aliases",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/one/.well-known/openid-configuration")
				t.Setenv("DESCOPE_CONFIG_URL", "https://api.descope.com/two/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "insecure discovery URL",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "http://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "wrong discovery path",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/config")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "insecure resource URL",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "http://pgpeek.example.com/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "resource is not MCP endpoint",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "resource has unsupported path prefix",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/tenant/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "resource URL contains query",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp?tenant=one")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "malformed discovery URL",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "://bad")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "resource URL contains user information",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://user@pgpeek.example.com/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "openid")
			},
		},
		{
			name: "invalid scope token",
			setup: func(t *testing.T) {
				t.Setenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL", "https://api.descope.com/issuer/.well-known/openid-configuration")
				t.Setenv("PGPEEK_MCP_SERVER_URL", "https://pgpeek.example.com/mcp")
				t.Setenv("PGPEEK_MCP_REQUIRED_SCOPES", "mcp:read\"all")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a partial or insecure MCP auth configuration.
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://h/db")
			tt.setup(t)

			// When: application configuration is loaded.
			_, err := Load()

			// Then: startup fails instead of silently exposing an anonymous MCP endpoint.
			if err == nil {
				t.Fatal("expected MCP auth configuration error")
			}
		})
	}
}

func TestParseMCPScopes_rejectsEmptyAndNonASCIITokens(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: " , \t"},
		{name: "non-ASCII", value: "mcp:rëad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a value that cannot be represented as an RFC 6749 scope-token.
			// When: it is parsed at the environment boundary.
			_, err := parseMCPScopes(tt.value)

			// Then: configuration rejects it.
			if err == nil {
				t.Fatal("expected scope parsing error")
			}
		})
	}
	if validOAuthScopeToken("") {
		t.Fatal("empty scope token should be invalid")
	}
}
