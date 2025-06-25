package tmux

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// startFakeTmux creates a fake "tmux" binary in a temporary directory. The
// binary records its arguments to argsFile and prints the provided output.
// It returns the path to a fake socket, the args file and a cleanup function.
func startFakeTmux(t *testing.T, output string) (sock string, argsFile string, cleanup func()) {
	t.Helper()
	tmp := t.TempDir()

	sock = filepath.Join(tmp, "tmux.sock")
	if err := os.WriteFile(sock, nil, 0600); err != nil {
		t.Fatalf("create fake socket: %v", err)
	}

	argsFile = filepath.Join(tmp, "args")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" > %s\ncat <<'EOF'\n%sEOF\n", argsFile, output)
	bin := filepath.Join(tmp, "tmux")
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+string(os.PathListSeparator)+oldPath)
	cleanup = func() { os.Setenv("PATH", oldPath) }
	return sock, argsFile, cleanup
}

func TestCapturePaneWithTarget(t *testing.T) {
	sock, argsFile, cleanup := startFakeTmux(t, "hello\n")
	defer cleanup()
	os.Setenv("TMUX", sock+",session")

	got, err := CapturePane("%1")
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if got != "hello\n" {
		t.Fatalf("unexpected output: %q", got)
	}

	b, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(bytes.TrimSpace(b))
	expected := fmt.Sprintf("-S %s capture-pane -p -t %s", sock, "%1")
	if args != expected {
		t.Fatalf("unexpected args: %q", args)
	}
}

func TestCapturePaneCurrent(t *testing.T) {
	sock, argsFile, cleanup := startFakeTmux(t, "hi\n")
	defer cleanup()
	os.Setenv("TMUX", sock+",session")

	got, err := CapturePane("")
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if got != "hi\n" {
		t.Fatalf("unexpected output: %q", got)
	}

	b, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(bytes.TrimSpace(b))
	expected := fmt.Sprintf("-S %s capture-pane -p", sock)
	if args != expected {
		t.Fatalf("unexpected args: %q", args)
	}
}

func TestSendKeys(t *testing.T) {
	sock, argsFile, cleanup := startFakeTmux(t, "")
	defer cleanup()
	os.Setenv("TMUX", sock+",session")

	if err := SendKeys("%2", "echo", "hi", "Enter"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}

	b, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(bytes.TrimSpace(b))
	expected := fmt.Sprintf("-S %s send-keys -t %s echo hi Enter", sock, "%2")
	if args != expected {
		t.Fatalf("unexpected args: %q", args)
	}
}

func TestListBuffers(t *testing.T) {
	sock, argsFile, cleanup := startFakeTmux(t, "buf1\nbuf2\n")
	defer cleanup()
	os.Setenv("TMUX", sock+",s")

	bufs, err := ListBuffers()
	if err != nil {
		t.Fatalf("ListBuffers: %v", err)
	}
	if len(bufs) != 2 || bufs[0] != "buf1" || bufs[1] != "buf2" {
		t.Fatalf("unexpected buffers: %v", bufs)
	}

	b, _ := os.ReadFile(argsFile)
	args := string(bytes.TrimSpace(b))
	expected := fmt.Sprintf("-S %s list-buffers -F #{buffer_name}", sock)
	if args != expected {
		t.Fatalf("unexpected args: %q", args)
	}
}

func TestShowBuffer(t *testing.T) {
	sock, argsFile, cleanup := startFakeTmux(t, "hello\n")
	defer cleanup()
	os.Setenv("TMUX", sock+",s")

	out, err := ShowBuffer("foo")
	if err != nil {
		t.Fatalf("ShowBuffer: %v", err)
	}
	if out != "hello\n" {
		t.Fatalf("unexpected output: %q", out)
	}

	b, _ := os.ReadFile(argsFile)
	args := string(bytes.TrimSpace(b))
	expected := fmt.Sprintf("-S %s show-buffer -b foo", sock)
	if args != expected {
		t.Fatalf("unexpected args: %q", args)
	}
}

func TestSetBuffer(t *testing.T) {
	sock, argsFile, cleanup := startFakeTmux(t, "")
	defer cleanup()
	os.Setenv("TMUX", sock+",s")

	if err := SetBuffer("foo", "data"); err != nil {
		t.Fatalf("SetBuffer: %v", err)
	}

	b, _ := os.ReadFile(argsFile)
	args := string(bytes.TrimSpace(b))
	expected := fmt.Sprintf("-S %s set-buffer -b foo -", sock)
	if args != expected {
		t.Fatalf("unexpected args: %q", args)
	}
}
