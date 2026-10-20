//go:build windows

package desktop_lyrics

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-musicfox/go-musicfox/internal/configs"
)

type lyricSettingsRequest struct {
	change SettingsChange
	result chan lyricSettingsResult
}

type lyricSettingsResult struct {
	cfg configs.DesktopLyricsConfig
	err error
}

func (c *windowsController) Settings() configs.DesktopLyricsConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg := c.cfg
	cfg.RecentColors = slices.Clone(cfg.RecentColors)
	return cfg
}

func (c *windowsController) SetSettingsHandler(handler func()) {
	c.mu.Lock()
	c.settingsHandler = handler
	c.mu.Unlock()
}

func (c *windowsController) requestTerminalSettings() {
	c.mu.Lock()
	handler := c.settingsHandler
	c.mu.Unlock()
	if handler != nil {
		handler()
	}
}

func (c *windowsController) EditSettings(change SettingsChange) (configs.DesktopLyricsConfig, error) {
	request := &lyricSettingsRequest{change, make(chan lyricSettingsResult, 1)}
	c.mu.Lock()
	hwnd, closed := c.hwnd, c.closed
	c.mu.Unlock()
	if closed {
		return c.Settings(), fmt.Errorf("歌词窗口已关闭")
	}
	select {
	case c.settingsQueue <- request:
	case <-c.done:
		return c.Settings(), fmt.Errorf("歌词窗口已关闭")
	}
	if ok, _, err := dlUser.NewProc("PostMessageW").Call(hwnd, dlEditSettings, 0, 0); ok == 0 {
		return c.Settings(), fmt.Errorf("无法更新歌词设置：%w", err)
	}
	select {
	case result := <-request.result:
		return result.cfg, result.err
	case <-c.done:
		return c.Settings(), fmt.Errorf("歌词窗口已关闭")
	}
}

func (c *windowsController) editSettingsOnThread(change SettingsChange) error {
	command := uintptr(0)
	switch change.Key {
	case "fontSize":
		command = dlLarger
		if change.Step < 0 {
			command = dlSmaller
		}
	case "fontDialog":
		command = dlChooseFont
	case "textColorDialog":
		command = dlTextColor
	case "shadowColorDialog":
		command = dlShadowColor
	case "fontBold":
		command = dlBold
	case "fontItalic":
		command = dlItalic
	case "draggable":
		command = dlLock
	case "oneLineMode":
		command = dlOneLine
	case "vertical":
		command = dlVertical
		if c.cfg.Vertical {
			command = dlHorizontal
		}
	case "resetPosition":
		command = dlResetPosition
	case "preset":
		index, err := strconv.Atoi(change.Value)
		if err != nil || index < 0 || index >= len(lyricStylePresets) {
			return fmt.Errorf("未知外观预设")
		}
		command = dlPresetBase + uintptr(index)
	}
	if command != 0 {
		return c.settingsCommand(c.hwnd, command)
	}
	next := c.cfg
	switch change.Key {
	case "fontName":
		next.FontName = change.Value
	case "textAlign":
		if change.Value != "left" && change.Value != "center" && change.Value != "right" {
			return fmt.Errorf("未知对齐方式")
		}
		next.TextAlign = change.Value
	case "windowAlpha":
		next.WindowAlpha = max(.1, min(1, next.WindowAlpha+float64(change.Step)*.05))
	case "textColor", "shadowColor":
		if !validLyricColor(change.Value) {
			return fmt.Errorf("无效颜色")
		}
		if change.Key == "textColor" {
			next.TextColor = change.Value
		} else {
			next.ShadowColor = change.Value
		}
	default:
		return fmt.Errorf("未知歌词设置：%s", change.Key)
	}
	return c.applySettings(c.hwnd, normalizeLyricSettings(next))
}

func validLyricColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(value[1:], 16, 24)
	return err == nil
}

func rememberLyricColor(colors []string, value string) []string {
	if !validLyricColor(value) {
		return colors
	}
	result := []string{strings.ToUpper(value)}
	for _, color := range colors {
		if validLyricColor(color) && !strings.EqualFold(color, value) && len(result) < 8 {
			result = append(result, strings.ToUpper(color))
		}
	}
	return result
}
