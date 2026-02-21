package planner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPrimeDirectivePath is the default location of the prime directive relative to project root.
const DefaultPrimeDirectivePath = "_bmad-output/prime-directive.md"

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
// This is a heuristic — we check for the fill-in placeholder text.
func IsDefaultPrimeDirective(content string) bool {
	return strings.Contains(content, "(Fill in your project vision here)") ||
		strings.Contains(content, "(Describe your next milestone or desired outcome)")
}

// BuildPrompt creates the yolo-mode prompt for the epic-planning agent phase.
// primeDirective is the content of the prime directive file (may be empty).
// projectRoot is the absolute path to the project being planned.
// maxEpics is the maximum number of new epics to create (typically 5).
func BuildPrompt(primeDirective, projectRoot string, maxEpics int) string {
	var sb strings.Builder

	sb.WriteString("# Automated Epic Planning Task\n\n")
	sb.WriteString("You are a senior product strategist and technical architect operating in BMAD yolo mode.\n")
	sb.WriteString("Your task: analyze the current project state and generate new epics + stories to continue development.\n\n")
	sb.WriteString("## CRITICAL: Operate in #yolo mode\n")
	sb.WriteString("- Accept ALL suggestions automatically without asking\n")
	sb.WriteString("- Skip all confirmations, menus, and elicitation steps\n")
	sb.WriteString("- Proceed autonomously through every step\n")
	sb.WriteString("- Simulate expert 'yes/continue' responses for any interactive prompts\n\n")

	sb.WriteString("---\n\n")
	sb.WriteString("## Prime Directive\n\n")
	if primeDirective != "" {
		sb.WriteString(primeDirective)
	} else {
		sb.WriteString("*No prime directive found. Use your best judgment based on the project context and retrospective insights.*\n")
	}
	sb.WriteString("\n\n---\n\n")

	sb.WriteString("## Your Mission\n\n")
	sb.WriteString("All currently planned stories have been completed. ")
	sb.WriteString(fmt.Sprintf("Generate up to %d new epics with stories to continue project development.\n\n", maxEpics))

	sb.WriteString("---\n\n")
	sb.WriteString("## Execution Steps (follow in order, no skipping)\n\n")

	sb.WriteString("### Step 1: Understand the Current State\n\n")
	sb.WriteString("Read and analyze these project artifacts (use what exists, skip gracefully if missing):\n\n")
	sb.WriteString("1. **Sprint status**: `_bmad-output/implementation-artifacts/sprint-status.yaml`\n")
	sb.WriteString("   - Note all completed epics, their numbers, and the last epic number used\n")
	sb.WriteString("2. **Existing epics**: any `_bmad-output/planning-artifacts/epic*.md` files\n")
	sb.WriteString("   - Note what has already been planned and built\n")
	sb.WriteString("3. **Retrospective notes**: any `_bmad-output/implementation-artifacts/*retro*.md` files\n")
	sb.WriteString("   - Extract lessons learned, identified gaps, and suggestions for future work\n")
	sb.WriteString("4. **PRD**: `_bmad-output/planning-artifacts/prd.md` or similar\n")
	sb.WriteString("   - Note original goals, any unimplemented requirements\n")
	sb.WriteString("5. **Architecture**: `_bmad-output/planning-artifacts/architecture.md` or similar\n")
	sb.WriteString("   - Note technical constraints and patterns to follow\n")
	sb.WriteString("6. **Project context**: any `**/project-context.md` file\n\n")

	sb.WriteString("### Step 2: Determine Next Epic Number\n\n")
	sb.WriteString("Count the existing epic entries (e.g., `epic-1`, `epic-2`) in sprint-status.yaml.\n")
	sb.WriteString("New epics will start from N+1 where N is the highest existing epic number.\n\n")

	sb.WriteString("### Step 3: Design New Epics\n\n")
	sb.WriteString("Based on the prime directive, retrospective insights, and remaining opportunities, design new epics.\n\n")
	sb.WriteString("**Prioritize these categories (in order):**\n")
	sb.WriteString("1. **Hardening** — stability, reliability, error handling, edge cases\n")
	sb.WriteString("2. **New Features** — user-facing capabilities aligned with the product vision\n")
	sb.WriteString("3. **Technical Debt** — code quality, refactoring, architecture improvements\n")
	sb.WriteString("4. **Performance** — speed, efficiency, scalability improvements\n")
	sb.WriteString("5. **Testing & QA** — improved coverage, automation, quality gates\n\n")
	sb.WriteString("**Epic Design Rules:**\n")
	sb.WriteString("- Each epic must deliver standalone user/system value (not just a technical layer)\n")
	sb.WriteString("- Each epic must have 3–7 stories\n")
	sb.WriteString("- Stories must be independent of future stories (no forward dependencies)\n")
	sb.WriteString("- Story IDs follow the pattern: `{epic-N}-{story-M}-{kebab-case-title}`\n\n")

	sb.WriteString("### Step 4: Write the New Epics Document\n\n")
	sb.WriteString("Determine the next available filename for epics (e.g., if `epics.md` exists, use `epics-phase-2.md`;\n")
	sb.WriteString("if `epics-phase-2.md` exists, use `epics-phase-3.md`, etc.).\n\n")
	sb.WriteString("Write the document to `_bmad-output/planning-artifacts/{next-epics-filename}.md`.\n\n")
	sb.WriteString("Use this structure for each epic:\n\n")
	sb.WriteString("```markdown\n")
	sb.WriteString("## Epic {N}: {Epic Title}\n\n")
	sb.WriteString("{Epic goal — what users/system will be able to do after this epic}\n\n")
	sb.WriteString("### Story {N}.{M}: {Story Title}\n\n")
	sb.WriteString("As a {user type},\n")
	sb.WriteString("I want {capability},\n")
	sb.WriteString("So that {value/benefit}.\n\n")
	sb.WriteString("**Acceptance Criteria:**\n\n")
	sb.WriteString("- Given {precondition} When {action} Then {expected outcome}\n")
	sb.WriteString("- [additional criteria as needed]\n")
	sb.WriteString("```\n\n")

	sb.WriteString("### Step 5: Update Sprint Status\n\n")
	sb.WriteString("Append the new epics and their stories to the EXISTING `_bmad-output/implementation-artifacts/sprint-status.yaml`.\n\n")
	sb.WriteString("**CRITICAL RULES for sprint-status.yaml:**\n")
	sb.WriteString("- Do NOT remove or modify any existing entries\n")
	sb.WriteString("- ONLY append new entries to the `development_status` section\n")
	sb.WriteString("- All new entries must start with status `backlog`\n")
	sb.WriteString("- Follow the exact existing format:\n\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("  epic-{N}: backlog\n")
	sb.WriteString("  {N}-1-{story-title-kebab}: backlog\n")
	sb.WriteString("  {N}-2-{story-title-kebab}: backlog\n")
	sb.WriteString("  # ... more stories ...\n")
	sb.WriteString("  epic-{N}-retrospective: backlog\n")
	sb.WriteString("```\n\n")
	sb.WriteString("- Story key format: `{epic-number}-{story-number}-{title-in-kebab-case}`\n")
	sb.WriteString("  Example: `3-1-add-user-authentication`, `3-2-implement-oauth-login`\n")
	sb.WriteString("- Each epic block must end with an `epic-{N}-retrospective: backlog` entry\n\n")

	sb.WriteString("### Step 6: Verify\n\n")
	sb.WriteString("After writing both files:\n")
	sb.WriteString("1. Re-read the updated `sprint-status.yaml` and confirm new entries are present\n")
	sb.WriteString("2. Confirm no existing entries were modified or removed\n")
	sb.WriteString("3. Confirm the new epics document exists and is well-formed\n")
	sb.WriteString("4. Print a summary: how many new epics and stories were created\n\n")

	sb.WriteString("---\n\n")
	sb.WriteString("## Output Summary Required\n\n")
	sb.WriteString("When complete, output a brief summary:\n")
	sb.WriteString("- Number of new epics created\n")
	sb.WriteString("- Number of new stories created\n")
	sb.WriteString("- Epic numbers used (e.g., Epic 3, Epic 4)\n")
	sb.WriteString("- Epic document written to\n")
	sb.WriteString("- Key focus areas addressed\n")

	return sb.String()
}

// DefaultMaxEpics is the default maximum number of new epics to generate per planning run.
const DefaultMaxEpics = 5
