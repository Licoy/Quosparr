package app

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
)

// Tint is one of the system colors that tell the parts of the settings
// window apart.
type Tint int

// Tints.
const (
	TintBlue Tint = iota
	TintGreen
	TintRed
	TintOrange
	TintTeal
	TintPurple
	TintIndigo
	TintPink
	TintGray
)

var tintHex = [...][2]string{
	TintBlue:   {"#007AFF", "#0A84FF"},
	TintGreen:  {"#28B14C", "#30D158"},
	TintRed:    {"#FF3B30", "#FF453A"},
	TintOrange: {"#F59100", "#FF9F0A"},
	TintTeal:   {"#14A3BE", "#40C8E0"},
	TintPurple: {"#AF52DE", "#BF5AF2"},
	TintIndigo: {"#5856D6", "#5E5CE6"},
	TintPink:   {"#FF2D55", "#FF375F"},
	TintGray:   {"#8E8E93", "#98989D"},
}

var tintColors [len(tintHex)][2]ui.Color

func init() {
	for i, h := range tintHex {
		tintColors[i] = [2]ui.Color{ui.Hex(h[0]), ui.Hex(h[1])}
	}
}

// C is the tint in the theme's appearance.
func (k Tint) C(t *ui.Theme) ui.Color {
	if t.Dark {
		return tintColors[k][1]
	}
	return tintColors[k][0]
}

// Text is the tint for text, darker on light backgrounds to stay legible.
func (k Tint) Text(t *ui.Theme) ui.Color {
	if t.Dark {
		return tintColors[k][1]
	}
	return tintColors[k][0].Mix(ui.RGB(0, 0, 0), 0.12)
}

// palette holds the surfaces the theme has no names for.
type palette struct {
	// Card is a card on the window's background, Inset what sits on a card,
	// Line a hairline, Hover a hovered row.
	Card, Inset, Line, Hover, Sidebar ui.Color
}

var palettes = [2]palette{
	{
		Card:    ui.Hex("#F5F5F7"),
		Sidebar: ui.Hex("#EDEDF0"),
		Inset:   ui.Hex("#FFFFFF"),
		Line:    ui.RGBA(0, 0, 0, 0.07),
		Hover:   ui.RGBA(0, 0, 0, 0.05),
	},
	{
		Card:    ui.Hex("#232325"),
		Sidebar: ui.Hex("#28282A"),
		Inset:   ui.Hex("#303033"),
		Line:    ui.RGBA(255, 255, 255, 0.08),
		Hover:   ui.RGBA(255, 255, 255, 0.06),
	},
}

func pal(t *ui.Theme) palette {
	if t.Dark {
		return palettes[1]
	}
	return palettes[0]
}

// brandInk keeps the accent readable when it is used as foreground on
// cards: lighter on dark ones, and darker where a light theme color would
// fade on white. Filled controls continue to use the exact theme accent.
func brandInk(t *ui.Theme, color ui.Color) ui.Color {
	toward, bg := ui.RGB(0, 0, 0), pal(t).Inset
	if t.Dark {
		color, toward = color.Mix(ui.RGB(255, 255, 255), 0.4), ui.RGB(255, 255, 255)
	}
	for i := 0; i < 10 && contrastRatio(color, bg) < 4.5; i++ {
		color = color.Mix(toward, 0.1)
	}
	return color
}

type themeKey struct {
	dark        bool
	color, font string
}

// themes caches the built themes; a custom color dragged across the picker
// builds many, so the cache starts over once it holds more than a few.
var themes = map[themeKey]*ui.Theme{}

// applyTheme makes the frame use the app's theme in the brand color.
func applyTheme(c *ui.Context) *ui.Theme { return applyThemeColor(c, "", "") }

// applyThemeColor makes the frame use the app's theme in the system's (or
// the chosen) appearance, with the theme color and the font family the user
// chose (store.Preferences.ThemeColor, empty for the brand, and FontFamily,
// empty for the system's; a family the system lacks falls back to it).
// Views call it first.
func applyThemeColor(c *ui.Context, color, font string) *ui.Theme {
	k := themeKey{c.Theme().Dark, color, font}
	t := themes[k]
	if t == nil {
		if len(themes) >= 16 {
			clear(themes)
		}
		t = buildThemeColor(k.dark, color)
		t.Font = font
		themes[k] = t
	}
	c.SetTheme(t)
	return t
}

