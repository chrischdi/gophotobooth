// Command gallery serves an HTTP photo gallery at an obscure URL derived from
// the SHA-256 hash of the oldest photo's filename.
package main

import (
	"crypto/sha256"
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
)

const galleryTpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Photo Booth Gallery</title>
<style>
* { box-sizing: border-box; }
body { margin: 0; background: #111; color: #eee; font-family: sans-serif; }
h1 { text-align: center; padding: 1rem; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 1rem; padding: 1rem; }
.grid img { width: 100%; border-radius: 4px; display: block; cursor: pointer; }

/* Lightbox */
#lb { display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.92); z-index: 100; }
#lb.open { display: flex; align-items: center; justify-content: center; }
#lb img { max-width: 90vw; max-height: 90vh; object-fit: contain; user-select: none; }
#lb-prev, #lb-next {
  position: absolute; top: 0; height: 100%; width: 20%;
  display: flex; align-items: center; justify-content: center;
  cursor: pointer; opacity: 0; transition: opacity 0.2s;
}
#lb-prev { left: 0; }
#lb-next { right: 0; }
#lb:hover #lb-prev, #lb:hover #lb-next { opacity: 1; }
#lb-prev span, #lb-next span {
  font-size: 3rem; color: #fff; text-shadow: 0 0 8px #000;
  pointer-events: none;
}
#lb-close {
  position: absolute; top: 1rem; right: 1.5rem;
  font-size: 2rem; cursor: pointer; color: #fff;
  text-shadow: 0 0 8px #000; line-height: 1;
}
#lb-counter {
  position: absolute; bottom: 1rem; left: 50%; transform: translateX(-50%);
  color: #ccc; font-size: 0.9rem;
}
#lb-download {
  position: absolute; bottom: 1rem; right: 1.5rem;
  color: #fff; text-decoration: none;
  background: rgba(255,255,255,0.15); border: 1px solid rgba(255,255,255,0.3);
  padding: 0.35rem 0.75rem; border-radius: 4px; font-size: 0.85rem;
}
#lb-download:hover { background: rgba(255,255,255,0.25); }
</style>
</head>
<body>
<h1>Photo Gallery</h1>
<div class="grid">
{{range $i, $p := .Photos}}
<img src="/thumb/{{$p.Name}}" loading="lazy" alt="{{$p.Name}}" data-index="{{$i}}">
{{end}}
</div>
<p style="text-align:center;color:#555">{{.Count}} photo(s) &mdash; last updated {{.Updated}}</p>

<div id="lb">
  <img id="lb-img" src="" alt="">
  <div id="lb-prev"><span>&#8592;</span></div>
  <div id="lb-next"><span>&#8594;</span></div>
  <div id="lb-close">&#x2715;</div>
  <div id="lb-counter"></div>
  <a id="lb-download" href="" download>&#x2B07; HD</a>
</div>

<script>
const photos = [{{range .Photos}}"{{.Name}}",{{end}}];
const lb     = document.getElementById('lb');
const lbImg  = document.getElementById('lb-img');
const lbCtr  = document.getElementById('lb-counter');
const lbDl   = document.getElementById('lb-download');
let current  = 0;

function show(i) {
  current = (i + photos.length) % photos.length;
  lbImg.src = '/preview/' + photos[current];
  lbDl.href = '/photo/' + photos[current];
  lbDl.download = photos[current];
  lbCtr.textContent = (current + 1) + ' / ' + photos.length;
  lb.classList.add('open');
}

function close() { lb.classList.remove('open'); lbImg.src = ''; }

document.querySelectorAll('.grid img').forEach(img => {
  img.addEventListener('click', () => show(Number(img.dataset.index)));
});

document.getElementById('lb-prev').addEventListener('click', () => show(current - 1));
document.getElementById('lb-next').addEventListener('click', () => show(current + 1));
document.getElementById('lb-close').addEventListener('click', close);

lb.addEventListener('click', e => { if (e.target === lb || e.target === lbImg) close(); });

document.addEventListener('keydown', e => {
  if (!lb.classList.contains('open')) return;
  if (e.key === 'ArrowLeft')  show(current - 1);
  if (e.key === 'ArrowRight') show(current + 1);
  if (e.key === 'Escape')     close();
});
</script>
</body>
</html>`

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

func oldestPhotoName(dir string) (string, error) {
	photos, err := listPhotos(dir)
	if err != nil {
		return "", err
	}
	if len(photos) == 0 {
		return "", nil
	}
	// listPhotos returns newest-first; oldest is last.
	return photos[len(photos)-1].Name, nil
}

func hashPrefix(name string) string {
	sum := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", sum[:8]) // 16 hex chars
}

// waitForFirstPhoto blocks until at least one photo exists in dir.
func waitForFirstPhoto(dir string) (string, error) {
	name, err := oldestPhotoName(dir)
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
	defer watcher.Close()

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
			name, err := oldestPhotoName(dir)
			if err != nil || name == "" {
				continue
			}
			return name, nil
		case err := <-watcher.Errors:
			return "", err
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

			firstName, err := waitForFirstPhoto(photoDir)
			if err != nil {
				return fmt.Errorf("waiting for first photo: %w", err)
			}
			prefix := hashPrefix(firstName)
			galleryPath := "/g/" + prefix + "/"

			slog.Info("gallery URL ready", "path", galleryPath, "addr", listenAddr)

			mux := http.NewServeMux()

			// robots.txt — block all crawlers.
			mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprintln(w, "User-agent: *\nDisallow: /")
			})

			// Serve individual photos (full HD).
			mux.Handle("/photo/", http.StripPrefix("/photo/", http.FileServer(http.Dir(photoDir))))

			// Serve thumbnails (400px) and previews (1280px) via vipsthumbnail.
			mux.HandleFunc("/thumb/", serveResized(photoDir, cacheDir, 400))
			mux.HandleFunc("/preview/", serveResized(photoDir, cacheDir, 1280))

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
