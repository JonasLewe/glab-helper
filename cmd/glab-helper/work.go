package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	gitrepo "github.com/JonasLewe/glab-helper/internal/git"
	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/ui"
	"github.com/JonasLewe/glab-helper/internal/workitem"
)

const (
	workActionBranch      = "Branch (checkout / create)"
	workActionDescription = "Edit description"
	workActionLabels      = "Edit labels"
	workActionAssignee    = "Edit assignee"
	workActionMilestone   = "Edit milestone"
	workActionClose       = "Close issue"
)

func runWorkItemWorkflow(ctx context.Context, picker *ui.Picker, client *gitlab.Client, project gitlab.Project, stdin io.Reader, stdout, stderr io.Writer) int {
	issues, err := client.ListIssues(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab issues for %s: %v\n", project.Path, err)
		return 1
	}
	tasks, err := client.ListTasks(ctx, project.Path)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab tasks for %s: %v\n", project.Path, err)
		return 1
	}
	gitClient := gitrepo.NewClient()
	if err := gitClient.PruneRemoteBranches(ctx); err != nil {
		fmt.Fprintf(stderr, "Cannot refresh remote branches for %s: %v\n", project.Path, err)
		return 1
	}
	branches, err := gitClient.ListBranches(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read current local and remote branches for %s: %v\n", project.Path, err)
		return 1
	}
	return selectWorkItem(ctx, picker, client, gitClient, project.ID, issues, tasks, branches, project.DefaultBranch, stdin, stdout, stderr)
}

func selectWorkItem(
	ctx context.Context,
	picker *ui.Picker,
	gitLabClient *gitlab.Client,
	gitClient *gitrepo.Client,
	projectID int64,
	issues []gitlab.Issue,
	tasks []gitlab.Task,
	branches []gitrepo.Branch,
	defaultBranch string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) int {
	input := bufio.NewReader(stdin)
	candidates := workitem.OpenCandidates(issues, tasks, branches)
	if len(candidates) == 0 {
		fmt.Fprintln(stdout, "No open GitLab issues or tasks found.")
		return 0
	}
	choices := make([]string, len(candidates))
	for index, candidate := range candidates {
		choices[index] = candidate.Display()
	}
	choice, selected, err := picker.Choose(ctx, choices, ui.Options{Prompt: "Issue or task", BorderLabel: "open issues and tasks", Accent: ui.Magenta})
	if err != nil {
		fmt.Fprintf(stderr, "Cannot select a GitLab issue or task: %v\n", err)
		return 1
	}
	if !selected {
		fmt.Fprintln(stdout, "Selection cancelled.")
		return 0
	}
	for index, rendered := range choices {
		if rendered != choice {
			continue
		}
		candidate := candidates[index]
		fmt.Fprintf(stdout, "Selected %s #%d: %s\n", candidate.Kind, candidate.IID, candidate.Title)
		if candidate.ParentIID > 0 {
			fmt.Fprintf(stdout, "Parent issue: #%d\n", candidate.ParentIID)
		}
		if candidate.Branch.Name != "" {
			fmt.Fprintf(stdout, "Existing branch: %s\n", candidate.Branch.Name)
		}
		var issue *gitlab.Issue
		if candidate.Kind == workitem.Issue {
			for issueIndex := range issues {
				if issues[issueIndex].IID == candidate.IID {
					issue = &issues[issueIndex]
					break
				}
			}
			if issue == nil {
				fmt.Fprintf(stderr, "Cannot find the selected GitLab issue #%d in the complete issue snapshot.\n", candidate.IID)
				return 1
			}
		}

		for {
			actions := []string{workActionBranch}
			if issue != nil {
				actions = append(actions, workActionDescription, workActionLabels, workActionAssignee, workActionMilestone, workActionClose)
			}
			workAction, selected, err := picker.Choose(ctx, actions, ui.Options{
				Prompt:      "Action",
				BorderLabel: fmt.Sprintf("%s #%d", candidate.Kind, candidate.IID),
				Header:      "ENTER=select  ESC=done",
				Accent:      ui.Magenta,
			})
			if err != nil {
				fmt.Fprintf(stderr, "Cannot select a work item action: %v\n", err)
				return 1
			}
			if !selected {
				fmt.Fprintln(stdout, "Done.")
				return 0
			}

			switch workAction {
			case workActionBranch:
				return manageWorkItemBranch(ctx, picker, gitClient, candidate, branches, defaultBranch, input, stdout, stderr)
			case workActionDescription:
				err = editIssueDescription(ctx, gitLabClient, projectID, issue, stdin, stdout, stderr)
			case workActionLabels:
				err = editIssueLabels(ctx, picker, gitLabClient, projectID, issue, stdout)
			case workActionAssignee:
				err = editIssueAssignee(ctx, picker, gitLabClient, projectID, issue, stdout)
			case workActionMilestone:
				err = editIssueMilestone(ctx, picker, gitLabClient, projectID, issue, stdout)
			case workActionClose:
				closed, closeErr := closeIssue(ctx, gitLabClient, projectID, issue, input, stdout)
				err = closeErr
				if err == nil && closed {
					return 0
				}
			}
			if err != nil {
				fmt.Fprintf(stderr, "Cannot apply the selected issue action: %v\n", err)
				return 1
			}
		}
	}
	fmt.Fprintln(stderr, "Cannot match the selected GitLab issue or task.")
	return 1
}

