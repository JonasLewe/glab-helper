package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	gitrepo "github.com/JonasLewe/glab-helper/internal/git"
	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/projectbackup"
	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/syncapply"
	"github.com/JonasLewe/glab-helper/internal/syncplan"
	"github.com/JonasLewe/glab-helper/internal/ui"
	"github.com/JonasLewe/glab-helper/internal/workitem"
	"github.com/JonasLewe/glab-helper/internal/youtrack"
)

const (
	actionSyncMilestones    = "Sync YouTrack milestones only"
	actionSyncAll           = "Sync all YouTrack work items"
	actionPreviewMilestones = "Preview YouTrack milestones only"
	actionPreviewAll        = "Preview all YouTrack work items"
	actionCreateIssue       = "Create GitLab issue"
	actionExportSnapshot    = "Export GitLab snapshot"

	createIssueFromYouTrack = "From YouTrack"
	createIssueManually     = "Manual"
)

type synchronizationScope int

const (
	syncAll synchronizationScope = iota
	syncMilestones
)

var currentTime = time.Now

func projectHasTarget(config projectconfig.Config, target string) bool {
	for _, configured := range config.GitLab.Targets {
		if configured == target {
			return true
		}
	}
	return false
}

func projectConfigForScope(config projectconfig.Config, scope synchronizationScope) (projectconfig.Config, error) {
	if scope == syncAll {
		return config, nil
	}
	if scope != syncMilestones {
		return projectconfig.Config{}, fmt.Errorf("unknown synchronization scope")
	}
	if !projectHasTarget(config, "milestone") {
		return projectconfig.Config{}, fmt.Errorf("the project configuration has no milestone target")
	}
	result := config
	result.GitLab.Targets = make(map[string]string, len(config.GitLab.Targets))
	for role, target := range config.GitLab.Targets {
		if target == "milestone" {
			result.GitLab.Targets[role] = target
		} else {
			result.GitLab.Targets[role] = "ignore"
		}
	}
	if err := result.Validate(); err != nil {
		return projectconfig.Config{}, fmt.Errorf("validate milestone-only project configuration: %w", err)
	}
	return result, nil
}

func createIssueWorkflow(
	ctx context.Context,
	picker *ui.Picker,
	client *gitlab.Client,
	project gitlab.Project,
	youTrackAvailable bool,
	youTrackConfig youtrack.Config,
	projectConfig projectconfig.Config,
	stdin io.Reader,
	stdout, stderr io.Writer,
) int {
	if youTrackAvailable {
		mode, selected, err := picker.Choose(ctx, []string{createIssueFromYouTrack, createIssueManually}, ui.Options{Prompt: "Create issue", BorderLabel: "create GitLab issue"})
		if err != nil {
			fmt.Fprintf(stderr, "Cannot select an issue creation mode: %v\n", err)
			return 1
		}
		if !selected {
			fmt.Fprintln(stdout, "Issue creation cancelled.")
			return 0
		}
		if mode == createIssueFromYouTrack {
			return createIssueFromSource(ctx, picker, client, project, youTrackConfig, projectConfig, stdin, stdout, stderr)
		}
	}
	return createManualIssue(ctx, picker, client, project, stdin, stdout, stderr)
}

