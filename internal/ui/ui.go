package ui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"atomicgo.dev/cursor"
	"github.com/pterm/pterm"
)

// ansiEscape strips ANSI escape sequences (e.g. \033[31m) from a string.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// FormatLastLineForStatus truncates and sanitizes a line for display in the spinner.
// Strips ANSI codes and control chars, truncates to statusTruncate chars.
func FormatLastLineForStatus(line string) string {
	line = ansiEscape.ReplaceAllString(line, "")
	line = strings.ReplaceAll(line, "\r", " ")
	line = strings.ReplaceAll(line, "\n", " ")
	line = strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' {
			return -1
		}
		return r
	}, line)
	line = strings.TrimSpace(line)
	if len(line) > statusTruncate {
		return line[:statusTruncate-3] + "..."
	}
	return line
}

const statusTruncate = 60

const (
	barWidth  = 20
	blockSize = 3
)

var brailleChars = []rune{'⣾', '⣷', '⣯', '⣟', '⡿', '⢿', '⣻', '⣽'}

// generateBounceFrames produces frames for a bouncing block across a bar,
// with a synchronized braille spinner. Each frame: "⣾  ▓▓▓░░░░...   ".
func generateBounceFrames() []string {
	// Bounce: 0→17, then 16→0, repeat
	positions := make([]int, 0, 36)
	for i := 0; i <= barWidth-blockSize; i++ {
		positions = append(positions, i)
	}
	for i := barWidth - blockSize - 1; i >= 1; i-- {
		positions = append(positions, i)
	}

	frames := make([]string, 0, len(positions))
	for i, pos := range positions {
		braille := brailleChars[i%len(brailleChars)]
		bar := make([]rune, barWidth)
		for j := 0; j < barWidth; j++ {
			if j >= pos && j < pos+blockSize {
				bar[j] = '▓'
			} else {
				bar[j] = '░'
			}
		}
		frames = append(frames, string(braille)+"  "+string(bar))
	}
	return frames
}

// NewPhaseSpinner returns a SpinnerPrinter configured with the bounce animation
// and 80ms delay. Call .Start(label) / .Success() / .Fail() as usual.
func NewPhaseSpinner() *pterm.SpinnerPrinter {
	frames := generateBounceFrames()
	return pterm.DefaultSpinner.
		WithSequence(frames...).
		WithDelay(80 * time.Millisecond).
		WithShowTimer(false)
}

// PhaseDisplay renders the bounce animation and a rolling log preview in-place
// using atomicgo/cursor Area (no terminal scrolling). Use Tick to advance the frame
// and refresh log lines; call Success or Fail when the phase ends.
type PhaseDisplay struct {
	area         cursor.Area
	frames       []string
	phase        string
	frameIdx     int
	logLineCount int
	mu           sync.Mutex
	active       bool
}

// NewPhaseDisplay starts an in-place live area for the given phase.
// logLines controls how many preview lines are shown below the animation.
func NewPhaseDisplay(phase string, logLines int) *PhaseDisplay {
	return &PhaseDisplay{
		area:         cursor.NewArea(),
		frames:       generateBounceFrames(),
		phase:        phase,
		logLineCount: logLines,
		active:       true,
	}
}

// Tick advances the animation by one frame and redraws with the provided log lines.
// Serialized to prevent overlapping updates.
func (d *PhaseDisplay) Tick(logLines []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return
	}
	frame := d.frames[d.frameIdx%len(d.frames)]
	d.frameIdx++
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s  Executing %s...\n", frame, d.phase))
	for i := 0; i < d.logLineCount; i++ {
		if i < len(logLines) {
			sb.WriteString(fmt.Sprintf("  │ %s\n", FormatLastLineForStatus(logLines[i])))
		} else {
			sb.WriteString("  │\n")
		}
	}
	d.area.Update(sb.String())
}

// Success stops the area and prints a success message.
func (d *PhaseDisplay) Success() {
	d.mu.Lock()
	d.active = false
	d.area.Clear()
	d.mu.Unlock()
	pterm.Success.Printf("Phase %s completed\n", d.phase)
}

// Fail stops the area and prints a failure message.
func (d *PhaseDisplay) Fail() {
	d.mu.Lock()
	d.active = false
	d.area.Clear()
	d.mu.Unlock()
	pterm.Error.Printf("Phase %s failed\n", d.phase)
}

// PrintBanner prints the BMAD RUNNER ASCII banner and tagline.
func PrintBanner() {
	pterm.DefaultBigText.WithLetters(
		pterm.NewLettersFromStringWithStyle("BMAD", pterm.NewStyle(pterm.FgWhite, pterm.Bold)),
		pterm.NewLettersFromStringWithStyle("RUNNER", pterm.NewStyle(pterm.FgCyan, pterm.Bold)),
	).Render()
	pterm.Println()
	pterm.Println(pterm.Gray("Orchestrate BMAD workflow phases"))
	pterm.Println()
}

