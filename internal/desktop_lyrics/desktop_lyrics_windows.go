//go:build windows

package desktop_lyrics

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	pixelfont "github.com/go-musicfox/go-musicfox/assets/fonts/fusion-pixel"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/storage"
	"github.com/go-musicfox/go-musicfox/utils/app"
)

const DesktopLyricsAvailable = true

const (
	dlRefresh   = 0x8001 // WM_APP + 1
	dlClose     = 0x0010
	dlColorKey  = 0x010101
	dlTextFlags = 0x0010 | 0x0800 | 0x0001 // DT_WORDBREAK | DT_NOPREFIX | DT_CENTER
)

var (
	dlUser       = windows.NewLazySystemDLL("user32.dll")
	dlGDI        = windows.NewLazySystemDLL("gdi32.dll")
	dlKernel     = windows.NewLazySystemDLL("kernel32.dll")
	dlContexts   sync.Map
	dlClassOnce  sync.Once
	dlClassError error
	dlCallback   = windows.NewCallback(desktopLyricsWndProc)
)

type dlPoint struct{ x, y int32 }
type dlRect struct{ left, top, right, bottom int32 }
type dlMessage struct {
	hwnd           uintptr
	message        uint32
	wParam, lParam uintptr
	time           uint32
	point          dlPoint
	private        uint32
}
type dlWindowClass struct {
	size, style                        uint32
	callback                           uintptr
	classExtra, windowExtra            int32
	instance, icon, cursor, background uintptr
	menu, name                         *uint16
	smallIcon                          uintptr
}
type dlPaint struct {
	dc              uintptr
	erase           int32
	rect            dlRect
	restore, update int32
	reserved        [32]byte
}

// All HWND/GDI operations run on one locked thread. Playback goroutines only
// replace pending text and post a refresh; they never draw or move the window.
type windowsController struct {
	cfg             configs.DesktopLyricsConfig
	mu              sync.Mutex
	hwnd            uintptr
	visible, closed bool
	current, next   string
	done            chan struct{}
	configPath      string
	// UI-thread-owned resources and layout.
	menuActive                bool
	font, fontResource, brush uintptr
	width, height, fontSize   int32
	work                      dlRect
}

func newController(cfg configs.DesktopLyricsConfig) Controller {
	if !cfg.Enable {
		return nil
	}
	c := &windowsController{cfg: normalizeLyricSettings(cfg), done: make(chan struct{}), configPath: app.ConfigFilePath()}
	ready := make(chan error, 1)
	go c.run(ready)
	if err := <-ready; err != nil {
		slog.Error("Failed to create Windows desktop lyrics", "error", err)
		return nil
	}
	return c
}

func (c *windowsController) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Draw at physical pixel sizes even when the terminal uses display scaling.
	dpiContext := dlUser.NewProc("SetThreadDpiAwarenessContext")
	if dpiContext.Find() == nil {
		previous, _, _ := dpiContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2
		if previous != 0 {
			defer dpiContext.Call(previous)
		}
	}
	defer close(c.done)
	defer c.release()
	if err := c.create(); err != nil {
		ready <- err
		return
	}
	ready <- nil
	var message dlMessage
	for {
		result, _, _ := dlUser.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		dlUser.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
		dlUser.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
	}
}

