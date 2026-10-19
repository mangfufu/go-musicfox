//go:build windows

package desktop_lyrics

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"

	pixelfont "github.com/go-musicfox/go-musicfox/assets/fonts/fusion-pixel"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"golang.org/x/sys/windows"
)

const (
	dlPanelBackground = 0x111415
	dlPanelSurface    = 0x272E30
	dlPanelBorder     = 0x3E484B
	dlPanelInk        = 0xBBCBD0
	dlPanelAccent     = 0x4D77D4
	dlPanelMuted      = 0x829499
)

var (
	dlPanels          sync.Map
	dlPanelClassOnce  sync.Once
	dlPanelClassError error
	dlPanelCallback   = windows.NewCallback(lyricPanelWndProc)
)

type lyricSettingsPanel struct {
	hwnd, font, smallFont uintptr
	controller            *windowsController
	buttons               map[uintptr]uintptr
}

type dlDrawItem struct {
	controlType, controlID, itemID, action, state uint32
	item, dc                                      uintptr
	rect                                          dlRect
	data                                          uintptr
}

func (c *windowsController) settingsOwner(fallback uintptr) uintptr {
	if c.panel != nil {
		return c.panel.hwnd
	}
	return fallback
}

func (c *windowsController) showSettingsPanel() {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return
	}
	if c.panel != nil {
		dlUser.NewProc("ShowWindow").Call(c.panel.hwnd, 5)
		dlUser.NewProc("SetForegroundWindow").Call(c.panel.hwnd)
		return
	}
	instance, _, _ := dlKernel.NewProc("GetModuleHandleW").Call(0)
	className := windows.StringToUTF16Ptr("MusicfoxLyricSettings")
	dlPanelClassOnce.Do(func() {
		cursor, _, _ := dlUser.NewProc("LoadCursorW").Call(0, 32512)
		wc := dlWindowClass{callback: dlPanelCallback, instance: instance, cursor: cursor, name: className}
		wc.size = uint32(unsafe.Sizeof(wc))
		if atom, _, err := dlUser.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
			dlPanelClassError = fmt.Errorf("register settings panel: %w", err)
		}
	})
	if dlPanelClassError != nil {
		c.settingsError(c.hwnd, dlPanelClassError)
		return
	}
	p := &lyricSettingsPanel{controller: c, buttons: make(map[uintptr]uintptr)}
	p.font = createLyricFont(configs.DesktopLyricsConfig{FontName: pixelfont.Name, FontSize: 24})
	p.smallFont = createLyricFont(configs.DesktopLyricsConfig{FontName: pixelfont.Name, FontSize: 12})
	if p.font == 0 || p.smallFont == 0 {
		p.release()
		c.settingsError(c.hwnd, fmt.Errorf("无法创建设置面板字体"))
		return
	}
	const windowStyle = 0x00c80000   // WS_CAPTION | WS_SYSMENU
	const extendedStyle = 0x00010080 // WS_EX_CONTROLPARENT | WS_EX_TOOLWINDOW
	bounds := dlRect{0, 0, 600, 620}
	dlUser.NewProc("AdjustWindowRectEx").Call(uintptr(unsafe.Pointer(&bounds)), windowStyle, 0, extendedStyle)
	width, height := bounds.right-bounds.left, bounds.bottom-bounds.top
	x, y := c.work.left+(c.work.right-c.work.left-width)/2, max(c.work.top, c.work.top+(c.work.bottom-c.work.top-height)/2)
	var err error
	p.hwnd, _, err = dlUser.NewProc("CreateWindowExW").Call(extendedStyle, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("桌面歌词设置"))), windowStyle,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height), c.hwnd, 0, instance, 0)
	if p.hwnd == 0 {
		p.release()
		c.settingsError(c.hwnd, fmt.Errorf("创建设置面板失败：%w", err))
		return
	}
	c.panel = p
	dlPanels.Store(p.hwnd, p)
	var dark int32 = 1
	dwm := windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if dwm.Find() == nil {
		dwm.Call(p.hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	}
	button := func(id uintptr, label string, x, y, w int32) {
		if err := p.addButton(id, label, x, y, w, 40); err != nil {
			c.settingsError(p.hwnd, err)
		}
	}
	button(dlHorizontal, "横排", 24, 132, 120)
	button(dlVertical, "竖排", 156, 132, 120)
	button(dlSmaller, "-", 312, 132, 52)
	button(dlLarger, "+", 524, 132, 52)
	button(dlFontBase, "像素", 24, 208, 126)
	button(dlFontBase+1, "雅黑", 166, 208, 126)
	button(dlFontBase+2, "宋体", 308, 208, 126)
	button(dlChooseFont, "更多", 450, 208, 126)
	button(dlBold, "加粗", 24, 268, 126)
	button(dlItalic, "斜体", 166, 268, 126)
	button(dlTextColor, "文字颜色", 24, 348, 264)
	button(dlShadowColor, "描边颜色", 312, 348, 264)
	button(dlOpacityBase+50, "50%", 24, 428, 172)
	button(dlOpacityBase+75, "75%", 214, 428, 172)
	button(dlOpacityBase+100, "100%", 404, 428, 172)
	button(dlLock, "锁定位置", 24, 492, 264)
	button(dlOneLine, "仅当前句", 312, 492, 264)
	button(dlResetPosition, "位置复位", 24, 556, 264)
	button(dlPanelDone, "完成", 404, 556, 172)
	p.refresh()
	dlUser.NewProc("ShowWindow").Call(p.hwnd, 5)
	dlUser.NewProc("SetForegroundWindow").Call(p.hwnd)
	dlUser.NewProc("SetFocus").Call(p.buttons[dlHorizontal])
}

