package desktop_lyrics

import (
	"strings"

	"github.com/go-musicfox/go-musicfox/internal/configs"
)

const PixelFontName = "Fusion Pixel 12px Mono zh-Hans"

type StylePreset struct {
	Name, Description, Font, Text, Outline string
	Size                                   float64
	Bold                                   bool
}

var StylePresets = []StylePreset{
	{"默认", "像素 36 px · 暖白", PixelFontName, "#E5C1B3", "#101010", 36, false},
	{"琥珀像素", "像素 48 px · 加粗", PixelFontName, "#FFCD83", "#271B12", 48, true},
	{"冰蓝像素", "像素 36 px · 冰蓝", PixelFontName, "#A9DCF5", "#132333", 36, false},
	{"清晰雅黑", "雅黑 36 px · 加粗", "Microsoft YaHei", "#F5F7FA", "#141820", 36, true},
	{"书页宋体", "宋体 36 px · 米白", "SimSun", "#E8D8B5", "#282119", 36, false},
}

func (p StylePreset) Apply(cfg configs.DesktopLyricsConfig) configs.DesktopLyricsConfig {
	cfg.FontName, cfg.FontSize = p.Font, p.Size
	cfg.FontBold, cfg.FontItalic = p.Bold, false
	cfg.TextColor, cfg.ShadowColor, cfg.WindowAlpha = p.Text, p.Outline, 1
	return cfg
}

func (p StylePreset) Matches(cfg configs.DesktopLyricsConfig) bool {
	return strings.EqualFold(cfg.FontName, p.Font) && cfg.FontSize == p.Size &&
		cfg.FontBold == p.Bold && !cfg.FontItalic && cfg.WindowAlpha == 1 &&
		strings.EqualFold(cfg.TextColor, p.Text) && strings.EqualFold(cfg.ShadowColor, p.Outline)
}
