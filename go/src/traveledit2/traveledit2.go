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
import "io"
import "bufio"
import "encoding/json"
import "encoding/base64"
import "sync"
import "strconv"
import "crypto/md5"
import "github.com/NYTimes/gziphandler"
import "github.com/creack/pty"
// import "github.com/gorilla/websocket"

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
		
		if os.Getenv("SCREENSHARENOAUTH") == "1" {
		    if r.URL.Path == "/screenshare" || r.URL.Path == "/view" {
				handler.ServeHTTP(w, r)
				return
		    }
		}

		// if r.URL.Path == "/wsrender" {
		// 	handler.ServeHTTP(w, r)
		// 	return
		// }

		log.Printf("url hit: %s by %s", r.URL.Path, r.RemoteAddr)
		rUser, rPass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(rUser), []byte(user)) != 1 || subtle.ConstantTimeCompare([]byte(rPass), []byte(pass)) != 1 {
			log.Printf("unauthorized: %s", r.URL.Path)
			w.Header().Set("WWW-Authenticate", `Basic realm="Hi. Please log in."`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorized.\n"))
			return
		}
		handler.ServeHTTP(w, r)
	}
}

func logAndErr(w http.ResponseWriter, message string, args ...interface{}) {
	theLog := fmt.Sprintf(message, args...)
	ret := map[string]string{
	    "error": theLog, 
	}
	b, _ := json.Marshal(ret)
	log.Println(theLog)
	http.Error(w, string(b), 500)
}

// Index: foo
// ===================================================================
// --- foo	
// +++ foo	
// @@ -63,1 +63,1 @@
// -    // formats   
// +    // Here's what it looks like   
// 

// @@ -63 @@
// -    // formats   
// -}
// -
// +    // Here's what it looks like   
// +}
// +
func applyDiff(oldContents, diff string) (string, error) {
    // There is likely a much more optimized way of applying diff --
    // Maybe dealing with the lines in-place and keeping track of index adjustments
    
    // Doesn't handle issues related to new line at end of file
    oldLines := strings.Split(oldContents, "\n")
    diffLines := strings.Split(diff, "\n")
    
    lineI := -1
    diffI := -1
    state := "want@"
    newContentsSlice := []string{}
    nextDiffIndex := -1
    for i := 0; i < 20000; i++ {
       if state == "want@" {
           diffI++ 
           if diffI >= len(diffLines) {
               state = "doneDiff"
               continue
           }
           if !strings.HasPrefix(diffLines[diffI], "@@") {
               continue
           }
           nextDiffIndex = parseFirstNumber(diffLines[diffI]) - 1
           // log.Printf("FIRST NUMBER IS %d", nextDiffIndex)
           state = "getToNextIndex"
       } else if state == "getToNextIndex" {
           lineI++       
           if lineI >= len(oldLines) {
               break   
           }
           
           // this case only happens on first pass? or of there are adjacent chinks?
           if lineI == nextDiffIndex {
               lineI -= 1
               state = "in@"
               continue    
           } 
           newContentsSlice = append(newContentsSlice, oldLines[lineI])   
           // -1 works because chunks can't be adjacent?
           if lineI == nextDiffIndex - 1 { 
               state = "in@"
               // if diffI >= len(diffLines) {
               //     state = "doneDiff"
               //     continue
               // }
           }
       } else if state == "in@" {
           diffI++ 
           if diffI >= len(diffLines) {
               state = "doneDiff"
               continue
           }
           if strings.HasPrefix(diffLines[diffI], "-") {
               lineI++       
               if lineI >= len(oldLines) {
                   break   
               }
               // don't add   
               // you could check that the removed lines match
               // possibly optimize diff to not include the line removed, just the "-"?
           }  else if strings.HasPrefix(diffLines[diffI], "+") {
               newContentsSlice = append(newContentsSlice, diffLines[diffI][1:])   
           }  else if strings.HasPrefix(diffLines[diffI], "@@") {
               nextDiffIndex = parseFirstNumber(diffLines[diffI]) - 1
               state = "getToNextIndex"
           }  else {
               lineI++       
               if lineI >= len(oldLines) {
                   break   
               }
               // you could check that the lines match
               newContentsSlice = append(newContentsSlice, oldLines[lineI])   
           }
       } else if state == "doneDiff" {
           lineI++       
           if lineI >= len(oldLines) {
               break   
           }
           newContentsSlice = append(newContentsSlice, oldLines[lineI])   
       }
    }
    if state == "want@" {
        return oldContents, nil
    }
    return strings.Join(newContentsSlice, "\n"), nil
}

