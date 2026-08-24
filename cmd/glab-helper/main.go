package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/syncapply"
	"github.com/JonasLewe/glab-helper/internal/syncplan"
	"github.com/JonasLewe/glab-helper/internal/ui"
	"github.com/JonasLewe/glab-helper/internal/youtrack"
)

const version = "0.1.0"
const usage = "Usage: glab-helper [--dev | --maintenance] [--dry-run] [--version]"

const (
	actionSync = "Sync YouTrack"
	actionWork = "Work on existing issue"
	actionExit = "Exit"
)

var readYouTrackSnapshot = youtrack.ReadSnapshot

const help = `
  glab-helper — Interactive GitLab workflow helper

  ` + usage + `

  Options:
    --dev, -d   Show developer actions and skip the YouTrack target-project check
    --maintenance  Show destructive project maintenance actions
    --dry-run   Read-only mode; preview YouTrack syncs without any writes
    --version   Show version and exit

  Run from any cloned GitLab repo. Requires: glab, fzf, jq
  Authenticate first: glab auth login

`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var dev, maintenance, dryRun, showHelp, showVersion bool

	for _, arg := range args {
		switch arg {
		case "--dev", "-d":
			dev = true
		case "--dry-run":
			dryRun = true
		case "--maintenance":
			maintenance = true
		case "--help", "-h":
			showHelp = true
		case "--version":
			showVersion = true
		default:
			fmt.Fprintf(stderr, "Unknown argument: %s\n", arg)
			fmt.Fprintln(stderr, usage)
			return 2
		}
	}

	if dev && maintenance {
		fmt.Fprintln(stderr, "--dev and --maintenance cannot be combined.")
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "glab-helper %s\n", version)
		return 0
	}
	if showHelp {
		fmt.Fprint(stdout, help)
		return 0
	}
	terminal := ui.DetectTerminal(stdout)
	terminal.Clear(stdout)
	terminal.WriteStatus(stdout, "Connecting to GitLab...")

	ctx := context.Background()
	client := gitlab.NewClient()
	project, err := client.CurrentProject(ctx)
	terminal.ClearStatus(stdout)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot detect the current GitLab project: %v\n", err)
		return 1
	}
	terminal.WriteHeader(stdout, project.Path)
	terminal.WriteStatus(stdout, "Checking YouTrack integration...")
	projectConfigPath := os.Getenv("GLAB_HELPER_CONFIG")
	projectConfig, projectConfigErr := projectconfig.Load(projectConfigPath)
	if projectConfigErr != nil && (!errors.Is(projectConfigErr, projectconfig.ErrNotFound) || projectConfigPath != "") {
		terminal.ClearStatus(stdout)
		fmt.Fprintf(stderr, "Cannot load the project configuration: %v\n", projectConfigErr)
		return 1
	}
	projectConfigAvailable := projectConfigErr == nil
	youTrackConfig := youtrack.Config{}
	youTrackAvailable := false
	var youTrackConfigErr error
	if projectConfigAvailable {
		youTrackConfig, youTrackConfigErr = youtrack.LoadConfig(ctx, client, project.Path, os.Getenv("GLAB_HELPER_YOUTRACK_PROJECT_PATH"), dev)
		youTrackAvailable = youTrackConfigErr == nil
	}
	terminal.ClearStatus(stdout)
	if youTrackAvailable {
		terminal.WriteIntegrationAvailable(stdout, "YouTrack")
	}
	if maintenance && !youTrackAvailable {
		if !projectConfigAvailable {
			fmt.Fprintf(stderr, "Maintenance mode requires the project configuration: %v\n", projectConfigErr)
		} else {
			fmt.Fprintf(stderr, "Maintenance mode requires the configured YouTrack target project: %v\n", youTrackConfigErr)
		}
		return 1
	}
	picker := ui.NewPicker(terminal.ColorEnabled())
	if maintenance {
		action := actionResetProject
		if dryRun {
			action = actionPreviewReset
		}
		selectedAction, selected, err := chooseMainAction(ctx, picker, []string{action, actionExit})
		if err != nil {
			fmt.Fprintf(stderr, "Cannot select a maintenance action: %v\n", err)
			return 1
		}
		if !selected || selectedAction == actionExit {
			fmt.Fprintln(stdout, "Done.")
			return 0
		}
		return runMaintenanceReset(ctx, client, project, dryRun, stdin, stdout, stderr)
	}
	if dryRun && !youTrackAvailable {
		if !projectConfigAvailable {
			fmt.Fprintf(stderr, "YouTrack synchronization preview requires the project configuration: %v\n", projectConfigErr)
		} else {
			fmt.Fprintf(stderr, "Cannot load the YouTrack configuration for synchronization preview: %v\n", youTrackConfigErr)
		}
		return 1
	}
	if dryRun {
		if !dev {
			return runSynchronization(ctx, client, project.ID, project.Path, youTrackConfig, projectConfig, syncAll, true, stdin, stdout, stderr)
		}
		actions := []string{actionPreviewAll, actionExit}
		if projectHasTarget(projectConfig, "milestone") {
			actions = append([]string{actionPreviewMilestones}, actions...)
		}
		action, selected, err := chooseMainAction(ctx, picker, actions)
		if err != nil {
			fmt.Fprintf(stderr, "Cannot select a developer preview: %v\n", err)
			return 1
		}
		if !selected || action == actionExit {
			fmt.Fprintln(stdout, "Done.")
			return 0
		}
		if action == actionPreviewMilestones {
			return runSynchronization(ctx, client, project.ID, project.Path, youTrackConfig, projectConfig, syncMilestones, true, stdin, stdout, stderr)
		}
		return runSynchronization(ctx, client, project.ID, project.Path, youTrackConfig, projectConfig, syncAll, true, stdin, stdout, stderr)
	}
	actions := availableMainActions(dev, youTrackAvailable, projectHasTarget(projectConfig, "milestone"))
	action, selected, err := chooseMainAction(ctx, picker, actions)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot select an action: %v\n", err)
		return 1
	}
	if !selected {
		fmt.Fprintln(stdout, "Aborted.")
		return 0
	}
	switch action {
	case actionSync:
		return runSynchronization(ctx, client, project.ID, project.Path, youTrackConfig, projectConfig, syncAll, false, stdin, stdout, stderr)
	case actionSyncMilestones:
		return runSynchronization(ctx, client, project.ID, project.Path, youTrackConfig, projectConfig, syncMilestones, false, stdin, stdout, stderr)
	case actionSyncAll:
		return runSynchronization(ctx, client, project.ID, project.Path, youTrackConfig, projectConfig, syncAll, false, stdin, stdout, stderr)
	case actionCreateIssue:
		return createIssueWorkflow(ctx, picker, client, project, youTrackAvailable, youTrackConfig, projectConfig, stdin, stdout, stderr)
	case actionExportSnapshot:
		return exportProjectSnapshot(ctx, client, project, stdout, stderr)
	case actionWork:
		return runWorkItemWorkflow(ctx, picker, client, project, stdin, stdout, stderr)
	case actionExit:
		fmt.Fprintln(stdout, "Done.")
		return 0
	}
	fmt.Fprintln(stderr, "Cannot match the selected action.")
	return 2
}

