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
	Branch    gitrepo.Branch
}

func OpenCandidates(issues []gitlab.Issue, tasks []gitlab.Task, branches []gitrepo.Branch) []Candidate {
	branchByIID := make(map[int64]gitrepo.Branch)
	for _, branch := range branches {
		separator := strings.IndexByte(branch.Name, '-')
		if separator < 1 {
			continue
		}
		iid, err := strconv.ParseInt(branch.Name[:separator], 10, 64)
		if err != nil || iid < 1 || strconv.FormatInt(iid, 10) != branch.Name[:separator] {
			continue
		}
		if _, exists := branchByIID[iid]; !exists {
			branchByIID[iid] = branch
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
	if candidate.Branch.Name != "" {
		result += "  [branch: " + candidate.Branch.Name + "]"
	}
	return result
}

func BranchName(iid int64, title string) string {
	var slug strings.Builder
	previousDash := false
	for _, character := range strings.ToLower(title) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			slug.WriteRune(character)
			previousDash = false
			continue
		}
		if slug.Len() > 0 && !previousDash {
			slug.WriteByte('-')
			previousDash = true
		}
	}
	value := strings.Trim(slug.String(), "-")
	if value == "" {
		value = "issue"
	}
	name := strconv.FormatInt(iid, 10) + "-" + value
	if len(name) <= 60 {
		return name
	}
	name = name[:60]
	prefixLength := len(strconv.FormatInt(iid, 10)) + 1
	if separator := strings.LastIndexByte(name, '-'); separator >= prefixLength {
		name = name[:separator]
	}
	return name
}
