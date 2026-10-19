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
	send := func(message, command uintptr) uintptr {
		t.Helper()
		var result uintptr
		ok, _, err := dlUser.NewProc("SendMessageTimeoutW").Call(hwnd, message, command, 0, 2, 3000, uintptr(unsafe.Pointer(&result)))
		if ok == 0 {
			t.Fatalf("native settings action timed out: %v", err)
		}
		return result
	}
	apply := func(change func(*configs.DesktopLyricsConfig)) {
		t.Helper()
		cfg := c.Settings()
		change(&cfg)
		if err := c.ApplySettings(cfg); err != nil {
			t.Fatal(err)
		}
	}
	c.Update(LyricLine{Text: "设置预览"}, LyricLine{Text: "下一句保持同步"}, 0, 0, true)
	c.Show()
	send(dlRefresh, 0)
	dlUser.NewProc("UpdateWindow").Call(hwnd)
	if hit := send(0x0084, 0); hit != 2 {
		t.Fatalf("unlocked hit test = %d", hit)
	}
	apply(func(cfg *configs.DesktopLyricsConfig) { cfg.Draggable = !cfg.Draggable })
	if hit := send(0x0084, 0); hit != 1 {
		t.Fatalf("locked hit test = %d, want interactive client", hit)
	}
	var before, after dlRect
	dlUser.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&before)))
	apply(func(cfg *configs.DesktopLyricsConfig) {
		cfg.FontSize = 48
		cfg.FontBold, cfg.FontItalic = true, true
		cfg.WindowAlpha = .75
		cfg.OneLineMode = true
	})
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
	frame := captureLyricFrame(t, hwnd)
	foundAlpha := false
	for _, pixel := range frame.pixels {
		if pixel>>24 == 191 {
			foundAlpha = true
			break
		}
	}
	if !foundAlpha {
		t.Fatal("per-pixel opacity did not update")
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
	apply(func(cfg *configs.DesktopLyricsConfig) { cfg.Draggable = !cfg.Draggable })
	apply(func(cfg *configs.DesktopLyricsConfig) { cfg.FontName = "Microsoft YaHei" })
	c.mu.Lock()
	cfg = c.cfg
	c.mu.Unlock()
	if !cfg.Draggable || cfg.FontName != "Microsoft YaHei" {
		t.Fatal("unlock/font selection failed")
	}
	apply(func(cfg *configs.DesktopLyricsConfig) { cfg.Vertical = true })
	frame = captureLyricFrame(t, hwnd)
	if frame.height <= frame.width {
		t.Fatal("vertical layout was not applied")
	}
	if folder := os.Getenv("MUSICFOX_LYRICS_ARTIFACT_DIR"); folder != "" {
		saveLyricTestPNG(t, frame, filepath.Join(folder, "lyrics-vertical.png"), true)
	}
	if err := c.ResetPosition(); err != nil {
		t.Fatal(err)
	}
	// Right-clicking must no longer open a separate settings window.
	send(0x007b, 0)
	if panel, _, _ := dlUser.NewProc("GetWindow").Call(hwnd, 6); panel != 0 && panel != hwnd {
		t.Fatal("native settings window still present")
	}
	// A failed save must leave both live state and the font unchanged.
	original := c.Settings()
	c.mu.Lock()
	c.configPath = filepath.Join(t.TempDir(), "missing", "config.toml")
	c.mu.Unlock()
	changed := original
	changed.FontSize = 72
	if err := c.ApplySettings(changed); err == nil {
		t.Fatal("missing config did not fail")
	}
	if c.Settings() != original {
		t.Fatal("failed save changed live configuration")
	}
	c.Close()
	if err := c.ApplySettings(original); err == nil {
		t.Fatal("closed controller accepted changes")
	}

}

type dlLogFont struct {
	height, width, escapement, orientation, weight                      int32
	italic, underline, strikeout, charset, output, clip, quality, pitch byte
	face                                                                [32]uint16
}
