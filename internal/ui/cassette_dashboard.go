package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/anhoder/foxful-cli/style"
	"github.com/go-musicfox/go-musicfox/internal/cassette"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
)

// cassetteDashboard shares playback state with the full-screen deck. The native
// menu keeps ownership of navigation, search, pagination and its mouse targets.
type cassetteDashboard struct {
	netease *Netease
}

func (r *cassetteDashboard) Update(_ tea.Msg, a *model.App) {
	// App initialization and OS appearance changes rebuild the vendor StyleSet.
	// Repair its missing menu foreground before the native menu is rendered.
	if _, missing := style.CurrentStyleSet().MenuItem.GetForeground().(lipgloss.NoColor); missing && !style.AccessibleMode() {
		if ss := configs.CurrentThemeRegistry().CurrentStyleSet(style.HasDarkBackground()); ss != nil {
			style.SetStyleSet(*ss)
			a.SetStyleSet(*ss)
		}
	}
}

func (r *cassetteDashboard) View(a *model.App, main *model.Main) (string, int) {
	// Native menus and local search reserve two rows for their hint/input bar.
	height := max(0, main.EffectiveWindowHeight(a)-main.MenuBottomRow()-2)
	player := r.netease.player
	frame := int(player.PassedTime() / (time.Second / 8) % 8)
	s := r.netease.cassetteSnapshot(frame, player.CurSongIndex())
	handler := &EventHandler{keyToOperateMap: keybindings.BuildKeyToOperateTypeMap(configs.EffectiveKeybindings)}
	s.Hints = cassetteHints(handler, true)
	if player.CurSong().Id == 0 {
		s.LyricMessage = "在上方选择歌单，登录后开始播放"
	}
	return cassette.RenderDashboard(s, a.WindowWidth(), height), height
}

func cassettePalette() cassette.Palette {
	registry := configs.CurrentThemeRegistry()
	theme, ok := registry.Get(registry.CurrentName(style.HasDarkBackground()))
	if !ok {
		return cassette.DefaultPalette()
	}
	v := theme.Dark
	if !style.HasDarkBackground() {
		v = theme.Light
	}
	return (cassette.Palette{Background: v.App.Background, Foreground: v.Foreground,
		Secondary: v.Secondary, Muted: v.Muted, Accent: v.Primary, Shell: v.Surface, Edge: v.Border}).WithDefaults()
}

// Dashboard progress is three rows above the bottom content boundary.
func (h *EventHandler) handleCassetteMouse(msg tea.MouseMsg, a *model.App) (bool, model.Page, tea.Cmd) {
	main := a.MustMain()
	if _, ok := msg.(tea.MouseMotionMsg); ok {
		return false, nil, nil
	}
	m := msg.Mouse()
	if _, ok := msg.(tea.MouseClickMsg); ok && m.Button == tea.MouseLeft {
		height := main.EffectiveWindowHeight(a) - main.MenuBottomRow() - 2
		if target, ok := cassetteSeekTarget(m, a.WindowWidth(), main.EffectiveWindowHeight(a), height, h.netease.player.CurSong().Duration); ok {
			h.netease.player.Seek(target)
			return true, main, a.RerenderCmd(true)
		}
	}
	return false, nil, nil
}

func cassetteSeekTarget(m tea.Mouse, windowWidth, bottom, height int, duration time.Duration) (time.Duration, bool) {
	margin := 2
	if windowWidth < 50 {
		margin = 1
	}
	width := windowWidth - 2*margin
	if height < 4 || width <= 1 || duration <= 0 || m.Y != bottom-3 || m.X < margin || m.X >= margin+width {
		return 0, false
	}
	return time.Duration(float64(duration) * float64(m.X-margin) / float64(width-1)), true
}
