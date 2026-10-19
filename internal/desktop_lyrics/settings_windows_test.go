//go:build windows

package desktop_lyrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	pixelfont "github.com/go-musicfox/go-musicfox/assets/fonts/fusion-pixel"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"golang.org/x/sys/windows"
)

func TestLyricSettingsNormalizeFontAndPreserveUnrelatedKeys(t *testing.T) {
	pixel := normalizeLyricSettings(configs.DesktopLyricsConfig{FontSize: 31})
	if pixel.FontName != pixelfont.Name || pixel.FontSize != 36 {
		t.Fatal("pixel font lost its integer grid")
	}
	standard := normalizeLyricSettings(configs.DesktopLyricsConfig{FontName: "Microsoft YaHei", FontSize: 31})
	if standard.FontSize != 31 {
		t.Fatal("system font size was rounded to a pixel grid")
	}
	next := pixel
	next.Draggable = !pixel.Draggable
	updates := desktopSettingUpdates(pixel, next)
	if len(updates) != 1 || updates[0].KeyPath[3] != "draggable" {
		t.Fatalf("lock changed unrelated settings: %+v", updates)
	}
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(dlLogFont{}) != 92 || unsafe.Sizeof(dlChooseFontData{}) != 104 || unsafe.Sizeof(dlChooseColorData{}) != 72) {
		t.Fatal("incorrect common dialog ABI")
	}
}

func TestWindowsLyricsNativeSettings(t *testing.T) {
	if os.Getenv("MUSICFOX_NATIVE_LYRICS_TEST") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[main.lyric.desktopLyrics]\nfontSize = 36 # keep comment\ndraggable = true\n[theme]\nactiveTheme = 'Cassette'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	controller := NewController(configs.DesktopLyricsConfig{Enable: true, FontSize: 36, Draggable: true, YPositionFactor: 0.15})
	if controller == nil {
		t.Fatal("native controller initialization failed")
	}
	c := controller.(*windowsController)
	t.Cleanup(c.Close)
	c.mu.Lock()
	c.configPath = path
	hwnd := c.hwnd
	c.mu.Unlock()
	menu := c.buildSettingsMenu()
	if menu == 0 {
		t.Fatal("settings menu was not created")
	}
	var label [128]uint16
	dlUser.NewProc("GetMenuStringW").Call(menu, dlLock, uintptr(unsafe.Pointer(&label[0])), 128, 0)
	dlUser.NewProc("DestroyMenu").Call(menu)
	if !strings.Contains(windows.UTF16ToString(label[:]), "锁定位置") {
		t.Fatal("lock entry missing from real native menu")
	}
	send := func(message, command uintptr) uintptr {
		t.Helper()
		var result uintptr
		ok, _, err := dlUser.NewProc("SendMessageTimeoutW").Call(hwnd, message, command, 0, 2, 3000, uintptr(unsafe.Pointer(&result)))
		if ok == 0 {
			t.Fatalf("native settings action timed out: %v", err)
		}
		return result
	}
	c.Update(LyricLine{Text: "设置预览"}, LyricLine{Text: "下一句保持同步"}, 0, 0, true)
	c.Show()
	send(dlRefresh, 0)
	dlUser.NewProc("UpdateWindow").Call(hwnd)
	if hit := send(0x0084, 0); hit != 2 {
		t.Fatalf("unlocked hit test = %d", hit)
	}
	send(0x0111, dlLock)
	if hit := send(0x0084, 0); hit != 1 {
		t.Fatalf("locked hit test = %d, want interactive client", hit)
	}
	var before, after dlRect
	dlUser.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&before)))
	send(0x0111, dlSizeBase+48)
	send(0x0111, dlBold)
	send(0x0111, dlItalic)
	send(0x0111, dlOpacityBase+75)
	send(0x0111, dlOneLine)
	dlUser.NewProc("UpdateWindow").Call(hwnd)
	dlUser.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&after)))
	if before.left != after.left || before.top != after.top {
		t.Fatal("locked lyrics moved after appearance changes")
	}
	c.mu.Lock()
	cfg, font, next := c.cfg, c.font, c.next
	c.mu.Unlock()
	if cfg.FontSize != 48 || !cfg.FontBold || !cfg.FontItalic || cfg.WindowAlpha != .75 || cfg.Draggable || !cfg.OneLineMode {
		t.Fatalf("settings did not apply: %+v", cfg)
	}
	if next == "" {
		t.Fatal("single-line mode discarded the pending next lyric")
	}
	var selected dlLogFont
	dlGDI.NewProc("GetObjectW").Call(font, unsafe.Sizeof(selected), uintptr(unsafe.Pointer(&selected)))
	if selected.height != -48 || selected.weight != 700 || selected.italic != 1 {
		t.Fatalf("GDI font did not change: %+v", selected)
	}
	var alpha byte
	var key, flags uint32
	dlUser.NewProc("GetLayeredWindowAttributes").Call(hwnd, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&alpha)), uintptr(unsafe.Pointer(&flags)))
	if alpha != 191 {
		t.Fatalf("opacity = %d", alpha)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fontSize = 48.0 # keep comment", "draggable = false", "fontBold = true", "fontItalic = true", "activeTheme = 'Cassette'"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("setting not persisted: %s\n%s", want, data)
		}
	}
	bindings := configs.EffectiveKeybindings
	t.Cleanup(func() { configs.EffectiveKeybindings = bindings })
	reloaded, err := configs.NewConfigFromTomlFile(path)
	if err != nil {
		t.Fatal(err)
	}
	restored := reloaded.Main.Lyric.DesktopLyrics
	if restored.FontSize != 48 || !restored.FontBold || !restored.FontItalic || restored.WindowAlpha != .75 || restored.Draggable || !restored.OneLineMode {
		t.Fatalf("saved settings did not reload: %+v", restored)
	}
	send(0x0111, dlLock)
	send(0x0111, dlFontBase+1)
	c.mu.Lock()
	cfg = c.cfg
	c.mu.Unlock()
	if !cfg.Draggable || cfg.FontName != "Microsoft YaHei" {
		t.Fatal("unlock/font selection failed")
	}
}

