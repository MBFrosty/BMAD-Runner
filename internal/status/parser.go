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

// NextWork returns the next action to take: "story", "retrospective", or "" if all done.
// epicKey is the epic identifier for display.
// storyKey is the first pending story key when action is "story"; empty for retrospective.
// Skips retrospectives for epics that have been "passed" (a later epic has pending stories).
func (s *SprintStatus) NextWork() (action, epicKey, storyKey string, found bool) {
	groups := s.EpicGroups()

	// First pass: find any epic with pending stories
	for _, g := range groups {
		for _, st := range g.Stories {
			if st.Value != "done" && st.Value != "deferred" {
				return "story", g.EpicKey, st.Key, true
			}
		}
	}

	// No pending stories: run retrospective only for the last epic that needs it.
	// Skip retros for "passed" epics (we're focused on a later epic).
	// Treat both "done" and "completed" as finished (BMAD may use either).
	for i := len(groups) - 1; i >= 0; i-- {
		g := groups[i]
		retroComplete := g.RetroStatus == "done" || g.RetroStatus == "completed"
		if g.RetroKey != "" && !retroComplete {
			return "retrospective", g.EpicKey, "", true
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
