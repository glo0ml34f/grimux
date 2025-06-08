package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/example/grimux/internal/repl"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	sessionPath := flag.String("session", "", "session file to load")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	if *sessionPath != "" {
		repl.SetSessionFile(*sessionPath)
	}

	if err := repl.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
