function init(handle)
  local info = {name="http_sample", grimux="0.1.0", version="0.1.0"}
  local json = string.format('{"name":"%s","grimux":"%s","version":"%s"}', info.name, info.grimux, info.version)
  plugin.register(handle, json)
  plugin.print(handle, "http sample loaded")

  -- GET request with params and headers
  local getOpts = '{"params":{"a":"1","b":"two"},"headers":{"X-Test":"yes"}}'
  local resp, status = plugin.http(handle, "GET", "https://httpbin.org/get", getOpts)
  plugin.print(handle, "GET status: " .. status .. ", url=" .. resp.url)

  -- POST form data
  local formOpts = '{"form":{"foo":"bar","baz":"qux"},"headers":{"X-Form":"true"}}'
  local resp2, status2 = plugin.http(handle, "POST", "https://httpbin.org/post", formOpts)
  plugin.print(handle, "POST form status: " .. status2 .. ", foo=" .. resp2.form.foo)

  -- POST JSON body
  local jsonOpts = '{"json":{"hello":"world","number":42},"headers":{"X-JSON":"true"}}'
  local resp3, status3 = plugin.http(handle, "POST", "https://httpbin.org/post", jsonOpts)
  plugin.print(handle, "POST JSON status: " .. status3 .. ", hello=" .. resp3.json.hello)
end

function shutdown(handle)
  -- cleanup
end

