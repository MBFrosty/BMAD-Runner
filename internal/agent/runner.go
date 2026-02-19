package agent

import (
	"bufio"
	"context"
	"encoding/json"
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

// scanTermLines is a bufio.SplitFunc that splits on \n, \r\n, or bare \r.
// This handles PTY output where agents use \r for in-place line updates.
func scanTermLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			return i + 1, data[:i], nil
		}
		if data[i] == '\r' {
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// readPTY reads from a PTY master, splitting on \n, \r\n, or bare \r,
// and pushes non-empty lines into buf.
func readPTY(r io.Reader, buf *lastLinesBuffer) {
	scanner := bufio.NewScanner(r)
	scanner.Split(scanTermLines)
	for scanner.Scan() {
		buf.push(scanner.Text())
	}
}

// --- claude-code stream-json parsing ---

type claudeEvent struct {
	Type    string     `json:"type"`
	Message *claudeMsg `json:"message,omitempty"`
}

type claudeMsg struct {
	Content []claudeBlock `json:"content"`
}

type claudeBlock struct {
	Type  string          `json:"type"` // "text" or "tool_use"
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// readStreamJSON reads JSONL from an agent's --output-format stream-json stdout,
// extracts human-readable status lines, and pushes them to buf.
func readStreamJSON(r io.Reader, buf *lastLinesBuffer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var ev claudeEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			continue
		}
		for _, line := range extractClaudeStatus(&ev) {
			buf.push(line)
		}
	}
}

func extractClaudeStatus(ev *claudeEvent) []string {
	if ev.Type != "assistant" || ev.Message == nil {
		return nil
	}
	var lines []string
	for _, block := range ev.Message.Content {
		switch block.Type {
		case "tool_use":
			lines = append(lines, formatToolUse(block))
		case "text":
			if last := lastNonEmptyLine(block.Text); last != "" {
				lines = append(lines, last)
			}
		}
	}
	return lines
}

func formatToolUse(block claudeBlock) string {
	var pathInput struct {
		Path string `json:"file_path"`
	}
	var cmdInput struct {
		Command string `json:"command"`
	}
	switch block.Name {
	case "Read", "read_file":
		if json.Unmarshal(block.Input, &pathInput) == nil && pathInput.Path != "" {
			return "Reading " + pathInput.Path
		}
	case "Edit", "edit_file":
		if json.Unmarshal(block.Input, &pathInput) == nil && pathInput.Path != "" {
			return "Editing " + pathInput.Path
		}
	case "Write", "write_file":
		if json.Unmarshal(block.Input, &pathInput) == nil && pathInput.Path != "" {
			return "Writing " + pathInput.Path
		}
	case "Bash", "execute_command":
		if json.Unmarshal(block.Input, &cmdInput) == nil && cmdInput.Command != "" {
			c := cmdInput.Command
			if len(c) > 50 {
				c = c[:47] + "..."
			}
			return "Running: " + c
		}
	case "Glob", "glob":
		return "Tool: Glob"
	case "Grep", "grep":
		return "Tool: Grep"
	}
	return "Tool: " + block.Name
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}

// --- main Run logic ---

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
			"--output-format", "stream-json",
			"--verbose",
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
			"--output-format", "stream-json",
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
		display := ui.NewPhaseDisplay(phase, lastLinesMax)

		if r.AgentType == "gemini-cli" {
			// gemini-cli: use PTY so the agent streams output in real time.
			ptmx, err := pty.Start(cmd)
			if err != nil {
				display.Fail()
				return fmt.Errorf("agent start failed for phase %s: %w", phase, err)
			}
			defer ptmx.Close()
			go readPTY(ptmx, buf)
		} else {
			// claude-code / cursor-agent: stream JSONL events via --output-format stream-json.
			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				display.Fail()
				return fmt.Errorf("stdout pipe: %w", err)
			}
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				display.Fail()
				return fmt.Errorf("stderr pipe: %w", err)
			}
			if err := cmd.Start(); err != nil {
				display.Fail()
				return fmt.Errorf("agent start failed for phase %s: %w", phase, err)
			}
			go readStreamJSON(stdoutPipe, buf)
			go readPipe(stderrPipe, nil, buf)
		}

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
