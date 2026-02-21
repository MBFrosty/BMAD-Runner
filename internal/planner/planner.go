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
	} else if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("checking prime directive: %w", err)
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
	matches, err := filepath.Glob(filepath.Join(projectRoot, "_bmad-output", "planning-artifacts", "*epic*.md"))
	if err != nil {
		return ""
	}
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
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	if maxFiles > 0 && len(matches) > maxFiles {
		return matches[:maxFiles]
	}
	return matches
}

// EpicPlanningContext holds all inputs for BuildCorrectCourseContext.
type EpicPlanningContext struct {
	// PrimeDirective is the content of the prime directive file (may be empty).
	PrimeDirective string

	// NextEpicNum is the sequential number of the epic to plan (e.g. 3 means "plan Epic 3").
	NextEpicNum int

	// EpicsFilePath is the path to the existing epics document.
	EpicsFilePath string

	// RetroFilePaths are paths to recent retrospective files (newest first).
	RetroFilePaths []string

	// CompletedEpics is the list of epic keys that are fully done (e.g. ["epic-1", "epic-2"]).
	CompletedEpics []string

	// StatusFilePath is the path to sprint-status.yaml.
	StatusFilePath string

	// ProjectRoot is the absolute path to the project root directory.
	// Used by BuildFeatureProposalPrompt to construct the proposal output path.
	ProjectRoot string

	// FeatureProposal is the output of the Feature Scout step (may be empty).
	// When non-empty, it is used as the concrete change trigger for correct-course,
	// replacing the generic "all work complete, plan Epic N" trigger.
	FeatureProposal string
}

// FeatureProposalOutputPath returns the standard path for the Feature Scout output file.
// The file is written by the agent during the feature scout step and read back by the runner.
func FeatureProposalOutputPath(projectRoot string, epicNum int) string {
	return filepath.Join(projectRoot, "_bmad-output", fmt.Sprintf("feature-proposal-epic-%d.md", epicNum))
}

