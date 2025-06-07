package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/example/grimux/internal/tmux"
)

func main() {
	target := flag.String("capture", "", "tmux pane to capture")
	verbose := flag.Bool("verbose", false, "enable verbose logging")
	flag.Parse()

	tmux.Verbose = *verbose

	if *target != "" {
		content, err := tmux.CapturePane(*target)
		if err != nil {
			fmt.Fprintln(os.Stderr, "capture error:", err)
			os.Exit(1)
		}
		fmt.Print(content)
		return
	}

	flag.Usage()
}
