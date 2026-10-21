package app

import (
	"bytes"
	"fmt"
	"image/png"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// The menu bar content is a row of components, one per provider or two
// stacked, drawn in the Swift status item's 22 pt coordinate system.
const (
	trayContentHeight             = 22
	trayHorizontalPadding float32 = 4
	// trayComponentGap sets the components apart.
	trayComponentGap float32 = 10
	// trayIconSize is the logo of a component's provider, trayRowIconSize
	// the logo of a stacked row.
	trayIconSize    float32 = 13
	trayRowIconSize float32 = 9
)

// trayRowCenters are the vertical centers of one row, or of two stacked.
var trayRowCenters = [...][]float32{{11}, {6, 16}}

func trayRows(n int) []float32 { return trayRowCenters[min(2, max(1, n))-1] }

// TrayImage renders menu bar content or returns the desktop application logo.
// No window, account access or display server is needed.
func TrayImage(r TrayRender, light, compact bool) ([]byte, error) {
	if compact {
		return desktopTrayIconPNG, nil
	}
	return renderTrayImage(r, trayImageOptions{light: light})
}

type trayImageOptions struct {
	light                  bool
	foreground, colorsOnly bool
}

// trayPaint is one layer's colors: the ink of glyphs, tracks and logos, and
// how opaque the quota colors are. macOS draws the ink as a template image
// and the colors over it, each with the other left out.
type trayPaint struct {
	ink          ui.Color
	colorOpacity float32
}

func renderTrayImage(r TrayRender, options trayImageOptions) ([]byte, error) {
	if !r.Style.Valid() {
		return nil, fmt.Errorf("unsupported tray style %q", r.Style)
	}
	width := trayContentWidth(r)
	img := ui.Render(func(c *ui.Context) {
		theme := ui.LightTheme()
		theme.Background = ui.Transparent
		c.SetTheme(theme)
		ui.Box(c).Fill().Draw(func(p *ui.Painter, rect ui.Rect) {
			ink := ui.RGB(0, 0, 0)
			if !options.light {
				ink = ui.RGB(255, 255, 255)
			}
			paint := trayPaint{ink: ink, colorOpacity: 1}
			if options.foreground {
				paint.colorOpacity = 0
			}
			if options.colorsOnly {
				paint.ink = ui.Transparent
			}
			drawTrayContent(p, r, paint)
		})
	}, int(math.Ceil(float64(width))), trayContentHeight, 2)
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("encode tray: %w", err)
	}
	return out.Bytes(), nil
}

// trayContentWidth is the width the content takes, padding included.
func trayContentWidth(r TrayRender) float32 { return layoutTray(trayPen{}, r) }

// drawTrayContent draws the content from the origin.
func drawTrayContent(p *ui.Painter, r TrayRender, paint trayPaint) {
	layoutTray(trayPen{p: p, paint: paint}, r)
}

// layoutTray places every component and returns the total width; the same
// pass measures and draws, so the two cannot drift apart.
func layoutTray(d trayPen, r TrayRender) float32 {
	x := trayHorizontalPadding
	comps := r.Components
	if len(comps) == 0 {
		comps = []runway.StatusBarComponent{{Cells: []runway.StatusBarCell{{Empty: true, Percent: "--", Countdown: "--", MeterText: "--", BatteryText: "--"}}}}
	}
	for i, comp := range comps {
		if i > 0 {
			x += trayComponentGap
		}
		x += layoutTrayComponent(d, r, comp, x)
	}
	return x + trayHorizontalPadding
}

// layoutTrayComponent lays out one component from x and returns its width.
// A provider alone leads with its logo; stacked providers lead each row
// with theirs.
func layoutTrayComponent(d trayPen, r TrayRender, comp runway.StatusBarComponent, x float32) float32 {
	if len(comp.Cells) == 0 {
		return 0
	}
	start := x
	icons := !r.HideIcons
	if r.Style == store.StatusBarStyleRings {
		return trayRings(d, comp, x, icons)
	}
	if icons && comp.Provider != "" {
		d.icon(comp.Provider, x, 11, trayIconSize)
		x += trayIconSize + 4
	}
	// Rows are told apart by the logo where two providers stack, by the
	// window name where one provider shows two windows.
	rowIcons := icons && comp.Provider == ""
	switch r.Style {
	case store.StatusBarStyleMeters:
		x += trayMeters(d, comp.Cells, x, rowIcons)
	case store.StatusBarStyleCountdown:
		x += trayCountdown(d, comp.Cells, x, rowIcons)
	case store.StatusBarStyleBattery:
		if r.Battery == store.BatteryVertical {
			x += trayVerticalBatteries(d, comp.Cells, x, rowIcons)
		} else {
			x += trayHorizontalBatteries(d, comp.Cells, x, rowIcons)
		}
	case store.StatusBarStyleText:
		x += trayText(d, comp.Cells, x)
	}
	return x - start
}

