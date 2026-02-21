package planner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultPrimeDirectivePath is the default location of the prime directive relative to project root.
const DefaultPrimeDirectivePath = "_bmad-output/prime-directive.md"

// DefaultMaxEpics is the default maximum number of new epics to plan per auto session.
const DefaultMaxEpics = 5

// defaultPrimeDirective is written when no prime directive exists yet.
const defaultPrimeDirective = `# Prime Directive

This file guides BMAD Runner when automatically planning new epics after all current work
is complete. Edit this file to align the AI's planning with your project goals.

---

## Project Vision

<!-- Describe the overall goal and north star for this project.
     What problem is it solving? Who is it for? What does success look like? -->

(Fill in your project vision here)

---

## Current Focus Areas

<!-- What should the AI prioritize when planning new work?
     List in order of importance. -->

- Hardening and stability improvements
- Error handling and edge-case coverage
- Performance optimizations
- Test coverage improvements
- New user-facing features aligned with the vision
- Technical debt reduction and refactoring

---

## Constraints and Guardrails

<!-- What should the AI avoid or be mindful of? -->

- Stay within the existing architectural decisions
- Maintain backward compatibility where appropriate
- Follow existing code patterns and conventions
- Do not introduce unnecessary external dependencies

---

## Goals for Next Phase

<!-- Specific outcomes you want the AI to consider when planning the next batch of epics.
     Be as specific as possible — the AI will use these to choose what to build next. -->

- (Describe your next milestone or desired outcome)
- (Add more goals as needed)

---

## Out of Scope

<!-- Anything that should NOT be built in the next phase, even if it seems related -->

- (List anything that should be deferred or avoided)
`

// EnsurePrimeDirective creates the prime directive file with defaults if it does not exist.
// Returns (created bool, err error). If created is true, the user should review the file
// before continuing.
func EnsurePrimeDirective(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil // already exists
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("creating prime directive directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultPrimeDirective), 0o644); err != nil {
		return false, fmt.Errorf("creating prime directive file: %w", err)
	}
	return true, nil
}

// ReadPrimeDirective reads the prime directive file content.
// Returns empty string if the file does not exist.
func ReadPrimeDirective(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading prime directive: %w", err)
	}
	return string(data), nil
}

// IsDefaultPrimeDirective returns true if the content still looks like the unedited default.
func IsDefaultPrimeDirective(content string) bool {
	return strings.Contains(content, "(Fill in your project vision here)") ||
		strings.Contains(content, "(Describe your next milestone or desired outcome)")
}

