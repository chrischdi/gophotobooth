// Command gallery serves an HTTP photo gallery at an obscure URL derived from
// the SHA-256 hash of the oldest photo's filename.
package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/chrischdi/gophotobooth/internal/galleryurl"
)

//go:embed templates/gallery.html
var galleryTpl string
type galleryData struct {
	Photos  []photoEntry
	Count   int
	Updated string
}

type photoEntry struct {
	Name string
}

func listPhotos(dir string) ([]photoEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var photos []photoEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			photos = append(photos, photoEntry{Name: name})
		}
	}
	// Newest first.
	sort.Slice(photos, func(i, j int) bool {
		return photos[i].Name > photos[j].Name
	})
	return photos, nil
}

// waitForFirstPhoto blocks until at least one photo exists in dir.
func waitForFirstPhoto(dir string) (string, error) {
	name, err := galleryurl.OldestPhotoName(dir)
	if err != nil {
		return "", err
	}
	if name != "" {
		return name, nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return "", err
	}
	defer watcher.Close() //nolint:errcheck

	if err := watcher.Add(dir); err != nil {
		return "", err
	}

	slog.Info("gallery: waiting for first photo", "dir", dir)
	for {
		select {
		case _, ok := <-watcher.Events:
			if !ok {
				return "", fmt.Errorf("watcher closed")
			}
			name, err := galleryurl.OldestPhotoName(dir)
			if err != nil || name == "" {
				continue
			}
			return name, nil
		case err := <-watcher.Errors:
			return "", err
		}
	}
}

// thumbWidthSmall and thumbWidthLarge are the two cached sizes (pixels wide).
const (
	thumbWidthSmall = 400
	thumbWidthLarge = 1280
)

// cleanStaleCache removes cached thumbnails whose source photo no longer exists.
func cleanStaleCache(photoDir, cacheDir string) {
	for _, width := range []int{thumbWidthSmall, thumbWidthLarge} {
		widthDir := filepath.Join(cacheDir, fmt.Sprintf("%d", width))
		entries, err := os.ReadDir(widthDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			slog.Error("cache cleanup: read dir", "dir", widthDir, "err", err)
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(photoDir, e.Name())); os.IsNotExist(err) {
				p := filepath.Join(widthDir, e.Name())
				slog.Info("removing stale cache entry", "name", e.Name(), "width", width)
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					slog.Error("cache cleanup: remove", "path", p, "err", err)
				}
			}
		}
	}
}

// preFillCache generates missing thumbnails for all photos already on disk.
func preFillCache(photoDir, cacheDir string) {
	photos, err := listPhotos(photoDir)
	if err != nil {
		slog.Error("pre-fill: list photos", "err", err)
		return
	}
	for _, p := range photos {
		for _, width := range []int{thumbWidthSmall, thumbWidthLarge} {
			srcPath := filepath.Join(photoDir, p.Name)
			dstPath := filepath.Join(cacheDir, fmt.Sprintf("%d", width), p.Name)
			if _, err := os.Stat(dstPath); os.IsNotExist(err) {
				slog.Info("pre-filling cache", "name", p.Name, "width", width)
				if err := generateThumbnail(srcPath, dstPath, width); err != nil {
					slog.Error("pre-fill thumbnail failed", "name", p.Name, "width", width, "err", err)
				}
			}
		}
	}
}

// watchAndFillCache watches photoDir and proactively generates thumbnails for new photos.
func watchAndFillCache(photoDir, cacheDir string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("cache watcher: create", "err", err)
		return
	}
	defer watcher.Close() //nolint:errcheck

	if err := watcher.Add(photoDir); err != nil {
		slog.Error("cache watcher: add dir", "err", err)
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create == 0 {
				continue
			}
			name := filepath.Base(event.Name)
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				continue
			}
			for _, width := range []int{thumbWidthSmall, thumbWidthLarge} {
				dstPath := filepath.Join(cacheDir, fmt.Sprintf("%d", width), name)
				slog.Info("proactive cache fill", "name", name, "width", width)
				if err := generateThumbnail(event.Name, dstPath, width); err != nil {
					slog.Error("proactive thumbnail failed", "name", name, "width", width, "err", err)
				}
			}
		case err := <-watcher.Errors:
			slog.Error("cache watcher error", "err", err)
		}
	}
}

