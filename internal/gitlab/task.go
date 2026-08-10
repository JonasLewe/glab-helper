package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const taskQuery = `query ProjectTasks($fullPath: ID!, $after: String) {
  namespace(fullPath: $fullPath) {
    workItems(types: [TASK], first: 100, after: $after) {
      nodes {
        id
        iid
        title
        description
        state
        widgets {
          __typename
          ... on WorkItemWidgetHierarchy {
            parent { iid }
          }
          ... on WorkItemWidgetLabels {
            labels(first: 100) {
              nodes { title }
              pageInfo { hasNextPage }
            }
          }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

type Task struct {
	ID          string
	IID         int64
	Title       string
	Description string
	Labels      []string
	State       string
	ParentIID   int64
}

type taskPageJSON struct {
	Namespace *struct {
		WorkItems *taskConnectionJSON `json:"workItems"`
	} `json:"namespace"`
}

type taskConnectionJSON struct {
	Nodes    *[]taskJSON       `json:"nodes"`
	PageInfo *taskPageInfoJSON `json:"pageInfo"`
}

type taskPageInfoJSON struct {
	HasNextPage *bool           `json:"hasNextPage"`
	EndCursor   json.RawMessage `json:"endCursor"`
}

type taskJSON struct {
	ID          *string           `json:"id"`
	IID         json.RawMessage   `json:"iid"`
	Title       *string           `json:"title"`
	Description json.RawMessage   `json:"description"`
	State       *string           `json:"state"`
	Widgets     *[]taskWidgetJSON `json:"widgets"`
}

type taskWidgetJSON struct {
	Type   *string         `json:"__typename"`
	Parent json.RawMessage `json:"parent"`
	Labels *taskLabelsJSON `json:"labels"`
}

type taskParentJSON struct {
	IID json.RawMessage `json:"iid"`
}

type taskLabelsJSON struct {
	Nodes    *[]taskLabelJSON       `json:"nodes"`
	PageInfo *taskLabelPageInfoJSON `json:"pageInfo"`
}

type taskLabelPageInfoJSON struct {
	HasNextPage *bool `json:"hasNextPage"`
}

type taskLabelJSON struct {
	Title *string `json:"title"`
}

func (client *Client) ListTasks(ctx context.Context, projectPath string) ([]Task, error) {
	tasks := make([]Task, 0)
	seenIIDs := make(map[int64]struct{})
	seenCursors := make(map[string]struct{})
	after := ""
	for page := 1; ; page++ {
		args := []string{"api", "graphql", "-f", "query=" + taskQuery, "-f", "fullPath=" + projectPath}
		if after != "" {
			args = append(args, "-f", "after="+after)
		}
		output, err := client.output(ctx, args...)
		if err != nil {
			return nil, fmt.Errorf("run glab task page %d: %w", page, err)
		}

		pageTasks, pageInfo, err := parseTaskPage(output)
		if err != nil {
			return nil, fmt.Errorf("decode GitLab task page %d: %w", page, err)
		}
		for _, task := range pageTasks {
			if _, exists := seenIIDs[task.IID]; exists {
				return nil, fmt.Errorf("duplicate GitLab task IID #%d across pages", task.IID)
			}
			seenIIDs[task.IID] = struct{}{}
			tasks = append(tasks, task)
		}
		if !pageInfo.HasNextPage {
			return tasks, nil
		}
		if pageInfo.EndCursor == "" {
			return nil, fmt.Errorf("GitLab task page %d has a next page but no end cursor", page)
		}
		if _, exists := seenCursors[pageInfo.EndCursor]; exists {
			return nil, fmt.Errorf("GitLab task pagination repeated cursor %q", pageInfo.EndCursor)
		}
		seenCursors[pageInfo.EndCursor] = struct{}{}
		after = pageInfo.EndCursor
	}
}

type taskPageInfo struct {
	HasNextPage bool
	EndCursor   string
}

func parseTaskPage(data []byte) ([]Task, taskPageInfo, error) {
	response, err := decodeGraphQL[taskPageJSON](data)
	if err != nil {
		return nil, taskPageInfo{}, err
	}
	if response.Namespace == nil {
		return nil, taskPageInfo{}, fmt.Errorf("missing GitLab namespace")
	}
	workItems := response.Namespace.WorkItems
	if workItems == nil || workItems.Nodes == nil || workItems.PageInfo == nil {
		return nil, taskPageInfo{}, fmt.Errorf("incomplete work item connection")
	}
	if workItems.PageInfo.HasNextPage == nil || workItems.PageInfo.EndCursor == nil {
		return nil, taskPageInfo{}, fmt.Errorf("incomplete work item page info")
	}

	pageInfo := taskPageInfo{HasNextPage: *workItems.PageInfo.HasNextPage}
	if !bytes.Equal(workItems.PageInfo.EndCursor, []byte("null")) {
		if err := json.Unmarshal(workItems.PageInfo.EndCursor, &pageInfo.EndCursor); err != nil {
			return nil, taskPageInfo{}, fmt.Errorf("field %q: %w", "endCursor", err)
		}
	}
	tasks := make([]Task, 0, len(*workItems.Nodes))
	for index, value := range *workItems.Nodes {
		task, err := parseTask(value)
		if err != nil {
			return nil, taskPageInfo{}, fmt.Errorf("task %d: %w", index+1, err)
		}
		tasks = append(tasks, task)
	}
	return tasks, pageInfo, nil
}

func parseTask(value taskJSON) (Task, error) {
	if value.ID == nil || strings.TrimSpace(*value.ID) == "" {
		return Task{}, fmt.Errorf("missing or invalid field %q", "id")
	}
	iid, err := parseGraphQLIID(value.IID)
	if err != nil {
		return Task{}, fmt.Errorf("field %q: %w", "iid", err)
	}
	if value.Title == nil {
		return Task{}, fmt.Errorf("missing or invalid field %q", "title")
	}
	if value.Description == nil {
		return Task{}, fmt.Errorf("missing field %q", "description")
	}
	if value.State == nil {
		return Task{}, fmt.Errorf("missing or invalid field %q", "state")
	}
	if value.Widgets == nil {
		return Task{}, fmt.Errorf("field %q must be an array", "widgets")
	}

	task := Task{ID: *value.ID, IID: iid, Title: *value.Title, State: *value.State}
	if !bytes.Equal(value.Description, []byte("null")) {
		if err := json.Unmarshal(value.Description, &task.Description); err != nil {
			return Task{}, fmt.Errorf("field %q: %w", "description", err)
		}
	}
	var foundHierarchy, foundLabels bool
	for _, widget := range *value.Widgets {
		if widget.Type == nil {
			return Task{}, fmt.Errorf("widget has no valid type")
		}
		switch *widget.Type {
		case "WorkItemWidgetHierarchy":
			foundHierarchy = true
			if widget.Parent == nil {
				return Task{}, fmt.Errorf("hierarchy widget is missing parent")
			}
			if !bytes.Equal(widget.Parent, []byte("null")) {
				var parent taskParentJSON
				if err := json.Unmarshal(widget.Parent, &parent); err != nil {
					return Task{}, fmt.Errorf("hierarchy parent: %w", err)
				}
				task.ParentIID, err = parseGraphQLIID(parent.IID)
				if err != nil {
					return Task{}, fmt.Errorf("hierarchy parent IID: %w", err)
				}
			}
		case "WorkItemWidgetLabels":
			foundLabels = true
			if widget.Labels == nil || widget.Labels.Nodes == nil || widget.Labels.PageInfo == nil || widget.Labels.PageInfo.HasNextPage == nil {
				return Task{}, fmt.Errorf("labels widget has no labels array")
			}
			if *widget.Labels.PageInfo.HasNextPage {
				return Task{}, fmt.Errorf("task has more than 100 labels")
			}
			task.Labels = make([]string, 0, len(*widget.Labels.Nodes))
			for index, label := range *widget.Labels.Nodes {
				if label.Title == nil || strings.TrimSpace(*label.Title) == "" {
					return Task{}, fmt.Errorf("label %d has no valid title", index+1)
				}
				task.Labels = append(task.Labels, *label.Title)
			}
		}
	}
	if !foundHierarchy || !foundLabels {
		return Task{}, fmt.Errorf("task is missing hierarchy or labels widget")
	}
	return task, nil
}
