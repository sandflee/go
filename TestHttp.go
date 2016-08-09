package main

import (
	"net/http"
	"io"
	_"net/http/pprof"
)


func defaultHandler(w http.ResponseWriter, r * http.Request)  {
	io.WriteString(w, "hello world")
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", defaultHandler)
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w,"test")
	})
	http.ListenAndServe(":1111", mux)
}
