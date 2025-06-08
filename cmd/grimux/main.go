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
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	if flag.NArg() > 0 {
		repl.SetSessionFile(flag.Arg(0))
	}

	if err := repl.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
