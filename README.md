# grimux 😈

Grimux is a playful tmux REPL obsessed with buffers, panes and mischievous hacking. It rides alongside your tmux session capturing pane output, piping commands through `$EDITOR`, rendering markdown via `batcat` (or `$VIEWER`) and generally encouraging outrageous experimentation.

## Why Buffers and Panes?
Buffers are named scratch spaces like `%file`, `%code`, `%@` and whatever else you dream up. Commands read from and write to these buffers so you can chain actions together. Panes are referenced by their tmux id (e.g. `%1`) and can be captured into a buffer with `!observe`. Once text is in a buffer you can run, edit or send it wherever you like.

## Commands at a Glance
- `!ls` – list panes and buffers
- `!observe <buffer> <pane>` – capture a pane into a buffer
- `!cat <buf1> [buf2 ...]` – print one or more buffers
- `!set <buffer> <text>` – store text (expands pane and buffer refs)
- `!run [buffer] <cmd>` – run a shell command, store output
- `!gen <buffer> <prompt>` – ask the AI and store the reply
- `!code <buffer> <prompt>` – like `!gen` but keep only the last code block
- `!rand <min> <max> <buffer>` – random number helper
- `!game` – silly number guessing diversion
- `!edit <buffer>` – open buffer in `$EDITOR` (defaults to vim)
- `!save <buffer> <file>` / `!file <path> [buf]` – load and save files
- `!session` – stash current session json in `%session`
- `!help` – list every command

Every command (except `!game`) dumps its output into the special `%@` buffer so you can immediately use it elsewhere.

## tmux Tips
Make sure `tmux` is running before starting grimux. Splitting panes lets you capture output from one and script it in another. Grimux leans heavily on tmux IDs so get used to `C-b q` to show them.

## Editors and Viewers
Grimux respects the `$EDITOR` and `$VIEWER` environment variables. Markdown replies are shown through `batcat` unless `$VIEWER` says otherwise. Editing buffers launches `$EDITOR`, typically vim.

## Building and Testing
```bash
go build ./cmd/grimux
go test ./...
```

Grimux is all about low friction, composable actions and keeping hacking fun. It’s a swiss army knife for professional mischief-makers who live in the terminal. Fire it up, poke around and enjoy the ride!