func createIssueFromSource(
	ctx context.Context,
	picker *ui.Picker,
	client *gitlab.Client,
	project gitlab.Project,
	youTrackConfig youtrack.Config,
	projectConfig projectconfig.Config,
	stdin io.Reader,
	stdout, stderr io.Writer,
) int {
	snapshot, err := readYouTrackSnapshot(ctx, youTrackConfig, projectConfig)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the complete YouTrack source snapshot: %v\n", err)
		return 1
	}
	current, err := readCurrentSyncState(ctx, client, project)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the complete GitLab synchronization state: %v\n", err)
		return 1
	}
	plan, err := syncplan.Build(snapshot, projectConfig, current)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot build a safe YouTrack issue creation plan: %v\n", err)
		return 1
	}
	choices := make([]string, 0)
	sourceByChoice := make(map[string]string)
	for _, action := range plan.Actions {
		if action.Operation != syncplan.Create || action.Desired.Target != syncplan.Issue {
			continue
		}
		choice := fmt.Sprintf("%-12s  %s", action.Desired.SourceID, action.Desired.Title)
		choices = append(choices, choice)
		sourceByChoice[choice] = action.Desired.SourceID
	}
	if len(choices) == 0 {
		fmt.Fprintln(stdout, "Every YouTrack item mapped to a GitLab issue is already synchronized.")
		return 0
	}
	sort.Strings(choices)
	choice, selected, err := picker.Choose(ctx, choices, ui.Options{Prompt: "YouTrack issue", BorderLabel: "unsynchronized YouTrack issues"})
	if err != nil {
		fmt.Fprintf(stderr, "Cannot select an unsynchronized YouTrack issue: %v\n", err)
		return 1
	}
	if !selected {
		fmt.Fprintln(stdout, "Issue creation cancelled.")
		return 0
	}
	selectedPlan, err := planForSingleIssue(plan, sourceByChoice[choice])
	if err != nil {
		fmt.Fprintf(stderr, "Cannot isolate a safe YouTrack issue creation plan: %v\n", err)
		return 1
	}
	syncplan.WritePreview(stdout, project.Path, selectedPlan)
	confirmed, err := readConfirmation(bufio.NewReader(stdin), stdout, "Apply this issue creation plan? [y/N] ")
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read issue creation confirmation: %v\n", err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(stdout, "Issue creation cancelled. No changes were applied.")
		return 0
	}
	result, err := syncapply.Apply(ctx, client, project.ID, project.Path, selectedPlan)
	if err != nil {
		fmt.Fprintf(stderr, "Issue creation stopped after %d of %d completed actions: %v\n", result.Applied, result.Total, err)
		return 1
	}
	fmt.Fprintf(stdout, "Applied %d GitLab actions for the selected YouTrack issue. YouTrack remained read-only.\n", result.Applied)
	return 0
}

func readCurrentSyncState(ctx context.Context, client *gitlab.Client, project gitlab.Project) (syncplan.Current, error) {
	issues, err := client.ListIssues(ctx, project.ID)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab issues: %w", err)
	}
	tasks, err := client.ListTasks(ctx, project.Path)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab tasks: %w", err)
	}
	milestones, err := client.ListMilestones(ctx, project.ID)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab milestones: %w", err)
	}
	labels, err := client.ListLabels(ctx, project.ID)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab labels: %w", err)
	}
	boards, err := client.ListBoards(ctx, project.ID)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab issue boards: %w", err)
	}
	return syncplan.Current{Issues: issues, Tasks: tasks, Milestones: milestones, Labels: labels, Boards: boards}, nil
}

func planForSingleIssue(plan syncplan.Plan, sourceID string) (syncplan.Plan, error) {
	var selected *syncplan.Action
	for index := range plan.Actions {
		action := &plan.Actions[index]
		if action.Desired.SourceID == sourceID && action.Operation == syncplan.Create && action.Desired.Target == syncplan.Issue {
			selected = action
			break
		}
	}
	if selected == nil {
		return syncplan.Plan{}, fmt.Errorf("source %q has no GitLab issue creation action", sourceID)
	}
	neededSources := map[string]struct{}{sourceID: {}}
	if selected.Desired.ParentSourceID != "" {
		neededSources[selected.Desired.ParentSourceID] = struct{}{}
	}
	neededLabels := make(map[string]struct{}, len(selected.Desired.Labels))
	for _, name := range selected.Desired.Labels {
		neededLabels[name] = struct{}{}
	}
	filtered := syncplan.Plan{References: plan.References}
	for _, action := range plan.Actions {
		include := false
		switch action.Desired.Target {
		case syncplan.Label, syncplan.BoardList:
			_, include = neededLabels[action.Desired.Title]
		default:
			_, include = neededSources[action.Desired.SourceID]
		}
		if include {
			filtered.Actions = append(filtered.Actions, action)
		}
	}
	return filtered, nil
}

