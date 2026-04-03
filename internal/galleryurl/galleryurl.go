// Package galleryurl provides helpers for constructing gallery deep-link URLs.
package galleryurl

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HashPrefix returns the first 8 bytes (16 hex chars) of the SHA-256 hash of name.
// This matches the gallery URL scheme: /g/<hash-prefix>/.
func HashPrefix(name string) string {
	sum := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", sum[:8])
}

// OldestPhotoName returns the filename (not full path) of the lexicographically
// smallest JPEG/PNG in dir, which corresponds to the oldest timestamp-named photo.
// Returns an empty string if the directory contains no photos.
func OldestPhotoName(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	oldest := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			continue
		}
		if oldest == "" || name < oldest {
			oldest = name
		}
	}
	return oldest, nil
}

// DirectURL returns the full deep-link URL for a photo, or an empty string if
// baseURL or oldestName is empty.
//
//	baseURL     e.g. "http://photobooth.local:8080"
//	oldestName  filename of the oldest photo (used to derive the gallery path)
//	photoName   filename of the photo to link to directly
func DirectURL(baseURL, oldestName, photoName string) string {
	if baseURL == "" || oldestName == "" {
		return ""
	}
	return baseURL + "/g/" + HashPrefix(oldestName) + "/#" + photoName
}