func parseFirstNumber(s string) int {
    numb := ""
    inNumber := false
    for _, c := range s {
        if inNumber {
            if c >= 48 && c <= 57 {
                numb += string(c)
            } else {
                break
            }
        } else {
            if c >= 48 && c <= 57 {
                numb += string(c)
                inNumber = true
            }
        }
    }
    if len(numb) > 10 {
        numb = numb[0:10]
    }
    n, _ := strconv.Atoi(numb)
    return n
}

// Will these die when the server restarts?
// I think not.
type TerminalSession struct{
    Cmd *exec.Cmd
    Pty *os.File
    ID int
    ReadBuffer []byte
    Closed bool
}

type TerminalResponse struct{
    Base64 string    
    // CWD ?? so we can keep track of directory changes
    Error string
    Closed bool
}
var terminalID = 0
var terminalMu sync.Mutex
var terminalSessions = map[int]*TerminalSession{}
var terminalCond *sync.Cond



// This is depredated
// needs to be able to
// ctrl+c
// ctrl+d
type ShellCommand struct {
    // not breaking it into command and args
    // because we don't parse the command,
    // we run i thru bash -c
    Command string        
    CWD string
    
    ClientID int
    
    // the line index of where the client has loaded
    // it's meant for simple streaming of output like tail.
    // Maybe in the future we do a full terminal emulator
    // tty, pty, Raw Mode? vt100?
    ClientOutIndex int
    ClientErrIndex int
    
    // This does not (yet?) understand terminal escape codes 
    OutLines []string
    ErrLines []string
    
    PID int
    
    // not specifying environment because
    // it's not yet an interactive shell
    // env vars will be the default plus whatever
    // is in the command itself 
}

func main() {
	serverAddress := flag.String("addr", "localhost:8000", "serverAddress to listen on")
	indexFile := flag.String("indexfile", "./public/index.html", "path to index html file")
	screenshareFile := flag.String("screensharefile", "./public/view.html", "path to view html file")
	location := flag.String("location", "", "path to directory to serve")
	proxyPath := flag.String("proxypath", "", "the path for proxies, what to ignore")
	// Whether or not the proxypath is removed by the reverse proxy
	// seems with apache ProxyPath it is removed.
	proxyPathTrimmed := flag.Bool("proxypathtrimmed", false, "does the reverse proxy trim the proxy path?")
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
	var renderCommands []interface{}
	var viewCounter int
	var viewFile string
	var viewSearch string
	var viewMu sync.Mutex
	viewCond := sync.NewCond(&viewMu)
	
	terminalCond = sync.NewCond(&terminalMu)
	
	// trying to use a single mutex for multiple shells? 
	// TODO: serialize and de-serialize the state
	
	shellIndex := -1
	var shellMu sync.Mutex
	shellCond := sync.NewCond(&shellMu) 
	shellCommands := map[int]*ShellCommand{}
	
	go func() {
		for range time.NewTicker(1 * time.Second).C {
			viewCond.Broadcast()
			shellCond.Broadcast()
			terminalCond.Broadcast()
		}
	}()

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./public"))
	mux.Handle("/tepublic/", http.StripPrefix("/tepublic/", fs))

	mux.HandleFunc("/yo", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./public/yo.html")
	})
	mux.HandleFunc("/yo/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("the yo path: %s", r.URL.Path)
		http.ServeFile(w, r, "./public/yo.html")
	})
	mux.HandleFunc("/screenshare", func(w http.ResponseWriter, r *http.Request) {
		// http.ServeFile(w, r, "./public/view.html")
		b, err := ioutil.ReadFile(*screenshareFile)
		if err != nil {
			logAndErr(w, "error reading screenshare file: %v", err)
			return
		}
		htmlString := string(b)
		if *proxyPath != "" {
			replaceProxyPath := "var proxyPath = \"" + *proxyPath + "\""
			htmlString = strings.Replace(htmlString, "// PROXYPATH GOES HERE", replaceProxyPath, 1)
			log.Printf("replaceProxyPath: %s", replaceProxyPath)
		}
		fmt.Fprintf(w, "%s", htmlString)
	})
	mux.HandleFunc("/render", func(w http.ResponseWriter, r *http.Request) {
		commands := []interface{}{}
		err := json.NewDecoder(r.Body).Decode(&commands)
		if err != nil {
			logAndErr(w, "could not decode commands: %v", err)
			return
		}
		viewMu.Lock()
		defer viewMu.Unlock()
		viewCounter += 1
		renderCommands = commands
		viewFile = r.Header.Get("X-File")
		viewSearch = r.Header.Get("X-Search")
		viewCond.Broadcast()
	})
	
	// Not using the websockets anymore
	// but still cool to see the code
	// upgrader := websocket.Upgrader{
	// 	CheckOrigin: func(r *http.Request) bool {
	// 		return true
	// 	},
	// }
	// mux.HandleFunc("/wsrender", func(w http.ResponseWriter, r *http.Request) {
	// 	log.Printf("got here!!!=========================")
	// 	c, err := upgrader.Upgrade(w, r, nil)
	// 	if err != nil {
	// 		logAndErr(w, "websocket upgrade: %v", err)
	// 		return
	// 	}
	// 	defer c.Close()
	// 	for {
	// 		_, message, err := c.ReadMessage()
	// 		if err != nil {
	// 			log.Printf("error reading: %v", err)
	// 			break
	// 		}
	// 		log.Printf("got from websocket: %d", len(message))
	// 		commands := []interface{}{}
	// 		err = json.Unmarshal(message, &commands)
	// 		if err != nil {
	// 			fmt.Sprintf("could not decode commands: %v", err)
	// 			break
	// 		}
	// 		viewMu.Lock()
	// 		viewCounter += 1
	// 		renderCommands = commands
	// 		viewCond.Broadcast()
	// 		viewMu.Unlock()
	// 	}
	// })
