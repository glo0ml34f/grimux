package repl

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplacePaneRefs(t *testing.T) {
	old := capturePane
	capturePane = func(target string) (string, error) {
		if target != "%1" {
			return "", errors.New("bad target")
		}
		return "hello\n", nil
	}
	defer func() { capturePane = old }()

	got := replacePaneRefs("before /{%1}/ after")
	expected := "before /\n```\nhello\n```\n/ after"
	if got != expected {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestReplacePaneRefsError(t *testing.T) {
	old := capturePane
	capturePane = func(target string) (string, error) {
		return "", errors.New("fail")
	}
	defer func() { capturePane = old }()

	got := replacePaneRefs("/{%2}/")
	if got != "/[capture error: fail]/" {
		t.Fatalf("unexpected error output: %q", got)
	}
}

func TestReplaceBufferRefs(t *testing.T) {
	buffers["%foo"] = "bar"
	defer func() { delete(buffers, "%foo") }()
	got := replaceBufferRefs("hello %foo world")
	if got != "hello bar world" {
		t.Fatalf("unexpected buffer replace: %q", got)
	}
}

func TestReplaceBufferRefsOutput(t *testing.T) {
	buffers["%@"] = "output"
	got := replaceBufferRefs("use %@ here")
	if got != "use output here" {
		t.Fatalf("unexpected output buffer replace: %q", got)
	}
}

func TestLastCodeBlock(t *testing.T) {
	text := "```\nfirst\n```\ntext\n```python\nsecond\n```"
	code := lastCodeBlock(text)
	if code != "second" {
		t.Fatalf("unexpected last code block: %q", code)
	}
}

func TestNullBuffer(t *testing.T) {
	writeBuffer("%null", "ignored")
	if val, ok := readBuffer("%null"); !ok || val != "" {
		t.Fatal("%null should always be empty")
	}
}

func TestHandleCommandBasic(t *testing.T) {
	// test !set, !prefix, !get_prompt, !unset and !new
	chatCtx = []byte("old context")

	if handleCommand("!set %foo bar") {
		t.Fatalf("!set should not request exit")
	}
	if val, ok := buffers["%foo"]; !ok || val != "bar" {
		t.Fatalf("buffer not set: %v %q", ok, val)
	}

	if handleCommand("!prefix %foo") {
		t.Fatalf("!prefix should not request exit")
	}
	if askPrefix != "bar" {
		t.Fatalf("prefix not updated: %q", askPrefix)
	}

	handleCommand("!get_prompt")
	if !strings.Contains(buffers["%@"], "bar") {
		t.Fatalf("!get_prompt output not captured")
	}

	if handleCommand("!unset %foo") {
		t.Fatalf("!unset should not request exit")
	}
	if _, ok := buffers["%foo"]; ok {
		t.Fatalf("buffer should be removed")
	}

	if handleCommand("!new") {
		t.Fatalf("!new should not request exit")
	}
	if len(chatCtx) != 0 {
		t.Fatalf("chat context not cleared")
	}
}

// startFakeTmux creates a fake tmux binary for testing.
func startFakeTmux(t *testing.T, output string) (sock, argsFile string, cleanup func()) {
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

func TestIsTmuxBuffer(t *testing.T) {
	sock, _, cleanup := startFakeTmux(t, "foo\nbar\n")
	defer cleanup()
	os.Setenv("TMUX", sock+",s")

	if !isTmuxBuffer("%foo") {
		t.Fatalf("%%foo should be detected as tmux buffer")
	}
	if isTmuxBuffer("%baz") {
		t.Fatalf("%%baz should not be detected as tmux buffer")
	}
}

func TestReadBufferTmuxRefresh(t *testing.T) {
	sock, argsFile, cleanup := startFakeTmux(t, "buf1\n")
	defer cleanup()
	os.Setenv("TMUX", sock+",s")

	writeBuffer("%buf1", "old")
	out, ok := readBuffer("%buf1")
	if !ok {
		t.Fatalf("readBuffer returned not ok")
	}
	if out != "buf1\n" {
		t.Fatalf("unexpected output: %q", out)
	}

	b, _ := os.ReadFile(argsFile)
	args := string(bytes.TrimSpace(b))
	expected := fmt.Sprintf("-S %s show-buffer -b buf1", sock)
	if args != expected {
		t.Fatalf("unexpected args: %q", args)
	}
}
