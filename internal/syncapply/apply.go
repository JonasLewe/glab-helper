package syncapply

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/syncplan"
)

type Client interface {
	CreateLabel(context.Context, int64, string) (gitlab.Label, error)
	CreateBoardList(context.Context, int64, int64, int64, string) error
	CreateMilestone(context.Context, int64, string, string, bool) (gitlab.Milestone, error)
	UpdateMilestone(context.Context, int64, int64, string, string, bool) error
	CreateIssue(context.Context, int64, string, string, []string, *int64, bool) (gitlab.CreatedIssue, error)
	UpdateIssue(context.Context, int64, int64, string, string, []string, *int64, bool) error
	WorkItemID(context.Context, string, int64, string) (string, error)
	TaskTypeID(context.Context, string) (string, error)
	CreateTask(context.Context, string, string, string, string, string) (gitlab.WorkItemReference, error)
	SetWorkItemParent(context.Context, string, string) error
}

type Result struct {
	Applied int
	Total   int
}

func Apply(ctx context.Context, client Client, projectID int64, projectPath string, plan syncplan.Plan) (Result, error) {
	result := Result{Total: len(plan.Actions)}
	if err := validatePlan(plan); err != nil {
		return result, fmt.Errorf("validate synchronization plan before writes: %w", err)
	}

	references := make(map[string]syncplan.Reference, len(plan.References))
	for sourceID, reference := range plan.References {
		references[sourceID] = reference
	}
	workItemIDs := make(map[string]string)
	for sourceID, reference := range references {
		if reference.Target == syncplan.Task && reference.ID != "" {
			workItemIDs[sourceID] = reference.ID
		}
	}
	taskTypeID := ""
	labelIDs := make(map[string]int64)
	for _, action := range plan.Actions {
		if action.Desired.Target == syncplan.BoardList && action.Desired.LabelID > 0 {
			labelIDs[action.Desired.Title] = action.Desired.LabelID
		}
	}

	for index, action := range plan.Actions {
		if err := applyAction(ctx, client, projectID, projectPath, action, references, workItemIDs, labelIDs, &taskTypeID); err != nil {
			identity := action.Desired.SourceID
			if identity == "" {
				identity = action.Desired.Title
			}
			return result, fmt.Errorf("action %d of %d (%s %s %q): %w", index+1, len(plan.Actions), action.Operation, action.Desired.Target, identity, err)
		}
		result.Applied++
	}
	return result, nil
}

func applyAction(ctx context.Context, client Client, projectID int64, projectPath string, action syncplan.Action, references map[string]syncplan.Reference, workItemIDs map[string]string, labelIDs map[string]int64, taskTypeID *string) error {
	desired := action.Desired
	switch desired.Target {
	case syncplan.Label:
		label, err := client.CreateLabel(ctx, projectID, desired.Title)
		if err != nil {
			return err
		}
		labelIDs[label.Name] = label.ID
		return nil
	case syncplan.BoardList:
		labelID := desired.LabelID
		if labelID < 1 {
			labelID = labelIDs[desired.Title]
		}
		if labelID < 1 {
			return fmt.Errorf("board list label %q has no resolved GitLab label ID", desired.Title)
		}
		return client.CreateBoardList(ctx, projectID, desired.BoardID, labelID, desired.Title)
	case syncplan.Milestone:
		if action.Operation == syncplan.Create {
			milestone, err := client.CreateMilestone(ctx, projectID, desired.Title, desired.Description, desired.Resolved)
			if err != nil {
				return err
			}
			references[desired.SourceID] = syncplan.Reference{Target: syncplan.Milestone, ID: strconv.FormatInt(milestone.ID, 10)}
			return nil
		}
		milestoneID, err := strconv.ParseInt(action.CurrentID, 10, 64)
		if err != nil || milestoneID < 1 {
			return fmt.Errorf("invalid current milestone ID %q", action.CurrentID)
		}
		return client.UpdateMilestone(ctx, projectID, milestoneID, desired.Title, desired.Description, hasChange(action, "state"))
	case syncplan.Issue:
		milestoneID, err := resolveMilestoneID(desired.ParentSourceID, references)
		if err != nil {
			return err
		}
		if action.Operation == syncplan.Create {
			issue, err := client.CreateIssue(ctx, projectID, desired.Title, desired.Description, desired.Labels, milestoneID, desired.Resolved)
			if err != nil {
				return err
			}
			references[desired.SourceID] = syncplan.Reference{Target: syncplan.Issue, ID: strconv.FormatInt(issue.IID, 10), IID: issue.IID}
			return nil
		}
		if err := client.UpdateIssue(ctx, projectID, action.CurrentIID, desired.Title, desired.Description, desired.Labels, milestoneID, hasChange(action, "state")); err != nil {
			return err
		}
		return nil
	case syncplan.Task:
		parentReference, exists := references[desired.ParentSourceID]
		if !exists || parentReference.Target != syncplan.Issue || parentReference.IID < 1 {
			return fmt.Errorf("task parent %q has no resolved GitLab issue", desired.ParentSourceID)
		}
		parentID, exists := workItemIDs[desired.ParentSourceID]
		if !exists {
			var err error
			parentID, err = client.WorkItemID(ctx, projectPath, parentReference.IID, "Issue")
			if err != nil {
				return err
			}
			workItemIDs[desired.ParentSourceID] = parentID
		}
		if action.Operation == syncplan.Create {
			if *taskTypeID == "" {
				var err error
				*taskTypeID, err = client.TaskTypeID(ctx, projectPath)
				if err != nil {
					return err
				}
			}
			task, err := client.CreateTask(ctx, projectPath, *taskTypeID, parentID, desired.Title, desired.Description)
			if err != nil {
				return err
			}
			references[desired.SourceID] = syncplan.Reference{Target: syncplan.Task, ID: task.ID, IID: task.IID}
			workItemIDs[desired.SourceID] = task.ID
			return client.UpdateIssue(ctx, projectID, task.IID, desired.Title, desired.Description, desired.Labels, nil, desired.Resolved)
		}
		taskID := workItemIDs[desired.SourceID]
		if taskID == "" {
			return fmt.Errorf("task %q has no GitLab work item ID", desired.SourceID)
		}
		if hasChange(action, "parent") {
			if err := client.SetWorkItemParent(ctx, taskID, parentID); err != nil {
				return err
			}
		}
		if hasChangeOtherThan(action, "parent") {
			return client.UpdateIssue(ctx, projectID, action.CurrentIID, desired.Title, desired.Description, desired.Labels, nil, hasChange(action, "state"))
		}
		return nil
	default:
		return fmt.Errorf("unsupported target %q", desired.Target)
	}
}

