package main

import (
	"context"
	"fmt"
	"io"
	"os"

	gitrepo "github.com/JonasLewe/glab-helper/internal/git"
	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/youtrack"
)

const version = "0.1.0"
const usage = "Usage: glab-helper [--dev | --maintenance] [--dry-run] [--version]"

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
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var dev, maintenance, showHelp, showVersion bool

	for _, arg := range args {
		switch arg {
		case "--dev", "-d":
			dev = true
		case "--dry-run":
			continue
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
	youTrackConfig, youTrackConfigErr := youtrack.LoadConfig(ctx, client, project.Path, os.Getenv("GLAB_HELPER_YOUTRACK_PROJECT_PATH"), dev)
	youTrackAvailable := youTrackConfigErr == nil
	if maintenance && !youTrackAvailable {
		fmt.Fprintln(stderr, "Maintenance mode requires the configured YouTrack target project.")
		return 1
	}
	youTrackItemCount := 0
	if youTrackAvailable {
		snapshot, err := readYouTrackSnapshot(ctx, youTrackConfig)
		if err != nil {
			fmt.Fprintf(stderr, "Cannot read the complete YouTrack source snapshot: %v\n", err)
			return 1
		}
		youTrackItemCount = len(snapshot.WorkItems)
	}

	issues, err := client.ListIssues(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read all GitLab issues for %s: %v\n", project.Path, err)
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

	youTrackStatus := "YouTrack configuration is unavailable"
	if youTrackAvailable {
		youTrackStatus = fmt.Sprintf("read %d YouTrack work items into memory", youTrackItemCount)
	}
	fmt.Fprintf(stderr, "Interactive workflow for %s is not migrated yet; read %d GitLab issues, %d milestones, %d labels, and %d remote branches without changes; %s.\n", project.Path, len(issues), len(milestones), len(labels), len(branches), youTrackStatus)
	return 2
}
