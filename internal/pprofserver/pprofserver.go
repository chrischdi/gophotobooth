// Package pprofserver starts a net/http/pprof endpoint in the background.
package pprofserver

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof" // registers handlers on http.DefaultServeMux
)

// Start listens on addr and serves pprof endpoints. It runs in a background
// goroutine and logs a warning if the server exits unexpectedly. Passing an
// empty addr is a no-op.
func Start(addr string) {
	if addr == "" {
		return
	}
	go func() {
		slog.Info("pprof listening", "addr", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			slog.Warn("pprof server stopped", "err", err)
		}
	}()
}
