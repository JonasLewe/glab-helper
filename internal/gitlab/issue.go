package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

type Issue struct {
	IID         int64
	Title       string
	Description string
	Labels      []string
	Milestone   *IssueMilestone
	State       string
	Assignees   []string
}

type IssueMilestone struct {
	Title string
}

type issueJSON struct {
	IID         *int64               `json:"iid"`
	Title       *string              `json:"title"`
	Description json.RawMessage      `json:"description"`
	Labels      *[]string            `json:"labels"`
	Milestone   json.RawMessage      `json:"milestone"`
	State       *string              `json:"state"`
	Assignees   *[]issueAssigneeJSON `json:"assignees"`
}

type issueMilestoneJSON struct {
	Title *string `json:"title"`
}

type issueAssigneeJSON struct {
	Username *string `json:"username"`
}

func (client *Client) ListIssues(ctx context.Context, projectID int64) ([]Issue, error) {
	endpoint := fmt.Sprintf("projects/%d/issues?state=all&per_page=%d", projectID, maxItemsPerPage)
	output, err := client.output(ctx, "api", "--paginate", endpoint)
	if err != nil {
		return nil, fmt.Errorf("run glab issue pagination: %w", err)
	}

	issues, err := parseIssuePages(output)
	if err != nil {
		return nil, fmt.Errorf("decode paginated GitLab issues: %w", err)
	}
	return issues, nil
}

func parseIssuePages(data []byte) ([]Issue, error) {
	return parseArrayPages(data, "issue", parseIssue)
}

func parseIssue(data []byte) (Issue, error) {
	var value issueJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return Issue{}, err
	}
	if value.IID == nil {
		return Issue{}, fmt.Errorf("missing or invalid field %q", "iid")
	}
	if *value.IID < 1 {
		return Issue{}, fmt.Errorf("field %q must be positive", "iid")
	}
	if value.Title == nil {
		return Issue{}, fmt.Errorf("missing or invalid field %q", "title")
	}
	if value.Description == nil {
		return Issue{}, fmt.Errorf("missing field %q", "description")
	}
	if value.Labels == nil {
		return Issue{}, fmt.Errorf("field %q must be an array", "labels")
	}
	if value.State == nil {
		return Issue{}, fmt.Errorf("missing or invalid field %q", "state")
	}
	if value.Milestone == nil {
		return Issue{}, fmt.Errorf("missing field %q", "milestone")
	}
	if value.Assignees == nil {
		return Issue{}, fmt.Errorf("field %q must be an array", "assignees")
	}

	issue := Issue{
		IID:       *value.IID,
		Title:     *value.Title,
		Labels:    *value.Labels,
		State:     *value.State,
		Assignees: make([]string, 0, len(*value.Assignees)),
	}
	if !bytes.Equal(value.Description, []byte("null")) {
		if err := json.Unmarshal(value.Description, &issue.Description); err != nil {
			return Issue{}, fmt.Errorf("field %q: %w", "description", err)
		}
	}
	if !bytes.Equal(value.Milestone, []byte("null")) {
		var milestone issueMilestoneJSON
		if err := json.Unmarshal(value.Milestone, &milestone); err != nil {
			return Issue{}, fmt.Errorf("field %q: %w", "milestone", err)
		}
		if milestone.Title == nil {
			return Issue{}, fmt.Errorf("field %q has no valid title", "milestone")
		}
		issue.Milestone = &IssueMilestone{Title: *milestone.Title}
	}
	for index, assignee := range *value.Assignees {
		if assignee.Username == nil {
			return Issue{}, fmt.Errorf("field %q item %d has no valid username", "assignees", index+1)
		}
		issue.Assignees = append(issue.Assignees, *assignee.Username)
	}

	return issue, nil
}
