package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/grimux/internal/repl"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	serious := flag.Bool("serious", false, "start in serious mode")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	repl.SetSeriousMode(*serious)
	home, _ := os.UserHomeDir()
	repl.SetBanFile(filepath.Join(home, ".grimux_banned"))
	if flag.NArg() > 0 {
		repl.SetSessionFile(flag.Arg(0))
	}

	if err := repl.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
