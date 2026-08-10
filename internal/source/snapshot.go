package source

type Snapshot struct {
	WorkItems []WorkItem
}

type WorkItem struct {
	ID          string
	Title       string
	Description string
	Kind        string
	Status      string
	Priority    string
	Tags        []string
	ParentID    string
	Resolved    bool
}
