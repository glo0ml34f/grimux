# grimux 😈
A grimey tmux sidekick for offensive security research.

## About 🚀
`grimux` is no longer a tiny proof of concept. It is a full blown REPL that latches on to your tmux session, captures panes and slings prompts at an LLM. It exists to aid pentesters and curious hackers who need a grumpy AI assistant that answers succinctly and with style.

When started it shows some ASCII art, checks OpenAI connectivity and spits out a random complaint from the void. Every interaction tries to keep replies brief—one spicy sentence if possible.

## Buffers ✨ (The Real Magic)
Buffers are named scratch pads like `%file`, `%code` or anything you create. Commands can fill them with pane captures, AI replies or your own text. You can pipe buffers to files, edit them in `$EDITOR`, or feed them back into new prompts. Think of them as a hacker grimoire: snippets, notes and payloads ready at a moment’s notice.

## Features 💥
* Capture tmux panes straight into a buffer with `!observe`.
* Ask the AI questions with `!a` and view the markdown nicely rendered.
* Store and run shell commands that reference buffers.
* Load files into `%file` via `!load`, edit buffers, or save them anywhere.
* Horizontal separators and complimentary colors keep REPL chatter, commands and LLM responses easy to read.
* A cute status spinner appears while the AI thinks and vanishes once it answers.
* Sessions persist so your buffers survive restarts.

## Building 🔧
```bash
go build ./cmd/grimux
```

## Example Workflow 🎬
1. Fire up tmux and split some panes.
2. Run `grimux` in one pane and issue `!ls` to see buffers and pane IDs.
3. Capture another pane’s text with `!observe %loot %1`.
4. Summon the AI with `!gen %analysis give me a quick summary`.
5. Use `cat %analysis` or save it with `!save %analysis report.md`.

## Running tests 🧪
```bash
go test ./...
```

Have fun spelunking through your terminal! When you exit, grimux waves goodbye with a snarky grin. 😜
