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
	greenSequence  = "\x1b[32m"
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
	projectPath = truncate(projectPath, 80)
	width := 48
	if projectLength := len([]rune(projectPath)) + 8; projectLength > width {
		width = projectLength
	}
	line := strings.Repeat("═", width)

	fmt.Fprintln(output)
	terminal.writeBoxBorder(output, "╔"+line+"╗")
	terminal.writeBoxLine(output, "GitLab Helper", width)
	terminal.writeBoxLine(output, projectPath, width)
	terminal.writeBoxBorder(output, "╚"+line+"╝")
	fmt.Fprintln(output)
}

func (terminal Terminal) WriteStatus(output io.Writer, message string) {
	if !terminal.interactive {
		return
	}
	if terminal.color {
		fmt.Fprintf(output, "  %s%s%s", dimSequence, message, resetSequence)
		return
	}
	fmt.Fprintf(output, "  %s", message)
}

func (terminal Terminal) ClearStatus(output io.Writer) {
	if !terminal.interactive {
		return
	}
	if terminal.ansi {
		fmt.Fprint(output, "\r\x1b[2K")
		return
	}
	fmt.Fprintln(output)
}

func (terminal Terminal) WriteIntegrationAvailable(output io.Writer, provider string) {
	if !terminal.interactive {
		return
	}
	if terminal.color {
		fmt.Fprintf(output, "  %s✔%s %s%s integration available%s\n\n", greenSequence, resetSequence, dimSequence, provider, resetSequence)
		return
	}
	fmt.Fprintf(output, "  ✔ %s integration available\n\n", provider)
}

func (terminal Terminal) ColorEnabled() bool {
	return terminal.color
}

func (terminal Terminal) writeBoxBorder(output io.Writer, value string) {
	if terminal.color {
		fmt.Fprintf(output, "  %s%s%s\n", accentSequence, value, resetSequence)
		return
	}
	fmt.Fprintf(output, "  %s\n", value)
}

func (terminal Terminal) writeBoxLine(output io.Writer, value string, width int) {
	padding := width - len([]rune(value)) - 2
	if padding < 0 {
		padding = 0
	}
	if terminal.color {
		fmt.Fprintf(output, "  %s║%s  %s%s%s║%s\n", accentSequence, resetSequence, value, strings.Repeat(" ", padding), accentSequence, resetSequence)
		return
	}
	fmt.Fprintf(output, "  ║  %s%s║\n", value, strings.Repeat(" ", padding))
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-3]) + "..."
}
