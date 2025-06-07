package tmux

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

// CapturePane connects to the tmux server over its UNIX socket and captures the
// contents of the specified pane. If target is empty, the current pane is
// captured.
func CapturePane(target string) (string, error) {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return "", errors.New("TMUX environment variable is not set")
	}

	socket := strings.Split(tmuxEnv, ",")[0]
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return "", fmt.Errorf("connect tmux socket: %w", err)
	}
	defer conn.Close()

	if target == "" {
		target = "" // current pane
	}

	cmd := fmt.Sprintf("capture-pane -p -t %s\n", target)
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
			return buf.String(), nil
		}
		if parsing {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("unexpected end of stream")
}
