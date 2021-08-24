package main

// KEYFILE="/etc/letsencrypt/live/lccny6.drewles.com/privkey.pem" \
// CERTFILE="/etc/letsencrypt/live/lccny6.drewles.com/fullchain.pem" \


import (
    "fmt"
    "os"
    "log"
    "net/http"
    "time"
    "io"
    // "io/ioutil"
)

func main() {
    fmt.Println("hello world")  
	mux := http.NewServeMux()
	
	fs := http.FileServer(http.Dir("./public"))
	mux.Handle("/public/", http.StripPrefix("/public/", fs))
	
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("/ hit")
		// b, err := ioutil.ReadFile("public/index.html")
		// if err != nil {
		//     panic(err)
		// }
		// fmt.Fprintf(w, "%s", string(b))
		http.ServeFile(w, r, "./public/index.html")
	})
	
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		log.Printf("/upload hit")
		// Maximum upload of 50 MB files
		r.ParseMultipartForm(50 << 20)
		// Get handler for filename, size and headers
		file, handler, err := r.FormFile("myfile")
		if err != nil {
			panic(err)
			return
		}
	
		defer file.Close()
		fmt.Printf("Uploaded File: %+v\n", handler.Filename)
		fmt.Printf("File Size: %+v\n", handler.Size)
		fmt.Printf("MIME Header: %+v\n", handler.Header)
	
		// Create file
		dst, err := os.Create("/tmp/" + handler.Filename)
		defer dst.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	
		// Copy the uploaded file to the created file on the filesystem
		if _, err := io.Copy(dst, file); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	
		// http.ServeFile(w, r, "./public/index.html")
		// w.Header.Set("")
		dur := time.Since(startTime)
		http.Redirect(w, r, "/?server_duration=" + fmt.Sprintf("%0.2f", dur.Seconds()), 302)
	})
	
	httpsServer := &http.Server{
		Addr:         ":" + os.Getenv("WEBSERVER_PORT"),
		ReadTimeout:  20 * time.Minute,
		WriteTimeout: 20 * time.Minute,
		IdleTimeout:  5 * time.Second,
		Handler: mux,
	}
    log.Fatal(httpsServer.ListenAndServeTLS(os.Getenv("CERTFILE"), os.Getenv("KEYFILE")))
}