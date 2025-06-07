package tmux

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

// Verbose controls whether debug logging is enabled.
var Verbose bool

func debugf(format string, args ...interface{}) {
	if Verbose {
		log.Printf(format, args...)
	}
}

// CapturePane connects to the tmux server over its UNIX socket and captures the
// contents of the specified pane. If target is empty, the current pane is
// captured.
func CapturePane(target string) (string, error) {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return "", errors.New("TMUX environment variable is not set")
	}

	socket := strings.Split(tmuxEnv, ",")[0]
	debugf("using tmux socket: %s", socket)
	if _, err := os.Stat(socket); err != nil {
		return "", fmt.Errorf("tmux socket missing: %w", err)
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return "", fmt.Errorf("connect tmux socket: %w", err)
	}
	defer conn.Close()

	cmd := "capture-pane -p"
	if target != "" {
		cmd += " -t " + target
	}
	cmd += "\n"
	debugf("sending command: %s", strings.TrimSpace(cmd))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return "", fmt.Errorf("write command: %w", err)
	}

	var buf bytes.Buffer
	scanner := bufio.NewScanner(conn)
	parsing := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "%begin"):
			parsing = true
			continue
		case strings.HasPrefix(line, "%end"):
			debugf("capture complete")
			return buf.String(), nil
		}
		if parsing {
			debugf("recv: %s", line)
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		debugf("scan error: %v", err)
		return "", err
	}
	return "", errors.New("unexpected end of stream")
}
