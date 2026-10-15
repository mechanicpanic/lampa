package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lampa/internal/color"
	"lampa/internal/engine"
	"lampa/internal/i18n"
	"lampa/internal/light"
)

// UI chrome uses the terminal's own ANSI palette, so it follows the user's
// theme (light or dark); only the spectrum and the lamps use true color.
var (
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	soft   = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	ok     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warn   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	border = lipgloss.Color("8")
)

const (
	// below this the tracker is guessing (Bach, ambient): show no number
	minTempoConfidence = 0.4
	levels             = " ▁▂▃▄▅▆▇█"
	minWidth           = 40
	minHeight          = 12
	tileWidth          = 18 // including its rounded border
)

var modeIcons = map[string]string{"Pulse": "✺", "Spectrum": "≋", "Chase": "↻", "Ambient": "☁", "Drop": "✦"}

func spectrumRows(bands []float64, rows int) []string {
	out := make([]string, rows)
	steps := 8 * rows
	for r := 0; r < rows; r++ {
		var b strings.Builder
		floor := (rows - 1 - r) * 8 // top row first
		for _, v := range bands {
			lvl := int(v*float64(steps) + 0.5)
			cell := max(0, min(8, lvl-floor))
			b.WriteRune([]rune(levels)[cell])
		}
		out[r] = b.String()
	}
	return out
}

func paletteByName(name string) color.Palette {
	for _, p := range color.Palettes {
		if p.Name == name {
			return p
		}
	}
	return color.Palettes[0]
}

func (m Model) View() string {
	w, h := m.width, m.height
	if w < minWidth || h < minHeight {
		msg := i18n.T("window too small ✿ stretch me")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, ansi.Truncate(soft.Render(msg), w, ""))
	}
	inner := w - 4 // rounded border + one space of padding on each side
	var body []string
	switch {
	case m.picking:
		body = m.pickerView()
	case m.lamps:
		body = m.lampPickerView()
	default:
		body = m.mainView(inner, h-2)
	}
	for i, l := range body {
		body[i] = ansi.Truncate(l, inner, "…")
	}
	if len(body) > h-2 {
		body = body[:h-2]
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(0, 1).Width(w - 2).Height(h - 2).
		Render(strings.Join(body, "\n"))
}

func (m Model) mainView(inner, height int) []string {
	v := m.v
	header := m.header(inner)
	status := m.statusLine()
	help := helpLines(inner)

	// header, gap, ribbon, gap, gap before status — plus status and help.
	fixed := 5 + len(status) + len(help)
	// Lamps come next (cards, or one-line chips when short on height);
	// the spectrum gets whatever is left.
	tiles := m.tiles(inner, height-fixed-2)
	specRows := max(2, min(12, height-fixed-len(tiles)))

	out := []string{header, ""}
	out = append(out, spectrum(m.bands[:], v.Palette, inner, specRows)...)
	out = append(out, ribbon(v.History, inner), "")
	out = append(out, tiles...)
	if pad := height - len(out) - 1 - len(status) - len(help); pad > 0 {
		out = append(out, make([]string, pad)...)
	}
	out = append(out, "")
	out = append(out, status...)
	return append(out, help...)
}

func (m Model) header(inner int) string {
	v := m.v
	sure := v.Features.BPM > 0 && v.Features.Confidence >= minTempoConfidence
	heart := dim.Render("♡")
	if sure {
		heart = heartGlyph(heartLevel(v.Features.BeatPhase))
	}
	bpm := dim.Render("— bpm")
	if sure {
		bpm = fmt.Sprintf("%.0f BPM", v.Features.BPM)
	}
	left := accent.Render("✿ lampa") + "  " + soft.Render(modeIcons[v.Mode]+" "+v.Mode) + dim.Render(" · ") + bpm + " " + heart
	right := dim.Render("♪ " + i18n.T(m.c.SourceName()))
	gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// spectrum spreads the bands over exactly `width` columns.
func spectrum(bands []float64, palette string, width, rows int) []string {
	n := len(bands)
	cols := make([]float64, width)
	for c := range cols {
		cols[c] = bands[c*n/width]
	}
	pal := paletteByName(palette)
	styles := make([]lipgloss.Style, n)
	for b := range styles {
		styles[b] = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.At(0.85 * float64(b) / float64(n)).ToRGB().Hex()))
	}
	raw := spectrumRows(cols, rows)
	out := make([]string, rows)
	for r, row := range raw {
		runes := []rune(row)
		var b strings.Builder
		for start := 0; start < width; { // one styled run per band
			band := start * n / width
			end := start + 1
			for end < width && end*n/width == band {
				end++
			}
			b.WriteString(styles[band].Render(string(runes[start:end])))
			start = end
		}
		out[r] = b.String()
	}
	return out
}

