// demo_anim.go — animation strip demos for agent output box.
//
// Research: Evil Martians (CLI UX), cli-spinners (sindresorhus), braille-pattern-cli-loading-indicator (Heroku).
// Braille chars enable smooth sub-character animation; 80ms interval is industry standard.
package ui

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"atomicgo.dev/cursor"
	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"
)

// colorDisabled is true when NO_COLOR is set; ansiWrap returns plain text.
// When NO_COLOR is set, all visual effects (ball glow, trail, paddle styling, hit flash, color cycling) are disabled.
var colorDisabled = os.Getenv("NO_COLOR") != ""

// ANSI SGR codes (omit when colorDisabled)
const (
	ansiBrightCyan  = "96"
	ansiBrightWhite = "97"
	ansiCyan        = "36"
	ansiBold        = "1"
	ansiDim         = "2"
	ansiBrightYellow = "93"
)

// ansiWrap wraps s with ANSI SGR codes; returns s unchanged when colorDisabled.
// Used for all visual effects: ball glow (bold + color), trail (dim), paddle styling (bright white/yellow flash), hit flash (bright yellow).
func ansiWrap(s string, codes ...string) string {
	if colorDisabled || len(codes) == 0 {
		return s
	}
	seq := "\x1b[" + strings.Join(codes, ";") + "m"
	return seq + s + "\x1b[0m"
}

// ballPalette cycles through bright colors for the ball (cyan → magenta → blue → yellow → green → red).
// Color cycling creates a visual effect where the ball glows in different colors as it moves.
var ballPalette = []string{"96", "95", "94", "93", "92", "91"}

// ballColor returns the ANSI color code for the ball at slowFrame (cycles through ballPalette).
// Used with ansiWrap to create the ball's glowing effect; disabled when NO_COLOR is set.
func ballColor(slowFrame int) string {
	if slowFrame < 0 {
		slowFrame = 0
	}
	return ballPalette[slowFrame%len(ballPalette)]
}

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

// pongBallPos returns the (row, col) of the ball at slowFrame for a court of totalLines.
// Pure function: horizontal bounce (left-right across strip), vertical bounce (top-bottom),
// wobble on paddle hit (random horizontal offset when ball reaches top/bottom row).
// Used by demoPongBounce to position the ball and calculate trail positions (slowFrame-1, slowFrame-2).
func pongBallPos(slowFrame, totalLines int) (row, col int) {
	horizontalSeq := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	horizLen := len(horizontalSeq)
	baseColAt := func(sf int) int {
		return horizontalSeq[mod(sf, horizLen)]
	}

	vertLen := 2 * (totalLines - 1)
	if vertLen < 1 {
		vertLen = 1
	}
	vertPhase := mod(slowFrame, vertLen)
	if vertPhase < totalLines {
		row = vertPhase
	} else {
		row = vertLen - vertPhase
	}

	base := baseColAt(slowFrame)
	if row == 0 || row == totalLines-1 {
		r := rand.New(rand.NewSource(int64(slowFrame)))
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
	col = base
	return row, col
}

// demoPongBounce: Pong-like ball bouncing left-right across the strip.
// Visual effects:
//   - Ball glow: bold ball character (●) with color cycling via ballColor(slowFrame) and ansiWrap
//   - Trail: two previous positions rendered as dimmed characters (░, ·) using ansiWrap with ansiDim
//   - Paddle styling: thick blocks (▓) in bright white, flash bright yellow on hit
//   - Hit flash: ball turns bright yellow when hitting top/bottom paddle
//   - Color cycling: ball cycles through ballPalette colors as it moves
//   - NO_COLOR: all ansiWrap calls return plain text when colorDisabled is true
// Paddles track ball position when ball is moving toward them, stay at last hit position otherwise.
func demoPongBounce(frameIdx, lineIdx, totalLines int) string {
	slowFrame := frameIdx / pongSlowDiv
	ballRow, ballCol := pongBallPos(slowFrame, totalLines)

	vertLen := 2 * (totalLines - 1)
	if vertLen < 1 {
		vertLen = 1
	}
	vertPhase := mod(slowFrame, vertLen)

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
	if lastBottomFrame < 0 {
		lastBottomFrame = 0
	}

	_, topPaddleCenter := pongBallPos(lastTopFrame, totalLines)
	_, bottomPaddleCenter := pongBallPos(lastBottomFrame, totalLines)
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

	// Trail positions (clamp slowFrame-1/-2 to 0 if negative)
	// Trail creates motion blur effect: previous positions rendered with dimmed characters
	prevRow1, prevCol1 := pongBallPos(max(0, slowFrame-1), totalLines)
	prevRow2, prevCol2 := pongBallPos(max(0, slowFrame-2), totalLines)

	// Hit flash: ball at top or bottom row triggers bright yellow flash for ball and paddle
	hitFlash := ballRow == 0 || ballRow == totalLines-1
	topPaddleHit := hitFlash && ballRow == 0
	bottomPaddleHit := hitFlash && ballRow == totalLines-1

		// Priority: ball > paddle > trail > empty
		// All visual effects use ansiWrap, which returns plain text when NO_COLOR is set
		var cells [pongStripWidth]string
		for col := 0; col < pongStripWidth; col++ {
			switch {
			case lineIdx == ballRow && col == ballCol:
				// Ball glow: bold character with color cycling (or bright yellow on hit flash)
				if hitFlash {
					cells[col] = ansiWrap("●", ansiBold, ansiBrightYellow)
				} else {
					cells[col] = ansiWrap("●", ansiBold, ballColor(slowFrame))
				}
			case inTopPaddle && col >= topLeft && col <= topRight:
				// Paddle styling: bright white normally, bright yellow flash on hit
				if topPaddleHit {
					cells[col] = ansiWrap("▓", ansiBrightYellow)
				} else {
					cells[col] = ansiWrap("▓", ansiBrightWhite)
				}
			case inBottomPaddle && col >= botLeft && col <= botRight:
				// Paddle styling: bright white normally, bright yellow flash on hit
				if bottomPaddleHit {
					cells[col] = ansiWrap("▓", ansiBrightYellow)
				} else {
					cells[col] = ansiWrap("▓", ansiBrightWhite)
				}
			case lineIdx == prevRow1 && col == prevCol1:
				// Trail: dimmed character with color from previous frame
				cells[col] = ansiWrap("░", ansiDim, ballColor(slowFrame-1))
			case lineIdx == prevRow2 && col == prevCol2:
				// Trail: dimmed dot for older position
				cells[col] = ansiWrap("·", ansiDim)
			default:
				cells[col] = " "
			}
		}
	return strings.Join(cells[:], "")
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
