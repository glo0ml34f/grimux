function init(h)
  local info = {name="hook_sample", grimux="0.1.0", version="0.1.0"}
  local json = plugin.format(h, '{"name":"%s","grimux":"%s","version":"%s"}', info.name, info.grimux, info.version)
  plugin.register(h, json)
  plugin.command(h, "demo")
  plugin.print(h, "hook_sample loaded")

  plugin.hook(h, "before_write", function(buf, val)
    return string.upper(val)
  end)

  plugin.hook(h, "after_read", function(buf, val)
    plugin.print(h, "read from " .. buf .. ": " .. val)
    return val
  end)

  plugin.hook(h, "before_command", function(_, cmd)
    plugin.print(h, "exec " .. cmd)
    return cmd
  end)

  plugin.hook(h, "before_markdown", function(_, md)
    return md .. "\n\n*rendered by hook_sample*"
  end)

  plugin.hook(h, "before_openai", function(_, p)
    return p .. " Be brief."
  end)

  plugin.hook(h, "after_openai", function(_, r)
    return r .. " 🚀"
  end)
end

function demo(h)
  plugin.write(h, "demo", "hello world")
  local val = plugin.read(h, "demo")
  plugin.print(h, "demo buffer: " .. val)
  local ans = plugin.prompt(h, "input", "Type something: ")
  plugin.print(h, "you typed: " .. ans)
  local _, status = plugin.http(h, "GET", "https://httpbin.org/get")
  plugin.print(h, "http status: " .. status)
end

function shutdown(h)
  -- cleanup
end
