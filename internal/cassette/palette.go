package cassette

// Palette colors both the dashboard and the full-screen cassette view.
type Palette struct {
	Background, Foreground, Secondary, Muted, Accent, Shell, Edge string
}

func DefaultPalette() Palette {
	return Palette{Background, Foreground, secondary, muted, accent, shell, edge}
}

// WithDefaults fills missing or transparent colors with the original deck palette.
func (p Palette) WithDefaults() Palette {
	d := DefaultPalette()
	fields := []*string{&p.Background, &p.Foreground, &p.Secondary, &p.Muted, &p.Accent, &p.Shell, &p.Edge}
	defaults := []string{d.Background, d.Foreground, d.Secondary, d.Muted, d.Accent, d.Shell, d.Edge}
	for i, field := range fields {
		if *field == "" || *field == "transparent" {
			*field = defaults[i]
		}
	}
	return p
}

type painter struct{ Palette }

func (r painter) color(c string) string {
	switch c {
	case Background:
		return r.Background
	case Foreground:
		return r.Foreground
	case secondary:
		return r.Secondary
	case muted:
		return r.Muted
	case accent:
		return r.Accent
	case shell:
		return r.Shell
	case edge:
		return r.Edge
	default:
		return c
	}
}
