package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"lampa/internal/engine"
	"lampa/internal/light"
)

var (
	frameStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	dim        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	accent     = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	warn       = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	ok         = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	bandColors = []string{"#ff5f5f", "#ff875f", "#ffaf5f", "#ffd75f", "#d7ff5f", "#87ff87", "#5fffd7", "#5fd7ff", "#87afff", "#af87ff"}
)

const levels = " ▁▂▃▄▅▆▇█"

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

func (m Model) View() string {
	if m.picking {
		return frameStyle.Render(m.pickerView())
	}
	if m.lamps {
		return frameStyle.Render(m.lampPickerView())
	}
	v := m.v
	inner := max(40, m.width-4)
	var b strings.Builder

	beat := dim.Render("○")
	if v.Features.BPM > 0 && v.Features.BeatPhase < 0.15 {
		beat = accent.Render("●")
	}
	bpm := "— BPM"
	if v.Features.BPM > 0 {
		bpm = fmt.Sprintf("%.0f BPM", v.Features.BPM)
	}
	b.WriteString(fmt.Sprintf("%s  %s · %s %s   %s\n\n",
		accent.Render("lampa"), v.Mode, bpm, beat, dim.Render(m.c.SourceName())))

	// spectrum: each band repeated to fill the width
	per := max(1, inner/len(v.Features.Bands))
	for _, row := range spectrumRows(v.Features.Bands[:], 4) {
		for i, ch := range []rune(row) {
			st := lipgloss.NewStyle().Foreground(lipgloss.Color(bandColors[i*len(bandColors)/len(v.Features.Bands)]))
			b.WriteString(st.Render(strings.Repeat(string(ch), per)))
		}
		b.WriteByte('\n')
	}

	// beat / onset ribbon over the last ~2 s
	b.WriteString(ribbon(v.History, inner))
	b.WriteString("\n\n")

	// lamp tiles
	var tiles []string
	for _, l := range v.Lamps {
		tiles = append(tiles, tile(l))
	}
	if len(tiles) == 0 {
		b.WriteString(warn.Render("ламп нет — нажми d, чтобы найти"))
	} else {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tiles...))
	}
	b.WriteString("\n\n")

	state := []string{
		"палитра " + v.Palette,
		fmt.Sprintf("чувств. %.2f", v.Sensitivity),
		fmt.Sprintf("задержка %+d мс", v.Latency.Milliseconds()),
	}
	if v.Paused {
		state = append(state, warn.Render("пауза"))
	}
	if !v.AudioAlive {
		state = append(state, warn.Render("нет звука"))
	}
	b.WriteString(strings.Join(state, " · "))
	if m.status != "" {
		b.WriteString("\n" + m.status)
	}
	b.WriteString("\n" + dim.Render("[1-5] режим  [p] палитра  [←→] чувств.  [↑↓] задержка  [s] источник  [l] лампы  [d] поиск  [space] пауза  [q] выход"))
	return frameStyle.Render(b.String())
}

func ribbon(h []engine.Marker, width int) string {
	if len(h) == 0 {
		return ""
	}
	var top, bottom strings.Builder
	for c := 0; c < width; c++ {
		lo := c * len(h) / width
		hi := max(lo+1, (c+1)*len(h)/width)
		var beat, low, high bool
		for _, mk := range h[lo:min(hi, len(h))] {
			beat, low, high = beat || mk.Beat, low || mk.Low, high || mk.High
		}
		switch {
		case beat:
			top.WriteString(accent.Render("┃"))
		default:
			top.WriteString(" ")
		}
		switch {
		case low:
			bottom.WriteString(warn.Render("█"))
		case high:
			bottom.WriteString(ok.Render("▪"))
		default:
			bottom.WriteString(dim.Render("·"))
		}
	}
	return top.String() + "\n" + bottom.String()
}

func tile(l engine.LampView) string {
	c := l.Color
	k := l.Brightness
	hex := fmt.Sprintf("#%02x%02x%02x", uint8(c.R*k*255), uint8(c.G*k*255), uint8(c.B*k*255))
	swatch := lipgloss.NewStyle().Background(lipgloss.Color(hex)).Width(14).Height(2).Render("")
	status := ok.Render(fmt.Sprintf("%2.0f fps ✓", l.Stats.FPS))
	switch {
	case !l.Enabled:
		status = dim.Render("выключена")
		swatch = lipgloss.NewStyle().Background(lipgloss.Color("#262626")).Width(14).Height(2).Render("")
	case l.Stats.Status == light.StatusLost:
		status = warn.Render("потеряна")
	case l.Stats.Status == light.StatusConnecting:
		status = dim.Render("подключаюсь…")
	}
	lines := []string{swatch, l.Info.Name, status}
	if l.Stats.Note != "" {
		lines = append(lines, dim.Render(l.Stats.Note))
	}
	return lipgloss.NewStyle().MarginRight(2).Width(16).Render(strings.Join(lines, "\n"))
}

func (m Model) pickerView() string {
	var b strings.Builder
	b.WriteString(accent.Render("Источник звука") + "\n\n")
	items := []string{"вся система"}
	for _, a := range m.apps {
		items = append(items, fmt.Sprintf("%s (pid %d)", a.Name, a.PID))
	}
	for i, it := range items {
		cursor := "  "
		if i == m.cursor {
			cursor = accent.Render("▸ ")
		}
		b.WriteString(cursor + it + "\n")
	}
	b.WriteString("\n" + dim.Render("[↑↓] выбрать  [enter] ок  [esc] отмена"))
	return b.String()
}

func (m Model) lampPickerView() string {
	var b strings.Builder
	b.WriteString(accent.Render("Лампы") + "\n\n")
	for i, l := range m.v.Lamps {
		cursor := "  "
		if i == m.lampCursor {
			cursor = accent.Render("▸ ")
		}
		mark := ok.Render("●")
		if !l.Enabled {
			mark = dim.Render("○")
		}
		b.WriteString(fmt.Sprintf("%s%s %-14s %s\n", cursor, mark, l.Info.Name, dim.Render(l.Info.Model)))
	}
	if len(m.v.Lamps) == 0 {
		b.WriteString(dim.Render("ламп нет — закрой и нажми d") + "\n")
	}
	b.WriteString("\n" + dim.Render("[↑↓] выбрать  [space] вкл/выкл  [esc] закрыть"))
	return b.String()
}
