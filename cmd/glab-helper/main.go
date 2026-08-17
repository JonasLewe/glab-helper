package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	gitrepo "github.com/JonasLewe/glab-helper/internal/git"
	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/syncapply"
	"github.com/JonasLewe/glab-helper/internal/syncplan"
	"github.com/JonasLewe/glab-helper/internal/ui"
	"github.com/JonasLewe/glab-helper/internal/workitem"
	"github.com/JonasLewe/glab-helper/internal/youtrack"
)

const version = "0.1.0"
const usage = "Usage: glab-helper [--dev | --maintenance] [--dry-run] [--version]"

const (
	actionSync = "Sync YouTrack"
	actionWork = "Work on existing issue or task"
	actionExit = "Exit"

	workActionBranch = "Branch (checkout / create)"
	workActionDone   = "Done"
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

	ctx := context.Background()
	client := gitlab.NewClient()
	project, err := client.CurrentProject(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot detect the current GitLab project: %v\n", err)
		return 1
	}
	projectConfigPath := os.Getenv("GLAB_HELPER_CONFIG")
	projectConfig, projectConfigErr := projectconfig.Load(projectConfigPath)
	if projectConfigErr != nil && (!errors.Is(projectConfigErr, projectconfig.ErrNotFound) || projectConfigPath != "") {
		fmt.Fprintf(stderr, "Cannot load the project configuration: %v\n", projectConfigErr)
		return 1
	}
	projectConfigAvailable := projectConfigErr == nil
	youTrackConfig := youtrack.Config{}
	youTrackAvailable := false
	if projectConfigAvailable {
		youTrackConfig, err = youtrack.LoadConfig(ctx, client, project.Path, os.Getenv("GLAB_HELPER_YOUTRACK_PROJECT_PATH"), dev)
		youTrackAvailable = err == nil
	}
	if maintenance && !youTrackAvailable {
		fmt.Fprintln(stderr, "Maintenance mode requires the configured YouTrack target project.")
		return 1
	}
	if dryRun && !youTrackAvailable {
		fmt.Fprintln(stderr, "YouTrack synchronization preview requires a valid project configuration and YouTrack access.")
		return 1
	}
	if dryRun {
		return runSynchronization(ctx, client, project.ID, project.Path, youTrackConfig, projectConfig, true, stdin, stdout, stderr)
	}
	if !maintenance {
		actions := []string{actionWork, actionExit}
		if youTrackAvailable {
			actions = append([]string{actionSync}, actions...)
		}
		picker := ui.NewPicker()
		action, selected, err := picker.Choose(ctx, actions, ui.Options{Prompt: "Action", BorderLabel: "action"})
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
			return runSynchronization(ctx, client, project.ID, project.Path, youTrackConfig, projectConfig, false, stdin, stdout, stderr)
		case actionWork:
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
			return selectWorkItem(ctx, picker, gitClient, issues, tasks, branches, project.DefaultBranch, stdin, stdout, stderr)
		case actionExit:
			fmt.Fprintln(stdout, "Done.")
			return 0
		}
	}
	sourceSnapshot, err := readYouTrackSnapshot(ctx, youTrackConfig, projectConfig)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the complete YouTrack source snapshot: %v\n", err)
		return 1
	}
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
	milestones, err := client.ListMilestones(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab milestones for %s: %v\n", project.Path, err)
		return 1
	}
	labels, err := client.ListLabels(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab labels for %s: %v\n", project.Path, err)
		return 1
	}
	branches, err := gitrepo.NewClient().ListRemoteBranches(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read current remote branches for %s: %v\n", project.Path, err)
		return 1
	}

	fmt.Fprintf(stderr, "Interactive workflow for %s is not migrated yet; read %d GitLab issues, %d tasks, %d milestones, %d labels, and %d remote branches without changes; read %d YouTrack work items into memory.\n", project.Path, len(issues), len(tasks), len(milestones), len(labels), len(branches), len(sourceSnapshot.WorkItems))
	return 2
}

func runSynchronization(
	ctx context.Context,
	client *gitlab.Client,
	projectID int64,
	projectPath string,
	youTrackConfig youtrack.Config,
	projectConfig projectconfig.Config,
	dryRun bool,
	stdin io.Reader,
	stdout, stderr io.Writer,
) int {
	sourceSnapshot, err := readYouTrackSnapshot(ctx, youTrackConfig, projectConfig)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the complete YouTrack source snapshot: %v\n", err)
		return 1
	}
	issues, err := client.ListIssues(ctx, projectID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab issues for %s: %v\n", projectPath, err)
		return 1
	}
	tasks, err := client.ListTasks(ctx, projectPath)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab tasks for %s: %v\n", projectPath, err)
		return 1
	}
	milestones, err := client.ListMilestones(ctx, projectID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab milestones for %s: %v\n", projectPath, err)
		return 1
	}
	labels, err := client.ListLabels(ctx, projectID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab labels for %s: %v\n", projectPath, err)
		return 1
	}
	plan, err := syncplan.Build(sourceSnapshot, projectConfig, syncplan.Current{
		Milestones: milestones,
		Issues:     issues,
		Tasks:      tasks,
		Labels:     labels,
	})
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

func selectWorkItem(
	ctx context.Context,
	picker *ui.Picker,
	gitClient *gitrepo.Client,
	issues []gitlab.Issue,
	tasks []gitlab.Task,
	branches []gitrepo.Branch,
	defaultBranch string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) int {
	candidates := workitem.OpenCandidates(issues, tasks, branches)
	if len(candidates) == 0 {
		fmt.Fprintln(stdout, "No open GitLab issues or tasks found.")
		return 0
	}
	choices := make([]string, len(candidates))
	for index, candidate := range candidates {
		choices[index] = candidate.Display()
	}
	choice, selected, err := picker.Choose(ctx, choices, ui.Options{Prompt: "Issue or task", BorderLabel: "open issues and tasks"})
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
		workAction, selected, err := picker.Choose(ctx, []string{workActionBranch, workActionDone}, ui.Options{Prompt: "Work item action", BorderLabel: fmt.Sprintf("%s #%d", candidate.Kind, candidate.IID)})
		if err != nil {
			fmt.Fprintf(stderr, "Cannot select a work item action: %v\n", err)
			return 1
		}
		if !selected || workAction == workActionDone {
			fmt.Fprintln(stdout, "Done. No changes were applied.")
			return 0
		}
		return manageWorkItemBranch(ctx, picker, gitClient, candidate, branches, defaultBranch, bufio.NewReader(stdin), stdout, stderr)
	}
	fmt.Fprintln(stderr, "Cannot match the selected GitLab issue or task.")
	return 1
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

func readConfirmation(input *bufio.Reader, output io.Writer, prompt string) (bool, error) {
	fmt.Fprint(output, prompt)
	answer, err := input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.TrimSpace(answer)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}
