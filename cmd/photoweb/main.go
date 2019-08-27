package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/pflag"
)

var directory = pflag.StringP("directory", "d", "/mnt", "directory where to save the pictures")
var port = pflag.IntP("port", "p", 8080, "port to serve the given directory")

func main() {
	pflag.Parse()

	host := fmt.Sprintf(":%d", *port)
	fmt.Printf("serving directory %s at %s\n", *directory, host)
	fs := &fileServer{
		http.FileServer(http.Dir(*directory)),
	}
	err := http.ListenAndServe(host, fs)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
}

type fileServer struct {
	fileHandler http.Handler
}

func (fs *fileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/" {
		fmt.Fprint(w, header)
	}
	fs.fileHandler.ServeHTTP(w, r)
}

const header = `
<title>Photobox</title>
<style>
@media screen and (-webkit-min-device-pixel-ratio: 1.5) {
	a {
		font-size:3vw;
	}
}
</style>
`
