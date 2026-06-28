package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/descope-sample-apps/pgpeek/internal/guard"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := mustGet(t, ts, "/healthz")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz = %d", resp.StatusCode)
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
