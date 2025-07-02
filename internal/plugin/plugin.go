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
	"regexp"
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
	Info     Info
	Handle   string
	path     string
	L        *lua.LState
	init     *lua.LFunction
	shut     *lua.LFunction
	commands map[string]*lua.LFunction
	hooks    map[string][]*lua.LFunction
	allowed  map[string]bool
}

// Manager keeps track of loaded plugins and the directory to load from.
type Manager struct {
	plugins  map[string]*Plugin
	dir      string
	mute     map[string]bool
	printFn  func(*Plugin, string)
	commands map[string]*Plugin
}

var mgr = &Manager{plugins: map[string]*Plugin{}, mute: map[string]bool{}, commands: map[string]*Plugin{}}

var readBufFn func(string) (string, bool)
var writeBufFn func(string, string)
var promptFn func(string) (string, error)
var addCmdFn func(string)
var delCmdFn func(string)
var genCmdFn func(string, string) (string, error)
var socatCmdFn func(string, []string) (string, error)
var pipeCmdFn func(string, string, []string) (string, error)

var bufferPattern = regexp.MustCompile(`%[@a-zA-Z0-9_]+`)

// GetManager returns the global plugin manager.
func GetManager() *Manager { return mgr }

// SetPrintHandler sets the function used to display plugin output.
func SetPrintHandler(fn func(*Plugin, string)) { mgr.printFn = fn }

// SetReadBufferFunc registers the function used to read buffer contents.
func SetReadBufferFunc(fn func(string) (string, bool)) { readBufFn = fn }

// SetWriteBufferFunc registers the function used to write buffer contents.
func SetWriteBufferFunc(fn func(string, string)) { writeBufFn = fn }

// SetPromptFunc registers the function used to prompt the user for input.
func SetPromptFunc(fn func(string) (string, error)) { promptFn = fn }

// SetCommandAddFunc registers a function called when a plugin adds a command.
func SetCommandAddFunc(fn func(string)) { addCmdFn = fn }

// SetCommandRemoveFunc registers a function called when a plugin command is removed.
func SetCommandRemoveFunc(fn func(string)) { delCmdFn = fn }

// SetGenCommandFunc registers the function used by plugin.gen.
func SetGenCommandFunc(fn func(string, string) (string, error)) { genCmdFn = fn }

// SetSocatCommandFunc registers the function used by plugin.socat.
func SetSocatCommandFunc(fn func(string, []string) (string, error)) { socatCmdFn = fn }

