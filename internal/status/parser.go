package status

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// OrderedEntry is a single development_status key-value pair in document order.
type OrderedEntry struct {
	Key   string
	Value string
}

// EpicGroup holds an epic key, its stories, and its retrospective entry in order.
type EpicGroup struct {
	EpicKey       string
	RetroKey      string
	RetroStatus   string
	Stories       []OrderedEntry
}

// SprintStatus represents the subset of sprint-status.yaml fields needed by the runner.
type SprintStatus struct {
	Generated     string            `yaml:"generated"`
	Project       string            `yaml:"project"`
	DevStatus     map[string]string `yaml:"development_status"`
	StoryLocation string            `yaml:"story_location"`

	// OrderedEntries preserves development_status key order for epic grouping.
	OrderedEntries []OrderedEntry
}

var (
	epicRe         = regexp.MustCompile(`^epic-\d+$`)
	storyRe        = regexp.MustCompile(`^\d+-\d+-.+`)
	retrospectiveRe = regexp.MustCompile(`^epic-\d+-retrospective$`)
)

// Parse reads and parses the sprint-status.yaml file, preserving development_status key order.
func Parse(path string) (*SprintStatus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading sprint-status file: %w", err)
	}
	return ParseBytes(data)
}

// ParseBytes parses sprint-status YAML from an in-memory byte slice.
// This is useful when the raw bytes are already loaded (e.g. for stall detection).
func ParseBytes(data []byte) (*SprintStatus, error) {
	var status SprintStatus
	if err := yaml.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("unmarshaling yaml: %w", err)
	}

	// Parse development_status in order using Node API
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshaling yaml node: %w", err)
	}

	entries, err := extractOrderedEntries(&doc)
	if err != nil {
		return nil, err
	}
	status.OrderedEntries = entries

	return &status, nil
}

func extractOrderedEntries(doc *yaml.Node) ([]OrderedEntry, error) {
	root := doc
	if len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected root mapping node")
	}

	// Find development_status key
	for i := 0; i < len(root.Content)-1; i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Value != "development_status" {
			continue
		}
		if valNode.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("development_status must be a mapping")
		}
		var entries []OrderedEntry
		for j := 0; j < len(valNode.Content)-1; j += 2 {
			k := valNode.Content[j]
			v := valNode.Content[j+1]
			if k.Kind != yaml.ScalarNode || v.Kind != yaml.ScalarNode {
				continue
			}
			entries = append(entries, OrderedEntry{Key: k.Value, Value: v.Value})
		}
		return entries, nil
	}
	return nil, fmt.Errorf("development_status key not found")
}

// EpicGroups returns development_status entries grouped by epic in document order.
func (s *SprintStatus) EpicGroups() []EpicGroup {
	var groups []EpicGroup

	for _, e := range s.OrderedEntries {
		switch {
		case epicRe.MatchString(e.Key):
			groups = append(groups, EpicGroup{EpicKey: e.Key})
		case storyRe.MatchString(e.Key):
			if len(groups) > 0 {
				groups[len(groups)-1].Stories = append(groups[len(groups)-1].Stories, e)
			}
		case retrospectiveRe.MatchString(e.Key):
			if len(groups) > 0 {
				groups[len(groups)-1].RetroKey = e.Key
				groups[len(groups)-1].RetroStatus = e.Value
			}
		}
	}
	return groups
}

// storyStarted returns true if the story status indicates work has begun (beyond backlog/drafted).
func storyStarted(status string) bool {
	switch status {
	case "backlog", "drafted":
		return false
	default:
		return true
	}
}

// NextWork returns the next action to take: "story", "retrospective", or "" if all done.
// epicKey is the epic identifier for display.
// storyKey is the first pending story key when action is "story"; empty for retrospective.
// Processes epics in document order: runs retrospective for completed epics before moving to the next epic's stories.
// For "optional" retros: runs when epic is done and no story has started on the next epic; skips otherwise.
func (s *SprintStatus) NextWork() (action, epicKey, storyKey string, found bool) {
	groups := s.EpicGroups()

	for i, g := range groups {
		allStoriesDone := true
		var firstPendingStory string
		for _, st := range g.Stories {
			if st.Value != "done" && st.Value != "deferred" {
				allStoriesDone = false
				if firstPendingStory == "" {
					firstPendingStory = st.Key
				}
			}
		}

		if allStoriesDone && g.RetroKey != "" {
			retroComplete := g.RetroStatus == "done" || g.RetroStatus == "completed"
			if !retroComplete {
				// "optional" = run only when no story has started on the next epic; skip if we've moved on
				if g.RetroStatus == "optional" {
					nextEpicStarted := false
					if i+1 < len(groups) {
						for _, st := range groups[i+1].Stories {
							if storyStarted(st.Value) {
								nextEpicStarted = true
								break
							}
						}
					}
					if nextEpicStarted {
						continue
					}
				}
				return "retrospective", g.EpicKey, "", true
			}
		}

		if !allStoriesDone {
			return "story", g.EpicKey, firstPendingStory, true
		}
	}
	return "", "", "", false
}

// NextEpicNumber returns the next unused epic number (highest existing number + 1).
// Returns 1 if no epics are present yet.
func (s *SprintStatus) NextEpicNumber() int {
	max := 0
	for _, g := range s.EpicGroups() {
		var n int
		if _, err := fmt.Sscanf(g.EpicKey, "epic-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return max + 1
}

// EpicProgress returns (storiesDone, storiesTotal) for the given epic key.
func (s *SprintStatus) EpicProgress(epicKey string) (done, total int) {
	for _, g := range s.EpicGroups() {
		if g.EpicKey != epicKey {
			continue
		}
		total = len(g.Stories)
		for _, st := range g.Stories {
			if st.Value == "done" || st.Value == "deferred" {
				done++
			}
		}
		return done, total
	}
	return 0, 0
}