func chooseMainAction(ctx context.Context, picker *ui.Picker, actions []string) (string, bool, error) {
	choices := make([]string, len(actions))
	for index, action := range actions {
		choices[index] = mainActionDisplay(action)
	}
	choice, selected, err := picker.Choose(ctx, choices, ui.Options{
		Prompt:      "What do you want to do?",
		BorderLabel: "action",
		Header:      "ENTER=select",
		Accent:      ui.Cyan,
	})
	if err != nil || !selected {
		return "", selected, err
	}
	for index, rendered := range choices {
		if choice == rendered {
			return actions[index], true, nil
		}
	}
	return "", false, fmt.Errorf("cannot match the selected main action %q", choice)
}

func mainActionDisplay(action string) string {
	switch action {
	case actionSync, actionSyncMilestones, actionSyncAll, actionPreviewMilestones, actionPreviewAll:
		return "~ " + action
	case actionCreateIssue:
		return "+ " + action
	case actionWork, actionExportSnapshot:
		return "▸ " + action
	case actionPreviewReset, actionResetProject:
		return "! " + action
	case actionExit:
		return "× " + action
	default:
		return action
	}
}

func availableMainActions(dev, youTrackAvailable, milestoneTarget bool) []string {
	if !dev {
		if youTrackAvailable {
			return []string{actionSync, actionExit}
		}
		return []string{actionExit}
	}
	actions := []string{actionCreateIssue, actionWork, actionExportSnapshot, actionExit}
	if !youTrackAvailable {
		return actions
	}
	syncActions := []string{actionSyncAll}
	if milestoneTarget {
		syncActions = append([]string{actionSyncMilestones}, syncActions...)
	}
	return append(syncActions, actions...)
}

