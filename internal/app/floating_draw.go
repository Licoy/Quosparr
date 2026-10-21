package app

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// The floating window is drawn by one painter, so the window can take the
// exact size the same layout computes (floatingSize). Sizes are DIPs.

// floatingInk are the colors of one appearance.
type floatingInk struct {
	bg, border, text, muted, track ui.Color
	dark                           bool
}

func floatingInkFor(t *ui.Theme) floatingInk {
	if t.Dark {
		return floatingInk{bg: ui.Hex("#1F1F21"), border: ui.RGBA(255, 255, 255, 0.12), text: ui.Hex("#F5F5F7"),
			muted: ui.RGBA(235, 235, 245, 0.6), track: ui.RGBA(255, 255, 255, 0.14), dark: true}
	}
	return floatingInk{bg: ui.Hex("#FFFFFF"), border: ui.RGBA(0, 0, 0, 0.1), text: ui.Hex("#1D1D1F"),
		muted: ui.RGBA(60, 60, 67, 0.62), track: ui.RGBA(0, 0, 0, 0.09)}
}

// level is the health color as a fill; as text on a light card it is
// darkened to stay readable.
func (k floatingInk) level(h runway.Health, text bool) ui.Color {
	c := trayLevelColor(h, true)
	if text && !k.dark {
		return c.Mix(ui.RGB(0, 0, 0), 0.28)
	}
	return c
}

func floatingFont(size float32, weight int, family string) ui.Font {
	return ui.Font{Family: family, Size: size, Weight: weight, Features: "tnum"}
}

func floatingTextWidth(s string, f ui.Font) float32 {
	var w float32
	for _, g := range ui.Shape(s, f) {
		w = max(w, g.X+g.Advance)
	}
	return float32(math.Ceil(float64(w)))
}

// floatingText draws s with its vertical center at cy, from x, clipped to
// maxW when it is positive.
func floatingText(p *ui.Painter, s string, f ui.Font, x, cy, maxW float32, c ui.Color) {
	if s == "" {
		return
	}
	if maxW > 0 {
		for w := floatingTextWidth(s, f); w > maxW+0.01 && len([]rune(s)) > 1; w = floatingTextWidth(s, f) {
			r := []rune(strings.TrimSuffix(s, "…"))
			s = strings.TrimSpace(string(r[:len(r)-1])) + "…"
		}
	}
	m := f.Metrics()
	p.Glyphs(ui.Shape(s, f), x, cy-(m.Ascent+m.Descent)/2+m.Ascent, c)
}

func percentText(m runway.FloatingMeter) string {
	if m.ValueText != "" {
		return m.ValueText
	}
	return strconv.Itoa(m.RemainingPercent) + "%"
}

func floatingProviderIcon(p *ui.Painter, prov store.Provider, r ui.Rect, k floatingInk) {
	switch prov {
	case store.ProviderCursor:
		p.Icon(cursorProviderIcon, r, k.text)
	case store.ProviderCodex:
		p.Image(codexProviderIcon, r, ui.Contain)
	case store.ProviderClaude:
		p.Image(claudeProviderIcon, r, ui.Contain)
	case store.ProviderGrok:
		// The Grok mark is monochrome: it takes the text color.
		p.Icon(grokProviderIcon, r, k.text)
	}
}

// floatingCornerRadius is the window card's corner radius; zero where the
// system rounds the window itself (Windows 11) or cannot clip it (Windows 10).
func floatingCornerRadius(style store.FloatingStyle, h float32) float32 {
	if !floatingTransparent() {
		return 0
	}
	return floatingStyleRadius(style, h)
}

// floatingStyleRadius is the corner radius the style is designed with.
func floatingStyleRadius(style store.FloatingStyle, h float32) float32 {
	switch style {
	case store.FloatingStyleCapsule:
		return h / 2
	case store.FloatingStyleCard, store.FloatingStyleRings:
		return 14
	}
	return 12
}