// ---- Meters: a bar per window, the detail after it.

func trayMeters(d trayPen, cells []runway.StatusBarCell, x float32, rowIcons bool) float32 {
	if len(cells) == 1 {
		c := cells[0]
		d.bar(x, 11, 34, 6, c)
		return 34 + 5 + d.text(c.MeterText, x+39, 11, 11, 600, 1, 96)
	}
	lead := d.rowLead(cells, x, rowIcons)
	w := float32(0)
	for i, c := range cells[:2] {
		cy := trayRows(2)[i]
		rx := x + lead
		d.bar(rx, cy, 28, 4, c)
		rx += 28 + 4
		rx += d.text(c.MeterText, rx, cy, 8.5, 600, 1, 84)
		w = max(w, rx-x)
	}
	return w
}

// ---- Countdown: the time left until the reset.

func trayCountdown(d trayPen, cells []runway.StatusBarCell, x float32, rowIcons bool) float32 {
	if len(cells) == 1 {
		return d.text(cells[0].Countdown, x, 11, 12.5, 600, 1, 120)
	}
	lead := d.rowLead(cells, x, rowIcons)
	w := float32(0)
	for i, c := range cells[:2] {
		w = max(w, lead+d.text(c.Countdown, x+lead, trayRows(2)[i], 9, 600, 1, 96))
	}
	return w
}

// ---- Battery lying flat: one window is a battery with the percentage
// inside and the detail after it; stacked rows are too low for text
// inside, so the detail follows a small battery.

func trayHorizontalBatteries(d trayPen, cells []runway.StatusBarCell, x float32, rowIcons bool) float32 {
	if len(cells) == 1 {
		c := cells[0]
		w := max(30, d.measure(c.Percent, 9, 700)+12)
		d.hBattery(ui.Rect{X: x, Y: 4.5, W: w, H: 13}, c, c.Percent, 9)
		w += 2.5
		if c.BatteryText != c.Percent {
			w += 5 + d.text(c.BatteryText, x+w+5, 11, 11, 600, 1, 96)
		}
		return w
	}
	lead := d.rowLead(cells, x, rowIcons)
	w := float32(0)
	for i, c := range cells[:2] {
		cy := trayRows(2)[i]
		rx := x + lead
		d.hBattery(ui.Rect{X: rx, Y: cy - 4, W: 17, H: 8}, c, "", 0)
		rx += 17 + 2 + 4
		rx += d.text(c.BatteryText, rx, cy, 8.5, 600, 1, 84)
		w = max(w, rx-x)
	}
	return w
}

// ---- Battery standing: a slim battery per window, filling up, with the
// text beside them: the percentage over the countdown for one window, a
// row of the chosen detail per window otherwise.

func trayVerticalBatteries(d trayPen, cells []runway.StatusBarCell, x float32, rowIcons bool) float32 {
	start := x
	for i, c := range cells[:min(2, len(cells))] {
		if i > 0 {
			x += 2.5
		}
		d.vBattery(ui.Rect{X: x, Y: 4.5, W: 8.5, H: 15}, c)
		x += 8.5
	}
	x += 5
	if len(cells) == 1 {
		c := cells[0]
		if c.Countdown == c.Percent || c.Empty {
			return x - start + d.text(c.Percent, x, 11, 11, 650, 1, 96)
		}
		w := d.text(c.Percent, x, 6.5, 9, 700, 1, 96)
		w = max(w, d.text(c.Countdown, x, 16, 8, 500, 0.62, 96))
		return x - start + w
	}
	lead := d.rowLead(cells, x, rowIcons)
	w := float32(0)
	for i, c := range cells[:2] {
		w = max(w, lead+d.text(c.BatteryText, x+lead, trayRows(2)[i], 8.5, 600, 1, 84))
	}
	return x - start + w
}

// ---- Rings: a ring around the logo, or concentric rings after it, the
// first window outside; the percentages beside them.

