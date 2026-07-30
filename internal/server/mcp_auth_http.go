package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxMCPAuthResponseBytes = 1 << 20

var errMCPAuthResponseTooLarge = errors.New("descope response exceeds size limit")

type boundedResponseTransport struct {
	base     http.RoundTripper
	maxBytes int64
}

func (t boundedResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.ContentLength > t.maxBytes {
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("close oversized Descope response: %w", err)
		}
		return nil, errMCPAuthResponseTooLarge
	}
	resp.Body = &boundedReadCloser{ReadCloser: resp.Body, remaining: t.maxBytes}
	return resp, nil
}

type boundedReadCloser struct {
	io.ReadCloser
	remaining int64
}

func (r *boundedReadCloser) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, errMCPAuthResponseTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.ReadCloser.Read(p)
	r.remaining -= int64(n)
	return n, err
}
