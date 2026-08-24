package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
)

const (
	actionPreviewReset = "Preview full project reset"
	actionResetProject = "Reset all issues and milestones"
)

type resetPlan struct {
	Issues     []gitlab.Issue
	Milestones []gitlab.Milestone
}

func runMaintenanceReset(ctx context.Context, client *gitlab.Client, project gitlab.Project, dryRun bool, stdin io.Reader, stdout, stderr io.Writer) int {
	plan, err := readResetPlan(ctx, client, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot build the complete GitLab deletion plan: %v\n", err)
		return 1
	}
	writeResetPreview(stdout, project.Path, plan)
	if len(plan.Issues) == 0 && len(plan.Milestones) == 0 {
		return 0
	}
	if dryRun {
		fmt.Fprintln(stdout, "Dry-run complete; no GitLab changes or local snapshots were written.")
		return 0
	}

	expected := "RESET ALL " + project.Path
	fmt.Fprintf(stdout, "This permanently deletes GitLab issues and their discussions.\nType %q to continue: ", expected)
	confirmation, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(stderr, "Cannot read reset confirmation: %v\n", err)
		return 1
	}
	if trimLineEnding(confirmation) != expected {
		fmt.Fprintln(stdout, "Reset cancelled. Nothing was deleted.")
		return 0
	}

	labels, err := client.ListLabels(ctx, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Reset aborted because labels for the mandatory snapshot could not be read completely: %v\n", err)
		return 1
	}
	snapshotDirectory, err := writeProjectBackup(
		".glab-helper-snapshots",
		currentTime(),
		project.ID,
		project.Path,
		"pre-reset-all-issues-milestones",
		plan.Issues,
		plan.Milestones,
		labels,
	)
	if err != nil {
		fmt.Fprintf(stderr, "Reset aborted because the mandatory pre-reset snapshot failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Mandatory pre-reset snapshot written to %s.\n", snapshotDirectory)

	current, err := readResetPlan(ctx, client, project.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Reset aborted because the deletion plan could not be revalidated: %v\n", err)
		return 1
	}
	if !sameResetPlan(plan, current) {
		fmt.Fprintln(stderr, "Reset aborted because the deletion plan changed after confirmation.")
		fmt.Fprintln(stderr, "Review a fresh --maintenance --dry-run preview before trying again.")
		return 1
	}

	deletedIssues, failedIssues := 0, 0
	for _, issue := range plan.Issues {
		if err := client.DeleteIssue(ctx, project.ID, issue.IID); err != nil {
			failedIssues++
			fmt.Fprintf(stderr, "Failed to delete issue #%d: %v\n", issue.IID, err)
			continue
		}
		deletedIssues++
	}
	if failedIssues > 0 {
		fmt.Fprintf(stderr, "Milestone deletion skipped because %d issue deletion(s) failed. %d issues were deleted.\n", failedIssues, deletedIssues)
		return 1
	}

	deletedMilestones, failedMilestones := 0, 0
	for _, milestone := range plan.Milestones {
		if err := client.DeleteMilestone(ctx, project.ID, milestone.ID); err != nil {
			failedMilestones++
			fmt.Fprintf(stderr, "Failed to delete milestone %d: %v\n", milestone.ID, err)
			continue
		}
		deletedMilestones++
	}
	if failedMilestones > 0 {
		fmt.Fprintf(stderr, "Reset incomplete: %d issues and %d milestones deleted; %d milestone deletion(s) failed.\n", deletedIssues, deletedMilestones, failedMilestones)
		return 1
	}

	fmt.Fprintf(stdout, "Reset complete: %d issues and %d milestones deleted.\n", deletedIssues, deletedMilestones)
	fmt.Fprintln(stdout, "Branches, labels, and merge requests were preserved.")
	return 0
}

func readResetPlan(ctx context.Context, client *gitlab.Client, projectID int64) (resetPlan, error) {
	issues, err := client.ListIssues(ctx, projectID)
	if err != nil {
		return resetPlan{}, fmt.Errorf("read all GitLab issues: %w", err)
	}
	milestones, err := client.ListMilestones(ctx, projectID)
	if err != nil {
		return resetPlan{}, fmt.Errorf("read all GitLab milestones: %w", err)
	}
	return canonicalResetPlan(resetPlan{Issues: issues, Milestones: milestones}), nil
}

func writeResetPreview(output io.Writer, projectPath string, plan resetPlan) {
	fmt.Fprintf(output, "Full project reset preview for %s\n", projectPath)
	fmt.Fprintf(output, "%d issues will be permanently deleted:\n", len(plan.Issues))
	for _, issue := range plan.Issues {
		fmt.Fprintf(output, "  DELETE issue #%d [%s] %s\n", issue.IID, issue.State, issue.Title)
	}
	fmt.Fprintf(output, "%d milestones will be permanently deleted:\n", len(plan.Milestones))
	for _, milestone := range plan.Milestones {
		fmt.Fprintf(output, "  DELETE milestone %d [%s] %s\n", milestone.ID, milestone.State, milestone.Title)
	}
	fmt.Fprintln(output, "Branches, labels, and merge requests will be preserved.")
}

func sameResetPlan(left, right resetPlan) bool {
	return reflect.DeepEqual(canonicalResetPlan(left), canonicalResetPlan(right))
}

func canonicalResetPlan(plan resetPlan) resetPlan {
	result := resetPlan{
		Issues:     append([]gitlab.Issue(nil), plan.Issues...),
		Milestones: append([]gitlab.Milestone(nil), plan.Milestones...),
	}
	for index := range result.Issues {
		result.Issues[index].Labels = append([]string(nil), result.Issues[index].Labels...)
		result.Issues[index].Assignees = append([]string(nil), result.Issues[index].Assignees...)
		sort.Strings(result.Issues[index].Labels)
		sort.Strings(result.Issues[index].Assignees)
	}
	sort.Slice(result.Issues, func(left, right int) bool { return result.Issues[left].IID < result.Issues[right].IID })
	sort.Slice(result.Milestones, func(left, right int) bool { return result.Milestones[left].ID < result.Milestones[right].ID })
	return result
}

func trimLineEnding(value string) string {
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r') {
		value = value[:len(value)-1]
	}
	return value
}
