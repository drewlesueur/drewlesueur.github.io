package main

import "flag"
import "net/http"
import "time"
import "log"

func main() {
	port := flag.String("p", "8000", "port to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("."))
	mux.Handle("/", http.StripPrefix("/", fs))

	srv := http.Server{
		Addr:         ":" + *port,
		Handler: mux,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	log.Printf("listening on :" + *port)
	log.Fatal(srv.ListenAndServe())

}
