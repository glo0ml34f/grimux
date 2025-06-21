package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/glo0ml34f/grimux/internal/plugin"
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
func TestOpenAIHooks(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req chatRequest
		_ = json.Unmarshal(b, &req)
		received = req.Messages[0].Content
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"content": "resp"}}]}`))
	}))
	defer srv.Close()
	_ = os.Setenv("OPENAI_API_URL", srv.URL)
	_ = os.Setenv("OPENAI_API_KEY", "test")
	plugin.GetManager().Shutdown()
	luaCode := `
function init(h)
  local info = {name="hooker", grimux="0.1.0", version="0.1.0"}
  local json = '{"name":"hooker","grimux":"0.1.0","version":"0.1.0"}'
  plugin.register(h, json)
  plugin.hook(h, "before_openai", function(buf,val) return val .. " mod" end)
  plugin.hook(h, "after_openai", function(buf,val) return val .. "!" end)
end
`
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	if err := os.WriteFile(luaFile, []byte(luaCode), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}
	plugin.SetPrintHandler(func(*plugin.Plugin, string) {})
	if _, err := plugin.GetManager().Load(luaFile); err != nil {
		t.Fatalf("load: %v", err)
	}
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.HTTPClient = srv.Client()
	reply, err := c.SendPrompt("hi")
	if err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	if received != "hi mod" {
		t.Fatalf("prompt=%s", received)
	}
	if reply != "resp!" {
		t.Fatalf("reply=%s", reply)
	}
}
