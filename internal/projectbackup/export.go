package projectbackup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
)

type metadata struct {
	CreatedAt string `json:"created_at"`
	RepoName  string `json:"repo_name"`
	ProjectID int64  `json:"project_id"`
	Label     string `json:"label"`
}

type issue struct {
	IID         int64           `json:"iid"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Labels      []string        `json:"labels"`
	Milestone   *issueMilestone `json:"milestone"`
	State       string          `json:"state"`
	Assignees   []string        `json:"assignees"`
}

type issueMilestone struct {
	Title string `json:"title"`
}

type milestone struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
}

type label struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func Write(root string, createdAt time.Time, projectID int64, projectPath, snapshotLabel string, issues []gitlab.Issue, milestones []gitlab.Milestone, labels []gitlab.Label) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("snapshot root must not be empty")
	}
	if projectID < 1 || strings.TrimSpace(projectPath) == "" {
		return "", fmt.Errorf("snapshot project identity is invalid")
	}
	if snapshotLabel = slug(snapshotLabel); snapshotLabel == "" {
		snapshotLabel = "snapshot"
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create snapshot root: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", fmt.Errorf("secure snapshot root: %w", err)
	}
	directory, err := os.MkdirTemp(root, createdAt.Format("20060102-150405")+"-"+snapshotLabel+"-")
	if err != nil {
		return "", fmt.Errorf("create snapshot directory: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(directory)
		}
	}()

	issueValues := make([]issue, len(issues))
	for index, value := range issues {
		var issueMilestoneValue *issueMilestone
		if value.Milestone != nil {
			issueMilestoneValue = &issueMilestone{Title: value.Milestone.Title}
		}
		issueValues[index] = issue{
			IID: value.IID, Title: value.Title, Description: value.Description,
			Labels: value.Labels, Milestone: issueMilestoneValue, State: value.State, Assignees: value.Assignees,
		}
	}
	milestoneValues := make([]milestone, len(milestones))
	for index, value := range milestones {
		milestoneValues[index] = milestone{ID: value.ID, Title: value.Title, Description: value.Description, State: value.State}
	}
	labelValues := make([]label, len(labels))
	for index, value := range labels {
		labelValues[index] = label{ID: value.ID, Name: value.Name}
	}
	files := []struct {
		name  string
		value any
	}{
		{name: "issues.json", value: issueValues},
		{name: "milestones.json", value: milestoneValues},
		{name: "labels.json", value: labelValues},
		{name: "metadata.json", value: metadata{
			CreatedAt: createdAt.UTC().Format(time.RFC3339), RepoName: projectPath, ProjectID: projectID, Label: snapshotLabel,
		}},
	}
	for _, file := range files {
		data, err := json.MarshalIndent(file.value, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode snapshot %s: %w", file.name, err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(directory, file.name), data, 0o600); err != nil {
			return "", fmt.Errorf("write snapshot %s: %w", file.name, err)
		}
	}
	complete = true
	return directory, nil
}

func slug(value string) string {
	var result strings.Builder
	previousDash := false
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			result.WriteRune(character)
			previousDash = false
			continue
		}
		if result.Len() > 0 && !previousDash {
			result.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(result.String(), "-")
}