func TestNativeFontAndColorDialogCancellation(t *testing.T) {
	if os.Getenv("MUSICFOX_NATIVE_LYRICS_TEST") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	// Exercise the real common dialog ABI and cancellation without user input.
	cancel := windows.NewCallback(func(hwnd, message, _, _ uintptr) uintptr {
		if message == 0x0110 {
			dlUser.NewProc("PostMessageW").Call(hwnd, 0x0111, 2, 0)
		}
		return 0
	})
	var font dlLogFont
	font.height, font.weight = -36, 400
	copy(font.face[:], windows.StringToUTF16("Microsoft YaHei"))
	fontChoice := dlChooseFontData{font: &font, flags: 0x49, hook: cancel}
	fontChoice.size = uint32(unsafe.Sizeof(fontChoice))
	if selected, _, _ := dlDialogs.NewProc("ChooseFontW").Call(uintptr(unsafe.Pointer(&fontChoice))); selected != 0 {
		t.Fatal("font dialog cancellation selected a font")
	}
	if code, _, _ := dlDialogs.NewProc("CommDlgExtendedError").Call(); code != 0 {
		t.Fatalf("font dialog failed: %#x", code)
	}
	var custom [16]uint32
	colorChoice := dlChooseColorData{custom: &custom, flags: 0x13, hook: cancel}
	colorChoice.size = uint32(unsafe.Sizeof(colorChoice))
	if selected, _, _ := dlDialogs.NewProc("ChooseColorW").Call(uintptr(unsafe.Pointer(&colorChoice))); selected != 0 {
		t.Fatal("color dialog cancellation selected a color")
	}
	if code, _, _ := dlDialogs.NewProc("CommDlgExtendedError").Call(); code != 0 {
		t.Fatalf("color dialog failed: %#x", code)
	}
}
