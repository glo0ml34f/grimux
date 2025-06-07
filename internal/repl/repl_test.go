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
	expected := "before ```\nhello\n``` after"
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
	if got != "[capture error: fail]" {
		t.Fatalf("unexpected error output: %q", got)
	}
}