// 
	// mux.HandleFunc("/wsview", func(w http.ResponseWriter, r *http.Request) {
	// 	c, err := upgrader.Upgrade(w, r, nil)
	// 	if err != nil {
	// 		logAndErr(w, "websocket upgrade: %v", err)
	// 		return
	// 	}
	// 	defer c.Close()
	// 	clientViewCounter := -1
	// 	var b []byte
	// 	for {
	// 		viewMu.Lock()
	// 		startWait := time.Now()
	// 		timedOut := false
	// 		for {
	// 			if time.Since(startWait) > (10 * time.Second) {
	// 				timedOut = true
	// 				break
	// 			}
	// 			if clientViewCounter != viewCounter {
	// 				break
	// 			}
	// 			viewCond.Wait()
	// 		}
	// 		if timedOut {
	// 			err = c.WriteMessage(1, []byte("[[6]]"))
	// 			if err != nil {
	// 				log.Printf("error writing to client: %v", err)
	// 				goto breakOut
	// 			}
	// 			goto finish
	// 		}
	// 		clientViewCounter = viewCounter
	// 		b, err = json.Marshal(renderCommands)
	// 		if err != nil {
	// 			log.Printf("could not marshal: %v", err)
	// 			goto finish
	// 		}
	// 		log.Printf("size of view payload: %d", len(b))
	// 		// save the raw render commands so you don't have to marshal, unmarshal etc.
	// 		err = c.WriteMessage(1, b)
	// 		if err != nil {
	// 			log.Printf("error writing to client: %v", err)
	// 			goto breakOut
	// 		}
	// 		// you could wait to make sure client got it before continuing the loop
// 
	// 	finish:
	// 		viewMu.Unlock()
	// 		continue
// 
	// 	breakOut:
	// 		viewMu.Unlock()
	// 		break
