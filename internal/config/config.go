package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Config holds the runner configuration
type Config struct {
	ProjectRoot string
	StatusFile  string
	AgentPath   string
	Model       string
}

// ResolveProjectRoot attempts to find the project root based on the status file path.
// If statusFile is relative, it's resolved against the current working directory.
// When status file is in the standard BMAD location (_bmad-output/implementation-artifacts/),
// project root is 3 levels up. For non-standard paths, falls back to the status file's directory.
func ResolveProjectRoot(statusFile string, projectRootOverride string) (string, string, error) {
	absStatusFile, err := filepath.Abs(statusFile)
	if err != nil {
		return "", "", fmt.Errorf("resolving absolute status file path: %w", err)
	}

	if projectRootOverride != "" {
		absRoot, err := filepath.Abs(projectRootOverride)
		if err != nil {
			return "", "", fmt.Errorf("resolving absolute project root: %w", err)
		}
		return absRoot, absStatusFile, nil
	}

	// Default project root is 3 levels up from _bmad-output/implementation-artifacts/sprint-status.yaml.
	// For non-standard paths (e.g. /tmp/sprint-status.yaml or ./sprint-status.yaml), going 3 levels up
	// can yield the filesystem root "/", which is wrong. Fall back to the directory containing the
	// status file in that case.
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(absStatusFile)))
	if projectRoot == "" || projectRoot == "/" || projectRoot == filepath.VolumeName(projectRoot)+string(filepath.Separator) {
		projectRoot = filepath.Dir(absStatusFile)
	}
	return projectRoot, absStatusFile, nil
}

// AgentType identifies which agent backend to use.
const (
	AgentTypeCursorAgent = "cursor-agent"
	AgentTypeClaudeCode  = "claude-code"
	AgentTypeGeminiCLI   = "gemini-cli"
)

// LookupAgent looks for the appropriate agent binary based on agentType.
// If agentPath is non-empty, it is returned as-is.
func LookupAgent(agentPath string, agentType string) (string, error) {
	if agentPath != "" {
		return agentPath, nil
	}

	var names []string
	switch agentType {
	case AgentTypeClaudeCode:
		names = []string{"claude"}
	case AgentTypeGeminiCLI:
		names = []string{"gemini"}
	case AgentTypeCursorAgent:
		fallthrough
	default:
		names = []string{"cursor-agent", "agent"}
	}

	for _, name := range names {
		path, err := execLookPath(name)
		if err == nil {
			return path, nil
		}
	}

	switch agentType {
	case AgentTypeClaudeCode:
		return "", fmt.Errorf("claude not found in PATH")
	case AgentTypeGeminiCLI:
		return "", fmt.Errorf("gemini not found in PATH")
	default:
		return "", fmt.Errorf("cursor-agent or agent not found in PATH")
	}
}

// execLookPath checks common locations and PATH for the named binary.
func execLookPath(name string) (string, error) {
	home, _ := os.UserHomeDir()
	commonPaths := []string{
		filepath.Join(home, ".local/bin", name),
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("%s not found", name)
}
