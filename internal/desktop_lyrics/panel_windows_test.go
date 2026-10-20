//go:build windows

package desktop_lyrics

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/storage"
)

func nativeLyricDPI() func() {
	runtime.LockOSThread()
	previous, _, _ := dlUser.NewProc("SetThreadDpiAwarenessContext").Call(^uintptr(3))
	return func() {
		if previous != 0 {
			dlUser.NewProc("SetThreadDpiAwarenessContext").Call(previous)
		}
		runtime.UnlockOSThread()
	}
}

func nativeLyricMessage(t *testing.T, hwnd, message, wParam, lParam uintptr) uintptr {
	t.Helper()
	var result uintptr
	if ok, _, err := dlUser.NewProc("SendMessageTimeoutW").Call(hwnd, message, wParam, lParam, 2, 3000, uintptr(unsafe.Pointer(&result))); ok == 0 {
		t.Fatalf("native message %#x failed: %v", message, err)
	}
	return result
}

func captureLyricFrame(t *testing.T, hwnd uintptr) *lyricSurface {
	t.Helper()
	restore := nativeLyricDPI()
	defer restore()
	nativeLyricMessage(t, hwnd, 0x000f, 0, 0)
	var bounds dlRect
	dlUser.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
	frame, err := newLyricSurface(bounds.right, bounds.bottom)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(frame.close)
	nativeLyricMessage(t, hwnd, 0x0318, frame.dc, 0)
	dlGDI.NewProc("GdiFlush").Call()
	return frame
}

func saveLyricTestPNG(t *testing.T, frame *lyricSurface, path string, transparent bool) {
	t.Helper()
	output := image.NewNRGBA(image.Rect(0, 0, int(frame.width), int(frame.height)))
	for i, pixel := range frame.pixels {
		r, g, b, a := uint8(pixel>>16), uint8(pixel>>8), uint8(pixel), uint8(255)
		if transparent {
			a = uint8(pixel >> 24)
			if a > 0 {
				r = uint8(uint32(r) * 255 / uint32(a))
				g = uint8(uint32(g) * 255 / uint32(a))
				b = uint8(uint32(b) * 255 / uint32(a))
			}
		}
		output.SetNRGBA(i%int(frame.width), i/int(frame.width), color.NRGBA{R: r, G: g, B: b, A: a})
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, output); err != nil {
		t.Fatal(err)
	}
}

