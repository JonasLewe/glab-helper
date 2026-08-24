package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const defaultSyncLabelColor = "#428BCA"

const workItemByIIDQuery = `query ProjectWorkItem($fullPath: ID!, $iid: String!) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
      iid
      workItemType { name }
    }
  }
}`

const taskTypeQuery = `query ProjectTaskType($fullPath: ID!) {
  namespace(fullPath: $fullPath) {
    workItemTypes(name: TASK) {
      nodes { id name }
    }
  }
}`

const createTaskMutation = `mutation CreateProjectTask($input: WorkItemCreateInput!) {
  workItemCreate(input: $input) {
    workItem { id iid workItemType { name } }
    errors
  }
}`

const setWorkItemParentMutation = `mutation SetWorkItemParent($input: WorkItemUpdateInput!) {
  workItemUpdate(input: $input) {
    workItem { id }
    errors
  }
}`

type CreatedIssue struct {
	IID int64
}

type WorkItemReference struct {
	ID  string
	IID int64
}

type workItemQueryJSON struct {
	Namespace *struct {
		WorkItem      *workItemJSON `json:"workItem"`
		WorkItemTypes *struct {
			Nodes *[]workItemTypeJSON `json:"nodes"`
		} `json:"workItemTypes"`
	} `json:"namespace"`
}

func (client *Client) CreateLabel(ctx context.Context, projectID int64, name string) (Label, error) {
	return client.CreateLabelWithColor(ctx, projectID, name, defaultSyncLabelColor)
}

func (client *Client) CreateLabelWithColor(ctx context.Context, projectID int64, name, color string) (Label, error) {
	endpoint := fmt.Sprintf("projects/%d/labels", projectID)
	output, err := client.output(ctx, "api", endpoint, "-X", "POST", "-f", "name="+name, "-f", "color="+color)
	if err != nil {
		return Label{}, fmt.Errorf("create GitLab label %q: %w", name, err)
	}
	label, err := parseLabel(output)
	if err != nil {
		return Label{}, fmt.Errorf("decode created GitLab label %q: %w", name, err)
	}
	if label.Name != name {
		return Label{}, fmt.Errorf("created GitLab label has unexpected name %q", label.Name)
	}
	return label, nil
}

func (client *Client) CreateBoardList(ctx context.Context, projectID, boardID, labelID int64, labelName string) error {
	endpoint := fmt.Sprintf("projects/%d/boards/%d/lists", projectID, boardID)
	output, err := client.output(ctx, "api", endpoint, "-X", "POST", "-f", "label_id="+strconv.FormatInt(labelID, 10))
	if err != nil {
		return fmt.Errorf("create GitLab board list for label %q: %w", labelName, err)
	}
	list, err := parseBoardList(output)
	if err != nil {
		return fmt.Errorf("decode created GitLab board list for label %q: %w", labelName, err)
	}
	if !strings.EqualFold(list.LabelName, labelName) {
		return fmt.Errorf("created GitLab board list has unexpected label %q", list.LabelName)
	}
	return nil
}

func (client *Client) CreateMilestone(ctx context.Context, projectID int64, title, description string, close bool) (Milestone, error) {
	endpoint := fmt.Sprintf("projects/%d/milestones", projectID)
	output, err := client.output(ctx, "api", endpoint, "-X", "POST", "-f", "title="+title, "-f", "description="+description)
	if err != nil {
		return Milestone{}, fmt.Errorf("create GitLab milestone %q: %w", title, err)
	}
	milestone, err := parseMilestone(output)
	if err != nil {
		return Milestone{}, fmt.Errorf("decode created GitLab milestone %q: %w", title, err)
	}
	if close {
		if err := client.UpdateMilestone(ctx, projectID, milestone.ID, title, description, true); err != nil {
			return Milestone{}, err
		}
		milestone.State = "closed"
	}
	return milestone, nil
}

func (client *Client) CreateManualMilestone(ctx context.Context, projectID int64, title, dueDate string) (Milestone, error) {
	endpoint := fmt.Sprintf("projects/%d/milestones", projectID)
	args := []string{"api", endpoint, "-X", "POST", "-f", "title=" + title}
	if dueDate != "" {
		args = append(args, "-f", "due_date="+dueDate)
	}
	output, err := client.output(ctx, args...)
	if err != nil {
		return Milestone{}, fmt.Errorf("create GitLab milestone %q: %w", title, err)
	}
	milestone, err := parseMilestone(output)
	if err != nil {
		return Milestone{}, fmt.Errorf("decode created GitLab milestone %q: %w", title, err)
	}
	if milestone.Title != title {
		return Milestone{}, fmt.Errorf("created GitLab milestone has unexpected title %q", milestone.Title)
	}
	return milestone, nil
}

func (client *Client) UpdateMilestone(ctx context.Context, projectID, milestoneID int64, title, description string, close bool) error {
	endpoint := fmt.Sprintf("projects/%d/milestones/%d", projectID, milestoneID)
	args := []string{"api", endpoint, "-X", "PUT", "-f", "title=" + title, "-f", "description=" + description}
	if close {
		args = append(args, "-f", "state_event=close")
	}
	if _, err := client.output(ctx, args...); err != nil {
		return fmt.Errorf("update GitLab milestone %d: %w", milestoneID, err)
	}
	return nil
}

