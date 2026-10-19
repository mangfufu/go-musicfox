//go:build windows

package desktop_lyrics

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/rivo/uniseg"
)

type dlBitmapInfo struct {
	size                   uint32
	width, height          int32
	planes, bitCount       uint16
	compression, imageSize uint32
	xPixels, yPixels       int32
	used, important        uint32
}

type lyricSurface struct {
	dc, bitmap, previous uintptr
	width, height        int32
	pixels               []uint32
}

func newLyricSurface(width, height int32) (*lyricSurface, error) {
	s := &lyricSurface{width: width, height: height}
	s.dc, _, _ = dlGDI.NewProc("CreateCompatibleDC").Call(0)
	if s.dc == 0 {
		return nil, fmt.Errorf("create lyric drawing context")
	}
	info := dlBitmapInfo{size: 40, width: width, height: -height, planes: 1, bitCount: 32}
	var bits unsafe.Pointer
	s.bitmap, _, _ = dlGDI.NewProc("CreateDIBSection").Call(s.dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if s.bitmap == 0 {
		s.close()
		return nil, fmt.Errorf("create lyric drawing surface")
	}
	s.previous, _, _ = dlGDI.NewProc("SelectObject").Call(s.dc, s.bitmap)
	s.pixels = unsafe.Slice((*uint32)(bits), int(width*height))
	return s, nil
}

func (s *lyricSurface) close() {
	if s.previous != 0 {
		dlGDI.NewProc("SelectObject").Call(s.dc, s.previous)
	}
	if s.bitmap != 0 {
		dlGDI.NewProc("DeleteObject").Call(s.bitmap)
	}
	if s.dc != 0 {
		dlGDI.NewProc("DeleteDC").Call(s.dc)
	}
}

func fillLyricRect(dc uintptr, rect dlRect, color uintptr) {
	dlGDI.NewProc("SetDCBrushColor").Call(dc, color)
	brush, _, _ := dlGDI.NewProc("GetStockObject").Call(18) // DC_BRUSH
	dlUser.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
}

func (c *windowsController) settingsHandleContains(p dlPoint) bool {
	return p.x >= c.width-64 && p.x < c.width-8 && p.y >= 4 && p.y < 32
}

// Keep grapheme clusters intact, including combining marks and emoji sequences.
func verticalLyricColumns(text string, rows int) [][]string {
	var columns [][]string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		var column []string
		graphemes := uniseg.NewGraphemes(line)
		for graphemes.Next() {
			column = append(column, graphemes.Str())
			if len(column) == max(1, rows) {
				columns = append(columns, column)
				column = nil
			}
		}
		if len(column) > 0 {
			columns = append(columns, column)
		}
	}
	return columns
}

