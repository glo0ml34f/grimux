# grimux 😈

Grimux is a whimsical tmux REPL designed for hackers who love composable text workflows. It lives inside your tmux session, capturing pane output, letting you pipe text through `$EDITOR`, rendering markdown through `batcat` (or `$PAGER`), and generally making mischief a breeze.

## Why Buffers and Panes?
Buffers are named scratch spaces like `%file`, `%code`, `%@` and whatever else you invent. Commands read from and write to these buffers so you can chain actions together. Panes are referenced by their tmux id (for example `%1`). Capture pane output with `!observe` and it lands in a buffer ready for editing, AI prompts or shell commands.

## Core Workflow
1. Use `!ls` to view panes and buffers.
2. Capture a pane with `!observe %buf %1`.
3. Edit that buffer with `!edit %buf` or run commands with `!run %buf cat`.
4. Pipe buffer text to the AI with `!gen` or `!code`.
5. Results land in `%@` so you can feed them right back into the next command.

The goal is low friction hacking. You work entirely in text buffers and every command plays nicely with the others.

## Command Reference
- `!ls` – list panes and buffers
- `!observe <buffer> <pane>` – capture pane into buffer
- `!cat <buf>` – print a buffer
- `!set <buffer> <text>` – store text in buffer
- `!run [buffer] <cmd>` – run shell command and store output
- `!gen <buffer> <prompt>` – ask the AI and store reply
- `!code <buffer> <prompt>` – AI prompt but keep last code block
- `!rand <min> <max> <buffer>` – store random number
- `!game` – goofy number guessing game
- `!edit <buffer>` – open buffer in `$EDITOR`
- `!save <buffer> <file>` – save buffer to file
- `!file <path> [buf]` – load file into optional buffer
- `!session` – stash current session JSON in `%session`
- `!grep <regex> [bufs]` – search buffers for regex
- `!model <name>` – set the OpenAI model
- `!sum <buffer>` – summarize buffer with the AI
- `!ascii <buffer>` – convert first five words to gothic ascii art
- `!nc <buffer> <args>` – pipe buffer through netcat
- `!a <prompt>` – ask the AI with the configured prefix
- `!help` – show this help
- `!helpme <question>` – send `!help` output and your question to the AI for terse support

Every command (except `!game`) stores its output in `%@` so you can immediately reuse it. Use `%` references anywhere to insert buffer contents or `{%1}` to embed pane captures.

## Hotkeys
- **Tab** – auto-complete commands and buffer names
- **Ctrl+L** – clear the screen
- **Ctrl+R** – reverse search command history
- **Escape** – clears the current line and starts a command with `!`
- **?** – show inline parameter help or run `!help` when pressed on an empty line

If you mash Enter without typing a command several times, Grimux will cheekily suggest you go touch some grass.

## Environment
- `$EDITOR` – editor used by `!edit` (defaults to `vim`)
- `$PAGER`  – viewer used for markdown output (falls back to `batcat`)

## Audit Mode
Start grimux with `-audit` to keep a log of AI replies. Once the log grows, grimux summarizes it and stores it in the session.
## Secret Agents
The `prompts/` directory contains short character blurbs that shape how Grimux's AI helpers speak. They act as your sneaky crew—crypto mages, red team pirates and more. Pick one as a prefix with `!prefix %file` to change the vibe of `!a`, `!gen` and friends.

## Building and Testing
```bash
go build ./cmd/grimux
go test ./...
```

Grimux keeps high scores from `!game` in your session and strives for minimal friction. Have fun, get stuff done and let the agents whisper their arcane knowledge!
