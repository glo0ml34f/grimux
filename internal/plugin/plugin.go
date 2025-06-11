package plugin

import (
	"encoding/json"
	"fmt"
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