// floatingSize is the window content size for the style and view.
func floatingSize(style store.FloatingStyle, v runway.FloatingView, family string) (float32, float32) {
	switch style {
	case store.FloatingStyleCard:
		return cardSize(v, family)
	case store.FloatingStyleRings:
		return ringsSize(v, family)
	case store.FloatingStyleBattery:
		return batterySize(v, family)
	}
	return capsuleSize(v, family)
}

// drawFloating paints the whole window: the card with corners of radius,
// then the style's content.
func drawFloating(p *ui.Painter, r ui.Rect, style store.FloatingStyle, v runway.FloatingView, k floatingInk, family string, radius float32) {
	p.Fill(r, k.bg, radius)
	if radius > 0 {
		p.Stroke(r, k.border, radius, 1)
	}
	switch style {
	case store.FloatingStyleCard:
		drawCard(p, r, v, k, family)
	case store.FloatingStyleRings:
		drawRings(p, r, v, k, family)
	case store.FloatingStyleBattery:
		drawBatteries(p, r, v, k, family)
	default:
		drawCapsule(p, r, v, k, family)
	}
}

// ---- Capsule: one line, the logo, the first window large, the second small.

const (
	capsuleH       = 32
	capsulePad     = 13
	capsuleGap     = 21 // between providers, with a hairline in the middle
	capsuleStatusW = 90
)

func capsuleParts(item runway.FloatingProvider, family string) (main, second string, mainW, secondW float32) {
	if len(item.Meters) == 0 {
		w := min(capsuleStatusW, floatingTextWidth(item.Status, floatingFont(12, 500, family)))
		return item.Status, "", w, 0
	}
	main = percentText(item.Meters[0])
	mainW = floatingTextWidth(main, floatingFont(14, 650, family))
	if len(item.Meters) > 1 {
		second = percentText(item.Meters[1])
		secondW = floatingTextWidth(second, floatingFont(11, 500, family))
	}
	return main, second, mainW, secondW
}

func capsuleItemWidth(item runway.FloatingProvider, family string) float32 {
	_, second, mw, sw := capsuleParts(item, family)
	w := 16 + 6 + mw
	if second != "" {
		w += 5 + sw
	}
	return w
}

func capsuleSize(v runway.FloatingView, family string) (float32, float32) {
	w := float32(2 * capsulePad)
	for i, item := range v.Providers {
		if i > 0 {
			w += capsuleGap
		}
		w += capsuleItemWidth(item, family)
	}
	return max(w, 64), capsuleH
}

func drawCapsule(p *ui.Painter, r ui.Rect, v runway.FloatingView, k floatingInk, family string) {
	x, cy := r.X+capsulePad, r.Y+r.H/2
	for i, item := range v.Providers {
		if i > 0 {
			p.Fill(ui.Rect{X: x + capsuleGap/2 - 0.5, Y: cy - 8, W: 1, H: 16}, k.track, 0)
			x += capsuleGap
		}
		floatingProviderIcon(p, item.Provider, ui.Rect{X: x, Y: cy - 8, W: 16, H: 16}, k)
		x += 22
		main, second, mw, sw := capsuleParts(item, family)
		if len(item.Meters) == 0 {
			floatingText(p, main, floatingFont(12, 500, family), x, cy, capsuleStatusW, k.muted)
			x += mw
			continue
		}
		floatingText(p, main, floatingFont(14, 650, family), x, cy, 0, k.level(item.Meters[0].Health, true))
		x += mw
		if second != "" {
			floatingText(p, second, floatingFont(11, 500, family), x+5, cy+1, 0, k.muted)
			x += 5 + sw
		}
	}
}

// ---- Card: a block per provider, a labeled bar per window.

const (
	cardW       = 232
	cardPad     = 12
	cardHeaderH = 22
	cardRowH    = 30
	cardGap     = 13 // between providers, with a hairline in the middle
)

func cardBlockHeight(item runway.FloatingProvider) float32 {
	return cardHeaderH + float32(max(1, len(item.Meters)))*cardRowH
}

func cardSize(v runway.FloatingView, _ string) (float32, float32) {
	h := float32(2*cardPad) - 4
	for i, item := range v.Providers {
		if i > 0 {
			h += cardGap
		}
		h += cardBlockHeight(item)
	}
	return cardW, h
}

