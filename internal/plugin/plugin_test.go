package plugin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
  plugin.register(h, json, {"http"})
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
  plugin.register(h, json, {"write","hook","read","prompt"})
end

function run(h)
  plugin.write(h, "foo", "bar")
  local fn = function(buf, val) return val .. "-mod" end
  plugin.hook(h, "after_read", fn)
  plugin.hook(h, "after_read", fn)
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
	SetReadBufferFunc(func(n string) (string, bool) {
		v, ok := buf[n]
		if ok {
			v = GetManager().RunHook("after_read", n, v)
		}
		return v, ok
	})
	SetWriteBufferFunc(func(n, v string) {
		v = GetManager().RunHook("before_write", n, v)
		buf[n] = v
	})
	SetPromptFunc(func(string) (string, error) { return "y", nil })

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
	if val := buf["%plug_ask"]; val != "y" {
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
  plugin.register(h, json, {"command"})
  plugin.command(h, "doit")
end

function doit(h, a, b)
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

func TestCommandArgBuffer(t *testing.T) {
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  local info = {name="pbuf", grimux="0.1.0", version="0.1.0"}
  local json = '{"name":"pbuf","grimux":"0.1.0","version":"0.1.0"}'
  plugin.register(h, json, {"command"})
  plugin.command(h, "echo")
end

function echo(h, val)
  last = val
end
`
	if err := os.WriteFile(luaFile, []byte(code), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}

	buf := map[string]string{"%foo": "bar"}
	SetPrintHandler(func(*Plugin, string) {})
	SetReadBufferFunc(func(n string) (string, bool) { v, ok := buf[n]; return v, ok })
	SetWriteBufferFunc(func(n, v string) { buf[n] = v })

	p, err := GetManager().Load(luaFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer GetManager().Unload(p.Info.Name)

	if err := GetManager().RunCommand("pbuf.echo", []string{"%foo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if v := p.L.GetGlobal("last"); v.String() != "bar" {
		t.Fatalf("last=%v", v)
	}
}
func TestHookPrintInfo(t *testing.T) {
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  local info = {name="phook", grimux="0.1.0", version="0.1.0"}
  local json = '{"name":"phook","grimux":"0.1.0","version":"0.1.0"}'
  plugin.register(h, json, {"hook","write"})
  plugin.hook(h, "before_write", function(b,v) return v end)
end
`
	if err := os.WriteFile(luaFile, []byte(code), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}

	var msgs []string
	SetPrintHandler(func(_ *Plugin, msg string) { msgs = append(msgs, msg) })
	if _, err := GetManager().Load(luaFile); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(msgs) == 0 || msgs[0] != "hook registered: before_write" {
		t.Fatalf("hook message=%v", msgs)
	}
	GetManager().Shutdown()
}

func TestBeforeWriteHook(t *testing.T) {
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  local info = {name="pre", grimux="0.1.0", version="0.1.0"}
  local json = '{"name":"pre","grimux":"0.1.0","version":"0.1.0"}'
  plugin.register(h, json, {"hook","write"})
end

function run(h)
  plugin.hook(h, "before_write", function(b,v) return v.."-x" end)
  plugin.write(h, "foo", "bar")
end
`
	if err := os.WriteFile(luaFile, []byte(code), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}
	buf := map[string]string{}
	SetPrintHandler(func(*Plugin, string) {})
	SetWriteBufferFunc(func(n, v string) {
		v = GetManager().RunHook("before_write", n, v)
		buf[n] = v
	})
	p, err := GetManager().Load(luaFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer GetManager().Unload(p.Info.Name)
	if err := p.L.CallByParam(lua.P{Fn: p.L.GetGlobal("run"), NRet: 0, Protect: true}, lua.LString(p.Handle)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if buf["%pre_foo"] != "bar-x" {
		t.Fatalf("buffer=%s", buf["%pre_foo"])
	}
}

func TestHasHook(t *testing.T) {
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  local info = {name="hh", grimux="0.1.0", version="0.1.0"}
  local json = '{"name":"hh","grimux":"0.1.0","version":"0.1.0"}'
  plugin.register(h, json, {"hook"})
  plugin.hook(h, "before_openai", function(b,v) return v end)
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
	if !GetManager().HasHook("before_openai") {
		t.Fatalf("missing hook")
	}
	if err := GetManager().Unload(p.Info.Name); err != nil {
		t.Fatalf("unload: %v", err)
	}
	if GetManager().HasHook("before_openai") {
		t.Fatalf("hook still registered")
	}
}

func TestManyHooks(t *testing.T) {
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  local info = {name="many", grimux="0.1.0", version="0.1.0"}
  local json = '{"name":"many","grimux":"0.1.0","version":"0.1.0"}'
  plugin.register(h, json, {"hook"})
  for i=1,20 do
    plugin.hook(h, "h"..i, function(b,v) return v end)
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
	if len(p.hooks) != 20 {
		t.Fatalf("hook count=%d", len(p.hooks))
	}
}

func TestPluginGen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices": [{"message": {"content": "ok"}}]}`)
	}))
	defer srv.Close()
	_ = os.Setenv("OPENAI_API_URL", srv.URL)
	_ = os.Setenv("OPENAI_API_KEY", "x")
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  plugin.register(h, '{"name":"aiplug","grimux":"0.1.0","version":"0.1.0"}', {"gen"})
end
function run(h)
  plugin.gen(h, "out", "hi")
end
`
	if err := os.WriteFile(luaFile, []byte(code), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}
	storage := map[string]string{}
	SetPrintHandler(func(*Plugin, string) {})
	SetGenCommandFunc(func(b, prompt string) (string, error) {
		storage[b] = "ok"
		return "ok", nil
	})
	SetReadBufferFunc(func(n string) (string, bool) { v, ok := storage[n]; return v, ok })
	SetWriteBufferFunc(func(n, v string) { storage[n] = v })
	p, err := GetManager().Load(luaFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer GetManager().Unload(p.Info.Name)
	if err := p.L.CallByParam(lua.P{Fn: p.L.GetGlobal("run"), NRet: 0, Protect: true}, lua.LString(p.Handle)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if storage["%aiplug_out"] != "ok" {
		t.Fatalf("got=%s", storage["%aiplug_out"])
	}
}

func TestPluginSocat(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat not installed")
	}
	dir := t.TempDir()
	luaFile := filepath.Join(dir, "plug.lua")
	code := `
function init(h)
  plugin.register(h, '{"name":"soc","grimux":"0.1.0","version":"0.1.0"}', {"socat","write"})
end
function run(h)
  plugin.write(h, "in", "hello")
  out = plugin.socat(h, "in", "-u", "-", "-")
end
`
	if err := os.WriteFile(luaFile, []byte(code), 0o600); err != nil {
		t.Fatalf("write lua: %v", err)
	}
	buf := map[string]string{}
	SetPrintHandler(func(*Plugin, string) {})
	SetSocatCommandFunc(func(b string, args []string) (string, error) {
		return buf[b], nil
	})
	SetReadBufferFunc(func(n string) (string, bool) { v, ok := buf[n]; return v, ok })
	SetWriteBufferFunc(func(n, v string) { buf[n] = v })
	p, err := GetManager().Load(luaFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer GetManager().Unload(p.Info.Name)
	if err := p.L.CallByParam(lua.P{Fn: p.L.GetGlobal("run"), NRet: 0, Protect: true}, lua.LString(p.Handle)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out := p.L.GetGlobal("out").String(); strings.TrimSpace(out) != "hello" {
		t.Fatalf("out=%q", out)
	}
}
