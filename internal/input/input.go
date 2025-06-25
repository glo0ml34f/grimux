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

// GetReadline returns the current global readline instance.
func GetReadline() *readline.Instance { return rl }

// ReadPasswordPrompt reads a secret line displaying the given prompt.
func ReadPasswordPrompt(prompt string) (string, error) {
	if rl != nil {
		b, err := rl.ReadPassword(prompt)
		return string(b), err
	}
	fmt.Fprint(os.Stdout, prompt)
	return ReadPassword()
}

// ReadLinePrompt reads a line showing the given prompt.
func ReadLinePrompt(prompt string) (string, error) {
	if rl != nil {
		fmt.Fprint(rl.Stdout(), prompt)
		return rl.Readline()
	}
	fmt.Fprint(os.Stdout, prompt)
	return ReadLine()
}

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
