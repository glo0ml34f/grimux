package tmux

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func startFakeTmux(t *testing.T, expected string, lines []string) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	sock := filepath.Join(tmp, "tmux.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		cmd, _ := r.ReadString('\n')
		if cmd != expected {
			t.Errorf("unexpected command: %q", cmd)
		}
		for _, line := range lines {
			fmt.Fprintln(conn, line)
		}
	}()
	cleanup := func() {
		ln.Close()
		<-done
	}
	return sock, cleanup
}

func TestCapturePaneWithTarget(t *testing.T) {
	expected := "capture-pane -p -t %1\n"
	sock, cleanup := startFakeTmux(t, expected, []string{"%begin", "hello", "%end"})
	defer cleanup()
	os.Setenv("TMUX", sock+",session")
	got, err := CapturePane("%1")
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if got != "hello\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestCapturePaneCurrent(t *testing.T) {
	expected := "capture-pane -p\n"
	sock, cleanup := startFakeTmux(t, expected, []string{"%begin", "hi", "%end"})
	defer cleanup()
	os.Setenv("TMUX", sock+",session")
	got, err := CapturePane("")
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if got != "hi\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
