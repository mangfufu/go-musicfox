package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/charmbracelet/x/ansi"
	pixelfont "github.com/go-musicfox/go-musicfox/assets/fonts/fusion-pixel"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
	"github.com/go-musicfox/go-musicfox/utils/app"
)

type lyricsSettingsRow struct{ key, label, value string }
type lyricsColor struct{ name, value string }

var lyricsColors = []lyricsColor{
	{"暖白", "#E5C1B3"}, {"纯白", "#FFFFFF"}, {"米黄", "#E8D8B5"},
	{"琥珀", "#FFCD83"}, {"浅蓝", "#A9DCF5"}, {"粉色", "#F4A7C5"},
	{"薄荷", "#A8D5BA"}, {"淡紫", "#C4B5FD"}, {"橙色", "#D4774D"},
	{"深灰", "#101010"}, {"深棕", "#271B12"}, {"深蓝", "#132333"},
}
var lyricsFontNames = []string{pixelfont.Name, "Microsoft YaHei", "SimSun"}
var lyricsPresetNames = []string{"默认", "琥珀像素", "冰蓝像素", "清晰雅黑", "书页宋体"}

type lyricsSettingsPage struct {
	netease                    *Netease
	from                       model.Page
	editor                     desktop.SettingsEditor
	cfg                        configs.DesktopLyricsConfig
	selected, offset, preset   int
	colorKey                   string
	colorSelected, colorOffset int
	busy                       bool
	status                     string
}
type lyricsSettingsResult struct {
	page *lyricsSettingsPage
	cfg  configs.DesktopLyricsConfig
	err  error
}

func newLyricsSettingsPage(n *Netease, from model.Page, editor desktop.SettingsEditor) *lyricsSettingsPage {
	return &lyricsSettingsPage{netease: n, from: from, editor: editor, cfg: editor.Settings(), status: "修改立即生效并自动保存"}
}
func (p *lyricsSettingsPage) Type() model.PageType             { return "desktop_lyrics_settings" }
func (p *lyricsSettingsPage) IgnoreQuitKeyMsg(tea.KeyMsg) bool { return true }
func (p *lyricsSettingsPage) Msg() tea.Msg                     { return nil }

