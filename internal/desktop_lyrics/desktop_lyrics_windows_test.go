//go:build windows

package desktop_lyrics

import (
	"os"
	"testing"
	"time"
	"unsafe"

	pixelfont "github.com/go-musicfox/go-musicfox/assets/fonts/fusion-pixel"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"golang.org/x/sys/windows"
)

func TestWindowsLyricContent(t *testing.T) {
	if NewController(configs.DesktopLyricsConfig{}) != nil {
		t.Fatal("disabled lyrics created a window")
	}
	line := LyricLine{Text: "原文\n译文", Words: []LyricWord{{Word: "timed words"}}}
	if desktopLineText(line) != line.Text {
		t.Fatal("translation was lost")
	}
	if desktopLineText(LyricLine{Words: []LyricWord{{Word: "像素"}, {Word: "歌词"}}}) != "像素歌词" {
		t.Fatal("YRC fallback was lost")
	}
	if lyricColor("#123456", 0) != 0x563412 || lyricColor("#invalid", 7) != 7 {
		t.Fatal("incorrect GDI color")
	}
}

func TestWindowsLyricsNativeLifecycle(t *testing.T) {
	if os.Getenv("MUSICFOX_NATIVE_LYRICS_TEST") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	controller := NewController(configs.DesktopLyricsConfig{Enable: true, FontSize: 36, Draggable: true, TextColor: "#E5C1B3", ShadowColor: "#101010", YPositionFactor: 0.15})
	if controller == nil {
		t.Fatal("native controller initialization failed")
	}
	c := controller.(*windowsController)
	t.Cleanup(c.Close)
	c.mu.Lock()
	hwnd := c.hwnd
	c.mu.Unlock()
	// Check the real selected GDI font, not only the requested family name.
	dc, _, _ := dlGDI.NewProc("CreateCompatibleDC").Call(0)
	old, _, _ := dlGDI.NewProc("SelectObject").Call(dc, c.font)
	var face [128]uint16
	dlGDI.NewProc("GetTextFaceW").Call(dc, 128, uintptr(unsafe.Pointer(&face[0])))
	dlGDI.NewProc("SelectObject").Call(dc, old)
	dlGDI.NewProc("DeleteDC").Call(dc)
	if name := windows.UTF16ToString(face[:]); name != pixelfont.Name {
		t.Fatalf("font fallback: %q", name)
	}
	waitVisible := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			visible, _, _ := dlUser.NewProc("IsWindowVisible").Call(hwnd)
			if (visible != 0) == want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("native visibility did not become %v", want)
	}
	c.Update(LyricLine{Text: "桌面歌词 · 像素字体\nDesktop lyrics / pixel font"}, LyricLine{Text: "拖动文字可以移动窗口"}, 0, 0, true)
	c.Show()
	waitVisible(true)
	styleIndex := int32(-20)
	style, _, _ := dlUser.NewProc("GetWindowLongW").Call(hwnd, uintptr(styleIndex))
	if style&0x08080088 != 0x08080088 {
		t.Fatalf("missing overlay styles: %#x", style)
	}
	dlUser.NewProc("UpdateWindow").Call(hwnd)
	windowDC, _, _ := dlUser.NewProc("GetDC").Call(hwnd)
	foundText := false
	for y := 12; y < 65 && !foundText; y += 2 {
		for x := 20; x < int(c.width)-20; x += 2 {
			color, _, _ := dlGDI.NewProc("GetPixel").Call(windowDC, uintptr(x), uintptr(y))
			if color == 0xB3C1E5 {
				foundText = true
				break
			}
		}
	}
	dlUser.NewProc("ReleaseDC").Call(hwnd, windowDC)
	if !foundText {
		t.Fatal("native paint produced no foreground lyric pixels")
	}
	if os.Getenv("MUSICFOX_LYRICS_PREVIEW") == "1" {
		time.Sleep(45 * time.Second)
	}
	c.Hide()
	waitVisible(false)
	c.Show()
	waitVisible(true)
	c.Update(LyricLine{Text: "下一句\nNext line"}, LyricLine{}, 1, 1000, false)
	c.Close()
	if c.IsVisible() {
		t.Fatal("closed controller is visible")
	}
	if alive, _, _ := dlUser.NewProc("IsWindow").Call(hwnd); alive != 0 {
		t.Fatal("native window leaked")
	}
	c.Show()
	c.Hide()
	c.Update(LyricLine{}, LyricLine{}, -1, 0, false)
}
