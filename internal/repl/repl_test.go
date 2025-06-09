package repl

import (
	"errors"
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
	text := "``go\nfirst\n```\ntext\n```python\nsecond\n```"
	code := lastCodeBlock(text)
	if code != "second" {
		t.Fatalf("unexpected last code block: %q", code)
	}
}