// PrintPipeline prints the phase pipeline with current phase highlighted.
func PrintPipeline(phases []string, currentIdx int) {
	for i, phase := range phases {
		display := strings.ReplaceAll(phase, "-", " ")
		switch {
		case i < currentIdx:
			pterm.Println(pterm.Green("  ✔  " + display))
		case i == currentIdx:
			pterm.Println(pterm.Cyan("  ▶  " + display))
		default:
			pterm.Println(pterm.Gray("  ○  " + display))
		}
	}
	pterm.Println()
}

// PrintEpicProgress prints the current epic and story progress for the auto loop.
func PrintEpicProgress(epicKey string, storiesDone, storiesTotal int) {
	if storiesTotal > 0 {
		pterm.DefaultSection.Printf("Epic %s — stories %d/%d", epicKey, storiesDone, storiesTotal)
	} else {
		pterm.DefaultSection.Printf("Epic %s", epicKey)
	}
	pterm.Println()
}

// PrintEpicPlanningBanner prints a banner before a single-epic automated planning run.
// nextEpicNum is the epic number being planned; sessionCap is the max for this session.
func PrintEpicPlanningBanner(primeDirectivePath string, nextEpicNum, sessionCap int) {
	pterm.DefaultSection.Println("Automated Epic Planning")
	pterm.Info.Printf("Planning:     Epic %d\n", nextEpicNum)
	pterm.Info.Printf("Session cap:  %d epic(s) max\n", sessionCap)
	pterm.Info.Printf("Prime directive: %s\n", primeDirectivePath)
	pterm.Println()
	pterm.Println(pterm.Cyan("  ◈  Phase A: create-epics-and-stories (one new epic)"))
	pterm.Println(pterm.Cyan("  ◈  Phase B: sprint-planning (update sprint-status.yaml)"))
	pterm.Println()
}

// PrintEpicPlanningSessionComplete prints a human-in-the-loop pause message after the
// runner reaches its epic planning limit for this session.
func PrintEpicPlanningSessionComplete(epicCount, sessionCap int) {
	pterm.Println()
	pterm.DefaultHeader.WithFullWidth().Println("Epic Planning Session Complete — Human Review Requested")
	pterm.Println()
	pterm.Success.Printf("Planned %d new epic(s) this session (limit: %d).\n", epicCount, sessionCap)
	pterm.Info.Println("The runner is pausing here so you can review what was planned.")
	pterm.Println()
	pterm.Println(pterm.Cyan("  Next steps:"))
	pterm.Println(pterm.Gray("  1. Review the newly planned epics in _bmad-output/planning-artifacts/"))
	pterm.Println(pterm.Gray("  2. Check the updated sprint-status.yaml for accuracy"))
	pterm.Println(pterm.Gray("  3. Edit the prime directive if needed (_bmad-output/prime-directive.md)"))
	pterm.Println(pterm.Gray("  4. Re-run with --enable-epic-planning to continue development"))
	pterm.Println()
}

// WorkPlan holds the next work item from sprint-status for display.
type WorkPlan struct {
	Action    string // "story", "retrospective", or ""
	EpicKey   string
	StoryKey  string
	Done      int
	Total     int
	Project   string
	StatusPath string
}

// PrintWorkPlan displays what will be worked on from sprint-status.yaml.
func PrintWorkPlan(w WorkPlan) {
	pterm.DefaultSection.Println("Work plan (from sprint-status.yaml)")
	pterm.Info.Printf("Project: %s\n", w.Project)
	pterm.Info.Printf("Status:  %s\n", w.StatusPath)
	switch w.Action {
	case "story":
		pterm.Println()
		pterm.Println(pterm.Cyan("  ▶  Epic:  ") + w.EpicKey)
		pterm.Println(pterm.Cyan("  ▶  Story: ") + w.StoryKey)
		if w.Total > 0 {
			pterm.Info.Printf("Progress: %d/%d stories in epic\n", w.Done, w.Total)
		}
	case "retrospective":
		pterm.Println()
		pterm.Println(pterm.Yellow("  ◐  Retrospective for epic: ") + w.EpicKey)
	case "":
		pterm.Println()
		pterm.Success.Println("  ✔  All work complete — nothing pending")
	}
	pterm.Println()
}

// StatusIcon returns a styled status string with icon prefix for the status command.
func StatusIcon(status string) string {
	switch status {
	case "done":
		return pterm.Green("✔ " + status)
	case "in-progress":
		return pterm.Cyan("▶ " + status)
	case "backlog":
		return pterm.Gray("○ " + status)
	case "deferred":
		return pterm.Yellow("⚑ " + status)
	default:
		return status
	}
}
