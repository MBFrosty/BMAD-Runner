package planner

import (
	"fmt"
	"os"
	"path/filepath"
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

// BuildEpicContext returns a small targeted preamble to prepend to the BMAD
// create-epics-and-stories command file. It scopes the BMAD workflow to adding
// ONE incremental epic for an existing in-progress project, starting at nextEpicNum.
//
// This is NOT a self-contained prompt — it is a context block prepended before the
// actual BMAD command content so the BMAD workflow executes with the right scope.
func BuildEpicContext(primeDirective string, nextEpicNum int) string {
	var sb strings.Builder

	sb.WriteString("# Incremental Epic Planning — Context (Read This First)\n\n")
	sb.WriteString("> **IMPORTANT**: This is an EXISTING in-progress project.\n")
	sb.WriteString("> You are NOT starting a new project from scratch.\n")
	sb.WriteString("> The BMAD workflow below is being used to add **one new epic** to an existing project.\n\n")

	sb.WriteString("## Your Specific Scope\n\n")
	sb.WriteString(fmt.Sprintf("- Plan **Epic %d only** (the next epic in sequence)\n", nextEpicNum))
	sb.WriteString("- **Append** this epic to the existing epics document — do NOT overwrite or rewrite existing epics\n")
	sb.WriteString("- Give this epic **3–7 stories** that are actionable and independently testable\n")
	sb.WriteString("- Skip strict prerequisite validation steps — use whatever documents already exist in the project\n")
	sb.WriteString("- If the workflow asks to design all epics, limit yourself to this one new epic only\n\n")

	if primeDirective != "" {
		sb.WriteString("## Prime Directive (Your Strategic Guide)\n\n")
		sb.WriteString("Use the following to determine the focus area for this new epic:\n\n")
		sb.WriteString(primeDirective)
		sb.WriteString("\n\n")
	}

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
