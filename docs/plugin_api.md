# Grimux Plugin API

Grimux plugins are Lua scripts loaded from `~/.grimux/plugins` by default or a directory supplied via the `-plugins` flag. A plugin can register new REPL commands, react to buffer events and perform HTTP requests.

Each plugin defines at least an `init(handle)` function. Optional `run(handle, ...)` and `shutdown(handle)` functions allow command execution and clean up. During `init` you **must** call `plugin.register` to set the plugin's metadata.

```lua
function init(h)
  local info = {name="sample", grimux="0.1.0", version="1.0.0"}
  local json = plugin.format(h,
    '{"name":"%s","grimux":"%s","version":"%s"}',
    info.name, info.grimux, info.version)
  plugin.register(h, json)
  plugin.command(h, "doit") -- exposes `sample.doit`
end

function run(h, ...)
  plugin.print(h, "args: " .. table.concat({...}, ","))
end
```

## Available functions

| Function | Description |
|----------|-------------|
| `plugin.register(handle, json)` | Register plugin metadata. The JSON must contain `name`, `grimux` and `version`. |
| `plugin.print(handle, message)` | Display a message in the REPL. Respects plugin mute state. |
| `plugin.format(handle, fmt, ...)` | Returns a formatted string similar to `string.format`. Useful for building JSON. |
| `plugin.read(handle, buffer)` | Read the contents of a buffer. The plugin name is prepended automatically unless the buffer already starts with `%`. |
| `plugin.write(handle, buffer, data)` | Write data to a buffer. |
| `plugin.prompt(handle, buffer, message)` | Prompt the user. The response is returned and written to `buffer`. |
| `plugin.hook(handle, name, fn(buf, val))` | Register a hook callback. Hooks include `before_write`, `after_read`, `before_command`, `before_markdown`, `before_openai` and `after_openai`. |
| `plugin.command(handle, name)` | Register a plugin command. Invoking `<plugin>.<name>` will call `run`. |
| `plugin.http(handle, method, url [, opts])` | Perform an HTTP request. `opts` is a JSON object supporting `headers`, `params`, `form`, `json`, `body` and `content_type`. Returns the response body (parsed as a Lua table if JSON) and status code. |
| `plugin.gen(handle, buffer, prompt)` | Invoke the `!gen` command using Grimux's OpenAI config. The response is written to `buffer`. |
| `plugin.socat(handle, buffer, args)` | Run the `!socat` command with `args` to send `buffer` contents through socat. Returns command output. |

Buffers written via `plugin.write` are namespaced as `%<plugin>_<buffer>` to avoid clashing with user buffers. Use leading `%` to access a global buffer directly.

## Hooks

Hooks let plugins modify data at various points. For example, a plugin could upper‑case all writes:

```lua
plugin.hook(handle, "before_write", function(buf, val)
  return string.upper(val)
end)
```

Hooks receive the buffer name and current value. The returned value replaces the original.

## Shutdown

If a `shutdown(handle)` function is present, it is called when the plugin is unloaded or when Grimux exits.

