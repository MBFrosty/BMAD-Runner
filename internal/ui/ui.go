package ui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"atomicgo.dev/cursor"
	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"
)

// ansiEscape strips ANSI escape sequences (e.g. \033[31m) from a string.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// FormatLastLineForStatus truncates and sanitizes a line for display in the cordoned section.
// Strips ANSI codes and control chars, truncates to statusTruncate display width (runewidth).
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
	return runewidth.Truncate(line, statusTruncate, "...")
}

const statusTruncate = 60

// cordonBoxWidth is the interior width of the agent output box (fits ~80-char terminals).
const cordonBoxWidth = 48

const (
	barWidth  = 20
	blockSize = 3
)

var brailleChars = []rune{'⣾', '⣷', '⣯', '⣟', '⡿', '⢿', '⣻', '⣽'}

// Light braille for trail fade (1–4 dots per cell)
var lightBraille = []rune{'⠁', '⠂', '⠃', '⠄', '⠅', '⠆', '⠇'}

func mod(a, b int) int {
	r := a % b
	if r < 0 {
		r += b
	}
	return r
}

// rainDropChar returns the braille for one rain stream: bright head, fading trail.
// All streams use the same logic — head (full braille), trail (light braille).
func rainDropChar(slowFrame, lineIdx, totalLines, phase int) rune {
	headAt := mod(slowFrame+phase, totalLines)
	trail1At := mod(slowFrame+phase-1, totalLines)
	trail2At := mod(slowFrame+phase-2, totalLines)
	trail3At := mod(slowFrame+phase-3, totalLines)
	switch lineIdx {
	case headAt:
		return brailleChars[slowFrame%8]
	case trail1At:
		return lightBraille[(slowFrame+1)%len(lightBraille)]
	case trail2At:
		return lightBraille[(slowFrame+2)%len(lightBraille)]
	case trail3At:
		return lightBraille[(slowFrame+3)%len(lightBraille)]
	default:
		return 0
	}
}

// brailleStripLine returns one line of the Matrix rain strip (4 columns).
// All streams operate the same: bright head + fading trail, staggered phases.
func brailleStripLine(frameIdx, lineIdx, totalLines int) string {
	slowFrame := frameIdx / 2
	var buf [4]rune
	// 2 streams per column, 8 total — phases staggered for dense rain
	phases := []int{0, totalLines/8, totalLines/4, 3*totalLines/8, totalLines/2, 5*totalLines/8, 3*totalLines/4, 7*totalLines/8}
	for col := 0; col < 4; col++ {
		var r rune
		for _, phase := range phases[col*2 : col*2+2] {
			if c := rainDropChar(slowFrame, lineIdx, totalLines, phase); c != 0 {
				r = c
				break
			}
		}
		if r == 0 {
			r = ' '
		}
		buf[col] = r
	}
	return string(buf[:])
}

// generateBounceFrames produces frames for a bouncing block across a bar,
// with a synchronized braille spinner. Used by NewPhaseSpinner for CI mode.
func generateBounceFrames() []string {
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

// generateBrailleOnlyFrames produces simple braille-only frames for CI spinner.
func generateBrailleOnlyFrames() []string {
	frames := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		frames = append(frames, string(brailleChars[i%8])+string(brailleChars[(i+1)%8]))
	}
	return frames
}

// NewPhaseSpinner returns a SpinnerPrinter configured with braille-only animation
// for CI/non-live mode. Call .Start(label) / .Success() / .Fail() as usual.
func NewPhaseSpinner() *pterm.SpinnerPrinter {
	frames := generateBrailleOnlyFrames()
	return pterm.DefaultSpinner.
		WithSequence(frames...).
		WithDelay(80 * time.Millisecond).
		WithShowTimer(false)
}

// rotateText is the 12-character string that rotates around the box perimeter.
const rotateText = "agent output "

// PhaseDisplay renders a braille strip and rolling log preview in-place
// using atomicgo/cursor Area. The braille strip animates as one cohesive unit.
// "agent output" rotates around the heavy box perimeter.
type PhaseDisplay struct {
	area         cursor.Area
	phase        string
	frameIdx     int
	logLineCount int
	mu           sync.Mutex
	active       bool
}

