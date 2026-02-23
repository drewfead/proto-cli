package bubbles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// bigGlyphs maps printable runes to a 5-row × 5-col pixel pattern.
// '█' is a filled pixel; ' ' is empty. Each row is exactly 5 characters wide.
var bigGlyphs = map[rune][5]string{
	'0': {" ███ ", "█   █", "█   █", "█   █", " ███ "},
	'1': {"  █  ", " ██  ", "  █  ", "  █  ", " ███ "},
	'2': {" ███ ", "█   █", "  ██ ", " █   ", "█████"},
	'3': {"████ ", "    █", " ███ ", "    █", "████ "},
	'4': {"█   █", "█   █", "█████", "    █", "    █"},
	'5': {"█████", "█    ", "████ ", "    █", "████ "},
	'6': {" ███ ", "█    ", "████ ", "█   █", " ███ "},
	'7': {"█████", "    █", "  ██ ", "  █  ", "  █  "},
	'8': {" ███ ", "█   █", " ███ ", "█   █", " ███ "},
	'9': {" ███ ", "█   █", " ████", "    █", " ███ "},
	'!': {"  █  ", "  █  ", "  █  ", "     ", "  █  "},
	'.': {"     ", "     ", "     ", "  █  ", "     "},
	'…': {"     ", "     ", "     ", "█ █ █", "     "},
	',': {"     ", "     ", "     ", "  █  ", " █   "},
	' ': {"     ", "     ", "     ", "     ", "     "},
}

// BigText renders s as a large pixel-font string coloured with color.
// Each pixel is scaled 3× wide and 2× tall for a "very large" terminal display.
// Characters not in the built-in glyph set are rendered as blank space.
// color accepts any lipgloss.TerminalColor value (lipgloss.Color, lipgloss.AdaptiveColor, etc.).
func BigText(s string, color lipgloss.TerminalColor) string {
	return bigTextScaled(s, 3, 2, color)
}

// bigTextScaled renders s at an arbitrary pixel scale.
// xScale controls horizontal stretch (each pixel → xScale chars).
// yScale controls vertical stretch (each pixel row → yScale output lines).
func bigTextScaled(s string, xScale, yScale int, color lipgloss.TerminalColor) string {
	runes := []rune(s)
	numLines := 5 * yScale
	builders := make([]strings.Builder, numLines)

	for ci, ch := range runes {
		pattern, ok := bigGlyphs[ch]
		if !ok {
			pattern = bigGlyphs[' ']
		}

		// Scale the pixel row horizontally once and reuse for all yScale repeats.
		scaledRows := [5]string{}
		for pixRow := 0; pixRow < 5; pixRow++ {
			var sb strings.Builder
			for _, px := range pattern[pixRow] {
				if px == '█' {
					sb.WriteString(strings.Repeat("█", xScale))
				} else {
					sb.WriteString(strings.Repeat(" ", xScale))
				}
			}
			scaledRows[pixRow] = sb.String()
		}

		// Append to output lines, inserting a gap between characters.
		gap := strings.Repeat(" ", xScale)
		for pixRow := 0; pixRow < 5; pixRow++ {
			for rep := 0; rep < yScale; rep++ {
				idx := pixRow*yScale + rep
				if ci > 0 {
					builders[idx].WriteString(gap)
				}
				builders[idx].WriteString(scaledRows[pixRow])
			}
		}
	}

	lines := make([]string, numLines)
	for i := range builders {
		lines[i] = builders[i].String()
	}
	return lipgloss.NewStyle().Foreground(color).Render(strings.Join(lines, "\n"))
}
