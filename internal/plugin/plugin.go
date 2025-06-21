package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	lua "github.com/yuin/gopher-lua"
)

// Info describes a plugin's metadata.
type Info struct {
	Name    string `json:"name"`
	Grimux  string `json:"grimux"`
	Version string `json:"version"`
}

// Plugin represents a loaded Lua plugin.
type Plugin struct {
	Info   Info
	Handle string
	path   string
	L      *lua.LState
	init   *lua.LFunction
	shut   *lua.LFunction
}

// Manager keeps track of loaded plugins and the directory to load from.
type Manager struct {
	plugins map[string]*Plugin
	dir     string
	mute    map[string]bool
	printFn func(*Plugin, string)
}

var mgr = &Manager{plugins: map[string]*Plugin{}, mute: map[string]bool{}}

// GetManager returns the global plugin manager.
func GetManager() *Manager { return mgr }

// SetPrintHandler sets the function used to display plugin output.
func SetPrintHandler(fn func(*Plugin, string)) { mgr.printFn = fn }

// Dir returns the configured plugin directory.
func (m *Manager) Dir() string { return m.dir }

// SetDir sets the directory from which plugins are loaded.
func (m *Manager) SetDir(dir string) {
	m.dir = dir
}

// SetPrintHandler registers a function used to display plugin messages.
func (m *Manager) SetPrintHandler(fn func(*Plugin, string)) { m.printFn = fn }

// ToggleMute switches the muted state for a plugin and returns the new state.
func (m *Manager) ToggleMute(name string) bool {
	m.mute[name] = !m.mute[name]
	return m.mute[name]
}

// Muted reports whether prints from the plugin are muted.
func (m *Manager) Muted(name string) bool { return m.mute[name] }

// LoadAll loads all Lua plugins from the configured directory or
// ~/.grimux/plugins if none was set.
func (m *Manager) LoadAll() error {
	dir := m.dir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".grimux", "plugins")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".lua") {
			path := filepath.Join(dir, e.Name())
			if _, err := m.Load(path); err != nil {
				fmt.Fprintf(os.Stderr, "plugin %s load error: %v\n", e.Name(), err)
			}
		}
	}
	return nil
}

