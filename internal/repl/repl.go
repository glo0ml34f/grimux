package repl

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unsafe"

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

// startRaw puts the terminal into raw mode.
func startRaw() (*syscall.Termios, error) {
	fd := int(os.Stdin.Fd())
	var old syscall.Termios
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&old)), 0, 0, 0); err != 0 {
		return nil, err
	}
	newState := old
	newState.Lflag &^= syscall.ICANON | syscall.ECHO
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&newState)), 0, 0, 0); err != 0 {
		return nil, err
	}
	return &old, nil
}

// stopRaw restores the terminal to a previous state.
func stopRaw(state *syscall.Termios) {
	if state == nil {
		return
	}
	fd := int(os.Stdin.Fd())
	syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(state)), 0, 0, 0)
}

// Run launches the interactive REPL.
func Run() error {
	oldState, err := startRaw()
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer stopRaw(oldState)

	reader := bufio.NewReader(os.Stdin)
	history := []string{}
	histIdx := 0
	lineBuf := bytes.Buffer{}

	fmt.Println(asciiArt + "\nWelcome to grimux! 💀")

	rand.Seed(time.Now().UnixNano())
	if client, err := openai.NewClient(); err == nil {
		p := prompts[rand.Intn(len(prompts))]
		if reply, err := client.SendPrompt(p); err == nil {
			fmt.Println(reply)
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
		cmds := []string{"!capture", "!list", "!quit", "!ask"}
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
		fmt.Println()
		fmt.Println(strings.Join(matches, "  "))
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
				fmt.Printf("\n%s\n", history[i])
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
			fmt.Println()
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
					fmt.Println(err)
				} else {
					promptText := replacePaneRefs(line)
					reply, err := client.SendPrompt(promptText)
					if err != nil {
						fmt.Println("openai error:", err)
					} else {
						fmt.Println(reply)
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
			fmt.Println()
			return nil
		case 4: // Ctrl+D
			fmt.Println()
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
		case 27: // escape sequences (arrows)
			next1, _, err := reader.ReadRune()
			if err != nil {
				return err
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
			fmt.Printf("%c", r)
		}
	}
}

// handleCommand executes a ! command. Returns true if repl should quit.
func handleCommand(cmd string) bool {
	fields := strings.Fields(cmd)
	switch fields[0] {
	case "!quit", "!exit":
		return true
	case "!list":
		c := exec.Command("tmux", "list-panes", "-F", "#{pane_id} #{pane_title}")
		c.Stdout = os.Stdout
		c.Run()
	case "!capture":
		if len(fields) < 2 {
			fmt.Println("usage: !capture <pane-id>")
			return false
		}
		out, err := exec.Command(os.Args[0], "-capture", fields[1]).Output()
		if err != nil {
			fmt.Println("capture error:", err)
			return false
		}
		fmt.Print(string(out))
	case "!ask":
		if len(fields) < 2 {
			fmt.Println("usage: !ask <prompt>")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			fmt.Println(err)
			return false
		}
		promptText := replacePaneRefs(strings.Join(fields[1:], " "))
		promptText = "You are a offensive security co-pilot, please answer the following prompt with high technical accuracy from a pentesting angle. Please response to the following prompt using hacker lingo and use pithy markdown with liberal emojis: " + promptText
		reply, err := client.SendPrompt(promptText)
		if err != nil {
			fmt.Println("openai error:", err)
			return false
		}
		cmd := exec.Command("batcat", "-l", "markdown")
		cmd.Stdin = strings.NewReader(reply)
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			fmt.Println("batcat error:", err)
		}
	default:
		fmt.Println("unknown command")
	}
	return false
}
