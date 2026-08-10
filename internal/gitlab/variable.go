package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
)

type projectVariableJSON struct {
	Value *string `json:"value"`
}

func (client *Client) ProjectVariable(ctx context.Context, projectPath, key string) (string, error) {
	if projectPath == "" || key == "" {
		return "", fmt.Errorf("project path and variable key must not be empty")
	}

	endpoint := fmt.Sprintf("projects/%s/variables/%s", projectPath, key)
	output, err := client.output(ctx, "api", endpoint)
	if err != nil {
		return "", fmt.Errorf("run glab project variable read: %w", err)
	}

	var variable projectVariableJSON
	if err := json.Unmarshal(output, &variable); err != nil {
		return "", fmt.Errorf("decode GitLab project variable: %w", err)
	}
	if variable.Value == nil || *variable.Value == "" {
		return "", fmt.Errorf("decode GitLab project variable: missing value")
	}
	return *variable.Value, nil
}