// ribbon: the last ~2 s of beats (♥), kicks (•) and hats (·).
func ribbon(h []engine.Marker, width int) string {
	if len(h) == 0 {
		return dim.Render(strings.Repeat("·", width))
	}
	var b strings.Builder
	for c := 0; c < width; c++ {
		lo := c * len(h) / width
		hi := max(lo+1, (c+1)*len(h)/width)
		var beat, low, high bool
		for _, mk := range h[lo:min(hi, len(h))] {
			beat, low, high = beat || mk.Beat, low || mk.Low, high || mk.High
		}
		switch {
		case beat:
			b.WriteString(accent.Render("♥"))
		case low:
			b.WriteString(soft.Render("•"))
		case high:
			b.WriteString(dim.Render("·"))
		default:
			b.WriteString(" ")
		}
	}
	return b.String()
}

func (m Model) tiles(inner, avail int) []string {
	if len(m.v.Lamps) == 0 {
		return []string{warn.Render(i18n.T("no lamps yet — press d and I'll look ✿"))}
	}
	if cards := m.cards(inner); len(cards) <= avail {
		return cards
	}
	return m.chips(inner)
}

// chips: one short line per lamp, flowed into rows — for short windows.
func (m Model) chips(inner int) []string {
	var lines []string
	cur := ""
	for _, l := range m.v.Lamps {
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color.Hex())).Render("●")
		mark := ok.Render("♪")
		switch {
		case !l.Enabled:
			dot, mark = dim.Render("○"), dim.Render("💤")
		case l.Stats.Status == light.StatusLost:
			mark = warn.Render("✕")
		}
		chip := dot + " " + l.Info.Name + " " + mark
		if cur != "" && lipgloss.Width(cur)+3+lipgloss.Width(chip) > inner {
			lines = append(lines, cur)
			cur = ""
		}
		if cur != "" {
			cur += "   "
		}
		cur += chip
	}
	return append(lines, cur)
}

func (m Model) cards(inner int) []string {
	per := max(1, (inner+1)/(tileWidth+1))
	tw := min(inner, (inner+1)/per-1) // stretch tiles to use the row
	var rows []string
	for i := 0; i < len(m.v.Lamps); i += per {
		var row []string
		for _, l := range m.v.Lamps[i:min(i+per, len(m.v.Lamps))] {
			row = append(row, tile(l, m.shownFPS(l), tw))
		}
		joined := lipgloss.JoinHorizontal(lipgloss.Top, intersperse(row, " ")...)
		rows = append(rows, lipgloss.PlaceHorizontal(inner, lipgloss.Center, joined))
	}
	return strings.Split(strings.Join(rows, "\n"), "\n")
}

func intersperse(xs []string, sep string) []string {
	var out []string
	for i, x := range xs {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, x)
	}
	return out
}

