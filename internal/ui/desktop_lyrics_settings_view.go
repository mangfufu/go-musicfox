package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/charmbracelet/x/ansi"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
)

const (
	lsHorizontal = iota
	lsVertical
	lsSmaller
	lsSize
	lsLarger
	lsPixel
	lsYaHei
	lsSong
	lsMoreFont
	lsBold
	lsItalic
	lsText
	lsOutline
	lsAlpha50
	lsAlpha75
	lsAlpha100
	lsLock
	lsOneLine
	lsReset
	lsDone
	lsPreset
)

const lyricsSettingsBackground = "#151411"
const lyricsSettingsAccent = "#D4774D"

type lyricSettingButton struct {
	id, x, y, width int
	label           string
	active          bool
}

func (p *desktopSettingsPage) layout(width int) {
	c := p.config
	p.buttons = []lyricSettingButton{
		{lsHorizontal, 0, 1, 14, "横排", !c.Vertical}, {lsVertical, 16, 1, 14, "竖排", c.Vertical},
		{lsSmaller, 32, 1, 7, "-", false}, {lsSize, 41, 1, 14, fmt.Sprintf("%.0f px", c.FontSize), false}, {lsLarger, 57, 1, 7, "+", false},
		{lsPixel, 0, 6, 14, "像素", strings.EqualFold(c.FontName, desktop.PixelFontName)},
		{lsYaHei, 16, 6, 14, "雅黑", c.FontName == "Microsoft YaHei"}, {lsSong, 32, 6, 14, "宋体", c.FontName == "SimSun"},
		{lsMoreFont, 48, 6, 16, "更多", c.FontName != desktop.PixelFontName && c.FontName != "Microsoft YaHei" && c.FontName != "SimSun"},
		{lsBold, 0, 10, 14, "加粗", c.FontBold}, {lsItalic, 16, 10, 14, "斜体", c.FontItalic},
		{lsText, 0, 15, 30, "文字颜色 " + c.TextColor, false}, {lsOutline, 32, 15, 32, "描边颜色 " + c.ShadowColor, false},
		{lsAlpha50, 0, 20, 20, "50%", c.WindowAlpha == .5}, {lsAlpha75, 22, 20, 20, "75%", c.WindowAlpha == .75}, {lsAlpha100, 44, 20, 20, "100%", c.WindowAlpha == 1},
		{lsLock, 0, 24, 30, "锁定位置", !c.Draggable}, {lsOneLine, 32, 24, 32, "仅当前句", c.OneLineMode},
		{lsReset, 0, 28, 30, "位置复位", false}, {lsDone, 44, 28, 20, "完成", true},
	}
	p.contentWidth, p.contentHeight = 92, 33
	for i, preset := range desktop.StylePresets {
		x, y, w := 68, 1+i*6, 24
		if width < 96 {
			x, y, w = (i%2)*34, 36+(i/2)*6, 30
			p.contentWidth, p.contentHeight = 64, 55
		}
		p.buttons = append(p.buttons, lyricSettingButton{lsPreset + i, x, y, w, preset.Name, preset.Matches(c)})
	}
}

type lyricPanelSpan struct {
	x    int
	text string
}

