package main

import "net/http"
import "net/http/httptest"
import "net/http/httputil"
import "net/url"
import "time"
import "log"
import "flag"
import "fmt"
import "strings"
import "os"
import "os/exec"
import "path"
import "crypto/subtle"
import "io/ioutil"
import "io"
import "encoding/json"
import "encoding/base64"
import "sync"
import "strconv"
import "crypto/md5"
import "html"
import "bytes"
import "github.com/NYTimes/gziphandler"

// import "github.com/gorilla/websocket"

type SaveResponse struct {
	Saved bool   `json:"saved"`
	Error string `json:"error"`
}

func PretendBasicAuth(r *http.Request) (string, string, bool) {
	cookie, err := r.Cookie("pretendba")
	if err != nil {
		return "", "", false
	}
	cookieDecoded, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return "", "", false
	}
	cookieBytes, err := base64.StdEncoding.DecodeString(cookieDecoded)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(string(cookieBytes), ":")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true

}
func BasicAuth(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := os.Getenv("BASICUSER")
		pass := os.Getenv("BASICPASS")
		if user == "" || pass == "" {
			log.Fatal("BASICUSER or BASICPASS environment variables not set")
		}
		if r.URL.Path == "/login" {
			handler.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/tepublic") {
			handler.ServeHTTP(w, r)
			return
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

		log.Printf("url hit: %s by %s (%s)", r.URL.Path, r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
		//rUser, rPass, ok := r.BasicAuth()
		rUser, rPass, ok := PretendBasicAuth(r)
		if !ok || subtle.ConstantTimeCompare([]byte(rUser), []byte(user)) != 1 || subtle.ConstantTimeCompare([]byte(rPass), []byte(pass)) != 1 {
			http.Redirect(w, r, *proxyPath+"/login", 302)
			return
			// below here is basic auth stuff
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
	for i := 0; i < 1000000; i++ {
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
			if lineI == nextDiffIndex-1 {
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
			} else if strings.HasPrefix(diffLines[diffI], "+") {
				newContentsSlice = append(newContentsSlice, diffLines[diffI][1:])
			} else if strings.HasPrefix(diffLines[diffI], "@@") {
				nextDiffIndex = parseFirstNumber(diffLines[diffI]) - 1
				state = "getToNextIndex"
			} else {
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

type HighlightMatch struct {
	Regex           string
	BackgroundColor string
	TextColor       string
	UnderlineColor  string
}

type HighlightRange struct {
	StartY int
	StartX int

	StopY int // inclusive
	StopX int // exclusive

	BackgroundColor string
	TextColor       string
	UnderlineColor  string
}

// Will these die when the server restarts?
// I think not.
// Interesting how we have different fields for separate File types
// Maybe I could have ised an interface
// But also maybe would be cool if Go had sum types
type File struct {
	ID         int
	Type       string // terminal, file, directory, remotefile, shell(semi interactive)
	FullPath   string
	LineNumber int

	// CSS color
	Color string
	Group string

	HighlightText   string // deprecated
	HighlightRanges []*HighlightRange

	// fields for remotefile
	LocalTmpPath string // temorary file
	Remote       string // like user@host

	// fields for shell
	CWD         string
	LastCommand string

	// fields for terminal
	Cmd        *exec.Cmd
	Pty        *os.File
	ReadBuffer []byte
	Closed     bool
	Name       string
}

type Workspace struct {
	Files     []*File
	Name      string
	DarkMode  bool
	FontName  string
	FontScale float64

	HighlightMatches []*HighlightMatch
}

func (w *Workspace) GetFile(id int) (*File, bool) {
	for _, f := range w.Files {
		if id == f.ID {
			return f, true
		}
	}
	return nil, false
}
func (w *Workspace) RemoveFile(id int) {
	for i, f := range w.Files {
		if id == f.ID {
			log.Printf("removed file: %d", id)
			// w.Files = append(w.Files[0:i], w.Files[i+1:]...)
			// https://github.com/golang/go/wiki/SliceTricks
			copy(w.Files[i:], w.Files[i+1:])
			w.Files[len(w.Files)-1] = nil
			w.Files = w.Files[0 : len(w.Files)-1]
			// I think even with the copy it won't shrink the original array size
			// I think we'd have to copy to a whole new slice for that
			// why
			break
		}
	}

	// fun global action
	// delete workspace if it's empty except for last one
	go func() {
		// funky, doing it delayed so the close/open flow for clickItemInDirectory
		// doesn't immediately close the workspace
		time.Sleep(3 * time.Second)
		workspaceMu.Lock()
		defer workspaceMu.Unlock()
		if len(workspace.Files) == 0 && len(workspaces) > 1 {
			for i, w2 := range workspaces {
				if w2 == w {
					copy(workspaces[i:], workspaces[i+1:])
					workspaces[len(workspaces)-1] = nil
					workspaces = workspaces[0 : len(workspaces)-1]
					break
				}
			}
		}
	}()
}

func writeWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceViews := []map[string]interface{}{}
	for _, w := range workspaces {
		workspaceViews = append(workspaceViews, workspaceView(w))
	}
	jsonBytes, err := json.MarshalIndent(workspaceViews, "", "    ")
	if err != nil {
		logAndErr(w, "marshalling for mysaveworkspace: %v", err)
		return
	}
	err = ioutil.WriteFile("workspaces.json", jsonBytes, 0644)
	if err != nil {
		logAndErr(w, "saving workspaces.json: %v", err)
		return
	}
}

// workspaceView is a function that returns a json marshallable version of a
// workspace for use in saving a file and in the front end
// we could maybe just serialize the raw workspace?
// or create a toJSON func? but this works
func workspaceView(w *Workspace) map[string]interface{} {
	// workspaceMu lock needs to be held when calling this function
	files := []map[string]interface{}{}
	for _, f := range w.Files {
		files = append(files, map[string]interface{}{
			"ID":              f.ID,
			"Name":            f.Name,
			"Type":            f.Type,
			"FullPath":        f.FullPath,
			"LineNumber":      f.LineNumber,
			"CWD":             f.CWD,
			"Color":           f.Color,
			"Group":           f.Group,
			"HighlightText":   f.HighlightText,
			"HighlightRanges": f.HighlightRanges,
		})
	}
	workspaceRet := map[string]interface{}{
		"Name":             w.Name,
		"DarkMode":         w.DarkMode,
		"FontName":         w.FontName,
		"FontScale":        w.FontScale,
		"HighlightMatches": w.HighlightMatches,
		"Files":            files,
	}
	return workspaceRet
}
func workspaceViewWithList(w *Workspace) map[string]interface{} {
	// workspaceMu lock needs to be held when calling this function
	workspacesList := []map[string]interface{}{}
	for _, w := range workspaces {
		workspacesList = append(workspacesList, map[string]interface{}{
			"Name": w.Name,
		})
	}
	// return workspaceRet
	return map[string]interface{}{
		"workspace":      workspaceView(w),
		"workspacesList": workspacesList,
	}
}
func runShellCommand(id string, cmdString string, cwd string, w http.ResponseWriter) {
	workspaceMu.Lock()

	ID, _ := strconv.Atoi(id)
	if cmdString == "" {
		cmdString = ":"
	}

	// add the cwd so the client can remember it
	cmdString = "cd " + cwd + ";\n" + cmdString + ";\necho ''; pwd"

	log.Printf("the command we want is: %s", cmdString)
	cmd := exec.Command("bash", "-c", cmdString)
	var f *File
	if ID == 0 {
		lastFileID++
		f = &File{
			Type: "shell",
			// FullPath: "(shell)/???",
			ID:  lastFileID,
			CWD: cwd,
		}
		workspace.Files = append(workspace.Files, f)
	} else if t, ok := workspace.GetFile(ID); ok {
		f = t
	} else {
		workspaceMu.Unlock()
		logAndErr(w, "no bash session found: %d", ID)
		return
	}
	// log.Printf("the file is %+v", f)
	// curious this case?
	if f.Cmd != nil && f.Cmd.Process != nil {
		// close the last process if there is one
		f.Cmd.Process.Kill()
	}
	f.Cmd = cmd
	workspaceMu.Unlock()

	ret, err := cmd.CombinedOutput()
	if err != nil {
		logAndErr(w, "error running command: %s: %v", cmdString, err)
		return
	}

	lines := strings.Split(string(ret), "\n")
	if len(lines) >= 2 {
		workspaceMu.Lock()
		f.CWD = lines[len(lines)-2]
		workspaceMu.Unlock()
	}

	log.Printf("the combined output of the command is: %s", string(ret))
	if ID == 0 {
		w.Header().Set("X-ID", strconv.Itoa(f.ID))
	}
	w.Write(ret)
}

var workspaces []*Workspace
var workspace *Workspace

type TerminalResponse struct {
	Base64 string
	// CWD ?? so we can keep track of directory changes
	Error  string `json:",omitempty"`
	Closed bool   `json:",omitempty"`
}

var lastFileID = 0
var workspaceMu sync.Mutex
var workspaceCond *sync.Cond

var proxyPath *string

func main() {
	workspaceCond = sync.NewCond(&workspaceMu)
	// TODO: #wschange save workspace to file so ot persists
	// TODO: secial path prefix for saving/loading files not just /

	// read in the existing workspaces
	workspacesJSON, err := ioutil.ReadFile("./workspaces.json")
	if err != nil {
		log.Printf("could not read workspaces.json: %v", err)
	} else {
		var tmpWorkspaces []*Workspace
		err := json.Unmarshal(workspacesJSON, &tmpWorkspaces)
		if err != nil {
			log.Printf("could not parse workspaces.json: %v", err)
		} else {
			// reload the workspace
			for _, tmpW := range tmpWorkspaces {
				workspace = &Workspace{
					FontScale:        tmpW.FontScale,
					FontName:         tmpW.FontName,
					DarkMode:         tmpW.DarkMode,
					Name:             tmpW.Name,
					HighlightMatches: tmpW.HighlightMatches,
				}
				for _, f := range tmpW.Files {
					if f.Type == "file" {
						// TODO: I think you might not be taking into account *location
						// maybe I shoulf remove that feature and always make it /

						// also for the addFile portion you might just be able to set thr file
						// instrad of calling addFile
						// it's the shell and terminal types that need to start a process
						addFile("", "file", f.FullPath)
					} else if f.Type == "directory" {
						addFile("", "directory", f.FullPath)
					} else if f.Type == "iframe" {
						addFile("", "iframe", f.FullPath)
					} else if f.Type == "terminal" {
						openTerminal(f.CWD, httptest.NewRecorder()) // being lazy with ResponseRecorder for now
					} else if f.Type == "shell" {
						runShellCommand("", "", f.CWD, httptest.NewRecorder()) // being lazy with ResponseRecorder for now
					}
					// update the editable props too
					// the way I am doing it here is a little kludgy
					// sort of retrofitting the existing code to recreate the files.
					// (See httptest.NewRecorder for example)
					addedFile := workspace.Files[len(workspace.Files)-1]
					addedFile.LineNumber = f.LineNumber
					addedFile.Name = f.Name
					addedFile.Color = f.Color
					addedFile.Group = f.Group
					addedFile.HighlightText = f.HighlightText
					addedFile.HighlightRanges = f.HighlightRanges
				}

				workspaces = append(workspaces, workspace)
			}

			// TODO: you could remember the lst workspace
			if len(workspaces) > 0 {
				workspace = workspaces[0]
			}
		}
	}

	if workspace == nil {
		workspace = &Workspace{Name: "default"}
		workspaces = []*Workspace{workspace}
		addFile("", "directory", "/")
	}
	serverAddress := flag.String("addr", "localhost:8000", "serverAddress to listen on")
	indexFile := flag.String("indexfile", "./public/index.html", "path to index html file")
	screenshareFile := flag.String("screensharefile", "./public/view.html", "path to view html file")
	loginFile := flag.String("loginFile", "./public/login.html", "path to login  html file")
	location := flag.String("location", "", "path to directory to serve")
	proxyPath = flag.String("proxypath", "", "the path for proxies, what to ignore")

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
	// Simple case only allows 1 proxy!
	allowedXForwardedForsStr := os.Getenv("ALLOWEDXFORWARDEDFORS")
	allowedXForwardedFors := strings.Split(allowedXForwardedForsStr, ",")
	allowedXForwardedForsMap := map[string]bool{}
	for _, ip := range allowedXForwardedFors {
		if ip != "" {
			allowedXForwardedForsMap[ip] = true
		}
	}
	certFile := os.Getenv("CERTFILE")
	keyFile := os.Getenv("KEYFILE")
	flag.Parse()
	log.Printf("certFile: %s", certFile)
	log.Printf("keyfile: %s", keyFile)
	log.Printf("proxyPath is: %s", *proxyPath)

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

	// trying to use a single mutex for multiple shells?
	// TODO: serialize and de-serialize the state

    // 📖📖📖📖📖📖📖📖📖📖📖📖
    // single global mutex for everything.
    var pollerMu sync.Mutex
    var pollerRequestID = 0
    pollerCond := sync.NewCond(&pollerMu)
    var requestsForPolling = map[string][]*PolledRequest{}
    var responsesForPolling = map[string]*PolledResponse{}
	go func() {
		for range time.NewTicker(1 * time.Second).C {
			viewCond.Broadcast()
			pollerCond.Broadcast()
			workspaceCond.Broadcast()
		}
	}()

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./public"))
	mux.Handle("/tepublic/", http.StripPrefix("/tepublic/", fs))

	publicPath2 := os.Getenv("PUBLICPATH")
	if publicPath2 != "" {
		fs2 := http.FileServer(http.Dir(publicPath2))
		mux.Handle("/tepublic2/", http.StripPrefix("/tepublic2/", fs2))
	}

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
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		// http.ServeFile(w, r, "./public/view.html")
		b, err := ioutil.ReadFile(*loginFile)
		if err != nil {
			logAndErr(w, "error reading login file: %v", err)
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
	// but still cool to see the code, and we might add it back
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
			// newF, err = os.Create("./uploads/" + fh.Filename)
			newF, err = os.Create(r.FormValue("thedirectory") + "/" + fh.Filename)
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

	// #wschange myterminalname
	mux.HandleFunc("/myname", func(w http.ResponseWriter, r *http.Request) {
		// load existing terminal sessions.
		workspaceMu.Lock()
		defer workspaceMu.Unlock()
		idStr := r.FormValue("id")
		name := r.FormValue("name")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			logAndErr(w, "invalid terminal id")
			return
		}
		t, ok := workspace.GetFile(id)
		if !ok {
			logAndErr(w, "not found")
			return
		}
		t.Name = name
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})
	})

	mux.HandleFunc("/mycolor", func(w http.ResponseWriter, r *http.Request) {
		// load existing terminal sessions.
		workspaceMu.Lock()
		defer workspaceMu.Unlock()
		idStr := r.FormValue("id")
		color := r.FormValue("color")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			logAndErr(w, "invalid terminal id")
			return
		}
		t, ok := workspace.GetFile(id)
		if !ok {
			logAndErr(w, "not found")
			return
		}
		t.Color = color
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})
	})

	// #wschange this replaced myterminals, now is an array not map
	mux.HandleFunc("/myWorkspaceWithList", func(w http.ResponseWriter, r *http.Request) {
		workspaceMu.Lock()
		defer workspaceMu.Unlock()
		indexStr := r.FormValue("index")

		for {
			if indexStr != "" {
				if indexStr == "new" {
					log.Println("========= ok doing a new one")
					workspace = &Workspace{Name: "workspace " + strconv.Itoa(len(workspaces)+1)}
					workspaces = append(workspaces, workspace)
					addFile("", "directory", "/")
					writeWorkspaceFile(w, r)
					break
				}
				index, err := strconv.Atoi(indexStr)
				if err != nil {
					logAndErr(w, "parsing index for myWorkspaceWithList: %v", err)
					return
				}
				if index >= len(workspaces) {
					logAndErr(w, "incorrect index: %d", index)
					return
				}
				workspace = workspaces[index]
			}

			break
		}
		workspaceWithListRet := workspaceViewWithList(workspace)
		json.NewEncoder(w).Encode(workspaceWithListRet)
	})

	// #wschange this replaced myterminals, now is an array not map
	mux.HandleFunc("/mysaveworkspace", func(w http.ResponseWriter, r *http.Request) {
		workspaceMu.Lock()
		defer workspaceMu.Unlock()
		tmpWorkspace := Workspace{}
		err := json.NewDecoder(r.Body).Decode(&tmpWorkspace)
		if err != nil {
			logAndErr(w, "parsing for mysaveworkspace: %v", err)
			return
		}

		// TODO #workspaceids
		workspaceNameToCheck := tmpWorkspace.Name
		if r.FormValue("oldWorkspaceName") != "" {
			workspaceNameToCheck = r.FormValue("oldWorkspaceName")
		}
		if workspaceNameToCheck != workspace.Name {
			logAndErr(w, "preventing workspace clash: %s, %s", workspaceNameToCheck, workspace.Name)
			return
		}
		filesByID := map[int]*File{}
		for _, f := range workspace.Files {
			filesByID[f.ID] = f
		}

		newFiles := []*File{}
		for _, fc := range tmpWorkspace.Files {
			if f, ok := filesByID[fc.ID]; ok {
				delete(filesByID, fc.ID)
				// Let's update the editable foelds while we are at it.
				f.LineNumber = fc.LineNumber
				f.Name = fc.Name
				f.Color = fc.Color
				f.Group = fc.Group
				f.HighlightText = fc.HighlightText
				f.HighlightRanges = fc.HighlightRanges
				newFiles = append(newFiles, f)
			}
		}
		// if we missed any add them at the end
		for _, f := range filesByID {
			newFiles = append(newFiles, f)
		}
		workspace.Files = newFiles
		workspace.DarkMode = tmpWorkspace.DarkMode
		workspace.FontName = tmpWorkspace.FontName
		workspace.FontScale = tmpWorkspace.FontScale
		workspace.HighlightMatches = tmpWorkspace.HighlightMatches
		workspace.Name = tmpWorkspace.Name
		writeWorkspaceFile(w, r)
	})
	mux.HandleFunc("/myterminalpoll", func(w http.ResponseWriter, r *http.Request) {
		workspaceMu.Lock()
		defer workspaceMu.Unlock()
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
			for _, t := range workspace.Files {
				// only "terminal" files will have a ReadBuffer
				if len(t.ReadBuffer) > 0 {
					break WaitLoop
				}
			}
			workspaceCond.Wait()
			// log.Println("done waiting")
		}

		if !timedOut {
			for _, t := range workspace.Files {
				tResp := TerminalResponse{}
				if t.Closed {
					// we only delete it after the client gets it
					// maybe have a timeout and cleanup later?
					// or actually maybe delete it right away when it's closed
					// and then keepntrack of closed ids to send?
					workspace.RemoveFile(t.ID)
				} else {
					if len(t.ReadBuffer) == 0 {
						continue
					}
				}
				tResp.Base64 = base64.StdEncoding.EncodeToString(t.ReadBuffer)
				tResp.Closed = t.Closed
				t.ReadBuffer = []byte{}
				ret[t.ID] = tResp
			}
		}
		json.NewEncoder(w).Encode(ret)
	})
	mux.HandleFunc("/myterminalopen", func(w http.ResponseWriter, r *http.Request) {
		cwd := r.FormValue("cwd")
		openTerminal(cwd, w)
	})
	mux.HandleFunc("/myterminalsend", func(w http.ResponseWriter, r *http.Request) {
		// TODO: do consider an rwlock
		// creak/pty example shows reading and writing in separate goroutines
		workspaceMu.Lock()
		defer workspaceMu.Unlock()

		ID, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			logAndErr(w, "invalid id: %s: %v", r.FormValue("id"), err)
			return
		}

		if f, ok := workspace.GetFile(ID); ok {
			payloadBytes := []byte(r.FormValue("payload"))
			n, err := f.Pty.Write(payloadBytes)
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

	mux.HandleFunc("/myaddfile", func(w http.ResponseWriter, r *http.Request) {
		// only used for iframe for now, other typed handled their own way
		workspaceMu.Lock()
		defer workspaceMu.Unlock()
		newID := addFile("", r.FormValue("fileType"), r.FormValue("fullPath"))
		w.Header().Set("X-ID", strconv.Itoa(newID))
	})
	// #wschange myterminalclose
	mux.HandleFunc("/myclose", func(w http.ResponseWriter, r *http.Request) {
		workspaceMu.Lock()
		defer workspaceMu.Unlock()

		ID, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			logAndErr(w, "invalid id: %s: %v", r.FormValue("id"), err)
			return
		}

		if t, ok := workspace.GetFile(ID); ok {
			workspace.RemoveFile(ID)

			if t.Type == "shell" {
				err := t.Cmd.Process.Kill()
				if err != nil {
					logAndErr(w, "closing pty: %d: %v", ID, err)
					return
				}
				return
			}

			// TODO remotefile
			if t.Type == "terminal" {
				err := t.Pty.Close()
				if err != nil {
					logAndErr(w, "closing pty: %d: %v", ID, err)
					return
				}
			}
		}
	})

	mux.HandleFunc("/myquickshell", func(w http.ResponseWriter, r *http.Request) {
		cwd := r.FormValue("cwd") // current working directory
		cmdString := r.FormValue("cmd")
		cmd := exec.Command("bash", "-c", cmdString)
		cmd.Dir = cwd
		ret, err := cmd.CombinedOutput()
		if err != nil {
			logAndErr(w, "myquickshell error running command: %s: %v", cmdString, err)
			return
		}
		w.Write(ret)
	})
	// #wschange make a File and add the cmd, and the CWD
	mux.HandleFunc("/myshell", func(w http.ResponseWriter, r *http.Request) {
		runShellCommand(r.FormValue("id"), r.FormValue("cmd"), r.FormValue("cwd"), w)
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		os.Exit(1)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Yay, index was hit")
		// TODO #wschange: you could hydrate the original files list.
        var usedIndexFile = *indexFile
        if r.FormValue("indexFile") != "" {
            usedIndexFile = r.FormValue("indexFile")
        }
		b, err := ioutil.ReadFile(usedIndexFile)
		if err != nil {
			logAndErr(w, "error reading index file: %v", err)
			return
		}
		htmlString := string(b)
		// contentString := string(c)
		// contentLines := strings.Split(contentString, "\n")
		// contentLinesJSON, err := json.MarshalIndent(contentLines, "", " ")
		// contentLinesJSONString := string(contentLinesJSON)

		// if isDir {
		// 	htmlString = strings.Replace(htmlString, "// FILEMODE DIRECTORY GOES HERE", "fileMode = \"directory\"", 1)
		// } else {
		// 	htmlString = strings.Replace(htmlString, "// FIRSTFILEMD5 GOES HERE", `var firstFileMD5 = "`+md5String+`"`, 1)
		// }
		// htmlString = strings.Replace(htmlString, "// ROOTLOCATION GOES HERE", "var rootLocation = \""+*location+"\"", 1)

		if *proxyPath != "" {
			replaceProxyPath := "var proxyPath = \"" + *proxyPath + "\""
			htmlString = strings.Replace(htmlString, "// PROXYPATH GOES HERE", replaceProxyPath, 1)
			log.Printf("replaceProxyPath: %s", replaceProxyPath)

			var replaceIsGitBash string
			if os.Getenv("ISGITBASH") == "1" {
				replaceIsGitBash = "var isGitBash = true"
			} else {
				replaceIsGitBash = "var isGitBash = false"
			}
			htmlString = strings.Replace(htmlString, "// ISGITBASH GOES HERE", replaceIsGitBash, 1)
		}

		// This content lines has to be the last one.
		// htmlString = strings.Replace(htmlString, "// LINES GO HERE", "var lines = "+contentLinesJSONString, 1)

		// TODO: when shell mode is disabled, don't do this part.
		log.Printf("yea I set rootLocation to be: %s", *location)
		if r.FormValue("src") != "1" {
			w.Header().Set("Content-Type", "text/html")
		}

		// save the file to list of files
		// addFile(r, isDir, fullPath)
		ioutil.WriteFile("tmp", []byte(htmlString), 0777)
		fmt.Fprintf(w, "%s", htmlString)
	})
	mux.HandleFunc("/duplfile", func(w http.ResponseWriter, r *http.Request) {
		ID, _ := strconv.Atoi(r.FormValue("id"))
		IDToDup, _ := strconv.Atoi(r.FormValue("idtodup"))
		workspaceMu.Lock()
		defer workspaceMu.Unlock()
		f, _ := workspace.GetFile(IDToDup)
		if f == nil {
			return
		}
		f2 := *f // copy

		if ID == 0 {
			lastFileID++
			f2.ID = lastFileID
			w.Header().Set("X-ID", strconv.Itoa(f2.ID))
			workspace.Files = append(workspace.Files, &f2)
		} else {
		}
		// add to end for now
	})
	// mux.HandleFunc("/links", func(w http.ResponseWriter, r *http.Request) {
	// 	linksTextBytes, err := ioutil.ReadFile("./links.txt")
	// 	if err != nil {
	// 		logAndErr(w, "couldn't get links file.")
	// 		return
	// 	}
	// 	linksTextString := string(linksTextBytes)
	// 	// not updating r.URL.RawPath.
	// 	// nor r.RequestURI
	//
	//     // /links/device_type_audit
	//
	// })

	mux.HandleFunc("/saveload", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains("..", r.URL.Path) {
			logAndErr(w, "the path has a .. in it")
			return
		}
		if r.Method == "GET" {
			log.Printf("===id is %s", r.FormValue("id"))
			var c []byte

			thePath := r.FormValue("fullpath")
			// trimming off the :line suffix
			parts := strings.Split(thePath, ":")
			thePath = parts[0]
			fullPath := combinePath(*location, thePath)
			log.Printf("the full path is: %s", fullPath)
			fileInfo, err := os.Stat(fullPath)
			if err != nil {
				logAndErr(w, "error determining file type")
				return
			}
			md5String := ""
			fileType := "file"
			if fileInfo.IsDir() {
				fileType = "directory"
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

				if r.FormValue("raw") == "1" || r.FormValue("browser") == "1" {
					// TODO: get rid of noid at some point
					if r.FormValue("noid") != "1" && r.FormValue("browser") != "1" {
						workspaceMu.Lock()
						newID := addFile(r.FormValue("id"), fileType, fullPath)
						workspaceMu.Unlock()
						if newID != 0 {
							w.Header().Set("X-ID", strconv.Itoa(newID))
						}
					}

					if r.FormValue("browser") == "1" {
						browserLines := []string{
							`<!doctype html><ul>`,
						}
						browserLines = append(browserLines, `<h1>`)
						browserLines = append(browserLines, thePath)
						browserLines = append(browserLines, `</h1>`)
						browserLines = append(browserLines, `<h2>`)
						browserLines = append(browserLines, `<a href="`+*proxyPath+`/saveload?browser=1&fullpath=`+html.EscapeString(url.QueryEscape(path.Dir(thePath)))+`">up</a>`)
						browserLines = append(browserLines, `</h2>`)
						for _, f := range files {
							// TODO: some escaping issues
							browserLines = append(browserLines, `<li><a href="`+*proxyPath+`/saveload?browser=1&fullpath=`+html.EscapeString(url.QueryEscape(thePath))+`/`+f.Name()+`">`+html.EscapeString(f.Name())+`</a></li>`)
						}
						browserLines = append(browserLines, `</ul>`)
						w.Write([]byte(strings.Join(browserLines, "\n")))
					} else {
						w.Write([]byte(strings.Join(fileNames, "\n")))
					}
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

				if r.FormValue("raw") == "1" || r.FormValue("browser") == "1" {
					// TODO: get rid of noid at some point
					if r.FormValue("noid") != "1" && r.FormValue("browser") != "1" {
						workspaceMu.Lock()
						newID := addFile(r.FormValue("id"), fileType, fullPath)
						workspaceMu.Unlock()
						if newID != 0 {
							w.Header().Set("X-ID", strconv.Itoa(newID))
						}
					}
					if r.FormValue("download") == "1" {
						parts := strings.Split(r.FormValue("fullpath"), "/")
						theName := parts[len(parts)-1]
						w.Header().Set("Content-Type", `text/plain`)
						w.Header().Set("Content-Disposition", `attachment; filename="`+theName+`"`)
					} else {
						parts := strings.Split(r.FormValue("fullpath"), "/")
						theName := parts[len(parts)-1]
						w.Header().Set("Content-Type", GetContentType(r.FormValue("fullpath")))
						// w.Header().Set("Content-Disposition", "inline;filename=myfile.pdf")
						w.Header().Set("Content-Disposition", `inline; filename="`+theName+`"`)
					}
					w.Write(c)
					return
				}
			}

		} else if r.Method == "POST" {
			thePath := r.FormValue("fullpath")
			theFilePath := combinePath(*location, thePath)
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
					logAndErr(w, "hash doesn't match: expected: %s, actual: %s", expectedNewMD5, newmd5)
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

	langServerURL, _ := url.Parse("http://localhost:12345/")
	proxyToLangServer := &httputil.ReverseProxy{Director: func(r *http.Request) {
		r.URL.Host = langServerURL.Host
		r.URL.Scheme = "http"
	}}
	mux.HandleFunc("/mylangserver", func(w http.ResponseWriter, r *http.Request) {
		proxyToLangServer.ServeHTTP(w, r)
	})
	mux.HandleFunc("/pollerResponse", func(w http.ResponseWriter, r *http.Request) {
	})
	mux.HandleFunc("/pollForRequests", func(w http.ResponseWriter, r *http.Request) {
	    // 👂👂👂👂👂👂
	    pollerName := r.FormValue("poller_name")
	    pollerMu.Lock()
	    defer pollerMu.Unlock()
		startWait := time.Now()
		var prs = []*PolledRequest{}
		for {
			if time.Since(startWait) > (10 * time.Second) {
				fmt.Fprintf(w, "%s", "{}")
				return
			}
	    	prs = requestsForPolling[pollerName]
			if len(prs) > 0 {
				break
			}
			pollerCond.Wait()
		}
	    pr := prs[0]
	    // prs = prs[1:]
	    // shift
	    copy(prs, prs[1:])
	    prs = prs[0:len(prs)-1]
		requestsForPolling[pollerName] = prs
	    // TODO: underlying array stays large, you could trim it at some point?
	    json.NewEncoder(w).Encode(pr)
	})

	var mainMux http.Handler = mux
	if os.Getenv("NOGZIP") != "1" {
		mainMux = gziphandler.GzipHandler(mux)
	}
	
	if os.Getenv("NOBASICAUTH") == "" {
		mainMux = BasicAuth(mainMux)
		log.Printf("doing basic auth")
	} else {
		log.Printf("Not doing basic auth")
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
					fmt.Fprintf(w, "%s", ipParts[0])
					return
				}
			}
			oldMainMux.ServeHTTP(w, r)
		})
	}

	if len(allowedXForwardedForsMap) > 0 {
		oldMainMux := mainMux
		mainMux = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ipParts := strings.Split(r.Header.Get("X-Forwarded-For"), ":")
			for i := 0; i < 1; i++ {
				if len(allowedXForwardedForsMap) == 0 {
					break
				}
				if len(ipParts) == 0 {
					return
				}
				if _, ok := allowedXForwardedForsMap[ipParts[0]]; !ok {
					log.Printf("unalowed ip: %s", ipParts[0])
					fmt.Fprintf(w, "%s", ipParts[0])
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

	if os.Getenv("POLLERPROXYSERVER") != "" {
		pollForRequests(mainMux)
		return
	}
	
	// Doing the polling handling after we send thr mainMux to pollForRequests
	// so that we don't get into infinite loop.
	oldMainMux := mainMux
	// 🙊🙊🙊🙊🙊🙊🙊🙊
	mainMux = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pollerName := r.Header.Get("X-Poller-Name")
		if pollerName == "" {
			oldMainMux.ServeHTTP(w, r)
			return
		}
		pollerRequestID++
		requestIDString := fmt.Sprintf("%d", pollerRequestID)
		rBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logAndErr(w, "couldn't open file: %v", err)
			return
		}
        polledRequest := &PolledRequest{
            RequestID: requestIDString,
            Method: r.Method,
            URL: r.RequestURI,
            Header: r.Header,
            Body: rBody,
        }
		pollerMu.Lock()
        requestsForPolling[pollerName] = append(requestsForPolling[pollerName], polledRequest)
		pollerCond.Broadcast()
		pollerMu.Unlock()
		// dead zone. need to unlock so that pollForRequests endpoint can get it and return it
		pollerMu.Lock()
		defer pollerMu.Unlock()
		startWait := time.Now()
		var polledResponse *PolledResponse
		for {
			if time.Since(startWait) > (10 * time.Second) {
 	 			w.WriteHeader(504)
				fmt.Fprintf(w, "%s", "{}")
				return
			}
	 	 	polledResponse = responsesForPolling[requestIDString]
	 	 	if polledResponse != nil {
	 	 	    break
	 	 	}
			pollerCond.Wait()
		}
 	 	delete(responsesForPolling, requestIDString)
 	 	
 	 	w.WriteHeader(polledResponse.StatusCode)
 	 	// add headers
 	 	for k, v := range polledResponse.Header {
 	 	    w.Header().Set(k, v[0])
 	 	}
 	 	w.Write(polledResponse.Body)
 	 	
	})
	

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


