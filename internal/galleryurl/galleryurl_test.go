package galleryurl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPrefix(t *testing.T) {
	got := HashPrefix("photo.jpg")
	assert.Len(t, got, 16, "must return exactly 16 hex chars")

	// Deterministic
	assert.Equal(t, got, HashPrefix("photo.jpg"))

	// Different inputs differ
	assert.NotEqual(t, HashPrefix("a.jpg"), HashPrefix("b.jpg"))

	// Empty string doesn't panic
	empty := HashPrefix("")
	assert.Len(t, empty, 16)
}

func TestOldestPhotoName_Empty(t *testing.T) {
	dir := t.TempDir()
	name, err := OldestPhotoName(dir)
	require.NoError(t, err)
	assert.Empty(t, name)
}

func TestOldestPhotoName_NoImages(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0644))
	name, err := OldestPhotoName(dir)
	require.NoError(t, err)
	assert.Empty(t, name)
}

func TestOldestPhotoName_Single(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("x"), 0644))
	name, err := OldestPhotoName(dir)
	require.NoError(t, err)
	assert.Equal(t, "photo.jpg", name)
}

func TestOldestPhotoName_Multiple(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"2026-03-01.jpg", "2026-01-01.jpeg", "2026-06-01.png"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644))
	}
	// Also add a non-image file and a subdirectory — both should be ignored
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))

	name, err := OldestPhotoName(dir)
	require.NoError(t, err)
	assert.Equal(t, "2026-01-01.jpeg", name, "should return lexicographically smallest")
}

func TestOldestPhotoName_ExtensionCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "A.JPG"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "B.JPEG"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "C.PNG"), []byte("x"), 0644))
	name, err := OldestPhotoName(dir)
	require.NoError(t, err)
	assert.Equal(t, "A.JPG", name)
}

func TestOldestPhotoName_NonexistentDir(t *testing.T) {
	_, err := OldestPhotoName("/does/not/exist")
	assert.Error(t, err)
}

func TestGalleryURL_EmptyBase(t *testing.T) {
	got := GalleryURL("", "oldest.jpg")
	assert.Empty(t, got)
}

func TestGalleryURL_EmptyOldest(t *testing.T) {
	got := GalleryURL("http://example.com", "")
	assert.Empty(t, got)
}

func TestGalleryURL_Full(t *testing.T) {
	base := "http://example.com"
	oldest := "2026-01-01.jpg"
	got := GalleryURL(base, oldest)
	want := base + "/g/" + HashPrefix(oldest) + "/"
	assert.Equal(t, want, got)
}

func TestDirectURL_EmptyBase(t *testing.T) {
	got := DirectURL("", "oldest.jpg", "photo.jpg")
	assert.Empty(t, got)
}

func TestDirectURL_EmptyOldest(t *testing.T) {
	got := DirectURL("http://example.com", "", "photo.jpg")
	assert.Empty(t, got)
}

func TestDirectURL_Full(t *testing.T) {
	base := "http://example.com"
	oldest := "2026-01-01.jpg"
	photo := "2026-03-22.jpg"
	got := DirectURL(base, oldest, photo)
	want := base + "/g/" + HashPrefix(oldest) + "/#" + photo
	assert.Equal(t, want, got)
}
