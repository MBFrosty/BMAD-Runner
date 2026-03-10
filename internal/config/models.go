package config

// DefaultModels maps each agent type to a map of workflow phase -> model name.
// The special key "default" is used as a fallback for phases not explicitly listed.
//
// Edit this map to change which model is used for each agent/phase combination.
var DefaultModels = map[string]map[string]string{
	AgentTypeCursorAgent: {
		"create-story":    "claude-4.6-sonnet-medium",
		"dev-story":       "composer-1.5",
		"code-review":     "gemini-3-flash",
		"retrospective":   "gemini-3-flash",
		"correct-course":  "claude-4.6-sonnet-medium",
		"sprint-planning": "claude-4.6-sonnet-medium",
		"default":         "composer-1.5",
	},
	AgentTypeClaudeCode: {
		"create-story":    "sonnet",
		"dev-story":       "haiku",
		"code-review":     "sonnet",
		"retrospective":   "sonnet",
		"correct-course":  "sonnet",
		"sprint-planning": "sonnet",
		"default":         "sonnet",
	},
	AgentTypeGeminiCLI: {
		"create-story":    "gemini-3-pro",
		"dev-story":       "gemini-3-flash",
		"code-review":     "gemini-3-pro",
		"retrospective":   "gemini-3-pro",
		"correct-course":  "gemini-3-pro",
		"sprint-planning": "gemini-3-pro",
		"default":         "gemini-3-pro",
	},
	AgentTypeOpenCode: {
		"create-story":    "opencode-go/kimi-k2.5",
		"dev-story":       "opencode-go/minimax-m2.5",
		"code-review":     "opencode-go/kimi-k2.5",
		"retrospective":   "opencode-go/kimi-k2.5",
		"correct-course":  "opencode-go/glm-5",
		"sprint-planning": "opencode-go/glm-5",
		"default":         "opencode-go/kimi-k2.5",
	},
}

// DefaultModel returns the default model name for the given agent type and workflow phase.
// Falls back to the cursor-agent config if agentType is unrecognised, and to the
// agent's "default" entry if the phase has no explicit mapping.
func DefaultModel(agentType, phase string) string {
	phases, ok := DefaultModels[agentType]
	if !ok {
		phases = DefaultModels[AgentTypeCursorAgent]
	}
	if model, ok := phases[phase]; ok {
		return model
	}
	return phases["default"]
}
