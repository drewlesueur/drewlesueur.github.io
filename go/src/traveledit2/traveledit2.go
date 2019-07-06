package main

import "net/http"
import "time"
import "log"
import "flag"
import "fmt"
import "strings"
import "os"
import "os/exec"
import "crypto/subtle"
import "io/ioutil"
import "encoding/json"
import "github.com/NYTimes/gziphandler"

type SaveResponse struct {
	Saved bool   `json:"saved"`
	Error string `json:"error"`
}

func BasicAuth(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := os.Getenv("BASICUSER")
		pass := os.Getenv("BASICPASS")
		if user == "" || pass == "" {
			log.Fatal("BASICUSER or BASICPASS environment variables not set")
		}

		log.Printf("url hit: %s by %s", r.URL.Path, r.RemoteAddr)
		rUser, rPass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(rUser), []byte(user)) != 1 || subtle.ConstantTimeCompare([]byte(rPass), []byte(pass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Hi. Please log in."`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorized.\n"))
			return
		}
		handler.ServeHTTP(w, r)
	}
}

func main() {
	port := flag.String("p", "8000", "port to listen on")
	indexFile := flag.String("indexfile", "./public/index.html", "path to index html file")
	location := flag.String("location", ".", "path to directory to serve")

	certFile := os.Getenv("CERTFILE")
	keyFile := os.Getenv("KEYFILE")
	flag.Parse()
    log.Printf("certFile: %s", certFile)
    log.Printf("keyfile: %s", keyFile)

	mux := http.NewServeMux()
	// fs := http.FileServer(http.Dir("./public"))
	//mux.Handle("/", http.StripPrefix("/", fs))


	mux.HandleFunc("/mybash", func(w http.ResponseWriter, r *http.Request) {
	  cmd := exec.Command("bash", "-c", r.FormValue("cmd"))
	  ret, err := cmd.CombinedOutput()
	  if err != nil {
	    http.Error(w, err.Error(), http.StatusInternalServerError)
	    return
	  }
	  w.Write(ret)
	})
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
			c, err := ioutil.ReadFile(*location + "/" + r.URL.Path[1:])
			if err != nil {
				http.Error(w, "error reading requested file", http.StatusInternalServerError)
				return
			}
			
			if r.FormValue("raw") == "1" {
			  w.Write(c)
			  return
			}
			
			b, err := ioutil.ReadFile("./public/index.html")
			if err != nil {
				http.Error(w, "error reading index file", http.StatusInternalServerError)
				return
			}
			htmlString := string(b)
			contentString := string(c)
			contentLines := strings.Split(contentString, "\n")
			contentLinesJSON, err := json.MarshalIndent(contentLines, "", " ")
			contentLinesJSONString := string(contentLinesJSON)
			htmlString = strings.Replace(htmlString, "// LINES GO HERE", "var lines = "+contentLinesJSONString, 1)
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
			err := ioutil.WriteFile(*location + "/" + r.URL.Path[1:], []byte(content), 0644)
			if err != nil {
				s.Error = err.Error()
			} else {
				s.Saved = true
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(s)
		}
	})

	mainMux := BasicAuth(gziphandler.GzipHandler(mux))
	redirectMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+r.URL.Host, http.StatusFound)
	})
	httpServer := http.Server{
		Addr:         ":" + *port,
		Handler:      redirectMux,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	httpsServer := &http.Server{
		Addr:         ":" + *port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		Handler:      mainMux,
	}
	if keyFile == "" && certFile == "" {
		httpServer.Handler = mainMux
		log.Fatal(httpServer.ListenAndServe())
		return
	}

	log.Fatal(httpsServer.ListenAndServeTLS(certFile, keyFile))
	return

}
