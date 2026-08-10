package syncplan

import (
	"fmt"
	"io"
	"strings"
)

func WritePreview(writer io.Writer, projectPath string, plan Plan) {
	fmt.Fprintf(writer, "YouTrack synchronization preview for %s (read-only)\n", projectPath)
	if len(plan.Actions) == 0 {
		fmt.Fprintln(writer, "No GitLab changes are needed.")
	}
	creates, updates := 0, 0
	for _, action := range plan.Actions {
		switch action.Operation {
		case Create:
			creates++
		case Update:
			updates++
		}
		writeAction(writer, action)
	}
	fmt.Fprintf(writer, "Summary: %d to create, %d to update, %d unchanged, %d ignored source items.\n", creates, updates, plan.Unchanged, plan.Ignored)
	fmt.Fprintln(writer, "No GitLab or YouTrack changes were applied.")
}

func writeAction(writer io.Writer, action Action) {
	if action.Desired.Target == Label {
		fmt.Fprintf(writer, "CREATE label %q\n", action.Desired.Title)
		return
	}
	reference := action.Desired.SourceID
	if action.Operation == Update {
		if action.Desired.Target == Milestone {
			reference = action.CurrentID
		} else {
			reference = fmt.Sprintf("#%d", action.CurrentIID)
		}
	}
	fmt.Fprintf(writer, "%s %s %s from YouTrack %s: %q", strings.ToUpper(string(action.Operation)), action.Desired.Target, reference, action.Desired.SourceID, action.Desired.Title)
	if action.Desired.ParentSourceID != "" {
		fmt.Fprintf(writer, " under %s", action.Desired.ParentSourceID)
	}
	if len(action.Desired.Labels) > 0 && action.Operation == Create {
		fmt.Fprintf(writer, " [labels: %s]", strings.Join(action.Desired.Labels, ", "))
	}
	if action.Desired.Resolved && action.Operation == Create {
		fmt.Fprint(writer, " [state: closed]")
	}
	fprintln(writer)
	for _, change := range action.Changes {
		if change.Field == "description" {
			fmt.Fprintln(writer, "  description: changed")
			continue
		}
		fmt.Fprintf(writer, "  %s: %q -> %q\n", change.Field, displayEmpty(change.From), displayEmpty(change.To))
	}
}

func fprintln(writer io.Writer) {
	fmt.Fprintln(writer)
}

func displayEmpty(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
