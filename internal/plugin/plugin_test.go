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

func TestBuffersAndHook(t *testing.T) {
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  local info = {name="plug", grimux="0.1.0", version="0.1.0"}
  local json = '{"name":"plug","grimux":"0.1.0","version":"0.1.0"}'
  plugin.register(h, json)
end

function run(h)
  plugin.write(h, "foo", "bar")
  plugin.hook(h, "after_read", function(buf, val) return val .. "-mod" end)
  local val = plugin.read(h, "foo")
  plugin.prompt(h, "ask", "? ")
  return val
end
`
	if err := os.WriteFile(luaFile, []byte(code), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}

	buf := map[string]string{}
	SetPrintHandler(func(*Plugin, string) {})
	SetReadBufferFunc(func(n string) (string, bool) { v, ok := buf[n]; return v, ok })
	SetWriteBufferFunc(func(n, v string) { buf[n] = v })
	SetPromptFunc(func(string) (string, error) { return "typed", nil })

	p, err := GetManager().Load(luaFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer GetManager().Unload(p.Info.Name)

	if err := p.L.CallByParam(lua.P{Fn: p.L.GetGlobal("run"), NRet: 1, Protect: true}, lua.LString(p.Handle)); err != nil {
		t.Fatalf("call run: %v", err)
	}
	ret := p.L.Get(-1).String()
	p.L.Pop(1)
	if ret != "bar-mod" {
		t.Fatalf("hook result=%s", ret)
	}
	if val := buf["%plug_foo"]; val != "bar" {
		t.Fatalf("buffer foo=%s", val)
	}
	if val := buf["%plug_ask"]; val != "typed" {
		t.Fatalf("prompt buffer=%s", val)
	}
}

func TestCommandRun(t *testing.T) {
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  local info = {name="plug", grimux="0.1.0", version="0.1.0"}
  local json = '{"name":"plug","grimux":"0.1.0","version":"0.1.0"}'
  plugin.register(h, json)
  plugin.command(h, "doit")
end

function run(h, a, b)
  if b == nil then
    last = a
  else
    last = a .. "+" .. b
  end
end
`
	if err := os.WriteFile(luaFile, []byte(code), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}

	SetPrintHandler(func(*Plugin, string) {})
	p, err := GetManager().Load(luaFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer GetManager().Unload(p.Info.Name)

	if !GetManager().IsCommand("plug.doit") {
		t.Fatalf("command not registered")
	}
	if err := GetManager().RunCommand("plug.doit", []string{"a", "b"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if v := p.L.GetGlobal("last"); v.String() != "a+b" {
		t.Fatalf("last=%v", v)
	}
}