// tile: a rounded card framed in the lamp's own color; the bar shows brightness.
func tile(l engine.LampView, fps float64, width int) string {
	cw := width - 2
	hex := l.Color.Hex()
	fill := lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
	frame := lipgloss.Color(hex)
	full := int(math.Round(l.Brightness * float64(cw)))
	bar := fill.Render(strings.Repeat("█", full)) + fill.Faint(true).Render(strings.Repeat("░", cw-full))

	status := ok.Render(fmt.Sprintf("♪ %.0f fps", fps))
	switch {
	case !l.Enabled:
		frame = border
		bar = dim.Render(strings.Repeat("░", cw))
		status = dim.Render(i18n.T("💤 sleeping"))
	case l.Stats.Status == light.StatusLost:
		status = warn.Render(i18n.T("✕ lost"))
	case l.Stats.Status == light.StatusConnecting:
		status = dim.Render(i18n.T("… looking"))
	}
	lines := []string{bar, ansi.Truncate(l.Info.Name, cw, "…"), ansi.Truncate(status, cw, "…")}
	if l.Stats.Note != "" && l.Enabled {
		lines = append(lines, ansi.Truncate(dim.Render(i18n.T(l.Stats.Note)), cw, "…"))
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(frame).
		Width(cw).Render(strings.Join(lines, "\n"))
}

func (m Model) statusLine() []string {
	v := m.v
	parts := []string{
		i18n.T("palette") + " " + soft.Render(v.Palette),
		fmt.Sprintf(i18n.T("sens %.2f"), v.Sensitivity),
		fmt.Sprintf(i18n.T("delay %+d ms"), v.Latency.Milliseconds()),
	}
	if v.Paused {
		parts = append(parts, warn.Render(i18n.T("⏸ paused")))
	}
	if !v.AudioAlive {
		parts = append(parts, soft.Render(i18n.T("no sound — play some music ♪")))
	}
	out := []string{strings.Join(parts, dim.Render(" · "))}
	if m.status != "" {
		out = append(out, soft.Render(m.status))
	}
	return out
}

var helpKeys = [][2]string{
	{"1-5", "mode"}, {"p", "palette"}, {"←→", "sens"}, {"↑↓", "delay"},
	{"s", "source"}, {"l", "lamps"}, {"d", "find"}, {"␣", "pause"}, {"t", "lang"}, {"q", "quit"},
}

// helpLines: full labels on one or two lines if they fit, otherwise keys only.
func helpLines(inner int) []string {
	render := func(keys [][2]string, labels bool) string {
		var parts []string
		for _, k := range keys {
			p := soft.Render(k[0])
			if labels {
				p += " " + dim.Render(i18n.T(k[1]))
			}
			parts = append(parts, p)
		}
		return strings.Join(parts, dim.Render("  "))
	}
	if full := render(helpKeys, true); lipgloss.Width(full) <= inner {
		return []string{full}
	}
	half := (len(helpKeys) + 1) / 2
	a, b := render(helpKeys[:half], true), render(helpKeys[half:], true)
	if lipgloss.Width(a) <= inner && lipgloss.Width(b) <= inner {
		return []string{a, b}
	}
	return []string{render(helpKeys, false)}
}

func (m Model) pickerView() []string {
	out := []string{accent.Render(i18n.T("♪ Audio source")), ""}
	items := []string{i18n.T("whole system")}
	for _, a := range m.apps {
		items = append(items, fmt.Sprintf("%s %s", a.Name, dim.Render(fmt.Sprintf("(pid %d)", a.PID))))
	}
	for i, it := range items {
		cursor := "  "
		if i == m.cursor {
			cursor = accent.Render("▸ ")
		}
		out = append(out, cursor+it)
	}
	return append(out, "", dim.Render(i18n.T("↑↓ choose · enter ok · esc cancel")))
}

func (m Model) lampPickerView() []string {
	out := []string{accent.Render(i18n.T("✿ Lamps")), ""}
	for i, l := range m.v.Lamps {
		cursor := "  "
		if i == m.lampCursor {
			cursor = accent.Render("▸ ")
		}
		mark := lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color.Hex())).Render("●")
		if !l.Enabled {
			mark = dim.Render("○")
		}
		out = append(out, fmt.Sprintf("%s%s %-14s %s", cursor, mark, l.Info.Name, dim.Render(l.Info.Model)))
	}
	if len(m.v.Lamps) == 0 {
		out = append(out, dim.Render(i18n.T("no lamps — close this and press d")))
	}
	return append(out, "", dim.Render(i18n.T("↑↓ choose · space on/off · esc close")))
}

// The heart beats: full on the beat, then fades step by step until the next.
const heartLevels = 4

// heartGlyph renders at call time (styles depend on the detected terminal).
func heartGlyph(level int) string {
	switch level {
	case 3:
		return accent.Render("♥")
	case 2:
		return soft.Render("♥")
	case 1:
		return soft.Faint(true).Render("♥")
	}
	return dim.Render("♡")
}

// heartLevel: 3 on the beat, falling to 0 before the next one.
func heartLevel(phase float64) int {
	switch {
	case phase < 0.15:
		return 3
	case phase < 0.35:
		return 2
	case phase < 0.6:
		return 1
	}
	return 0
}