// FindEpicsFile searches for the existing epics document in standard BMAD output locations.
// Returns the path if found, or empty string if not.
func FindEpicsFile(projectRoot string) string {
	candidates := []string{
		filepath.Join(projectRoot, "_bmad-output", "planning-artifacts", "epics.md"),
		filepath.Join(projectRoot, "_bmad-output", "planning-artifacts", "bmm-epics.md"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// Fuzzy glob fallback: any *epic*.md in planning-artifacts
	matches, _ := filepath.Glob(filepath.Join(projectRoot, "_bmad-output", "planning-artifacts", "*epic*.md"))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// FindRetroFiles returns paths to retrospective files in the implementation-artifacts directory,
// sorted newest-first (by filename, which encodes the date). maxFiles limits the result;
// pass 0 for no limit.
func FindRetroFiles(projectRoot string, maxFiles int) []string {
	pattern := filepath.Join(projectRoot, "_bmad-output", "implementation-artifacts", "epic-*-retro-*.md")
	matches, _ := filepath.Glob(pattern)
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	if maxFiles > 0 && len(matches) > maxFiles {
		return matches[:maxFiles]
	}
	return matches
}

// EpicPlanningContext holds all inputs for BuildEpicPlanningPrompt.
type EpicPlanningContext struct {
	// PrimeDirective is the content of the prime directive file (may be empty).
	PrimeDirective string

	// NextEpicNum is the sequential number of the epic to plan (e.g. 3 means "plan Epic 3").
	NextEpicNum int

	// EpicsFilePath is the path to the existing epics document.
	// When set, the agent is directed to read it first so it doesn't duplicate existing epics.
	EpicsFilePath string

	// RetroFilePaths are paths to recent retrospective files (newest first).
	// When set, the agent is directed to read them for lessons learned and tech debt items.
	RetroFilePaths []string

	// CompletedEpics is the list of epic keys that are fully done (e.g. ["epic-1", "epic-2"]).
	// Injected into the context so the agent knows what's already been built.
	CompletedEpics []string

	// StatusFilePath is the path to sprint-status.yaml.
	// Shown to the agent so it can verify the current project state.
	StatusFilePath string
}

// BuildEpicPlanningPrompt returns a self-contained task prompt for a targeted RunWithPrompt
// agent activation. The prompt tells the agent to:
//  1. Read the prime directive, existing epics document, recent retrospective(s), and sprint-status
//  2. Synthesize the project's current state and the prime directive's direction
//  3. Append exactly ONE new epic (nextEpicNum) in BMAD format to the existing epics document
//
// This is NOT a preamble for the BMAD create-epics-and-stories workflow.
// That workflow re-generates ALL epics from the PRD and is designed for initial project planning.
// For incremental epic planning on an existing project, a targeted RunWithPrompt is more reliable.
//
// The sprint-planning BMAD command is still used afterward (Phase B) to update sprint-status.yaml.
func BuildEpicPlanningPrompt(ctx EpicPlanningContext) string {
	var sb strings.Builder

	sb.WriteString("# Task: Add Epic ")
	sb.WriteString(fmt.Sprintf("%d", ctx.NextEpicNum))
	sb.WriteString(" to Existing Project\n\n")

	sb.WriteString("You are a BMAD Product Manager. Your task is to append ONE new epic to an existing\n")
	sb.WriteString("in-progress project. Do NOT create a new project. Do NOT regenerate existing epics.\n\n")

	// --- Step 1: Read context files ---
	sb.WriteString("## Step 1: Read These Files First (MANDATORY)\n\n")
	sb.WriteString("Before writing anything, read ALL of the following files to understand the project:\n\n")

	step := 1
	if ctx.EpicsFilePath != "" {
		sb.WriteString(fmt.Sprintf("**%d. Existing Epics Document** (read in full — know what's already planned):\n", step))
		sb.WriteString(fmt.Sprintf("   `%s`\n", ctx.EpicsFilePath))
		if len(ctx.CompletedEpics) > 0 {
			sb.WriteString(fmt.Sprintf("   Already completed: %s\n", strings.Join(ctx.CompletedEpics, ", ")))
		}
		sb.WriteString("\n")
		step++
	}

	if len(ctx.RetroFilePaths) > 0 {
		sb.WriteString(fmt.Sprintf("**%d. Recent Retrospective(s)** (read for lessons learned and recommended next steps):\n", step))
		for _, p := range ctx.RetroFilePaths {
			sb.WriteString(fmt.Sprintf("   `%s`\n", p))
		}
		sb.WriteString("\n")
		step++
	}

	if ctx.StatusFilePath != "" {
		sb.WriteString(fmt.Sprintf("**%d. Sprint Status** (confirm what's done, in-progress, or backlogged):\n", step))
		sb.WriteString(fmt.Sprintf("   `%s`\n", ctx.StatusFilePath))
		sb.WriteString("\n")
		step++
	}

	if ctx.PrimeDirective != "" {
		sb.WriteString(fmt.Sprintf("**%d. Prime Directive** (your strategic guide for what to build next):\n\n", step))
		sb.WriteString(ctx.PrimeDirective)
		sb.WriteString("\n\n")
		step++
	}

	// --- Step 2: Decide the focus ---
	sb.WriteString("## Step 2: Decide the Epic Focus\n\n")
	sb.WriteString("After reading the files above, determine:\n")
	sb.WriteString("- What work is already completed or in progress?\n")
	sb.WriteString("- What does the most recent retrospective recommend?\n")
	sb.WriteString("- What does the prime directive say to prioritize?\n\n")
	sb.WriteString("Choose ONE focused theme for this new epic. Good themes (in order of typical priority):\n")
	sb.WriteString("1. **Hardening** — stability, reliability, error handling, edge cases\n")
	sb.WriteString("2. **New Features** — valuable additions aligned with the product vision\n")
	sb.WriteString("3. **Technical Debt** — code quality, refactoring, maintainability\n")
	sb.WriteString("4. **Performance** — speed, scalability, efficiency\n")
	sb.WriteString("5. **Testing & Observability** — coverage, monitoring, quality gates\n\n")

	// --- Step 3: Write the epic ---
	epicsTarget := ctx.EpicsFilePath
	if epicsTarget == "" {
		epicsTarget = "_bmad-output/planning-artifacts/epics.md"
	}

	sb.WriteString("## Step 3: Append the Epic\n\n")
	sb.WriteString(fmt.Sprintf("Append Epic %d to `%s`.\n\n", ctx.NextEpicNum, epicsTarget))
	sb.WriteString("**CRITICAL RULES:**\n")
	sb.WriteString("- APPEND ONLY — do NOT modify or delete any existing content in the file\n")
	sb.WriteString("- If the file does not exist, create it\n")
	sb.WriteString("- 3–7 stories per epic; each must be completable independently by a single dev agent\n")
	sb.WriteString("- Each story delivers user value (no 'set up database' or 'create boilerplate' stories)\n")
	sb.WriteString("- Stories must NOT depend on future stories within the same epic\n\n")
	sb.WriteString("Use this EXACT format:\n\n")
	sb.WriteString("```markdown\n")
	sb.WriteString(fmt.Sprintf("## Epic %d: [Epic Title]\n\n", ctx.NextEpicNum))
	sb.WriteString("[One sentence: what user value this epic delivers]\n\n")
	sb.WriteString(fmt.Sprintf("### Story %d.1: [Story Title]\n\n", ctx.NextEpicNum))
	sb.WriteString("As a [user type],\n")
	sb.WriteString("I want [capability],\n")
	sb.WriteString("So that [value/benefit].\n\n")
	sb.WriteString("**Acceptance Criteria:**\n\n")
	sb.WriteString("**Given** [precondition]\n")
	sb.WriteString("**When** [action]\n")
	sb.WriteString("**Then** [expected outcome]\n\n")
	sb.WriteString("[Repeat Story N.M block for each story, 3-7 total]\n")
	sb.WriteString("```\n\n")
	sb.WriteString("After writing the epic, confirm: 'Epic ")
	sb.WriteString(fmt.Sprintf("%d", ctx.NextEpicNum))
	sb.WriteString(" appended to epics document.'\n")

	return sb.String()
}
