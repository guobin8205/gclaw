package memory

// Entry represents a single memory record.
type Entry struct {
	Title       string
	Content     string
	Type        string // user, feedback, project, reference
	File        string // relative path to the .md file
}
