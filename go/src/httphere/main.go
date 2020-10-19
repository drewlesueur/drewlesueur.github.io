package main

import "flag"
import "net/http"
import "time"
import "log"

func main() {
	port := flag.String("p", "8000", "port to listen on")
	prefix := flag.String("prefix", "/", "proxy prefix to trim")
	flag.Parse()

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("."))
	mux.Handle(*prefix, http.StripPrefix(*prefix, fs))

	srv := http.Server{
		Addr:         ":" + *port,
		Handler: mux,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	log.Printf("listening on :" + *port)
	log.Fatal(srv.ListenAndServe())

}
