package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

type Milestone struct {
	ID          int64
	Title       string
	Description string
	State       string
}

type milestoneJSON struct {
	ID          *int64          `json:"id"`
	Title       *string         `json:"title"`
	Description json.RawMessage `json:"description"`
	State       *string         `json:"state"`
}

func (client *Client) ListMilestones(ctx context.Context, projectID int64) ([]Milestone, error) {
	endpoint := fmt.Sprintf("projects/%d/milestones?per_page=%d", projectID, maxItemsPerPage)
	output, err := client.output(ctx, "api", "--paginate", endpoint)
	if err != nil {
		return nil, fmt.Errorf("run glab milestone pagination: %w", err)
	}

	milestones, err := parseArrayPages(output, "milestone", parseMilestone)
	if err != nil {
		return nil, fmt.Errorf("decode paginated GitLab milestones: %w", err)
	}
	return milestones, nil
}

func parseMilestone(data []byte) (Milestone, error) {
	var value milestoneJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return Milestone{}, err
	}
	if value.ID == nil {
		return Milestone{}, fmt.Errorf("missing or invalid field %q", "id")
	}
	if *value.ID < 1 {
		return Milestone{}, fmt.Errorf("field %q must be positive", "id")
	}
	if value.Title == nil {
		return Milestone{}, fmt.Errorf("missing or invalid field %q", "title")
	}
	if value.Description == nil {
		return Milestone{}, fmt.Errorf("missing field %q", "description")
	}
	if value.State == nil {
		return Milestone{}, fmt.Errorf("missing or invalid field %q", "state")
	}

	milestone := Milestone{
		ID:    *value.ID,
		Title: *value.Title,
		State: *value.State,
	}
	if !bytes.Equal(value.Description, []byte("null")) {
		if err := json.Unmarshal(value.Description, &milestone.Description); err != nil {
			return Milestone{}, fmt.Errorf("field %q: %w", "description", err)
		}
	}

	return milestone, nil
}
