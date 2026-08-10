package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStatic_revalidatesAndServes304ForUnchangedBuild(t *testing.T) {
	// Given: a build of the embedded UI.
	web := fstest.MapFS{"app.js": &fstest.MapFile{Data: []byte("export const v = 1;")}}
	handler := staticHandler(web, "1.2.3")

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
	oldTag, nextTag := staticETag(old, "1.2.3"), staticETag(next, "1.2.3")

	// Then: the released build revalidates to a miss, so the client refetches.
	if oldTag == "" || oldTag == nextTag {
		t.Fatalf("etag %q did not change across builds", oldTag)
	}
}

// unreadableFS lists its files but refuses to open one of them.
type unreadableFS struct {
	files fs.FS
	fail  string
}

func (u unreadableFS) Open(name string) (fs.File, error) {
	if name == u.fail {
		return nil, fs.ErrPermission
	}
	return u.files.Open(name)
}

func TestStatic_unhashableAssetsDropTheETagRatherThanPinAStaleOne(t *testing.T) {
	// Given: an asset tree with one unreadable corner, so no honest tag covers
	// the build (a partial hash would pin clients to a build it never saw).
	unreadable := unreadableFS{
		files: fstest.MapFS{
			"app.js":           &fstest.MapFile{Data: []byte("export const v = 1;")},
			"vendor/preact.js": &fstest.MapFile{Data: []byte("export const p = 1;")},
		},
		fail: "vendor",
	}

	// When: a readable asset is served.
	rec := httptest.NewRecorder()
	staticHandler(unreadable, "1.2.3").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))

	// Then: it is still served and still revalidated, just without a tag —
	// no ETag means no conditional hit, never a stale hit.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("ETag = %q, want none", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestStatic_unhashableTreeHasNoETag(t *testing.T) {
	// Given: asset trees that cannot be listed, and that list a file they
	// cannot then read.
	assets := fstest.MapFS{"app.js": &fstest.MapFile{Data: []byte("export const v = 1;")}}
	for name, files := range map[string]fs.FS{
		"unlistable": unreadableFS{files: assets, fail: "."},
		"unreadable": unreadableFS{files: assets, fail: "app.js"},
	} {
		// When/Then: hashing gives up rather than tag a build it never read.
		if got := staticETag(files, "1.2.3"); got != "" {
			t.Errorf("%s: etag = %q, want none", name, got)
		}
	}
}

func TestStatic_etagCoversEveryAsset(t *testing.T) {
	// Given: a release that only changes a vendored module app.js imports.
	base := fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<h1>pgpeek</h1>")},
		"vendor/preact.js": &fstest.MapFile{Data: []byte("export const a = 1;")},
	}
	changed := fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<h1>pgpeek</h1>")},
		"vendor/preact.js": &fstest.MapFile{Data: []byte("export const a = 2;")},
	}

	// When/Then: the whole asset set revalidates, not just the file that moved.
	if staticETag(base, "1.2.3") == staticETag(changed, "1.2.3") {
		t.Fatal("etag ignored a changed module")
	}
}

func TestStatic_releaseWithIdenticalAssetsStillChangesETag(t *testing.T) {
	// Given: a release that rebuilds the same assets under a new version. The
	// served index.html carries the version, so a shared tag would 304 clients
	// onto a document that names the build they are leaving.
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(buildMetaPlaceholder)}}

	// When/Then: the version alone moves the tag.
	if staticETag(assets, "1.2.3") == staticETag(assets, "1.3.0") {
		t.Fatal("etag ignored the version")
	}
}

func TestStatic_indexCarriesTheRunningBuild(t *testing.T) {
	// Given: the UI, whose build meta is empty in the embedded bytes.
	web := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<head>" + buildMetaPlaceholder + "</head>")}}

	// When: the document is served.
	rec := httptest.NewRecorder()
	staticHandler(web, "1.2.3").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	// Then: the client can tell which build it is running without asking the
	// server, so a release during page load cannot become its baseline.
	if got := rec.Body.String(); !strings.Contains(got, `content="1.2.3"`) {
		t.Fatalf("index = %q, want the build stamped in", got)
	}
	if rec.Header().Get("ETag") == "" || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("headers = %v", rec.Header())
	}
}

func TestStatic_indexEscapesTheBuildIntoTheAttribute(t *testing.T) {
	// Given: a build string that would otherwise break out of the attribute.
	web := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(buildMetaPlaceholder)}}

	// When: the document is served.
	rec := httptest.NewRecorder()
	staticHandler(web, `1.0"><script>x()</script>`).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	// Then: it stays inside the attribute.
	if got := rec.Body.String(); strings.Contains(got, "<script>") {
		t.Fatalf("index = %q, want the build escaped", got)
	}
}

func TestStatic_shippedIndexHasSomethingToStamp(t *testing.T) {
	// Given: the index.html that actually ships. Stamping is a silent no-op if
	// the tag drifts from the constant, so pin them together.
	shipped, err := os.ReadFile(filepath.Join("..", "..", "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	// When/Then: the placeholder is still there to replace.
	if !bytes.Contains(shipped, []byte(buildMetaPlaceholder)) {
		t.Fatalf("web/index.html no longer contains %s", buildMetaPlaceholder)
	}
}

func TestStatic_servesAssetsWhenThereIsNoIndexToStamp(t *testing.T) {
	// Given: an asset tree with no index.html.
	web := fstest.MapFS{"app.js": &fstest.MapFile{Data: []byte("export const v = 1;")}}

	// When: an asset is requested.
	rec := httptest.NewRecorder()
	staticHandler(web, "1.2.3").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))

	// Then: serving carries on regardless.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
