package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glo0ml34f/grimux/internal/plugin"
	"github.com/glo0ml34f/grimux/internal/repl"
)

// version is set at build time using -ldflags. Default is "dev".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	serious := flag.Bool("serious", false, "start in serious mode")
	audit := flag.Bool("audit", false, "enable audit logging")
	pluginDir := flag.String("plugins", "", "plugins directory")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	repl.SetSeriousMode(*serious)
	repl.SetAuditMode(*audit)
	repl.SetVersion(version)
	home, _ := os.UserHomeDir()
	repl.SetBanFile(filepath.Join(home, ".grimux_banned"))
	if *pluginDir != "" {
		plugin.GetManager().SetDir(*pluginDir)
	}
	if flag.NArg() > 0 {
		repl.SetSessionFile(flag.Arg(0))
	}

	if err := repl.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
