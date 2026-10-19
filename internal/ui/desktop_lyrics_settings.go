package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
)

const desktopSettingsPageType model.PageType = "desktopLyricsSettings"

type desktopSettingsResult struct {
	page *desktopSettingsPage
	err  error
}
type desktopSettingsPage struct {
	controller                                                          desktop.SettingsController
	config                                                              configs.DesktopLyricsConfig
	back                                                                model.Page
	handler                                                             *EventHandler
	buttons                                                             []lyricSettingButton
	selected, hovered                                                   int
	scrollX, scrollY, viewportW, viewportH, contentWidth, contentHeight int
	ensureFocus                                                         bool
	input                                                               textinput.Model
	editing, pending                                                    bool
	editLabel, status                                                   string
}

func (n *Netease) openDesktopLyricsSettings(back model.Page) (model.Page, tea.Cmd) {
	if _, ok := back.(*desktopSettingsPage); ok {
		return back, nil
	}
	if back == nil {
		back = n.MustMain()
	}
	if cassette, ok := back.(*cassettePage); ok {
		cassette.pending = false
	}
	if n.coverRenderer != nil {
		n.coverRenderer.ClearDisplayed()
	}
	return newDesktopSettingsPage(n, back, NewEventHandler(n)), nil
}
func newDesktopSettingsPage(n *Netease, back model.Page, handler *EventHandler) *desktopSettingsPage {
	p := &desktopSettingsPage{back: back, handler: handler, input: textinput.New(), hovered: -1, ensureFocus: true}
	p.controller, _ = n.desktopLyrics.(desktop.SettingsController)
	p.config = desktop.StylePresets[0].Apply(configs.DesktopLyricsConfig{Draggable: true})
	if p.controller != nil {
		p.config = p.controller.Settings()
	} else {
		p.status = "请先启用 Windows 桌面歌词并重启播放器"
	}
	p.input.CharLimit = 100
	return p
}
func (p *desktopSettingsPage) Type() model.PageType             { return desktopSettingsPageType }
func (p *desktopSettingsPage) Msg() tea.Msg                     { return nil }
func (p *desktopSettingsPage) IgnoreQuitKeyMsg(tea.KeyMsg) bool { return true }
func (p *desktopSettingsPage) save(next configs.DesktopLyricsConfig, reset bool) tea.Cmd {
	p.pending, p.status = true, "正在保存…"
	return func() tea.Msg {
		var err error
		if reset {
			err = p.controller.ResetPosition()
		} else {
			err = p.controller.ApplySettings(next)
		}
		return desktopSettingsResult{p, err}
	}
}
func (p *desktopSettingsPage) leave(a *model.App) (model.Page, tea.Cmd) {
	return p.back, tea.Batch(a.RerenderCmd(true), func() tea.Msg { return p.back.Msg() })
}
func (p *desktopSettingsPage) activate(a *model.App) (model.Page, tea.Cmd) {
	if p.selected == lsDone {
		return p.leave(a)
	}
	if p.controller == nil || p.pending {
		return p, nil
	}
	next := p.config
	switch p.selected {
	case lsSize, lsMoreFont, lsText, lsOutline:
		value := next.FontName
		p.editLabel = "字体名称"
		switch p.selected {
		case lsSize:
			value = fmt.Sprintf("%.0f", next.FontSize)
			p.editLabel = "字号 12–96"
		case lsText:
			value = next.TextColor
			p.editLabel = "文字 #RRGGBB"
		case lsOutline:
			value = next.ShadowColor
			p.editLabel = "描边 #RRGGBB"
		}
		p.input.SetValue(value)
		p.input.CursorEnd()
		focusPageInput(&p.input)
		p.editing, p.status = true, ""
		return p, textinput.Blink
	case lsHorizontal:
		next.Vertical = false
	case lsVertical:
		next.Vertical = true
	case lsSmaller, lsLarger:
		step := 2.0
		if strings.EqualFold(next.FontName, desktop.PixelFontName) {
			step = 12
		}
		if p.selected == lsSmaller {
			step = -step
		}
		next.FontSize = max(12, min(96, next.FontSize+step))
	case lsPixel:
		next.FontName = desktop.PixelFontName
	case lsYaHei:
		next.FontName = "Microsoft YaHei"
	case lsSong:
		next.FontName = "SimSun"
	case lsBold:
		next.FontBold = !next.FontBold
	case lsItalic:
		next.FontItalic = !next.FontItalic
	case lsAlpha50:
		next.WindowAlpha = .5
	case lsAlpha75:
		next.WindowAlpha = .75
	case lsAlpha100:
		next.WindowAlpha = 1
	case lsLock:
		next.Draggable = !next.Draggable
	case lsOneLine:
		next.OneLineMode = !next.OneLineMode
	case lsReset:
		return p, p.save(next, true)
	default:
		index := p.selected - lsPreset
		if index < 0 || index >= len(desktop.StylePresets) {
			return p, nil
		}
		next = desktop.StylePresets[index].Apply(next)
	}
	return p, p.save(next, false)
}
func validLyricHex(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(value[1:], 16, 24)
	return err == nil
}
func (p *desktopSettingsPage) submit() tea.Cmd {
	value := strings.TrimSpace(p.input.Value())
	next := p.config
	switch p.selected {
	case lsSize:
		size, err := strconv.Atoi(value)
		if err != nil || size < 12 || size > 96 {
			p.status = "请输入 12 到 96 的整数"
			return nil
		}
		next.FontSize = float64(size)
	case lsMoreFont:
		if value == "" {
			p.status = "请输入已安装的字体名称"
			return nil
		}
		next.FontName = value
	case lsText, lsOutline:
		if !validLyricHex(value) {
			p.status = "请输入 #RRGGBB，例如 #E5C1B3"
			return nil
		}
		if p.selected == lsText {
			next.TextColor = strings.ToUpper(value)
		} else {
			next.ShadowColor = strings.ToUpper(value)
		}
	}
	p.editing = false
	blurPageInput(&p.input)
	return p.save(next, false)
}
func (p *desktopSettingsPage) buttonAt(x, y int) int {
	if y < 4 || y >= 4+p.viewportH || x < 2 || x >= 2+p.viewportW {
		return -1
	}
	x, y = x-2+p.scrollX, y-4+p.scrollY
	for _, b := range p.buttons {
		if x >= b.x && x < b.x+b.width && y >= b.y && y < b.y+3 {
			return b.id
		}
	}
	return -1
}
func (p *desktopSettingsPage) move(key string) {
	if key == "tab" || key == "shift+tab" {
		step := 1
		if key == "shift+tab" {
			step = -1
		}
		p.selected = (p.selected + step + len(p.buttons)) % len(p.buttons)
	} else {
		var origin lyricSettingButton
		for _, b := range p.buttons {
			if b.id == p.selected {
				origin = b
			}
		}
		best, score := p.selected, int(^uint(0)>>1)
		abs := func(x int) int {
			if x < 0 {
				return -x
			}
			return x
		}
		for _, b := range p.buttons {
			dx, dy := b.x+b.width/2-origin.x-origin.width/2, b.y-origin.y
			candidate := 0
			switch key {
			case "left":
				if dx >= 0 {
					continue
				}
				candidate = abs(dx) + abs(dy)*8
			case "right":
				if dx <= 0 {
					continue
				}
				candidate = abs(dx) + abs(dy)*8
			case "up":
				if dy >= 0 {
					continue
				}
				candidate = abs(dy)*8 + abs(dx)
			case "down":
				if dy <= 0 {
					continue
				}
				candidate = abs(dy)*8 + abs(dx)
			}
			if candidate < score {
				best, score = b.id, candidate
			}
		}
		p.selected = best
	}
	p.ensureFocus = true
}
func (p *desktopSettingsPage) Update(msg tea.Msg, a *model.App) (model.Page, tea.Cmd) {
	if result, ok := msg.(desktopSettingsResult); ok {
		if result.page != p {
			return p, nil
		}
		p.pending = false
		p.config = p.controller.Settings()
		p.status = "已保存，桌面歌词已更新"
		if result.err != nil {
			p.status = result.err.Error()
		}
		return p, nil
	}
	if p.editing {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				p.editing = false
				blurPageInput(&p.input)
				p.status = "已取消编辑"
				return p, nil
			case "enter":
				return p, p.submit()
			}
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return p, cmd
	}
	if p.pending {
		return p, nil
	}
	p.layout(a.WindowWidth())
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.ensureFocus = true
	case tea.KeyPressMsg:
		if msg.String() == "esc" || (p.handler != nil && p.handler.keyToOperateMap[msg.String()] == keybindings.OpDesktopLyricsSettings) {
			return p.leave(a)
		}
		switch msg.String() {
		case "tab", "shift+tab", "up", "down", "left", "right":
			p.move(msg.String())
		case "enter", "space":
			return p.activate(a)
		}
	case tea.MouseClickMsg:
		m := msg.Mouse()
		if m.Button == tea.MouseLeft {
			if id := p.buttonAt(m.X, m.Y); id >= 0 {
				p.selected = id
				return p.activate(a)
			}
		}
	case tea.MouseMotionMsg:
		m := msg.Mouse()
		p.hovered = p.buttonAt(m.X, m.Y)
	case tea.MouseWheelMsg:
		if msg.Mouse().Button == tea.MouseWheelUp {
			p.scrollY -= 3
		} else {
			p.scrollY += 3
		}
	}
	return p, nil
}
