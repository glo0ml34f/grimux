package openai

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSendPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"content": "ok"}}]}`))
	}))
	defer srv.Close()
	_ = os.Setenv("OPENAI_API_URL", srv.URL)
	_ = os.Setenv("OPENAI_API_KEY", "test")
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.HTTPClient = srv.Client()
	reply, err := c.SendPrompt("hi")
	if err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestSetModelName(t *testing.T) {
	old := GetModelName()
	SetModelName("dummy")
	if GetModelName() != "dummy" {
		t.Fatalf("model not set")
	}
	SetModelName(old)
}
