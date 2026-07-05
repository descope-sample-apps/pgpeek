package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	cloudflareAccessEmailHeader = "Cf-Access-Authenticated-User-Email"
	cloudflareAccessJWTHeader   = "Cf-Access-Jwt-Assertion"
)

type currentUser struct {
	Provider string
	Email    string
}

func cloudflareAccessUser(r *http.Request) currentUser {
	email := strings.TrimSpace(r.Header.Get(cloudflareAccessEmailHeader))
	if email == "" && strings.TrimSpace(r.Header.Get(cloudflareAccessJWTHeader)) == "" {
		return currentUser{Provider: "anonymous"}
	}
	return currentUser{Provider: "cloudflare-access", Email: email}
}

func requireCloudflareAccess(require bool, next http.Handler) http.Handler {
	if !require {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isProbePath(r.URL.Path) || cloudflareAccessUser(r).Provider == "cloudflare-access" {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "cloudflare access required", http.StatusForbidden)
	})
}

func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if isProbePath(r.URL.Path) {
			return
		}
		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

func isProbePath(path string) bool {
	return path == "/healthz" || path == "/readyz"
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' https://cdnjs.cloudflare.com; " +
	"style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
