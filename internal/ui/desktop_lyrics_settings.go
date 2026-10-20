package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
	"github.com/go-musicfox/go-musicfox/utils/app"
)

func (n *Netease) openDesktopLyricsSettings(from model.Page) (model.Page, tea.Cmd) {
	n.desktopLyricsMu.Lock()
	defer n.desktopLyricsMu.Unlock()
	// Opening settings must not change the user's overlay visibility preference.
	if n.desktopLyrics == nil {
		cfg := configs.AppConfig.Main.Lyric.DesktopLyrics
		cfg.Enable = true
		n.desktopLyrics = desktop.NewController(cfg)
	}
	if editor, ok := n.desktopLyrics.(desktop.SettingsEditor); ok {
		n.bindDesktopLyricsSettings()
		if _, active := from.(*lyricsSettingsPage); active {
			return from, nil
		}
		if n.coverRenderer != nil {
			n.coverRenderer.ClearDisplayed()
		}
		if cassette, ok := from.(*cassettePage); ok {
			cassette.pending = false
		}
		return newLyricsSettingsPage(n, from, editor), nil
	}
	n.Notify(model.NotificationSpec{Level: model.NotificationInfo, Title: "桌面歌词设置", Message: "当前平台不支持桌面歌词设置，或歌词窗口创建失败"})
	return nil, nil
}

func (n *Netease) bindDesktopLyricsSettings() {
	if editor, ok := n.desktopLyrics.(desktop.SettingsEditor); ok {
		editor.SetSettingsHandler(func() {
			n.App.RequestPage(func(a *model.App) (model.Page, tea.Cmd) {
				return n.openDesktopLyricsSettings(a.CurPage())
			})
		})
	}
}

func (n *Netease) toggleDesktopLyrics() {
	n.desktopLyricsMu.Lock()
	enabled := n.desktopLyricsDisabled
	n.desktopLyricsMu.Unlock()
	if err := n.setDesktopLyricsEnabled(enabled, app.ConfigFilePath()); err != nil {
		n.Notify(model.NotificationSpec{Level: model.NotificationError, Title: "桌面歌词", Message: err.Error()})
		return
	}
	message := "桌面歌词已关闭"
	if enabled {
		message = "桌面歌词已开启"
	}
	n.Notify(model.NotificationSpec{Level: model.NotificationInfo, Title: "桌面歌词", Message: message})
}

func (n *Netease) setDesktopLyricsEnabled(enabled bool, configPath string) error {
	err := func() error {
		n.desktopLyricsMu.Lock()
		defer n.desktopLyricsMu.Unlock()
		if enabled && n.desktopLyrics == nil {
			cfg := configs.AppConfig.Main.Lyric.DesktopLyrics
			cfg.Enable = true
			n.desktopLyrics = desktop.NewController(cfg)
			if n.desktopLyrics == nil {
				return fmt.Errorf("当前平台不支持桌面歌词，或歌词窗口创建失败")
			}
			n.bindDesktopLyricsSettings()
		}
		if err := configs.SetTOMLValue(configPath, []string{"main", "lyric", "desktopLyrics", "enable"}, enabled); err != nil {
			return fmt.Errorf("开关未保存，已保留原状态：%w", err)
		}
		n.desktopLyricsDisabled = !enabled
		if !enabled && n.desktopLyrics != nil {
			n.desktopLyrics.Hide()
		}
		return nil
	}()
	if err == nil && enabled && n.player != nil {
		n.player.updateDesktopLyrics()
	}
	return err
}