// ♠️♠️♠️♠️♠️♠️♠️
type PolledRequest struct {
	RequestID string
	Method string
	URL    string
	Header map[string][]string
	Body   []byte
}
// ♦️♦️♦️♦️♦️♦️♦️
type PolledResponse struct {
	PollerName string
	RequestID string
	StatusCode int
	Header    map[string][]string
	Body      []byte
}
// 📞📞📞📞📞📞☎️☎️☎️
func pollForRequests(mainMux http.Handler) {
	minWait := 1000 * time.Millisecond
	lastPoll := time.Now()
	httpClient := http.Client{
		Timeout: 30 * time.Second,
	}
	pollerProxyServer := os.Getenv("POLLERPROXYSERVER")
	pollerName := os.Getenv("POLLERNAME")
	for {
		timeSinceLastPoll := time.Since(lastPoll)
		if timeSinceLastPoll < minWait {
			time.Sleep(time.Duration(minWait.Milliseconds() - timeSinceLastPoll.Milliseconds()) * time.Millisecond)
		}
		log.Println("polling for requests")
		req, err  := http.NewRequest("GET", pollerProxyServer+"/pollForRequests?poller_name="+url.QueryEscape(pollerName), nil)
		if err != nil {
			log.Println("error creating request to poll: %v", err)
			continue
		}
		res, err := httpClient.Do(req)
		if err != nil {
			log.Println("error polling for requests: %v", err)
			continue
		}
		defer res.Body.Close()
		var pr PolledRequest
		// NOTE: we could pick a more optimal serialization format.
		// I think the bytes is base64 encoded.
		err = json.NewDecoder(res.Body).Decode(&pr)
		if err != nil {
			log.Println("error parsing polled request: %v", err)
			continue
		}
		// quick check for empty
		if pr.Method == "" {
		    // likely because of timeout, meaning we didn't get request
		    continue
		}
		go func(pr PolledRequest) {
			w := httptest.NewRecorder()
			r, err := http.NewRequest(pr.Method, pr.URL, bytes.NewReader(pr.Body))
			if err != nil {
				log.Println("error making local request object: %v", err)
				return
			}
			r.Header = http.Header(pr.Header)
			mainMux.ServeHTTP(w, r)

			resp := w.Result()
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println("error reading body of ResponseRecorder: %v", err)
				return
			}
			resp.Body.Close()
			pResp := PolledResponse{
				RequestID: pr.RequestID,
				PollerName: pollerName,
				StatusCode: resp.StatusCode,
				Header:     resp.Header,
				Body:       bodyBytes,
			}
			var writeBuf bytes.Buffer
			json.NewEncoder(&writeBuf).Encode(pResp)
			if err != nil {
				log.Println("error encoding json for poller response  %v", err)
				return
			}
			req, err := http.NewRequest("POST", pollerProxyServer+"/pollerResponse", &writeBuf)
			if err != nil {
				log.Println("error making request for response: %v", err)
				return
			}
			finalResponse, err := httpClient.Do(req)
			if err != nil {
				log.Println("error reading body of final response: %v", err)
				return
			}
			defer finalResponse.Body.Close()
			// not even reading this final response
		}(pr)

	}
}