func (p *desktopSettingsPage) View(a *model.App) string {
	w, h := max(1, a.WindowWidth()), max(1, a.WindowHeight())
	p.layout(w)
	p.viewportW, p.viewportH = max(1, w-4), max(1, h-8)
	if p.ensureFocus {
		for _, b := range p.buttons {
			if b.id == p.selected {
				p.scrollY = min(max(p.scrollY, b.y+3-p.viewportH), b.y)
				p.scrollX = min(max(p.scrollX, b.x+b.width-p.viewportW), b.x)
			}
		}
		p.ensureFocus = false
	}
	p.scrollY = max(0, min(p.scrollY, p.contentHeight-p.viewportH))
	p.scrollX = max(0, min(p.scrollX, p.contentWidth-p.viewportW))
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#D0CBBB")).Background(lipgloss.Color(lyricsSettingsBackground))
	muted := base.Foreground(lipgloss.Color("#999482"))
	accent := base.Foreground(lipgloss.Color(lyricsSettingsAccent))
	rows := make([][]lyricPanelSpan, p.contentHeight)
	put := func(x, y int, text string) {
		if y >= 0 && y < len(rows) {
			rows[y] = append(rows[y], lyricPanelSpan{x, text})
		}
	}
	for _, label := range []struct {
		x, y int
		text string
	}{{0, 0, "显示方向"}, {32, 0, "字号"}, {0, 5, "字体"}, {0, 14, "颜色"}, {0, 19, "不透明度"}} {
		put(label.x, label.y, muted.Render(label.text))
	}
	if w >= 96 {
		put(68, 0, muted.Render("搭配预设"))
		for y := 0; y < 32; y++ {
			put(66, y, muted.Render("│"))
		}
	} else {
		put(0, 34, muted.Render("搭配预设"))
	}
	preview := base.Bold(p.config.FontBold).Italic(p.config.FontItalic)
	if validLyricHex(p.config.TextColor) {
		preview = preview.Foreground(lipgloss.Color(p.config.TextColor))
	}
	put(36, 11, preview.Render("字形 Aa / 月光"))
	current := "自定义"
	for _, b := range p.buttons {
		ink, border, bg := "#D0CBBB", "#4B483E", "#302E27"
		if b.active {
			ink, border, bg = lyricsSettingsAccent, lyricsSettingsAccent, "#463327"
		}
		if b.id == p.selected || b.id == p.hovered {
			border = "#F4D5AE"
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(ink)).Background(lipgloss.Color(bg))
		edge := style.Foreground(lipgloss.Color(border))
		label := b.label
		if b.id == p.selected {
			label = "> " + label
		}
		label = ansi.Truncate(label, b.width-2, "")
		padding := max(0, b.width-2-ansi.StringWidth(label))
		put(b.x, b.y, edge.Render("┌"+strings.Repeat("─", b.width-2)+"┐"))
		put(b.x, b.y+1, edge.Render("│")+style.Render(strings.Repeat(" ", padding/2)+label+strings.Repeat(" ", padding-padding/2))+edge.Render("│"))
		put(b.x, b.y+2, edge.Render("└"+strings.Repeat("─", b.width-2)+"┘"))
		if b.id >= lsPreset {
			preset := desktop.StylePresets[b.id-lsPreset]
			put(b.x, b.y+3, muted.Render(preset.Description))
			if b.active {
				current = preset.Name
			}
		}
	}
	if w >= 96 {
		put(68, 32, accent.Render("当前："+current))
	} else {
		put(0, 54, accent.Render("当前："+current))
	}
	lines := []string{accent.Render("  MUSICFOX / DESKTOP LYRICS"), base.Bold(true).Render("  桌面歌词设置"), muted.Render("  实时生效 · 自动保存 · 字体效果显示在桌面歌词"), ""}
	for y := p.scrollY; y < p.scrollY+p.viewportH; y++ {
		var line strings.Builder
		if y < len(rows) {
			sort.Slice(rows[y], func(i, j int) bool { return rows[y][i].x < rows[y][j].x })
			x := 0
			for _, span := range rows[y] {
				line.WriteString(base.Render(strings.Repeat(" ", max(0, span.x-x))))
				line.WriteString(span.text)
				x = span.x + ansi.StringWidth(span.text)
			}
		}
		lines = append(lines, "  "+ansi.Cut(line.String(), p.scrollX, p.scrollX+p.viewportW))
	}
	status := p.status
	if p.editing {
		p.input.SetWidth(max(1, w-26))
		lines = append(lines, accent.Render("  "+p.editLabel+"  ")+p.input.View(), muted.Render("  Enter 保存 · Esc 取消输入"), accent.Render("  "+status))
	} else {
		lines = append(lines, muted.Render("  Tab / 方向键 选择 · Enter 点击 · 滚轮滚动 · Esc 返回"), accent.Render("  "+status), muted.Render(fmt.Sprintf("  当前搭配：%s    %d–%d / %d", current, p.scrollY+1, min(p.contentHeight, p.scrollY+p.viewportH), p.contentHeight)))
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w, "")
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return base.Width(w).Render(strings.Join(lines[:min(h, len(lines))], "\n"))
}
