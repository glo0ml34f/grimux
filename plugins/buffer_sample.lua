function init(handle)
  local info = {name="buffer_sample", grimux="0.1.0", version="0.1.0"}
  local json = plugin.format(handle, '{"name":"%s","grimux":"%s","version":"%s"}', info.name, info.grimux, info.version)
  plugin.register(handle, json)
  plugin.command(handle, "run")
  plugin.print(handle, "buffer sample loaded")
end

function run(handle)
  plugin.write(handle, "demo", "hello")
  plugin.hook(handle, "after_read", function(buf, val)
    plugin.print(handle, "after_read: " .. buf .. " -> " .. val)
    return val .. "-mod"
  end)
  local val = plugin.read(handle, "demo")
  plugin.print(handle, "read value: " .. val)
  local input = plugin.prompt(handle, "response", "Type something: ")
  plugin.print(handle, "prompted: " .. input)
end

function shutdown(handle)
  -- cleanup
end
