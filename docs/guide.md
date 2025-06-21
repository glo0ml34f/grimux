# Grimux Comprehensive Guide

Grimux is a whimsical REPL that lives inside tmux. It helps security researchers explore systems, capture output, manipulate text, and converse with an AI assistant all without leaving the terminal. This guide expands on the README with practical examples, workflows, and a glance at what might come next.

## Why Grimux?

Security work often juggles many terminal panes and ephemeral commands. Grimux glues those panes together with buffers so you can:

- Capture output from one pane and send it into another.
- Feed commands or captured text to the AI for summarization or code generation.
- Keep everything transient until you decide to save the session.

Use it for vulnerability research, exploit development, or just keeping a tidy hacking ritual.

## Buffers and Panes Recap

Buffers are named with a `%` prefix (e.g. `%notes`, `%file`, `%@`). Commands read from and write to these buffers. Panes are your tmux windows identified by IDs like `%1` or `%2`.

Example: to capture the output of pane `%1` into buffer `%loot`:

```bash
!observe %loot %1
```

Later you can review or manipulate that text:

```bash
!cat %loot
!grep "password" %loot
```

## Example Workflow

1. **List your panes and buffers**
   ```bash
   !ls
   ```
2. **Capture interesting output**
   ```bash
   !observe %loot %1
   ```
3. **Summarize with the AI**
   ```bash
   !sum %loot
   ```
   The summary appears in `%@`, ready for the next step.
4. **Generate exploit code**
   ```bash
   !code %sploit "write a PoC for CVE-XXXX"
   ```
5. **Send to a pane for compilation or execution**
   ```bash
   !run %sploit "gcc -o sploit -xc -"
   !run %null "./sploit"
   ```

Feel free to mix in `!edit %buffer` when manual tweaks are needed.

## Command Cheat Sheet with Use Cases

Below is an expanded reference with ideas for how each command might aid your research.

### Session and Navigation

- `!ls` – show panes and buffers. Handy when you forget which buffer name you used.
- `!quit` / `!x` – exit Grimux (`!quit` saves the session first).
- `!cd <dir>` and `!pwd` – move around the filesystem within the REPL.
- `!session` – dump the entire session as JSON into `%session` for archiving.

### Buffer Management

- `!observe <buf> <pane>` – capture a pane's visible text. Great for grabbing compiler output or command results.
- `!eat <buf> <pane>` – slurp the full scrollback for deep logs.
- `!cat <buf>` – display buffer contents.
- `!edit <buf>` – open `$EDITOR` to modify text.
- `!save <buf> <file>` / `!file <path> [buf]` – move between buffers and files.
- `!unset <buf>` / `!rm <buf>` – clear or remove a buffer when done.
- `%null` – special buffer that discards writes and always reads empty.
- `!grep <regex> [buffers...]` – search through your captured data.

### Running Commands

- `!run [buf] <cmd>` – execute a shell command, optionally piping in a buffer. Use this to compile code or run enumeration scripts.
- `!run_on <buf> <pane> <cmd>` – capture a pane, run a command with that capture as input, store output in `<buf>`.
- `!nc <buf> <args>` – pipe a buffer to netcat. Convenient for sending crafted payloads.
- `!curl <url> [buf] [hdrs]` – fetch a URL into a buffer, optionally using headers from `hdrs`.
- `!diff <a> <b> [buf]` – show a colorized diff between buffers or files.
- `!recap` – summarize the session.

### AI Integration

- `!gen <buf> <prompt>` – general purpose prompts to the AI. The response lands in `<buf>`.
- `!code <buf> <prompt>` – specifically ask the AI for code and store it.
- `!a <prompt>` – quick ask with your current prefix (see `!prefix`).
- `!sum <buf>` – summarize long output, such as logs or disassembly.
- `!helpme <question>` – ask for help about Grimux itself.
- `!model <name>` – change the OpenAI model if you have access to others.

### Environment and Utility

- `!setenv <var> <buf>` / `!getenv <var> <buf>` – bridge environment variables and buffers.
- `!rand <min> <max> <buf>` – generate a random integer; useful for filenames or tokens.
- `!ascii <buf>` – render gothic ASCII art of the first few words for flair.
- `!view <buf>` – open a viewer (default `batcat`) for nicer reading of long text.
- `!version` – print Grimux's version.
- `!game` – take a short break with a mini‑game; high scores persist in memory until you save.

### Prompt Prefixes

Set context for the AI with a prefix so you don't need to repeat yourself.

```bash
!prefix %file          # load prefix from file into the session
!a "summarize the binary protocol"
```

Use `!get_prompt` to show the current prefix, and `!reset` to clear it along with session state.

## Tips and Tricks

- Buffers can reference panes by using `{%1}` syntax inside prompts. This inlines the captured text when sending prompts to the AI.
- The hotkeys `Ctrl+G` or hitting `Escape` start a command quickly, keeping your hands on the keyboard.
- Chain commands using `!flow %a %b %c` to pipe the AI's output through multiple buffers.
- Play with the included persona prompts in the `prompts/` directory to change the AI's tone: `!prefix prompts/red_team.txt`.

## Finding Your Workflow

1. Split tmux panes for each major task: one for editing, one for running code, another for logs.
2. Use Grimux buffers as the glue to shuttle data between these panes.
3. Keep the AI handy for summarizing results or drafting exploits.
4. Save sessions when you want a record, otherwise enjoy the ephemeral nature during exploration.

Experiment to see which buffers you like to keep around (e.g. `%loot`, `%notes`, `%sploit`). Over time you'll build rituals that make sense for your own research style.

## Roadmap Ideas

While Grimux already covers the basics of buffer manipulation and AI integration, future enhancements could include:

- **Macro recording** to replay common sequences of commands.
- **Remote pane capture** for gathering output from tmux sessions on other hosts.
- **Graphical session viewer** that renders buffers in an HTML dashboard.
- **Integration with other LLM providers** for more model choices.
- **Collaborative mode** where buffers sync across multiple users.
- **Plugin examples** showcasing custom commands tailored for specific toolkits.
- **Buffer diffing** to compare different runs of exploit attempts.

Community feedback will help shape which of these ideas become reality. Contributions are welcome!

