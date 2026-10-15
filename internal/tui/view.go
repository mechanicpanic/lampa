package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

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

var modeIcons = map[string]string{"Pulse": "✺", "Spectrum": "≋", "Chase": "↻", "Ambient": "☁", "Drop": "✦", "Meter": "▮"}

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
	return zone.Scan(lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(0, 1).Width(w - 2).Height(h - 2).
		Render(strings.Join(body, "\n")))
}

func (m Model) mainView(inner, height int) []string {
	v := m.v
	header := m.header(inner)
	status := m.bottom(inner, false)
	if height-(5+len(status)) < 12 { // short window: give the room to the spectrum
		status = m.bottom(inner, true)
	}
	var help []string

	// header, gap, ribbon, gap, gap before status — plus status and help.
	fixed := 5 + len(status) + len(help)
	// Lamps come next (cards, or one-line chips when short on height);
	// the spectrum gets whatever is left.
	tiles := m.tiles(inner, height-fixed-2)
	specRows := max(2, min(12, height-fixed-len(tiles)))

	out := []string{header, ""}
	out = append(out, spectrum(m.bands[:], v.Palette, inner, specRows)...)
	out = append(out, ribbon(m.ribbon, inner), "")
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
	what := m.c.NowPlaying() // the track, else where the sound comes from
	if what == "" {
		what = i18n.T(m.c.SourceName())
	}
	room := inner - lipgloss.Width(left) - 2
	if room < 8 {
		return left
	}
	right := dim.Render(ansi.Truncate("♪ "+what, room, "…"))
	return left + strings.Repeat(" ", inner-lipgloss.Width(left)-lipgloss.Width(right)) + right
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
			seg := string(runes[start:end])
			if r == rows-1 { // keep a faint floor so silence doesn't look broken
				for _, ch := range runes[start:end] {
					if ch == ' ' {
						b.WriteString(styles[band].Faint(true).Render("▁"))
					} else {
						b.WriteString(styles[band].Render(string(ch)))
					}
				}
				start = end
				continue
			}
			b.WriteString(styles[band].Render(seg))
			start = end
		}
		out[r] = b.String()
	}
	return out
}

// ribbon: newest column at the right; beats (♥), kicks (•), hats (·).
func ribbon(cols []engine.Marker, width int) string {
	if len(cols) > width {
		cols = cols[len(cols)-width:]
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", width-len(cols)))
	for _, mk := range cols {
		switch {
		case mk.Beat:
			b.WriteString(accent.Render("♥"))
		case mk.Low:
			b.WriteString(soft.Render("•"))
		case mk.High:
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
	for _, it := range m.items() {
		var chip string
		if it.group != "" {
			chip = zone.Mark("group:"+it.group, groupChip(it))
		} else {
			chip = zone.Mark("lamp:"+it.single.Info.ID, lampChip(*it.single))
		}
		if cur != "" && lipgloss.Width(cur)+3+lipgloss.Width(chip) > inner {
			lines = append(lines, cur)
			cur = ""
		}
		if cur != "" {
			cur += "   "
		}
		cur += chip
	}
	lines = append(lines, cur)
	for i, l := range lines {
		lines[i] = lipgloss.PlaceHorizontal(inner, lipgloss.Center, l)
	}
	return lines
}

func lampChip(l engine.LampView) string {
	dot := lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color.Hex())).Render("●")
	mark := ok.Render("♪")
	switch {
	case !l.Enabled:
		dot, mark = dim.Render("○"), dim.Render("💤")
	case l.Stats.Status == light.StatusLost:
		mark = warn.Render("✕")
	}
	return dot + " " + l.Info.Name + " " + mark
}

// groupChip: one ▮ per floor, bottom to top left to right.
func groupChip(it lampItem) string {
	var b strings.Builder
	for i := len(it.members) - 1; i >= 0; i-- {
		l := it.members[i]
		if l.Enabled {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color.Hex())).Render("▮"))
		} else {
			b.WriteString(dim.Render("▯"))
		}
	}
	return b.String() + " " + it.group + " " + groupMark(it)
}

func groupMark(it lampItem) string {
	all, lost := true, false
	for _, l := range it.members {
		all = all && !l.Enabled
		lost = lost || (l.Enabled && l.Stats.Status == light.StatusLost)
	}
	switch {
	case all:
		return dim.Render("💤")
	case lost:
		return warn.Render("✕")
	}
	return ok.Render("♪")
}

// lampItem is one card: a single lamp, or a stacked group (members top first).
type lampItem struct {
	single  *engine.LampView
	group   string
	members []engine.LampView
}

// items: lamps in view order; a group appears once, where its first lamp is.
func (m Model) items() []lampItem {
	var out []lampItem
	at := map[string]int{}
	for i := range m.v.Lamps {
		l := m.v.Lamps[i]
		if m.hideSleeping && !l.Enabled {
			continue // hidden by choice; still listed in the lamp picker
		}
		if l.Info.Group == "" {
			out = append(out, lampItem{single: &m.v.Lamps[i]})
			continue
		}
		k, ok := at[l.Info.Group]
		if !ok {
			k = len(out)
			at[l.Info.Group] = k
			out = append(out, lampItem{group: l.Info.Group})
		}
		out[k].members = append(out[k].members, l)
	}
	for _, it := range out {
		sort.SliceStable(it.members, func(a, b int) bool { return it.members[a].Info.Level > it.members[b].Info.Level })
	}
	return out
}