const manualDescriptionTemplate = `## Context


## Acceptance Criteria
- [ ] ...

## Dependencies
- Blocked by: #
- Enables: #`

const manualDefaultLabelColor = "#428BCA"

type plannedLabel struct {
	Name  string
	Color string
}

func createManualIssue(ctx context.Context, picker *ui.Picker, client *gitlab.Client, project gitlab.Project, stdin io.Reader, stdout, stderr io.Writer) int {
	labels, err := client.ListLabels(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab labels for manual issue creation: %v\n", err)
		return 1
	}
	members, err := client.ListMembers(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab project members for manual issue creation: %v\n", err)
		return 1
	}
	milestones, err := client.ListMilestones(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab milestones for manual issue creation: %v\n", err)
		return 1
	}

	input := bufio.NewReader(stdin)
	fmt.Fprint(stdout, "Issue title: ")
	title, err := input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(stderr, "Cannot read the issue title: %v\n", err)
		return 1
	}
	title = strings.TrimSpace(title)
	if title == "" {
		fmt.Fprintln(stderr, "Issue creation requires a non-empty title.")
		return 1
	}

	labelsByName := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		labelsByName[strings.ToLower(label.Name)] = struct{}{}
	}
	plannedLabels := make(map[string]plannedLabel)
	chosenLabels := make([]string, 0)
	for {
		sort.SliceStable(labels, func(left, right int) bool {
			return strings.ToLower(labels[left].Name) < strings.ToLower(labels[right].Name)
		})
		labelChoices := []string{"No labels", "Create new label..."}
		labelByChoice := make(map[string]string, len(labels)+len(plannedLabels))
		for _, label := range labels {
			choice := "Label: " + label.Name
			labelChoices = append(labelChoices, choice)
			labelByChoice[choice] = label.Name
		}
		plannedNames := make([]string, 0, len(plannedLabels))
		for _, label := range plannedLabels {
			plannedNames = append(plannedNames, label.Name)
		}
		sort.Strings(plannedNames)
		for _, name := range plannedNames {
			choice := "New label: " + name
			labelChoices = append(labelChoices, choice)
			labelByChoice[choice] = name
		}
		selectedLabels, selected, err := picker.ChooseMany(ctx, labelChoices, ui.Options{Prompt: "Labels", BorderLabel: "issue labels"})
		if err != nil {
			fmt.Fprintf(stderr, "Cannot select issue labels: %v\n", err)
			return 1
		}
		if !selected {
			fmt.Fprintln(stdout, "Issue creation cancelled.")
			return 0
		}
		createLabel := false
		chosenLabels = chosenLabels[:0]
		for _, choice := range selectedLabels {
			if choice == "Create new label..." {
				createLabel = true
				continue
			}
			if name := labelByChoice[choice]; name != "" {
				chosenLabels = append(chosenLabels, name)
			}
		}
		if !createLabel {
			sort.Strings(chosenLabels)
			break
		}

		fmt.Fprint(stdout, "New label name: ")
		name, readErr := input.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			fmt.Fprintf(stderr, "Cannot read the new label name: %v\n", readErr)
			return 1
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, "\r\n,") {
			fmt.Fprintln(stderr, "A new label requires a non-empty name without commas or line breaks.")
			return 1
		}
		normalizedName := strings.ToLower(name)
		if _, exists := labelsByName[normalizedName]; exists {
			fmt.Fprintf(stderr, "GitLab label %q already exists; select it from the list.\n", name)
			return 1
		}
		if _, exists := plannedLabels[normalizedName]; exists {
			fmt.Fprintf(stderr, "GitLab label %q is already planned.\n", name)
			return 1
		}
		fmt.Fprintf(stdout, "New label color [%s]: ", manualDefaultLabelColor)
		color, readErr := input.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			fmt.Fprintf(stderr, "Cannot read the new label color: %v\n", readErr)
			return 1
		}
		color = strings.TrimSpace(color)
		if color == "" {
			color = manualDefaultLabelColor
		}
		if !validHexColor(color) {
			fmt.Fprintln(stderr, "Label color must use #RRGGBB format.")
			return 1
		}
		plannedLabels[normalizedName] = plannedLabel{Name: name, Color: color}
		fmt.Fprintf(stdout, "Label %q is planned; select all final labels in the reopened list.\n", name)
	}

	sort.SliceStable(members, func(left, right int) bool {
		return strings.ToLower(members[left].Username) < strings.ToLower(members[right].Username)
	})
	memberChoices := []string{"No assignee"}
	membersByChoice := make(map[string]gitlab.Member, len(members))
	for _, member := range members {
		choice := member.Username
		if strings.TrimSpace(member.Name) != "" {
			choice += " (" + member.Name + ")"
		}
		memberChoices = append(memberChoices, choice)
		membersByChoice[choice] = member
	}
	memberChoice, selected, err := picker.Choose(ctx, memberChoices, ui.Options{Prompt: "Assignee", BorderLabel: "issue assignee"})
	if err != nil {
		fmt.Fprintf(stderr, "Cannot select an issue assignee: %v\n", err)
		return 1
	}
	if !selected {
		fmt.Fprintln(stdout, "Issue creation cancelled.")
		return 0
	}
	var assigneeID *int64
	assigneeName := "none"
	if memberChoice != "No assignee" {
		member := membersByChoice[memberChoice]
		assigneeID = &member.ID
		assigneeName = member.Username
	}

	sort.SliceStable(milestones, func(left, right int) bool {
		if strings.EqualFold(milestones[left].Title, milestones[right].Title) {
			return milestones[left].ID < milestones[right].ID
		}
		return strings.ToLower(milestones[left].Title) < strings.ToLower(milestones[right].Title)
	})
	milestoneChoices := []string{"No milestone", "Create new milestone..."}
	milestonesByChoice := make(map[string]gitlab.Milestone, len(milestones))
	for _, milestone := range milestones {
		choice := fmt.Sprintf("%s [milestone %d]", milestone.Title, milestone.ID)
		milestoneChoices = append(milestoneChoices, choice)
		milestonesByChoice[choice] = milestone
	}
	milestoneChoice, selected, err := picker.Choose(ctx, milestoneChoices, ui.Options{Prompt: "Milestone", BorderLabel: "issue milestone"})
	if err != nil {
		fmt.Fprintf(stderr, "Cannot select an issue milestone: %v\n", err)
		return 1
	}
	if !selected {
		fmt.Fprintln(stdout, "Issue creation cancelled.")
		return 0
	}
	var milestoneID *int64
	milestoneName := "none"
	plannedMilestoneTitle := ""
	plannedMilestoneDueDate := ""
	if milestoneChoice == "Create new milestone..." {
		fmt.Fprint(stdout, "New milestone title: ")
		plannedMilestoneTitle, err = input.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(stderr, "Cannot read the new milestone title: %v\n", err)
			return 1
		}
		plannedMilestoneTitle = strings.TrimSpace(plannedMilestoneTitle)
		if plannedMilestoneTitle == "" || strings.ContainsAny(plannedMilestoneTitle, "\r\n") {
			fmt.Fprintln(stderr, "A new milestone requires a non-empty single-line title.")
			return 1
		}
		fmt.Fprint(stdout, "Due date [YYYY-MM-DD, empty for none]: ")
		plannedMilestoneDueDate, err = input.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(stderr, "Cannot read the new milestone due date: %v\n", err)
			return 1
		}
		plannedMilestoneDueDate = strings.TrimSpace(plannedMilestoneDueDate)
		if plannedMilestoneDueDate != "" {
			if _, err := time.Parse("2006-01-02", plannedMilestoneDueDate); err != nil {
				fmt.Fprintln(stderr, "Milestone due date must use YYYY-MM-DD format.")
				return 1
			}
		}
		milestoneName = plannedMilestoneTitle
	} else if milestoneChoice != "No milestone" {
		milestone := milestonesByChoice[milestoneChoice]
		milestoneID = &milestone.ID
		milestoneName = milestone.Title
	}

	fmt.Fprintln(stdout, "Opening the issue description template in your editor...")
	description, err := ui.EditText(ctx, manualDescriptionTemplate, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot edit the issue description: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Create issue %q?\n", title)
	fmt.Fprintf(stdout, "Labels: %s\n", listOrNone(chosenLabels))
	fmt.Fprintf(stdout, "Assignee: %s\n", assigneeName)
	fmt.Fprintf(stdout, "Milestone: %s\n", milestoneName)
	confirmed, err := readConfirmation(input, stdout, "Create this GitLab issue? [y/N] ")
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read issue creation confirmation: %v\n", err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(stdout, "Issue creation cancelled. No changes were applied.")
		return 0
	}
	for _, name := range chosenLabels {
		label, planned := plannedLabels[strings.ToLower(name)]
		if !planned {
			continue
		}
		if _, err := client.CreateLabelWithColor(ctx, project.ID, label.Name, label.Color); err != nil {
			fmt.Fprintf(stderr, "Cannot create required GitLab label %q; the issue was not created: %v\n", label.Name, err)
			return 1
		}
	}
	if plannedMilestoneTitle != "" {
		milestone, err := client.CreateManualMilestone(ctx, project.ID, plannedMilestoneTitle, plannedMilestoneDueDate)
		if err != nil {
			fmt.Fprintf(stderr, "Cannot create the required GitLab milestone; the issue was not created: %v\n", err)
			return 1
		}
		milestoneID = &milestone.ID
	}
	created, err := client.CreateManualIssue(ctx, project.ID, title, description, chosenLabels, milestoneID, assigneeID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot create the GitLab issue: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Created GitLab issue #%d.\n", created.IID)

	manageBranch, err := readConfirmation(input, stdout, "Create or check out a branch for this issue now? [y/N] ")
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read branch confirmation: %v\n", err)
		return 1
	}
	if !manageBranch {
		return 0
	}
	gitClient := gitrepo.NewClient()
	if err := gitClient.PruneRemoteBranches(ctx); err != nil {
		fmt.Fprintf(stderr, "Issue #%d was created, but remote branches could not be refreshed: %v\n", created.IID, err)
		return 1
	}
	branches, err := gitClient.ListBranches(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "Issue #%d was created, but branches could not be read: %v\n", created.IID, err)
		return 1
	}
	candidate := workitem.Candidate{Kind: workitem.Issue, IID: created.IID, Title: title}
	return manageWorkItemBranch(ctx, picker, gitClient, candidate, branches, project.DefaultBranch, input, stdout, stderr)
}

func validHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		if character >= '0' && character <= '9' || character >= 'a' && character <= 'f' || character >= 'A' && character <= 'F' {
			continue
		}
		return false
	}
	return true
}

func exportProjectSnapshot(ctx context.Context, client *gitlab.Client, project gitlab.Project, stdout, stderr io.Writer) int {
	issues, err := client.ListIssues(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot export the GitLab snapshot because issues could not be read completely: %v\n", err)
		return 1
	}
	milestones, err := client.ListMilestones(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot export the GitLab snapshot because milestones could not be read completely: %v\n", err)
		return 1
	}
	labels, err := client.ListLabels(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot export the GitLab snapshot because labels could not be read completely: %v\n", err)
		return 1
	}
	directory, err := projectbackup.Write(".glab-helper-snapshots", currentTime(), project.ID, project.Path, "manual", issues, milestones, labels)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot write the GitLab snapshot: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Exported GitLab snapshot to %s.\n", directory)
	return 0
}
