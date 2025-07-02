package repl

import (
	"fmt"
	"strings"
)

// helpListener intercepts '?' key presses to display inline help.
type helpListener struct{}

func (h *helpListener) OnChange(line []rune, pos int, key rune) ([]rune, int, bool) {
	switch key {
	case '?':
	case 6: // Ctrl+F
		return []rune("!grep "), len("!grep "), true
	case 19: // Ctrl+S
		return []rune("!save "), len("!save "), true
	case 15: // Ctrl+O
		return []rune("!load "), len("!load "), true
	case 24: // Ctrl+X
		fmt.Println()
		handleCommand("!x")
		forceEnter()
		return []rune{}, 0, true
	case 4: // Ctrl+D
		fmt.Println()
		handleCommand("!quit")
		forceEnter()
		return []rune{}, 0, true
	default:
		return nil, 0, false
	}
	before := strings.TrimSpace(string(line[:pos]))
	if before != "" && !strings.HasPrefix(before, "!") {
		return nil, 0, false
	}
	if pos > 0 {
		line = append(line[:pos-1], line[pos:]...)
		pos--
	}
	trimmed := strings.TrimSpace(string(line))
	fmt.Println()
	if trimmed == "" {
		handleCommand("!help")
	} else if strings.HasPrefix(trimmed, "!") {
		fields := strings.Fields(trimmed)
		name := fields[0]
		if info, ok := commands[name]; ok {
			cmdPrintln(info.Usage + " - " + info.Desc)
			for _, p := range info.Params {
				cmdPrintln(fmt.Sprintf("  %s - %s", p.Name, p.Desc))
			}
		}
	}
	forceEnter()
	return line, pos, true
}
