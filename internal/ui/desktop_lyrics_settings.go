package ui

import (
	"github.com/anhoder/foxful-cli/model"
	desktop "github.com/go-musicfox/go-musicfox/internal/desktop_lyrics"
)

func (n *Netease) openDesktopLyricsSettings() {
	if opener, ok := n.desktopLyrics.(desktop.SettingsOpener); ok {
		opener.OpenSettings()
		return
	}
	n.Notify(model.NotificationSpec{Level: model.NotificationInfo, Title: "桌面歌词设置", Message: "请先启用 Windows 桌面歌词并重启播放器"})
}