func trayRings(d trayPen, comp runway.StatusBarComponent, x float32, icons bool) float32 {
	cells := comp.Cells
	start := x
	if len(cells) > 1 && icons {
		// Two rings leave no room inside for the logo.
		d.icon(cells[0].Provider, x, 11, trayIconSize)
		x += trayIconSize + 4
	}
	cx, cy := x+10.25, float32(11)
	if len(cells) == 1 {
		d.ring(cx, cy, 8.4, 2.3, cells[0])
		if icons {
			d.icon(cells[0].Provider, cx-4.75, cy, 9.5)
		}
	} else {
		d.ring(cx, cy, 9.1, 2, cells[0])
		d.ring(cx, cy, 5.4, 2, cells[1])
	}
	x += 20.5 + 4
	if len(cells) == 1 {
		return x - start + d.text(cells[0].Percent, x, 11, 11, 650, 1, 96)
	}
	w := float32(0)
	for i, c := range cells[:2] {
		w = max(w, d.text(c.Percent, x, trayRows(2)[i], 8.5, 600, 1, 84))
	}
	return x - start + w
}

// ---- Text: the main window large, the second small after it.

func trayText(d trayPen, cells []runway.StatusBarCell, x float32) float32 {
	w := d.text(cells[0].Percent, x, 11, 12.5, 650, 1, 96)
	if len(cells) > 1 {
		w += 3 + d.text(cells[1].Percent, x+w+3, 12, 9.5, 500, 0.6, 72)
	}
	return w
}

// trayPen draws in a layer, or only measures without a painter.
type trayPen struct {
	p     *ui.Painter
	paint trayPaint
}

func (d trayPen) draws() bool { return d.p != nil }

func (d trayPen) inks() bool { return d.p != nil && d.paint.ink.A > 0 }

func (d trayPen) measure(s string, size float32, weight int) float32 {
	return trayTextWidth(s, trayFontWeight(size, weight))
}

// text draws s centered on cy from x, at most maxW wide, and returns its
// width. alpha dims the ink of secondary text.
func (d trayPen) text(s string, x, cy, size float32, weight int, alpha, maxW float32) float32 {
	f := trayFontWeight(size, weight)
	s = trayFit(s, f, maxW)
	w := trayTextWidth(s, f)
	if d.inks() {
		m := f.Metrics()
		d.p.Glyphs(ui.Shape(s, f), x, cy-(m.Ascent+m.Descent)/2+m.Ascent, d.paint.ink.Alpha(alpha))
	}
	return w
}

// rowLead draws what tells two stacked rows apart, the providers' logos or
// the windows' names, and returns the width it takes.
func (d trayPen) rowLead(cells []runway.StatusBarCell, x float32, icons bool) float32 {
	if icons {
		for i, c := range cells[:min(2, len(cells))] {
			d.icon(c.Provider, x, trayRows(2)[i], trayRowIconSize)
		}
		return trayRowIconSize + 3.5
	}
	w := float32(0)
	for _, c := range cells {
		if c.Label != "" {
			w = max(w, d.measure(c.Label, 8, 500))
		}
	}
	if w == 0 {
		return 0
	}
	for i, c := range cells[:min(2, len(cells))] {
		d.text(c.Label, x, trayRows(2)[i], 8, 500, 0.62, 40)
	}
	return min(w, 40) + 4
}

func (d trayPen) icon(prov store.Provider, x, cy, size float32) {
	if !d.inks() {
		return
	}
	if svg := trayProviderIcon(prov); svg != nil {
		d.p.Icon(svg, ui.Rect{X: x, Y: cy - size/2, W: size, H: size}, d.paint.ink)
	}
}

// level is the cell's quota color, as this layer draws it, and the
// fraction left.
func (d trayPen) level(c runway.StatusBarCell) (ui.Color, float32) {
	if c.Empty {
		return ui.Transparent, 0
	}
	return trayLevelColor(c.Health, true).Alpha(d.paint.colorOpacity), float32(min(100, max(0, c.Remaining))) / 100
}

func (d trayPen) bar(x, cy, w, h float32, c runway.StatusBarCell) {
	if !d.draws() {
		return
	}
	r := ui.Rect{X: x, Y: cy - h/2, W: w, H: h}
	if d.inks() {
		d.p.Fill(r, d.paint.ink.Alpha(0.22), h/2)
	}
	if fill, level := d.level(c); level > 0 && fill.A > 0 {
		r.W = max(h, w*level)
		d.p.Fill(r, fill, h/2)
	}
}

func (d trayPen) ring(cx, cy, radius, width float32, c runway.StatusBarCell) {
	if !d.draws() {
		return
	}
	if d.inks() {
		d.p.StrokePath((&ui.Path{}).Circle(cx, cy, radius), width, d.paint.ink.Alpha(0.22))
	}
	if fill, level := d.level(c); level > 0 && fill.A > 0 {
		ringArc(d.p, cx, cy, radius, width, float64(level), fill)
	}
}