func editIssueDescription(
	ctx context.Context,
	client *gitlab.Client,
	projectID int64,
	issue *gitlab.Issue,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	fmt.Fprintln(stdout, "Opening the current issue description in your editor...")
	description, err := ui.EditText(ctx, issue.Description, stdin, stdout, stderr)
	if err != nil {
		return fmt.Errorf("edit GitLab issue #%d description: %w", issue.IID, err)
	}
	if description == issue.Description {
		fmt.Fprintln(stdout, "Description unchanged.")
		return nil
	}
	if err := client.UpdateIssueDescription(ctx, projectID, issue.IID, description); err != nil {
		return err
	}
	issue.Description = description
	fmt.Fprintf(stdout, "Updated description for issue #%d.\n", issue.IID)
	return nil
}

func editIssueLabels(ctx context.Context, picker *ui.Picker, client *gitlab.Client, projectID int64, issue *gitlab.Issue, stdout io.Writer) error {
	labels, err := client.ListLabels(ctx, projectID)
	if err != nil {
		return fmt.Errorf("read GitLab labels: %w", err)
	}
	sort.SliceStable(labels, func(left, right int) bool {
		return strings.ToLower(labels[left].Name) < strings.ToLower(labels[right].Name)
	})
	current := make(map[string]struct{}, len(issue.Labels))
	for _, label := range issue.Labels {
		current[label] = struct{}{}
	}
	choices := make([]string, 0, len(labels))
	labelByChoice := make(map[string]string, len(labels))
	for _, label := range labels {
		action := "Add label: "
		if _, exists := current[label.Name]; exists {
			action = "Remove label: "
		}
		choice := action + label.Name
		choices = append(choices, choice)
		labelByChoice[choice] = label.Name
	}
	if len(choices) == 0 {
		fmt.Fprintln(stdout, "No GitLab labels are available.")
		return nil
	}
	choice, selected, err := picker.Choose(ctx, choices, ui.Options{Prompt: "Label", BorderLabel: fmt.Sprintf("issue #%d labels", issue.IID), Accent: ui.Magenta})
	if err != nil {
		return fmt.Errorf("select a GitLab issue label: %w", err)
	}
	if !selected {
		fmt.Fprintln(stdout, "Labels unchanged.")
		return nil
	}
	labelName := labelByChoice[choice]
	updated := append([]string(nil), issue.Labels...)
	if _, exists := current[labelName]; exists {
		updated = removeString(updated, labelName)
	} else {
		updated = append(updated, labelName)
	}
	sort.Strings(updated)
	if err := client.SetIssueLabels(ctx, projectID, issue.IID, updated); err != nil {
		return err
	}
	issue.Labels = updated
	fmt.Fprintf(stdout, "Updated labels for issue #%d: %s\n", issue.IID, listOrNone(updated))
	return nil
}

