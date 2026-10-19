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
	dlSizeBase      = 200
	dlFontBase      = 400
	dlOpacityBase   = 600
)

var lyricFontChoices = []struct{ label, face string }{
	{"中文像素 · Fusion Pixel", pixelfont.Name},
	{"微软雅黑", "Microsoft YaHei"},
	{"宋体", "SimSun"},
	{"等宽 · Consolas", "Consolas"},
}

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

func appendLyricMenu(menu, flags, command uintptr, label string) {
	dlUser.NewProc("AppendMenuW").Call(menu, flags, command, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(label))))
}

func (c *windowsController) buildSettingsMenu() uintptr {
	menu, _, _ := dlUser.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return 0
	}
	checked := func(value bool) uintptr {
		if value {
			return 8
		}
		return 0
	}
	appendLyricMenu(menu, checked(!c.cfg.Draggable), dlLock, "锁定位置（仍可右键设置）")
	appendLyricMenu(menu, 0x0800, 0, "")
	sizes, _, _ := dlUser.NewProc("CreatePopupMenu").Call()
	for _, size := range []int{12, 24, 36, 48, 60, 72, 96} {
		appendLyricMenu(sizes, checked(int(c.cfg.FontSize) == size), dlSizeBase+uintptr(size), fmt.Sprintf("%d px", size))
	}
	appendLyricMenu(menu, 0x0010, sizes, fmt.Sprintf("字号 · %d px", int(c.cfg.FontSize)))
	fonts, _, _ := dlUser.NewProc("CreatePopupMenu").Call()
	for i, choice := range lyricFontChoices {
		appendLyricMenu(fonts, checked(c.cfg.FontName == choice.face), dlFontBase+uintptr(i), choice.label)
	}
	appendLyricMenu(fonts, 0x0800, 0, "")
	appendLyricMenu(fonts, 0, dlChooseFont, "选择其他字体与样式…")
	appendLyricMenu(menu, 0x0010, fonts, "字体")
	appendLyricMenu(menu, checked(c.cfg.FontBold), dlBold, "加粗")
	appendLyricMenu(menu, checked(c.cfg.FontItalic), dlItalic, "斜体")
	appendLyricMenu(menu, 0, dlTextColor, "文字颜色…")
	appendLyricMenu(menu, 0, dlShadowColor, "描边颜色…")
	opacity, _, _ := dlUser.NewProc("CreatePopupMenu").Call()
	for _, percent := range []int{50, 75, 100} {
		appendLyricMenu(opacity, checked(int(math.Round(c.cfg.WindowAlpha*100)) == percent), dlOpacityBase+uintptr(percent), fmt.Sprintf("%d%%", percent))
	}
	appendLyricMenu(menu, 0x0010, opacity, "不透明度")
	appendLyricMenu(menu, checked(c.cfg.OneLineMode), dlOneLine, "只显示当前句")
	appendLyricMenu(menu, 0x0800, 0, "")
	appendLyricMenu(menu, 0, dlResetPosition, "移回屏幕底部中央")
	return menu
}

func (c *windowsController) showSettingsMenu(hwnd uintptr, x, y int32) {
	if c.menuActive {
		return
	}
	menu := c.buildSettingsMenu()
	if menu == 0 {
		return
	}
	defer dlUser.NewProc("DestroyMenu").Call(menu)
	c.menuActive = true
	previous, _, _ := dlUser.NewProc("GetForegroundWindow").Call()
	defer func() {
		c.menuActive = false
		dlUser.NewProc("PostMessageW").Call(hwnd, dlRefresh, 0, 0)
		if current, _, _ := dlUser.NewProc("GetForegroundWindow").Call(); current == hwnd && previous != 0 {
			dlUser.NewProc("SetForegroundWindow").Call(previous)
		}
	}()
	if x == -1 && y == -1 {
		var position dlRect
		dlUser.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&position)))
		x, y = position.left+16, position.top+16
	}
	dlUser.NewProc("SetForegroundWindow").Call(hwnd)
	command, _, _ := dlUser.NewProc("TrackPopupMenu").Call(menu, 0x0102, uintptr(x), uintptr(y), 0, hwnd, 0)
	dlUser.NewProc("PostMessageW").Call(hwnd, 0, 0, 0)
	if command != 0 {
		if err := c.settingsCommand(hwnd, command); err != nil {
			c.settingsError(hwnd, err)
		}
	}
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
	case command == dlLock:
		next.Draggable = !next.Draggable
	case command == dlBold:
		next.FontBold = !next.FontBold
	case command == dlItalic:
		next.FontItalic = !next.FontItalic
	case command == dlOneLine:
		next.OneLineMode = !next.OneLineMode
	case command > dlSizeBase && command <= dlSizeBase+96:
		next.FontSize = float64(command - dlSizeBase)
	case command >= dlFontBase && command < dlFontBase+uintptr(len(lyricFontChoices)):
		next.FontName = lyricFontChoices[command-dlFontBase].face
	case command >= dlOpacityBase+50 && command <= dlOpacityBase+100:
		next.WindowAlpha = float64(command-dlOpacityBase) / 100
	case command == dlChooseFont:
		var ok bool
		next, ok = c.chooseFont(hwnd)
		if !ok {
			return nil
		}
	case command == dlTextColor || command == dlShadowColor:
		color := next.TextColor
		if command == dlShadowColor {
			color = next.ShadowColor
		}
		selected, ok := chooseLyricColor(hwnd, color)
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
	dlUser.NewProc("SetLayeredWindowAttributes").Call(hwnd, dlColorKey, uintptr(next.WindowAlpha*255), 3)
	dlUser.NewProc("InvalidateRect").Call(hwnd, 0, 0)
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
