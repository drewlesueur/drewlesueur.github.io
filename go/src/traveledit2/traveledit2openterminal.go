//go:build !windows
package main

import (
    "net/http"
    "strconv"
    "time"
    "os"
    "os/exec"
    "encoding/json"
    "log"
    "github.com/creack/pty"
)

func openTerminal(cwd string, w http.ResponseWriter) {
	log.Println("my terminal open!")
	lastFileID++
	// TODO: configurable shell, login shell (-l)?
	cmd := exec.Command("bash", "-l")
	// cmd := exec.Command("bash")
	// cmd := exec.Command("zsh", "-l")
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
	file := &File{
		Type: "terminal",
		// FullPath: "(terminal)/???",
		Cmd: cmd,
		ID:  lastFileID,
		CWD: cwd,
		Pty: f,
	}
	workspaceMu.Lock()
	workspace.Files = append(workspace.Files, file)
	workspaceMu.Unlock()

	// in a go func, continually read from the pty and write to buffer
	go func() {
		for {
			log.Println("a loop!")
			// TODO: reuse buffer?
			b := make([]byte, 1024)
			n, err := file.Pty.Read(b)
			// if err != nil && err != io.EOF {
			if err != nil {
				workspaceMu.Lock()
				log.Printf("error reading terminal: %v", err) // could be just EOF
				file.Pty.Close()
				file.Closed = true
				workspaceCond.Broadcast()
				workspaceMu.Unlock()
				break
			}
			if n == 0 {
				continue
			}
			workspaceMu.Lock()
			file.ReadBuffer = append(file.ReadBuffer, b[0:n]...)

			// little protection from runaway
			if len(file.ReadBuffer) > 5000000 {
				file.ReadBuffer = nil
			}
			log.Printf("<==========")
			log.Printf("%s", string(file.ReadBuffer))
			log.Printf("==========>")
			workspaceCond.Broadcast()
			workspaceMu.Unlock()
			// should we put this before the unlock?
		}
	}()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ID": lastFileID,
	})
}