// SetPipeCommandFunc registers the function used by plugin.pipe.
func SetPipeCommandFunc(fn func(string, string, []string) (string, error)) { pipeCmdFn = fn }

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
	p := &Plugin{Handle: uuid.NewString(), L: L, path: path, hooks: map[string][]*lua.LFunction{}}
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
			if L.GetTop() >= 3 {
				tbl := L.CheckTable(3)
				var funcs []string
				tbl.ForEach(func(_, v lua.LValue) {
					funcs = append(funcs, v.String())
				})
				if promptFn != nil && len(funcs) > 0 {
					msg := fmt.Sprintf("Plugin %s wants API functions %s. Allow? [y/N] ", inf.Name, strings.Join(funcs, ","))
					resp, err := promptFn(msg)
					if err != nil || strings.ToLower(strings.TrimSpace(resp)) != "y" {
						L.RaiseError("api denied")
						return 0
					}
				}
				p.allowed = map[string]bool{}
				for _, f := range funcs {
					p.allowed[f] = true
				}
			}
			L.Push(lua.LString(p.Handle))
			return 1
		},
		"print": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			if p.allowed != nil && !p.allowed["print"] {
				L.RaiseError("print not allowed")
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
		"read": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			if p.allowed != nil && !p.allowed["read"] {
				L.RaiseError("read not allowed")
				return 0
			}
			name := L.CheckString(2)
			if readBufFn == nil {
				L.Push(lua.LNil)
				return 1
			}
			val, ok := readBufFn(pluginBufferName(p.Info.Name, name))
			if ok {
				L.Push(lua.LString(val))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		},
		"write": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			if p.allowed != nil && !p.allowed["write"] {
				L.RaiseError("write not allowed")
				return 0
			}
			name := L.CheckString(2)
			data := L.CheckString(3)
			if writeBufFn != nil {
				writeBufFn(pluginBufferName(p.Info.Name, name), data)
			}
			L.Push(lua.LString(p.Handle))
			return 1
		},
		"prompt": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			if p.allowed != nil && !p.allowed["prompt"] {
				L.RaiseError("prompt not allowed")
				return 0
			}
			name := L.CheckString(2)
			msg := L.CheckString(3)
			if promptFn == nil {
				L.Push(lua.LString(""))
				return 1
			}
			resp, err := promptFn(msg)
			if err != nil {
				L.RaiseError("prompt: %v", err)
				return 0
			}
			if readBufFn != nil {
				resp = bufferPattern.ReplaceAllStringFunc(resp, func(tok string) string {
					if val, ok := readBufFn(tok); ok {
						return val
					}
					return tok
				})
			}
			if writeBufFn != nil {
				writeBufFn(pluginBufferName(p.Info.Name, name), resp)
			}
			L.Push(lua.LString(resp))
			return 1
		},
		"hook": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			hookName := L.CheckString(2)
			cb := L.CheckFunction(3)
			if p.hooks == nil {
				p.hooks = map[string][]*lua.LFunction{}
			}
			if promptFn != nil {
				msg := fmt.Sprintf("Plugin %s wants hook '%s'. Malicious hooks can run any command. Allow? [y/N] ", p.Info.Name, hookName)
				resp, err := promptFn(msg)
				if err == nil && strings.ToLower(strings.TrimSpace(resp)) != "y" {
					m.Unload(p.Info.Name)
					L.RaiseError("hook denied")
					return 0
				}
			}
			exists := false
			for _, f := range p.hooks[hookName] {
				if f == cb {
					exists = true
					break
				}
			}
			if !exists {
				p.hooks[hookName] = append(p.hooks[hookName], cb)
				if m.printFn != nil && !m.mute[p.Info.Name] {
					m.printFn(p, fmt.Sprintf("hook registered: %s", hookName))
				}
			}
			L.Push(lua.LString(p.Handle))
			return 1
		},
		"command": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			cmd := L.CheckString(2)
			if err := m.RegisterCommand(p, cmd); err != nil {
				L.RaiseError("command: %v", err)
				return 0
			}
			L.Push(lua.LString(p.Handle))
			return 1
		},
		"http": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			if p.allowed != nil && !p.allowed["http"] {
				L.RaiseError("http not allowed")
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
		"gen": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			if p.allowed != nil && !p.allowed["gen"] {
				L.RaiseError("gen not allowed")
				return 0
			}
			buf := L.CheckString(2)
			prompt := L.CheckString(3)
			if genCmdFn == nil {
				L.Push(lua.LString(""))
				return 1
			}
			reply, err := genCmdFn(pluginBufferName(p.Info.Name, buf), prompt)
			if err != nil {
				L.RaiseError("gen: %v", err)
				return 0
			}
			if writeBufFn != nil {
				writeBufFn(pluginBufferName(p.Info.Name, buf), reply)
			}
			L.Push(lua.LString(reply))
			return 1
		},
		"socat": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			if p.allowed != nil && !p.allowed["socat"] {
				L.RaiseError("socat not allowed")
				return 0
			}
			buf := L.CheckString(2)
			args := make([]string, 0, L.GetTop()-2)
			for i := 3; i <= L.GetTop(); i++ {
				args = append(args, L.CheckString(i))
			}
			if socatCmdFn == nil {
				L.Push(lua.LString(""))
				return 1
			}
			out, err := socatCmdFn(pluginBufferName(p.Info.Name, buf), args)
			if err != nil {
				L.RaiseError("socat: %v", err)
				return 0
			}
			L.Push(lua.LString(out))
			return 1
		},
		"pipe": func(L *lua.LState) int {
			handle := L.CheckString(1)
			if handle != p.Handle {
				L.RaiseError("invalid handle")
				return 0
			}
			if p.allowed != nil && !p.allowed["pipe"] {
				L.RaiseError("pipe not allowed")
				return 0
			}
			buf := L.CheckString(2)
			cmd := L.CheckString(3)
			args := make([]string, 0, L.GetTop()-3)
			for i := 4; i <= L.GetTop(); i++ {
				args = append(args, L.CheckString(i))
			}
			if pipeCmdFn == nil {
				L.Push(lua.LString(""))
				return 1
			}
			out, err := pipeCmdFn(pluginBufferName(p.Info.Name, buf), cmd, args)
			if err != nil {
				L.RaiseError("pipe: %v", err)
				return 0
			}
			L.Push(lua.LString(out))
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
	for cmd, pl := range m.commands {
		if pl == p {
			delete(m.commands, cmd)
			if delCmdFn != nil {
				delCmdFn(cmd)
			}
		}
	}
	p.commands = nil
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

// RunHook executes the registered callbacks for the given hook name.
func (m *Manager) RunHook(name, buf string, data string) string {
	out := data
	for _, p := range m.plugins {
		if fns, ok := p.hooks[name]; ok {
			for _, fn := range fns {
				if err := p.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, lua.LString(buf), lua.LString(out)); err == nil {
					val := p.L.Get(-1)
					out = val.String()
					p.L.Pop(1)
				} else {
					p.L.Pop(p.L.GetTop())
				}
			}
		}
	}
	return out
}

