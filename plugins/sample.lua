function init(handle)
  local info = {name="sample", grimux="0.1.0", version="0.1.0"}
  local json = plugin.format(handle, '{"name":"%s","grimux":"%s","version":"%s"}', info.name, info.grimux, info.version)
  plugin.register(handle, json)
  plugin.print(handle, "sample plugin loaded")
end

function shutdown(handle)
  -- cleanup
end
