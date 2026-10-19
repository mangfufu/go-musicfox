package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/charmbracelet/x/ansi"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
)

const desktopSettingsPageType model.PageType = "desktopLyricsSettings"

var desktopSettingLabels = []string{"字号", "字体", "粗体", "斜体", "文字颜色", "描边颜色", "不透明度", "显示方向", "锁定位置", "只显示当前句", "移回屏幕底部中央"}

type desktopSettingsResult struct {
	page *desktopSettingsPage
	err  error
}

type desktopSettingsPage struct {
	controller                                  desktop.SettingsController
	config                                      configs.DesktopLyricsConfig
	back                                        model.Page
	handler                                     *EventHandler
	selected, offset, rowStart, rowCount, width int
	input                                       textinput.Model
	editing, pending                            bool
	status                                      string
}

func newDesktopSettingsPage(n *Netease, back model.Page, handler *EventHandler) *desktopSettingsPage {
	p := &desktopSettingsPage{back: back, handler: handler, input: textinput.New()}
	p.controller, _ = n.desktopLyrics.(desktop.SettingsController)
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

func (p *desktopSettingsPage) values() []string {
	on := func(b bool) string {
		if b {
			return "开启"
		}
		return "关闭"
	}
	direction := "横排"
	if p.config.Vertical {
		direction = "竖排"
	}
	return []string{
		fmt.Sprintf("%.0f px", p.config.FontSize), p.config.FontName,
		on(p.config.FontBold), on(p.config.FontItalic), p.config.TextColor, p.config.ShadowColor,
		fmt.Sprintf("%.0f%%", p.config.WindowAlpha*100), direction, on(!p.config.Draggable), on(p.config.OneLineMode), "执行",
	}
}

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

func (p *desktopSettingsPage) adjust(delta int, edit bool) tea.Cmd {
	if p.controller == nil || p.pending {
		return nil
	}
	next := p.config
	if edit && (p.selected == 0 || p.selected == 1 || p.selected == 4 || p.selected == 5 || p.selected == 6) {
		value := p.values()[p.selected]
		if p.selected == 0 {
			value = fmt.Sprintf("%.0f", next.FontSize)
		}
		if p.selected == 6 {
			value = fmt.Sprintf("%.0f", next.WindowAlpha*100)
		}
		p.input.SetValue(value)
		p.input.CursorEnd()
		p.editing, p.status = true, "Enter 保存 · Esc 取消；颜色使用 #RRGGBB，字体可输入已安装字体名称"
		focusPageInput(&p.input)
		return textinput.Blink
	}
	switch p.selected {
	case 0:
		step := 2.0
		if strings.HasPrefix(strings.ToLower(next.FontName), "fusion pixel") {
			step = 12
		}
		next.FontSize = max(12, min(96, next.FontSize+float64(delta)*step))
	case 1:
		fonts := []string{"Fusion Pixel 12px Mono zh-Hans", "Microsoft YaHei", "SimSun"}
		index := 0
		for i, font := range fonts {
			if font == next.FontName {
				index = i
				break
			}
		}
		next.FontName = fonts[(index+delta+len(fonts))%len(fonts)]
	case 2:
		next.FontBold = !next.FontBold
	case 3:
		next.FontItalic = !next.FontItalic
	case 4, 5:
		return p.adjust(delta, true)
	case 6:
		next.WindowAlpha = float64(max(50, min(100, int(next.WindowAlpha*100+0.5)+delta*5))) / 100
	case 7:
		next.Vertical = !next.Vertical
	case 8:
		next.Draggable = !next.Draggable
	case 9:
		next.OneLineMode = !next.OneLineMode
	case 10:
		return p.save(next, true)
	}
	return p.save(next, false)
}

func (p *desktopSettingsPage) submit() tea.Cmd {
	value := strings.TrimSpace(p.input.Value())
	next := p.config
	switch p.selected {
	case 0, 6:
		number, err := strconv.Atoi(value)
		low, high := 12, 96
		if p.selected == 6 {
			low, high = 50, 100
		}
		if err != nil || number < low || number > high {
			p.status = fmt.Sprintf("请输入 %d 到 %d 的整数", low, high)
			return nil
		}
		if p.selected == 0 {
			next.FontSize = float64(number)
		} else {
			next.WindowAlpha = float64(number) / 100
		}
	case 1:
		if value == "" {
			p.status = "字体名称不能为空"
			return nil
		}
		next.FontName = value
	case 4, 5:
		_, err := strconv.ParseUint(strings.TrimPrefix(value, "#"), 16, 24)
		if len(value) != 7 || value[0] != '#' || err != nil {
			p.status = "颜色格式为 #RRGGBB，例如 #E5C1B3"
			return nil
		}
		if p.selected == 4 {
			next.TextColor = strings.ToUpper(value)
		} else {
			next.ShadowColor = strings.ToUpper(value)
		}
	}
	p.editing = false
	blurPageInput(&p.input)
	return p.save(next, false)
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
	// Finish an in-flight save before leaving so reopening cannot show stale values.
	if p.pending {
		return p, nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "esc" || (p.handler != nil && p.handler.keyToOperateMap[msg.String()] == keybindings.OpDesktopLyricsSettings) {
			return p.back, tea.Batch(a.RerenderCmd(true), func() tea.Msg { return p.back.Msg() })
		}
		switch msg.String() {
		case "up", "k", "shift+tab":
			p.selected = (p.selected + len(desktopSettingLabels) - 1) % len(desktopSettingLabels)
		case "down", "j", "tab":
			p.selected = (p.selected + 1) % len(desktopSettingLabels)
		case "left", "-":
			return p, p.adjust(-1, false)
		case "right", "+", "=":
			return p, p.adjust(1, false)
		case "enter", "space":
			return p, p.adjust(1, true)
		}
	case tea.MouseClickMsg:
		m := msg.Mouse()
		if m.Button != tea.MouseLeft {
			break
		}
		if m.Y == 1 && m.X >= 2 && m.X < 12 {
			return p.back, tea.Batch(a.RerenderCmd(true), func() tea.Msg { return p.back.Msg() })
		}
		if m.X >= 2 && m.X < p.width-2 && m.Y >= p.rowStart && m.Y < p.rowStart+p.rowCount {
			p.selected = p.offset + m.Y - p.rowStart
			return p, p.adjust(1, true)
		}
	case tea.MouseWheelMsg:
		if msg.Mouse().Button == tea.MouseWheelUp {
			p.selected = max(0, p.selected-1)
		} else {
			p.selected = min(len(desktopSettingLabels)-1, p.selected+1)
		}
	}
	return p, nil
}

func (p *desktopSettingsPage) View(a *model.App) string {
	w, h := max(1, a.WindowWidth()), max(1, a.WindowHeight())
	p.width = w
	palette := cassettePalette().WithDefaults()
	base := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Foreground)).Background(lipgloss.Color(palette.Background))
	accent := base.Foreground(lipgloss.Color(palette.Accent)).Bold(true)
	muted := base.Foreground(lipgloss.Color(palette.Secondary))
	lines := []string{"", accent.Render("  ← 返回    桌面歌词设置"), muted.Render("  DESKTOP LYRICS / 修改自动保存"), ""}
	p.rowStart = len(lines)
	p.rowCount = min(len(desktopSettingLabels), max(1, h-10))
	p.offset = min(max(p.offset, p.selected-p.rowCount+1), p.selected)
	values := p.values()
	for i := p.offset; i < min(len(desktopSettingLabels), p.offset+p.rowCount); i++ {
		prefix := "    "
		if i == p.selected {
			prefix = "  > "
		}
		value := values[i]
		if i == p.selected && p.editing {
			p.input.SetWidth(max(8, w-30))
			value = p.input.View()
		}
		label := desktopSettingLabels[i] + strings.Repeat(" ", max(2, 24-ansi.StringWidth(desktopSettingLabels[i])))
		line := prefix + label + value
		if i == p.selected {
			line = accent.Render(line)
		} else {
			line = base.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", muted.Render("  ↑↓ 选择 · ←→ 调整 · Enter 编辑/切换 · Esc 返回"), muted.Render("  字体：左右切换像素/雅黑/宋体，Enter 输入其他字体"), accent.Render("  "+p.status))
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w, "")
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return base.Width(w).Render(strings.Join(lines[:min(h, len(lines))], "\n"))
}
