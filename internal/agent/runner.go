package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MBFrosty/BMAD-Runner/internal/ui"
	"github.com/creack/pty"
	"github.com/pterm/pterm"
)

const (
	lastLinesMax  = 3
	frameInterval = 80 * time.Millisecond
)

// Runner orchestrates cursor-agent or claude-code invocations
type Runner struct {
	AgentPath    string
	AgentType    string
	ProjectRoot  string
	Model        string
	NoLiveStatus bool // disable last-lines display in spinner (e.g. CI, --no-live-status)
}

// lastLinesBuffer is a thread-safe rolling buffer of the last N lines.
type lastLinesBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func (b *lastLinesBuffer) push(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.max {
		b.lines = b.lines[len(b.lines)-b.max:]
	}
}

func (b *lastLinesBuffer) get() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// readPipe reads from r, tees each line to w (if non-nil), and pushes complete lines to buf.
func readPipe(r io.Reader, w io.Writer, buf *lastLinesBuffer) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if w != nil {
			fmt.Fprintln(w, line)
		}
		buf.push(line)
	}
}

// Run executes a BMAD workflow phase (create-story, dev-story, code-review)
func (r *Runner) Run(phase string) error {
	// 1. Read command file
	commandFile := filepath.Join(r.ProjectRoot, ".cursor", "commands", fmt.Sprintf("bmad-bmm-%s.md", phase))
	data, err := os.ReadFile(commandFile)
	if err != nil {
		return fmt.Errorf("reading command file %s: %w", commandFile, err)
	}

	// 2. Build prompt with yolo preamble
	prompt := buildYoloPrompt(string(data))

	var cmd *exec.Cmd
	switch r.AgentType {
	case "claude-code":
		cmd = exec.Command(r.AgentPath,
			"-p",
			"--model", r.Model,
			"--dangerously-skip-permissions",
			prompt,
		)
	case "gemini-cli":
		cmd = exec.Command(r.AgentPath,
			"--approval-mode", "yolo",
			"--model", r.Model,
			"-p",
			prompt,
		)
	default:
		cmd = exec.Command(r.AgentPath,
			"-p",
			"-f",
			"--approve-mcps",
			"--model", r.Model,
			"--workspace", r.ProjectRoot,
			prompt,
		)
	}

	cmd.Dir = r.ProjectRoot

	pterm.DefaultSection.Printf("BMAD Workflow: %s", strings.ReplaceAll(phase, "-", " "))
	pterm.Info.Printf("Project Root: %s\n", r.ProjectRoot)
	pterm.Info.Printf("Agent:        %s\n", r.AgentType)
	pterm.Info.Printf("Model:        %s\n", r.Model)

	buf := &lastLinesBuffer{max: lastLinesMax}

	var runErr error

	if r.NoLiveStatus {
		// CI/script mode: pipe output directly to terminal, plain spinner for progress.
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("stdout pipe: %w", err)
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("stderr pipe: %w", err)
		}
		go readPipe(stdoutPipe, os.Stdout, buf)
		go readPipe(stderrPipe, os.Stderr, buf)

		spinner, _ := ui.NewPhaseSpinner().Start(fmt.Sprintf("Executing %s...", phase))
		if err := cmd.Start(); err != nil {
			spinner.Fail(fmt.Sprintf("Phase %s failed", phase))
			return fmt.Errorf("agent start failed for phase %s: %w", phase, err)
		}
		runErr = cmd.Wait()
		if runErr != nil {
			spinner.Fail(fmt.Sprintf("Phase %s failed", phase))
		} else {
			spinner.Success(fmt.Sprintf("Phase %s completed", phase))
		}
	} else {
		// Live mode: start the agent under a PTY so it thinks it's in a terminal
		// and streams output in real time. Read PTY output into the rolling buffer.
		display := ui.NewPhaseDisplay(phase, lastLinesMax)

		ptmx, err := pty.Start(cmd)
		if err != nil {
			display.Fail()
			return fmt.Errorf("agent start failed for phase %s: %w", phase, err)
		}
		defer ptmx.Close()

		go readPipe(ptmx, nil, buf)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			ticker := time.NewTicker(frameInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					display.Tick(buf.get())
				}
			}
		}()

		runErr = cmd.Wait()
		cancel()

		if runErr != nil {
			display.Fail()
		} else {
			display.Success()
		}
	}

	if runErr != nil {
		return fmt.Errorf("agent execution failed for phase %s: %w", phase, runErr)
	}
	return nil
}

func buildYoloPrompt(commandContent string) string {
	var sb strings.Builder
	sb.WriteString("Execute the following BMAD workflow. CRITICAL: Run in #yolo mode from the start.\n")
	sb.WriteString("- Accept ALL BMAD suggestions without asking\n")
	sb.WriteString("- Skip all confirmations and elicitation\n")
	sb.WriteString("- Proceed automatically through every step\n")
	sb.WriteString("- Simulate expert user responses (y/continue) for any prompts\n\n")
	sb.WriteString(commandContent)
	return sb.String()
}
