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

const (
	dlLock          = 100
	dlBold          = 101
	dlItalic        = 102
	dlOneLine       = 103
	dlChooseFont    = 104
	dlTextColor     = 105
	dlShadowColor   = 106
	dlResetPosition = 107
	dlVertical      = 108
	dlHorizontal    = 109
	dlSmaller       = 110
	dlLarger        = 111
	dlPanelDone     = 112
	dlFontBase      = 400
	dlOpacityBase   = 600
)

var lyricFontChoices = []string{pixelfont.Name, "Microsoft YaHei", "SimSun"}

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

func (c *windowsController) settingsCommand(hwnd, command uintptr) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil
	}
	next := c.cfg
	switch {
	case command == dlVertical:
		next.Vertical = true
	case command == dlHorizontal:
		next.Vertical = false
	case command == dlSmaller || command == dlLarger:
		step := 2.0
		if strings.EqualFold(next.FontName, pixelfont.Name) {
			step = 12
		}
		if command == dlSmaller {
			step = -step
		}
		next.FontSize = max(12, min(96, next.FontSize+step))
	case command == dlLock:
		next.Draggable = !next.Draggable
	case command == dlBold:
		next.FontBold = !next.FontBold
	case command == dlItalic:
		next.FontItalic = !next.FontItalic
	case command == dlOneLine:
		next.OneLineMode = !next.OneLineMode
	case command >= dlFontBase && command < dlFontBase+uintptr(len(lyricFontChoices)):
		next.FontName = lyricFontChoices[command-dlFontBase]
	case command >= dlOpacityBase+50 && command <= dlOpacityBase+100:
		next.WindowAlpha = float64(command-dlOpacityBase) / 100
	case command == dlChooseFont:
		var ok bool
		next, ok = c.chooseFont(c.settingsOwner(hwnd))
		if !ok {
			return nil
		}
	case command == dlTextColor || command == dlShadowColor:
		color := next.TextColor
		if command == dlShadowColor {
			color = next.ShadowColor
		}
		selected, ok := chooseLyricColor(c.settingsOwner(hwnd), color)
		if !ok {
			return nil
		}
		if command == dlTextColor {
			next.TextColor = selected
		} else {
			next.ShadowColor = selected
		}
	case command == dlResetPosition:
		x := c.work.left + (c.work.right-c.work.left-c.width)/2
		y := max(c.work.top, c.work.bottom-c.height-48)
		dlUser.NewProc("SetWindowPos").Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x0015)
		c.persistPosition(hwnd)
		return nil
	default:
		return nil
	}
	return c.applySettings(hwnd, normalizeLyricSettings(next))
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
	if c.panel != nil {
		c.panel.refresh()
	}
	if err := c.renderLyrics(hwnd); err != nil {
		return err
	}
	c.persistPosition(hwnd)
	return nil
}

func (c *windowsController) settingsError(hwnd uintptr, err error) {
	dlUser.NewProc("MessageBoxW").Call(hwnd, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(err.Error()))), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("桌面歌词设置"))), 0x10)
}

type dlLogFont struct {
	height, width, escapement, orientation, weight                      int32
	italic, underline, strikeout, charset, output, clip, quality, pitch byte
	face                                                                [32]uint16
}
type dlChooseFontData struct {
	size                                  uint32
	owner, dc                             uintptr
	font                                  *dlLogFont
	pointSize                             int32
	flags, color                          uint32
	data, hook, template, instance, style uintptr
	fontType, alignment                   uint16
	minSize, maxSize                      int32
}
type dlChooseColorData struct {
	size                 uint32
	owner, instance      uintptr
	color                uint32
	custom               *[16]uint32
	flags                uint32
	data, hook, template uintptr
}

var dlDialogs = windows.NewLazySystemDLL("comdlg32.dll")

func (c *windowsController) chooseFont(hwnd uintptr) (configs.DesktopLyricsConfig, bool) {
	var font dlLogFont
	dlGDI.NewProc("GetObjectW").Call(c.font, unsafe.Sizeof(font), uintptr(unsafe.Pointer(&font)))
	choice := dlChooseFontData{owner: hwnd, font: &font, flags: 0x41 | 0x2000 | 0x00800000, minSize: 9, maxSize: 72}
	choice.size = uint32(unsafe.Sizeof(choice))
	result, _, _ := dlDialogs.NewProc("ChooseFontW").Call(uintptr(unsafe.Pointer(&choice)))
	next := c.cfg
	if result == 0 {
		return next, false
	}
	next.FontName = windows.UTF16ToString(font.face[:])
	next.FontSize = math.Abs(float64(font.height))
	next.FontBold, next.FontItalic = font.weight >= 600, font.italic != 0
	return next, true
}

func chooseLyricColor(hwnd uintptr, initial string) (string, bool) {
	var custom [16]uint32
	choice := dlChooseColorData{owner: hwnd, color: uint32(lyricColor(initial, 0xffffff)), custom: &custom, flags: 3}
	choice.size = uint32(unsafe.Sizeof(choice))
	result, _, _ := dlDialogs.NewProc("ChooseColorW").Call(uintptr(unsafe.Pointer(&choice)))
	return fmt.Sprintf("#%02X%02X%02X", choice.color&0xff, choice.color>>8&0xff, choice.color>>16&0xff), result != 0
}
