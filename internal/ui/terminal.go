package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const terminalClearSequence = "\x1b[H\x1b[2J\x1b[3J"

const (
	accentSequence = "\x1b[1;36m"
	dimSequence    = "\x1b[2m"
	resetSequence  = "\x1b[0m"
)

// Terminal describes presentation capabilities only. It never probes the
// terminal or executes an external command, so unsupported styling always
// degrades to plain text.
type Terminal struct {
	interactive bool
	ansi        bool
	color       bool
}

func DetectTerminal(output io.Writer) Terminal {
	file, ok := output.(*os.File)
	if !ok {
		return Terminal{}
	}
	info, err := file.Stat()
	if err != nil {
		return Terminal{}
	}
	_, noColorSet := os.LookupEnv("NO_COLOR")
	return NewTerminal(
		info.Mode()&os.ModeCharDevice != 0,
		os.Getenv("TERM"),
		noColorSet,
		os.Getenv("CLICOLOR"),
	)
}

func NewTerminal(interactive bool, term string, noColorSet bool, cliColor string) Terminal {
	term = strings.TrimSpace(term)
	ansi := interactive && term != "" && !strings.EqualFold(term, "dumb")
	color := ansi && !noColorSet && strings.TrimSpace(cliColor) != "0"
	return Terminal{interactive: interactive, ansi: ansi, color: color}
}

func (terminal Terminal) Clear(output io.Writer) {
	if terminal.ansi {
		fmt.Fprint(output, terminalClearSequence)
	}
}

func (terminal Terminal) WriteHeader(output io.Writer, projectPath string) {
	if !terminal.interactive {
		return
	}
	if terminal.color {
		fmt.Fprintf(output, "  %s◆ glab-helper%s\n", accentSequence, resetSequence)
		fmt.Fprintf(output, "    %s%s%s\n\n", dimSequence, projectPath, resetSequence)
		return
	}
	// Keep the same useful context without emitting terminal control codes.
	fmt.Fprintf(output, "  ◆ glab-helper\n    %s\n\n", projectPath)
}

func (terminal Terminal) ColorEnabled() bool {
	return terminal.color
}
