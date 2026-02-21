// demo_anim.go — animation strip demos for agent output box.
//
// Research: Evil Martians (CLI UX), cli-spinners (sindresorhus), braille-pattern-cli-loading-indicator (Heroku).
// Braille chars enable smooth sub-character animation; 80ms interval is industry standard.
package ui

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"atomicgo.dev/cursor"
	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"
)

// DemoStripStyle identifies which animation style to use for the demo strip.
type DemoStripStyle int

const (
	DemoStyleWaveStream DemoStripStyle = iota + 1
	DemoStylePongBounce // ball bounces left-right and up-down (Pong-like)
)

// DemoStripLine returns one line (4 chars) of the animation strip for the given style.
func DemoStripLine(style DemoStripStyle, frameIdx, lineIdx, totalLines int) string {
	switch style {
	case DemoStyleWaveStream:
		return demoWaveStream(frameIdx, lineIdx, totalLines)
	case DemoStylePongBounce:
		return demoPongBounce(frameIdx, lineIdx, totalLines)
	default:
		return "    "
	}
}

// demoWaveStream: true wave shape with braille, trail fade (Heroku/cli-spinners inspired)
// Slower t step (0.15) for fluid motion; crest has bright braille, trail fades.
func demoWaveStream(frameIdx, lineIdx, totalLines int) string {
	// Wave flows down: crest position advances with frame
	t := float64(frameIdx) * 0.15
	var buf [4]rune
	for col := 0; col < 4; col++ {
		// Wave: sin(phase) gives crest/trough; col offsets for horizontal wave
		phase := t - float64(lineIdx)*0.4 + float64(col)*0.6
		v := math.Sin(phase)
		// Map to intensity: v in [-1,1] -> 0-7 for braille
		idx := int((v+1)*3.5) % 8
		if idx < 0 {
			idx += 8
		}
		if idx < len(lightBraille) {
			buf[col] = lightBraille[idx]
		} else {
			buf[col] = brailleChars[idx%8]
		}
	}
	return string(buf[:])
}

// demoPongBounce: Pong-like ball bouncing left-right across the strip.
// Wider (6 cols), slower (2x), thicker paddles (2 rows) that the ball collides with.
func demoPongBounce(frameIdx, lineIdx, totalLines int) string {
	slowFrame := frameIdx / pongSlowDiv

	// Horizontal bounce: left wall (0) to right wall (12, at log box) and back
	horizontalSeq := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	horizLen := len(horizontalSeq)
	mod := func(a, n int) int {
		r := a % n
		if r < 0 {
			r += n
		}
		return r
	}
	baseColAt := func(sf int) int {
		return horizontalSeq[mod(sf, horizLen)]
	}

	// Vertical bounce: ball moves up-down for full Pong court
	vertLen := 2 * (totalLines - 1)
	if vertLen < 1 {
		vertLen = 1
	}
	vertPhase := mod(slowFrame, vertLen)
	var ballRow int
	if vertPhase < totalLines {
		ballRow = vertPhase
	} else {
		ballRow = vertLen - vertPhase
	}

	// Wobble only when ball hits a paddle (top or bottom), not in mid-flight
	ballColAt := func(sf int) int {
		base := baseColAt(sf)
		vp := mod(sf, vertLen)
		var row int
		if vp < totalLines {
			row = vp
		} else {
			row = vertLen - vp
		}
		if row == 0 || row == totalLines-1 {
			r := rand.New(rand.NewSource(int64(sf)))
			if r.Intn(4) == 0 {
				base += r.Intn(3) - 1
			}
		}
		if base < 0 {
			base = 0
		}
		if base >= pongStripWidth {
			base = pongStripWidth - 1
		}
		return base
	}
	ballCol := ballColAt(slowFrame)

	// Paddles track the ball only when it's headed towards them; otherwise stay at last hit position
	ballMovingDown := vertPhase < totalLines
	ballMovingUp := !ballMovingDown

	topPaddleEnd := pongPaddleThickness
	bottomPaddleStart := totalLines - pongPaddleThickness
	if bottomPaddleStart < topPaddleEnd {
		bottomPaddleStart = topPaddleEnd
	}
	inTopPaddle := lineIdx < topPaddleEnd
	inBottomPaddle := lineIdx >= bottomPaddleStart

	// Ball position when last at each paddle (no return to center — stays until ball comes back)
	lastTopFrame := slowFrame - (slowFrame % vertLen)
	lastBottomFrame := slowFrame - (slowFrame % vertLen) + (totalLines - 1)
	if lastBottomFrame > slowFrame {
		lastBottomFrame -= vertLen
	}

	topPaddleCenter := ballColAt(lastTopFrame)
	bottomPaddleCenter := ballColAt(lastBottomFrame)
	if ballMovingUp {
		topPaddleCenter = ballCol
	}
	if ballMovingDown {
		bottomPaddleCenter = ballCol
	}

	half := pongPaddleWidth / 2
	clampPaddle := func(center int) (left, right int) {
		left = center - half
		right = center + half
		if left < 0 {
			left = 0
			right = pongPaddleWidth - 1
		}
		if right >= pongStripWidth {
			right = pongStripWidth - 1
			left = right - pongPaddleWidth + 1
		}
		return left, right
	}

	topLeft, topRight := clampPaddle(topPaddleCenter)
	botLeft, botRight := clampPaddle(bottomPaddleCenter)

	var buf [pongStripWidth]rune
	for col := 0; col < pongStripWidth; col++ {
		if lineIdx == ballRow && col == ballCol {
			buf[col] = '●' // ball
		} else if inTopPaddle && col >= topLeft && col <= topRight {
			buf[col] = '═' // top paddle
		} else if inBottomPaddle && col >= botLeft && col <= botRight {
			buf[col] = '═' // bottom paddle
		} else {
			buf[col] = ' '
		}
	}
	return string(buf[:])
}