// Load loads a plugin from the given path.
func (m *Manager) Load(path string) (*Plugin, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	p := &Plugin{Handle: uuid.NewString(), L: L, path: path}
	api := L.NewTable()
	L.SetGlobal("plugin", api)
	L.SetFuncs(api, map[string]lua.LGFunction{
		"register": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			infoStr := L.CheckString(2)
			var inf Info
			if err := json.Unmarshal([]byte(infoStr), &inf); err != nil {
				L.RaiseError("register: %v", err)
				return 0
			}
			p.Info = inf
			L.Push(lua.LString(p.Handle))
			return 1
		},
		"print": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			msg := L.CheckString(2)
			if m.printFn != nil && !m.mute[p.Info.Name] {
				m.printFn(p, msg)
			}
			L.Push(lua.LString(p.Handle))
			return 1
		},
		"format": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			format := L.CheckString(2)
			args := make([]interface{}, 0, L.GetTop()-2)
			for i := 3; i <= L.GetTop(); i++ {
				val := L.Get(i)
				switch v := val.(type) {
				case lua.LBool:
					args = append(args, bool(v))
				case lua.LNumber:
					args = append(args, float64(v))
				case lua.LString:
					args = append(args, string(v))
				default:
					args = append(args, val.String())
				}
			}
			L.Push(lua.LString(fmt.Sprintf(format, args...)))
			return 1
		},
		"http": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			method := strings.ToUpper(L.CheckString(2))
			rawURL := L.CheckString(3)
			var opts struct {
				Headers     map[string]string `json:"headers"`
				Params      map[string]string `json:"params"`
				Form        map[string]string `json:"form"`
				JSON        interface{}       `json:"json"`
				Body        string            `json:"body"`
				ContentType string            `json:"content_type"`
			}
			if L.GetTop() >= 4 {
				optStr := L.CheckString(4)
				if err := json.Unmarshal([]byte(optStr), &opts); err != nil {
					L.RaiseError("opts parse: %v", err)
					return 0
				}
			}
			u, err := url.Parse(rawURL)
			if err != nil {
				L.RaiseError("bad url: %v", err)
				return 0
			}
			if len(opts.Params) > 0 {
				q := u.Query()
				for k, v := range opts.Params {
					q.Set(k, v)
				}
				u.RawQuery = q.Encode()
			}
			var body io.Reader
			if opts.JSON != nil {
				b, err := json.Marshal(opts.JSON)
				if err != nil {
					L.RaiseError("json body: %v", err)
					return 0
				}
				body = bytes.NewReader(b)
				if opts.ContentType == "" {
					opts.ContentType = "application/json"
				}
			} else if len(opts.Form) > 0 {
				data := url.Values{}
				for k, v := range opts.Form {
					data.Set(k, v)
				}
				body = strings.NewReader(data.Encode())
				if opts.ContentType == "" {
					opts.ContentType = "application/x-www-form-urlencoded"
				}
			} else if opts.Body != "" {
				body = strings.NewReader(opts.Body)
			}
			req, err := http.NewRequest(method, u.String(), body)
			if err != nil {
				L.RaiseError("http: %v", err)
				return 0
			}
			if opts.ContentType != "" && body != nil {
				req.Header.Set("Content-Type", opts.ContentType)
			}
			for k, v := range opts.Headers {
				req.Header.Set(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				L.RaiseError("http: %v", err)
				return 0
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				L.RaiseError("read body: %v", err)
				return 0
			}
			ct := resp.Header.Get("Content-Type")
			if strings.Contains(ct, "application/json") {
				var val interface{}
				if err := json.Unmarshal(b, &val); err == nil {
					L.Push(toLValue(L, val))
				} else {
					L.Push(lua.LString(string(b)))
				}
			} else {
				L.Push(lua.LString(string(b)))
			}
			L.Push(lua.LNumber(resp.StatusCode))
			return 2
		},
	})
	if err := L.DoFile(path); err != nil {
		L.Close()
		return nil, err
	}
	if fn, ok := L.GetGlobal("init").(*lua.LFunction); ok {
		p.init = fn
		if err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, lua.LString(p.Handle)); err != nil {
			L.Close()
			return nil, err
		}
	}
	if p.Info.Name == "" {
		L.Close()
		return nil, fmt.Errorf("plugin missing register call")
	}
	if fn, ok := L.GetGlobal("shutdown").(*lua.LFunction); ok {
		p.shut = fn
	}
	m.plugins[p.Info.Name] = p
	return p, nil
}

// Unload unloads the named plugin.
func (m *Manager) Unload(name string) error {
	p, ok := m.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not loaded")
	}
	if p.shut != nil {
		_ = p.L.CallByParam(lua.P{Fn: p.shut, NRet: 0, Protect: true}, lua.LString(p.Handle))
	}
	p.L.Close()
	delete(m.plugins, name)
	return nil
}

// Reload reloads the named plugin from disk.
func (m *Manager) Reload(name string) error {
	p, ok := m.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not loaded")
	}
	path := p.path
	if err := m.Unload(name); err != nil {
		return err
	}
	_, err := m.Load(path)
	return err
}

// List returns all loaded plugin infos sorted by name.
func (m *Manager) List() []Info {
	infos := make([]Info, 0, len(m.plugins))
	for _, p := range m.plugins {
		infos = append(infos, p.Info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

// Shutdown unloads all plugins.
func (m *Manager) Shutdown() {
	for name := range m.plugins {
		m.Unload(name)
	}
}

func toLValue(L *lua.LState, v interface{}) lua.LValue {
	switch val := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(val)
	case float64:
		return lua.LNumber(val)
	case string:
		return lua.LString(val)
	case []interface{}:
		tbl := L.NewTable()
		for i, it := range val {
			tbl.RawSetInt(i+1, toLValue(L, it))
		}
		return tbl
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, it := range val {
			tbl.RawSetString(k, toLValue(L, it))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", val))
	}
}
