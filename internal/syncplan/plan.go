package syncplan

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/source"
)

type Operation string

const (
	Create Operation = "create"
	Update Operation = "update"
)

type Target string

const (
	Label     Target = "label"
	Milestone Target = "milestone"
	Issue     Target = "issue"
	Task      Target = "task"
)

type DesiredItem struct {
	SourceID       string
	Target         Target
	Title          string
	Description    string
	Labels         []string
	ParentSourceID string
	ParentTitle    string
	Resolved       bool
}

type Change struct {
	Field string
	From  string
	To    string
}

type Action struct {
	Operation  Operation
	Desired    DesiredItem
	CurrentID  string
	CurrentIID int64
	Changes    []Change
}

type Plan struct {
	Actions   []Action
	Unchanged int
	Ignored   int
}

type Current struct {
	Milestones []gitlab.Milestone
	Issues     []gitlab.Issue
	Tasks      []gitlab.Task
	Labels     []gitlab.Label
}

type existingItem struct {
	target      Target
	id          string
	iid         int64
	title       string
	description string
	labels      []string
	state       string
	milestone   string
	parentIID   int64
}

func Build(snapshot source.Snapshot, config projectconfig.Config, current Current) (Plan, error) {
	if err := config.Validate(); err != nil {
		return Plan{}, fmt.Errorf("invalid project configuration: %w", err)
	}
	itemsByID, levelsByRole, err := validateSourceSnapshot(snapshot, config)
	if err != nil {
		return Plan{}, err
	}

	desired := make([]DesiredItem, 0, len(snapshot.WorkItems))
	ignored := 0
	for _, item := range snapshot.WorkItems {
		target, _ := config.TargetForRole(item.Role)
		if target == "ignore" {
			ignored++
			continue
		}
		desiredItem, err := makeDesiredItem(item, Target(target), itemsByID, config)
		if err != nil {
			return Plan{}, err
		}
		desired = append(desired, desiredItem)
	}
	sort.Slice(desired, func(left, right int) bool {
		leftRole := itemsByID[desired[left].SourceID].Role
		rightRole := itemsByID[desired[right].SourceID].Role
		if levelsByRole[leftRole] != levelsByRole[rightRole] {
			return levelsByRole[leftRole] < levelsByRole[rightRole]
		}
		return desired[left].SourceID < desired[right].SourceID
	})

	existing := collectExisting(current)
	markers, err := indexMarkers(existing, itemsByID)
	if err != nil {
		return Plan{}, err
	}
	matched := make(map[string]string)
	existingIssuesBySource := make(map[string]existingItem)
	for _, desiredItem := range desired {
		candidate, found, err := findExisting(desiredItem, existing, markers)
		if err != nil {
			return Plan{}, err
		}
		if !found || desiredItem.Target != Issue {
			continue
		}
		existingIssuesBySource[desiredItem.SourceID] = candidate
	}

	plan := Plan{Ignored: ignored}
	neededLabels := make(map[string]struct{})
	for _, desiredItem := range desired {
		candidate, found, err := findExisting(desiredItem, existing, markers)
		if err != nil {
			return Plan{}, err
		}
		for _, label := range desiredItem.Labels {
			neededLabels[label] = struct{}{}
		}
		if !found {
			plan.Actions = append(plan.Actions, Action{Operation: Create, Desired: desiredItem})
			continue
		}
		if desiredItem.Target == Issue || desiredItem.Target == Task {
			desiredItem.Labels = mergeManagedLabels(candidate.labels, desiredItem.Labels)
		}
		identity := candidate.target.String() + ":" + candidate.id
		if previous, exists := matched[identity]; exists {
			return Plan{}, fmt.Errorf("GitLab %s %s matches both YouTrack %q and %q", candidate.target, candidate.reference(), previous, desiredItem.SourceID)
		}
		matched[identity] = desiredItem.SourceID

		changes := compare(desiredItem, candidate, existingIssuesBySource)
		if len(changes) == 0 {
			plan.Unchanged++
			continue
		}
		plan.Actions = append(plan.Actions, Action{
			Operation:  Update,
			Desired:    desiredItem,
			CurrentID:  candidate.id,
			CurrentIID: candidate.iid,
			Changes:    changes,
		})
	}

	existingLabels := make(map[string]struct{}, len(current.Labels))
	for _, label := range current.Labels {
		existingLabels[label.Name] = struct{}{}
	}
	labelNames := make([]string, 0, len(neededLabels))
	for label := range neededLabels {
		if _, exists := existingLabels[label]; !exists {
			labelNames = append(labelNames, label)
		}
	}
	sort.Strings(labelNames)
	labelActions := make([]Action, 0, len(labelNames))
	for _, label := range labelNames {
		labelActions = append(labelActions, Action{
			Operation: Create,
			Desired:   DesiredItem{Target: Label, Title: label},
		})
	}
	plan.Actions = append(labelActions, plan.Actions...)
	return plan, nil
}

