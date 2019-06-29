package main

import "net/http"
import "time"
import "log"
import "flag"
import "fmt"
import "strings"
import "io/ioutil"
import "encoding/json"

type SaveResponse struct {
    Saved bool `json:"saved"`
    Error string `json:"error"`
}

func main() {
	port := flag.String("p", "8000", "port to listen on")
	indexFile := flag.String("indexfile", "./public/index.html", "path to index html file")
	flag.Parse()

	mux := http.NewServeMux()
	// fs := http.FileServer(http.Dir("./public"))
	//mux.Handle("/", http.StripPrefix("/", fs))

    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/" {
            http.ServeFile(w, r, *indexFile)
            return
        }
        if strings.Contains("..", r.URL.Path) {
            http.Error(w, "the path has a .. in it", http.StatusBadRequest)
            return 
        }
        if r.Method == "GET" {
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
            content := r.FormValue("content")
            // added this because once when I was traveling and
            // lost network connection while it was trying to save
            // it somehow saved an empty file. Partial request?
            if len(content) == 0 {
                http.Error(w, "empty content", http.StatusBadRequest)
                return
            } 
            s := SaveResponse{}
            err := ioutil.WriteFile(r.URL.Path[1:], []byte(content), 0644)
            if err != nil {
                s.Error = err.Error() 
            } else {
                s.Saved = true
            }
            
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(s)
        }
    })

	srv := http.Server{
		Addr:         ":" + *port,
		Handler: mux,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	log.Printf("listening on :" + *port)
	go log.Fatal(srv.ListenAndServe())


  httpsServer := &http.Server{
  		Addr:         ":443",
  		ReadTimeout:  30 * time.Second,
  		WriteTimeout: 30 * time.Second,
  		Handler:      authenticate(gziphandler.GzipHandler(mux)),
  	}

}