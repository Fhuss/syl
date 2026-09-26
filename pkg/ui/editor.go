package ui

import (
	"fmt"
	"strings"
	"unicode"
)

// parseEditorCommand splits $EDITOR into an executable and arguments without
// invoking a shell. Quoted paths and escaped spaces are supported; shell
// substitutions and pipes are intentionally not evaluated.
func parseEditorCommand(command string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped, started := false, false
	flush := func() {
		if started {
			args = append(args, current.String())
			current.Reset()
			started = false
		}
	}
	for _, r := range command {
		switch {
		case escaped:
			current.WriteRune(r)
			started, escaped = true, false
		case r == '\\' && quote != '\'':
			escaped = true
			started = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			started = true
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unclosed quote or escape in $EDITOR")
	}
	flush()
	if len(args) == 0 || args[0] == "" {
		return nil, fmt.Errorf("empty $EDITOR command")
	}
	return args, nil
}