// generateThumbnail resizes srcPath to maxWidth pixels wide using vipsthumbnail
// and writes the result to dstPath.
func generateThumbnail(srcPath, dstPath string, maxWidth int) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	quality := 80
	if maxWidth <= 400 {
		quality = 70
	}
	size := fmt.Sprintf("%dx99999", maxWidth)
	output := fmt.Sprintf("%s[Q=%d]", dstPath, quality)
	cmd := exec.Command("vipsthumbnail", srcPath, "--size", size, "-o", output)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vipsthumbnail: %w", err)
	}
	return nil
}

// serveResized handles thumbnail/preview requests. It generates a resized JPEG
// on demand via vipsthumbnail and caches the result under cacheDir/<maxWidth>/.
func serveResized(photoDir, cacheDir string, maxWidth int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			http.NotFound(w, r)
			return
		}
		srcPath := filepath.Join(photoDir, name)
		dstPath := filepath.Join(cacheDir, fmt.Sprintf("%d", maxWidth), name)

		if _, err := os.Stat(dstPath); os.IsNotExist(err) {
			slog.Info("generating thumbnail", "name", name, "width", maxWidth)
			if err := generateThumbnail(srcPath, dstPath, maxWidth); err != nil {
				slog.Error("thumbnail generation failed", "name", name, "err", err)
				http.Error(w, "failed to generate thumbnail", http.StatusInternalServerError)
				return
			}
		}
		http.ServeFile(w, r, dstPath)
	}
}

func main() {
	var (
		listenAddr string
		photoDir   string
		cacheDir   string
	)

	cmd := &cobra.Command{
		Use:   "gallery",
		Short: "HTTP photo gallery server",
		RunE: func(cmd *cobra.Command, args []string) error {
			tpl, err := template.New("gallery").Parse(galleryTpl)
			if err != nil {
				return fmt.Errorf("parse template: %w", err)
			}

			cleanStaleCache(photoDir, cacheDir)

			firstName, err := waitForFirstPhoto(photoDir)
			if err != nil {
				return fmt.Errorf("waiting for first photo: %w", err)
			}
			prefix := galleryurl.HashPrefix(firstName)
			galleryPath := "/g/" + prefix + "/"

			slog.Info("gallery URL ready", "path", galleryPath, "addr", listenAddr)

			go preFillCache(photoDir, cacheDir)
			go watchAndFillCache(photoDir, cacheDir)

			mux := http.NewServeMux()

			// robots.txt — block all crawlers.
			mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprintln(w, "User-agent: *\nDisallow: /") //nolint:errcheck
			})

			// Serve individual photos (full HD).
			mux.Handle("/photo/", http.StripPrefix("/photo/", http.FileServer(http.Dir(photoDir))))

			// Serve thumbnails and previews via vipsthumbnail.
			mux.HandleFunc("/thumb/", serveResized(photoDir, cacheDir, thumbWidthSmall))
			mux.HandleFunc("/preview/", serveResized(photoDir, cacheDir, thumbWidthLarge))

			// Gallery page at the obscure path.
			mux.HandleFunc(galleryPath, func(w http.ResponseWriter, r *http.Request) {
				photos, err := listPhotos(photoDir)
				if err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				data := galleryData{
					Photos:  photos,
					Count:   len(photos),
					Updated: time.Now().Format(time.RFC1123),
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if err := tpl.Execute(w, data); err != nil {
					slog.Error("template execute", "err", err)
				}
			})

			// All other paths return 404.
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			})

			slog.Info("gallery listening", "addr", listenAddr)
			return http.ListenAndServe(listenAddr, mux)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", "0.0.0.0:8080", "HTTP listen address")
	cmd.Flags().StringVar(&photoDir, "directory", "/photos", "directory containing photos")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "/cache", "directory for generated thumbnails and previews")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
