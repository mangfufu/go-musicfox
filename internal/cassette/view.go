// Package cassette renders a terminal-only cassette deck from a playback snapshot.
package cassette

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	Background = "#151411"
	Foreground = "#D0CBBB"
	secondary  = "#999482"
	muted      = "#696556"
	accent     = "#D4774D"
	shell      = "#302E27"
	edge       = "#4B483E"
)

type Track struct{ Title, Artist string }
type Line struct{ Text, Translation string }
type Hint struct{ Key, Label string }

// Snapshot contains display data only. Position always comes from the player.
type Snapshot struct {
	Palette Palette
	Track
	Album, Quality, Mode, Status     string
	Position, Duration               time.Duration
	Volume, Current, Selected, Frame int
	Queue                            []Track
	Lyrics                           []Line
	LyricIndex                       int
	LyricMessage                     string
	Hints                            []Hint
}

func (r painter) ink(text, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(r.color(color))).Background(lipgloss.Color(r.Background)).Render(text)
}

// clean removes terminal commands and embedded control characters from metadata.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
}

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

func stamp(d time.Duration) string {
	s := int(max(0, d) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func (r painter) heading(n, title string) string { return r.ink(n+" — "+title, secondary) }

// Render produces exactly height rows of width terminal cells, including the footer.
func Render(s Snapshot, width, height int) string {
	return render(s, width, height, false)
}

// RenderDashboard fits the deck below the native library menu.
func RenderDashboard(s Snapshot, width, height int) string {
	return render(s, width, height, true)
}

func render(s Snapshot, width, height int, compact bool) string {
	r := painter{s.Palette.WithDefaults()}
	if width <= 0 || height <= 0 {
		return ""
	}
	margin := 2
	if width < 50 {
		margin = 1
	}
	w := max(1, width-2*margin)
	bodyHeight := max(0, height-4)
	leftWidth := w
	rightWidth := 0
	if width >= 80 && (height >= 22 || compact && height >= 19) {
		rightWidth = (w - 4) * 40 / 100
		leftWidth = w - 4 - rightWidth
	}
	left := r.trackLines(s, leftWidth, bodyHeight, compact)
	if bodyHeight > len(left)+1 {
		left = append(left, "", r.heading("04", "L Y R I C S"))
		left = append(left, r.lyricLines(s, bodyHeight-len(left))...)
	}
	var right []string
	if rightWidth > 0 {
		right = r.tape(s, rightWidth)
		right = append(right, "", r.heading("05", "U P  N E X T"))
		right = append(right, r.queueLines(s, max(0, bodyHeight-len(right)))...)
	}
	rows := make([]string, 0, height)
	for row := 0; row < bodyHeight; row++ {
		l, r := "", ""
		if row < len(left) {
			l = left[row]
		}
		if row < len(right) {
			r = right[row]
		}
		line := fit(l, leftWidth)
		if rightWidth > 0 {
			line += "    " + fit(r, rightWidth)
		}
		rows = append(rows, strings.Repeat(" ", margin)+line)
	}
	footer := []string{"", r.progress(s, w), r.controls(s.Hints, w), r.ink("CASSETTE / MUSICFOX", muted) + r.ink("   "+s.Status+fmt.Sprintf("   VOL %02d", s.Volume), secondary)}
	for _, line := range footer[max(0, 4-height):] {
		rows = append(rows, strings.Repeat(" ", margin)+line)
	}
	// Restore the page background after nested styles reset ANSI attributes.
	// Otherwise padding cells would inherit the user's terminal background.
	fr, fg, fb, _ := lipgloss.Color(r.Foreground).RGBA()
	br, bg, bb, _ := lipgloss.Color(r.Background).RGBA()
	base := fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", fr>>8, fg>>8, fb>>8, br>>8, bg>>8, bb>>8)
	restore := strings.NewReplacer("\x1b[m", base, "\x1b[0m", base)
	for i := range rows {
		rows[i] = base + restore.Replace(fit(rows[i], width)) + "\x1b[0m"
	}
	return strings.Join(rows, "\n")
}

func (r painter) trackLines(s Snapshot, width, height int, compact bool) []string {
	title := clean(s.Title)
	if title == "" {
		title = "等待放入一盘磁带"
	}
	artist := clean(s.Artist)
	if s.Album != "" {
		artist += "  —  " + clean(s.Album)
	}
	if artist == "" {
		artist = "返回歌单，选择一首歌开始播放"
	}
	index := 0
	if len(s.Queue) > 0 {
		index = s.Current + 1
	}
	meta := fmt.Sprintf("TYPE II   %s   %s   %02d / %03d", clean(s.Quality), clean(s.Mode), index, len(s.Queue))
	rows := []string{r.heading("02", "T R A C K"), "", r.ink(title, Foreground), r.ink(artist, secondary), "", r.ink(meta, secondary), "", r.heading("03", "C O U N T E R")}
	if compact {
		rows = []string{r.heading("02", "N O W  P L A Y I N G"), r.ink(title, Foreground), r.ink(artist, secondary), r.ink(meta, secondary), ""}
	}
	total, remaining := "--:--", "--:--"
	if s.Duration > 0 {
		total, remaining = stamp(s.Duration), stamp(max(0, s.Duration-s.Position))
	}
	clock := stamp(s.Position)
	if (height >= 19 || compact && height >= 12) && width >= 34 && len(clock) == 5 {
		digits := r.counter(clock)
		for i, line := range digits {
			if i == len(digits)-1 && width >= 47 {
				line += r.ink("  -"+remaining+" / "+total, secondary)
			}
			rows = append(rows, line)
		}
		if width < 47 {
			rows = append(rows, r.ink("-"+remaining+" / "+total, secondary))
		}
	} else {
		rows = append(rows, r.ink(clock+"  /  "+total+"   -"+remaining, Foreground))
	}
	if height < 12 {
		return []string{r.ink(title, Foreground), r.ink(artist, secondary), r.ink(clock+" / "+total, Foreground)}
	}
	return rows
}

var digitSegments = [10]uint8{0x3f, 0x06, 0x5b, 0x4f, 0x66, 0x6d, 0x7d, 0x07, 0x7f, 0x6f}

func (r painter) counter(value string) []string {
	rows := make([]string, 5)
	for _, ch := range value {
		if ch == ':' {
			for y := range rows {
				if y == 1 || y == 3 {
					rows[y] += " ▪  "
				} else {
					rows[y] += "    "
				}
			}
			continue
		}
		mask := digitSegments[ch-'0']
		on := func(bit uint, yes, no string) string {
			if mask&(1<<bit) != 0 {
				return yes
			}
			return no
		}
		rows[0] += on(0, " ▄▄▄ ", "     ") + " "
		rows[1] += on(5, "█", " ") + "   " + on(1, "█", " ") + " "
		rows[2] += on(5, "█", " ") + on(6, "▄▄▄", "   ") + on(1, "█", " ") + " "
		rows[3] += on(4, "█", " ") + "   " + on(2, "█", " ") + " "
		rows[4] += on(4, "█", " ") + on(3, "▄▄▄", "   ") + on(2, "█", " ") + " "
	}
	for i := range rows {
		rows[i] = r.ink(rows[i], Foreground)
	}
	return rows
}

func (r painter) lyricLines(s Snapshot, count int) []string {
	if count <= 0 {
		return nil
	}
	if len(s.Lyrics) == 0 {
		msg := s.LyricMessage
		if msg == "" {
			msg = "暂无歌词 / 纯音乐"
		}
		return []string{r.ink(clean(msg), secondary)}
	}
	var rows []string
	var originals []bool
	activeRow := 0
	for i, line := range s.Lyrics {
		prefix, color := "  ", muted
		if i == s.LyricIndex {
			prefix, color, activeRow = "› ", Foreground, len(rows)
		}
		rows = append(rows, r.ink(prefix, accent)+r.ink(clean(line.Text), color))
		originals = append(originals, true)
		if line.Translation != "" {
			transColor := muted
			if i == s.LyricIndex {
				transColor = secondary
			}
			rows = append(rows, r.ink("  "+clean(line.Translation), transColor))
			originals = append(originals, false)
		}
	}
	start := max(0, activeRow-count/3)
	if start > 0 && !originals[start] {
		start--
	}
	return rows[start:min(len(rows), start+count)]
}

func (r painter) queueLines(s Snapshot, count int) []string {
	if count <= 0 {
		return nil
	}
	if len(s.Queue) == 0 {
		return []string{r.ink("尚未添加歌曲", secondary)}
	}
	selected := min(max(0, s.Selected), len(s.Queue)-1)
	start := max(0, selected-count/2)
	start = min(start, max(0, len(s.Queue)-count))
	var rows []string
	for i := start; i < min(len(s.Queue), start+count); i++ {
		selector, playing, color := " ", " ", secondary
		if i == selected {
			selector, color = "›", Foreground
		}
		if i == s.Current {
			playing = "▶"
		}
		rows = append(rows, r.ink(selector+playing, accent)+r.ink(fmt.Sprintf("%02d  %s — %s", i+1, clean(s.Queue[i].Title), clean(s.Queue[i].Artist)), color))
	}
	return rows
}

func (r painter) progress(s Snapshot, width int) string {
	position := 0
	if s.Duration > 0 {
		position = int(float64(width-1) * math.Max(0, math.Min(1, float64(s.Position)/float64(s.Duration))))
	}
	return r.ink(strings.Repeat("━", position)+"●", accent) + r.ink(strings.Repeat("─", max(0, width-position-1)), edge)
}

func (r painter) controls(hints []Hint, width int) string {
	var parts []string
	used := 0
	for _, h := range hints {
		if h.Key == "" {
			continue
		}
		text := clean(h.Key) + " " + clean(h.Label)
		length := ansi.StringWidth(text)
		if used+length > width {
			continue
		}
		parts = append(parts, r.ink(clean(h.Key), Foreground)+r.ink(" "+clean(h.Label), secondary))
		used += length + 3
	}
	return strings.Join(parts, "   ")
}

// tape uses two square pixels per terminal cell; it needs no image protocol.
func (r painter) tape(s Snapshot, width int) []string {
	w := min(width, 48)
	h := 30
	if w < 36 {
		h = 24
	}
	pixels := make([][]string, h)
	for y := range pixels {
		pixels[y] = make([]string, w)
		for x := range pixels[y] {
			c := shell
			switch {
			case (x < 2 || x >= w-2) && (y < 2 || y >= h-2):
				c = Background
			case x == 0 || y == h-1 || x == w-1:
				c = "#211F1A"
			case y < 2 || x == 1:
				c = edge
			case y == 2:
				c = "#3C3931"
			case y >= 4 && y <= 8 && x >= 3 && x < w-3:
				c = "#201F1A"
			case y == 9 && x >= 3 && x < w-3:
				c = accent
			case y >= 11 && y < h-6 && x >= 3 && x < w-3:
				c = "#1C1B17"
			case y == h-5 && x > 4 && x < w-5:
				c = edge
			}
			pixels[y][x] = c
		}
	}
	cy := float64(11+h-7) / 2
	radius := float64(h-17) / 2
	for _, cx := range []float64{float64(w) * .27, float64(w) * .73} {
		for y := 10; y < h-5; y++ {
			for x := 3; x < w-3; x++ {
				dx, dy := float64(x)-cx, float64(y)-cy
				r := math.Hypot(dx, dy)
				angle := math.Atan2(dy, dx) + float64(s.Frame%8)*math.Pi/4
				switch {
				case r < 1.2:
					pixels[y][x] = "#171611"
				case r < radius*.6:
					pixels[y][x] = "#C5C1B5"
					if math.Cos(angle*3) > .2 && r > radius*.25 {
						pixels[y][x] = "#716E63"
					}
				case r < radius*.78:
					pixels[y][x] = "#716A5A"
				case r < radius:
					pixels[y][x] = "#574136"
				}
			}
		}
	}
	for _, x := range []int{3, w - 4, w/2 - 6, w/2 + 6} {
		pixels[h-3][x] = "#161510"
		pixels[h-4][x] = "#161510"
	}
	for x := w/2 - 2; x <= w/2+2; x++ {
		pixels[h-2][x] = Background
		pixels[h-1][x] = Background
	}
	rows := make([]string, h/2)
	for y := 0; y < h; y += 2 {
		var row strings.Builder
		for x := 0; x < w; x++ {
			row.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(r.color(pixels[y][x]))).Background(lipgloss.Color(r.color(pixels[y+1][x]))).Render("▀"))
		}
		rows[y/2] = row.String()
	}
	label := lipgloss.NewStyle().Foreground(lipgloss.Color(Foreground)).Background(lipgloss.Color("#201F1A"))
	for i, text := range []string{"A  " + clean(s.Title), "   " + clean(s.Artist)} {
		right := []string{"C-120", "TYPE II"}[i]
		rows[2+i] = lipgloss.NewStyle().Background(lipgloss.Color(r.color(shell))).Render("   ") + label.Render(fit(text, w-7-len(right))+" "+right) + lipgloss.NewStyle().Background(lipgloss.Color(r.color(shell))).Render("   ")
	}
	return rows
}
