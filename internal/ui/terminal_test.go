package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestTerminalPresentationFallsBackWithoutColor(t *testing.T) {
	tests := []struct {
		name        string
		terminal    Terminal
		wantClear   bool
		wantHeader  bool
		wantColor   bool
		wantControl bool
	}{
		{
			name:        "color terminal",
			terminal:    newTerminal(true, "xterm-256color", false, ""),
			wantClear:   true,
			wantHeader:  true,
			wantColor:   true,
			wantControl: true,
		},
		{
			name:        "NO_COLOR",
			terminal:    newTerminal(true, "xterm-256color", true, ""),
			wantClear:   true,
			wantHeader:  true,
			wantControl: true,
		},
		{
			name:        "CLICOLOR disabled",
			terminal:    newTerminal(true, "xterm-256color", false, "0"),
			wantClear:   true,
			wantHeader:  true,
			wantControl: true,
		},
		{
			name:       "dumb terminal",
			terminal:   newTerminal(true, "dumb", false, ""),
			wantHeader: true,
		},
		{
			name:     "redirected output",
			terminal: newTerminal(false, "xterm-256color", false, ""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			test.terminal.Clear(&output)
			test.terminal.WriteHeader(&output, "group/project")
			value := output.String()
			if got := strings.Contains(value, terminalClearSequence); got != test.wantClear {
				t.Fatalf("clear sequence present = %t, want %t; output %q", got, test.wantClear, value)
			}
			if got := strings.Contains(value, "GitLab Helper"); got != test.wantHeader {
				t.Fatalf("header present = %t, want %t; output %q", got, test.wantHeader, value)
			}
			if got := test.terminal.ColorEnabled(); got != test.wantColor {
				t.Fatalf("color enabled = %t, want %t", got, test.wantColor)
			}
			if got := strings.Contains(value, accentSequence); got != test.wantColor {
				t.Fatalf("color sequence present = %t, want %t; output %q", got, test.wantColor, value)
			}
			if got := strings.Contains(value, "\x1b["); got != test.wantControl {
				t.Fatalf("control sequence present = %t, want %t; output %q", got, test.wantControl, value)
			}
			if test.wantHeader && !strings.Contains(value, "group/project") {
				t.Fatalf("project context missing from output %q", value)
			}
			if test.wantHeader && (!strings.Contains(value, "╔") || !strings.Contains(value, "╚")) {
				t.Fatalf("v1-style header box missing from output %q", value)
			}
		})
	}
}
