package main

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chrischdi/gophotobooth/internal/galleryurl"
)

// testMux builds the gallery HTTP mux wired to the given directories.
// galleryPath is the obscure path prefix (e.g. "/g/abcdef0123456789/").
func testMux(photoDir, cacheDir, galleryPath string, tpl *template.Template) *http.ServeMux {
	mux := http.NewServeMux()

	prefix := strings.TrimSuffix(strings.TrimPrefix(galleryPath, "/g/"), "/")

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
	})

	mux.Handle(galleryPath+"photo/", http.StripPrefix(galleryPath+"photo/", http.FileServer(http.Dir(photoDir))))
	mux.HandleFunc(galleryPath+"thumb/", serveResized(photoDir, cacheDir, 400))
	mux.HandleFunc(galleryPath+"preview/", serveResized(photoDir, cacheDir, 1280))

	mux.HandleFunc(galleryPath, func(w http.ResponseWriter, r *http.Request) {
		photos, err := listPhotos(photoDir)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tpl.Execute(w, galleryData{Photos: photos, Prefix: prefix, Count: len(photos)})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	return mux
}

func parseTpl(t *testing.T) *template.Template {
	t.Helper()
	tpl, err := template.New("gallery").Parse(galleryTpl)
	require.NoError(t, err)
	return tpl
}

// --- listPhotos tests ---

func TestListPhotos_Empty(t *testing.T) {
	dir := t.TempDir()
	photos, err := listPhotos(dir)
	require.NoError(t, err)
	assert.Empty(t, photos)
}

func TestListPhotos_SortedNewestFirst(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2026-01-01.jpg", "2026-06-01.jpg", "2026-03-01.jpg"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644))
	}
	// Non-image files and dirs should be excluded
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))

	photos, err := listPhotos(dir)
	require.NoError(t, err)
	require.Len(t, photos, 3)
	assert.Equal(t, "2026-06-01.jpg", photos[0].Name, "newest first")
	assert.Equal(t, "2026-03-01.jpg", photos[1].Name)
	assert.Equal(t, "2026-01-01.jpg", photos[2].Name)
}

func TestListPhotos_Extensions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpeg", "c.png", "d.gif", "e.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644))
	}
	photos, err := listPhotos(dir)
	require.NoError(t, err)
	names := make([]string, len(photos))
	for i, p := range photos {
		names[i] = p.Name
	}
	assert.ElementsMatch(t, []string{"a.jpg", "b.jpeg", "c.png"}, names)
}

// --- robots.txt ---

func TestRobotsTxt(t *testing.T) {
	mux := testMux(t.TempDir(), t.TempDir(), "/g/test/", parseTpl(t))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/robots.txt", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "User-agent: *")
	assert.Contains(t, body, "Disallow: /")
}

// --- gallery page ---

func TestGalleryPage_RendersPhotos(t *testing.T) {
	photoDir := t.TempDir()
	for _, name := range []string{"2026-01-01.jpg", "2026-03-01.jpg"} {
		require.NoError(t, os.WriteFile(filepath.Join(photoDir, name), []byte("x"), 0644))
	}
	oldest := "2026-01-01.jpg"
	prefix := galleryurl.HashPrefix(oldest)
	galleryPath := "/g/" + prefix + "/"

	mux := testMux(photoDir, t.TempDir(), galleryPath, parseTpl(t))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", galleryPath, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	body := rec.Body.String()
	assert.Contains(t, body, "2026-01-01.jpg")
	assert.Contains(t, body, "2026-03-01.jpg")
}

func TestGalleryPage_WrongHash_NotFound(t *testing.T) {
	mux := testMux(t.TempDir(), t.TempDir(), "/g/correcthash123456/", parseTpl(t))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/g/wronghash/", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// --- photo endpoint ---

func TestPhotoHandler_ServesFile(t *testing.T) {
	photoDir := t.TempDir()
	content := []byte("fake jpeg content")
	require.NoError(t, os.WriteFile(filepath.Join(photoDir, "photo.jpg"), content, 0644))

	mux := testMux(photoDir, t.TempDir(), "/g/x/", parseTpl(t))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/g/x/photo/photo.jpg", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, content, body)
}

// --- serveResized ---

func skipIfNoVips(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("vipsthumbnail"); err != nil {
		t.Skip("vipsthumbnail not available")
	}
}

func TestServeResized_BadExtension(t *testing.T) {
	mux := testMux(t.TempDir(), t.TempDir(), "/g/x/", parseTpl(t))

	for _, path := range []string{"/g/x/thumb/evil.sh", "/g/x/thumb/evil.exe", "/g/x/preview/script.py"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		assert.Equal(t, http.StatusNotFound, rec.Code, "path %s should return 404", path)
	}
}

func TestServeResized_MissingSource(t *testing.T) {
	skipIfNoVips(t)
	photoDir := t.TempDir()
	cacheDir := t.TempDir()
	mux := testMux(photoDir, cacheDir, "/g/x/", parseTpl(t))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/g/x/thumb/nonexistent.jpg", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- waitForFirstPhoto ---

func TestWaitForFirstPhoto_Existing(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("x"), 0644))

	name, err := waitForFirstPhoto(dir)
	require.NoError(t, err)
	assert.Equal(t, "photo.jpg", name)
}

// --- unknown path ---

func TestUnknownPath_NotFound(t *testing.T) {
	mux := testMux(t.TempDir(), t.TempDir(), "/g/x/", parseTpl(t))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/unknown/path", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