func drawCard(p *ui.Painter, r ui.Rect, v runway.FloatingView, k floatingInk, family string) {
	x, w := r.X+cardPad, r.W-2*cardPad
	y := r.Y + cardPad - 2
	for i, item := range v.Providers {
		if i > 0 {
			p.Fill(ui.Rect{X: x, Y: y + cardGap/2 - 0.5, W: w, H: 1}, k.track, 0)
			y += cardGap
		}
		cy := y + cardHeaderH/2
		floatingProviderIcon(p, item.Provider, ui.Rect{X: x, Y: cy - 7, W: 14, H: 14}, k)
		floatingText(p, item.Name, floatingFont(12.5, 600, family), x+20, cy, w-20, k.text)
		y += cardHeaderH
		if len(item.Meters) == 0 {
			floatingText(p, item.Status, floatingFont(12, 500, family), x, y+cardRowH/2-2, w, k.muted)
			y += cardRowH
			continue
		}
		for _, m := range item.Meters {
			pct := percentText(m)
			pf := floatingFont(13, 650, family)
			pw := floatingTextWidth(pct, pf)
			label := m.Label
			if m.Reset != "" {
				label += " · " + m.Reset
			}
			ly := y + 8
			floatingText(p, label, floatingFont(11, 500, family), x, ly, w-pw-8, k.muted)
			floatingText(p, pct, pf, x+w-pw, ly, 0, k.level(m.Health, true))
			bar := ui.Rect{X: x, Y: y + 18, W: w, H: 5}
			p.Fill(bar, k.track, 2.5)
			if lv := float32(min(100, max(0, m.RemainingPercent))) / 100; lv > 0 {
				bar.W = max(5, bar.W*lv)
				p.Fill(bar, k.level(m.Health, false), 2.5)
			}
			y += cardRowH
		}
	}
}

// ---- Rings: concentric rings per provider, the first window outside.

const (
	ringsPad   = 10
	ringsCellW = 60
	ringsGap   = 6
	ringOuter  = 22
	ringInner  = 15
	ringsH     = 2*ringsPad + 2*ringOuter + 4 + 6 + 14
)

func ringsCaption(item runway.FloatingProvider) string {
	if len(item.Meters) == 0 {
		return item.Status
	}
	parts := make([]string, len(item.Meters))
	for i, m := range item.Meters {
		parts[i] = strconv.Itoa(m.RemainingPercent)
		if m.ValueText != "" {
			parts[i] = m.ValueText
		}
	}
	return strings.Join(parts, " · ")
}

func ringsSize(v runway.FloatingView, _ string) (float32, float32) {
	n := float32(max(1, len(v.Providers)))
	return 2*ringsPad + n*ringsCellW + (n-1)*ringsGap, ringsH
}

func drawRings(p *ui.Painter, r ui.Rect, v runway.FloatingView, k floatingInk, family string) {
	x := r.X + ringsPad
	for _, item := range v.Providers {
		cx, cy := x+ringsCellW/2, r.Y+ringsPad+3+ringOuter
		for i, radius := range []float32{ringOuter, ringInner} {
			width := float32(4.5)
			if i == 1 {
				width = 3.5
			}
			p.StrokePath((&ui.Path{}).Circle(cx, cy, radius), width, k.track)
			if i < len(item.Meters) {
				m := item.Meters[i]
				ringArc(p, cx, cy, radius, width, float64(min(100, max(0, m.RemainingPercent)))/100, k.level(m.Health, false))
			}
		}
		floatingProviderIcon(p, item.Provider, ui.Rect{X: cx - 7, Y: cy - 7, W: 14, H: 14}, k)
		caption := ringsCaption(item)
		f := floatingFont(11.5, 600, family)
		if len(item.Meters) == 0 {
			f.Weight = 500
		}
		cw := min(ringsCellW, floatingTextWidth(caption, f))
		col := k.text
		if len(item.Meters) == 0 {
			col = k.muted
		}
		floatingText(p, caption, f, cx-cw/2, r.Y+r.H-ringsPad-7, ringsCellW, col)
		x += ringsCellW + ringsGap
	}
}