func (client *Client) CreateIssue(ctx context.Context, projectID int64, title, description string, labels []string, milestoneID *int64, close bool) (CreatedIssue, error) {
	created, err := client.createIssue(ctx, projectID, title, description, labels, milestoneID, nil)
	if err != nil {
		return CreatedIssue{}, err
	}
	if close {
		if err := client.UpdateIssue(ctx, projectID, created.IID, title, description, labels, milestoneID, true); err != nil {
			return CreatedIssue{}, err
		}
	}
	return created, nil
}

func (client *Client) CreateManualIssue(ctx context.Context, projectID int64, title, description string, labels []string, milestoneID, assigneeID *int64) (CreatedIssue, error) {
	return client.createIssue(ctx, projectID, title, description, labels, milestoneID, assigneeID)
}

func (client *Client) createIssue(ctx context.Context, projectID int64, title, description string, labels []string, milestoneID, assigneeID *int64) (CreatedIssue, error) {
	endpoint := fmt.Sprintf("projects/%d/issues", projectID)
	args := []string{
		"api", endpoint, "-X", "POST",
		"-f", "title=" + title,
		"-f", "description=" + description,
		"-f", "labels=" + strings.Join(labels, ","),
		"-f", "issue_type=issue",
	}
	if milestoneID != nil {
		args = append(args, "-f", "milestone_id="+strconv.FormatInt(*milestoneID, 10))
	}
	if assigneeID != nil {
		args = append(args, "-f", "assignee_id="+strconv.FormatInt(*assigneeID, 10))
	}
	output, err := client.output(ctx, args...)
	if err != nil {
		return CreatedIssue{}, fmt.Errorf("create GitLab issue %q: %w", title, err)
	}
	created, err := parseCreatedIssue(output)
	if err != nil {
		return CreatedIssue{}, fmt.Errorf("decode created GitLab issue %q: %w", title, err)
	}
	return created, nil
}

func (client *Client) UpdateIssue(ctx context.Context, projectID, issueIID int64, title, description string, labels []string, milestoneID *int64, close bool) error {
	endpoint := fmt.Sprintf("projects/%d/issues/%d", projectID, issueIID)
	args := []string{
		"api", endpoint, "-X", "PUT",
		"-f", "title=" + title,
		"-f", "description=" + description,
		"-f", "labels=" + strings.Join(labels, ","),
	}
	if milestoneID != nil {
		args = append(args, "-f", "milestone_id="+strconv.FormatInt(*milestoneID, 10))
	}
	if close {
		args = append(args, "-f", "state_event=close")
	}
	if _, err := client.output(ctx, args...); err != nil {
		return fmt.Errorf("update GitLab issue #%d: %w", issueIID, err)
	}
	return nil
}

