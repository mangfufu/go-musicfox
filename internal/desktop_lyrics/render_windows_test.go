//go:build windows

package desktop_lyrics

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
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
