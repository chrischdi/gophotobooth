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
	err := http.ListenAndServe(host, http.FileServer(http.Dir(*directory)))
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
}