func addFile(id string, fileType string, fullPath string) int {
	if id == "" {
		lastFileID++
		f := &File{
			FullPath: fullPath,
			ID:       lastFileID,
			Type:     fileType,
		}
		workspace.Files = append(workspace.Files, f)
		return f.ID
	}
	return 0
}

func logJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		log.Printf("error logging json: %v", err)
	}
	log.Printf(string(b))
}

func combinePath(a, b string) string {
	if !strings.HasSuffix(a, "/") {
		a = a + "/"
	}
	if strings.HasPrefix(b, "/") {
		b = b[1:]
	}

	return a + b
}

var extensionsToMime = map[string]string{
	"html": "text/html",
	"txt":  "text/plain",
	"js":   "text/javascript",
	"json": "application/json",
	"css":  "text/css",
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"gif":  "image/gif",
	"svg":  "image/svg+xml",
	"pdf":  "application/pdf",
}

func GetContentType(thePath string) string {
	var mime string
	var ok bool
	for {
		parts := strings.Split(thePath, ".")
		if len(parts) == 1 {
			mime = "text/plain"
			break
		}
		theExtension := parts[len(parts)-1]
		mime, ok = extensionsToMime[strings.ToLower(theExtension)]
		if !ok {
			mime = "text/plain"
		}
		break
	}
	return mime + ";charset=utf-8"
}