// DemoStyleName returns a short name for the style.
func DemoStyleName(style DemoStripStyle) string {
	switch style {
	case DemoStyleWaveStream:
		return "Wave stream"
	case DemoStylePongBounce:
		return "Pong bounce"
	default:
		return "Unknown"
	}
}

const defaultDemoDuration = 4 * time.Second
const demoFrameInterval = 80 * time.Millisecond

// Pong strip: extends to log box (13 cols = 10 court + 3 gap) so ball bounces off box
const pongStripWidth = 13
const pongSlowDiv = 4
const pongPaddleThickness = 1 // single line at top and bottom only
const pongPaddleWidth = 3

// RunDemoAnim runs a single animation demo in the terminal.
// duration overrides the default 4s when > 0.
func RunDemoAnim(style DemoStripStyle, duration time.Duration) {
	demoDuration := defaultDemoDuration
	if duration > 0 {
		demoDuration = duration
	}
	area := cursor.NewArea()
	logLines := []string{
		"[demo] Simulated agent output line 1",
		"[demo] Simulated agent output line 2",
		"[demo] Simulated agent output line 3",
	}
	totalLines := 1 + len(logLines) + 1
	truncate := func(s string) string { return runewidth.Truncate(s, cordonBoxWidth, "…") }

	interiorWidth := cordonBoxWidth + 2
	labelWidth := runewidth.StringWidth(agentOutputLabel)
	dashTotal := interiorWidth - labelWidth
	if dashTotal < 0 {
		dashTotal = 0
	}
	leftDashes := dashTotal / 2
	rightDashes := dashTotal - leftDashes
	topLine := "┏" + strings.Repeat("━", leftDashes) + agentOutputLabel + strings.Repeat("━", rightDashes) + "┓"
	bottomLine := "┗" + strings.Repeat("━", interiorWidth) + "┛"

	deadline := time.Now().Add(demoDuration)
	frameIdx := 0

	// Pong strip extends to box (no gap) so ball bounces off it; wave has gap
	stripSep := "   "
	if style == DemoStylePongBounce {
		stripSep = ""
	}
	for time.Now().Before(deadline) {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("  Demo: %s (Ctrl+C to skip)\n\n", DemoStyleName(style)))
		sb.WriteString(fmt.Sprintf("  %s%s%s\n", DemoStripLine(style, frameIdx, 0, totalLines), stripSep, topLine))
		for i := 0; i < len(logLines); i++ {
			content := truncate(logLines[i])
			pad := cordonBoxWidth - runewidth.StringWidth(content)
			if pad < 0 {
				pad = 0
			}
			inner := " " + content + strings.Repeat(" ", pad) + " "
			sb.WriteString(fmt.Sprintf("  %s%s┃%s┃\n", DemoStripLine(style, frameIdx, 1+i, totalLines), stripSep, inner))
		}
		sb.WriteString(fmt.Sprintf("  %s%s%s\n", DemoStripLine(style, frameIdx, totalLines-1, totalLines), stripSep, bottomLine))
		area.Update(sb.String())

		frameIdx++
		time.Sleep(demoFrameInterval)
	}

	area.Clear()
}

// RunAllDemoAnims runs all animation demos in sequence.
// duration overrides per-demo duration when > 0.
func RunAllDemoAnims(duration time.Duration) {
	styles := []DemoStripStyle{
		DemoStyleWaveStream,
		DemoStylePongBounce,
	}

	pterm.DefaultHeader.WithFullWidth().Println("Animation Demo")
	pterm.Info.Println("Each animation runs for 4 seconds. Press Ctrl+C to skip.")
	pterm.Println()

	for i, style := range styles {
		pterm.DefaultSection.Printf("Option %d: %s", i+1, DemoStyleName(style))
		RunDemoAnim(style, duration)
		if i < len(styles)-1 {
			pterm.Println()
		}
	}

	pterm.Success.Println("Demo complete.")
}