// NewPhaseDisplay starts an in-place live area for the given phase.
// logLines controls how many preview lines are shown in the box.
func NewPhaseDisplay(phase string, logLines int) *PhaseDisplay {
	return &PhaseDisplay{
		area:         cursor.NewArea(),
		phase:        phase,
		logLineCount: logLines,
		active:       true,
	}
}

// Tick advances the animation by one frame and redraws with the provided log lines.
// Uses heavy box, 2×2 braille left of box, and "agent output" rotating around the perimeter.
func (d *PhaseDisplay) Tick(logLines []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return
	}
	d.frameIdx++

	truncate := func(s string) string { return runewidth.Truncate(s, cordonBoxWidth, "…") }

	interiorWidth := cordonBoxWidth + 2
	topLen := interiorWidth - 2
	rightLen := d.logLineCount
	bottomLen := interiorWidth - 2
	leftLen := d.logLineCount
	perimeter := topLen + 1 + rightLen + 1 + bottomLen + 1 + leftLen + 1
	offset := (d.frameIdx / 2) % perimeter

	// Only 12 consecutive positions show "agent output"; rest show dash.
	charAt := func(p int) (r rune, ok bool) {
		relPos := (p - offset + perimeter) % perimeter
		if relPos < 0 {
			relPos += perimeter
		}
		if relPos < 12 {
			return rune(rotateText[relPos]), true
		}
		return 0, false
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Executing %s...\n\n", d.phase))

	// Braille strip: one cohesive animation — diagonal pattern flows through all lines
	totalLines := 1 + d.logLineCount + 1

	// Segment boundaries (clockwise: top → top-right corner → right → bottom-right → bottom → bottom-left → left → top-left)
	topRightCorner := topLen
	bottomRightCorner := topLen + 1 + rightLen + 1
	bottomStart := bottomRightCorner + 1
	bottomEnd := bottomStart + bottomLen - 1
	bottomLeftCorner := bottomEnd + 1
	topLeftCorner := bottomLeftCorner + 1 + leftLen + 1

	// Top line: ┏ + top-left corner + top segment + top-right corner + ┓
	topLine := "┏"
	if c, ok := charAt(topLeftCorner); ok && c != ' ' {
		topLine += string(c)
	} else {
		topLine += "━"
	}
	for i := 0; i < topLen; i++ {
		if c, ok := charAt(i); ok && c != ' ' {
			topLine += string(c)
		} else {
			topLine += "━"
		}
	}
	if c, ok := charAt(topRightCorner); ok && c != ' ' {
		topLine += string(c) + "┓"
	} else {
		topLine += "━┓"
	}
	sb.WriteString(fmt.Sprintf("  %s   %s\n", brailleStripLine(d.frameIdx, 0, totalLines), topLine))

	// Content lines: keep log readable — no rotating chars in content area
	for i := 0; i < d.logLineCount; i++ {
		var content string
		if i < len(logLines) {
			content = truncate(FormatLastLineForStatus(logLines[i]))
		}
		pad := cordonBoxWidth - runewidth.StringWidth(content)
		if pad < 0 {
			pad = 0
		}
		inner := " " + content + strings.Repeat(" ", pad) + " "
		sb.WriteString(fmt.Sprintf("  %s   ┃%s┃\n", brailleStripLine(d.frameIdx, 1+i, totalLines), inner))
	}

	// Bottom line: ┗ + bottomLeftCorner + bottom segment (L→R) + bottomRightCorner + ┛
	bottomLine := "┗"
	if c, ok := charAt(bottomLeftCorner); ok && c != ' ' {
		bottomLine += string(c)
	} else {
		bottomLine += "━"
	}
	for i := bottomLen - 1; i >= 0; i-- {
		p := bottomStart + i
		if c, ok := charAt(p); ok && c != ' ' {
			bottomLine += string(c)
		} else {
			bottomLine += "━"
		}
	}
	if c, ok := charAt(bottomRightCorner); ok && c != ' ' {
		bottomLine += string(c) + "┛"
	} else {
		bottomLine += "━┛"
	}
	sb.WriteString(fmt.Sprintf("  %s   %s\n", brailleStripLine(d.frameIdx, totalLines-1, totalLines), bottomLine))
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
	pterm.Println(pterm.Cyan("  ◈  correct-course (plan Epic N + update sprint-status.yaml)"))
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