func editIssueAssignee(ctx context.Context, picker *ui.Picker, client *gitlab.Client, projectID int64, issue *gitlab.Issue, stdout io.Writer) error {
	members, err := client.ListMembers(ctx, projectID)
	if err != nil {
		return fmt.Errorf("read GitLab project members: %w", err)
	}
	sort.SliceStable(members, func(left, right int) bool {
		return strings.ToLower(members[left].Username) < strings.ToLower(members[right].Username)
	})
	choices := []string{"Unassign"}
	memberByChoice := make(map[string]gitlab.Member, len(members))
	for _, member := range members {
		choice := member.Username
		if strings.TrimSpace(member.Name) != "" {
			choice += " (" + member.Name + ")"
		}
		choices = append(choices, choice)
		memberByChoice[choice] = member
	}
	choice, selected, err := picker.Choose(ctx, choices, ui.Options{Prompt: "Assignee", BorderLabel: fmt.Sprintf("issue #%d assignee", issue.IID), Accent: ui.Magenta})
	if err != nil {
		return fmt.Errorf("select a GitLab issue assignee: %w", err)
	}
	if !selected {
		fmt.Fprintln(stdout, "Assignee unchanged.")
		return nil
	}
	if choice == "Unassign" {
		if len(issue.Assignees) == 0 {
			fmt.Fprintln(stdout, "Issue is already unassigned.")
			return nil
		}
		if err := client.SetIssueAssignee(ctx, projectID, issue.IID, nil); err != nil {
			return err
		}
		issue.Assignees = nil
		fmt.Fprintf(stdout, "Unassigned issue #%d.\n", issue.IID)
		return nil
	}
	member := memberByChoice[choice]
	if len(issue.Assignees) == 1 && issue.Assignees[0] == member.Username {
		fmt.Fprintf(stdout, "Issue #%d is already assigned to %s.\n", issue.IID, member.Username)
		return nil
	}
	if err := client.SetIssueAssignee(ctx, projectID, issue.IID, &member.ID); err != nil {
		return err
	}
	issue.Assignees = []string{member.Username}
	fmt.Fprintf(stdout, "Assigned issue #%d to %s.\n", issue.IID, member.Username)
	return nil
}

func editIssueMilestone(ctx context.Context, picker *ui.Picker, client *gitlab.Client, projectID int64, issue *gitlab.Issue, stdout io.Writer) error {
	milestones, err := client.ListMilestones(ctx, projectID)
	if err != nil {
		return fmt.Errorf("read GitLab milestones: %w", err)
	}
	sort.SliceStable(milestones, func(left, right int) bool {
		if strings.EqualFold(milestones[left].Title, milestones[right].Title) {
			return milestones[left].ID < milestones[right].ID
		}
		return strings.ToLower(milestones[left].Title) < strings.ToLower(milestones[right].Title)
	})
	choices := []string{"Remove milestone"}
	milestoneByChoice := make(map[string]gitlab.Milestone, len(milestones))
	for _, milestone := range milestones {
		choice := fmt.Sprintf("%s [milestone %d]", milestone.Title, milestone.ID)
		choices = append(choices, choice)
		milestoneByChoice[choice] = milestone
	}
	choice, selected, err := picker.Choose(ctx, choices, ui.Options{Prompt: "Milestone", BorderLabel: fmt.Sprintf("issue #%d milestone", issue.IID), Accent: ui.Magenta})
	if err != nil {
		return fmt.Errorf("select a GitLab issue milestone: %w", err)
	}
	if !selected {
		fmt.Fprintln(stdout, "Milestone unchanged.")
		return nil
	}
	if choice == "Remove milestone" {
		if issue.Milestone == nil {
			fmt.Fprintln(stdout, "Issue already has no milestone.")
			return nil
		}
		if err := client.SetIssueMilestone(ctx, projectID, issue.IID, nil); err != nil {
			return err
		}
		issue.Milestone = nil
		fmt.Fprintf(stdout, "Removed the milestone from issue #%d.\n", issue.IID)
		return nil
	}
	milestone := milestoneByChoice[choice]
	if issue.Milestone != nil && issue.Milestone.Title == milestone.Title {
		fmt.Fprintf(stdout, "Issue #%d already uses milestone %s.\n", issue.IID, milestone.Title)
		return nil
	}
	if err := client.SetIssueMilestone(ctx, projectID, issue.IID, &milestone.ID); err != nil {
		return err
	}
	issue.Milestone = &gitlab.IssueMilestone{Title: milestone.Title}
	fmt.Fprintf(stdout, "Set issue #%d milestone to %s.\n", issue.IID, milestone.Title)
	return nil
}

func closeIssue(ctx context.Context, client *gitlab.Client, projectID int64, issue *gitlab.Issue, stdin *bufio.Reader, stdout io.Writer) (bool, error) {
	confirmed, err := readConfirmation(stdin, stdout, fmt.Sprintf("Close issue #%d? [y/N] ", issue.IID))
	if err != nil {
		return false, fmt.Errorf("read close confirmation: %w", err)
	}
	if !confirmed {
		fmt.Fprintln(stdout, "Issue close cancelled.")
		return false, nil
	}
	if err := client.CloseIssue(ctx, projectID, issue.IID); err != nil {
		return false, err
	}
	issue.State = "closed"
	fmt.Fprintf(stdout, "Closed issue #%d.\n", issue.IID)
	return true, nil
}

