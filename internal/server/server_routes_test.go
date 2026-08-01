package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/guard"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

func TestHealthz(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	web := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("pgpeek")}}
	srv := New(&fakeQuerier{}, st, web, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Second,
		BuildInfo("1.2.3", "abc1234", "2026-08-01T00:00:00Z"))
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	resp := mustGet(t, ts, "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz = %d", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	if got["status"] != "ok" || got["version"] != "1.2.3" || got["commit"] != "abc1234" || got["buildDate"] != "2026-08-01T00:00:00Z" {
		t.Fatalf("healthz = %#v", got)
	}
}

func TestCurrentUser(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/user", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(cloudflareAccessEmailHeader, "alice@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["provider"] != "cloudflare-access" || got["email"] != "alice@example.com" {
		t.Fatalf("user = %#v", got)
	}
}

func TestCloudflareAccessRequired(t *testing.T) {
	ts, _ := newCloudflareRequiredTestServer(t, &fakeQuerier{})
	resp := mustGet(t, ts, "/api/databases")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without access = %d, want 403", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/databases", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(cloudflareAccessJWTHeader, "token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("with access = %d, want 200", resp.StatusCode)
	}

	resp = mustGet(t, ts, "/healthz")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", resp.StatusCode)
	}
}

func TestReadyz(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		ts, _ := newTestServer(t, &fakeQuerier{})
		resp := mustGet(t, ts, "/readyz")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("readyz = %d, want 200", resp.StatusCode)
		}
	})
	t.Run("db down", func(t *testing.T) {
		ts, _ := newTestServer(t, &fakeQuerier{pingErr: errors.New("down")})
		resp := mustGet(t, ts, "/readyz")
		resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("readyz = %d, want 503", resp.StatusCode)
		}
	})
}

func TestSecurityHeaders(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := mustGet(t, ts, "/healthz")
	resp.Body.Close()
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := resp.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("missing CSP: %q", csp)
	}
}

func TestUIServed(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := mustGet(t, ts, "/")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("pgpeek")) {
		t.Errorf("index not served: %q", body)
	}
}

// DefaultPresets must all survive the read-only guard, otherwise the saved-query
// validation would reject them if a user re-saved one.
func TestDefaultPresetsPassGuard(t *testing.T) {
	for _, p := range store.DefaultPresets {
		if err := guard.Validate(p.SQL); err != nil {
			t.Errorf("preset %q fails guard: %v", p.Name, err)
		}
	}
}
