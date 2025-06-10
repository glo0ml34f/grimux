package input

import (
	"bufio"
	"fmt"
	"os"

	"github.com/chzyer/readline"
)

var rl *readline.Instance

// SetReadline assigns the global readline instance used for interactive input.
func SetReadline(r *readline.Instance) { rl = r }

// ReadPassword reads a line from stdin without echoing the input.
func ReadPassword() (string, error) {
	if rl != nil {
		b, err := rl.ReadPassword("")
		fmt.Println()
		return string(b), err
	}
	old, err := startRaw()
	if err != nil {
		return "", err
	}
	defer stopRaw(old)
	reader := bufio.NewReader(os.Stdin)
	var buf []rune
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return "", err
		}
		if r == '\n' || r == '\r' {
			break
		}
		buf = append(buf, r)
	}
	fmt.Println()
	return string(buf), nil
}

// ReadLine reads a line from stdin echoing the input.
func ReadLine() (string, error) {
	if rl != nil {
		return rl.Readline()
	}
	reader := bufio.NewReader(os.Stdin)
	var buf []rune
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return "", err
		}
		if r == '\n' || r == '\r' {
			break
		}
		if r == 127 || r == '\b' {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Print("\b \b")
			}
			continue
		}
		buf = append(buf, r)
		fmt.Print(string(r))
	}
	fmt.Println()
	return string(buf), nil
}
