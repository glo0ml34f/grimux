# grimux 🎭
A tmux-driven AI-assisted rapid prototyping interface

## About 🚀
`grimux` is a small proof-of-concept that shows how a Go CLI can talk to tmux directly. The long term idea is to build a TUI that can capture pane contents, send them to an AI backend and display the results in another pane. For now the tool just demonstrates how to capture text from another pane via tmux's UNIX socket.

## Building 🔧
```bash
go build ./cmd/grimux
```

## Capturing a pane 📋
```bash
./grimux -capture <pane-id>
```
If `<pane-id>` is omitted, the current pane is captured.
Pass `-verbose` to see detailed tmux communication logs.

## Finding pane IDs 🆔
You can discover the ID of each pane with `tmux list-panes -F '#{pane_id} #{pane_title}'`. The `#{pane_id}` values (like `%1`, `%2` ...) can then be passed to `-capture`.

## Example session 🎬
1. Start a new tmux session:
   ```bash
tmux new -s demo
```
2. Create a sample file and open it with `less` in the first pane:
   ```bash
seq 1 100 > sample.txt
less sample.txt
```
3. Split the window and build `grimux` in the second pane:
   ```bash
# in tmux: press Ctrl-b then % (or run `tmux split-window -h`)
go build ./cmd/grimux
```
4. List panes to obtain the ID of the pane running `less`:
   ```bash
tmux list-panes -F '#{pane_id} #{pane_current_command}'
```
5. Run the binary in the other pane using that ID:
   ```bash
./grimux -capture %1
```
   You should see the contents displayed in the `grimux` pane.

## Running tests 🧪
Unit tests can be executed with:
```bash
go test ./...
```