// 
	// 	}
	// })
	
	mux.HandleFunc("/view", func(w http.ResponseWriter, r *http.Request) {
		clientViewCounter, _ := strconv.Atoi(r.FormValue("viewCounter"))

		viewMu.Lock()
		defer viewMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-View-Counter", strconv.Itoa(viewCounter))
		w.Header().Set("X-File", viewFile)
		w.Header().Set("X-Search", viewSearch)
		// if clientViewCounter == viewCounter {
		//     fmt.Fprintf(w, "%s", "[[6]]")
		//     return
		// }

		startWait := time.Now()
		timedOut := false
		for {
			if time.Since(startWait) > (10 * time.Second) {
				timedOut = true
				break
			}
			if clientViewCounter != viewCounter {
				break
			}
			viewCond.Wait()
		}

		if timedOut {
			fmt.Fprintf(w, "%s", "[[6]]")
			return
		}

		b, err := json.Marshal(renderCommands)
		if err != nil {
			logAndErr(w, "could not marshal: %v", err)
			return
		}
		log.Printf("size of view payload: %d", len(b))
		w.Write(b)
		// json.NewEncoder(w).Encode(renderCommands)
	})
	mux.HandleFunc("/myuploadfiles", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("uploading files: %s", r.Header.Get("Content-Type"))
		err := r.ParseMultipartForm(256 << 20) // 256MB
		if err != nil {
			logAndErr(w, "error parsing body: %v", err)
			return
		}

		fhs := r.MultipartForm.File["thefiles"]
		log.Printf("There are %d files", len(fhs))
		for _, fh := range fhs {
			var bytesWritten int64
			var newF *os.File
			log.Printf("a file! %s", fh.Filename)
			f, err := fh.Open()
			if err != nil {
				logAndErr(w, "file upload error: %v", err)
				goto finish
			}
			newF, err = os.Create("./uploads/" + fh.Filename)
			if err != nil {
				logAndErr(w, "file upload error: %v", err)
				goto finish
			}
			bytesWritten, err = io.Copy(newF, f)
			if bytesWritten != fh.Size {
				logAndErr(w, "file not written: missing bytes")
				goto finish
			}
			if err != nil {
				logAndErr(w, "file not written: %v", err)
				goto finish
			}

		finish:
			f.Close()
			newF.Close()
		}
	})
	
	mux.HandleFunc("/myterminalload", func(w http.ResponseWriter, r *http.Request) {
	    // my terminal load
	    // load existing terminal sessions.
	})
	mux.HandleFunc("/myterminalpoll", func(w http.ResponseWriter, r *http.Request) {
	    terminalMu.Lock()    
	    defer terminalMu.Unlock()
	    ret := map[int]TerminalResponse{}
	    timedOut := false
	    startWait := time.Now()
    WaitLoop:
		for {
			if time.Since(startWait) > (10 * time.Second) {
				timedOut = true
				break
			}
			
			// If multiple clients were to need to connect to the terminals
			// then we'd have to have a "stream-like" data structure for ReadBuffer
			// and also would need the client to keep track of where it was
    		for _, t := range terminalSessions {
				if len(t.ReadBuffer) > 0 { 
					break WaitLoop
				}
    		}
			terminalCond.Wait()
			log.Println("done waiting")
    		// logJSON(terminalSessions)
		}
		
		if !timedOut {
            log.Println("stuff found on terminal!")
    		for ID, t := range terminalSessions {
    		    tResp := TerminalResponse{} 
    		    if t.Closed {
    		        // we only delete it after the client gets it
    		        // maybe have a timeout and cleanup later?
    		        // or actually maybe delete it right away when it's closed
    		        // and then keepntrack of closed ids to send?
    		        delete(terminalSessions, ID)    
    		    } else {
    		        if len(t.ReadBuffer) == 0 {
    		            continue
    		        }
    		    }
    		    tResp.Base64 = base64.StdEncoding.EncodeToString(t.ReadBuffer)
    		    tResp.Closed = t.Closed
    		    t.ReadBuffer = []byte{}
    		    ret[ID] = tResp
    		}
		}
	    json.NewEncoder(w).Encode(ret)
	})
	mux.HandleFunc("/myterminalopen", func(w http.ResponseWriter, r *http.Request) {
	    log.Println("my terminal open!")
	    terminalID++
	    // TODO: configurable shell, login shell (-l)?
		cmd := exec.Command("bash")
		cwd := r.FormValue("cwd")
		cmd.Dir = cwd
	    
		f, err := pty.Start(cmd)
	    if err != nil {
			logAndErr(w, "starting pty: %s: %v", cwd, err) 
			return
	    }
	    // append the pid to a file for debugging
	    pidF, err := os.OpenFile("pid.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	    if err != nil {
			logAndErr(w, "opening pid file for logging: %s: %v", cwd, err) 
			f.Close()
			return
	    }
	    if _, err := pidF.WriteString(strconv.Itoa(cmd.Process.Pid) + " " + time.Now().Format("2006-01-02 15:04:05") + "\n"); err != nil {
			logAndErr(w, "writing pid: %s: %v", cwd, err) 
			f.Close()
			return
	    }
	    terminalSession := &TerminalSession{
	        Cmd: cmd,
	        ID: terminalID,
	        Pty: f,       
	    }
	    terminalMu.Lock()
	    terminalSessions[terminalID] = terminalSession
	    terminalMu.Unlock()
	    
	    // in a go func, continually read from the pty and write to buffer
	    go func() {
            for {
                log.Println("a loop!")
                // TODO: reuse buffer?
                b := make([]byte, 1024)
	    	    n, err := terminalSession.Pty.Read(b)
	    	    // if err != nil && err != io.EOF {
	    	    if err != nil {
	    	        terminalMu.Lock()
	    	        log.Printf("error reading terminal: %v", err) // could be just EOF
	    	        f.Close()
	    	        terminalSession.Closed = true
	    	        terminalCond.Broadcast()
	    	        terminalMu.Unlock()
	    	    }
	    	    if n == 0 {
	    	        continue
	    	    }
    	        terminalMu.Lock()
    	        terminalSession.ReadBuffer = append(terminalSession.ReadBuffer, b[0:n]...)
    	        log.Printf("<==========")
    	        log.Printf("%s", string(terminalSession.ReadBuffer))
    	        log.Printf("==========>")
    	        terminalCond.Broadcast()
    	        terminalMu.Unlock()
    	        // should we put this before the unlock?
	    	}
	    }()
	    
	    json.NewEncoder(w).Encode(map[string]interface{}{
	        "ID": terminalID,
	    })
	})
	mux.HandleFunc("/myterminalsend", func(w http.ResponseWriter, r *http.Request) {
	    // TODO: do consider an rwlock
	    // creak/pty example shows reading and writing in separate goroutines
	    terminalMu.Lock()    
	    defer terminalMu.Unlock()
	    
		ID, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			logAndErr(w, "invalid id: %s: %v", r.FormValue("id"), err) 
			return
		}
		
	    if t, ok := terminalSessions[ID]; ok {
	    	payloadBytes := []byte(r.FormValue("payload"))
	    	n, err := t.Pty.Write(payloadBytes)
	    	if err != nil {
				logAndErr(w, "wriring pty: %d: %v", ID, err) 
				return
	    	}
	    	if n != len(payloadBytes) {
				logAndErr(w, "wriring pty: not enough bytes written") 
				return
	    	}
	    }
	})
	mux.HandleFunc("/myterminalclose", func(w http.ResponseWriter, r *http.Request) {
	    terminalMu.Lock()    
	    defer terminalMu.Unlock()
	    
		ID, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			logAndErr(w, "invalid id: %s: %v", r.FormValue("id"), err) 
			return
		}
		
	    if t, ok := terminalSessions[ID]; ok {
	    	err := t.Pty.Close()
	    	if err != nil {
				logAndErr(w, "closing pty: %d: %v", ID, err) 
				return
	    	}
	    	delete(terminalSessions, ID)
	    }
	})
	
	// deprecated, see myterminalpoll
	mux.HandleFunc("/mybashstream", func(w http.ResponseWriter, r *http.Request) {
		cmdString := r.FormValue("cmd")
		if cmdString == "" {
			cmdString = ":"
		}
		cwd := r.FormValue("cwd") // current working directory
		cmdString = "cd " + cwd + ";\n" + cmdString + ";\necho ''; pwd"
		shellCommand := &ShellCommand{
		    CWD: cwd,
		    Command: cmdString,
		}
		
		cmd := exec.Command("bash", "-c", cmdString)
		stderr, err := cmd.StderrPipe()
		if err != nil {
			logAndErr(w, "getting stderr: %s: %v", cmdString, err) 
			return
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			logAndErr(w, "getting stdout: %s: %v", cmdString, err) 
			return
		}
		
		shellMu.Lock()
		shellIndex += 1
		shellCommands[shellIndex] = shellCommand
		shellMu.Unlock()
		
		err = cmd.Start()
		if err != nil {
			logAndErr(w, "starting command: %s: %v", cmdString, err) 
			return
		}
		
		
		// Docs say: It is thus incorrect to call Wait before all reads from the pipe have completed.
		// so we will read and then call wait
		
		// could have used channel
		var wg sync.WaitGroup
		go func() {
		    reader := bufio.NewReader(stdout)
		    wg.Add(1)
		    for {
		        line, err := reader.ReadString('\n')   
		        _ = line
		        if err != nil {
		            break
		        } 
		    }
		}()
		
		go func() {
		   _ = stderr   
		}()
		
		wg.Wait()
		
		err = cmd.Wait()
		if err != nil {
			logAndErr(w, "waiting for command: %s: %v", cmdString, err) 
			return
		}
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
			logAndErr(w, "error running command: %s: %v", cmdString, err) 
			return
		}
		//lines := strings.Split(string(r), "\n")

		log.Printf("the combined output of the command is: %s", string(ret))
		w.Write(ret)
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
	    os.Exit(1)    
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains("..", r.URL.Path) {
			logAndErr(w, "the path has a .. in it")
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
			// trimming off the :line suffix
			parts = strings.Split(filePath, ":")
			filePath = parts[0]
			fullPath := *location + "/" + filePath
			log.Printf("the full path is: %s", fullPath)
			fileInfo, err := os.Stat(fullPath)
			if err != nil {
				logAndErr(w, "error determining file type")
				return
			}
			md5String := ""
			isDir := false
			if fileInfo.IsDir() {
				isDir = true
				files, err := ioutil.ReadDir(fullPath)
				if err != nil {
					logAndErr(w, "could not read files: %v", err)
					return
				}
				fileNames := make([]string, len(files)+1)
				fileNames[0] = ".."
				for i, f := range files {
					fileNames[i+1] = f.Name()
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
					logAndErr(w, "error reading requested file: %v", err)
					return
				}
				c = c2


			    m := md5.New()
			    if _, err = m.Write(c); err != nil {
					logAndErr(w, "couldn't md5 file: %v", err)
			        return
			    }
			    md5String = fmt.Sprintf("%x", m.Sum(nil))
				w.Header().Set("X-MD5", md5String)


				if r.FormValue("raw") == "1" {
					w.Write(c)
					return
				}
			}
			log.Printf("is dir? %t", isDir)

			b, err := ioutil.ReadFile(*indexFile)
			if err != nil {
				logAndErr(w, "error reading index file: %v", err)
				return
			}
			htmlString := string(b)
			contentString := string(c)
			contentLines := strings.Split(contentString, "\n")
			contentLinesJSON, err := json.MarshalIndent(contentLines, "", " ")
			contentLinesJSONString := string(contentLinesJSON)

			if isDir {
				htmlString = strings.Replace(htmlString, "// FILEMODE DIRECTORY GOES HERE", "fileMode = \"directory\"", 1)
			} else {
				htmlString = strings.Replace(htmlString, "// FIRSTFILEMD5 GOES HERE", `var firstFileMD5 = "`+md5String+`"`, 1)
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
			theFilePath := *location+"/"+r.URL.Path[1:]
			content := ""
			diff := r.FormValue("diff")
			oldmd5 := r.FormValue("oldmd5")
			newmd5 := r.FormValue("newmd5")
			if diff != "" && oldmd5 != "" && newmd5 != "" {
			    oldBytes, err := ioutil.ReadFile(theFilePath)
			    if err != nil {
					logAndErr(w, "couldn't open file: %v", err)
			        return
			    }
			    oldH := md5.New()
			    if _, err = oldH.Write(oldBytes); err != nil {
					logAndErr(w, "couldn't md5 old bytes: %v", err)
			        return
			    }
			    expectedOldMD5 := fmt.Sprintf("%x", oldH.Sum(nil))
			    if expectedOldMD5 != oldmd5 {
					logAndErr(w, "couldn't hex old bytes: %s != %s", expectedOldMD5, oldmd5)
			    	return
			    } 
			    content, err = applyDiff(string(oldBytes), diff)
			    if err != nil {
					logAndErr(w, "couldn't apply diff: %v", err)
			    	return
			    }
			    newBytes := []byte(content)
			    newH := md5.New()
			    if _, err = newH.Write(newBytes); err != nil {
					logAndErr(w, "couldn't md5 new bytes: %v", err)
			        return
			    }
			    expectedNewMD5 := fmt.Sprintf("%x", newH.Sum(nil))
			    if expectedNewMD5 != newmd5 {
					logAndErr(w, "hash doesn't match: %v", err)
			    	return
			    } 
			} else {
				content = r.FormValue("content")
				// added this because once when I was traveling and
				// lost network connection while it was trying to save
				// it somehow saved an empty file. Partial request?
				if len(content) == 0 {
					logAndErr(w, "empty content: no content")
					return
				}
			}
			s := SaveResponse{}
			err := ioutil.WriteFile(theFilePath, []byte(content), 0644)
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
			log.Printf("original URL: %s =====", r.URL.Path)
			parts := strings.Split(r.URL.Path, ",")
			for i, part := range parts {
				if (i == 0 && !*proxyPathTrimmed) || i > 0 {
					part = strings.TrimPrefix(part, *proxyPath)
				}
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
		Addr:         *serverAddress,
		Handler:      redirectMux,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	httpsServer := &http.Server{
		Addr:         *serverAddress,
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

func logJSON(v interface{}) {
    b, err := json.MarshalIndent(v, "", "    ")    
    if err != nil {
        log.Printf("error logging json: %v", err)
    }
    log.Printf(string(b))
}