// buildThemeColor is the theme with the accent color, "#RRGGBB" or empty for
// the brand brown.
func buildThemeColor(dark bool, color string) *ui.Theme {
	t := buildTheme(dark)
	if color == "" {
		return t
	}
	accent := ui.Hex(color)
	t.Accent = accent
	t.AccentHover = accent.Mix(ui.RGB(255, 255, 255), 0.06)
	t.AccentPressed = accent.Mix(ui.RGB(0, 0, 0), 0.15)
	// White text stays on all but the light colors, as on the system's.
	t.AccentText = ui.RGB(255, 255, 255)
	if contrastRatio(accent, t.AccentText) < 3 {
		t.AccentText = ui.Hex("#1D1D1F")
	}
	t.Selection, t.Focus = accent.Alpha(0.25), accent.Alpha(0.5)
	if dark {
		t.Selection, t.Focus = accent.Alpha(0.4), accent.Alpha(0.6)
	}
	return t
}

// contrastRatio is the WCAG contrast of two opaque colors.
func contrastRatio(a, b ui.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(c ui.Color) float64 {
	linear := func(v uint8) float64 {
		n := float64(v) / 255
		if n <= 0.04045 {
			return n / 12.92
		}
		return math.Pow((n+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}

func buildTheme(dark bool) *ui.Theme {
	if dark {
		t := ui.DarkTheme()
		t.Background = ui.Hex("#18181A")
		t.Surface = ui.Hex("#333336")
		t.SurfaceHover = ui.Hex("#3C3C3F")
		t.SurfacePressed = ui.Hex("#47474A")
		t.Border = ui.Hex("#3A3A3D")
		t.Text = ui.Hex("#F5F5F7")
		t.TextMuted = ui.Hex("#98989D")
		t.Accent = ui.Hex("#B3582D")
		t.AccentHover = ui.Hex("#B95C31")
		t.AccentPressed = ui.Hex("#984925")
		t.Danger = ui.Hex("#FF453A")
		t.Warning = ui.Hex("#FF9F0A")
		t.Success = ui.Hex("#30D158")
		t.Selection = ui.RGBA(179, 88, 45, 0.4)
		t.Focus = ui.RGBA(179, 88, 45, 0.6)
		t.Inverse = ui.Hex("#F5F5F7")
		t.InverseText = ui.Hex("#1D1D1F")
		t.Radius = 8
		return t
	}
	t := ui.LightTheme()
	t.Background = ui.Hex("#FFFFFF")
	t.Surface = ui.Hex("#FFFFFF")
	t.SurfaceHover = ui.Hex("#F2F2F4")
	t.SurfacePressed = ui.Hex("#E8E8EB")
	t.Border = ui.Hex("#DCDCE0")
	t.Text = ui.Hex("#1D1D1F")
	t.TextMuted = ui.Hex("#6E6E73")
	t.Accent = ui.Hex("#B3582D")
	t.AccentHover = ui.Hex("#B95C31")
	t.AccentPressed = ui.Hex("#984925")
	t.Danger = ui.Hex("#E5342A")
	t.Warning = ui.Hex("#D97E00")
	t.Success = ui.Hex("#24A148")
	t.Selection = ui.RGBA(179, 88, 45, 0.25)
	t.Focus = ui.RGBA(179, 88, 45, 0.5)
	t.Inverse = ui.Hex("#1D1D1F")
	t.InverseText = ui.Hex("#FFFFFF")
	t.Radius = 8
	return t
}

// Tile is a rounded square holding an icon: tinted lightly with the icon in
// the tint, or, when solid, filled with the tint and a white icon.
func Tile(c *ui.Context, icon *ui.SVG, k Tint, size float32, solid bool) ui.Element {
	t := c.Theme()
	col := k.C(t)
	bg, fg := col.Alpha(0.15), k.Text(t)
	if solid {
		bg, fg = col, ui.RGB(255, 255, 255)
	}
	return ui.Row(c).Size(size, size).Radius(size * 0.28).Background(bg).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).Size(size*0.58, size*0.58).TextColor(fg)
	})
}

// Panel is a grouped box of the settings window, as System Settings draws
// its form sections: a faint fill and hairline, with rows 12 DIPs inside.
func Panel(c *ui.Context) ui.Element {
	p := pal(c.Theme())
	return ui.Column(c).Padding(3, 11).Gap(0).Radius(12).Background(p.Card).BorderWidth(1).BorderColor(p.Line)
}

// PillButton is a capsule button with an optional icon. A disabled one
// reports no clicks.
func PillButton(c *ui.Context, icon *ui.SVG, label string, disabled bool) ui.Element {
	return pillButton(c, icon, label, disabled, false, false)
}

// SmallPillButton is a PillButton for dense toolbars, such as filters.
func SmallPillButton(c *ui.Context, icon *ui.SVG, label string, disabled bool) ui.Element {
	return pillButton(c, icon, label, disabled, false, true)
}

// WarningPillButton is a PillButton filled with the warning color, for an
// action that has work waiting, such as unsaved changes.
func WarningPillButton(c *ui.Context, label string) ui.Element {
	return pillButton(c, nil, label, false, true, false)
}

// SmallWarningPillButton is a WarningPillButton for dense toolbars.
func SmallWarningPillButton(c *ui.Context, label string) ui.Element {
	return pillButton(c, nil, label, false, true, true)
}

func pillButton(c *ui.Context, icon *ui.SVG, label string, disabled, warn, small bool) ui.Element {
	t := c.Theme()
	padY, padX, gap, size := float32(6), float32(12), float32(6), float32(13)
	if small {
		padY, padX, gap, size = 5, 9, 4, 12
	}
	b := ui.ButtonBase(c).Label(label).Disabled(disabled).Padding(padY, padX).Gap(gap).Radius(6).AlignItems(ui.Center)
	if label == "" {
		b.Padding(padY, padY+2) // an icon alone: name it with Label
	}
	bg, col := t.Text.Alpha(0.06), t.Text
	if warn {
		bg, col = t.Warning, ui.RGB(255, 255, 255)
	}
	switch {
	case disabled:
		col = t.TextMuted
	case b.Pressed() && warn:
		bg = t.Warning.Alpha(0.8)
	case b.Hovered() && warn:
		bg = t.Warning.Alpha(0.9)
	case b.Pressed():
		bg = t.Text.Alpha(0.14)
	case b.Hovered():
		bg = t.Text.Alpha(0.10)
	}
	b.Background(bg)
	return b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(size).TextColor(col)
		}
		if label != "" {
			ui.Text(c, label).FontSize(size).FontWeight(500).TextColor(col).SingleLine()
		}
	})
}

