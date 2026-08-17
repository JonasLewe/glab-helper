package workitem

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	gitrepo "github.com/JonasLewe/glab-helper/internal/git"
	"github.com/JonasLewe/glab-helper/internal/gitlab"
)

type Kind string

const (
	Issue Kind = "issue"
	Task  Kind = "task"
)

type Candidate struct {
	Kind      Kind
	IID       int64
	Title     string
	ParentIID int64
	Branch    string
}

func OpenCandidates(issues []gitlab.Issue, tasks []gitlab.Task, branches []gitrepo.RemoteBranch) []Candidate {
	branchByIID := make(map[int64]string)
	for _, branch := range branches {
		separator := strings.IndexByte(branch.Name, '-')
		if separator < 1 {
			continue
		}
		iid, err := strconv.ParseInt(branch.Name[:separator], 10, 64)
		if err != nil || iid < 1 {
			continue
		}
		if _, exists := branchByIID[iid]; !exists {
			branchByIID[iid] = branch.Name
		}
	}

	candidates := make([]Candidate, 0, len(issues)+len(tasks))
	for _, issue := range issues {
		if !strings.EqualFold(issue.State, "opened") {
			continue
		}
		candidates = append(candidates, Candidate{Kind: Issue, IID: issue.IID, Title: issue.Title, Branch: branchByIID[issue.IID]})
	}
	for _, task := range tasks {
		if !strings.EqualFold(task.State, "open") {
			continue
		}
		candidates = append(candidates, Candidate{Kind: Task, IID: task.IID, Title: task.Title, ParentIID: task.ParentIID, Branch: branchByIID[task.IID]})
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].IID > candidates[right].IID
	})
	return candidates
}

func (candidate Candidate) Display() string {
	title := strings.Join(strings.Fields(candidate.Title), " ")
	result := fmt.Sprintf("%-5s  #%d  %s", strings.ToUpper(string(candidate.Kind[:1]))+string(candidate.Kind[1:]), candidate.IID, title)
	if candidate.Kind == Task && candidate.ParentIID > 0 {
		result += fmt.Sprintf("  [parent: #%d]", candidate.ParentIID)
	}
	if candidate.Branch != "" {
		result += "  [branch: " + candidate.Branch + "]"
	}
	return result
}