func (c *windowsController) create() error {
	instance, _, _ := dlKernel.NewProc("GetModuleHandleW").Call(0)
	className := windows.StringToUTF16Ptr("MusicfoxPixelLyrics")
	dlClassOnce.Do(func() {
		cursor, _, _ := dlUser.NewProc("LoadCursorW").Call(0, 32512)
		wc := dlWindowClass{callback: dlCallback, instance: instance, cursor: cursor, name: className}
		wc.size = uint32(unsafe.Sizeof(wc))
		atom, _, err := dlUser.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			dlClassError = fmt.Errorf("register lyric window: %w", err)
		}
	})
	if dlClassError != nil {
		return dlClassError
	}
	var count uint32
	c.fontResource, _, _ = dlGDI.NewProc("AddFontMemResourceEx").Call(uintptr(unsafe.Pointer(&pixelfont.Data[0])), uintptr(len(pixelfont.Data)), 0, uintptr(unsafe.Pointer(&count)))
	if c.fontResource == 0 {
		return fmt.Errorf("load embedded pixel font")
	}
	c.fontSize = int32(c.cfg.FontSize)
	c.font = createLyricFont(c.cfg)
	if c.font == 0 {
		return fmt.Errorf("create desktop lyric font")
	}
	c.brush, _, _ = dlGDI.NewProc("CreateSolidBrush").Call(dlColorKey)
	if c.brush == 0 {
		return fmt.Errorf("create transparent lyric background")
	}
	dlUser.NewProc("SystemParametersInfoW").Call(0x0030, 0, uintptr(unsafe.Pointer(&c.work)), 0) // SPI_GETWORKAREA
	if c.work.right <= c.work.left {
		c.work = dlRect{0, 0, 1280, 720}
	}
	ratio := c.cfg.MaxWindowWidth
	if ratio < 0.3 || ratio > 0.9 {
		ratio = 0.7
	}
	c.width = int32(float64(c.work.right-c.work.left) * ratio)
	c.height = c.fontSize*4 + 32
	x, y := c.initialPosition()
	// WS_EX_LAYERED | WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE.
	hwnd, _, err := dlUser.NewProc("CreateWindowExW").Call(0x08080088, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Musicfox Desktop Lyrics"))), 0x80000000,
		uintptr(x), uintptr(y), uintptr(c.width), uintptr(c.height), 0, 0, instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("create lyric window: %w", err)
	}
	c.hwnd = hwnd
	dlContexts.Store(hwnd, c)
	alpha := c.cfg.WindowAlpha
	if alpha <= 0 || alpha > 1 {
		alpha = 1
	}
	if ok, _, err := dlUser.NewProc("SetLayeredWindowAttributes").Call(hwnd, dlColorKey, uintptr(alpha*255), 3); ok == 0 {
		dlUser.NewProc("DestroyWindow").Call(hwnd)
		return fmt.Errorf("set lyric transparency: %w", err)
	}
	return nil
}

func (c *windowsController) initialPosition() (int32, int32) {
	xf, yf := c.cfg.XPositionFactor, c.cfg.YPositionFactor
	if xf <= 0 || xf > 1 {
		xf = 0.5
	}
	if yf <= 0 || yf > 1 {
		yf = 0.15
	}
	var saved storage.DesktopLyricsPosition
	if storage.DBManager != nil {
		if data, err := storage.NewTable().GetByKVModel(saved); err == nil && json.Unmarshal(data, &saved) == nil && saved.ScreenID == 0 && saved.XFactor > 0 && saved.YFactor > 0 {
			xf, yf = saved.XFactor, saved.YFactor
		}
	}
	x := c.work.left + int32(float64(c.work.right-c.work.left)*xf) - c.width/2
	y := c.work.top + int32(float64(c.work.bottom-c.work.top)*(1-yf)) - c.height/2
	return max(c.work.left, min(x, c.work.right-c.width)), max(c.work.top, min(y, c.work.bottom-c.height))
}

func desktopLyricsWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, ok := dlContexts.Load(hwnd)
	if !ok {
		result, _, _ := dlUser.NewProc("DefWindowProcW").Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	c := value.(*windowsController)
	switch message {
	case dlRefresh:
		if c.menuActive {
			return 0
		}
		c.mu.Lock()
		visible := c.visible && !c.closed
		c.mu.Unlock()
		show := uintptr(0)
		if visible {
			show = 4
		} // SW_SHOWNOACTIVATE
		dlUser.NewProc("ShowWindow").Call(hwnd, show)
		dlUser.NewProc("InvalidateRect").Call(hwnd, 0, 0)
		return 0
	case 0x000f: // WM_PAINT
		c.paint(hwnd)
		return 0
	case 0x0014: // WM_ERASEBKGND: paint via a back buffer instead.
		return 1
	case 0x0021: // WM_MOUSEACTIVATE
		return 3 // MA_NOACTIVATE
	case 0x0084: // WM_NCHITTEST
		if c.cfg.Draggable {
			return 2
		} // HTCAPTION; transparent pixels pass through.
		return 1 // HTCLIENT: locked text still accepts the settings menu.
	case 0x007b: // WM_CONTEXTMENU, generated for both client and caption clicks.
		c.showSettingsMenu(hwnd, int32(int16(lParam&0xffff)), int32(int16(lParam>>16&0xffff)))
		return 0
	case 0x0111: // WM_COMMAND
		if err := c.settingsCommand(hwnd, wParam&0xffff); err != nil {
			c.settingsError(hwnd, err)
		}
		return 0
	case 0x0232: // WM_EXITSIZEMOVE
		c.persistPosition(hwnd)
	case dlClose:
		dlUser.NewProc("EndMenu").Call()
		dlUser.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case 0x0002: // WM_DESTROY
		c.mu.Lock()
		c.hwnd, c.visible, c.closed = 0, false, true
		c.mu.Unlock()
		dlContexts.Delete(hwnd)
		dlUser.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	result, _, _ := dlUser.NewProc("DefWindowProcW").Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (c *windowsController) paint(hwnd uintptr) {
	var ps dlPaint
	dc, _, _ := dlUser.NewProc("BeginPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer dlUser.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if dc == 0 {
		return
	}
	c.mu.Lock()
	current, next := c.current, c.next
	c.mu.Unlock()
	if c.cfg.OneLineMode {
		next = ""
	}
	buffer, _, _ := dlGDI.NewProc("CreateCompatibleDC").Call(dc)
	if buffer == 0 {
		return
	}
	defer dlGDI.NewProc("DeleteDC").Call(buffer)
	oldFont, _, _ := dlGDI.NewProc("SelectObject").Call(buffer, c.font)
	defer dlGDI.NewProc("SelectObject").Call(buffer, oldFont)
	dlGDI.NewProc("SetBkMode").Call(buffer, 1)
	measure := func(text string) int32 {
		if text == "" {
			return 0
		}
		rect := dlRect{0, 0, c.width - 32, 0}
		drawLyricText(buffer, text, &rect, dlTextFlags|0x0400) // DT_CALCRECT
		return rect.bottom
	}
	currentHeight, nextHeight := measure(current), measure(next)
	height := min(max(c.fontSize+32, currentHeight+nextHeight+40), (c.work.bottom-c.work.top)/2)
	if height != c.height {
		c.height = height
		var bounds dlRect
		dlUser.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
		// Resize without activating or moving the text below the taskbar.
		y := bounds.top
		if c.cfg.Draggable {
			y = min(y, c.work.bottom-height)
		}
		dlUser.NewProc("SetWindowPos").Call(hwnd, 0, uintptr(bounds.left), uintptr(y), uintptr(c.width), uintptr(height), 0x0014)
	}
	bitmap, _, _ := dlGDI.NewProc("CreateCompatibleBitmap").Call(dc, uintptr(c.width), uintptr(c.height))
	if bitmap == 0 {
		return
	}
	oldBitmap, _, _ := dlGDI.NewProc("SelectObject").Call(buffer, bitmap)
	defer func() {
		dlGDI.NewProc("SelectObject").Call(buffer, oldBitmap)
		dlGDI.NewProc("DeleteObject").Call(bitmap)
	}()
	area := dlRect{0, 0, c.width, c.height}
	dlUser.NewProc("FillRect").Call(buffer, uintptr(unsafe.Pointer(&area)), c.brush)
	active := lyricColor(c.cfg.TextColor, 0xB3C1E5)
	inactive := ((active & 0xff) * 2 / 3) | (((active >> 8 & 0xff) * 2 / 3) << 8) | (((active >> 16 & 0xff) * 2 / 3) << 16)
	draw := func(text string, y, h int32, color uintptr) {
		rect := dlRect{16, y, c.width - 16, min(c.height-8, y+h)}
		// Hard pixel outlines remain legible over both light and dark desktops.
		dlGDI.NewProc("SetTextColor").Call(buffer, lyricColor(c.cfg.ShadowColor, 0x101010))
		for _, offset := range []dlPoint{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}} {
			shadow := dlRect{rect.left + offset.x, rect.top + offset.y, rect.right + offset.x, rect.bottom + offset.y}
			drawLyricText(buffer, text, &shadow, dlTextFlags)
		}
		dlGDI.NewProc("SetTextColor").Call(buffer, color)
		drawLyricText(buffer, text, &rect, dlTextFlags)
	}
	draw(current, 12, currentHeight, active)
	if next != "" {
		draw(next, 20+currentHeight, nextHeight, inactive)
	}
	dlGDI.NewProc("BitBlt").Call(dc, 0, 0, uintptr(c.width), uintptr(c.height), buffer, 0, 0, 0x00CC0020)
}

func drawLyricText(dc uintptr, text string, rect *dlRect, flags uintptr) {
	encoded, _ := windows.UTF16FromString(strings.ReplaceAll(text, "\x00", ""))
	dlUser.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(&encoded[0])), uintptr(len(encoded)-1), uintptr(unsafe.Pointer(rect)), flags)
}

