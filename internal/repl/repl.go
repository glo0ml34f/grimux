package repl

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"github.com/example/grimux/internal/tmux"
)

var capturePane = tmux.CapturePane

var panePattern = regexp.MustCompile(`/\{%(\d+)\}/`)

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
		return "```\n" + content + "```"
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
	lineBuf := bytes.Buffer{}

	prompt := func() {
		fmt.Print("grimux> ")
	}

	clearScreen := func() {
		fmt.Print("\033[H\033[2J")
	}

	autocomplete := func(prefix string) {
		cmds := []string{"!capture", "!list", "!quit"}
		for _, c := range cmds {
			if strings.HasPrefix(c, prefix) {
				fmt.Println()
				fmt.Println(c)
			}
		}
		prompt()
		fmt.Print(prefix)
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
				break
			}
		}
		prompt()
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
			} else {
				fmt.Println(replacePaneRefs(line))
				history = append(history, line)
			}
			prompt()
		case 12: // Ctrl+L
			clearScreen()
			prompt()
		case 3: // Ctrl+C
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
			autocomplete(lineBuf.String())
		case 18: // Ctrl+R reverse search
			reverseSearch()
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
	default:
		fmt.Println("unknown command")
	}
	return false
}