func (p *lyricSettingsPanel) addButton(id uintptr, label string, x, y, w, h int32) error {
	// Native buttons retain keyboard navigation and accessible names; only paint is custom.
	button, _, err := dlUser.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("BUTTON"))), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(label))),
		0x5001000b, uintptr(x), uintptr(y), uintptr(w), uintptr(h), p.hwnd, id, 0, 0) // CHILD | VISIBLE | TABSTOP | BS_OWNERDRAW
	if button == 0 {
		return fmt.Errorf("创建设置按钮失败：%w", err)
	}
	p.buttons[id] = button
	dlUser.NewProc("SendMessageW").Call(button, 0x30, p.font, 0) // WM_SETFONT
	return nil
}

func lyricPanelWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, ok := dlPanels.Load(hwnd)
	if !ok {
		result, _, _ := dlUser.NewProc("DefWindowProcW").Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	p := value.(*lyricSettingsPanel)
	switch message {
	case 0x000f:
		var ps dlPaint
		dc, _, _ := dlUser.NewProc("BeginPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		p.paint(dc)
		dlUser.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0318: // WM_PRINTCLIENT, also used by rendering verification.
		p.paint(wParam)
		return 0
	case 0x0014:
		return 1
	case 0x002b: // WM_DRAWITEM
		p.drawButton((*dlDrawItem)(unsafe.Pointer(lParam)))
		return 1
	case 0x0111:
		id := wParam & 0xffff
		if id == dlPanelDone || id == 2 {
			dlUser.NewProc("DestroyWindow").Call(hwnd)
			return 0
		}
		if wParam>>16 == 0 {
			if err := p.controller.settingsCommand(p.controller.hwnd, id); err != nil {
				p.controller.settingsError(hwnd, err)
			}
		}
		return 0
	case 0x0010:
		dlUser.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case 0x0002:
		dlPanels.Delete(hwnd)
		p.controller.panel = nil
		p.release()
		return 0 // Closing settings must not terminate the lyric thread.
	}
	result, _, _ := dlUser.NewProc("DefWindowProcW").Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (p *lyricSettingsPanel) release() {
	if p.font != 0 {
		dlGDI.NewProc("DeleteObject").Call(p.font)
	}
	if p.smallFont != 0 {
		dlGDI.NewProc("DeleteObject").Call(p.smallFont)
	}
}

func (p *lyricSettingsPanel) refresh() {
	for id, enabled := range map[uintptr]bool{dlSmaller: p.controller.cfg.FontSize > 12, dlLarger: p.controller.cfg.FontSize < 96} {
		state := uintptr(0)
		if enabled {
			state = 1
		}
		dlUser.NewProc("EnableWindow").Call(p.buttons[id], state)
	}
	dlUser.NewProc("InvalidateRect").Call(p.hwnd, 0, 0)
	for _, button := range p.buttons {
		dlUser.NewProc("InvalidateRect").Call(button, 0, 0)
	}
}

func panelText(dc, font uintptr, text string, rect dlRect, color uintptr, flags uintptr) {
	old, _, _ := dlGDI.NewProc("SelectObject").Call(dc, font)
	defer dlGDI.NewProc("SelectObject").Call(dc, old)
	dlGDI.NewProc("SetBkMode").Call(dc, 1)
	dlGDI.NewProc("SetTextColor").Call(dc, color)
	drawLyricText(dc, text, &rect, flags|0x0800)
}

func (p *lyricSettingsPanel) paint(dc uintptr) {
	fillLyricRect(dc, dlRect{0, 0, 600, 620}, dlPanelBackground)
	panelText(dc, p.smallFont, "MUSICFOX / DESKTOP LYRICS", dlRect{24, 20, 576, 36}, dlPanelAccent, 0)
	panelText(dc, p.font, "桌面歌词", dlRect{24, 42, 576, 72}, dlPanelInk, 0)
	panelText(dc, p.smallFont, "实时预览 · 自动保存", dlRect{24, 80, 576, 96}, dlPanelMuted, 0)
	fillLyricRect(dc, dlRect{24, 104, 576, 105}, dlPanelBorder)
	for _, label := range []struct {
		text string
		x, y int32
	}{
		{"显示方向", 24, 114}, {"字号", 312, 114}, {"字体", 24, 188}, {"颜色", 24, 326}, {"不透明度", 24, 406},
	} {
		panelText(dc, p.smallFont, label.text, dlRect{label.x, label.y, label.x + 220, label.y + 16}, dlPanelMuted, 0)
	}
	panelText(dc, p.font, fmt.Sprintf("%d px", int(p.controller.cfg.FontSize)), dlRect{366, 132, 522, 172}, dlPanelInk, 0x25)
	cfg := p.controller.cfg
	preview := createLyricFont(configs.DesktopLyricsConfig{FontName: cfg.FontName, FontSize: 24, FontBold: cfg.FontBold, FontItalic: cfg.FontItalic})
	if preview != 0 {
		panelText(dc, preview, "字形 Aa", dlRect{312, 268, 576, 308}, lyricColor(cfg.TextColor, dlPanelInk), 0x25)
		dlGDI.NewProc("DeleteObject").Call(preview)
	}
	panelText(dc, p.smallFont, "右键歌词区域或在终端按 F10 可再次打开", dlRect{24, 604, 576, 620}, dlPanelMuted, 0)
}

func (p *lyricSettingsPanel) selected(id uintptr) bool {
	cfg := p.controller.cfg
	switch id {
	case dlHorizontal:
		return !cfg.Vertical
	case dlVertical:
		return cfg.Vertical
	case dlBold:
		return cfg.FontBold
	case dlItalic:
		return cfg.FontItalic
	case dlLock:
		return !cfg.Draggable
	case dlOneLine:
		return cfg.OneLineMode
	case dlChooseFont:
		for _, choice := range lyricFontChoices {
			if strings.EqualFold(cfg.FontName, choice) {
				return false
			}
		}
		return true
	}
	if id >= dlFontBase && id < dlFontBase+3 {
		return strings.EqualFold(cfg.FontName, lyricFontChoices[id-dlFontBase])
	}
	if id >= dlOpacityBase {
		return int(cfg.WindowAlpha*100) == int(id-dlOpacityBase)
	}
	return false
}

func (p *lyricSettingsPanel) drawButton(item *dlDrawItem) {
	id := uintptr(item.controlID)
	border, background, ink := uintptr(dlPanelBorder), uintptr(dlPanelSurface), uintptr(dlPanelInk)
	if p.selected(id) || id == dlPanelDone {
		border, background, ink = dlPanelAccent, 0x273346, dlPanelAccent
	}
	if item.state&1 != 0 {
		background = 0x111415
	}
	if item.state&4 != 0 {
		ink = dlPanelMuted
	}
	r := item.rect
	fillLyricRect(item.dc, r, border)
	fillLyricRect(item.dc, dlRect{r.left + 1, r.top + 1, r.right - 1, r.bottom - 1}, background)
	if id == dlTextColor || id == dlShadowColor {
		color := p.controller.cfg.TextColor
		if id == dlShadowColor {
			color = p.controller.cfg.ShadowColor
		}
		fillLyricRect(item.dc, dlRect{r.left + 10, r.top + 10, r.left + 30, r.bottom - 10}, lyricColor(color, dlPanelAccent))
		r.left += 24
	}
	var label [128]uint16
	dlUser.NewProc("GetWindowTextW").Call(item.item, uintptr(unsafe.Pointer(&label[0])), 128)
	panelText(item.dc, p.font, windows.UTF16ToString(label[:]), r, ink, 0x25)
	if item.state&0x10 != 0 {
		focus := dlRect{item.rect.left + 4, item.rect.top + 4, item.rect.right - 4, item.rect.bottom - 4}
		dlUser.NewProc("DrawFocusRect").Call(item.dc, uintptr(unsafe.Pointer(&focus)))
	}
}
