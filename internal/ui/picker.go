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
	output       commandOutput
	colorEnabled bool
}

type Options struct {
	Prompt      string
	BorderLabel string
}

func NewPicker(colorEnabled bool) *Picker {
	return &Picker{output: runFZF, colorEnabled: colorEnabled}
}

func (picker *Picker) Choose(ctx context.Context, choices []string, options Options) (string, bool, error) {
	known, err := validateChoices(choices)
	if err != nil || len(choices) == 0 {
		return "", false, err
	}

	input := strings.Join(choices, "\n") + "\n"
	arguments := []string{
		"--prompt=  " + options.Prompt + " > ",
		"--header=  ENTER=select  ESC=cancel",
		"--height=~40",
		"--reverse",
		"--border=rounded",
		"--border-label= " + options.BorderLabel + " ",
	}
	arguments = picker.withOptionalColor(arguments)
	output, err := picker.output(ctx, input, arguments...)
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

func (picker *Picker) ChooseMany(ctx context.Context, choices []string, options Options) ([]string, bool, error) {
	known, err := validateChoices(choices)
	if err != nil || len(choices) == 0 {
		return nil, false, err
	}

	input := strings.Join(choices, "\n") + "\n"
	arguments := []string{
		"--multi",
		"--prompt=  " + options.Prompt + " > ",
		"--header=  TAB=select  ENTER=confirm  ESC=cancel",
		"--height=~40",
		"--reverse",
		"--border=rounded",
		"--border-label= " + options.BorderLabel + " ",
	}
	arguments = picker.withOptionalColor(arguments)
	output, err := picker.output(ctx, input, arguments...)
	if errors.Is(err, errCancelled) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	value := strings.TrimSuffix(string(output), "\n")
	value = strings.TrimSuffix(value, "\r")
	if value == "" {
		return []string{}, true, nil
	}
	selected := strings.Split(value, "\n")
	seen := make(map[string]struct{}, len(selected))
	for index, choice := range selected {
		choice = strings.TrimSuffix(choice, "\r")
		selected[index] = choice
		if _, exists := known[choice]; !exists {
			return nil, false, fmt.Errorf("fzf returned an unknown selection %q", choice)
		}
		if _, exists := seen[choice]; exists {
			return nil, false, fmt.Errorf("fzf returned duplicate selection %q", choice)
		}
		seen[choice] = struct{}{}
	}
	return selected, true, nil
}

func (picker *Picker) withOptionalColor(arguments []string) []string {
	if !picker.colorEnabled {
		return arguments
	}
	return append(arguments, "--color=border:cyan,header:-1:dim,prompt:cyan,pointer:cyan,marker:cyan")
}

func validateChoices(choices []string) (map[string]struct{}, error) {
	known := make(map[string]struct{}, len(choices))
	for _, choice := range choices {
		if strings.ContainsAny(choice, "\r\n") {
			return nil, fmt.Errorf("selection choice contains a line break")
		}
		if _, exists := known[choice]; exists {
			return nil, fmt.Errorf("duplicate selection choice %q", choice)
		}
		known[choice] = struct{}{}
	}
	return known, nil
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
