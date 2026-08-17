package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
)

type Project struct {
	ID            int64
	Path          string
	DefaultBranch string
}

func CurrentProject(ctx context.Context) (Project, error) {
	return NewClient().CurrentProject(ctx)
}

func (client *Client) CurrentProject(ctx context.Context) (Project, error) {
	output, err := client.output(ctx, "repo", "view", "--output", "json")
	if err != nil {
		return Project{}, fmt.Errorf("run glab repo view: %w", err)
	}

	return parseProject(output)
}

func parseProject(data []byte) (Project, error) {
	var value struct {
		ID                int64  `json:"id"`
		PathWithNamespace string `json:"path_with_namespace"`
		NameWithNamespace string `json:"name_with_namespace"`
		Name              string `json:"name"`
		DefaultBranch     string `json:"default_branch"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return Project{}, fmt.Errorf("decode glab project: %w", err)
	}

	path := value.PathWithNamespace
	if path == "" {
		path = value.NameWithNamespace
	}
	if path == "" {
		path = value.Name
	}
	if value.ID < 1 || path == "" {
		return Project{}, fmt.Errorf("decode glab project: missing id or path")
	}

	return Project{ID: value.ID, Path: path, DefaultBranch: value.DefaultBranch}, nil
}
