Grimux strives for low-friction, composable hacking rituals. Commands interact through buffers so you can capture text from tmux panes, transform it, feed it to the AI or run it through your shell. The key commands are:

- `!ls`, `!observe`, `!cat`, `!set`, `!run`, `!gen`, `!code`, `!rand`, `!game`, `!edit`, `!save`, `!file`, `!session` and more.
- Output from commands lands in `%@` for further use.
- The whimsical REPL is designed to keep offensive security researchers in flow.
- High scores from `!game` persist in the session.

Primary goals: minimal friction, composable actions, a hint of ritual and hacker mind ergonomics. Have fun and get things done.

## Architecture notes for Codex

- The project is a REPL implemented in `internal/repl`. All state lives in in-memory buffers.
- Commands and external resources (tmux panes, files, the OpenAI API) are treated as sources or sinks for these buffers.
- The main entry point is `cmd/grimux/main.go`; supporting packages live under `internal/`:
  - `openai` handles API calls and configuration prompts.
  - `input` wraps interactive readline helpers.
  - `tmux` deals with pane capture and keystroke injection.
- Nothing is persisted except through the session save file. High scores and audit logs are stored in memory until saved.
- When modifying code or docs you **must** run `go test ./...` before committing.
- Keep the focus on composable workflows; new features should read from and write to buffers where possible.