func (m Model) cards(inner int) []string {
	per := max(1, (inner+1)/(tileWidth+1))
	tw := min(inner, (inner+1)/per-1) // stretch tiles to use the row
	var rows []string
	items := m.items()
	for i := 0; i < len(items); i += per {
		var row []string
		for _, it := range items[i:min(i+per, len(items))] {
			if it.group != "" {
				row = append(row, m.groupTile(it, tw))
			} else {
				row = append(row, tile(*it.single, m.shownFPS(*it.single), tw))
			}
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
	return zone.Mark("lamp:"+l.Info.ID, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(frame).
		Width(cw).Render(strings.Join(lines, "\n")))
}

// bottom: the settings as pills (key + current value), an optional line of
// notes (paused, no sound, messages) and the quieter list of actions.
func (m Model) bottom(inner int, compact bool) []string {
	out := m.pills(inner, compact)
	if notes := m.notes(); notes != "" {
		out = append(out, lipgloss.PlaceHorizontal(inner, lipgloss.Center, notes))
	}
	return append(out, actionLines(inner)...)
}

func pill(key, body string) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(0, 1).Render(soft.Render(key) + " " + body)
}

// swatch: three blocks in the palette's colors.
func swatch(name string) string {
	p := paletteByName(name)
	var b strings.Builder
	for _, t := range []float64{0, 1.0 / 3, 2.0 / 3} {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(p.At(t).ToRGB().Hex())).Render("■"))
	}
	return b.String()
}

func meter(v float64) string {
	n := int(math.Round(math.Max(0, math.Min(1, v)) * 5))
	return soft.Render(strings.Repeat("▰", n)) + dim.Render(strings.Repeat("▱", 5-n))
}

func (m Model) pills(inner int, compact bool) []string {
	v := m.v
	items := []string{
		pill("1-6", modeIcons[v.Mode]+" "+v.Mode),
		pill("p", swatch(v.Palette)+" "+v.Palette),
		pill("←→", meter(v.Sensitivity)+" "+fmt.Sprintf("%.2f", v.Sensitivity)),
		pill("↑↓", "◷ "+fmt.Sprintf(i18n.T("%+d ms"), v.Latency.Milliseconds())),
	}
	if r := lipgloss.JoinHorizontal(lipgloss.Top, intersperse(items, " ")...); !compact && lipgloss.Width(r) <= inner {
		return strings.Split(lipgloss.PlaceHorizontal(inner, lipgloss.Center, r), "\n")
	}
	// narrow or short window: one plain line
	return []string{lipgloss.PlaceHorizontal(inner, lipgloss.Center, strings.Join([]string{
		modeIcons[v.Mode] + " " + v.Mode, swatch(v.Palette) + " " + v.Palette,
		fmt.Sprintf("%.2f", v.Sensitivity), fmt.Sprintf(i18n.T("%+d ms"), v.Latency.Milliseconds()),
	}, dim.Render(" · ")))}
}

func (m Model) notes() string {
	var parts []string
	if m.v.Paused {
		parts = append(parts, warn.Render(i18n.T("⏸ paused")))
	}
	if !m.v.AudioAlive {
		parts = append(parts, soft.Render(i18n.T("no sound — play some music ♪")))
	}
	if m.status != "" {
		parts = append(parts, soft.Render(m.status))
	}
	return strings.Join(parts, dim.Render(" · "))
}

var actionKeys = [][2]string{
	{"s", "source"}, {"l", "lamps"}, {"d", "find"}, {"␣", "pause"}, {"t", "lang"}, {"q", "quit"},
}

// actionLines: the actions with labels if they fit, otherwise keys only.
func actionLines(inner int) []string {
	render := func(labels bool) string {
		var parts []string
		for _, k := range actionKeys {
			p := soft.Render(k[0])
			if labels {
				p += " " + dim.Render(i18n.T(k[1]))
			}
			parts = append(parts, p)
		}
		return " " + strings.Join(parts, dim.Render(" · "))
	}
	line := render(true)
	if lipgloss.Width(line) > inner {
		line = render(false)
	}
	return []string{lipgloss.PlaceHorizontal(inner, lipgloss.Center, line)}
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
	hide := i18n.T("h hide sleeping")
	if m.hideSleeping {
		hide = i18n.T("h show sleeping")
	}
	return append(out, "", dim.Render(i18n.T("↑↓ choose · space on/off · esc close")+" · "+hide))
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

// groupTile: one card for a stacked fixture, as tall as a lamp card — a bar
// per floor (top first) on the left, the group name and status beside them.
// Each bar is clickable (that lamp), the name too (the whole group).
func (m Model) groupTile(it lampItem, width int) string {
	cw := width - 2
	barW := max(4, cw/2)
	textW := max(1, cw-barW-1)
	fps := math.Inf(1)
	var bars []string
	for _, l := range it.members {
		fill := lipgloss.NewStyle().Foreground(lipgloss.Color(l.Color.Hex()))
		full := int(math.Round(l.Brightness * float64(barW)))
		bar := fill.Render(strings.Repeat("█", full)) + fill.Faint(true).Render(strings.Repeat("░", barW-full))
		if !l.Enabled {
			bar = dim.Render(strings.Repeat("░", barW))
		} else {
			fps = math.Min(fps, m.shownFPS(l))
		}
		bars = append(bars, zone.Mark("lamp:"+l.Info.ID, bar))
	}
	status := groupMark(it)
	if !math.IsInf(fps, 1) && status == ok.Render("♪") {
		status = ok.Render(fmt.Sprintf("♪ %.0f fps", fps))
	}
	text := []string{zone.Mark("group:"+it.group, ansi.Truncate(it.group, textW, "…")), ansi.Truncate(status, textW, "…")}
	var lines []string
	for i := 0; i < max(len(bars), len(text)); i++ {
		bar := strings.Repeat(" ", barW)
		if i < len(bars) {
			bar = bars[i]
		}
		t := ""
		if i < len(text) {
			t = text[i]
		}
		lines = append(lines, bar+" "+t)
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Width(cw).Render(strings.Join(lines, "\n"))
}
