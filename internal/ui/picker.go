package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var errCancelled = errors.New("selection cancelled")

type commandOutput func(context.Context, string, ...string) ([]byte, error)

type Picker struct {
	output commandOutput
}

type Options struct {
	Prompt      string
	BorderLabel string
}

func NewPicker() *Picker {
	return &Picker{output: runFZF}
}

func (picker *Picker) Choose(ctx context.Context, choices []string, options Options) (string, bool, error) {
	if len(choices) == 0 {
		return "", false, nil
	}
	known := make(map[string]struct{}, len(choices))
	for _, choice := range choices {
		if strings.ContainsAny(choice, "\r\n") {
			return "", false, fmt.Errorf("selection choice contains a line break")
		}
		if _, exists := known[choice]; exists {
			return "", false, fmt.Errorf("duplicate selection choice %q", choice)
		}
		known[choice] = struct{}{}
	}

	input := strings.Join(choices, "\n") + "\n"
	output, err := picker.output(
		ctx,
		input,
		"--prompt=  "+options.Prompt+" > ",
		"--header=  ENTER=select  ESC=cancel",
		"--height=~40",
		"--reverse",
		"--border=rounded",
		"--border-label= "+options.BorderLabel+" ",
	)
	if errors.Is(err, errCancelled) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	selected := strings.TrimSuffix(string(output), "\n")
	selected = strings.TrimSuffix(selected, "\r")
	if _, exists := known[selected]; !exists {
		return "", false, fmt.Errorf("fzf returned an unknown selection %q", selected)
	}
	return selected, true, nil
}

func runFZF(ctx context.Context, input string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	command := exec.CommandContext(ctx, "fzf", args...)
	command.Stdin = strings.NewReader(input)
	command.Stderr = &stderr
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 130 {
		return nil, errCancelled
	}
	cause := strings.TrimSpace(stderr.String())
	if cause == "" {
		return nil, fmt.Errorf("run fzf: %w", err)
	}
	return nil, fmt.Errorf("run fzf: %s: %w", cause, err)
}