func (c *windowsController) renderLyrics(hwnd uintptr) error {
	c.mu.Lock()
	current, next := c.current, c.next
	c.mu.Unlock()
	if c.cfg.OneLineMode {
		next = ""
	}
	measureDC, _, _ := dlGDI.NewProc("CreateCompatibleDC").Call(0)
	if measureDC == 0 {
		return fmt.Errorf("measure lyric text")
	}
	defer dlGDI.NewProc("DeleteDC").Call(measureDC)
	oldFont, _, _ := dlGDI.NewProc("SelectObject").Call(measureDC, c.font)
	defer dlGDI.NewProc("SelectObject").Call(measureDC, oldFont)
	measure := func(text string, width int32) dlRect {
		if text == "" {
			return dlRect{}
		}
		rect := dlRect{0, 0, width, 0}
		drawLyricText(measureDC, text, &rect, dlTextFlags|0x0400)
		return rect
	}
	maxHeight := max(120, (c.work.bottom-c.work.top)*3/4)
	var currentColumns, nextColumns [][]string
	var currentRect, nextRect dlRect
	width, height := c.maxWidth, int32(0)
	cell := c.fontSize + 8
	if c.cfg.Vertical {
		rows := max(1, int((maxHeight-56)/cell))
		currentColumns, nextColumns = verticalLyricColumns(current, rows), verticalLyricColumns(next, rows)
		maxColumns := max(1, int((c.maxWidth-32)/cell))
		if len(currentColumns)+len(nextColumns) > maxColumns {
			all := append(append([][]string{}, currentColumns...), nextColumns...)[:maxColumns]
			last := all[len(all)-1]
			last[len(last)-1] = "…"
			count := min(len(currentColumns), maxColumns)
			currentColumns, nextColumns = all[:count], all[count:]
		}
		longest := 1
		for _, columns := range [][][]string{currentColumns, nextColumns} {
			for _, column := range columns {
				longest = max(longest, len(column))
			}
		}
		width = max(128, int32(len(currentColumns)+len(nextColumns))*cell+32)
		height = int32(longest)*cell + 48
	} else {
		currentRect, nextRect = measure(current, width-32), measure(next, width-32)
		width = min(c.maxWidth, max(160, max(currentRect.right, nextRect.right)+32))
		currentRect, nextRect = measure(current, width-32), measure(next, width-32)
		height = min(max(c.fontSize+48, currentRect.bottom+nextRect.bottom+56), maxHeight)
	}
	var bounds dlRect
	dlUser.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
	position := dlPoint{bounds.left, bounds.top}
	if !c.hasRendered && !c.restoredOrigin {
		position.x += (c.width - width) / 2
		position.y += (c.height - height) / 2
	} else if c.cfg.Draggable && c.hasRendered {
		position.x += (c.width - width) / 2
	}
	if c.cfg.Draggable || !c.hasRendered {
		position.x = max(c.work.left, min(position.x, c.work.right-width))
		position.y = max(c.work.top, min(position.y, c.work.bottom-height))
	}
	c.hasRendered = true
	c.width, c.height = width, height
	if c.surface == nil || c.surface.width != width || c.surface.height != height {
		surface, err := newLyricSurface(width, height)
		if err != nil {
			return err
		}
		if c.surface != nil {
			c.surface.close()
		}
		c.surface = surface
	}
	s := c.surface
	fillLyricRect(s.dc, dlRect{0, 0, width, height}, dlColorKey)
	oldFont, _, _ = dlGDI.NewProc("SelectObject").Call(s.dc, c.font)
	defer dlGDI.NewProc("SelectObject").Call(s.dc, oldFont)
	dlGDI.NewProc("SetBkMode").Call(s.dc, 1)
	active := lyricColor(c.cfg.TextColor, 0xB3C1E5)
	inactive := ((active & 0xff) * 2 / 3) | (((active >> 8 & 0xff) * 2 / 3) << 8) | (((active >> 16 & 0xff) * 2 / 3) << 16)
	draw := func(text string, rect dlRect, color uintptr) {
		dlGDI.NewProc("SetTextColor").Call(s.dc, lyricColor(c.cfg.ShadowColor, 0x101010))
		for _, offset := range []dlPoint{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}} {
			shadow := dlRect{rect.left + offset.x, rect.top + offset.y, rect.right + offset.x, rect.bottom + offset.y}
			drawLyricText(s.dc, text, &shadow, dlTextFlags)
		}
		dlGDI.NewProc("SetTextColor").Call(s.dc, color)
		drawLyricText(s.dc, text, &rect, dlTextFlags)
	}
	if c.cfg.Vertical {
		columnIndex := int32(0)
		for group, columns := range [][][]string{currentColumns, nextColumns} {
			color := active
			if group == 1 {
				color = inactive
			}
			for _, column := range columns {
				for row, text := range column {
					x, y := int32(16)+columnIndex*cell, int32(36)+int32(row)*cell
					draw(text, dlRect{x, y, x + cell, y + cell}, color)
				}
				columnIndex++
			}
		}
	} else {
		draw(current, dlRect{16, 36, width - 16, min(height-8, 36+currentRect.bottom)}, active)
		if next != "" {
			draw(next, dlRect{16, 44 + currentRect.bottom, width - 16, height - 8}, inactive)
		}
	}
	// A persistent, generously padded handle makes the settings entry discoverable.
	fillLyricRect(s.dc, dlRect{width - 64, 4, width - 8, 32}, 0x272E30)
	dlGDI.NewProc("SelectObject").Call(s.dc, c.handleFont)
	dlGDI.NewProc("SetTextColor").Call(s.dc, 0x4D77D4)
	handle := dlRect{width - 64, 4, width - 8, 32}
	drawLyricText(s.dc, "设置", &handle, 0x25) // DT_CENTER | DT_VCENTER | DT_SINGLELINE
	dlGDI.NewProc("GdiFlush").Call()
	// Alpha=2 keeps the entire padded rectangle hit-testable without a visible box.
	// Text is premultiplied independently; global opacity must not erase the hit area.
	alpha := uint32(c.cfg.WindowAlpha * 255)
	for i, pixel := range s.pixels {
		if pixel&0xffffff == dlColorKey {
			s.pixels[i] = 2 << 24
			continue
		}
		r, g, b := (pixel>>16&255)*alpha/255, (pixel>>8&255)*alpha/255, (pixel&255)*alpha/255
		s.pixels[i] = alpha<<24 | r<<16 | g<<8 | b
	}
	size, origin := dlPoint{width, height}, dlPoint{}
	blend := [4]byte{0, 0, 255, 1} // AC_SRC_OVER, AC_SRC_ALPHA
	if ok, _, err := dlUser.NewProc("UpdateLayeredWindow").Call(hwnd, 0, uintptr(unsafe.Pointer(&position)), uintptr(unsafe.Pointer(&size)), s.dc, uintptr(unsafe.Pointer(&origin)), 0, uintptr(unsafe.Pointer(&blend)), 2); ok == 0 {
		return fmt.Errorf("update lyric surface: %w", err)
	}
	return nil
}
