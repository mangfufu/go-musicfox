package cassette

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderFitsTerminal(t *testing.T) {
	s := Snapshot{Track: Track{Title: strings.Repeat("中文 é 🎵 ", 20), Artist: "测试歌手"},
		Duration: 4 * time.Minute, Position: 24 * time.Second, Current: 1, Selected: 2,
		Queue:  []Track{{Title: "one"}, {Title: "two"}, {Title: "three"}},
		Lyrics: []Line{{Text: "第一句", Translation: "first"}, {Text: "当前句", Translation: "current"}}, LyricIndex: 1,
		Hints: []Hint{{"Esc", "BACK"}, {"Space", "PLAY"}}, Status: "PAUSED"}
	for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}, {79, 24}, {60, 18}, {40, 10}, {10, 3}, {1, 1}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			view := Render(s, size[0], size[1])
			rows := strings.Split(view, "\n")
			if len(rows) != size[1] {
				t.Fatalf("rows = %d, want %d", len(rows), size[1])
			}
			for i, row := range rows {
				if w := ansi.StringWidth(row); w != size[0] {
					t.Fatalf("row %d width = %d, want %d", i, w, size[0])
				}
			}
			if size[0] >= 40 && size[1] >= 4 && !strings.Contains(ansi.Strip(rows[len(rows)-2]), "BACK") {
				t.Fatal("back control is hidden")
			}
		})
	}
}

func TestLyricViewportKeepsCurrentTranslation(t *testing.T) {
	s := Snapshot{LyricIndex: 75}
	for i := 0; i < 100; i++ {
		s.Lyrics = append(s.Lyrics, Line{fmt.Sprintf("line-%d", i), fmt.Sprintf("translation-%d", i)})
	}
	rows := (painter{DefaultPalette()}).lyricLines(s, 8)
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if len(rows) > 8 || !strings.Contains(plain, "> line-75\n  translation-75") {
		t.Fatalf("current lyric lost: %s", plain)
	}
}

func TestQueueSelectionAndPlaybackAreIndependent(t *testing.T) {
	s := Snapshot{Current: 0, Selected: 2, Queue: []Track{{Title: "playing"}, {Title: "middle"}, {Title: "selected"}}}
	got := ansi.Strip(strings.Join((painter{DefaultPalette()}).queueLines(s, 4), "\n"))
	if !strings.Contains(got, " ▶01") || !strings.Contains(got, "> 03") {
		t.Fatal(got)
	}
}

func TestCounterAndTapeDimensionsStayStable(t *testing.T) {
	for _, value := range []string{"00:00", "11:11", "28:59", "59:59"} {
		for _, row := range (painter{DefaultPalette()}).counter(value) {
			if ansi.StringWidth(row) != 28 {
				t.Fatalf("unstable counter: %q", value)
			}
		}
	}
	for frame := 0; frame < 8; frame++ {
		for _, w := range []int{28, 36, 44, 48} {
			for _, row := range (painter{DefaultPalette()}).tape(Snapshot{Frame: frame}, w) {
				if ansi.StringWidth(row) != w {
					t.Fatalf("tape frame %d width != %d", frame, w)
				}
			}
		}
	}
	if (painter{DefaultPalette()}).tape(Snapshot{Frame: 0}, 44)[8] == (painter{DefaultPalette()}).tape(Snapshot{Frame: 1}, 44)[8] {
		t.Fatal("reels do not animate")
	}
}

func TestEdgeCasesAndUntrustedMetadata(t *testing.T) {
	for _, s := range []Snapshot{{}, {Position: -time.Minute}, {Position: 2 * time.Hour, Duration: time.Hour}, {Duration: -1},
		{Track: Track{Title: "title\n\x1b[2J\ttail"}}} {
		view := Render(s, 80, 24)
		if strings.Contains(view, "\x1b[2J") {
			t.Fatal("metadata escaped the layout")
		}
		if len(strings.Split(view, "\n")) != 24 {
			t.Fatal("metadata changed row count")
		}
	}
	if got := stamp(2*time.Hour + 3*time.Second); got != "02:00:03" {
		t.Fatal(got)
	}
	if Render(Snapshot{}, 0, 10) != "" {
		t.Fatal("zero width should be empty")
	}
}

func TestDashboardPaletteAndCompactSizes(t *testing.T) {
	palettes := []Palette{DefaultPalette(), {Background: "#EEE7D9", Foreground: "#423C32", Accent: "#A04D31"}}
	for _, palette := range palettes {
		for _, size := range [][2]int{{120, 30}, {100, 20}, {80, 14}, {40, 6}, {1, 1}} {
			view := RenderDashboard(Snapshot{Palette: palette, Track: Track{Title: "中文测试"}}, size[0], size[1])
			rows := strings.Split(view, "\n")
			if len(rows) != size[1] {
				t.Fatal("dashboard height changed")
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != size[0] {
					t.Fatal("dashboard width changed")
				}
			}
			if palette.Background == "#EEE7D9" && !strings.Contains(view, "48;2;238;231;217") {
				t.Fatal("dashboard did not use the selected background")
			}
		}
	}
}
