//go:build windows

package desktop_lyrics

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
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
	dlSettings  = 0x8002 // WM_APP + 2
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
	onSettings      func()
	// UI-thread-owned resources and layout.
	font, fontResource                uintptr
	width, height, fontSize, maxWidth int32
	settings                          chan lyricSettingsRequest
	surface                           *lyricSurface
	hasRendered, restoredOrigin       bool
	work                              dlRect
}

func newController(cfg configs.DesktopLyricsConfig) Controller {
	if !cfg.Enable {
		return nil
	}
	c := &windowsController{cfg: normalizeLyricSettings(cfg), done: make(chan struct{}), settings: make(chan lyricSettingsRequest, 1), configPath: app.ConfigFilePath()}
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
	dlUser.NewProc("SystemParametersInfoW").Call(0x0030, 0, uintptr(unsafe.Pointer(&c.work)), 0) // SPI_GETWORKAREA
	if c.work.right <= c.work.left {
		c.work = dlRect{0, 0, 1280, 720}
	}
	ratio := c.cfg.MaxWindowWidth
	if ratio < 0.3 || ratio > 0.9 {
		ratio = 0.7
	}
	c.width = int32(float64(c.work.right-c.work.left) * ratio)
	c.maxWidth = c.width
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
	if saved.WindowsOrigin != nil && saved.WindowsOrigin.XFactor >= 0 && saved.WindowsOrigin.XFactor < 1 && saved.WindowsOrigin.YFactor >= 0 && saved.WindowsOrigin.YFactor < 1 {
		c.restoredOrigin = true
		return c.work.left + int32(math.Round(saved.WindowsOrigin.XFactor*float64(c.work.right-c.work.left))), c.work.top + int32(math.Round(saved.WindowsOrigin.YFactor*float64(c.work.bottom-c.work.top)))
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
	case 0x0318: // WM_PRINTCLIENT: expose the real rendered surface for capture.
		if c.renderLyrics(hwnd) == nil && c.surface != nil {
			dlGDI.NewProc("BitBlt").Call(wParam, 0, 0, uintptr(c.width), uintptr(c.height), c.surface.dc, 0, 0, 0x00CC0020)
		}
		return 0
	case 0x0014: // WM_ERASEBKGND: paint via a back buffer instead.
		return 1
	case 0x0021: // WM_MOUSEACTIVATE
		return 3 // MA_NOACTIVATE
	case 0x0084: // WM_NCHITTEST
		if c.cfg.Draggable {
			return 2 // HTCAPTION: drag from the padded lyric rectangle.
		}
		return 1 // HTCLIENT
	case 0x00a5, 0x007b: // Caption/client context clicks share the terminal entry.
		c.mu.Lock()
		handler := c.onSettings
		c.mu.Unlock()
		if handler != nil {
			handler()
		}
		return 0
	case dlSettings:
		request := <-c.settings
		if request.reset {
			x := c.work.left + (c.work.right-c.work.left-c.width)/2
			y := max(c.work.top, c.work.bottom-c.height-48)
			dlUser.NewProc("SetWindowPos").Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x0015)
			c.persistPosition(hwnd)
			request.result <- nil
		} else {
			request.result <- c.applySettings(hwnd, normalizeLyricSettings(request.config))
		}
		return 0
	case 0x0232: // WM_EXITSIZEMOVE
		c.persistPosition(hwnd)
	case dlClose:
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
	dlUser.NewProc("BeginPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer dlUser.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if err := c.renderLyrics(hwnd); err != nil {
		slog.Error("Render desktop lyrics", "error", err)
	}
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
		WindowsOrigin: &storage.DesktopLyricsOrigin{
			XFactor: float64(rect.left-c.work.left) / float64(c.work.right-c.work.left),
			YFactor: float64(rect.top-c.work.top) / float64(c.work.bottom-c.work.top),
		},
	}
	if err := storage.NewTable().SetByKVModel(position, position); err != nil {
		slog.Warn("Save desktop lyric position", "error", err)
	}
}

func (c *windowsController) release() {
	if c.surface != nil {
		c.surface.close()
	}
	if c.font != 0 {
		dlGDI.NewProc("DeleteObject").Call(c.font)
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
