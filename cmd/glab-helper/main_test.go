package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestOfflineCLI(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "help", args: []string{"--help"}, want: "Usage: glab-helper"},
		{name: "version", args: []string{"--version"}, want: "glab-helper 0.1.0"},
		{name: "interactive not migrated", code: 2, want: "not migrated yet"},
		{name: "unknown argument", args: []string{"--unknown"}, code: 2, want: "Unknown argument"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != test.code {
				t.Fatalf("exit code = %d, want %d", code, test.code)
			}
			if output := stdout.String() + stderr.String(); !strings.Contains(output, test.want) {
				t.Fatalf("output %q does not contain %q", output, test.want)
			}
		})
	}
}
