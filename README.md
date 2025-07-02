![20250624_1954_Grimux_ Cyberpunk Hack Demon_simple_compose_01jyj94cr1f55sw04c4dfq1v6m](https://github.com/user-attachments/assets/96d82172-67e2-4f77-aed5-4cd916646644)


# grimux 😈

Grimux is a tmux  co-pilot REPL built for composable hacking rituals with LLMs. Commands read from and write to named buffers so you can capture text from panes, transform it and feed it back into the next step. Nothing is stored unless you save a session, keeping the workflow quick and ephemeral.
![grimux_in_use](https://github.com/user-attachments/assets/cc86eeb1-55e8-45b3-8259-7d615b5d40a9)



## Goals
- Minimal friction text manipulation
- Buffers as the glue between panes, files, shell commands and the AI
- A touch of whimsy to keep hackers in flow

## Quick start
```bash
# build the binary
go build ./cmd/grimux
# run inside tmux (optionally pass a session file)
./grimux [session.grimux]
```
Press `?` for inline help or `!help` once the prompt appears.
For a deeper walkthrough see [docs/guide.md](docs/guide.md)
To create your own commands see [docs/plugin_api.md](docs/plugin_api.md).



## Buffers and panes
Buffers are scratch spaces like `%file`, `%code` and `%@`. They hold text for
commands to consume or produce. Panes are addressed by their tmux id (e.g. `%1`).
`!observe %buf %1` captures pane output into `%buf`; sending text to a pane works
the same way using its id as a buffer name.
Use `%null` when you want to discard output entirely.

## Core workflow
1. `!ls` shows available panes and buffers
2. `!observe %buf %1` grabs output from a pane
3. Edit or run commands on that buffer with `!edit %buf` or `!run %buf <cmd>`
4. Send the text to the AI with `!gen %buf <prompt>` or `!code %buf <prompt>`
5. Results land in `%@` for chaining into the next action
6. Plain text entered at the prompt is sent to the AI using your current prefix (Grimux by default)

## Command reference
- `!quit` – save session and quit
- `!x` – exit immediately
- `!ls` – list panes and buffers
- `!observe <buffer> <pane-id>` – capture a pane into a buffer
- `!save <buffer> <file>` – save buffer to file
- `!load <path>` – load file into `%file`
- `!file <path> [buffer]` – load file into buffer
- `!edit <buffer>` – edit buffer in `$EDITOR`
- `!run [buffer] <command>` – run shell command
- `!gen <buffer> <prompt>` – AI prompt into buffer
- `!code <buffer> <prompt>` – AI prompt, store code
- `!cat <buffer>` – print buffer contents
- `!set <buffer> <text>` – store text in buffer
- `!prefix <buffer|file>` – set prefix from buffer or file
- `!reset` – reset session and prefix
- `!new` – clear chat context to free tokens
- `!unset <buffer>` – clear buffer
- `%null` – special buffer that discards all writes and always reads empty
- `!get_prompt` – show current prefix
- `!session` – store session JSON in `%session`
- `!run_on <buffer> <pane> <cmd>` – run a command on another pane and store its output
- `!flow <buf1> [buf2 ... buf10]` – chain prompts using buffers
- `!grep <regex> [buffers...]` – search buffers for regex
- `!model <name>` – set OpenAI model
- `!pwd` – print working directory
- `!cd <dir>` – change working directory
- `!setenv <var> <buffer>` – set env variable from buffer
- `!getenv <var> <buffer>` – store env variable in buffer
- `!env` – list environment variables
- `!sum <buffer>` – summarize buffer with LLM
- `!rand <min> <max> <buffer>` – store random number
- `!ascii <buffer>` – gothic ascii art of first 5 words
- `!pipe <buffer> <cmd> [args]` – pipe buffer to a command
- `!encode <buffer> <encoding>` – encode a buffer (base64, urlsafe, uri, hex)
- `!hash <buffer> <algo>` – hash a buffer (md5, sha1, sha256, sha512)
- `!socat <buffer> <args>` – pipe buffer to socat
- `!curl <url> [buffer] [headers]` – HTTP GET and store body with optional headers
- `!diff <left> <right> [buffer]` – diff two buffers or files
- `!recap` – summarize the session
- `!eat <buffer> <pane>` – capture full scrollback
- `!view <buffer>` – show buffer in `$VIEWER`
- `!rm <buffer>` – remove a buffer
- `!game` – play a tiny game
- `!version` – show grimux version
- `!help` – show this help
- `!helpme <question>` – ask the AI for help using grimux
- `!idk <prompt>` – get strategic encouragement

Every command except `!game` writes its output to `%@`. Use `%name` references in
any command to insert buffer contents or `{%1}` to embed a pane capture.

## Hotkeys
- **Tab** – auto-complete commands and buffer names
- **Ctrl+L** – clear the screen
- **Ctrl+R** – reverse search history
- **Escape** – clear the line and start a command with `!`
- **Ctrl+G** – instantly start a command with `!`
- **?** – inline parameter help or `!help` when pressed on an empty line

## Environment
- `OPENAI_API_KEY` – API key used by AI commands
- `OPENAI_API_URL` – override the OpenAI endpoint
- `OPENAI_MODEL` – preferred OpenAI model (prompted if unset)
- `$EDITOR` – editor for `!edit` (defaults to `vim`)
- `$VIEWER` – viewer for `!view` (defaults to `batcat`)

## CLI flags
- `-audit` – enable audit logging
- `-serious` – start in serious mode
- `-version` – print version and exit
- `[session file]` – path to load/save session

## Architecture
The REPL lives in `internal/repl` with supporting packages under `internal/` for
OpenAI, tmux and input handling. All state is kept in memory as buffers. The
entry point is `cmd/grimux/main.go`. Session files are optional and only saved
when you choose to.

## Building and testing
```bash
go build ./cmd/grimux
go test ./...
```

Grimux strives for minimal friction and composable workflows. Enjoy the ritual
and let your agents whisper arcane knowledge.

![grimux](docs/screenshot.png)
