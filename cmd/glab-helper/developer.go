package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/projectbackup"
	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/syncapply"
	"github.com/JonasLewe/glab-helper/internal/syncplan"
	"github.com/JonasLewe/glab-helper/internal/ui"
	"github.com/JonasLewe/glab-helper/internal/youtrack"
)

const (
	actionSyncMilestones    = "Sync YouTrack milestones only"
	actionSyncAll           = "Sync all YouTrack work items"
	actionPreviewMilestones = "Preview YouTrack milestones only"
	actionPreviewAll        = "Preview all YouTrack work items"
	actionCreateIssue       = "Create issue"
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
var writeProjectBackup = projectbackup.Write

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
		mode, selected, err := picker.Choose(ctx, []string{createIssueFromYouTrack, createIssueManually}, ui.Options{Prompt: "Create issue", BorderLabel: "create GitLab issue", Accent: ui.Magenta})
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
	current, err := readSynchronizationState(ctx, client, project.ID, project.Path)
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
	choice, selected, err := picker.Choose(ctx, choices, ui.Options{Prompt: "YouTrack issue", BorderLabel: "unsynchronized YouTrack issues", Accent: ui.Magenta})
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
	directory, err := writeProjectBackup(".glab-helper-snapshots", currentTime(), project.ID, project.Path, "manual", issues, milestones, labels)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot write the GitLab snapshot: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Exported GitLab snapshot to %s.\n", directory)
	return 0
}
