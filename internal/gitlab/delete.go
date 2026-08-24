package gitlab

import (
	"context"
	"fmt"
)

func (client *Client) DeleteIssue(ctx context.Context, projectID, issueIID int64) error {
	endpoint := fmt.Sprintf("projects/%d/issues/%d", projectID, issueIID)
	if _, err := client.output(ctx, "api", endpoint, "-X", "DELETE"); err != nil {
		return fmt.Errorf("delete GitLab issue #%d: %w", issueIID, err)
	}
	return nil
}

func (client *Client) DeleteMilestone(ctx context.Context, projectID, milestoneID int64) error {
	endpoint := fmt.Sprintf("projects/%d/milestones/%d", projectID, milestoneID)
	if _, err := client.output(ctx, "api", endpoint, "-X", "DELETE"); err != nil {
		return fmt.Errorf("delete GitLab milestone %d: %w", milestoneID, err)
	}
	return nil
}
