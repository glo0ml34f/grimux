package plugin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := fmt.Sprintf(`
function init(h)
  local info = {name="plug", grimux="0.1.0", version="0.1.0"}
  local json = "{\"name\":\"plug\",\"grimux\":\"0.1.0\",\"version\":\"0.1.0\"}"
  plugin.register(h, json)
  local resp, status = plugin.http(h, "GET", "%s")
  got_ok = resp.ok
  got_status = status
end
`, srv.URL)
	if err := os.WriteFile(luaFile, []byte(code), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}

	SetPrintHandler(func(*Plugin, string) {})
	p, err := GetManager().Load(luaFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer GetManager().Unload(p.Info.Name)

	if v := p.L.GetGlobal("got_ok"); v != lua.LTrue {
		t.Fatalf("expected true, got %v", v)
	}
	if v := p.L.GetGlobal("got_status"); lua.LVAsNumber(v) != 200 {
		t.Fatalf("expected 200, got %v", v)
	}
}
