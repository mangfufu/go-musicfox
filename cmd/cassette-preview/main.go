// cassette-preview prints a deterministic visual fixture, without login or audio.
package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/go-musicfox/go-musicfox/internal/cassette"
)

func main() {
	w := flag.Int("width", 120, "terminal columns")
	h := flag.Int("height", 40, "terminal rows")
	frame := flag.Int("frame", 0, "reel frame (0-7)")
	flag.Parse()
	s := cassette.Snapshot{
		Track: cassette.Track{Title: "Midnight, Side A", Artist: "The Quiet Hours"}, Album: "Room Tapes / Vol. 01",
		Quality: "目标 无损", Mode: "列表循环", Status: "PREVIEW / 示例数据", Volume: 80,
		Position: 24 * time.Second, Duration: 238 * time.Second, Current: 0, Selected: 1, Frame: *frame, LyricIndex: 4,
		Hints: []cassette.Hint{{"Esc", "BACK"}, {"Space", "PLAY"}, {"[", "PREV"}, {"]", "NEXT"}, {"−/+", "VOL"}, {"p", "MODE"}, {"↑/↓", "SELECT"}, {"Enter", "LOAD"}},
	}
	for _, title := range []string{"Midnight, Side A", "The Last Train Home", "雨停之后", "Soft Focus", "Somewhere, Slowly", "Blue Hour", "Letters on the Table", "海岸线", "A Room with No Clocks", "First Light", "Stay for a While", "Side B"} {
		s.Queue = append(s.Queue, cassette.Track{Title: title, Artist: "The Quiet Hours"})
	}
	for _, line := range [][2]string{
		{"The city folds its paper wings", "城市收起纸做的翅膀"},
		{"A little light beneath the door", "门缝里留着一点微光"},
		{"We leave the windows open", "我们让窗户敞着"},
		{"And let the evening in", "让傍晚慢慢走进来"},
		{"Turn the tape and stay a while", "翻过磁带，再待一会儿"},
		{"There is nowhere else to be", "此刻无需赶往别处"},
		{"The last train hums across the bridge", "末班车轻轻驶过桥面"},
		{"Its lights dissolve into the sea", "灯火渐渐融进海里"},
		{"We keep a space for silence", "我们为安静留一个位置"},
		{"Between the songs we used to know", "在那些熟悉的歌之间"},
		{"A quiet room, a turning reel", "一间静室，缓缓转动的卷轴"},
		{"And nothing left to hurry for", "再没有需要匆忙的理由"},
	} {
		s.Lyrics = append(s.Lyrics, cassette.Line{Text: line[0], Translation: line[1]})
	}
	fmt.Print(cassette.Render(s, *w, *h))
}