// HasHook reports whether any plugin has registered the given hook name.
func (m *Manager) HasHook(name string) bool {
	for _, p := range m.plugins {
		if len(p.hooks[name]) > 0 {
			return true
		}
	}
	return false
}

// HookNames returns the names of hooks registered by the plugin.
func (m *Manager) HookNames(name string) []string {
	p, ok := m.plugins[name]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(p.hooks))
	for h := range p.hooks {
		names = append(names, h)
	}
	sort.Strings(names)
	return names
}

// RegisterCommand registers a new REPL command for the plugin.
func (m *Manager) RegisterCommand(p *Plugin, name string) error {
	if p.Info.Name == "" {
		return fmt.Errorf("plugin not registered")
	}
	cmd := p.Info.Name + "." + name
	if _, ok := m.commands[cmd]; ok {
		return fmt.Errorf("command exists")
	}
	fn, ok := p.L.GetGlobal(name).(*lua.LFunction)
	if !ok {
		return fmt.Errorf("function %s not found", name)
	}
	if p.commands == nil {
		p.commands = map[string]*lua.LFunction{}
	}
	p.commands[name] = fn
	m.commands[cmd] = p
	if addCmdFn != nil {
		addCmdFn(cmd)
	}
	return nil
}

// IsCommand checks whether the name corresponds to a plugin command.
func (m *Manager) IsCommand(name string) bool {
	_, ok := m.commands[name]
	return ok
}

// RunCommand invokes the plugin run function for the command.
func (m *Manager) RunCommand(name string, args []string) error {
	p, ok := m.commands[name]
	if !ok {
		return fmt.Errorf("command not found")
	}
	local := strings.TrimPrefix(name, p.Info.Name+".")
	fn, ok := p.commands[local]
	if !ok {
		return fmt.Errorf("plugin has no %s function", local)
	}
	vals := []lua.LValue{lua.LString(p.Handle)}
	for _, a := range args {
		if readBufFn != nil {
			a = bufferPattern.ReplaceAllStringFunc(a, func(tok string) string {
				if val, ok := readBufFn(tok); ok {
					return val
				}
				return tok
			})
		}
		vals = append(vals, lua.LString(a))
	}
	return p.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, vals...)
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

func pluginBufferName(pluginName, name string) string {
	if strings.HasPrefix(name, "%") {
		return name
	}
	return "%" + pluginName + "_" + name
}
