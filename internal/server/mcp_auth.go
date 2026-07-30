package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/descope-sample-apps/pgpeek/internal/mcpauth"
)

const (
	protectedResourceMetadataPath = "/.well-known/oauth-protected-resource"
	openIDConfigurationPathSuffix = "/.well-known/openid-configuration"
	mcpAuthHTTPTimeout            = 10 * time.Second
)

// DescopeMCPAuthConfig configures pgpeek as an OAuth resource server backed by
// a Descope MCP authorization server.
type DescopeMCPAuthConfig struct {
	WellKnownURL   string
	ResourceURL    string
	RequiredScopes []string
}

// MCPAuthorization is the resolved bearer-token verifier and public resource
// metadata for an authenticated MCP endpoint.
type MCPAuthorization struct {
	verifier            *oidc.IDTokenVerifier
	metadata            *oauthex.ProtectedResourceMetadata
	resourceMetadataURL string
	requiredScopes      []string
}

// NewDescopeMCPAuthorization resolves Descope's discovery document, requires
// DCR support, and prepares local JWT validation against Descope's JWK set.
func NewDescopeMCPAuthorization(ctx context.Context, cfg DescopeMCPAuthConfig) (*MCPAuthorization, error) {
	return newDescopeMCPAuthorization(ctx, cfg, newMCPAuthHTTPClient())
}

func newDescopeMCPAuthorization(ctx context.Context, cfg DescopeMCPAuthConfig, client *http.Client) (*MCPAuthorization, error) {
	issuer, err := issuerFromWellKnownURL(cfg.WellKnownURL)
	if err != nil {
		return nil, fmt.Errorf("descope well-known URL: %w", err)
	}
	resourceURL, err := mcpauth.ParseSecureURL(cfg.ResourceURL)
	if err != nil {
		return nil, fmt.Errorf("MCP resource URL: %w", err)
	}
	if resourceURL.Path != "/mcp" {
		return nil, errors.New("MCP resource URL path must be exactly /mcp")
	}
	requiredScopes, err := normalizeMCPAuthScopes(cfg.RequiredScopes)
	if err != nil {
		return nil, err
	}

	metadata, err := oauthex.GetAuthServerMeta(ctx, cfg.WellKnownURL, issuer, client)
	if err != nil {
		return nil, fmt.Errorf("load Descope discovery metadata: %w", err)
	}
	if metadata == nil {
		return nil, errors.New("descope discovery metadata not found")
	}
	if metadata.RegistrationEndpoint == "" {
		return nil, errors.New("descope discovery metadata does not advertise DCR; enable Dynamic Client Registration")
	}
	for _, endpoint := range []struct {
		name  string
		value string
	}{
		{name: "authorization_endpoint", value: metadata.AuthorizationEndpoint},
		{name: "token_endpoint", value: metadata.TokenEndpoint},
		{name: "registration_endpoint", value: metadata.RegistrationEndpoint},
		{name: "jwks_uri", value: metadata.JWKSURI},
	} {
		if _, err := mcpauth.ParseSecureURL(endpoint.value); err != nil {
			return nil, fmt.Errorf("descope %s: %w", endpoint.name, err)
		}
	}
	for _, scope := range requiredScopes {
		if !slices.Contains(metadata.ScopesSupported, scope) {
			return nil, fmt.Errorf("required MCP scope %q is not advertised by Descope", scope)
		}
	}
	providerContext := oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(providerContext, metadata.Issuer)
	if err != nil {
		return nil, fmt.Errorf("load Descope OpenID metadata: %w", err)
	}
	signingAlgorithms, err := mcpAuthSigningAlgorithms(provider)
	if err != nil {
		return nil, err
	}

	metadataURL := *resourceURL
	metadataURL.Path = protectedResourceMetadataPath
	metadataURL.RawPath = ""
	keySetContext := oidc.ClientContext(ctx, client)
	verifier := oidc.NewVerifier(metadata.Issuer, oidc.NewRemoteKeySet(keySetContext, metadata.JWKSURI), &oidc.Config{
		ClientID:             cfg.ResourceURL,
		SupportedSigningAlgs: signingAlgorithms,
	})
	return &MCPAuthorization{
		verifier: verifier,
		metadata: &oauthex.ProtectedResourceMetadata{
			Resource:               cfg.ResourceURL,
			AuthorizationServers:   []string{metadata.Issuer},
			ScopesSupported:        slices.Clone(requiredScopes),
			BearerMethodsSupported: []string{"header"},
			ResourceName:           "pgpeek PostgreSQL browser",
		},
		resourceMetadataURL: metadataURL.String(),
		requiredScopes:      slices.Clone(requiredScopes),
	}, nil
}

