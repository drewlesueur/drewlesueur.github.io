package main

import "net/http"
import "time"
import "log"
import "flag"
import "fmt"
import "strings"
import "io/ioutil"
import "encoding/json"


func main() {
	port := flag.String("p", "8000", "port to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	// fs := http.FileServer(http.Dir("./public"))
	//mux.Handle("/", http.StripPrefix("/", fs))

    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/" {
            http.ServeFile(w, r, "./public/index.html")
            return
        }
        if r.Method == "GET" {
            if strings.Contains("..", r.URL.Path) {
                http.Error(w, "the path has a .. in it", http.StatusBadRequest)
                return 
            }
            b, err := ioutil.ReadFile("./public/index.html")
            if err != nil {
                http.Error(w, "error reading index file", http.StatusInternalServerError)
                return
            }
            c, err := ioutil.ReadFile(r.URL.Path[1:])
            if err != nil {
                http.Error(w, "error reading requested file", http.StatusInternalServerError)
                return
            }
            htmlString := string(b)
            contentString := string(c)
            contentLines := strings.Split(contentString, "\n")
            contentLinesJSON, err := json.MarshalIndent(contentLines, "", " ")
            contentLinesJSONString := string(contentLinesJSON)
            htmlString = strings.Replace(htmlString, "// LINES GO HERE", "var lines = " + contentLinesJSONString, 1)
            if r.FormValue("src") != "1" {
                w.Header().Set("Content-Type", "text/html")
            }
            fmt.Fprintf(w, "%s", htmlString)
        } else if r.Method == "POST" {
            
        }
    })

	srv := http.Server{
		Addr:         ":" + *port,
		Handler: mux,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	log.Printf("listening on :" + *port)
	log.Fatal(srv.ListenAndServe())

}