func parseCreatedIssue(data []byte) (CreatedIssue, error) {
	var value struct {
		IID *int64 `json:"iid"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return CreatedIssue{}, err
	}
	if value.IID == nil || *value.IID < 1 {
		return CreatedIssue{}, fmt.Errorf("missing or invalid field %q", "iid")
	}
	return CreatedIssue{IID: *value.IID}, nil
}

func (client *Client) WorkItemID(ctx context.Context, projectPath string, iid int64, expectedType string) (string, error) {
	output, err := client.output(ctx,
		"api", "graphql",
		"-f", "query="+workItemByIIDQuery,
		"-f", "fullPath="+projectPath,
		"-f", "iid="+strconv.FormatInt(iid, 10),
	)
	if err != nil {
		return "", fmt.Errorf("query GitLab %s #%d work item ID: %w", expectedType, iid, err)
	}

	response, err := decodeGraphQL[workItemQueryJSON](output)
	if err != nil {
		return "", fmt.Errorf("decode GitLab %s #%d work item ID: %w", expectedType, iid, err)
	}
	if response.Namespace == nil {
		return "", fmt.Errorf("GitLab namespace %q was not found", projectPath)
	}
	workItem := response.Namespace.WorkItem
	if workItem == nil {
		return "", fmt.Errorf("GitLab %s #%d work item was not found", expectedType, iid)
	}
	parsedIID, err := parseGraphQLIID(workItem.IID)
	if err != nil || parsedIID != iid {
		return "", fmt.Errorf("GitLab work item returned an unexpected IID")
	}
	if workItem.ID == nil || strings.TrimSpace(*workItem.ID) == "" {
		return "", fmt.Errorf("GitLab work item has no valid ID")
	}
	if workItem.WorkItemType == nil || workItem.WorkItemType.Name == nil || !strings.EqualFold(*workItem.WorkItemType.Name, expectedType) {
		return "", fmt.Errorf("GitLab work item #%d is not a %s", iid, expectedType)
	}
	return *workItem.ID, nil
}

func (client *Client) TaskTypeID(ctx context.Context, projectPath string) (string, error) {
	output, err := client.output(ctx,
		"api", "graphql",
		"-f", "query="+taskTypeQuery,
		"-f", "fullPath="+projectPath,
	)
	if err != nil {
		return "", fmt.Errorf("query GitLab task work item type: %w", err)
	}
	response, err := decodeGraphQL[workItemQueryJSON](output)
	if err != nil {
		return "", fmt.Errorf("decode GitLab task work item type: %w", err)
	}
	if response.Namespace == nil {
		return "", fmt.Errorf("GitLab namespace %q was not found", projectPath)
	}
	workItemTypes := response.Namespace.WorkItemTypes
	if workItemTypes == nil || workItemTypes.Nodes == nil || len(*workItemTypes.Nodes) != 1 {
		return "", fmt.Errorf("expected exactly one GitLab task work item type")
	}
	taskType := (*workItemTypes.Nodes)[0]
	if taskType.ID == nil || strings.TrimSpace(*taskType.ID) == "" || taskType.Name == nil || !strings.EqualFold(*taskType.Name, "Task") {
		return "", fmt.Errorf("GitLab task work item type is incomplete")
	}
	return *taskType.ID, nil
}

func (client *Client) CreateTask(ctx context.Context, projectPath, taskTypeID, parentID, title, description string) (WorkItemReference, error) {
	input := struct {
		NamespacePath     string `json:"namespacePath"`
		WorkItemTypeID    string `json:"workItemTypeId"`
		Title             string `json:"title"`
		DescriptionWidget struct {
			Description string `json:"description"`
		} `json:"descriptionWidget"`
		HierarchyWidget struct {
			ParentID string `json:"parentId"`
		} `json:"hierarchyWidget"`
	}{NamespacePath: projectPath, WorkItemTypeID: taskTypeID, Title: title}
	input.DescriptionWidget.Description = description
	input.HierarchyWidget.ParentID = parentID
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return WorkItemReference{}, fmt.Errorf("encode GitLab task input: %w", err)
	}
	output, err := client.output(ctx,
		"api", "graphql",
		"-f", "query="+createTaskMutation,
		"-F", "input="+string(inputJSON),
	)
	if err != nil {
		return WorkItemReference{}, fmt.Errorf("create GitLab task %q: %w", title, err)
	}
	return parseWorkItemMutation(output, "workItemCreate", "Task")
}

func (client *Client) SetWorkItemParent(ctx context.Context, taskID, parentID string) error {
	input := struct {
		ID              string `json:"id"`
		HierarchyWidget struct {
			ParentID string `json:"parentId"`
		} `json:"hierarchyWidget"`
	}{ID: taskID}
	input.HierarchyWidget.ParentID = parentID
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode GitLab task parent input: %w", err)
	}
	output, err := client.output(ctx,
		"api", "graphql",
		"-f", "query="+setWorkItemParentMutation,
		"-F", "input="+string(inputJSON),
	)
	if err != nil {
		return fmt.Errorf("set GitLab task parent: %w", err)
	}
	if _, err := parseWorkItemMutation(output, "workItemUpdate", ""); err != nil {
		return fmt.Errorf("set GitLab task parent: %w", err)
	}
	return nil
}

func parseWorkItemMutation(data []byte, field, expectedType string) (WorkItemReference, error) {
	response, err := decodeGraphQL[map[string]json.RawMessage](data)
	if err != nil {
		return WorkItemReference{}, err
	}
	payloadJSON, exists := (*response)[field]
	if !exists {
		return WorkItemReference{}, fmt.Errorf("GraphQL response is missing %s", field)
	}
	var payload *struct {
		WorkItem *workItemJSON `json:"workItem"`
		Errors   []string      `json:"errors"`
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil || payload == nil {
		if err == nil {
			err = fmt.Errorf("GraphQL response is missing %s", field)
		}
		return WorkItemReference{}, err
	}
	if len(payload.Errors) > 0 {
		return WorkItemReference{}, fmt.Errorf("GraphQL mutation errors: %s", strings.Join(payload.Errors, "; "))
	}
	if payload.WorkItem == nil {
		return WorkItemReference{}, fmt.Errorf("GraphQL mutation returned no work item")
	}
	workItem := payload.WorkItem
	if workItem.ID == nil || strings.TrimSpace(*workItem.ID) == "" {
		return WorkItemReference{}, fmt.Errorf("GraphQL mutation returned no work item ID")
	}
	reference := WorkItemReference{ID: *workItem.ID}
	if workItem.IID != nil {
		parsedIID, err := parseGraphQLIID(workItem.IID)
		if err != nil {
			return WorkItemReference{}, fmt.Errorf("GraphQL mutation returned invalid IID: %w", err)
		}
		reference.IID = parsedIID
	}
	if expectedType != "" {
		if reference.IID < 1 {
			return WorkItemReference{}, fmt.Errorf("GraphQL mutation returned no IID")
		}
		if workItem.WorkItemType == nil || workItem.WorkItemType.Name == nil || !strings.EqualFold(*workItem.WorkItemType.Name, expectedType) {
			return WorkItemReference{}, fmt.Errorf("GraphQL mutation returned unexpected work item type")
		}
	}
	return reference, nil
}
