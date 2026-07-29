package server

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBoundedResponseTransport_ReportsUpstreamError(t *testing.T) {
	transport := boundedResponseTransport{base: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream")
	}), maxBytes: 1}
	if _, err := transport.RoundTrip(&http.Request{}); err == nil {
		t.Fatal("expected upstream error")
	}
}

func TestBoundedResponseTransport_RejectsDeclaredOversize(t *testing.T) {
	tests := []struct {
		name string
		body io.ReadCloser
	}{
		{"closed", io.NopCloser(strings.NewReader("xx"))},
		{"close error", closeErrorReader{Reader: strings.NewReader("xx")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := boundedResponseTransport{base: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{Body: tt.body, ContentLength: 2}, nil
			}), maxBytes: 1}
			if _, err := transport.RoundTrip(&http.Request{}); err == nil {
				t.Fatal("expected size error")
			}
		})
	}
}

func TestBoundedReadCloser_AcceptsExactLimit(t *testing.T) {
	body := &boundedReadCloser{ReadCloser: io.NopCloser(strings.NewReader("x")), remaining: 1}
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "x" {
		t.Fatalf("body = %q", got)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type closeErrorReader struct{ io.Reader }

func (closeErrorReader) Close() error { return errors.New("close") }
