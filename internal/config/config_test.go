package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProjectRoot(t *testing.T) {
	tmp := t.TempDir()
	tests := []struct {
		name              string
		statusFile        string
		projectRootOverride string
		check             func(*testing.T, string, string, error)
	}{
		{
			name:               "with override",
			statusFile:         "s.yaml",
			projectRootOverride: tmp,
			check: func(t *testing.T, root, status string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				absTmp, _ := filepath.Abs(tmp)
				if root != absTmp {
					t.Errorf("root = %q, want %q", root, absTmp)
				}
				if status == "" {
					t.Error("status file should be resolved")
				}
			},
		},
		{
			name:               "without override default logic",
			statusFile:         filepath.Join(tmp, "a", "b", "status.yaml"),
			projectRootOverride: "",
			check: func(t *testing.T, root, status string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				wantRoot := filepath.Clean(tmp)
				if filepath.Clean(root) != wantRoot {
					t.Errorf("root = %q, want %q", filepath.Clean(root), wantRoot)
				}
				if !filepath.IsAbs(status) {
					t.Errorf("status file should be absolute, got %q", status)
				}
			},
		},
		{
			name:               "relative status file resolves",
			statusFile:         "relative/path.yaml",
			projectRootOverride: "",
			check: func(t *testing.T, root, status string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !filepath.IsAbs(status) {
					t.Errorf("returned statusFile should be absolute, got %q", status)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, status, err := ResolveProjectRoot(tt.statusFile, tt.projectRootOverride)
			tt.check(t, root, status, err)
		})
	}
}

func TestDefaultModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		agentType string
		phase    string
		want     string
	}{
		{AgentTypeCursorAgent, "create-story", "claude-4.6-sonnet-medium"},
		{AgentTypeClaudeCode, "dev-story", "haiku"},
		{AgentTypeGeminiCLI, "code-review", "gemini-3-pro"},
		{AgentTypeCursorAgent, "nonexistent-phase", "composer-1.5"},
		{"unknown-agent", "create-story", "claude-4.6-sonnet-medium"},
		{"unknown-agent", "nonexistent", "composer-1.5"},
	}
	for _, tt := range tests {
		t.Run(tt.agentType+"_"+tt.phase, func(t *testing.T) {
			t.Parallel()
			got := DefaultModel(tt.agentType, tt.phase)
			if got != tt.want {
				t.Errorf("DefaultModel(%q, %q) = %q, want %q", tt.agentType, tt.phase, got, tt.want)
			}
		})
	}
}

func TestLookupAgent(t *testing.T) {
	original := execLookPath
	defer func() { execLookPath = original }()

	tests := []struct {
		name      string
		agentPath string
		agentType string
		stub      func(string) (string, error)
		wantPath  string
		wantErr   string
	}{
		{
			name:      "explicit agentPath returned as-is",
			agentPath: "/my/agent",
			agentType: AgentTypeClaudeCode,
			wantPath:  "/my/agent",
		},
		{
			name:      "claude-code found via PATH",
			agentPath: "",
			agentType: AgentTypeClaudeCode,
			stub: func(name string) (string, error) {
				if name == "claude" {
					return "/usr/bin/claude", nil
				}
				return "", fmt.Errorf("not found")
			},
			wantPath: "/usr/bin/claude",
		},
		{
			name:      "gemini-cli found via PATH",
			agentPath: "",
			agentType: AgentTypeGeminiCLI,
			stub: func(name string) (string, error) {
				if name == "gemini" {
					return "/usr/bin/gemini", nil
				}
				return "", fmt.Errorf("not found")
			},
			wantPath: "/usr/bin/gemini",
		},
		{
			name:      "cursor-agent first name found",
			agentPath: "",
			agentType: AgentTypeCursorAgent,
			stub: func(name string) (string, error) {
				if name == "cursor-agent" {
					return "/usr/bin/cursor-agent", nil
				}
				return "", fmt.Errorf("not found")
			},
			wantPath: "/usr/bin/cursor-agent",
		},
		{
			name:      "cursor-agent fallback to agent",
			agentPath: "",
			agentType: AgentTypeCursorAgent,
			stub: func(name string) (string, error) {
				if name == "agent" {
					return "/usr/bin/agent", nil
				}
				return "", fmt.Errorf("not found")
			},
			wantPath: "/usr/bin/agent",
		},
		{
			name:      "claude-code not found",
			agentPath: "",
			agentType: AgentTypeClaudeCode,
			stub:      func(string) (string, error) { return "", fmt.Errorf("not found") },
			wantErr:   "claude not found",
		},
		{
			name:      "gemini-cli not found",
			agentPath: "",
			agentType: AgentTypeGeminiCLI,
			stub:      func(string) (string, error) { return "", fmt.Errorf("not found") },
			wantErr:   "gemini not found",
		},
		{
			name:      "default type not found",
			agentPath: "",
			agentType: "",
			stub:      func(string) (string, error) { return "", fmt.Errorf("not found") },
			wantErr:   "cursor-agent or agent not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stub != nil {
				execLookPath = tt.stub
			}
			got, err := LookupAgent(tt.agentPath, tt.agentType)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %v does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantPath {
				t.Errorf("LookupAgent() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}
