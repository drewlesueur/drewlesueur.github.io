package main

import (
	"fmt"
	"github.com/creack/pty"
	"io"
	"os"
	"os/exec"
	"time"
)

func main() {
	c := exec.Command("redis-cli", "-h", "redis01.ue1-dev-portal.gpsinsight.com")
	f, err := pty.Start(c)
	if err != nil {
		panic(err)
	}

	go func() {
		// f.Write([]byte("info\r\n"))
		time.Sleep(2 * time.Second)
		fmt.Println("===first")
		f.Write([]byte("\r\n"))
		fmt.Println("===done first")
		time.Sleep(2 * time.Second)
		fmt.Println("===second")
		f.Write([]byte("set foo bar\r\n"))
		fmt.Println("===done second")
		time.Sleep(2 * time.Second)
		fmt.Println("===third")
		f.Write([]byte("get foo\r\n"))
		fmt.Println("===done third")
		// f.Write([]byte{4}) // EOT
	}()
	io.Copy(os.Stdout, f)

	// c := exec.Command("grep", "--color=auto", "bar")
	// f, err := pty.Start(c)
	// if err != nil {
	// 	panic(err)
	// }
	//
	// go func() {
	// 	f.Write([]byte("foo\n"))
	// 	f.Write([]byte("bar\n"))
	// 	f.Write([]byte("baz\n"))
	// 	f.Write([]byte{4}) // EOT
	// }()
	// io.Copy(os.Stdout, f)

}
