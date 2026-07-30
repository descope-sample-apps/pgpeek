package server

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

type fakeDescopeServer struct {
	t                     *testing.T
	server                *httptest.Server
	key                   *rsa.PrivateKey
	keyID                 string
	metadata              oauthex.AuthServerMeta
	signingAlgs           []string
	discoveryStatus       int
	secondDiscoveryStatus int
	discoveryRequests     int
	jwksStatus            int
	discoveryPadding      int
	jwksPadding           int
}

func newFakeDescopeServer(t *testing.T) *fakeDescopeServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fake := &fakeDescopeServer{t: t, key: key, keyID: "test-key", signingAlgs: []string{"RS256"}, discoveryStatus: http.StatusOK, jwksStatus: http.StatusOK}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	t.Cleanup(fake.server.Close)
	fake.metadata = oauthex.AuthServerMeta{
		Issuer:                        fake.server.URL + "/issuer",
		AuthorizationEndpoint:         fake.server.URL + "/authorize",
		TokenEndpoint:                 fake.server.URL + "/token",
		JWKSURI:                       fake.server.URL + "/jwks",
		RegistrationEndpoint:          fake.server.URL + "/register",
		ScopesSupported:               []string{"openid", "mcp:pgpeek.read", "mcp:pgpeek.schema"},
		ResponseTypesSupported:        []string{"code"},
		CodeChallengeMethodsSupported: []string{"S256"},
	}
	return fake
}

func (f *fakeDescopeServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/issuer/.well-known/openid-configuration":
		f.discoveryRequests++
		status := f.discoveryStatus
		if f.discoveryRequests > 1 && f.secondDiscoveryStatus != 0 {
			status = f.secondDiscoveryStatus
		}
		if status == http.StatusOK {
			value := struct {
				oauthex.AuthServerMeta
				IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
				Padding                          string   `json:"padding,omitempty"`
			}{AuthServerMeta: f.metadata, IDTokenSigningAlgValuesSupported: f.signingAlgs, Padding: strings.Repeat("x", f.discoveryPadding)}
			f.writeJSON(w, value)
		} else {
			w.WriteHeader(status)
		}
	case "/jwks":
		if f.jwksStatus == http.StatusOK {
			f.writeJSON(w, map[string]any{"keys": []jose.JSONWebKey{{
				Key:       &f.key.PublicKey,
				KeyID:     f.keyID,
				Algorithm: string(jose.RS256),
				Use:       "sig",
			}}, "padding": strings.Repeat("x", f.jwksPadding)})
		} else {
			w.WriteHeader(f.jwksStatus)
		}
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeDescopeServer) writeJSON(w http.ResponseWriter, value any) {
	f.t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		f.t.Errorf("Encode: %v", err)
	}
}

func (f *fakeDescopeServer) config(resourceURL string, requiredScopes ...string) DescopeMCPAuthConfig {
	return DescopeMCPAuthConfig{
		WellKnownURL:   f.metadata.Issuer + "/.well-known/openid-configuration",
		ResourceURL:    resourceURL,
		RequiredScopes: requiredScopes,
	}
}

func (f *fakeDescopeServer) token(t *testing.T, resourceURL string, overrides map[string]any) string {
	t.Helper()
	now := time.Now()
	claims := map[string]any{
		"iss":   f.metadata.Issuer,
		"sub":   "user-123",
		"aud":   []string{resourceURL},
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"scope": "openid mcp:pgpeek.read",
	}
	for key, value := range overrides {
		claims[key] = value
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal claims: %v", err)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: f.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", f.keyID),
	)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	token, err := signed.CompactSerialize()
	if err != nil {
		t.Fatalf("CompactSerialize: %v", err)
	}
	return token
}

func newMCPAuthTestServer(t *testing.T, authz *MCPAuthorization, opts ...Option) *httptest.Server {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	web := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("x")}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": &fakeQuerier{}},
	}
	serverOptions := append([]Option{WithMCPAuthorization(authz)}, opts...)
	srv := NewWithRegistry(registry, st, web, log, time.Second, serverOptions...)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts
}

func bearerRequest(t *testing.T, method, target, token string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader("{}"))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}
