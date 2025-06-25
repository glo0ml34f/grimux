package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
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

	args := []string{"-S", socket, "capture-pane", "-p"}
	if target != "" {
		args = append(args, "-t", target)
	}
	debugf("running: tmux %s", strings.Join(args, " "))
	cmd := exec.Command("tmux", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux command: %w", err)
	}
	return buf.String(), nil
}

// CapturePaneFull grabs the entire scrollback of the pane.
func CapturePaneFull(target string) (string, error) {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return "", errors.New("TMUX environment variable is not set")
	}
	socket := strings.Split(tmuxEnv, ",")[0]
	args := []string{"-S", socket, "capture-pane", "-p", "-J", "-S", "-32768"}
	if target != "" {
		args = append(args, "-t", target)
	}
	cmd := exec.Command("tmux", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux command: %w", err)
	}
	return buf.String(), nil
}

// SendKeys sends the given keys to the specified pane using tmux send-keys.
// The keys slice is passed as individual arguments to the tmux command.
func SendKeys(target string, keys ...string) error {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return errors.New("TMUX environment variable is not set")
	}
	socket := strings.Split(tmuxEnv, ",")[0]
	if _, err := os.Stat(socket); err != nil {
		return fmt.Errorf("tmux socket missing: %w", err)
	}
	args := []string{"-S", socket, "send-keys"}
	if target != "" {
		args = append(args, "-t", target)
	}
	args = append(args, keys...)
	debugf("running: tmux %s", strings.Join(args, " "))
	cmd := exec.Command("tmux", args...)
	return cmd.Run()
}

// ListPaneIDs returns the IDs of all tmux panes.
func ListPaneIDs() ([]string, error) {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return nil, errors.New("TMUX environment variable is not set")
	}
	socket := strings.Split(tmuxEnv, ",")[0]
	args := []string{"-S", socket, "list-panes", "-F", "#{pane_id}"}
	cmd := exec.Command("tmux", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tmux command: %w", err)
	}
	out := strings.Fields(buf.String())
	return out, nil
}

// ListBuffers returns the names of all tmux buffers.
func ListBuffers() ([]string, error) {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return nil, errors.New("TMUX environment variable is not set")
	}
	socket := strings.Split(tmuxEnv, ",")[0]
	args := []string{"-S", socket, "list-buffers", "-F", "#{buffer_name}"}
	cmd := exec.Command("tmux", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tmux command: %w", err)
	}
	out := strings.Fields(buf.String())
	return out, nil
}

// ShowBuffer returns the contents of the specified tmux buffer.
func ShowBuffer(name string) (string, error) {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return "", errors.New("TMUX environment variable is not set")
	}
	socket := strings.Split(tmuxEnv, ",")[0]
	args := []string{"-S", socket, "show-buffer"}
	if name != "" {
		args = append(args, "-b", name)
	}
	cmd := exec.Command("tmux", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux command: %w", err)
	}
	return buf.String(), nil
}

// SetBuffer stores data into a named tmux buffer.
func SetBuffer(name, data string) error {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return errors.New("TMUX environment variable is not set")
	}
	socket := strings.Split(tmuxEnv, ",")[0]
	args := []string{"-S", socket, "set-buffer"}
	if name != "" {
		args = append(args, "-b", name)
	}
	args = append(args, "-")
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = strings.NewReader(data)
	return cmd.Run()
}