func validateSourceSnapshot(snapshot source.Snapshot, config projectconfig.Config) (map[string]source.WorkItem, map[string]int, error) {
	itemsByID := make(map[string]source.WorkItem, len(snapshot.WorkItems))
	levelsByRole := make(map[string]int, len(config.YouTrack.Hierarchy))
	for index, level := range config.YouTrack.Hierarchy {
		levelsByRole[level.Role] = index
	}
	for _, item := range snapshot.WorkItems {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Title) == "" {
			return nil, nil, fmt.Errorf("source work item has an empty ID or title")
		}
		if _, exists := itemsByID[item.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate source work item %q", item.ID)
		}
		level, exists := levelsByRole[item.Role]
		if !exists {
			return nil, nil, fmt.Errorf("source work item %q has unknown role %q", item.ID, item.Role)
		}
		role, configuredLevel, found := config.RoleForKind(item.Kind)
		if !found || role != item.Role || configuredLevel != level {
			return nil, nil, fmt.Errorf("source work item %q kind %q does not match role %q", item.ID, item.Kind, item.Role)
		}
		itemsByID[item.ID] = item
	}
	for _, item := range snapshot.WorkItems {
		level := levelsByRole[item.Role]
		if level == 0 {
			if item.ParentID != "" {
				return nil, nil, fmt.Errorf("source root work item %q unexpectedly has parent %q", item.ID, item.ParentID)
			}
			continue
		}
		parent, exists := itemsByID[item.ParentID]
		if !exists {
			return nil, nil, fmt.Errorf("source work item %q references missing parent %q", item.ID, item.ParentID)
		}
		expectedRole := config.YouTrack.Hierarchy[level-1].Role
		if parent.Role != expectedRole {
			return nil, nil, fmt.Errorf("source work item %q requires parent role %q, got %q", item.ID, expectedRole, parent.Role)
		}
	}
	return itemsByID, levelsByRole, nil
}

func makeDesiredItem(item source.WorkItem, target Target, itemsByID map[string]source.WorkItem, config projectconfig.Config) (DesiredItem, error) {
	desired := DesiredItem{
		SourceID:    item.ID,
		Target:      target,
		Description: withSourceMarker(item.Description, item.ID),
		Resolved:    item.Resolved,
	}
	if target == Milestone {
		desired.Title = item.Title
	} else {
		desired.Title = fmt.Sprintf("[%s] %s", item.ID, item.Title)
		desired.Labels = desiredLabels(item)
	}

	parentID := item.ParentID
	for parentID != "" {
		parent := itemsByID[parentID]
		parentTarget, _ := config.TargetForRole(parent.Role)
		if parentTarget != "ignore" {
			desired.ParentSourceID = parent.ID
			desired.ParentTitle = parent.Title
			break
		}
		parentID = parent.ParentID
	}
	if target == Milestone && desired.ParentSourceID != "" {
		return DesiredItem{}, fmt.Errorf("milestone source %q unexpectedly has synchronized parent %q", item.ID, desired.ParentSourceID)
	}
	if target == Task && desired.ParentSourceID == "" {
		return DesiredItem{}, fmt.Errorf("task source %q has no synchronized issue parent", item.ID)
	}
	return desired, nil
}

func desiredLabels(item source.WorkItem) []string {
	labels := append([]string(nil), item.Tags...)
	if priority := strings.TrimSpace(item.Priority); priority != "" {
		labels = append(labels, "prio::"+priority)
	}
	if status := strings.TrimSpace(item.Status); status != "" {
		labels = append(labels, "status::"+status)
	}
	return sortedUnique(labels)
}

func withSourceMarker(description, sourceID string) string {
	marker := "<!-- glab-helper:youtrack:" + sourceID + " -->"
	if strings.TrimSpace(description) == "" {
		return marker
	}
	return strings.TrimRight(description, "\r\n") + "\n\n" + marker
}

func collectExisting(current Current) []existingItem {
	items := make([]existingItem, 0, len(current.Milestones)+len(current.Issues)+len(current.Tasks))
	for _, milestone := range current.Milestones {
		items = append(items, existingItem{
			target: Milestone, id: strconv.FormatInt(milestone.ID, 10), title: milestone.Title,
			description: milestone.Description, state: milestone.State,
		})
	}
	for _, issue := range current.Issues {
		item := existingItem{
			target: Issue, id: strconv.FormatInt(issue.IID, 10), iid: issue.IID, title: issue.Title,
			description: issue.Description, labels: issue.Labels, state: issue.State,
		}
		if issue.Milestone != nil {
			item.milestone = issue.Milestone.Title
		}
		items = append(items, item)
	}
	for _, task := range current.Tasks {
		items = append(items, existingItem{
			target: Task, id: task.ID, iid: task.IID, title: task.Title, description: task.Description,
			labels: task.Labels, state: task.State, parentIID: task.ParentIID,
		})
	}
	return items
}