func runSynchronization(
	ctx context.Context,
	client *gitlab.Client,
	projectID int64,
	projectPath string,
	youTrackConfig youtrack.Config,
	projectConfig projectconfig.Config,
	scope synchronizationScope,
	dryRun bool,
	stdin io.Reader,
	stdout, stderr io.Writer,
) int {
	sourceSnapshot, err := readYouTrackSnapshot(ctx, youTrackConfig, projectConfig)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the complete YouTrack source snapshot: %v\n", err)
		return 1
	}
	syncConfig, err := projectConfigForScope(projectConfig, scope)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot prepare the selected YouTrack synchronization scope: %v\n", err)
		return 1
	}
	current, err := readSynchronizationState(ctx, client, projectID, projectPath)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the complete GitLab synchronization state for %s: %v\n", projectPath, err)
		return 1
	}
	plan, err := syncplan.Build(sourceSnapshot, syncConfig, current)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot build a safe YouTrack synchronization preview: %v\n", err)
		return 1
	}
	syncplan.WritePreview(stdout, projectPath, plan)
	if dryRun || len(plan.Actions) == 0 {
		return 0
	}
	fmt.Fprint(stdout, "Apply this synchronization plan? [y/N] ")
	answer, readErr := bufio.NewReader(stdin).ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		fmt.Fprintf(stderr, "Cannot read synchronization confirmation: %v\n", readErr)
		return 1
	}
	answer = strings.TrimSpace(answer)
	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		fmt.Fprintln(stdout, "Synchronization cancelled. No changes were applied.")
		return 0
	}
	result, err := syncapply.Apply(ctx, client, projectID, projectPath, plan)
	if err != nil {
		fmt.Fprintf(stderr, "Synchronization stopped after %d of %d completed actions: %v\n", result.Applied, result.Total, err)
		fmt.Fprintln(stderr, "The current action may be partially applied; run --dry-run again before retrying.")
		return 1
	}
	fmt.Fprintf(stdout, "Applied %d GitLab synchronization actions. YouTrack remained read-only.\n", result.Applied)
	return 0
}

func readSynchronizationState(ctx context.Context, client *gitlab.Client, projectID int64, projectPath string) (syncplan.Current, error) {
	issues, err := client.ListIssues(ctx, projectID)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab issues: %w", err)
	}
	tasks, err := client.ListTasks(ctx, projectPath)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab tasks: %w", err)
	}
	milestones, err := client.ListMilestones(ctx, projectID)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab milestones: %w", err)
	}
	labels, err := client.ListLabels(ctx, projectID)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab labels: %w", err)
	}
	boards, err := client.ListBoards(ctx, projectID)
	if err != nil {
		return syncplan.Current{}, fmt.Errorf("read GitLab issue boards: %w", err)
	}
	return syncplan.Current{Issues: issues, Tasks: tasks, Milestones: milestones, Labels: labels, Boards: boards}, nil
}