func mcpAuthSigningAlgorithms(provider *oidc.Provider) ([]string, error) {
	var claims struct {
		SigningAlgorithms []string `json:"id_token_signing_alg_values_supported"`
	}
	if err := provider.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parse Descope OpenID metadata: %w", err)
	}
	return supportedMCPAuthSigningAlgorithms(claims.SigningAlgorithms)
}

func newMCPAuthHTTPClient() *http.Client {
	return &http.Client{
		Transport: boundedResponseTransport{base: http.DefaultTransport, maxBytes: maxMCPAuthResponseBytes},
		Timeout:   mcpAuthHTTPTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many Descope discovery redirects")
			}
			_, err := mcpauth.ParseSecureURL(req.URL.String())
			return err
		},
	}
}

func issuerFromWellKnownURL(raw string) (string, error) {
	wellKnownURL, err := mcpauth.ParseSecureURL(raw)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(wellKnownURL.Path, openIDConfigurationPathSuffix) {
		return "", fmt.Errorf("path must end with %s", openIDConfigurationPathSuffix)
	}
	issuerURL := *wellKnownURL
	issuerURL.Path = strings.TrimSuffix(issuerURL.Path, openIDConfigurationPathSuffix)
	issuerURL.RawPath = ""
	return strings.TrimSuffix(issuerURL.String(), "/"), nil
}

func normalizeMCPAuthScopes(scopes []string) ([]string, error) {
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if !validMCPAuthScopeToken(scope) {
			return nil, fmt.Errorf("invalid required MCP scope %q", scope)
		}
		if !slices.Contains(normalized, scope) {
			normalized = append(normalized, scope)
		}
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one required MCP scope must be configured")
	}
	return normalized, nil
}

func validMCPAuthScopeToken(scope string) bool {
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

func supportedMCPAuthSigningAlgorithms(advertised []string) ([]string, error) {
	algorithms := make([]string, 0, len(advertised))
	for _, algorithm := range advertised {
		if isAsymmetricJWTSigningAlgorithm(algorithm) && !slices.Contains(algorithms, algorithm) {
			algorithms = append(algorithms, algorithm)
		}
	}
	if len(algorithms) == 0 {
		return nil, errors.New("descope discovery metadata does not advertise a supported asymmetric JWT signing algorithm")
	}
	return algorithms, nil
}

func isAsymmetricJWTSigningAlgorithm(algorithm string) bool {
	switch algorithm {
	case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA":
		return true
	default:
		return false
	}
}

func (a *MCPAuthorization) protect(next http.Handler) http.Handler {
	return auth.RequireBearerToken(a.verifyToken, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: a.resourceMetadataURL,
		Scopes:              slices.Clone(a.requiredScopes),
	})(next)
}

func (a *MCPAuthorization) metadataHandler() http.Handler {
	return auth.ProtectedResourceMetadataHandler(a.metadata)
}

func (a *MCPAuthorization) verifyToken(ctx context.Context, rawToken string, _ *http.Request) (*auth.TokenInfo, error) {
	token, err := a.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, auth.ErrInvalidToken
	}
	var claims struct {
		Scope string `json:"scope"`
	}
	if err := token.Claims(&claims); err != nil {
		return nil, auth.ErrInvalidToken
	}
	return &auth.TokenInfo{
		Scopes:     strings.Fields(claims.Scope),
		Expiration: token.Expiry,
		UserID:     token.Subject,
	}, nil
}
