package repl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/example/grimux/internal/openai"
	"github.com/example/grimux/internal/tmux"
)

var capturePane = tmux.CapturePane

const asciiArt = "\033[1;36m" + `
  ____ ____ ____ ____ _________ ____ ____ ____ ____
 ||g |||r |||i |||m |||       |||u |||x |||  |||  ||
 ||__|||__|||__|||__|||_______|||__|||__|||__|||__||
 |/__\|/__\|/__\|/__\|/_______\|/__\|/__\|/__\|/__\|
` + "\033[0m"

var prompts = []string{
	"In a haze of eternal night, grumble about being woken for more human nonsense",
	"As a whimsical witch-hacker, sigh at mortals poking around your terminal",
	"With playful disdain, lament yet another ridiculous request from the living",
}

var panePattern = regexp.MustCompile(`\{\%(\d+)\}`)

var bufferPattern = regexp.MustCompile(`%[a-zA-Z0-9_]+`)
var codeBlockPattern = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]+)\n(.*?)\n```")
var buffers = map[string]string{
	"%file": "",
	"%code": "",
}

var history []string

var sessionFile string
var sessionName string
var sessionPass string

// askPrefix is prepended to user prompts when using !ask.
var askPrefix = "You are a offensive security co-pilot, please answer the following prompt with high technical accuracy from a pentesting angle. Please response to the following prompt using hacker lingo and use pithy markdown with liberal emojis: "

type session struct {
	History []string          `json:"history"`
	Buffers map[string]string `json:"buffers"`
	Prompt  string            `json:"prompt"`
	APIKey  string            `json:"apikey"`
}

const (
	grimColor    = "\033[38;5;141m" // internal grimux messages
	cmdColor     = "\033[38;5;51m"  // user commands
	respColor    = "\033[38;5;205m" // LLM responses
	successColor = "\033[38;5;82m"  // success messages
	warnColor    = "\033[38;5;196m" // warnings or important prompts
)

func colorize(color, s string) string { return color + s + "\033[0m" }
func cprintln(s string)               { fmt.Println(colorize(grimColor, s)) }
func cprint(s string)                 { fmt.Print(colorize(grimColor, s)) }
func cmdPrintln(s string)             { fmt.Println(colorize(cmdColor, s)) }
func respPrintln(s string)            { fmt.Println(colorize(respColor, s)) }
func successPrintln(s string)         { fmt.Println(colorize(successColor, s)) }
func warnPrintln(s string)            { fmt.Println(colorize(warnColor, s)) }
func ok() string                      { return colorize(successColor, "✅") }

var respSep = colorize(respColor, strings.Repeat("─", 40))

// SetSessionFile changes the path used when loading or saving session state.
func SetSessionFile(path string) {
	if path != "" {
		sessionFile = path
		sessionName = strings.TrimSuffix(filepath.Base(path), ".grimux")
	}
}

type paramInfo struct {
	Name string
	Desc string
}

type commandInfo struct {
	Usage  string
	Desc   string
	Params []paramInfo
}

var commandOrder = []string{
	"!capture", "!list", "!quit", "!exit", "!ask", "!save",
	"!var", "!varcode", "!file", "!edit", "!run", "!print",
	"!prompt", "!set_prompt", "!get_prompt", "!session", "!run_on", "!help",
}

var commands = map[string]commandInfo{
	"!quit":       {Usage: "!quit", Desc: "save session and quit"},
	"!exit":       {Usage: "!exit", Desc: "exit immediately"},
	"!list":       {Usage: "!list", Desc: "list panes and buffers"},
	"!capture":    {Usage: "!capture <buffer> <pane-id>", Desc: "capture a pane into a buffer", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<pane-id>", "tmux pane id"}}},
	"!save":       {Usage: "!save <buffer> <file>", Desc: "save buffer to file", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<file>", "path to file"}}},
	"!file":       {Usage: "!file <path>", Desc: "load file into %file", Params: []paramInfo{{"<path>", "file path"}}},
	"!edit":       {Usage: "!edit <buffer>", Desc: "edit buffer in $EDITOR", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!run":        {Usage: "!run <command>", Desc: "run shell command", Params: []paramInfo{{"<command>", "command to run"}}},
	"!var":        {Usage: "!var <buffer> <prompt>", Desc: "AI prompt into buffer", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<prompt>", "text prompt"}}},
	"!varcode":    {Usage: "!varcode <buffer> <prompt>", Desc: "AI prompt, store code", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<prompt>", "text prompt"}}},
	"!print":      {Usage: "!print <buffer>", Desc: "print buffer contents", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!prompt":     {Usage: "!prompt <buffer> <text>", Desc: "store text in buffer", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<text>", "text to store"}}},
	"!set_prompt": {Usage: "!set_prompt <buffer>", Desc: "set prefix from buffer", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!get_prompt": {Usage: "!get_prompt", Desc: "show current prefix"},
	"!session":    {Usage: "!session", Desc: "store session JSON in %session"},
	"!run_on":     {Usage: "!run_on <buffer> <pane> <cmd>", Desc: "run command using pane capture", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<pane>", "pane to read"}, {"<cmd>", "command"}}},
	"!ask":        {Usage: "!ask <prompt>", Desc: "ask the AI with prefix", Params: []paramInfo{{"<prompt>", "text prompt"}}},
	"!help":       {Usage: "!help", Desc: "show this help"},
}

func replacePaneRefs(text string) string {
	return panePattern.ReplaceAllStringFunc(text, func(tok string) string {
		m := panePattern.FindStringSubmatch(tok)
		if len(m) < 2 {
			return tok
		}
		id := "%" + m[1]
		content, err := capturePane(id)
		if err != nil {
			return fmt.Sprintf("[capture error: %v]", err)
		}
		content = strings.TrimSpace(content)
		return "\n```\n" + content + "\n```\n"
	})
}

func replaceBufferRefs(text string) string {
	return bufferPattern.ReplaceAllStringFunc(text, func(tok string) string {
		if val, ok := buffers[tok]; ok {
			return val
		}
		return tok
	})
}

func lastCodeBlock(text string) string {
	matches := codeBlockPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][2]
}

var requiredBins = []string{"tmux", "vim", "batcat", "bash"}

func checkDeps() error {
	for _, b := range requiredBins {
		cprint(fmt.Sprintf("Checking %s... ", b))
		if _, err := exec.LookPath(b); err != nil {
			cprintln("❌")
			return fmt.Errorf("missing dependency: %s", b)
		}
		cprintln(ok())
	}
	cprintln("Flux capacitor charged... " + ok())
	return nil
}

// spinner displays a cute "thinking" indicator until the returned function is
// called. It rewrites the same line without printing newlines.
func spinner() func() {
	frames := []string{"😈", "👿", "😈", "🤔"}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-done:
				fmt.Print("\r\033[K")
				close(finished)
				return
			default:
				fmt.Printf("\r%s", colorize(respColor, frames[i%len(frames)]))
				time.Sleep(300 * time.Millisecond)
				i++
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
}

// forceEnter prints a newline to ensure the prompt is visible after
// running external commands while in raw mode.
func forceEnter() {
	fmt.Print("\r\n")
}

// bootScreen displays a campy supercomputer boot screen.
func bootScreen() {
	lines := []string{
		"Initializing Darkstar AI Core...",
		"Loading neural subroutines... " + ok(),
		"Calibrating sarcasm engines... " + ok(),
		"Quantum flux capacitor stable... " + ok(),
		colorize(warnColor, "PRESS RETURN IF YOU DARE"),
	}
	for _, l := range lines {
		cprintln(l)
	}
}

// startRaw puts the terminal into raw mode.

func readPassword() (string, error) {
	old, err := startRaw()
	if err != nil {
		return "", err
	}
	defer stopRaw(old)
	reader := bufio.NewReader(os.Stdin)
	var buf bytes.Buffer
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return "", err
		}
		if r == '\n' || r == '\r' {
			break
		}
		buf.WriteRune(r)
	}
	fmt.Println()
	return buf.String(), nil
}

// Run launches the interactive REPL.
func Run() error {
	if err := checkDeps(); err != nil {
		return err
	}
	// load session before switching to raw mode
	reader := bufio.NewReader(os.Stdin)
	history = []string{}
	buffers = map[string]string{"%file": "", "%code": ""}
	if sessionFile != "" {
		if b, err := os.ReadFile(sessionFile); err == nil {
			var s session
			trimmed := bytes.TrimSpace(b)
			if len(trimmed) > 0 && trimmed[0] != '{' {
				cprint("Password: ")
				pwd, _ := readPassword()
				sessionPass = pwd
				dec, err := decryptData(trimmed, sessionPass)
				if err == nil {
					if dec, err = decompressData(dec); err == nil {
						json.Unmarshal(dec, &s)
					}
				}
			} else {
				json.Unmarshal(trimmed, &s)
			}
			history = s.History
			buffers = s.Buffers
			if buffers == nil {
				buffers = map[string]string{"%file": "", "%code": ""}
			}
			if s.Prompt != "" {
				askPrefix = s.Prompt
			}
			if s.APIKey != "" && os.Getenv("OPENAI_API_KEY") == "" {
				openai.SetSessionAPIKey(s.APIKey)
			}
		}
	}
	if sessionFile != "" && sessionName == "" {
		sessionName = strings.TrimSuffix(filepath.Base(sessionFile), ".grimux")
	}

	oldState, err := startRaw()
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer stopRaw(oldState)
	defer cprintln("So long, and thanks for all the hacks! 🤘")

	reader = bufio.NewReader(os.Stdin)
	histIdx := 0
	lineBuf := bytes.Buffer{}
	lastQuestion := false

	cprintln(asciiArt + "\nWelcome to grimux! 💀")
	cprintln("Press Tab for auto-completion. Type !help for more info.")

	rand.Seed(time.Now().UnixNano())
	if client, err := openai.NewClient(); err != nil {
		cprintln("⚠️  " + err.Error())
	} else {
		p := prompts[rand.Intn(len(prompts))]
		stop := spinner()
		reply, err := client.SendPrompt(p + "and please keep your response short, pithy, and funny")
		stop()
		if err == nil {
			cprintln("Checking OpenAI integration... " + ok())
			respPrintln(respSep)
			respPrintln(reply)
			forceEnter()
			respPrintln(respSep)
			bootScreen()
		} else {
			cprintln("openai error: " + err.Error())
		}
	}

	prompt := func() {
		if sessionName != "" {
			fmt.Printf("\033[1;35mgrimux(%s)😈> \033[0m", sessionName)
		} else {
			fmt.Print("\033[1;35mgrimux😈> \033[0m")
		}
	}

	clearScreen := func() {
		fmt.Print("\033[H\033[2J")
	}

	printLine := func() {
		fmt.Print("\r\033[K")
		prompt()
		fmt.Print(lineBuf.String())
	}

	autocomplete := func() {
		prefix := lineBuf.String()
		fields := strings.Fields(prefix)
		if len(fields) > 0 && (fields[0] == "!save" || fields[0] == "!file") {
			if len(fields) >= 2 && !strings.HasSuffix(prefix, " ") {
				pattern := fields[len(fields)-1] + "*"
				matches, _ := filepath.Glob(pattern)
				if len(matches) == 1 {
					fields[len(fields)-1] = matches[0]
					lineBuf.Reset()
					lineBuf.WriteString(strings.Join(fields, " "))
					printLine()
					return
				}
				if len(matches) > 1 {
					cprintln("")
					cprintln(strings.Join(matches, "  "))
					printLine()
					return
				}
			}
		}
		matches := []string{}
		for _, c := range commandOrder {
			if strings.HasPrefix(c, prefix) {
				matches = append(matches, c)
			}
		}
		if len(matches) == 0 {
			return
		}
		if len(matches) == 1 {
			lineBuf.Reset()
			lineBuf.WriteString(matches[0])
			printLine()
			return
		}
		cprintln("")
		cprintln(strings.Join(matches, "  "))
		printLine()
	}

	reverseSearch := func() {
		fmt.Print("\n(reverse-i-search)")
		query := ""
		for {
			r, _, err := reader.ReadRune()
			if err != nil {
				return
			}
			if r == '\n' || r == '\r' {
				break
			}
			query += string(r)
		}
		for i := len(history) - 1; i >= 0; i-- {
			if strings.Contains(history[i], query) {
				cprint("\n")
				cprintln(history[i])
				lineBuf.Reset()
				lineBuf.WriteString(history[i])
				histIdx = i
				break
			}
		}
		printLine()
	}

	paramHelp := func() bool {
		line := lineBuf.String()
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0][0] != '!' {
			return false
		}
		info, ok := commands[fields[0]]
		if !ok {
			return false
		}
		var idx int
		if len(fields) == 1 {
			idx = 0
		} else if strings.HasSuffix(line, " ") {
			idx = len(fields) - 1
		} else {
			idx = len(fields) - 2
		}
		if idx < 0 || idx >= len(info.Params) {
			return false
		}
		p := info.Params[idx]
		cprintln("")
		cprintln(p.Name + " - " + p.Desc)
		printLine()
		return true
	}

	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return err
		}
		switch r {
		case '\n', '\r':
			lastQuestion = false
			line := lineBuf.String()
			lineBuf.Reset()
			cprintln("")
			if len(line) == 0 {
				prompt()
				continue
			}
			if line == string(rune(12)) { // ctrl+l
				clearScreen()
				fmt.Println()
				prompt()
				continue
			}
			if line[0] == '!' {
				if handleCommand(line) {
					return nil
				}
				history = append(history, line)
				histIdx = len(history)
			} else {
				client, err := openai.NewClient()
				if err != nil {
					cmdPrintln(err.Error())
				} else {
					promptText := replaceBufferRefs(replacePaneRefs(line))
					stop := spinner()
					reply, err := client.SendPrompt(promptText)
					stop()
					if err != nil {
						cprintln("openai error: " + err.Error())
					} else {
						respPrintln(respSep)
						respPrintln(reply)
						respPrintln(respSep)
						buffers["%code"] = lastCodeBlock(reply)
						forceEnter()
					}
				}
				history = append(history, line)
				histIdx = len(history)
			}
			fmt.Println()
			prompt()
		case 12: // Ctrl+L
			lastQuestion = false
			clearScreen()
			fmt.Println()
			prompt()
		case 3: // Ctrl+C
			lastQuestion = false
			cprintln("")
			return nil
		case 4: // Ctrl+D
			lastQuestion = false
			cprintln("")
			return nil
		case 127: // Backspace
			if lineBuf.Len() > 0 {
				lastQuestion = false
				buf := lineBuf.Bytes()
				lineBuf.Reset()
				lineBuf.Write(buf[:len(buf)-1])
				fmt.Print("\b \b")
			}
		case 9: // Tab
			lastQuestion = false
			autocomplete()
		case 18: // Ctrl+R reverse search
			lastQuestion = false
			reverseSearch()
		case '?':
			if lastQuestion || !paramHelp() {
				lineBuf.WriteRune('?')
				cprint("?")
				lastQuestion = false
			} else {
				lastQuestion = true
			}
		case 27: // escape sequences (arrows or alt-digit)
			lastQuestion = false
			next1, _, err := reader.ReadRune()
			if err != nil {
				return err
			}
			if next1 >= '0' && next1 <= '9' {
				token := fmt.Sprintf("{%%%c}", next1)
				lineBuf.WriteString(token)
				cprint(token)
				continue
			}
			if next1 != '[' {
				continue
			}
			next2, _, err := reader.ReadRune()
			if err != nil {
				return err
			}
			switch next2 {
			case 'A': // Up arrow
				if histIdx > 0 {
					histIdx--
					lineBuf.Reset()
					lineBuf.WriteString(history[histIdx])
					printLine()
				}
			case 'B': // Down arrow
				if histIdx < len(history)-1 {
					histIdx++
					lineBuf.Reset()
					lineBuf.WriteString(history[histIdx])
					printLine()
				} else if histIdx == len(history)-1 {
					histIdx = len(history)
					lineBuf.Reset()
					printLine()
				}
			}
		default:
			lastQuestion = false
			lineBuf.WriteRune(r)
			cprint(string(r))
		}
	}
}

// handleCommand executes a ! command. Returns true if repl should quit.
func saveSession() {
	reader := bufio.NewReader(os.Stdin)
	if sessionFile == "" {
		cprint("Session name (blank for hidden): ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)
		if name != "" {
			sessionFile = name + ".grimux"
			sessionName = name
		} else {
			sessionFile = ".grimux_session"
		}
	}
	if sessionPass == "" {
		cprint("Password: ")
		pwd, _ := readPassword()
		sessionPass = pwd
	}
	s := session{History: history, Buffers: buffers, Prompt: askPrefix, APIKey: openai.GetSessionAPIKey()}
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		if sessionPass == "" {
			os.WriteFile(sessionFile, b, 0644)
			return
		}
		comp, err := compressData(b)
		if err != nil {
			return
		}
		enc, err := encryptData(comp, sessionPass)
		if err != nil {
			return
		}
		os.WriteFile(sessionFile, enc, 0644)
	}
}

func handleCommand(cmd string) bool {
	fields := strings.Fields(cmd)
	usage := func(name string) {
		if info, ok := commands[name]; ok {
			cmdPrintln("usage: " + info.Usage + " - " + info.Desc)
		}
	}
	switch fields[0] {
	case "!quit":
		saveSession()
		return true
	case "!exit":
		return true
	case "!list":
		c := exec.Command("tmux", "list-panes", "-F", "#{pane_id} #{pane_title} #{pane_current_command}")
		c.Stdout = os.Stdout
		c.Run()
		forceEnter()
		for k, v := range buffers {
			cmdPrintln(fmt.Sprintf("%s (%d bytes)", k, len(v)))
		}
	case "!capture":
		if len(fields) < 3 {
			usage("!capture")
			return false
		}
		out, err := capturePane(fields[2])
		if err != nil {
			cmdPrintln("capture error: " + err.Error())
			return false
		}
		buffers[fields[1]] = out
		cprint(out)
	case "!save":
		if len(fields) < 3 {
			usage("!save")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		if err := os.WriteFile(fields[2], []byte(data), 0644); err != nil {
			cmdPrintln("save error: " + err.Error())
		}
	case "!file":
		if len(fields) < 2 {
			usage("!file")
			return false
		}
		b, err := os.ReadFile(fields[1])
		if err != nil {
			cmdPrintln("file error: " + err.Error())
			return false
		}
		buffers["%file"] = string(b)
	case "!edit":
		if len(fields) < 2 {
			usage("!edit")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		tmp, err := os.CreateTemp("", "grimux-edit-*.tmp")
		if err != nil {
			cmdPrintln("tempfile error: " + err.Error())
			return false
		}
		if _, err := tmp.WriteString(data); err != nil {
			cmdPrintln("write temp error: " + err.Error())
			return false
		}
		tmp.Close()
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}
		cmd := exec.Command(editor, tmp.Name())
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			cmdPrintln("vim error: " + err.Error())
		}
		if b, err := os.ReadFile(tmp.Name()); err == nil {
			buffers[fields[1]] = string(b)
		}
		os.Remove(tmp.Name())
		forceEnter()
	case "!run":
		if len(fields) < 2 {
			usage("!run")
			return false
		}
		cmdStr := replaceBufferRefs(strings.Join(fields[1:], " "))
		c := exec.Command("bash", "-c", cmdStr)
		var out bytes.Buffer
		c.Stdout = &out
		c.Stderr = &out
		if err := c.Run(); err != nil {
			cmdPrintln("run error: " + err.Error())
		}
		cprint(out.String())
		forceEnter()
	case "!var":
		if len(fields) < 3 {
			usage("!var")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[2:], " ")))
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		buffers[fields[1]] = reply
		respPrintln(respSep)
		respPrintln(reply)
		respPrintln(respSep)
		forceEnter()
	case "!varcode":
		if len(fields) < 3 {
			usage("!varcode")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cprintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[2:], " ")))
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		buffers[fields[1]] = lastCodeBlock(reply)
		respPrintln(respSep)
		respPrintln(reply)
		respPrintln(respSep)
		forceEnter()
	case "!print":
		if len(fields) < 2 {
			return false
		}
		cprint(buffers[fields[1]])
	case "!prompt":
		if len(fields) < 3 {
			usage("!prompt")
			return false
		}
		buffers[fields[1]] = strings.Join(fields[2:], " ")
	case "!set_prompt":
		if len(fields) < 2 {
			usage("!set_prompt")
			return false
		}
		if v, ok := buffers[fields[1]]; ok {
			askPrefix = v
		} else {
			cmdPrintln("unknown buffer")
		}
	case "!get_prompt":
		cmdPrintln(askPrefix)
	case "!session":
		s := session{History: history, Buffers: buffers, Prompt: askPrefix}
		if b, err := json.MarshalIndent(s, "", "  "); err == nil {
			buffers["%session"] = string(b)
		}
	case "!run_on":
		if len(fields) < 4 {
			usage("!run_on")
			return false
		}
		cmdStr := replaceBufferRefs(replacePaneRefs(strings.Join(fields[3:], " ")))
		c := exec.Command("bash", "-c", cmdStr)
		var out bytes.Buffer
		c.Stdout = &out
		c.Stderr = &out
		if err := c.Run(); err != nil {
			cmdPrintln("run_on error: " + err.Error())
		}
		buffers[fields[1]] = out.String()
		forceEnter()
	case "!ask":
		if len(fields) < 2 {
			usage("!ask")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[1:], " ")))
		promptText = askPrefix + promptText
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		viewer := os.Getenv("VIEWER")
		if viewer == "" {
			viewer = "batcat"
		}
		args := []string{"-l", "markdown"}
		cmd := exec.Command(viewer, args...)
		cmd.Stdin = strings.NewReader(reply)
		cmd.Stdout = os.Stdout
		respPrintln(respSep)
		if err := cmd.Run(); err != nil {
			cmdPrintln(viewer + " error: " + err.Error())
		}
		respPrintln(respSep)
		buffers["%code"] = lastCodeBlock(reply)
		forceEnter()
	case "!help":
		for _, name := range commandOrder {
			info := commands[name]
			cmdPrintln(info.Usage + " - " + info.Desc)
		}
	default:
		cmdPrintln("unknown command")
	}
	return false
}
