package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io"
	"io/fs"
	"net/http"
	"time"
)

// buildMetaPlaceholder is stamped with the running build as index.html is
// served. The client needs the version of the assets it actually loaded: a
// release landing between the page load and the first /healthz would otherwise
// become the tab's baseline while it still runs the old client.
const buildMetaPlaceholder = `<meta name="pgpeek-build" content="" />`

// staticETag hashes the running build into one tag. The UI ships inside the
// binary, so a release changes the tag for the whole asset set and browsers
// revalidate their way onto the new client instead of replaying a heuristically
// cached one. The version is folded in because it is stamped into the served
// index.html without changing the embedded bytes.
func staticETag(files fs.FS, version string) string {
	sum := sha256.New()
	_, _ = io.WriteString(sum, version+"\x00")
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

// indexWithBuild returns index.html with the build stamped into its meta tag,
// or nil if there is nothing to stamp.
func indexWithBuild(files fs.FS, version string) []byte {
	data, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil
	}
	stamped := `<meta name="pgpeek-build" content="` + html.EscapeString(version) + `" />`
	return bytes.Replace(data, []byte(buildMetaPlaceholder), []byte(stamped), 1)
}

// staticHandler serves the embedded UI. Module imports carry no cache-busting
// query string, so every asset revalidates on load: with the ETag a warm client
// costs one 304, and a released client is picked up on the next reload.
func staticHandler(files fs.FS, version string) http.Handler {
	etag := staticETag(files, version)
	index := indexWithBuild(files, version)
	assets := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		if index != nil && r.URL.Path == "/" {
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
			return
		}
		assets.ServeHTTP(w, r)
	})
}
