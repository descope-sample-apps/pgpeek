package mcpauth

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ParseSecureURL parses an MCP authorization URL using the shared startup and
// runtime security policy.
func ParseSecureURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errors.New("must be an absolute URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return nil, errors.New("must not contain user information, a query, or a fragment")
	}
	if strings.EqualFold(u.Scheme, "https") {
		return u, nil
	}
	hostIP := net.ParseIP(u.Hostname())
	loopback := strings.EqualFold(u.Hostname(), "localhost") || hostIP != nil && hostIP.IsLoopback()
	if !strings.EqualFold(u.Scheme, "http") || !loopback {
		return nil, errors.New("must use HTTPS except on loopback hosts")
	}
	return u, nil
}
