function init(h)
  local info = {name="github_issue", grimux="0.1.0", version="0.1.0"}
  local json = plugin.format(h, '{"name":"%s","grimux":"%s","version":"%s"}', info.name, info.grimux, info.version)
  plugin.register(h, json)
  plugin.command(h, "create")
  plugin.print(h, "github_issue plugin loaded")
end

function create(h, repo, title, body, token, host)
  if not repo or repo == "" then
    repo = plugin.read(h, "repo")
    if not repo or repo == "" then
      repo = plugin.prompt(h, "repo", "repository (owner/repo): ")
    end
  end
  plugin.write(h, "repo", repo)

  if not title or title == "" then
    title = plugin.read(h, "title")
    if not title or title == "" then
      title = plugin.prompt(h, "title", "issue title: ")
    end
  end
  plugin.write(h, "title", title)

  if not body or body == "" then
    body = plugin.read(h, "body")
    if not body or body == "" then
      body = plugin.prompt(h, "body", "issue body: ")
    end
  end
  plugin.write(h, "body", body)

  if not host or host == "" then
    host = plugin.read(h, "host")
    if not host or host == "" then
      host = "api.github.com"
    end
  end
  plugin.write(h, "host", host)

  -- Load the token from the plugin buffer if available.
  if (not token or token == "") then
    local saved = plugin.read(h, "token")
    if saved ~= nil and saved ~= "" then
      token = saved
    else
      token = plugin.prompt(h, "token", "github token: ")
    end
  end
  -- Save the token for next time.
  plugin.write(h, "token", token)

  local url = plugin.format(h, "https://%s/repos/%s/issues", host, repo)
  local opts = plugin.format(h,
    '{"json":{"title":%q,"body":%q},"headers":{"Authorization":"token %s","User-Agent":"grimux"}}',
    title, body, token)
  local resp, status = plugin.http(h, "POST", url, opts)
  if status == 201 then
    plugin.print(h, "created issue: " .. resp.html_url)
  else
    plugin.print(h, "failed to create issue (status " .. status .. ")")
  end
end

function shutdown(h)
  -- cleanup
end
