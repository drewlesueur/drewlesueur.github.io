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
	location := flag.String("location", "", "path to directory to serve")
	proxyPath := flag.String("proxypath", "", "the path for proxies, what to ignore")
	allowedIPsStr := os.Getenv("ALLOWEDIPS")
	allowedIPs := strings.Split(allowedIPsStr, ",")
	allowedIPsMap := map[string]bool{}
	for _, ip := range allowedIPs {
		if ip != "" {
			allowedIPsMap[ip] = true
		}
	}
	certFile := os.Getenv("CERTFILE")
	keyFile := os.Getenv("KEYFILE")
	flag.Parse()
	log.Printf("certFile: %s", certFile)
	log.Printf("keyfile: %s", keyFile)

	if *location == "" {
		cmd := exec.Command("bash", "-c", "pwd")
		ret, err := cmd.Output()
		if err != nil {
			log.Fatal("could not get cwd")
		}
		*location = strings.TrimSpace(string(ret))
	}
	log.Printf("location: %s", *location)
	mux := http.NewServeMux()
	// fs := http.FileServer(http.Dir("./public"))
	//mux.Handle("/", http.StripPrefix("/", fs))

	mux.HandleFunc("/yo", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./public/yo.html")
	})
	mux.HandleFunc("/yo/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("the yo path: %s", r.URL.Path)
		http.ServeFile(w, r, "./public/yo.html")
	})
	mux.HandleFunc("/mybash", func(w http.ResponseWriter, r *http.Request) {
		cmdString := r.FormValue("cmd")
		if cmdString == "" {
			cmdString = ":"
		}
		cwd := r.FormValue("cwd") // current working directory

		// add the cwd so the client can remember it
		cmdString = "cd " + cwd + ";\n" + cmdString + ";\necho ''; pwd"

		log.Printf("the command we want is: %s", cmdString)
		cmd := exec.Command("bash", "-c", cmdString)
		ret, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("there was and error running command: %s", cmdString)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		//lines := strings.Split(string(r), "\n")

		log.Printf("the combined output of the command is: %s", string(ret))
		w.Write(ret)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains("..", r.URL.Path) {
			http.Error(w, "the path has a .. in it", http.StatusBadRequest)
			return
		}
		if r.Method == "GET" {
			var c []byte

			// splitting on comma and only loading the first one
			parts := strings.Split(r.URL.Path, ",")
			filePath := parts[0][1:]
			if filePath == "" {
				filePath = "."
			}
			fullPath := *location + "/" + filePath
			log.Printf("the full path is: %s", fullPath)
			fileInfo, err := os.Stat(fullPath)
			if err != nil {
				http.Error(w, "error determining file type", http.StatusInternalServerError)
				return
			}
			isDir := false
			if fileInfo.IsDir() {
				isDir = true
				files, err := ioutil.ReadDir(fullPath)
				if err != nil {
					http.Error(w, "could not read files", http.StatusInternalServerError)
					return
				}
				fileNames := make([]string, len(files))
				for i, f := range files {
					fileNames[i] = f.Name()
				}
				w.Header().Set("X-Is-Dir", "1")

				if r.FormValue("raw") == "1" {
					w.Write([]byte(strings.Join(fileNames, "\n")))
					return
				}

				c = []byte(strings.Join(fileNames, "\n"))
			} else {

				c2, err := ioutil.ReadFile(fullPath)

				if err != nil {
					http.Error(w, "error reading requested file", http.StatusInternalServerError)
					return
				}
				c = c2

				if r.FormValue("raw") == "1" {
					w.Write(c)
					return
				}
			}
			log.Printf("is dir? %t", isDir)

			b, err := ioutil.ReadFile(*indexFile)
			if err != nil {
				http.Error(w, "error reading index file", http.StatusInternalServerError)
				return
			}
			htmlString := string(b)
			contentString := string(c)
			contentLines := strings.Split(contentString, "\n")
			contentLinesJSON, err := json.MarshalIndent(contentLines, "", " ")
			contentLinesJSONString := string(contentLinesJSON)

			if isDir {
				htmlString = strings.Replace(htmlString, "// FILEMODE DIRECTORY GOES HERE", "fileMode = \"directory\"", 1)
			}
			htmlString = strings.Replace(htmlString, "// ROOTLOCATION GOES HERE", "var rootLocation = \""+*location+"\"", 1)
			if *proxyPath != "" {
				replaceProxyPath := "var proxyPath = \"" + *proxyPath + "\""
				htmlString = strings.Replace(htmlString, "// PROXYPATH GOES HERE", replaceProxyPath, 1)
				log.Printf("replaceProxyPath: %s", replaceProxyPath)
			}

			// This content lines has to be the last one.
			htmlString = strings.Replace(htmlString, "// LINES GO HERE", "var lines = "+contentLinesJSONString, 1)

			// TODO: when bash mode is disabled, don't do this part.
			log.Printf("yea I set rootLocation to be: %s", *location)
			if r.FormValue("src") != "1" {
				w.Header().Set("Content-Type", "text/html")
			}
			ioutil.WriteFile("tmp", []byte(htmlString), 0777)
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
			err := ioutil.WriteFile(*location+"/"+r.URL.Path[1:], []byte(content), 0644)
			if err != nil {
				s.Error = err.Error()
			} else {
				s.Saved = true
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(s)
		}
	})

	mainMux := gziphandler.GzipHandler(mux)
	if os.Getenv("NOBASICAUTH") == "" {
		mainMux = BasicAuth(mainMux)
	}

	if len(allowedIPsMap) > 0 {
		oldMainMux := mainMux
		mainMux = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ipParts := strings.Split(r.RemoteAddr, ":")
			for i := 0; i < 1; i++ {
				if len(allowedIPsMap) == 0 {
					break
				}
				if len(ipParts) == 0 {
					return
				}
				if _, ok := allowedIPsMap[ipParts[0]]; !ok {
					log.Printf("unalowed ip: %s", ipParts[0])
					return
				}
			}
			oldMainMux.ServeHTTP(w, r)
		})
	}
	redirectMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+r.URL.Host, http.StatusFound)
	})

	// Allow it to be behind a proxy.
	if proxyPath != nil && *proxyPath != "" {
		oldMainMux := mainMux
		mainMux = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			parts := strings.Split(r.URL.Path, ",")
			for i, part := range parts {
				part = strings.TrimPrefix(part, *proxyPath)
				parts[i] = part
			}
			r.URL.Path = strings.Join(parts, ",")
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
			if r.URL.Path[0:1] != "/" {
				r.URL.Path = "/" + r.URL.Path
			}
			log.Printf("processsed URL: %s =====", r.URL.Path)
			oldMainMux.ServeHTTP(w, r)
		})
	}

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
