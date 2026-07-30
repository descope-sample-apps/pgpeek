package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/descope-sample-apps/pgpeek/internal/mcpauth"
)

const openIDConfigurationSuffix = "/.well-known/openid-configuration"

// MCPAuth holds optional Descope authorization settings for the MCP endpoint.
type MCPAuth struct {
	WellKnownURL   string
	ResourceURL    string
	RequiredScopes []string
}

// Enabled reports whether Descope authorization is configured for MCP.
func (c MCPAuth) Enabled() bool {
	return c.WellKnownURL != ""
}

func loadMCPAuth() (MCPAuth, error) {
	canonicalURL := strings.TrimSpace(os.Getenv("DESCOPE_MCP_SERVER_WELL_KNOWN_URL"))
	aliasURL := strings.TrimSpace(os.Getenv("DESCOPE_CONFIG_URL"))
	if canonicalURL != "" && aliasURL != "" && canonicalURL != aliasURL {
		return MCPAuth{}, errors.New("DESCOPE_MCP_SERVER_WELL_KNOWN_URL and DESCOPE_CONFIG_URL disagree")
	}

	wellKnownURL := canonicalURL
	if wellKnownURL == "" {
		wellKnownURL = aliasURL
	}
	resourceURL := strings.TrimSpace(os.Getenv("PGPEEK_MCP_SERVER_URL"))
	scopesValue := strings.TrimSpace(os.Getenv("PGPEEK_MCP_REQUIRED_SCOPES"))
	if wellKnownURL == "" && resourceURL == "" && scopesValue == "" {
		return MCPAuth{}, nil
	}
	if wellKnownURL == "" || resourceURL == "" || scopesValue == "" {
		return MCPAuth{}, errors.New("DESCOPE_MCP_SERVER_WELL_KNOWN_URL (or DESCOPE_CONFIG_URL), PGPEEK_MCP_SERVER_URL, and PGPEEK_MCP_REQUIRED_SCOPES must be set together")
	}

	discoveryURL, err := mcpauth.ParseSecureURL(wellKnownURL)
	if err != nil {
		return MCPAuth{}, fmt.Errorf("DESCOPE_MCP_SERVER_WELL_KNOWN_URL: %w", err)
	}
	if !strings.HasSuffix(discoveryURL.Path, openIDConfigurationSuffix) {
		return MCPAuth{}, fmt.Errorf("DESCOPE_MCP_SERVER_WELL_KNOWN_URL path must end with %s", openIDConfigurationSuffix)
	}

	resource, err := mcpauth.ParseSecureURL(resourceURL)
	if err != nil {
		return MCPAuth{}, fmt.Errorf("PGPEEK_MCP_SERVER_URL: %w", err)
	}
	if resource.Path != "/mcp" {
		return MCPAuth{}, errors.New("PGPEEK_MCP_SERVER_URL path must be exactly /mcp")
	}

	scopes, err := parseMCPScopes(scopesValue)
	if err != nil {
		return MCPAuth{}, fmt.Errorf("PGPEEK_MCP_REQUIRED_SCOPES: %w", err)
	}
	return MCPAuth{WellKnownURL: wellKnownURL, ResourceURL: resourceURL, RequiredScopes: scopes}, nil
}

func parseMCPScopes(value string) ([]string, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	scopes := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, scope := range parts {
		if !validOAuthScopeToken(scope) {
			return nil, fmt.Errorf("invalid OAuth scope %q", scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	if len(scopes) == 0 {
		return nil, errors.New("must contain at least one OAuth scope")
	}
	return scopes, nil
}

func validOAuthScopeToken(scope string) bool {
	if scope == "" {
		return false
	}
	for i := range len(scope) {
		b := scope[i]
		if b == 0x21 || b >= 0x23 && b <= 0x5b || b >= 0x5d && b <= 0x7e {
			continue
		}
		return false
	}
	return true
}
