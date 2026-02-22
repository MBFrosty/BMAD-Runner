package status

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, devStatus string) *SprintStatus {
	t.Helper()
	var yaml string
	if devStatus == "" {
		yaml = "generated: \"2025-01-01\"\nproject: test-project\nstory_location: stories\ndevelopment_status: {}"
	} else {
		yaml = "generated: \"2025-01-01\"\nproject: test-project\nstory_location: stories\ndevelopment_status:\n" + devStatus
	}
	s, err := ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	return s
}

func TestParseBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		yaml    string
		wantErr string
		check   func(*testing.T, *SprintStatus)
	}{
		{
			name: "valid minimal YAML",
			yaml: "generated: \"2025-01-01\"\nproject: test-project\nstory_location: stories\ndevelopment_status:\n  epic-1: \"\"\n  1-1-foo: backlog",
			check: func(t *testing.T, s *SprintStatus) {
				if s.DevStatus == nil {
					t.Error("DevStatus should be populated")
				}
				if len(s.OrderedEntries) != 2 {
					t.Errorf("OrderedEntries length = %d, want 2", len(s.OrderedEntries))
				}
			},
		},
		{
			name: "empty development_status",
			yaml: "generated: \"2025-01-01\"\nproject: p\nstory_location: s\ndevelopment_status: {}",
			check: func(t *testing.T, s *SprintStatus) {
				if len(s.OrderedEntries) != 0 {
					t.Errorf("OrderedEntries length = %d, want 0", len(s.OrderedEntries))
				}
			},
		},
		{
			name:    "invalid YAML",
			yaml:    "\":\n  - ][bad",
			wantErr: "unmarshaling",
		},
		{
			name:    "missing development_status key",
			yaml:    "generated: \"2025-01-01\"\nproject: p\nstory_location: s",
			wantErr: "development_status key not found",
		},
		{
			name: "order preservation",
			yaml: "generated: \"2025-01-01\"\nproject: p\nstory_location: s\ndevelopment_status:\n  epic-1: \"\"\n  1-1-a: backlog\n  epic-2: \"\"\n  2-1-b: backlog",
			check: func(t *testing.T, s *SprintStatus) {
				keys := make([]string, len(s.OrderedEntries))
				for i, e := range s.OrderedEntries {
					keys[i] = e.Key
				}
				want := []string{"epic-1", "1-1-a", "epic-2", "2-1-b"}
				if len(keys) != len(want) {
					t.Errorf("got %d entries, want %d", len(keys), len(want))
				}
				for i := range want {
					if i < len(keys) && keys[i] != want[i] {
						t.Errorf("OrderedEntries[%d].Key = %q, want %q", i, keys[i], want[i])
					}
				}
			},
		},
		{
			name: "scalar fields populated",
			yaml: "generated: \"2026-02-22\"\nproject: my-project\nstory_location: _bmad/stories\ndevelopment_status:\n  epic-1: \"\"",
			check: func(t *testing.T, s *SprintStatus) {
				if s.Generated != "2026-02-22" {
					t.Errorf("Generated = %q, want 2026-02-22", s.Generated)
				}
				if s.Project != "my-project" {
					t.Errorf("Project = %q, want my-project", s.Project)
				}
				if s.StoryLocation != "_bmad/stories" {
					t.Errorf("StoryLocation = %q, want _bmad/stories", s.StoryLocation)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := ParseBytes([]byte(tt.yaml))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseBytes: expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ParseBytes: error %v does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBytes: %v", err)
			}
			if tt.check != nil {
				tt.check(t, s)
			}
		})
	}
}