func resolveMilestoneID(sourceID string, references map[string]syncplan.Reference) (*int64, error) {
	if sourceID == "" {
		return nil, nil
	}
	reference, exists := references[sourceID]
	if !exists || reference.Target != syncplan.Milestone {
		return nil, fmt.Errorf("issue parent %q has no resolved GitLab milestone", sourceID)
	}
	milestoneID, err := strconv.ParseInt(reference.ID, 10, 64)
	if err != nil || milestoneID < 1 {
		return nil, fmt.Errorf("issue parent %q has invalid GitLab milestone ID %q", sourceID, reference.ID)
	}
	return &milestoneID, nil
}

func hasChange(action syncplan.Action, field string) bool {
	for _, change := range action.Changes {
		if change.Field == field {
			return true
		}
	}
	return false
}

func hasChangeOtherThan(action syncplan.Action, field string) bool {
	for _, change := range action.Changes {
		if change.Field != field {
			return true
		}
	}
	return false
}

func validatePlan(plan syncplan.Plan) error {
	available := make(map[string]syncplan.Target, len(plan.References))
	for sourceID, reference := range plan.References {
		available[sourceID] = reference.Target
	}
	seenActions := make(map[string]struct{})
	availableLabels := make(map[string]struct{})
	seenBoardLists := make(map[string]struct{})
	for _, action := range plan.Actions {
		if action.Desired.Target == syncplan.BoardList && action.Desired.LabelID > 0 {
			availableLabels[action.Desired.Title] = struct{}{}
		}
	}
	for index, action := range plan.Actions {
		if action.Operation != syncplan.Create && action.Operation != syncplan.Update {
			return fmt.Errorf("action %d has unsupported operation %q", index+1, action.Operation)
		}
		if action.Desired.Target == syncplan.Label {
			if action.Operation != syncplan.Create || action.Desired.Title == "" {
				return fmt.Errorf("action %d has invalid label operation", index+1)
			}
			availableLabels[action.Desired.Title] = struct{}{}
			continue
		}
		if action.Desired.Target == syncplan.BoardList {
			if action.Operation != syncplan.Create || action.Desired.Title == "" || action.Desired.BoardID < 1 || action.Desired.BoardName == "" {
				return fmt.Errorf("action %d has invalid board list operation", index+1)
			}
			if _, exists := availableLabels[action.Desired.Title]; !exists {
				return fmt.Errorf("action %d board list label %q is not available", index+1, action.Desired.Title)
			}
			identity := strconv.FormatInt(action.Desired.BoardID, 10) + ":" + action.Desired.Title
			if _, exists := seenBoardLists[identity]; exists {
				return fmt.Errorf("board list %q has multiple actions", identity)
			}
			seenBoardLists[identity] = struct{}{}
			continue
		}
		if action.Desired.SourceID == "" || action.Desired.Title == "" {
			return fmt.Errorf("action %d has incomplete desired work item", index+1)
		}
		if action.Desired.Target != syncplan.Milestone && action.Desired.Target != syncplan.Issue && action.Desired.Target != syncplan.Task {
			return fmt.Errorf("action %d has unsupported target %q", index+1, action.Desired.Target)
		}
		if _, exists := seenActions[action.Desired.SourceID]; exists {
			return fmt.Errorf("source %q has multiple actions", action.Desired.SourceID)
		}
		seenActions[action.Desired.SourceID] = struct{}{}
		expectedParent := syncplan.Target("")
		switch action.Desired.Target {
		case syncplan.Issue:
			expectedParent = syncplan.Milestone
		case syncplan.Task:
			expectedParent = syncplan.Issue
		}
		parent := action.Desired.ParentSourceID
		if action.Desired.Target == syncplan.Task && parent == "" {
			return fmt.Errorf("task source %q has no issue parent", action.Desired.SourceID)
		}
		if parent != "" && available[parent] != expectedParent {
			return fmt.Errorf("source %q parent %q is not an available %s", action.Desired.SourceID, parent, expectedParent)
		}
		available[action.Desired.SourceID] = action.Desired.Target
	}
	return nil
}