func TestVerticalLyricsKeepClustersAndTranslation(t *testing.T) {
	got := verticalLyricColumns("天地玄黄\n宇宙洪荒", 3)
	want := [][]string{{"天", "地", "玄"}, {"黄"}, {"宇", "宙", "洪"}, {"荒"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v", got)
	}
	got = verticalLyricColumns("e\u0301👨‍👩‍👧‍👦中", 2)
	if len(got) != 2 || got[0][0] != "e\u0301" || got[0][1] != "👨‍👩‍👧‍👦" || got[1][0] != "中" {
		t.Fatal("grapheme cluster split across rows")
	}
}

func TestWindowsLyricsTerminalBridgeAndAlignment(t *testing.T) {
	if os.Getenv("MUSICFOX_NATIVE_LYRICS_TEST") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[main.lyric.desktopLyrics]\nenable = true\nfontSize = 36\ndraggable = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := NewController(configs.DesktopLyricsConfig{Enable: true, FontSize: 36, TextColor: "#FFFFFF", ShadowColor: "#101010", WindowAlpha: 1}).(*windowsController)
	defer c.Close()
	c.mu.Lock()
	c.configPath = path
	c.mu.Unlock()
	c.Update(LyricLine{Text: "一二三四五六七八九十"}, LyricLine{Text: "短句"}, 0, 0, true)
	c.Show()
	nativeLyricMessage(t, c.hwnd, dlRefresh, 0, 0)
	var starts []int
	var origin dlRect
	for index, align := range []string{"left", "center", "right"} {
		cfg, err := c.EditSettings(SettingsChange{Key: "textAlign", Value: align})
		if err != nil || cfg.TextAlign != align {
			t.Fatalf("alignment: %+v %v", cfg, err)
		}
		frame := captureLyricFrame(t, c.hwnd)
		first := int(frame.width)
		for y := int(frame.height) / 2; y < int(frame.height); y++ {
			for x := 0; x < int(frame.width); x++ {
				if frame.pixels[y*int(frame.width)+x]>>24 > 2 && x < first {
					first = x
				}
			}
		}
		starts = append(starts, first)
		var bounds dlRect
		dlUser.NewProc("GetWindowRect").Call(c.hwnd, uintptr(unsafe.Pointer(&bounds)))
		if index == 0 {
			origin = bounds
		} else if bounds.left != origin.left || bounds.top != origin.top {
			t.Fatal("text alignment moved the locked window")
		}
		if folder := os.Getenv("MUSICFOX_LYRICS_ARTIFACT_DIR"); folder != "" {
			saveLyricTestPNG(t, frame, filepath.Join(folder, "lyrics-"+align+".png"), true)
		}
	}
	if !(starts[0] < starts[1] && starts[1] < starts[2]) {
		t.Fatalf("native text did not align: %v", starts)
	}
	cfg, err := c.EditSettings(SettingsChange{Key: "textColor", Value: "#A9DCF5"})
	if err != nil || cfg.TextColor != "#A9DCF5" || len(cfg.RecentColors) == 0 || cfg.RecentColors[0] != "#A9DCF5" {
		t.Fatalf("color bridge: %+v %v", cfg, err)
	}
	c.Hide()
	if _, err = c.EditSettings(SettingsChange{Key: "fontSize", Step: 1}); err != nil {
		t.Fatal(err)
	}
	if c.IsVisible() {
		t.Fatal("editing enabled hidden lyrics")
	}
	for _, change := range []SettingsChange{{Key: "vertical"}, {Key: "fontBold"}, {Key: "draggable"}, {Key: "windowAlpha", Step: -1}, {Key: "preset", Value: "2"}} {
		if _, err = c.EditSettings(change); err != nil {
			t.Fatal(err)
		}
	}
	bindings := configs.EffectiveKeybindings
	defer func() { configs.EffectiveKeybindings = bindings }()
	loaded, err := configs.NewConfigFromTomlFile(path)
	if err != nil || loaded.Main.Lyric.DesktopLyrics.TextAlign != "right" || !loaded.Main.Lyric.DesktopLyrics.Vertical {
		t.Fatalf("settings reload: %v", err)
	}
	before := c.Settings()
	if _, err = c.EditSettings(SettingsChange{Key: "textAlign", Value: "invalid"}); err == nil {
		t.Fatal("invalid alignment accepted")
	}
	if c.Settings().TextAlign != before.TextAlign {
		t.Fatal("invalid edit changed settings")
	}
}
func TestWindowsLyricsNativePositionRestart(t *testing.T) {
	if os.Getenv("MUSICFOX_NATIVE_LYRICS_TEST") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	if os.Getenv("MUSICFOX_POSITION_TEST_CHILD") != "1" {
		// App paths are initialized once; a child process isolates the real database.
		cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsLyricsNativePositionRestart$", "-test.timeout=15s")
		cmd.Env = append(os.Environ(), "MUSICFOX_ROOT="+t.TempDir(), "MUSICFOX_POSITION_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("position restart test: %v\n%s", err, output)
		}
		return
	}
	restore := nativeLyricDPI()
	defer restore()
	storage.DBManager = &storage.LocalDBManager{}
	t.Cleanup(func() { _ = storage.DBManager.Close(); storage.DBManager = nil })
	saved := storage.DesktopLyricsPosition{XFactor: .5, YFactor: .5, WindowsOrigin: &storage.DesktopLyricsOrigin{XFactor: .2, YFactor: .1}}
	if err := storage.NewTable().SetByKVModel(saved, saved); err != nil {
		t.Fatal(err)
	}
	var previous dlRect
	for i, size := range []float64{36, 72} {
		controller := NewController(configs.DesktopLyricsConfig{Enable: true, FontSize: size, Vertical: i == 0, Draggable: false})
		if controller == nil {
			t.Fatal("create controller")
		}
		c := controller.(*windowsController)
		c.Update(LyricLine{Text: "固定位置"}, LyricLine{}, 0, 0, true)
		c.Show()
		nativeLyricMessage(t, c.hwnd, dlRefresh, 0, 0)
		nativeLyricMessage(t, c.hwnd, 0x000f, 0, 0)
		var bounds dlRect
		dlUser.NewProc("GetWindowRect").Call(c.hwnd, uintptr(unsafe.Pointer(&bounds)))
		if i > 0 && (bounds.left != previous.left || bounds.top != previous.top) {
			t.Fatal("font/layout change moved the restored locked origin")
		}
		if i == 0 {
			nativeLyricMessage(t, c.hwnd, 0x0232, 0, 0)
		} // Persist through the actual window procedure.
		previous = bounds
		c.Close()
	}
}