// ringArc strokes the remaining fraction clockwise from the top.
func ringArc(p *ui.Painter, cx, cy, radius, width float32, level float64, c ui.Color) {
	if level <= 0 {
		return
	}
	path := &ui.Path{}
	steps := max(2, int(math.Ceil(level*72)))
	for i := 0; i <= steps; i++ {
		a := -math.Pi/2 + level*2*math.Pi*float64(i)/float64(steps)
		x, y := cx+radius*float32(math.Cos(a)), cy+radius*float32(math.Sin(a))
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	p.StrokePath(path, width, c)
}

// ---- Battery: a battery per window, the percentage inside.

const (
	batteryPad  = 12
	batteryRowH = 30
	batteryW    = 52
	batteryH    = 20
	batteryGap  = 8
)

func batterySize(v runway.FloatingView, family string) (float32, float32) {
	w := float32(0)
	for _, item := range v.Providers {
		row := float32(16 + 10)
		if len(item.Meters) == 0 {
			row += min(capsuleStatusW, floatingTextWidth(item.Status, floatingFont(12, 500, family)))
		} else {
			row += float32(len(item.Meters))*(batteryW+4) + float32(len(item.Meters)-1)*batteryGap
		}
		w = max(w, row)
	}
	n := float32(max(1, len(v.Providers)))
	return max(w+2*batteryPad, 80), n*batteryRowH + 2*batteryPad - 6
}

func drawBatteries(p *ui.Painter, r ui.Rect, v runway.FloatingView, k floatingInk, family string) {
	y := r.Y + batteryPad - 3
	for _, item := range v.Providers {
		cy := y + batteryRowH/2
		x := r.X + batteryPad
		floatingProviderIcon(p, item.Provider, ui.Rect{X: x, Y: cy - 8, W: 16, H: 16}, k)
		x += 26
		if len(item.Meters) == 0 {
			floatingText(p, item.Status, floatingFont(12, 500, family), x, cy, capsuleStatusW, k.muted)
		}
		for i, m := range item.Meters {
			if i > 0 {
				x += batteryGap
			}
			drawFloatingBattery(p, ui.Rect{X: x, Y: cy - batteryH/2, W: batteryW, H: batteryH}, m, k, family)
			x += batteryW + 4
		}
		y += batteryRowH
	}
}

func drawFloatingBattery(p *ui.Painter, body ui.Rect, m runway.FloatingMeter, k floatingInk, family string) {
	p.Stroke(body, k.muted, 5, 1.4)
	p.Fill(ui.Rect{X: body.X + body.W + 1.2, Y: body.Y + body.H*0.32, W: 2.4, H: body.H * 0.36}, k.muted, 1)
	track := ui.Rect{X: body.X + 2.5, Y: body.Y + 2.5, W: body.W - 5, H: body.H - 5}
	level := float32(min(100, max(0, m.RemainingPercent))) / 100
	filled := track
	filled.W = min(track.W, max(3, track.W*level))
	if level > 0 {
		p.Fill(filled, k.level(m.Health, false), 3)
	}
	text := percentText(m)
	f := floatingFont(11, 650, family)
	tw := floatingTextWidth(text, f)
	tx, cy := body.X+(body.W-tw)/2, body.Y+body.H/2
	// One color across the fill edge: the level colors are bright enough for
	// the dark text of the light card and the white text of the dark one.
	floatingText(p, text, f, tx, cy, 0, k.text)
}

// floatingTooltip spells out every window, as the drawing has no labels in
// the compact styles.
func floatingTooltip(v runway.FloatingView) string {
	lines := make([]string, 0, len(v.Providers))
	for _, item := range v.Providers {
		if len(item.Meters) == 0 {
			lines = append(lines, item.Name+": "+item.Status)
			continue
		}
		parts := make([]string, len(item.Meters))
		for i, m := range item.Meters {
			parts[i] = m.Label + " " + percentText(m)
			if m.Reset != "" {
				parts[i] += " (" + m.Reset + ")"
			}
		}
		lines = append(lines, item.Name+": "+strings.Join(parts, " · "))
	}
	return strings.Join(lines, "\n")
}
