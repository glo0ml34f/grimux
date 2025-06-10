package repl

import (
	"path/filepath"
	"strings"

	"github.com/glo0ml34f/grimux/internal/tmux"
)

type autoCompleter struct{}

func (c *autoCompleter) Do(line []rune, pos int) ([][]rune, int) {
	input := string(line[:pos])
	fields := strings.Fields(input)
	prefix := ""
	if len(fields) > 0 && !strings.HasSuffix(input, " ") {
		prefix = fields[len(fields)-1]
	}
	var suggestions []string
	if strings.HasPrefix(prefix, "%") {
		for name := range buffers {
			if strings.HasPrefix(name, prefix) {
				suggestions = append(suggestions, name[len(prefix):])
			}
		}
		ids, _ := tmux.ListPaneIDs()
		for _, id := range ids {
			if strings.HasPrefix(id, prefix) {
				suggestions = append(suggestions, id[len(prefix):])
			}
		}
		matches, _ := filepath.Glob(prefix + "*")
		for _, m := range matches {
			if strings.HasPrefix(m, prefix) {
				suggestions = append(suggestions, m[len(prefix):])
			}
		}
	} else if strings.HasPrefix(prefix, "!") || len(fields) == 0 {
		for name := range commands {
			if strings.HasPrefix(name, prefix) {
				suggestions = append(suggestions, name[len(prefix):])
			}
		}
	} else {
		matches, _ := filepath.Glob(prefix + "*")
		for _, m := range matches {
			if strings.HasPrefix(m, prefix) {
				suggestions = append(suggestions, m[len(prefix):])
			}
		}
	}
	out := make([][]rune, len(suggestions))
	for i, s := range suggestions {
		out[i] = []rune(s)
	}
	return out, len([]rune(prefix))
}