// ReadFeatureProposal reads the Feature Scout output file and returns its content.
// Returns empty string if the file does not exist (no proposal generated).
func ReadFeatureProposal(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading feature proposal: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// BuildFeatureProposalPrompt returns a self-contained agent prompt for the Feature Scout step.
//
// The Feature Scout runs BEFORE correct-course. Its job is to analyze the project's
// current state (what has been built, what the prime directive calls for, lessons from
// retrospectives) and produce a concrete feature brief for the next epic.
//
// By separating feature ideation from epic planning we give correct-course a precise
// change trigger — a specific named feature with a problem statement and scope — rather
// than asking it to simultaneously decide what to build AND plan how to build it.
//
// The scout writes its proposal to a known file path that the runner reads back and
// injects into the correct-course context.
func BuildFeatureProposalPrompt(ctx EpicPlanningContext) string {
	var sb strings.Builder

	outputPath := FeatureProposalOutputPath(ctx.ProjectRoot, ctx.NextEpicNum)

	sb.WriteString("# Feature Scout — Autonomous Epic Proposal\n\n")
	sb.WriteString("You are a product strategist performing a project analysis to propose the single most valuable ")
	sb.WriteString(fmt.Sprintf("next feature to build as **Epic %d**.\n\n", ctx.NextEpicNum))

	sb.WriteString("## Step 1 — Read the Project Context\n\n")
	sb.WriteString("Read the following files in full before forming any opinions:\n\n")

	if ctx.PrimeDirective != "" {
		sb.WriteString("### Prime Directive\n\n")
		sb.WriteString("The prime directive below is the strategic compass for this project. ")
		sb.WriteString("Your proposal MUST align with it.\n\n")
		sb.WriteString(ctx.PrimeDirective)
		sb.WriteString("\n\n")
	}

	sb.WriteString("### Files to Read\n\n")
	if ctx.EpicsFilePath != "" {
		sb.WriteString(fmt.Sprintf("- **Existing Epics**: `%s`\n", ctx.EpicsFilePath))
		sb.WriteString("  Understand what has already been planned/built so you don't propose a duplicate.\n")
	}
	if len(ctx.RetroFilePaths) > 0 {
		sb.WriteString("- **Recent Retrospective(s)**:\n")
		for _, p := range ctx.RetroFilePaths {
			sb.WriteString(fmt.Sprintf("  - `%s`\n", p))
		}
		sb.WriteString("  Apply lessons learned and any next-epic recommendations from the team.\n")
	}
	sb.WriteString("- **PRD and Architecture**: Search `_bmad-output/planning-artifacts/` for the PRD, architecture, and UX documents.\n")
	sb.WriteString("  Understand the product vision, technical constraints, and existing design.\n\n")

	if len(ctx.CompletedEpics) > 0 {
		sb.WriteString(fmt.Sprintf("- Already completed epics: %s\n\n", strings.Join(ctx.CompletedEpics, ", ")))
	}

	sb.WriteString("## Step 2 — Select One Feature to Propose\n\n")
	sb.WriteString("After reading the above, choose **one** feature for Epic ")
	sb.WriteString(fmt.Sprintf("%d", ctx.NextEpicNum))
	sb.WriteString(". Use these criteria:\n\n")
	sb.WriteString("- Directly advances the prime directive's goals\n")
	sb.WriteString("- Provides concrete value to users (not purely internal/technical)\n")
	sb.WriteString("- Fits naturally within the existing architecture\n")
	sb.WriteString("- Is not a duplicate of any already-planned or completed epic\n")
	sb.WriteString("- Applies lessons from retrospectives where relevant\n\n")

	sb.WriteString("## Step 3 — Write the Feature Proposal File\n\n")
	sb.WriteString(fmt.Sprintf("Write your proposal to exactly this path: `%s`\n\n", outputPath))
	sb.WriteString("Use this exact markdown structure:\n\n")
	sb.WriteString("```markdown\n")
	sb.WriteString(fmt.Sprintf("# Epic %d Feature Proposal\n\n", ctx.NextEpicNum))
	sb.WriteString("## Feature Name\n\n")
	sb.WriteString("(A short, descriptive name for the feature)\n\n")
	sb.WriteString("## Problem Statement\n\n")
	sb.WriteString("(What gap or pain point does this address? Who benefits and how?)\n\n")
	sb.WriteString("## Proposed Solution\n\n")
	sb.WriteString("(High-level description of what will be built and how it fits the architecture)\n\n")
	sb.WriteString("## Key Capabilities\n\n")
	sb.WriteString("- (3–6 concrete capabilities or behaviours this feature delivers)\n\n")
	sb.WriteString("## Why This Epic Now\n\n")
	sb.WriteString("(Why is this the right next step? Reference the prime directive and any retro findings.)\n\n")
	sb.WriteString("## Suggested Epic Title\n\n")
	sb.WriteString("(A concise title suitable for use in epics.md and sprint-status.yaml)\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Constraints\n\n")
	sb.WriteString("- Propose exactly ONE feature — do not list alternatives\n")
	sb.WriteString("- Do NOT modify sprint-status.yaml, epics.md, or any other project file\n")
	sb.WriteString("- Your only output file is the proposal at the path above\n")
	sb.WriteString("- After writing the file, respond with: `Feature proposal complete for Epic ")
	sb.WriteString(fmt.Sprintf("%d", ctx.NextEpicNum))
	sb.WriteString("`\n")

	return sb.String()
}

// BuildCorrectCourseContext returns a context preamble to prepend to the BMAD
// correct-course command file for automated incremental epic planning.
//
// correct-course is the proper BMAD "anytime" workflow for adding new epics to an
// existing in-progress project (per the BMAD help catalog). It reads all project
// documents (PRD, epics, architecture) and updates sprint-status.yaml with the
// new epic/story entries via checklist item 6.4.
//
// The preamble provides the "change trigger" and prime directive that the workflow
// needs to proceed autonomously in yolo mode without halting for human input.
// It also instructs the agent to update both sprint-status.yaml AND the epics
// document so both sources stay in sync.
//
// Note: we do NOT run sprint-planning after correct-course. correct-course writes
// to sprint-status.yaml directly; running sprint-planning afterward would overwrite
// those entries by re-deriving from epics.md.
func BuildCorrectCourseContext(ctx EpicPlanningContext) string {
	var sb strings.Builder

	sb.WriteString("# Automated Epic Planning — Correct Course Session\n\n")

	// --- Change trigger ---
	// correct-course Step 1 asks for the change trigger. We provide it here so
	// the workflow doesn't halt waiting for user input.
	//
	// When a Feature Scout proposal is available we use it as the precise change
	// trigger so correct-course has a concrete feature brief to plan against.
	// Without it we fall back to the generic "all work complete, plan Epic N" trigger.
	sb.WriteString("## Change Trigger (Required by Correct Course Step 1)\n\n")
	if ctx.FeatureProposal != "" {
		// Feature Scout ran successfully — use its output as the authoritative trigger.
		if ctx.NextEpicNum > 1 && len(ctx.CompletedEpics) > 0 {
			sb.WriteString(fmt.Sprintf(
				"All planned work is complete (%s are done). ",
				strings.Join(ctx.CompletedEpics, ", "),
			))
		} else {
			sb.WriteString("All currently planned work is complete. ")
		}
		sb.WriteString(fmt.Sprintf(
			"A Feature Scout analysis has identified the following feature to build as **Epic %d**:\n\n",
			ctx.NextEpicNum,
		))
		sb.WriteString("---\n\n")
		sb.WriteString(ctx.FeatureProposal)
		sb.WriteString("\n\n---\n\n")
		sb.WriteString("Use the feature proposal above as your precise change trigger and planning scope. ")
		sb.WriteString("This is proactive forward planning, not a bug fix or mid-sprint pivot.\n\n")
	} else {
		// No Feature Scout output — fall back to generic trigger guided by prime directive.
		if ctx.NextEpicNum > 1 && len(ctx.CompletedEpics) > 0 {
			sb.WriteString(fmt.Sprintf(
				"All planned work is complete (%s are done). ",
				strings.Join(ctx.CompletedEpics, ", "),
			))
		} else {
			sb.WriteString("All currently planned work is complete. ")
		}
		sb.WriteString(fmt.Sprintf(
			"We need to plan and add **Epic %d** — the next phase of development.\n\n",
			ctx.NextEpicNum,
		))
		sb.WriteString("This is proactive forward planning, not a bug fix or mid-sprint pivot.\n\n")
	}

	// --- Pre-flight: extra files to read ---
	// correct-course Step 0.5 loads standard docs (PRD, architecture, UX).
	// We direct it to also read these files the workflow wouldn't otherwise prioritize.
	hasExtra := ctx.EpicsFilePath != "" || len(ctx.RetroFilePaths) > 0
	if hasExtra {
		sb.WriteString("## Additional Context Files (Read in Step 0.5)\n\n")
		sb.WriteString("In addition to the standard PRD/architecture/UX documents, also read:\n\n")
		if ctx.EpicsFilePath != "" {
			sb.WriteString(fmt.Sprintf("- **Existing Epics**: `%s`\n", ctx.EpicsFilePath))
			sb.WriteString("  Understand what's already planned so the new epic doesn't duplicate it.\n")
		}
		if len(ctx.RetroFilePaths) > 0 {
			sb.WriteString("- **Recent Retrospective(s)**:\n")
			for _, p := range ctx.RetroFilePaths {
				sb.WriteString(fmt.Sprintf("  - `%s`\n", p))
			}
			sb.WriteString("  Apply lessons learned and recommended next-epic priorities.\n")
		}
		sb.WriteString("\n")
	}

	// --- Prime directive ---
	if ctx.PrimeDirective != "" {
		sb.WriteString("## Prime Directive (Strategic Guide for Epic Focus)\n\n")
		sb.WriteString("Use the following to determine the theme and focus of the new epic:\n\n")
		sb.WriteString(ctx.PrimeDirective)
		sb.WriteString("\n\n")
	}

	// --- Autonomous execution guidance ---
	// Pre-answer the interactive steps so the workflow doesn't halt.
	sb.WriteString("## Autonomous Execution Instructions\n\n")
	sb.WriteString("You are running in automated mode — no human is available to respond to prompts.\n")
	sb.WriteString("Proceed through the correct-course workflow with these pre-decisions:\n\n")
	sb.WriteString(fmt.Sprintf("- **Change trigger**: All planned work is complete; add Epic %d\n", ctx.NextEpicNum))
	sb.WriteString("- **Mode**: Batch (present all changes at once, then execute immediately)\n")
	sb.WriteString("- **Scope**: Direct Adjustment — add new epic and stories only; do NOT modify existing epics\n")
	sb.WriteString("- **Approval**: Self-approve the Sprint Change Proposal (no human available to approve)\n")
	sb.WriteString("- **Epic theme**: Let the prime directive above guide the focus area\n\n")

	// --- Required outputs ---
	// Explicitly require both sprint-status and epics updates to keep them in sync.
	sb.WriteString("## Required Outputs (Both Are Mandatory)\n\n")
	sb.WriteString(fmt.Sprintf(
		"1. **Update `sprint-status.yaml`** (checklist 6.4): Add Epic %d and its stories with status `backlog`\n",
		ctx.NextEpicNum,
	))
	epicsTarget := ctx.EpicsFilePath
	if epicsTarget == "" {
		epicsTarget = "_bmad-output/planning-artifacts/epics.md"
	}
	sb.WriteString(fmt.Sprintf(
		"2. **Append to epics document** (`%s`): Add the new epic in BMAD format\n",
		epicsTarget,
	))
	sb.WriteString("\nComplete both before finishing. Do not skip either output.\n\n")

	sb.WriteString("---\n\n")
	sb.WriteString("Now load and execute the BMAD Correct Course workflow below:\n\n")

	return sb.String()
}
