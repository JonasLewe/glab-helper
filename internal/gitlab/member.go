package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Member struct {
	ID       int64
	Username string
	Name     string
}

type memberJSON struct {
	ID       *int64  `json:"id"`
	Username *string `json:"username"`
	Name     *string `json:"name"`
}

func (client *Client) ListMembers(ctx context.Context, projectID int64) ([]Member, error) {
	endpoint := fmt.Sprintf("projects/%d/members/all?per_page=%d", projectID, maxItemsPerPage)
	output, err := client.output(ctx, "api", "--paginate", endpoint)
	if err != nil {
		return nil, fmt.Errorf("run glab project member pagination: %w", err)
	}

	members, err := parseArrayPages(output, "member", parseMember)
	if err != nil {
		return nil, fmt.Errorf("decode paginated GitLab project members: %w", err)
	}
	return members, nil
}

func parseMember(data []byte) (Member, error) {
	var value memberJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return Member{}, err
	}
	if value.ID == nil || *value.ID < 1 {
		return Member{}, fmt.Errorf("missing or invalid field %q", "id")
	}
	if value.Username == nil || strings.TrimSpace(*value.Username) == "" {
		return Member{}, fmt.Errorf("missing or invalid field %q", "username")
	}
	if value.Name == nil {
		return Member{}, fmt.Errorf("missing or invalid field %q", "name")
	}

	return Member{ID: *value.ID, Username: *value.Username, Name: *value.Name}, nil
}
