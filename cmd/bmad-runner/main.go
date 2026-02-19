package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/MBFrosty/BMAD-Runner/internal/agent"
	"github.com/MBFrosty/BMAD-Runner/internal/config"
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
	var lastStalledStory string
	var stallCount int

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
			pterm.Success.Println("All work complete!")
			return nil
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
			return "opus" // bigger expensive model
		case "dev-story":
			return "haiku" // light fast model
		case "code-review", "retrospective":
			return "sonnet" // medium model
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
		default:
			return "gemini-3-pro"
		}
	default: // CursorAgent
		switch phase {
		case "create-story":
			return "gemini-3.1-pro-preview"
		case "dev-story":
			return "composer-1.5"
		case "code-review", "retrospective":
			return "gemini-3-flash-preview"
		default:
			return "composer-1.5"
		}
	}
}