func lyricOnOff(value bool) string {
	if value {
		return "开启"
	}
	return "关闭"
}
func (p *lyricsSettingsPage) enabled() bool {
	p.netease.desktopLyricsMu.Lock()
	defer p.netease.desktopLyricsMu.Unlock()
	return !p.netease.desktopLyricsDisabled
}
func (p *lyricsSettingsPage) rows() []lyricsSettingsRow {
	font := p.cfg.FontName
	for i, name := range lyricsFontNames {
		if name == font {
			font = []string{"像素", "微软雅黑", "宋体"}[i]
		}
	}
	align := map[string]string{"left": "靠左", "center": "居中", "right": "靠右"}[p.cfg.TextAlign]
	if align == "" {
		align = "居中"
	}
	direction := "横排"
	if p.cfg.Vertical {
		direction = "竖排"
		align += "（横排时生效）"
	}
	return []lyricsSettingsRow{
		{"enable", "桌面歌词", lyricOnOff(p.enabled())},
		{"fontName", "字体", font}, {"fontDialog", "更多字体", "打开系统字体选择框"},
		{"fontSize", "字号", fmt.Sprintf("%.0f px", p.cfg.FontSize)},
		{"fontBold", "加粗", lyricOnOff(p.cfg.FontBold)}, {"fontItalic", "斜体", lyricOnOff(p.cfg.FontItalic)},
		{"textColor", "文字颜色", p.cfg.TextColor}, {"shadowColor", "描边颜色", p.cfg.ShadowColor},
		{"windowAlpha", "不透明度", fmt.Sprintf("%.0f%%", p.cfg.WindowAlpha*100)},
		{"textAlign", "文字对齐", align}, {"vertical", "排列方向", direction},
		{"oneLineMode", "仅当前句", lyricOnOff(p.cfg.OneLineMode)},
		{"draggable", "锁定位置", lyricOnOff(!p.cfg.Draggable)},
		{"preset", "应用外观预设", lyricsPresetNames[p.preset]},
		{"resetPosition", "位置复位", "移回屏幕底部中央"},
	}
}
func (p *lyricsSettingsPage) colors() []lyricsColor {
	result := append([]lyricsColor(nil), lyricsColors...)
	for _, value := range p.cfg.RecentColors {
		found := false
		for _, item := range result {
			if strings.EqualFold(item.value, value) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, lyricsColor{"最近使用", value})
		}
	}
	return append(result, lyricsColor{"自定义颜色…", ""})
}
func (p *lyricsSettingsPage) cursor() (*int, *int, int) {
	if p.colorKey != "" {
		return &p.colorSelected, &p.colorOffset, len(p.colors())
	}
	return &p.selected, &p.offset, len(p.rows())
}
func (p *lyricsSettingsPage) scroll(height int) {
	selected, offset, total := p.cursor()
	*selected = max(0, min(*selected, total-1))
	visible := max(1, height-9)
	*offset = max(0, min(*offset, total-visible))
	if *selected < *offset {
		*offset = *selected
	}
	if *selected >= *offset+visible {
		*offset = *selected - visible + 1
	}
}
func (p *lyricsSettingsPage) back(a *model.App) (model.Page, tea.Cmd) {
	if p.colorKey != "" {
		p.colorKey = ""
		return p, nil
	}
	from := p.from
	if from == nil {
		from = a.MustMain()
	}
	return from, func() tea.Msg { return from.Msg() }
}
func (p *lyricsSettingsPage) Update(msg tea.Msg, a *model.App) (model.Page, tea.Cmd) {
	switch msg := msg.(type) {
	case lyricsSettingsResult:
		if msg.page != p {
			return p, nil
		}
		p.busy = false
		p.cfg = msg.cfg
		p.status = "已应用并保存"
		if msg.err != nil {
			p.status = msg.err.Error()
		}
	case tea.WindowSizeMsg:
		p.scroll(msg.Height)
	case tea.KeyPressMsg:
		if p.busy {
			return p, nil
		}
		key := msg.String()
		bindings := keybindings.BuildKeyToOperateTypeMap(configs.EffectiveKeybindings)
		if bindings[key] == keybindings.OpToggleDesktopLyrics {
			return p, p.changeEnable()
		}
		selected, _, total := p.cursor()
		switch key {
		case "esc", "q":
			return p.back(a)
		case "up", "k", "shift+tab":
			*selected = max(0, *selected-1)
		case "down", "j", "tab":
			*selected = min(total-1, *selected+1)
		case "pgup":
			*selected = max(0, *selected-max(1, a.WindowHeight()-9))
		case "pgdown":
			*selected = min(total-1, *selected+max(1, a.WindowHeight()-9))
		case "left", "h":
			return p, p.activate(-1, false)
		case "right", "l":
			return p, p.activate(1, false)
		case "enter", "space":
			return p, p.activate(1, true)
		}
		p.scroll(a.WindowHeight())
	case tea.MouseWheelMsg:
		if p.busy {
			return p, nil
		}
		selected, _, total := p.cursor()
		if msg.Mouse().Button == tea.MouseWheelUp {
			*selected = max(0, *selected-1)
		} else {
			*selected = min(total-1, *selected+1)
		}
		p.scroll(a.WindowHeight())
	case tea.MouseClickMsg:
		if p.busy || msg.Mouse().Button != tea.MouseLeft {
			return p, nil
		}
		mouse := msg.Mouse()
		if mouse.Y == 0 && mouse.X < 10 {
			return p.back(a)
		}
		selected, offset, total := p.cursor()
		index := mouse.Y - 6 + *offset
		if mouse.Y >= 6 && mouse.Y < a.WindowHeight()-3 && index < total && mouse.X >= 0 && mouse.X < a.WindowWidth() {
			*selected = index
			step := 1
			if mouse.X < max(20, a.WindowWidth()/2) {
				step = -1
			}
			if p.colorKey == "" && p.rows()[index].key == "preset" {
				p.preset = (p.preset + step + len(lyricsPresetNames)) % len(lyricsPresetNames)
			}
			return p, p.activate(step, true)
		}
	}
	return p, nil
}
func (p *lyricsSettingsPage) changeEnable() tea.Cmd {
	p.busy = true
	p.status = "正在保存…"
	enabled := !p.enabled()
	return func() tea.Msg {
		err := p.netease.setDesktopLyricsEnabled(enabled, app.ConfigFilePath())
		return lyricsSettingsResult{p, p.editor.Settings(), err}
	}
}
func (p *lyricsSettingsPage) edit(change desktop.SettingsChange) tea.Cmd {
	p.busy = true
	p.status = "正在应用…"
	if strings.HasSuffix(change.Key, "Dialog") {
		p.status = "请在系统选择框中确认或取消"
	}
	return func() tea.Msg { cfg, err := p.editor.EditSettings(change); return lyricsSettingsResult{p, cfg, err} }
}
func cycleLyricValue(values []string, current string, step int) string {
	index := 0
	for i, value := range values {
		if value == current {
			index = i
			break
		}
	}
	return values[(index+step+len(values))%len(values)]
}
func (p *lyricsSettingsPage) activate(step int, confirm bool) tea.Cmd {
	if p.colorKey != "" {
		colors := p.colors()
		if !confirm {
			p.colorSelected = (p.colorSelected + step + len(colors)) % len(colors)
			return nil
		}
		color := colors[p.colorSelected]
		key := p.colorKey
		if color.value == "" {
			return p.edit(desktop.SettingsChange{Key: key + "Dialog"})
		}
		return p.edit(desktop.SettingsChange{Key: key, Value: color.value})
	}
	row := p.rows()[p.selected]
	change := desktop.SettingsChange{Key: row.key, Step: step}
	switch row.key {
	case "enable":
		return p.changeEnable()
	case "textColor", "shadowColor":
		p.colorKey = row.key
		p.colorOffset = 0
		p.colorSelected = 0
		for i, color := range p.colors() {
			if strings.EqualFold(color.value, row.value) {
				p.colorSelected = i
				break
			}
		}
		return nil
	case "fontName":
		change.Value = cycleLyricValue(lyricsFontNames, p.cfg.FontName, step)
	case "textAlign":
		change.Value = cycleLyricValue([]string{"left", "center", "right"}, p.cfg.TextAlign, step)
	case "preset":
		if !confirm {
			p.preset = (p.preset + step + len(lyricsPresetNames)) % len(lyricsPresetNames)
			p.status = "回车应用当前预设"
			return nil
		}
		change.Value = strconv.Itoa(p.preset)
	case "fontDialog", "resetPosition":
		if !confirm {
			return nil
		}
	}
	return p.edit(change)
}
func (p *lyricsSettingsPage) View(a *model.App) string {
	w, h := a.WindowWidth(), a.WindowHeight()
	if w <= 0 || h <= 0 {
		return ""
	}
	if h < 10 || w < 24 {
		lines := make([]string, h)
		lines[0] = ansi.Truncate("桌面歌词设置", w, "")
		if h > 1 {
			lines[1] = ansi.Truncate("请放大终端窗口", w, "")
		}
		if h > 2 {
			lines[h-1] = ansi.Truncate("Esc 返回", w, "")
		}
		return strings.Join(lines, "\n")
	}
	palette := cassettePalette()
	base := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Foreground)).Background(lipgloss.Color(palette.Background))
	accent := base.Foreground(lipgloss.Color(palette.Accent))
	muted := base.Foreground(lipgloss.Color(palette.Secondary))
	line := func(text string) string { return base.Width(w).Render(ansi.Truncate(text, w, "…")) }
	lines := []string{line(accent.Render("< 返回  /  桌面歌词设置")), line(muted.Render("F10 开关  ·  修改自动保存"))}
	preview := lipgloss.NewStyle().Foreground(lipgloss.Color(p.cfg.TextColor)).Background(lipgloss.Color(palette.Shell)).Width(max(1, w-4))
	switch p.cfg.TextAlign {
	case "left":
		preview = preview.Align(lipgloss.Left)
	case "right":
		preview = preview.Align(lipgloss.Right)
	default:
		preview = preview.Align(lipgloss.Center)
	}
	lines = append(lines, line("  "+preview.Render(ansi.Truncate("让每一句歌词，都有自己的位置", max(1, w-4), "…"))))
	lines = append(lines, line(muted.Render("  终端示意颜色与对齐，实际字体和字号请看桌面歌词")), line(""))
	title := "  选择设置项                         < 调整 >"
	if p.colorKey != "" {
		title = "  选择文字颜色"
		if p.colorKey == "shadowColor" {
			title = "  选择描边颜色"
		}
		title += "  /  点击色块立即应用"
	}
	lines = append(lines, line(accent.Render(title)))
	p.scroll(h)
	selected, offset, total := p.cursor()
	for i := 0; i < max(0, h-9); i++ {
		index := *offset + i
		if index >= total {
			lines = append(lines, line(""))
			continue
		}
		text := ""
		color := ""
		if p.colorKey != "" {
			item := p.colors()[index]
			color = item.value
			text = item.name + "  " + item.value
		} else {
			row := p.rows()[index]
			text = lipgloss.NewStyle().Width(18).Render(row.label) + "  " + row.value
			if row.key == "textColor" || row.key == "shadowColor" {
				color = row.value
			}
		}
		marker := "  "
		rowStyle := base
		if index == *selected {
			marker = "> "
			rowStyle = base.Foreground(lipgloss.Color(palette.Accent)).Background(lipgloss.Color(palette.Shell))
		}
		if color != "" {
			text = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("██") + "  " + text
		}
		lines = append(lines, line(rowStyle.Render(marker+text)))
	}
	lines = append(lines, line(muted.Render(fmt.Sprintf("  %d / %d  ·  %s", *selected+1, total, p.status))), line("↑↓ 选择  ←→ 调整  Enter 确认  Esc 返回"), line("鼠标点击选项或颜色；滚轮翻动列表"))
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}