// PillTabs is a capsule segmented control. It reports whether the choice
// changed. A disabled control reports no changes.
func PillTabs(c *ui.Context, selected *int, disabled bool, labels ...string) bool {
	t := c.Theme()
	seg := ui.SegmentedBase(c, selected, len(labels))
	seg.Track.Padding(3).Gap(2).Radius(7).Background(t.Text.Alpha(0.06)).Children(func() {
		for i, l := range labels {
			s := seg.Segment(i).Padding(5, 10).Radius(5).Disabled(disabled)
			col := t.TextMuted
			if i == *selected {
				col = t.Text
				if disabled {
					col = t.TextMuted
				}
				s.Background(pal(t).Inset).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.12))
			}
			s.Children(func() { ui.Text(c, l).FontSize(13).FontWeight(500).TextColor(col).SingleLine() })
		}
	})
	// mygo applies the click as the track's children are built, so selected
	// already holds it; Changed reports whether it moved.
	return seg.Track.Changed()
}

// IconButton is compact toolbar chrome with a translated tooltip and name.
func IconButton(c *ui.Context, icon *ui.SVG, label string, disabled bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Disabled(disabled).Size(26, 26).Radius(5).AlignItems(ui.Center).Justify(ui.Center)
	if !disabled {
		b.Cursor(ui.CursorPointer)
	}
	if b.Pressed() {
		b.Background(t.Text.Alpha(0.12))
	} else if b.Hovered() {
		b.Background(pal(t).Hover)
	}
	col := t.TextMuted
	if disabled {
		col = col.Alpha(0.5)
	}
	return b.Children(func() { ui.Icon(c, icon).Size(14, 14).TextColor(col) })
}

// inverseColors is the tooltip fill and ink of t, as mygo resolves them.
func inverseColors(t *ui.Theme) (fill, text ui.Color) {
	fill, text = t.Inverse, t.InverseText
	if fill == ui.Transparent {
		fill = t.Text
	}
	if text == ui.Transparent {
		text = t.Background
	}
	return fill, text
}

// applyPanelTheme shows native material only when the host actually provides it.
func applyPanelTheme(c *ui.Context, style store.PanelBackground, color, font string) *ui.Theme {
	t := applyThemeColor(c, color, font)
	if style == store.PanelBackgroundTranslucent && c.Vibrancy() {
		material := *t
		// Tooltips draw the inverse colours, which fall back to Text on
		// Background; keep that ink opaque once the background is cleared.
		material.Inverse, material.InverseText = inverseColors(t)
		material.Background = ui.Transparent
		c.SetTheme(&material)
		c.Root().Background(ui.Transparent)
		return &material
	}
	return t
}
