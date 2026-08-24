package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func EditText(ctx context.Context, initial string, stdin io.Reader, stdout, stderr io.Writer) (string, error) {
	file, err := os.CreateTemp("", "glab-helper-issue-*.md")
	if err != nil {
		return "", fmt.Errorf("create temporary description file: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)

	if _, err := io.WriteString(file, initial); err != nil {
		file.Close()
		return "", fmt.Errorf("write temporary description file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temporary description file: %w", err)
	}

	editor, err := resolveEditor()
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, editor[0], append(editor[1:], path)...)
	// Passing a buffered or piped reader to an editor starts a background copy
	// that can consume later CLI confirmations. Interactive editors need the
	// actual terminal; non-terminal test or piped input is deliberately omitted.
	if terminalInput, ok := stdin.(*os.File); ok {
		command.Stdin = terminalInput
	}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("run editor %q: %w", editor[0], err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read edited description: %w", err)
	}
	return string(updated), nil
}

func resolveEditor() ([]string, error) {
	for _, variable := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(os.Getenv(variable)); value != "" {
			parts := strings.Fields(value)
			if len(parts) > 0 {
				return parts, nil
			}
		}
	}
	for _, candidate := range []string{"nvim", "vim", "vi"} {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return []string{path}, nil
		}
	}
	return nil, fmt.Errorf("no editor found; configure VISUAL or EDITOR")
}
