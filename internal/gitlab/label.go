package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
)

type Label struct {
	ID   int64
	Name string
}

type labelJSON struct {
	ID   *int64  `json:"id"`
	Name *string `json:"name"`
}

func (client *Client) ListLabels(ctx context.Context, projectID int64) ([]Label, error) {
	endpoint := fmt.Sprintf("projects/%d/labels?per_page=%d", projectID, maxItemsPerPage)
	output, err := client.output(ctx, "api", "--paginate", endpoint)
	if err != nil {
		return nil, fmt.Errorf("run glab label pagination: %w", err)
	}

	labels, err := parseArrayPages(output, "label", parseLabel)
	if err != nil {
		return nil, fmt.Errorf("decode paginated GitLab labels: %w", err)
	}
	return labels, nil
}

func parseLabel(data []byte) (Label, error) {
	var value labelJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return Label{}, err
	}
	if value.ID == nil || *value.ID < 1 {
		return Label{}, fmt.Errorf("missing or invalid field %q", "id")
	}
	if value.Name == nil {
		return Label{}, fmt.Errorf("missing or invalid field %q", "name")
	}

	return Label{ID: *value.ID, Name: *value.Name}, nil
}