func indexMarkers(existing []existingItem, sourceItems map[string]source.WorkItem) (map[string]existingItem, error) {
	markers := make(map[string]existingItem)
	for _, item := range existing {
		for _, sourceID := range sourceMarkers(item.description) {
			if _, relevant := sourceItems[sourceID]; !relevant {
				continue
			}
			if previous, exists := markers[sourceID]; exists && (previous.target != item.target || previous.id != item.id) {
				return nil, fmt.Errorf("YouTrack %q is marked on multiple GitLab items (%s %s and %s %s)", sourceID, previous.target, previous.reference(), item.target, item.reference())
			}
			markers[sourceID] = item
		}
	}
	return markers, nil
}

func sourceMarkers(description string) []string {
	var result []string
	for _, prefix := range []string{"<!-- glab-helper:youtrack:", "<!-- jira:"} {
		remainder := description
		for {
			start := strings.Index(remainder, prefix)
			if start < 0 {
				break
			}
			remainder = remainder[start+len(prefix):]
			end := strings.Index(remainder, " -->")
			if end < 0 {
				break
			}
			id := strings.TrimSpace(remainder[:end])
			if id != "" {
				result = append(result, id)
			}
			remainder = remainder[end+len(" -->"):]
		}
	}
	return sortedUnique(result)
}

func findExisting(desired DesiredItem, existing []existingItem, markers map[string]existingItem) (existingItem, bool, error) {
	if marked, found := markers[desired.SourceID]; found {
		if marked.target != desired.Target {
			return existingItem{}, false, fmt.Errorf("YouTrack %q is already represented by GitLab %s %s, but configuration targets %s", desired.SourceID, marked.target, marked.reference(), desired.Target)
		}
		return marked, true, nil
	}

	var matches []existingItem
	var wrongTarget []existingItem
	for _, item := range existing {
		if !fallbackIdentityMatches(desired, item) {
			continue
		}
		if item.target == desired.Target {
			matches = append(matches, item)
		} else {
			wrongTarget = append(wrongTarget, item)
		}
	}
	if len(matches) > 1 {
		return existingItem{}, false, fmt.Errorf("YouTrack %q ambiguously matches multiple GitLab %ss", desired.SourceID, desired.Target)
	}
	if len(matches) == 1 {
		return matches[0], true, nil
	}
	if len(wrongTarget) > 0 {
		item := wrongTarget[0]
		return existingItem{}, false, fmt.Errorf("YouTrack %q appears to be GitLab %s %s by legacy identity, but configuration targets %s", desired.SourceID, item.target, item.reference(), desired.Target)
	}
	return existingItem{}, false, nil
}

func fallbackIdentityMatches(desired DesiredItem, current existingItem) bool {
	if desired.Target == Milestone || current.target == Milestone {
		return desired.Target == Milestone && current.target == Milestone && strings.TrimSpace(current.title) == strings.TrimSpace(desired.Title)
	}
	prefix := "[" + desired.SourceID + "]"
	return strings.HasPrefix(current.title, prefix) && (len(current.title) == len(prefix) || current.title[len(prefix)] == ' ' || current.title[len(prefix)] == '\t')
}

func compare(desired DesiredItem, current existingItem, existingIssuesBySource map[string]existingItem) []Change {
	changes := make([]Change, 0, 5)
	if current.title != desired.Title {
		changes = append(changes, Change{Field: "title", From: current.title, To: desired.Title})
	}
	if current.description != desired.Description {
		changes = append(changes, Change{Field: "description", From: current.description, To: desired.Description})
	}
	if desired.Target == Issue || desired.Target == Task {
		if !sameStringSet(current.labels, desired.Labels) {
			changes = append(changes, Change{Field: "labels", From: strings.Join(sortedUnique(current.labels), ", "), To: strings.Join(desired.Labels, ", ")})
		}
	}
	if desired.Target == Issue && desired.ParentSourceID != "" {
		if current.milestone != desired.ParentTitle {
			changes = append(changes, Change{Field: "milestone", From: current.milestone, To: desired.ParentTitle})
		}
	}
	if desired.Target == Task {
		parent, parentExists := existingIssuesBySource[desired.ParentSourceID]
		if !parentExists || current.parentIID != parent.iid {
			from := ""
			if current.parentIID > 0 {
				from = "#" + strconv.FormatInt(current.parentIID, 10)
			}
			changes = append(changes, Change{Field: "parent", From: from, To: desired.ParentSourceID})
		}
	}
	if desired.Resolved && !strings.EqualFold(current.state, "closed") {
		changes = append(changes, Change{Field: "state", From: current.state, To: "closed"})
	}
	return changes
}

func mergeManagedLabels(current, desired []string) []string {
	merged := make([]string, 0, len(current)+len(desired))
	for _, label := range current {
		if strings.HasPrefix(label, "prio::") || strings.HasPrefix(label, "status::") {
			continue
		}
		merged = append(merged, label)
	}
	merged = append(merged, desired...)
	return sortedUnique(merged)
}

func sameStringSet(left, right []string) bool {
	left = sortedUnique(left)
	right = sortedUnique(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (target Target) String() string {
	return string(target)
}

func (item existingItem) reference() string {
	if item.target == Issue || item.target == Task {
		return "#" + strconv.FormatInt(item.iid, 10)
	}
	return item.id
}
