package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/glo0ml34f/grimux/internal/repl"
)

var version = getVersion()

func getVersion() string {
	cmd := exec.Command("git", "tag", "--points-at", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "dev"
	}
	tags := strings.Fields(string(out))
	for _, t := range tags {
		if strings.HasPrefix(t, "v") && strings.Count(t, ".") == 1 {
			return t
		}
	}
	return "dev"
}

func main() {
	showVersion := flag.Bool("version", false, "print version")
	serious := flag.Bool("serious", false, "start in serious mode")
	audit := flag.Bool("audit", false, "enable audit logging")
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
	if flag.NArg() > 0 {
		repl.SetSessionFile(flag.Arg(0))
	}

	if err := repl.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
