package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/chrischdi/gophotobooth/pkg/photobooth"
)

type CameraAPI struct {
	fileServer   http.Handler
	pb           *photobooth.Photobooth
	adminEnabled bool
	directory    string
}

func NewCameraAPI(pb *photobooth.Photobooth, directory string) *CameraAPI {
	return &CameraAPI{pb: pb, adminEnabled: true, directory: directory}
}

func (a *CameraAPI) Serve(addr string, timeout time.Duration) error {
	a.register()
	go a.disableAdmin(timeout)
	return http.ListenAndServe(addr, nil)
}

func (a *CameraAPI) disableAdmin(timeout time.Duration) {
	time.Sleep(timeout)
	a.adminEnabled = false
}

func (a *CameraAPI) register() {
	http.HandleFunc("/api/autofocus", a.serveAutofocus)
	http.HandleFunc("/api/shutterspeed/inc", a.shutterspeedInc)
	http.HandleFunc("/api/shutterspeed/dec", a.shutterspeedDec)
	http.HandleFunc("/api/toggle", a.toggle)
	http.HandleFunc("/", a.redirect)
	http.HandleFunc("/ui/", a.ui)
	a.fileServer = http.FileServer(http.Dir(a.directory))
	http.Handle("/files/", http.StripPrefix("/files/", a.fileServer))
}

func (a *CameraAPI) serveAutofocus(w http.ResponseWriter, r *http.Request) {
	if !a.adminEnabled {
		a.redirect(w, r)
		return
	}
	if err := a.pb.Cam.Focus(); err != nil {
		fmt.Fprintf(w, "error on Focus: %v", err)
		return
	}
	http.Redirect(w, r, "/ui/", http.StatusSeeOther)
}

func (a *CameraAPI) shutterspeedInc(w http.ResponseWriter, r *http.Request) {
	if !a.adminEnabled {
		a.redirect(w, r)
		return
	}
	if err := a.pb.Cam.ShutterspeedInc(); err != nil {
		fmt.Fprintf(w, "error on ShutterspeedInc: %v", err)
		return
	}
	http.Redirect(w, r, "/ui/", http.StatusSeeOther)
}

func (a *CameraAPI) shutterspeedDec(w http.ResponseWriter, r *http.Request) {
	if !a.adminEnabled {
		a.redirect(w, r)
		return
	}
	if err := a.pb.Cam.ShutterspeedDec(); err != nil {
		fmt.Fprintf(w, "error on ShutterspeedDec: %v", err)
		return
	}
	http.Redirect(w, r, "/ui/", http.StatusSeeOther)
}

func (a *CameraAPI) toggle(w http.ResponseWriter, r *http.Request) {
	if !a.adminEnabled {
		a.redirect(w, r)
		return
	}
	fmt.Printf("toggling\n")
	if err := a.pb.TriggerWorkflow(); err != nil {
		fmt.Fprintf(w, "error on ShutterspeedDec: %v", err)
		return
	}
	http.Redirect(w, r, "/ui/", http.StatusSeeOther)
}

func (a *CameraAPI) redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/files", http.StatusSeeOther)
}

func (a *CameraAPI) ui(w http.ResponseWriter, r *http.Request) {
	if !a.adminEnabled {
		a.redirect(w, r)
		return
	}
	fmt.Fprint(w, ui)
}

const (
	ui = `<!DOCTYPE html>
<html>

<head>
	<meta charset='utf-8'>
	<meta http-equiv='X-UA-Compatible' content='IE=edge'>
	<title>Gophotobooth configuration</title>
	<meta name='viewport' content='width=device-width, initial-scale=1'>
	<link rel='stylesheet' type='text/css' media='screen' href='main.css'>
	<script src='main.js'></script>
</head>

<body>
	<h1>Gophotobooth Settings</h1>
	<h2>Shutterspeed</h2>
	<form action="/api/shutterspeed/inc">
		<button type="submit">Brighter</button>
	</form>

	<form action="/api/shutterspeed/dec">
		<button type="submit">Darker</button>
	</form>
	<h2>Autofocus</h2>
	<form action="/api/autofocus">
		<button type="submit">Focus</button>
	</form>
	<h2>Toggle</h2>
	<form action="/api/toggle">
		<button type="submit">Shoot</button>
	</form>

</body>

</html>`
)
