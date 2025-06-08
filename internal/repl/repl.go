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
	"Give me a pithy complaint about being bothered with nonsense",
	"Provide a short gripe about having to deal with nonsense",
	"What's a witty moan about pointless nonsense?",
}

var panePattern = regexp.MustCompile(`\{\%(\d+)\}`)

var bufferPattern = regexp.MustCompile(`%[a-zA-Z0-9_]+`)
var codeBlockPattern = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]+)\n(.*?)\n```")
var buffers = map[string]string{
	"%file": "",
	"%code": "",
}

var history []string

const sessionFile = ".grimux_session"

var askPrefix = "You are a offensive security co-pilot, please answer the following prompt with high technical accuracy from a pentesting angle. Please response to the following prompt using hacker lingo and use pithy markdown with liberal emojis: "

type session struct {
	History []string          `json:"history"`
	Buffers map[string]string `json:"buffers"`
	Prompt  string            `json:"prompt"`
}

const grimColor = "\033[38;5;141m"

func colorize(s string) string { return grimColor + s + "\033[0m" }
func cprintln(s string)        { fmt.Println(colorize(s)) }
func cprint(s string)          { fmt.Print(colorize(s)) }

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

// startRaw puts the terminal into raw mode.

// Run launches the interactive REPL.
func Run() error {
	oldState, err := startRaw()
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer stopRaw(oldState)

	reader := bufio.NewReader(os.Stdin)
	history = []string{}
	if b, err := os.ReadFile(sessionFile); err == nil {
		var s session
		if json.Unmarshal(b, &s) == nil {
			history = s.History
			buffers = s.Buffers
			if s.Prompt != "" {
				askPrefix = s.Prompt
			}
		}
	}
	histIdx := 0
	lineBuf := bytes.Buffer{}

	cprintln(asciiArt + "\nWelcome to grimux! 💀")

	rand.Seed(time.Now().UnixNano())
	if client, err := openai.NewClient(); err == nil {
		p := prompts[rand.Intn(len(prompts))]
		if reply, err := client.SendPrompt(p); err == nil {
			cprintln(reply)
		}
	}

	prompt := func() {
		fmt.Print("\033[1;35mgrimux😈> \033[0m")
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
		cmds := []string{"!capture", "!list", "!quit", "!exit", "!ask", "!save", "!var", "!varcode", "!file", "!edit", "!run", "!print", "!prompt", "!set_prompt", "!get_prompt", "!run_on"}
		matches := []string{}
		for _, c := range cmds {
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

	prompt()
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return err
		}
		switch r {
		case '\n', '\r':
			line := lineBuf.String()
			lineBuf.Reset()
			cprintln("")
			if len(line) == 0 {
				prompt()
				continue
			}
			if line == string(rune(12)) { // ctrl+l
				clearScreen()
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
					cprintln(err.Error())
				} else {
					promptText := replaceBufferRefs(replacePaneRefs(line))
					reply, err := client.SendPrompt(promptText)
					if err != nil {
						cprintln("openai error: " + err.Error())
					} else {
						cprintln(reply)
						buffers["%code"] = lastCodeBlock(reply)
					}
				}
				history = append(history, line)
				histIdx = len(history)
			}
			prompt()
		case 12: // Ctrl+L
			clearScreen()
			prompt()
		case 3: // Ctrl+C
			cprintln("")
			return nil
		case 4: // Ctrl+D
			cprintln("")
			return nil
		case 127: // Backspace
			if lineBuf.Len() > 0 {
				buf := lineBuf.Bytes()
				lineBuf.Reset()
				lineBuf.Write(buf[:len(buf)-1])
				fmt.Print("\b \b")
			}
		case 9: // Tab
			autocomplete()
		case 18: // Ctrl+R reverse search
			reverseSearch()
		case 27: // escape sequences (arrows or alt-digit)
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
			lineBuf.WriteRune(r)
			cprint(string(r))
		}
	}
}

// handleCommand executes a ! command. Returns true if repl should quit.
func saveSession() {
	s := session{History: history, Buffers: buffers, Prompt: askPrefix}
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		os.WriteFile(sessionFile, b, 0644)
	}
}

func handleCommand(cmd string) bool {
	fields := strings.Fields(cmd)
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
		for k, v := range buffers {
			cprintln(fmt.Sprintf("%s (%d bytes)", k, len(v)))
		}
	case "!capture":
		if len(fields) < 3 {
			cprintln("usage: !capture <buffer> <pane-id>")
			return false
		}
		out, err := exec.Command(os.Args[0], "-capture", fields[2]).Output()
		if err != nil {
			cprintln("capture error: " + err.Error())
			return false
		}
		buffers[fields[1]] = string(out)
		cprint(string(out))
	case "!save":
		if len(fields) < 3 {
			cprintln("usage: !save <buffer> <file>")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cprintln("unknown buffer")
			return false
		}
		if err := os.WriteFile(fields[2], []byte(data), 0644); err != nil {
			cprintln("save error: " + err.Error())
		}
	case "!file":
		if len(fields) < 2 {
			cprintln("usage: !file <path>")
			return false
		}
		b, err := os.ReadFile(fields[1])
		if err != nil {
			cprintln("file error: " + err.Error())
			return false
		}
		buffers["%file"] = string(b)
	case "!edit":
		if len(fields) < 2 {
			cprintln("usage: !edit <buffer>")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cprintln("unknown buffer")
			return false
		}
		tmp, err := os.CreateTemp("", "grimux-edit-*.tmp")
		if err != nil {
			cprintln("tempfile error: " + err.Error())
			return false
		}
		if _, err := tmp.WriteString(data); err != nil {
			cprintln("write temp error: " + err.Error())
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
			cprintln("vim error: " + err.Error())
		}
		if b, err := os.ReadFile(tmp.Name()); err == nil {
			buffers[fields[1]] = string(b)
		}
		os.Remove(tmp.Name())
	case "!run":
		if len(fields) < 2 {
			cprintln("usage: !run <command>")
			return false
		}
		cmdStr := replaceBufferRefs(strings.Join(fields[1:], " "))
		c := exec.Command("bash", "-c", cmdStr)
		var out bytes.Buffer
		c.Stdout = &out
		c.Stderr = &out
		if err := c.Run(); err != nil {
			cprintln("run error: " + err.Error())
		}
		cprint(out.String())
	case "!var":
		if len(fields) < 3 {
			cprintln("usage: !var <buffer> <prompt>")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cprintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[2:], " ")))
		reply, err := client.SendPrompt(promptText)
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		buffers[fields[1]] = reply
		cprintln(reply)
	case "!varcode":
		if len(fields) < 3 {
			cprintln("usage: !varcode <buffer> <prompt>")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cprintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[2:], " ")))
		reply, err := client.SendPrompt(promptText)
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		buffers[fields[1]] = lastCodeBlock(reply)
		cprintln(reply)
	case "!print":
		if len(fields) < 2 {
			cprintln("usage: !print <buffer>")
			return false
		}
		cprint(buffers[fields[1]])
	case "!prompt":
		if len(fields) < 3 {
			cprintln("usage: !prompt <buffer> <text>")
			return false
		}
		buffers[fields[1]] = strings.Join(fields[2:], " ")
	case "!set_prompt":
		if len(fields) < 2 {
			cprintln("usage: !set_prompt <buffer>")
			return false
		}
		if v, ok := buffers[fields[1]]; ok {
			askPrefix = v
		} else {
			cprintln("unknown buffer")
		}
	case "!get_prompt":
		cprintln(askPrefix)
	case "!run_on":
		if len(fields) < 4 {
			cprintln("usage: !run_on <buffer> <pane> <cmd>")
			return false
		}
		cmdStr := replaceBufferRefs(replacePaneRefs(strings.Join(fields[3:], " ")))
		c := exec.Command("bash", "-c", cmdStr)
		var out bytes.Buffer
		c.Stdout = &out
		c.Stderr = &out
		if err := c.Run(); err != nil {
			cprintln("run_on error: " + err.Error())
		}
		buffers[fields[1]] = out.String()
	case "!ask":
		if len(fields) < 2 {
			cprintln("usage: !ask <prompt>")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cprintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[1:], " ")))
		promptText = askPrefix + promptText
		reply, err := client.SendPrompt(promptText)
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		viewer := os.Getenv("VIEWER")
		if viewer == "" {
			viewer = "batcat"
		}
		cmd := exec.Command(viewer, "-l", "markdown")
		cmd.Stdin = strings.NewReader(reply)
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			cprintln(viewer + " error: " + err.Error())
		}
		buffers["%code"] = lastCodeBlock(reply)
	default:
		cprintln("unknown command")
	}
	return false
}