// hBattery is a battery lying in body, its cap at the right, with text
// inside when size is set. The text is ink over the empty part and white
// over the fill, which the color layer covers the ink with.
func (d trayPen) hBattery(body ui.Rect, c runway.StatusBarCell, text string, size float32) {
	if !d.draws() {
		return
	}
	stroke, radius, inset := float32(1.1), body.H*0.3, float32(1.9)
	if body.H < 10 {
		stroke, radius, inset = 1, 2.2, 1.6
	}
	if d.inks() {
		d.p.Stroke(body, d.paint.ink.Alpha(0.5), radius, stroke)
		d.p.Fill(ui.Rect{X: body.X + body.W + 0.9, Y: body.Y + body.H*0.32, W: 1.6, H: body.H * 0.36}, d.paint.ink.Alpha(0.5), 0.8)
	}
	track := inset2(body, inset)
	fill, level := d.level(c)
	filled := track
	filled.W = min(track.W, max(1.5, track.W*level))
	if level > 0 && fill.A > 0 {
		d.p.Fill(filled, fill, max(1, radius-inset))
	}
	if text == "" {
		return
	}
	f := trayFontWeight(size, 700)
	text = trayFit(text, f, track.W-2)
	w := trayTextWidth(text, f)
	m := f.Metrics()
	tx, base := track.X+(track.W-w)/2, body.Y+(body.H-m.Ascent-m.Descent)/2+m.Ascent
	if d.inks() {
		d.p.Glyphs(ui.Shape(text, f), tx, base, d.paint.ink)
	}
	if level > 0 && fill.A > 0 {
		d.p.Clip(filled, max(1, radius-inset), func() { d.p.Glyphs(ui.Shape(text, f), tx, base, ui.RGB(255, 255, 255)) })
	}
}

// vBattery is a battery standing in body, its cap on top, filling up.
func (d trayPen) vBattery(body ui.Rect, c runway.StatusBarCell) {
	if !d.draws() {
		return
	}
	if d.inks() {
		d.p.Stroke(body, d.paint.ink.Alpha(0.5), 2.4, 1.1)
		d.p.Fill(ui.Rect{X: body.X + body.W*0.3, Y: body.Y - 2.2, W: body.W * 0.4, H: 1.5}, d.paint.ink.Alpha(0.5), 0.7)
	}
	if fill, level := d.level(c); level > 0 && fill.A > 0 {
		track := inset2(body, 1.8)
		h := min(track.H, max(1.5, track.H*level))
		d.p.Fill(ui.Rect{X: track.X, Y: track.Y + track.H - h, W: track.W, H: h}, fill, 1)
	}
}

func inset2(r ui.Rect, d float32) ui.Rect {
	return ui.Rect{X: r.X + d, Y: r.Y + d, W: max(0, r.W-2*d), H: max(0, r.H-2*d)}
}

// trayProviderIcon is the monochrome logo; the menu bar tints it.
func trayProviderIcon(p store.Provider) *ui.SVG {
	switch p {
	case store.ProviderCodex:
		return codexMonoProviderIcon
	case store.ProviderClaude:
		return claudeMonoProviderIcon
	case store.ProviderGrok:
		return grokProviderIcon
	}
	return nil
}

func trayTextWidth(s string, font ui.Font) float32 {
	var width float32
	for _, g := range ui.Shape(s, font) {
		width = max(width, g.X+g.Advance)
	}
	return width
}

// trayFontWeight is the menu bar font: tabular digits keep a ticking
// countdown from shifting what follows it.
func trayFontWeight(size float32, weight int) ui.Font {
	return ui.Font{Size: size, Weight: weight, Features: "tnum"}
}

// trayFit shortens s with an ellipsis to fit maxW. A subpixel float32
// difference must not replace a fitting last glyph with the ellipsis.
func trayFit(s string, f ui.Font, maxW float32) string {
	if maxW <= 0 || trayTextWidth(s, f) <= maxW+0.01 {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		if t := strings.TrimSpace(string(runes)) + "…"; trayTextWidth(t, f) <= maxW+0.01 {
			return t
		}
	}
	return "…"
}

// Bright status accents stay consistent while AppKit tints template foregrounds
func trayLevelColor(health runway.Health, _ bool) ui.Color {
	return [...]ui.Color{ui.RGB(48, 209, 88), ui.RGB(255, 159, 10), ui.RGB(255, 69, 58)}[health]
}