func lyricColor(hex string, fallback uintptr) uintptr {
	value, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 24)
	if err != nil || len(strings.TrimPrefix(hex, "#")) != 6 {
		return fallback
	}
	color := uintptr((value&0xff)<<16 | value&0xff00 | value>>16)
	if color == dlColorKey {
		return fallback
	}
	return color
}

func (c *windowsController) persistPosition(hwnd uintptr) {
	if storage.DBManager == nil {
		return
	}
	var rect dlRect
	dlUser.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	position := storage.DesktopLyricsPosition{
		XFactor: float64(rect.left+c.width/2-c.work.left) / float64(c.work.right-c.work.left),
		YFactor: 1 - float64(rect.top+c.height/2-c.work.top)/float64(c.work.bottom-c.work.top),
	}
	if err := storage.NewTable().SetByKVModel(position, position); err != nil {
		slog.Warn("Save desktop lyric position", "error", err)
	}
}

func (c *windowsController) release() {
	if c.font != 0 {
		dlGDI.NewProc("DeleteObject").Call(c.font)
	}
	if c.brush != 0 {
		dlGDI.NewProc("DeleteObject").Call(c.brush)
	}
	if c.fontResource != 0 {
		dlGDI.NewProc("RemoveFontMemResourceEx").Call(c.fontResource)
	}
}

func (c *windowsController) Show() { c.setVisible(true) }
func (c *windowsController) Hide() { c.setVisible(false) }
func (c *windowsController) setVisible(visible bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.visible == visible {
		return
	}
	c.visible = visible
	dlUser.NewProc("PostMessageW").Call(c.hwnd, dlRefresh, 0, 0)
}
func (c *windowsController) IsVisible() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.visible && !c.closed
}
func (c *windowsController) Update(current, next LyricLine, _ int, _ int64, _ bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	curText, nextText := desktopLineText(current), desktopLineText(next)
	if c.current == curText && c.next == nextText {
		return
	}
	c.current, c.next = curText, nextText
	dlUser.NewProc("PostMessageW").Call(c.hwnd, dlRefresh, 0, 0)
}
func desktopLineText(line LyricLine) string {
	if line.Text != "" {
		return line.Text
	}
	var text strings.Builder
	for _, word := range line.Words {
		text.WriteString(word.Word)
	}
	return text.String()
}
func (c *windowsController) UpdateSpectrum(player.SpectrumFrame)    {}
func (c *windowsController) UpdateRawSamples(player.RawSampleFrame) {}
func (c *windowsController) SetSpectrumAvailable(bool)              {}
func (c *windowsController) Close() {
	c.mu.Lock()
	if !c.closed {
		c.closed, c.visible = true, false
		dlUser.NewProc("PostMessageW").Call(c.hwnd, dlClose, 0, 0)
	}
	c.mu.Unlock()
	<-c.done
}
