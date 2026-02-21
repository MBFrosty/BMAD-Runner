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

// EpicPlanningContext holds all inputs for BuildEpicContext.
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

// BuildEpicContext returns a targeted preamble to prepend to the BMAD
// create-epics-and-stories command file. It scopes the BMAD workflow to adding
// ONE incremental epic for an existing in-progress project.
//
// The preamble includes explicit pre-flight read instructions so the agent
// loads existing epics and retrospective notes before planning — this prevents
// duplication and incorporates lessons learned without requiring a separate
// context-gathering agent invocation.
//
// This is NOT a self-contained prompt — it is prepended before the actual BMAD
// command content so the real BMAD workflow executes with the right scope.
func BuildEpicContext(ctx EpicPlanningContext) string {
	var sb strings.Builder

	sb.WriteString("# Incremental Epic Planning — Context (Read This First)\n\n")
	sb.WriteString("> **IMPORTANT**: This is an EXISTING in-progress project.\n")
	sb.WriteString("> You are NOT starting a new project from scratch.\n")
	sb.WriteString("> The BMAD workflow below is being used to add **one new epic** to an existing project.\n\n")

	// --- Pre-flight read instructions ---
	// Direct the agent to read key project files BEFORE executing the BMAD workflow.
	// This grounds the planning in actual project state rather than just PRD/architecture.
	hasPreflight := ctx.EpicsFilePath != "" || len(ctx.RetroFilePaths) > 0 || ctx.StatusFilePath != ""
	if hasPreflight {
		sb.WriteString("## Pre-Flight: Read These Files Before Planning\n\n")
		sb.WriteString("You **MUST** read the following files before executing the BMAD workflow.\n")
		sb.WriteString("They contain critical context that the workflow steps do not automatically load.\n\n")

		if ctx.EpicsFilePath != "" {
			sb.WriteString(fmt.Sprintf("**1. Existing Epics Document** — read this FIRST:\n"))
			sb.WriteString(fmt.Sprintf("   - File: `%s`\n", ctx.EpicsFilePath))
			sb.WriteString("   - Purpose: Understand ALL epics already planned; do NOT duplicate or conflict with them\n")
			if len(ctx.CompletedEpics) > 0 {
				sb.WriteString(fmt.Sprintf("   - Already completed: %s\n", strings.Join(ctx.CompletedEpics, ", ")))
			}
			sb.WriteString("\n")
		}

		if len(ctx.RetroFilePaths) > 0 {
			sb.WriteString("**2. Recent Retrospective(s)** — read for lessons learned:\n")
			for _, p := range ctx.RetroFilePaths {
				sb.WriteString(fmt.Sprintf("   - `%s`\n", p))
			}
			sb.WriteString("   - Purpose: Apply technical debt items, action items, and next-epic recommendations\n\n")
		}

		if ctx.StatusFilePath != "" {
			sb.WriteString("**3. Sprint Status** — confirm current project state:\n")
			sb.WriteString(fmt.Sprintf("   - File: `%s`\n", ctx.StatusFilePath))
			sb.WriteString("   - Purpose: Verify which stories are done, in-progress, or backlogged\n\n")
		}

		sb.WriteString("---\n\n")
	}

	// --- Scope definition ---
	sb.WriteString("## Your Specific Scope\n\n")
	sb.WriteString(fmt.Sprintf("- Plan **Epic %d only** (the next epic in sequence)\n", ctx.NextEpicNum))
	sb.WriteString("- **Append** this epic to the existing epics document — do NOT overwrite or rewrite existing epics\n")
	sb.WriteString("- Give this epic **3–7 stories** that are actionable and independently testable\n")
	sb.WriteString("- Skip strict prerequisite validation steps — use whatever documents already exist in the project\n")
	sb.WriteString("- If the workflow asks to design all epics, limit yourself to this one new epic only\n\n")

	// --- Prime directive ---
	if ctx.PrimeDirective != "" {
		sb.WriteString("## Prime Directive (Your Strategic Guide)\n\n")
		sb.WriteString("Use the following to determine the focus area for this new epic:\n\n")
		sb.WriteString(ctx.PrimeDirective)
		sb.WriteString("\n\n")
	}

	// --- Fallback focus priorities ---
	sb.WriteString("## Focus Priorities (if prime directive does not specify)\n\n")
	sb.WriteString("Consider these areas in order of impact:\n")
	sb.WriteString("1. **Hardening** — stability, reliability, error handling, edge cases\n")
	sb.WriteString("2. **New Features** — valuable additions aligned with the product vision\n")
	sb.WriteString("3. **Technical Debt** — code quality, refactoring, architecture improvements\n")
	sb.WriteString("4. **Performance** — speed, efficiency, scalability\n")
	sb.WriteString("5. **Testing & QA** — improved coverage, automation, quality gates\n\n")

	sb.WriteString("---\n\n")
	sb.WriteString("Now execute the BMAD workflow below, keeping the above context in mind:\n\n")

	return sb.String()
}
