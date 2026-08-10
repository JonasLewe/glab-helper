package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
)

type Label struct {
	Name string
}

type labelJSON struct {
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
	if value.Name == nil {
		return Label{}, fmt.Errorf("missing or invalid field %q", "name")
	}

	return Label{Name: *value.Name}, nil
}
