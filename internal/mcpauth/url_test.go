package mcpauth

import "testing"

func TestParseSecureURL_enforcesSharedMCPPolicy(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "HTTPS", raw: "https://pgpeek.example.com/mcp"},
		{name: "loopback HTTP", raw: "http://127.0.0.1:9000/mcp"},
		{name: "localhost HTTP", raw: "http://localhost:9000/mcp"},
		{name: "relative URL", raw: "/mcp", wantErr: true},
		{name: "non-loopback HTTP", raw: "http://pgpeek.example.com/mcp", wantErr: true},
		{name: "user information", raw: "https://user@pgpeek.example.com/mcp", wantErr: true},
		{name: "query", raw: "https://pgpeek.example.com/mcp?tenant=one", wantErr: true},
		{name: "empty query delimiter", raw: "https://pgpeek.example.com/mcp?", wantErr: true},
		{name: "fragment", raw: "https://pgpeek.example.com/mcp#fragment", wantErr: true},
		{name: "empty fragment delimiter", raw: "https://pgpeek.example.com/mcp#", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: an MCP URL crosses the shared trust boundary.
			got, err := ParseSecureURL(tt.raw)

			// Then: only absolute HTTPS or loopback HTTP URLs without extra components pass.
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected URL validation error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSecureURL: %v", err)
			}
			if got.String() != tt.raw {
				t.Fatalf("URL = %q, want %q", got, tt.raw)
			}
		})
	}
}
