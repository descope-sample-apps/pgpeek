package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStatic_revalidatesAndServes304ForUnchangedBuild(t *testing.T) {
	// Given: a build of the embedded UI.
	web := fstest.MapFS{"app.js": &fstest.MapFile{Data: []byte("export const v = 1;")}}
	handler := staticHandler(web)

	// When: a browser loads the UI.
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/app.js", nil))

	// Then: it is told to revalidate and given a tag to revalidate with.
	if got := first.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}

	// When: the same build is revalidated.
	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, req)

	// Then: the cached copy is confirmed rather than resent.
	if second.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", second.Code)
	}
}

func TestStatic_newBuildChangesETag(t *testing.T) {
	// Given: two builds whose assets differ.
	old := fstest.MapFS{"app.js": &fstest.MapFile{Data: []byte("export const v = 1;")}}
	next := fstest.MapFS{"app.js": &fstest.MapFile{Data: []byte("export const v = 2;")}}

	// When: each is hashed.
	oldTag, nextTag := staticETag(old), staticETag(next)

	// Then: the released build revalidates to a miss, so the client refetches.
	if oldTag == "" || oldTag == nextTag {
		t.Fatalf("etag %q did not change across builds", oldTag)
	}
}

func TestStatic_etagCoversEveryAsset(t *testing.T) {
	// Given: a release that only changes a module app.js imports.
	base := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<h1>pgpeek</h1>")},
		"sidebar.js": &fstest.MapFile{Data: []byte("export const a = 1;")},
	}
	changed := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<h1>pgpeek</h1>")},
		"sidebar.js": &fstest.MapFile{Data: []byte("export const a = 2;")},
	}

	// When/Then: the whole asset set revalidates, not just the file that moved.
	if staticETag(base) == staticETag(changed) {
		t.Fatal("etag ignored a changed module")
	}
}
