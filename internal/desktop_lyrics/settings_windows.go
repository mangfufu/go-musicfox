//go:build windows

package desktop_lyrics

import (
	"fmt"
	"math"
	"strings"
	"unsafe"

	pixelfont "github.com/go-musicfox/go-musicfox/assets/fonts/fusion-pixel"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"golang.org/x/sys/windows"
)

func normalizeLyricSettings(cfg configs.DesktopLyricsConfig) configs.DesktopLyricsConfig {
	if cfg.FontName == "" {
		cfg.FontName = pixelfont.Name
	}
	if cfg.FontSize <= 0 || math.IsNaN(cfg.FontSize) || math.IsInf(cfg.FontSize, 0) {
		cfg.FontSize = 24
	}
	if strings.EqualFold(cfg.FontName, pixelfont.Name) {
		cfg.FontSize = math.Round(cfg.FontSize/12) * 12
	}
	cfg.FontSize = math.Round(max(12, min(96, cfg.FontSize)))
	if cfg.WindowAlpha <= 0 || cfg.WindowAlpha > 1 {
		cfg.WindowAlpha = 1
	}
	return cfg
}

func createLyricFont(cfg configs.DesktopLyricsConfig) uintptr {
	weight, italic, quality := uintptr(400), uintptr(0), uintptr(4)
	if cfg.FontBold {
		weight = 700
	}
	if cfg.FontItalic {
		italic = 1
	}
	if strings.EqualFold(cfg.FontName, pixelfont.Name) {
		quality = 3
	}
	face := windows.StringToUTF16Ptr(cfg.FontName)
	font, _, _ := dlGDI.NewProc("CreateFontW").Call(uintptr(-int32(cfg.FontSize)), 0, 0, 0, weight, italic, 0, 0, 1, 0, 0, quality, 0, uintptr(unsafe.Pointer(face)))
	return font
}

func desktopSettingUpdates(before, after configs.DesktopLyricsConfig) []configs.TOMLUpdate {
	var updates []configs.TOMLUpdate
	for _, item := range []struct {
		key       string
		old, next any
	}{
		{"fontName", before.FontName, after.FontName}, {"fontSize", before.FontSize, after.FontSize},
		{"fontBold", before.FontBold, after.FontBold}, {"fontItalic", before.FontItalic, after.FontItalic},
		{"textColor", before.TextColor, after.TextColor}, {"shadowColor", before.ShadowColor, after.ShadowColor},
		{"windowAlpha", before.WindowAlpha, after.WindowAlpha}, {"draggable", before.Draggable, after.Draggable},
		{"oneLineMode", before.OneLineMode, after.OneLineMode},
		{"vertical", before.Vertical, after.Vertical},
	} {
		if item.old != item.next {
			updates = append(updates, configs.TOMLUpdate{KeyPath: []string{"main", "lyric", "desktopLyrics", item.key}, Value: item.next})
		}
	}
	return updates
}

func (c *windowsController) applySettings(hwnd uintptr, next configs.DesktopLyricsConfig) error {
	updates := desktopSettingUpdates(c.cfg, next)
	if len(updates) == 0 {
		return nil
	}
	font := createLyricFont(next)
	if font == 0 {
		return fmt.Errorf("无法创建所选字体")
	}
	c.mu.Lock()
	path, closed := c.configPath, c.closed
	c.mu.Unlock()
	if closed {
		dlGDI.NewProc("DeleteObject").Call(font)
		return nil
	}
	if err := configs.SetTOMLValues(path, updates); err != nil {
		dlGDI.NewProc("DeleteObject").Call(font)
		return fmt.Errorf("设置未保存，已保留原设置：%w", err)
	}
	c.mu.Lock()
	oldFont := c.font
	c.cfg, c.font, c.fontSize = next, font, int32(next.FontSize)
	c.mu.Unlock()
	dlGDI.NewProc("DeleteObject").Call(oldFont)
	if err := c.renderLyrics(hwnd); err != nil {
		return err
	}
	c.persistPosition(hwnd)
	return nil
}

// Settings returns a snapshot safe for the terminal event loop.
func (c *windowsController) Settings() configs.DesktopLyricsConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

type lyricSettingsRequest struct {
	config configs.DesktopLyricsConfig
	reset  bool
	result chan error
}

func (c *windowsController) ApplySettings(cfg configs.DesktopLyricsConfig) error {
	return c.requestSettings(lyricSettingsRequest{config: cfg})
}

func (c *windowsController) ResetPosition() error {
	return c.requestSettings(lyricSettingsRequest{reset: true})
}

// Marshal configuration and GDI changes onto the lyric window's owning thread.
func (c *windowsController) requestSettings(request lyricSettingsRequest) error {
	request.result = make(chan error, 1)
	select {
	case <-c.done:
		return fmt.Errorf("桌面歌词窗口已关闭")
	case c.settings <- request:
	}
	c.mu.Lock()
	hwnd, closed := c.hwnd, c.closed
	c.mu.Unlock()
	if closed {
		return fmt.Errorf("桌面歌词窗口已关闭")
	}
	if ok, _, err := dlUser.NewProc("PostMessageW").Call(hwnd, dlSettings, 0, 0); ok == 0 {
		<-c.settings
		return fmt.Errorf("提交歌词设置失败：%w", err)
	}
	select {
	case err := <-request.result:
		return err
	case <-c.done:
		return fmt.Errorf("桌面歌词窗口已关闭")
	}
}