func removeString(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func manageWorkItemBranch(
	ctx context.Context,
	picker *ui.Picker,
	gitClient *gitrepo.Client,
	candidate workitem.Candidate,
	branches []gitrepo.Branch,
	defaultBranch string,
	stdin *bufio.Reader,
	stdout, stderr io.Writer,
) int {
	if candidate.Branch.Name != "" {
		confirmed, err := readConfirmation(stdin, stdout, fmt.Sprintf("Check out existing branch %s? [y/N] ", candidate.Branch.Name))
		if err != nil {
			fmt.Fprintf(stderr, "Cannot read branch checkout confirmation: %v\n", err)
			return 1
		}
		if !confirmed {
			fmt.Fprintln(stdout, "Branch checkout cancelled. No changes were applied.")
			return 0
		}
		if err := gitClient.CheckoutBranch(ctx, candidate.Branch); err != nil {
			fmt.Fprintf(stderr, "Cannot check out the existing work item branch: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Checked out branch %s.\n", candidate.Branch.Name)
		return 0
	}

	defaultName := workitem.BranchName(candidate.IID, candidate.Title)
	fmt.Fprintf(stdout, "Branch name [%s]: ", defaultName)
	branchName, err := stdin.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(stderr, "Cannot read the branch name: %v\n", err)
		return 1
	}
	if errors.Is(err, io.EOF) && branchName == "" {
		fmt.Fprintln(stdout, "Branch creation cancelled. No changes were applied.")
		return 0
	}
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		branchName = defaultName
	}
	for _, branch := range branches {
		if branch.Name != branchName {
			continue
		}
		confirmed, err := readConfirmation(stdin, stdout, fmt.Sprintf("Branch %s already exists. Check it out? [y/N] ", branchName))
		if err != nil {
			fmt.Fprintf(stderr, "Cannot read branch checkout confirmation: %v\n", err)
			return 1
		}
		if !confirmed {
			fmt.Fprintln(stdout, "Branch checkout cancelled. No changes were applied.")
			return 0
		}
		if err := gitClient.CheckoutBranch(ctx, branch); err != nil {
			fmt.Fprintf(stderr, "Cannot check out the existing branch: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Checked out branch %s.\n", branch.Name)
		return 0
	}

	if len(branches) == 0 {
		fmt.Fprintln(stderr, "Cannot create a branch because the repository has no local or remote base branch.")
		return 1
	}
	baseBranches := append([]gitrepo.Branch(nil), branches...)
	if defaultBranch != "" {
		for index, branch := range baseBranches {
			if branch.Name != defaultBranch || index == 0 {
				continue
			}
			copy(baseBranches[1:index+1], baseBranches[0:index])
			baseBranches[0] = branch
			break
		}
	}
	baseChoices := make([]string, len(baseBranches))
	for index, branch := range baseBranches {
		baseChoices[index] = branch.Name
		if branch.Name == defaultBranch {
			baseChoices[index] += " (default)"
		}
	}
	baseChoice, selected, err := picker.Choose(ctx, baseChoices, ui.Options{Prompt: "Base branch", BorderLabel: "base branch"})
	if err != nil {
		fmt.Fprintf(stderr, "Cannot select a base branch: %v\n", err)
		return 1
	}
	if !selected {
		fmt.Fprintln(stdout, "Branch creation cancelled. No changes were applied.")
		return 0
	}
	baseIndex := -1
	for index, rendered := range baseChoices {
		if rendered == baseChoice {
			baseIndex = index
			break
		}
	}
	if baseIndex < 0 {
		fmt.Fprintln(stderr, "Cannot match the selected base branch.")
		return 1
	}
	if err := gitClient.CreateBranch(ctx, branchName, baseBranches[baseIndex]); err != nil {
		fmt.Fprintf(stderr, "Cannot create the work item branch: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Created branch %s from %s.\n", branchName, baseBranches[baseIndex].Name)
	confirmed, err := readConfirmation(stdin, stdout, "Check out the new branch now? [y/N] ")
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read branch checkout confirmation: %v\n", err)
		return 1
	}
	if !confirmed {
		return 0
	}
	if err := gitClient.CheckoutBranch(ctx, gitrepo.Branch{Name: branchName, Local: true}); err != nil {
		fmt.Fprintf(stderr, "Branch %s was created, but checkout failed: %v\n", branchName, err)
		return 1
	}
	fmt.Fprintf(stdout, "Checked out branch %s.\n", branchName)
	return 0
}
