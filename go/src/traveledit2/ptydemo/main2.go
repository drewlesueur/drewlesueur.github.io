package main

import (
	"io"
	"log"
	"os"
	"os/exec"
	// "os/signal"
	// "syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

func test() error {
	// Create arbitrary command.
	c := exec.Command("bash")

	// Start the command with a pty.
	ptmx, err := pty.Start(c)
	if err != nil {
		return err
	}
	// Make sure to close the pty at the end.
	defer func() { _ = ptmx.Close() }() // Best effort.

	// Handle pty size.
	// ch := make(chan os.Signal, 1)
	// signal.Notify(ch, syscall.SIGWINCH)
	// go func() {
	//         for range ch {
	//                 if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
	//                         log.Printf("error resizing pty: %s", err)
	//                 }
	//         }
	// }()
	// ch <- syscall.SIGWINCH // Initial resize.

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	stdinFile, err := os.Create("./thestdin")
	if err != nil {
		panic(err)
	}
	stdoutFile, err := os.Create("./thestdout")
	if err != nil {
		panic(err)
	}
	teeIn := io.TeeReader(os.Stdin, stdinFile)
	teeOut := io.TeeReader(ptmx, stdoutFile)

	// Copy stdin to the pty and the pty to stdout.
	// go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	go func() { _, _ = io.Copy(ptmx, teeIn) }()
	_, _ = io.Copy(os.Stdout, teeOut)

	return nil
}

func main() {
	if err := test(); err != nil {
		log.Fatal(err)
	}
}