func TestEpicGroups(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		dev    string
		check  func(*testing.T, []EpicGroup)
	}{
		{
			name: "single epic two stories",
			dev:  "  epic-1: \"\"\n  1-1-foo: backlog\n  1-2-bar: done",
			check: func(t *testing.T, groups []EpicGroup) {
				if len(groups) != 1 {
					t.Fatalf("len(groups) = %d, want 1", len(groups))
				}
				if len(groups[0].Stories) != 2 {
					t.Errorf("Stories count = %d, want 2", len(groups[0].Stories))
				}
				if groups[0].RetroKey != "" {
					t.Errorf("RetroKey = %q, want empty", groups[0].RetroKey)
				}
			},
		},
		{
			name: "two epics",
			dev:  "  epic-1: \"\"\n  1-1-a: done\n  epic-2: \"\"\n  2-1-b: backlog",
			check: func(t *testing.T, groups []EpicGroup) {
				if len(groups) != 2 {
					t.Fatalf("len(groups) = %d, want 2", len(groups))
				}
				if len(groups[0].Stories) != 1 {
					t.Errorf("Epic 1 stories = %d, want 1", len(groups[0].Stories))
				}
				if len(groups[1].Stories) != 1 {
					t.Errorf("Epic 2 stories = %d, want 1", len(groups[1].Stories))
				}
			},
		},
		{
			name: "epic with retrospective",
			dev:  "  epic-1: \"\"\n  1-1-foo: done\n  epic-1-retrospective: pending",
			check: func(t *testing.T, groups []EpicGroup) {
				if len(groups) != 1 {
					t.Fatalf("len(groups) = %d, want 1", len(groups))
				}
				if groups[0].RetroKey != "epic-1-retrospective" {
					t.Errorf("RetroKey = %q, want epic-1-retrospective", groups[0].RetroKey)
				}
				if groups[0].RetroStatus != "pending" {
					t.Errorf("RetroStatus = %q, want pending", groups[0].RetroStatus)
				}
			},
		},
		{
			name: "orphan story before epic",
			dev:  "  1-1-foo: backlog\n  epic-1: \"\"",
			check: func(t *testing.T, groups []EpicGroup) {
				if len(groups) != 1 {
					t.Fatalf("len(groups) = %d, want 1", len(groups))
				}
				if len(groups[0].Stories) != 0 {
					t.Errorf("Stories count = %d, want 0 (orphan dropped)", len(groups[0].Stories))
				}
			},
		},
		{
			name: "empty entries",
			dev:  "",
			check: func(t *testing.T, groups []EpicGroup) {
				if len(groups) != 0 {
					t.Errorf("len(groups) = %d, want 0", len(groups))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := mustParse(t, tt.dev)
			groups := s.EpicGroups()
			tt.check(t, groups)
		})
	}
}

