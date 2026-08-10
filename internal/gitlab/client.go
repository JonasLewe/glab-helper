package gitlab

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type commandOutput func(context.Context, ...string) ([]byte, error)

type Client struct {
	output commandOutput
}

func NewClient() *Client {
	return &Client{output: runGLab}
}

func runGLab(ctx context.Context, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	command := exec.CommandContext(ctx, "glab", args...)
	command.Stderr = &stderr

	output, err := command.Output()
	if err == nil {
		return output, nil
	}

	cause := strings.TrimSpace(stderr.String())
	if cause == "" {
		return nil, err
	}
	return nil, fmt.Errorf("%s: %w", cause, err)
}
