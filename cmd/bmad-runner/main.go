package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MBFrosty/BMAD-Runner/internal/agent"
	"github.com/MBFrosty/BMAD-Runner/internal/config"
	"github.com/MBFrosty/BMAD-Runner/internal/planner"
	"github.com/MBFrosty/BMAD-Runner/internal/status"
	"github.com/MBFrosty/BMAD-Runner/internal/ui"

	"github.com/pterm/pterm"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

func main() {
	commonFlags := []cli.Flag{
		&cli.StringFlag{
			Name:    "status-file",
			Aliases: []string{"s"},
			Usage:   "Path to sprint-status.yaml",
			Value:   "_bmad-output/implementation-artifacts/sprint-status.yaml",
		},
		&cli.StringFlag{
			Name:    "agent-path",
			Aliases: []string{"a"},
			Usage:   "Path to agent binary (lookup in PATH by default)",
		},
		&cli.StringFlag{
			Name:    "agent-type",
			Aliases: []string{"t"},
			Usage:   "Agent backend: cursor-agent, claude-code, or gemini-cli",
			Value:   "cursor-agent",
		},
		&cli.StringFlag{
			Name:    "project-root",
			Aliases: []string{"r"},
			Usage:   "Override project root (default: derived from status-file parent)",
		},
		&cli.StringFlag{
			Name:    "model",
			Aliases: []string{"m"},
			Usage:   "Model to use (default: composer-1.5 for cursor-agent, sonnet for claude-code)",
		},
		&cli.BoolFlag{
			Name:  "no-live-status",
			Usage: "Disable last-lines display in spinner (e.g. for CI/scripts)",
		},
	}

	app := &cli.App{
		Name:                   "bmad-runner",
		Usage:                  "Orchestrate BMAD workflow phases (create-story → dev-story → code-review) using cursor-agent, claude-code, or gemini-cli",
		UseShortOptionHandling: true,
		Commands: []*cli.Command{
			{
				Name:  "status",
				Usage: "Show current sprint status from YAML",
				Flags: commonFlags,
				Action: func(c *cli.Context) error {
					_, statusPath, err := config.ResolveProjectRoot(c.String("status-file"), c.String("project-root"))
					if err != nil {
						return fmt.Errorf("resolving project root: %w", err)
					}

					s, err := status.Parse(statusPath)
					if err != nil {
						return fmt.Errorf("parsing status file: %w", err)
					}

					pterm.DefaultHeader.WithFullWidth().Println("Sprint Status: " + s.Project)
					pterm.Info.Printf("File: %s\n", statusPath)
					pterm.Info.Printf("Generated: %s\n\n", s.Generated)

					tableData := pterm.TableData{
						{"Type", "Key", "Status"},
					}

					for k, v := range s.DevStatus {
						t := "Story"
						if strings.HasPrefix(k, "epic-") {
							t = "Epic"
						}
						tableData = append(tableData, []string{t, k, ui.StatusIcon(v)})
					}

					pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
					return nil
				},
			},
			{
				Name:  "run",
				Usage: "Run BMAD workflow phases",
				Flags: commonFlags,
				Subcommands: []*cli.Command{
					{
						Name:  "create-story",
						Usage: "Run create-story phase",
						Flags: commonFlags,
						Action: func(c *cli.Context) error {
							ui.PrintBanner()
							printWorkPlanFromContext(c)
							return runPhase(c, "create-story")
						},
					},
					{
						Name:  "dev-story",
						Usage: "Run dev-story phase",
						Flags: commonFlags,
						Action: func(c *cli.Context) error {
							ui.PrintBanner()
							printWorkPlanFromContext(c)
							return runPhase(c, "dev-story")
						},
					},
					{
						Name:  "code-review",
						Usage: "Run code-review phase",
						Flags: commonFlags,
						Action: func(c *cli.Context) error {
							ui.PrintBanner()
							printWorkPlanFromContext(c)
							return runPhase(c, "code-review")
						},
					},
					{
						Name:  "plan-epics",
						Usage: "Plan the next epic using the prime directive and update sprint status (targeted epic append + sprint-planning)",
						Flags: append(commonFlags,
							&cli.StringFlag{
								Name:  "prime-directive",
								Usage: "Path to the prime directive file (default: <project-root>/_bmad-output/prime-directive.md)",
							},
							&cli.IntFlag{
								Name:  "max-new-epics",
								Usage: "Maximum number of new epics to generate",
								Value: planner.DefaultMaxEpics,
							},
						),
						Action: func(c *cli.Context) error {
							ui.PrintBanner()
							return runPlanEpicsCommand(c)
						},
					},
					{
						Name:  "auto",
						Usage: "Loop through pending stories and epics until all done; run retrospective when epic completes",
						Flags: append(commonFlags,
							&cli.IntFlag{
								Name:  "max-iterations",
								Usage: "Maximum loop iterations (safety limit)",
								Value: 50,
							},
							&cli.BoolFlag{
								Name:  "no-pause-after-retro",
								Usage: "Do not prompt after retrospective; continue immediately (for scripts/CI)",
							},
							&cli.BoolFlag{
								Name:  "ignore-stall",
								Usage: "Continue even when sprint-status.yaml is unchanged after a story (avoids exit on workflow sync failures)",
							},
							&cli.BoolFlag{
								Name:  "enable-epic-planning",
								Usage: "When all stories are done, automatically plan the next epic via BMAD create-epics-and-stories + sprint-planning",
							},
							&cli.StringFlag{
								Name:  "prime-directive",
								Usage: "Path to the prime directive file that guides epic planning (default: <project-root>/_bmad-output/prime-directive.md)",
							},
							&cli.IntFlag{
								Name:  "max-new-epics",
								Usage: "Maximum number of new epics to plan across this auto session (one per no-work event)",
								Value: planner.DefaultMaxEpics,
							},
						),
						Action: func(c *cli.Context) error {
							ui.PrintBanner()
							return runAuto(c)
						},
					},
				},
				Action: func(c *cli.Context) error {
					ui.PrintBanner()
					_, statusPath, err := config.ResolveProjectRoot(c.String("status-file"), c.String("project-root"))
					if err != nil {
						return fmt.Errorf("resolving project root: %w", err)
					}
					s, err := status.Parse(statusPath)
					if err != nil {
						pterm.Warning.Printf("Could not parse sprint-status: %v — running full pipeline\n", err)
						printWorkPlanFromContext(c)
						return runFullPipeline(c)
					}
					action, epicKey, storyKey, found := s.NextWork()
					w := ui.WorkPlan{
						Project:    s.Project,
						StatusPath: statusPath,
					}
					if found {
						w.Action = action
						w.EpicKey = epicKey
						w.StoryKey = storyKey
						w.Done, w.Total = s.EpicProgress(epicKey)
					}
					ui.PrintWorkPlan(w)
					if !found {
						pterm.Success.Println("All work complete — nothing to run.")
						return nil
					}
					if action == "retrospective" {
						return runPhase(c, "retrospective")
					}
					return runFullPipeline(c)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printWorkPlanFromContext(c *cli.Context) {
	_, statusPath, err := config.ResolveProjectRoot(c.String("status-file"), c.String("project-root"))
	if err != nil {
		pterm.Warning.Printf("Could not resolve status file: %v\n", err)
		return
	}
	s, err := status.Parse(statusPath)
	if err != nil {
		pterm.Warning.Printf("Could not parse sprint-status: %v\n", err)
		return
	}
	action, epicKey, storyKey, found := s.NextWork()
	w := ui.WorkPlan{
		Project:    s.Project,
		StatusPath: statusPath,
	}
	if found {
		w.Action = action
		w.EpicKey = epicKey
		w.StoryKey = storyKey
		w.Done, w.Total = s.EpicProgress(epicKey)
	}
	ui.PrintWorkPlan(w)
}

func runFullPipeline(c *cli.Context) error {
	phases := []string{"create-story", "dev-story", "code-review"}
	for i, phase := range phases {
		ui.PrintPipeline(phases, i)
		if err := runPhase(c, phase); err != nil {
			pterm.Error.Printf("Phase %s failed: %v\n", phase, err)
			return err
		}
	}
	pterm.Success.Println("Full BMAD pipeline completed!")
	return nil
}

func runPhase(c *cli.Context, phase string) error {
	projectRoot, _, err := config.ResolveProjectRoot(c.String("status-file"), c.String("project-root"))
	if err != nil {
		return fmt.Errorf("resolving project root: %w", err)
	}

	agentType := resolveAgentType(c.String("agent-type"))
	agentPath, err := config.LookupAgent(c.String("agent-path"), agentType)
	if err != nil {
		return fmt.Errorf("looking up agent: %w", err)
	}

	model := c.String("model")
	if model == "" {
		model = defaultModelForAgentType(agentType, phase)
	}

	r := &agent.Runner{
		AgentPath:    agentPath,
		AgentType:    agentType,
		ProjectRoot:  projectRoot,
		NoLiveStatus: c.Bool("no-live-status") || !term.IsTerminal(int(os.Stdout.Fd())),
	}

	return r.Run(phase, model)
}

func runAuto(c *cli.Context) error {
	projectRoot, statusPath, err := config.ResolveProjectRoot(c.String("status-file"), c.String("project-root"))
	if err != nil {
		return fmt.Errorf("resolving project root: %w", err)
	}

	agentType := resolveAgentType(c.String("agent-type"))
	agentPath, err := config.LookupAgent(c.String("agent-path"), agentType)
	if err != nil {
		return fmt.Errorf("looking up agent: %w", err)
	}

	r := &agent.Runner{
		AgentPath:    agentPath,
		AgentType:    agentType,
		ProjectRoot:  projectRoot,
		NoLiveStatus: c.Bool("no-live-status") || !term.IsTerminal(int(os.Stdout.Fd())),
	}

	maxIter := c.Int("max-iterations")
	phases := []string{"create-story", "dev-story", "code-review"}
	ignoreStall := c.Bool("ignore-stall")
	enableEpicPlanning := c.Bool("enable-epic-planning")
	maxNewEpics := c.Int("max-new-epics")
	if maxNewEpics <= 0 {
		maxNewEpics = planner.DefaultMaxEpics
	}

	var lastStalledStory string
	var stallCount int
	// epicPlanningCount tracks how many new epics have been planned this session.
	// Each "no work found" event plans ONE epic (via one targeted BMAD invocation).
	// Stops when we reach maxNewEpics to prevent unbounded planning.
	epicPlanningCount := 0

	primeDirectivePath := c.String("prime-directive")
	if primeDirectivePath == "" {
		primeDirectivePath = filepath.Join(projectRoot, planner.DefaultPrimeDirectivePath)
	}

	for iter := 0; iter < maxIter; iter++ {
		statusData, err := os.ReadFile(statusPath)
		if err != nil {
			return fmt.Errorf("reading status file: %w", err)
		}

		s, err := status.Parse(statusPath)
		if err != nil {
			return fmt.Errorf("parsing status file: %w", err)
		}

		action, epicKey, storyKey, found := s.NextWork()
		if !found {
			if !enableEpicPlanning {
				pterm.Success.Println("All work complete!")
				return nil
			}
			if epicPlanningCount >= maxNewEpics {
				// Reached the session planning limit — pause for human review.
				ui.PrintEpicPlanningSessionComplete(epicPlanningCount, maxNewEpics)
				return nil
			}

			// Plan ONE new epic via two targeted BMAD invocations, then continue.
			nextEpicNum := s.NextEpicNumber()
			planErr := runOneEpicPlanning(c, r, statusPath, primeDirectivePath, agentType, nextEpicNum, statusData)
			switch planErr {
			case nil:
				// Sprint-status was updated — loop will pick up the new stories.
				epicPlanningCount++
				lastStalledStory = ""
				stallCount = 0
			case errEpicPlanningNoNewWork:
				// BMAD ran but didn't add anything new; stop with a summary.
				if epicPlanningCount > 0 {
					ui.PrintEpicPlanningSessionComplete(epicPlanningCount, maxNewEpics)
				} else {
					pterm.Success.Println("All work complete!")
				}
				return nil
			case errEpicPlanningPrimeDirectiveCreated:
				// Newly created prime directive — user must edit before continuing.
				pterm.Success.Println("All work complete — review the prime directive and re-run to enable epic planning.")
				return nil
			default:
				return planErr
			}
			continue
		}

		done, total := s.EpicProgress(epicKey)
		ui.PrintEpicProgress(epicKey, done, total)
		if action == "story" && storyKey != "" {
			pterm.Info.Printf("Story: %s\n", storyKey)
		}
		pterm.Println()

		if action == "retrospective" {
			pterm.DefaultSection.Printf("Epic %s complete — running retrospective", epicKey)

			phaseModel := c.String("model")
			if phaseModel == "" {
				phaseModel = defaultModelForAgentType(agentType, "retrospective")
			}

			if err := r.Run("retrospective", phaseModel); err != nil {
				pterm.Error.Printf("Retrospective failed: %v\n", err)
				return err
			}
			if !c.Bool("no-pause-after-retro") && term.IsTerminal(int(os.Stdin.Fd())) {
				pterm.Info.Println("Press Enter to continue to next epic...")
				bufio.NewReader(os.Stdin).ReadBytes('\n')
			}
			continue
		}

		// Run story pipeline
		for i, phase := range phases {
			ui.PrintPipeline(phases, i)

			phaseModel := c.String("model")
			if phaseModel == "" {
				phaseModel = defaultModelForAgentType(agentType, phase)
			}

			if err := r.Run(phase, phaseModel); err != nil {
				pterm.Error.Printf("Phase %s failed: %v\n", phase, err)
				return err
			}
		}

		pterm.Success.Printf("Story %s complete — continuing to next\n", storyKey)
		pterm.Println()

		// Stall detection: status file should change after run (code-review updates sprint-status)
		newData, err := os.ReadFile(statusPath)
		if err != nil {
			return fmt.Errorf("re-reading status file: %w", err)
		}
		if bytes.Equal(statusData, newData) {
			if storyKey == lastStalledStory {
				stallCount++
			} else {
				lastStalledStory = storyKey
				stallCount = 1
			}
			if !ignoreStall && stallCount >= 2 {
				return fmt.Errorf("stall detected: sprint-status.yaml unchanged after 2 runs for story %s — update it manually (mark story done) and run again", storyKey)
			}
			pterm.Warning.Printf("sprint-status.yaml unchanged — workflow may not have updated it. Continuing to next iteration.\n")
		} else {
			lastStalledStory = ""
			stallCount = 0
		}
	}

	return fmt.Errorf("max iterations (%d) reached", maxIter)
}

// runOneEpicPlanning handles one epic planning cycle when all current work is done.
//
// It runs two agent invocations in sequence:
//  1. Plan-epic (RunWithPrompt) — a targeted self-contained prompt that reads the prime
//     directive, existing epics, recent retrospective(s), and sprint-status, then appends
//     exactly ONE new epic in BMAD format to the existing epics document.
//     NOTE: We do NOT use the BMAD create-epics-and-stories command here — that workflow
//     is designed for initial project planning and regenerates ALL epics from the PRD.
//  2. sprint-planning — the real BMAD command that reads ALL epic files and rebuilds
//     sprint-status.yaml, preserving existing story statuses.
//
// Returns nil if new work was successfully staged, or a sentinel error for graceful exits.
func runOneEpicPlanning(
	c *cli.Context,
	r *agent.Runner,
	statusPath, primeDirectivePath, agentType string,
	nextEpicNum int,
	statusBefore []byte,
) error {
	pterm.DefaultSection.Printf("No stories remain — planning Epic %d via BMAD workflow", nextEpicNum)

	// Ensure the prime directive exists; create default if missing.
	created, err := planner.EnsurePrimeDirective(primeDirectivePath)
	if err != nil {
		pterm.Warning.Printf("Could not create prime directive: %v\n", err)
	}
	if created {
		pterm.Info.Printf("Created prime directive at: %s\n", primeDirectivePath)
		pterm.Info.Println("Review and edit it to guide epic planning, then re-run.")
		return errEpicPlanningPrimeDirectiveCreated
	}

	pdContent, err := planner.ReadPrimeDirective(primeDirectivePath)
	if err != nil {
		return fmt.Errorf("reading prime directive: %w", err)
	}

	if planner.IsDefaultPrimeDirective(pdContent) {
		pterm.Warning.Println("Prime directive appears to be unedited — results may be generic.")
		pterm.Info.Printf("Edit %s to describe your project goals.\n", primeDirectivePath)
	}

	maxNewEpics := c.Int("max-new-epics")
	ui.PrintEpicPlanningBanner(primeDirectivePath, nextEpicNum, maxNewEpics)

	// Discover project files to ground the planning context in actual project state.
	projectRoot, _, _ := config.ResolveProjectRoot(c.String("status-file"), c.String("project-root"))
	epicsFile := planner.FindEpicsFile(projectRoot)
	retroFiles := planner.FindRetroFiles(projectRoot, 2) // include at most last 2 retros

	// Collect the list of fully-completed epic keys so the agent knows what's already built.
	// Parse the status from statusBefore (the snapshot taken just before planning starts).
	var completedEpics []string
	if s, parseErr := status.ParseBytes(statusBefore); parseErr == nil {
		for k, v := range s.DevStatus {
			if strings.HasPrefix(k, "epic-") && !strings.Contains(k, "-retrospective") && v == "done" {
				completedEpics = append(completedEpics, k)
			}
		}
	}

	// --- Phase A: append one new epic via targeted RunWithPrompt ---
	// NOTE: We do NOT use the BMAD create-epics-and-stories command here.
	// That workflow is designed for initial project planning — it re-extracts ALL
	// requirements from the PRD and regenerates the entire epics document from scratch.
	// For incremental planning we need a targeted RunWithPrompt that:
	//   1. Reads the prime directive, existing epics, retros, and sprint-status for context
	//   2. Appends exactly ONE new epic in BMAD format without touching existing content
	// Phase B (sprint-planning) is still the real BMAD command for updating sprint-status.yaml.
	epicCtx := planner.EpicPlanningContext{
		PrimeDirective: pdContent,
		NextEpicNum:    nextEpicNum,
		EpicsFilePath:  epicsFile,
		RetroFilePaths: retroFiles,
		CompletedEpics: completedEpics,
		StatusFilePath: statusPath,
	}
	epicPrompt := planner.BuildEpicPlanningPrompt(epicCtx)
	epicModel := c.String("model")
	if epicModel == "" {
		epicModel = defaultModelForAgentType(agentType, "plan-epic")
	}

	pterm.Info.Printf("Phase A: appending Epic %d to epics document\n", nextEpicNum)
	if err := r.RunWithPrompt(epicPrompt, "plan-epic", epicModel); err != nil {
		pterm.Error.Printf("epic planning failed: %v\n", err)
		return err
	}

	// --- Phase B: sprint-planning to update sprint-status.yaml ---
	// IMPORTANT: epics.md is the source of truth in this flow.
	// Phase A writes new epics to epics.md; Phase B rebuilds sprint-status.yaml from it.
	//
	// This is intentionally different from correct-course, which writes directly to
	// sprint-status.yaml (checklist 6.4) and bypasses epics.md. If a user has run
	// correct-course manually outside this flow, those sprint-status entries will be
	// overwritten here unless the corresponding epics are also present in epics.md.
	// The fix is to ensure any manually-created epics are in epics.md before running auto.
	sprintModel := c.String("model")
	if sprintModel == "" {
		sprintModel = defaultModelForAgentType(agentType, "sprint-planning")
	}

	pterm.Info.Println("Phase B: sprint-planning (updating sprint-status.yaml)")
	if err := r.Run("sprint-planning", sprintModel); err != nil {
		pterm.Error.Printf("sprint-planning failed: %v\n", err)
		return err
	}

	// Check whether sprint-status.yaml was actually updated.
	statusAfter, err := os.ReadFile(statusPath)
	if err != nil {
		return fmt.Errorf("re-reading status file after sprint-planning: %w", err)
	}
	if bytes.Equal(statusBefore, statusAfter) {
		pterm.Warning.Println("sprint-planning did not update sprint-status.yaml — no new stories detected.")
		return errEpicPlanningNoNewWork
	}

	pterm.Success.Printf("Epic %d staged — continuing auto loop.\n", nextEpicNum)
	pterm.Println()

	// Pause for review in interactive terminals (reuse --no-pause-after-retro flag).
	if !c.Bool("no-pause-after-retro") && term.IsTerminal(int(os.Stdin.Fd())) {
		pterm.Info.Printf("Review the planned epic, then press Enter to begin development...\n")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	return nil
}

// runPlanEpicsCommand is the action for `bmad-runner run plan-epics`.
// Plans the next epic standalone (without the auto loop).
func runPlanEpicsCommand(c *cli.Context) error {
	projectRoot, statusPath, err := config.ResolveProjectRoot(c.String("status-file"), c.String("project-root"))
	if err != nil {
		return fmt.Errorf("resolving project root: %w", err)
	}

	agentType := resolveAgentType(c.String("agent-type"))
	agentPath, err := config.LookupAgent(c.String("agent-path"), agentType)
	if err != nil {
		return fmt.Errorf("looking up agent: %w", err)
	}

	r := &agent.Runner{
		AgentPath:    agentPath,
		AgentType:    agentType,
		ProjectRoot:  projectRoot,
		NoLiveStatus: c.Bool("no-live-status") || !term.IsTerminal(int(os.Stdout.Fd())),
	}

	primeDirectivePath := c.String("prime-directive")
	if primeDirectivePath == "" {
		primeDirectivePath = filepath.Join(projectRoot, planner.DefaultPrimeDirectivePath)
	}

	statusData, err := os.ReadFile(statusPath)
	if err != nil {
		return fmt.Errorf("reading status file: %w", err)
	}

	s, err := status.Parse(statusPath)
	if err != nil {
		return fmt.Errorf("parsing status file: %w", err)
	}

	nextEpicNum := s.NextEpicNumber()
	planErr := runOneEpicPlanning(c, r, statusPath, primeDirectivePath, agentType, nextEpicNum, statusData)
	switch planErr {
	case nil:
		pterm.Success.Printf("Epic %d planning complete.\n", nextEpicNum)
	case errEpicPlanningPrimeDirectiveCreated, errEpicPlanningNoNewWork:
		// Graceful exit — message already printed.
	default:
		return planErr
	}
	return nil
}

// Sentinel errors for runOneEpicPlanning — treated as graceful exits (exit 0).
var (
	// errEpicPlanningNoNewWork is returned when both BMAD phases ran but sprint-status
	// was not updated (e.g. the agent didn't find/add anything new).
	errEpicPlanningNoNewWork = fmt.Errorf("epic planning: no new work added to sprint-status")
	// errEpicPlanningPrimeDirectiveCreated is returned when a default prime directive
	// was just created and the user must review it before planning can run.
	errEpicPlanningPrimeDirectiveCreated = fmt.Errorf("prime directive created: review and re-run")
)

func resolveAgentType(s string) string {
	switch s {
	case config.AgentTypeClaudeCode:
		return config.AgentTypeClaudeCode
	case config.AgentTypeGeminiCLI:
		return config.AgentTypeGeminiCLI
	default:
		return config.AgentTypeCursorAgent
	}
}

func defaultModelForAgentType(agentType string, phase string) string {
	switch agentType {
	case config.AgentTypeClaudeCode:
		switch phase {
		case "create-story":
			return "opus"
		case "dev-story":
			return "haiku"
		case "code-review", "retrospective":
			return "sonnet"
		// Epic planning uses the most capable model: creating good epics saves
		// many dev cycles later.
		case "plan-epic", "sprint-planning":
			return "opus"
		default:
			return "sonnet"
		}
	case config.AgentTypeGeminiCLI:
		switch phase {
		case "create-story":
			return "gemini-3-pro"
		case "dev-story":
			return "gemini-3-flash"
		case "code-review", "retrospective":
			return "gemini-3-pro"
		case "plan-epic", "sprint-planning":
			return "gemini-3-pro"
		default:
			return "gemini-3-pro"
		}
	default: // CursorAgent
		switch phase {
		case "create-story":
			return "claude-4.6-sonnet-medium"
		case "dev-story":
			return "composer-1.5"
		case "code-review", "retrospective":
			return "gemini-3-flash"
		case "plan-epic", "sprint-planning":
			return "claude-4.6-sonnet-medium"
		default:
			return "composer-1.5"
		}
	}
}