func TestNextWork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		dev          string
		wantAction   string
		wantEpicKey  string
		wantStoryKey string
		wantFound    bool
	}{
		{
			name:         "first story pending",
			dev:          "  epic-1: \"\"\n  1-1-a: backlog",
			wantAction:   "story",
			wantEpicKey:  "epic-1",
			wantStoryKey: "1-1-a",
			wantFound:    true,
		},
		{
			name:         "all stories done retro pending",
			dev:          "  epic-1: \"\"\n  1-1-a: done\n  epic-1-retrospective: pending",
			wantAction:   "retrospective",
			wantEpicKey:  "epic-1",
			wantStoryKey: "",
			wantFound:    true,
		},
		{
			name:         "all stories done retro done",
			dev:          "  epic-1: \"\"\n  1-1-a: done\n  epic-1-retrospective: done",
			wantAction:   "",
			wantEpicKey:  "",
			wantStoryKey: "",
			wantFound:    false,
		},
		{
			name:         "retro completed synonym",
			dev:          "  epic-1: \"\"\n  1-1-a: done\n  epic-1-retrospective: completed",
			wantAction:   "",
			wantEpicKey:  "",
			wantStoryKey: "",
			wantFound:    false,
		},
		{
			name:         "multiple epics first incomplete",
			dev:          "  epic-1: \"\"\n  1-1-a: backlog\n  epic-2: \"\"\n  2-1-x: backlog",
			wantAction:   "story",
			wantEpicKey:  "epic-1",
			wantStoryKey: "1-1-a",
			wantFound:    true,
		},
		{
			name:         "first epic done second has pending",
			dev:          "  epic-1: \"\"\n  1-1-a: done\n  epic-1-retrospective: done\n  epic-2: \"\"\n  2-1-x: backlog",
			wantAction:   "story",
			wantEpicKey:  "epic-2",
			wantStoryKey: "2-1-x",
			wantFound:    true,
		},
		{
			name:         "optional retro next epic not started",
			dev:          "  epic-1: \"\"\n  1-1-a: done\n  epic-1-retrospective: optional\n  epic-2: \"\"\n  2-1-x: backlog",
			wantAction:   "retrospective",
			wantEpicKey:  "epic-1",
			wantStoryKey: "",
			wantFound:    true,
		},
		{
			name:         "optional retro next epic started",
			dev:          "  epic-1: \"\"\n  1-1-a: done\n  epic-1-retrospective: optional\n  epic-2: \"\"\n  2-1-x: in-progress",
			wantAction:   "story",
			wantEpicKey:  "epic-2",
			wantStoryKey: "2-1-x",
			wantFound:    true,
		},
		{
			name:         "deferred stories count as done",
			dev:          "  epic-1: \"\"\n  1-1-a: deferred\n  epic-1-retrospective: pending",
			wantAction:   "retrospective",
			wantEpicKey:  "epic-1",
			wantStoryKey: "",
			wantFound:    true,
		},
		{
			name:         "first pending story picked",
			dev:          "  epic-1: \"\"\n  1-1-a: done\n  1-2-b: backlog\n  1-3-c: backlog",
			wantAction:   "story",
			wantEpicKey:  "epic-1",
			wantStoryKey: "1-2-b",
			wantFound:    true,
		},
		{
			name:         "no epics",
			dev:          "  x-foo: bar",
			wantAction:   "",
			wantEpicKey:  "",
			wantStoryKey: "",
			wantFound:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := mustParse(t, tt.dev)
			action, epicKey, storyKey, found := s.NextWork()
			if action != tt.wantAction {
				t.Errorf("action = %q, want %q", action, tt.wantAction)
			}
			if epicKey != tt.wantEpicKey {
				t.Errorf("epicKey = %q, want %q", epicKey, tt.wantEpicKey)
			}
			if storyKey != tt.wantStoryKey {
				t.Errorf("storyKey = %q, want %q", storyKey, tt.wantStoryKey)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

func TestNextEpicNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		dev  string
		want int
	}{
		{"no epics", "", 1},
		{"one epic", "  epic-1: \"\"", 2},
		{"three epics", "  epic-1: \"\"\n  epic-2: \"\"\n  epic-3: \"\"", 4},
		{"gap", "  epic-1: \"\"\n  epic-5: \"\"", 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := mustParse(t, tt.dev)
			got := s.NextEpicNumber()
			if got != tt.want {
				t.Errorf("NextEpicNumber() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEpicProgress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		dev       string
		epicKey   string
		wantDone  int
		wantTotal int
	}{
		{
			name:      "all done",
			dev:       "  epic-1: \"\"\n  1-1-a: done\n  1-2-b: done\n  1-3-c: done",
			epicKey:   "epic-1",
			wantDone:  3,
			wantTotal: 3,
		},
		{
			name:      "mixed",
			dev:       "  epic-1: \"\"\n  1-1-a: done\n  1-2-b: deferred\n  1-3-c: backlog",
			epicKey:   "epic-1",
			wantDone:  2,
			wantTotal: 3,
		},
		{
			name:      "none done",
			dev:       "  epic-1: \"\"\n  1-1-a: backlog\n  1-2-b: backlog",
			epicKey:   "epic-1",
			wantDone:  0,
			wantTotal: 2,
		},
		{
			name:      "unknown epic key",
			dev:       "  epic-1: \"\"\n  1-1-a: done",
			epicKey:   "epic-99",
			wantDone:  0,
			wantTotal: 0,
		},
		{
			name:      "no stories",
			dev:       "  epic-1: \"\"",
			epicKey:   "epic-1",
			wantDone:  0,
			wantTotal: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := mustParse(t, tt.dev)
			done, total := s.EpicProgress(tt.epicKey)
			if done != tt.wantDone {
				t.Errorf("done = %d, want %d", done, tt.wantDone)
			}
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
		})
	}
}
