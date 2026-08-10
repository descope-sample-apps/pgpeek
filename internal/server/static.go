package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
)

// staticETag hashes every embedded asset into one build-wide tag. The UI ships
// inside the binary, so a release changes the tag for the whole asset set and
// browsers revalidate their way onto the new client instead of replaying a
// heuristically cached one.
func staticETag(files fs.FS) string {
	sum := sha256.New()
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, readErr := fs.ReadFile(files, p)
		if readErr != nil {
			return readErr
		}
		// hash.Hash writes never fail.
		_, _ = io.WriteString(sum, p+"\x00")
		_, _ = sum.Write(data)
		return nil
	})
	if err != nil {
		return ""
	}
	return `"` + hex.EncodeToString(sum.Sum(nil))[:16] + `"`
}

// staticHandler serves the embedded UI. Module imports carry no cache-busting
// query string, so every asset revalidates on load: with the ETag a warm client
// costs one 304, and a released client is picked up on the next reload.
func staticHandler(files fs.FS) http.Handler {
	etag := staticETag(files)
	assets := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		assets.ServeHTTP(w, r)
	})
}
