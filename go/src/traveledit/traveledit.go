package main

import "log"
import "net/http"
import "flag"
import "strings"
import "io/ioutil"
import "html"
import "time"
import "os"

func main() {

    var port = flag.String("port", "9000", "Port to listen on")
    var requiredKey = flag.String("key", "needthiskey", "key to block access")
    flag.Parse()
    cwd, err := os.Getwd()
    if err != nil {
        log.Fatal(err)
    }
    serveMux := http.NewServeMux()

    serveMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        key := r.FormValue("key")
        filepath := r.FormValue("filepath")
        bodycontent := r.FormValue("bodycontent")
        getpath := r.FormValue("getpath")
        _ = getpath
        action := r.FormValue("action")
        if key != *requiredKey {
            http.Error(w, "bad credentials", http.StatusInternalServerError)
            return
        }

        // Make sure you can't go up a directory
        filepath = strings.Replace(filepath, "..", "", -1)

        // And that you don't start at a root directory
        if len(filepath) > 1 && filepath[0:1] == "/" {
            filepath = filepath[1:]
        }

        log.Printf("filepath is: %s", filepath)
        log.Printf("bodyContent is: %s", bodycontent)
        log.Printf("action is: %s", action)

        if r.Method == "POST" && action == "Load" {
            // we redirect so the url is affected
            //http.Redirect(
            //return
        }

        if r.Method == "POST" && action == "Save" {
            // todo: is there a way to do this in a setting?
            bodycontent = strings.Replace(bodycontent, "\r\n", "\n", -1)
            err := ioutil.WriteFile(filepath, []byte(bodycontent), 0664)
            if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
        }

        if r.Method == "POST" && action == "Stop" {
            log.Printf("Stopping by request. We should restart.")
            os.Exit(1)
        }

        w.Header().Set("Content-Type", "text/html")

        // Because Chrome doesnt like when we are posting inputs and textareas I guess.
        // And it seems Safari will disable JavaScript unless I add this.
        w.Header().Set("X-XSS-Protection", "0")

        var indexPage string
        indexPageBytes, err := ioutil.ReadFile("./index.html")
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        } else {
            indexPage = string(indexPageBytes)
        }

        bodyContentBytes, err := ioutil.ReadFile(filepath)
        if err != nil {
            if strings.Contains(err.Error(), "no such file or directory") {
                bodyContentBytes = []byte("")
            } else {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
        }
        replacer := strings.NewReplacer(
            "FILEPATH", html.EscapeString(filepath),
            "BODYCONTENT", html.EscapeString(string(bodyContentBytes)),
            "CWD", cwd,
        )

        indexPage = replacer.Replace(indexPage)
        w.Write([]byte(indexPage))

    })

    // Start the http server.
    srv := &http.Server{
        Addr:         ":" + *port,
        ReadTimeout:  20 * time.Second,
        WriteTimeout: 20 * time.Second,
        Handler:      serveMux,
    }
    log.Println("http listening on", ":"+*port)
    log.Fatal(srv.ListenAndServe())
}

