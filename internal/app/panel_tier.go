package app

import (
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
)

// tierLook is the metallic or plain look of a subscription capsule and of
// the identity text (Swift SubscriptionTierLook).
type tierLook struct {
	foreground ui.Color
	fill       []ui.Color
	stroke     ui.Color
	sheen      ui.Color
	// text is the readable gradient of the identity text.
	text []ui.Color
}

type rgb3 [3]float64

func col(v rgb3) ui.Color { return colA(v, 1) }

func colA(v rgb3, a float32) ui.Color {
	b := func(x float64) uint8 { return uint8(min(255, max(0, x*255+0.5))) }
	return ui.RGBA(b(v[0]), b(v[1]), b(v[2]), a)
}

func cols(list ...rgb3) []ui.Color {
	out := make([]ui.Color, len(list))
	for i, v := range list {
		out[i] = col(v)
	}
	return out
}

func pick[T any](light bool, a, b T) T {
	if light {
		return a
	}
	return b
}

func plainLook(t *ui.Theme) tierLook {
	ink := inkSecondary(t)
	light := !t.Dark
	return tierLook{
		foreground: ink,
		fill:       []ui.Color{pick(light, ui.RGBA(0, 0, 0, 0.10), ui.RGBA(255, 255, 255, 0.14))},
		stroke:     hairlineColor(t).Alpha(pick[float32](light, 0.9, 0.55) * 1.0),
		text:       []ui.Color{ink},
	}
}

func silverLook(light bool) tierLook {
	return tierLook{
		sheen:      ui.RGBA(255, 255, 255, pick[float32](light, 0.72, 0.55)),
		foreground: pick(light, col(rgb3{0.22, 0.24, 0.28}), col(rgb3{0.94, 0.95, 0.98})),
		fill: pick(light,
			cols(rgb3{0.78, 0.80, 0.84}, rgb3{0.94, 0.95, 0.97}, rgb3{0.70, 0.73, 0.78}),
			cols(rgb3{0.42, 0.45, 0.50}, rgb3{0.72, 0.74, 0.78}, rgb3{0.38, 0.40, 0.45})),
		stroke: pick(light, colA(rgb3{0.58, 0.61, 0.66}, 0.85), colA(rgb3{0.82, 0.84, 0.88}, 0.55)),
		text: pick(light,
			cols(rgb3{0.32, 0.34, 0.38}, rgb3{0.52, 0.55, 0.60}, rgb3{0.72, 0.74, 0.78}, rgb3{0.42, 0.44, 0.48}),
			cols(rgb3{0.78, 0.80, 0.84}, rgb3{0.94, 0.95, 0.97}, rgb3{1, 1, 1}, rgb3{0.82, 0.84, 0.88})),
	}
}

func goldLook(light bool) tierLook {
	return tierLook{
		sheen:      colA(rgb3{1, 0.96, 0.78}, pick[float32](light, 0.78, 0.62)),
		foreground: pick(light, col(rgb3{0.35, 0.22, 0.05}), col(rgb3{1.0, 0.92, 0.62})),
		fill: pick(light,
			cols(rgb3{0.82, 0.62, 0.18}, rgb3{0.98, 0.86, 0.42}, rgb3{0.72, 0.52, 0.12}),
			cols(rgb3{0.55, 0.40, 0.10}, rgb3{0.88, 0.70, 0.28}, rgb3{0.48, 0.34, 0.08})),
		stroke: pick(light, colA(rgb3{0.62, 0.44, 0.10}, 0.9), colA(rgb3{0.95, 0.78, 0.35}, 0.55)),
		text: pick(light,
			cols(rgb3{0.48, 0.32, 0.04}, rgb3{0.72, 0.52, 0.10}, rgb3{0.92, 0.72, 0.22}, rgb3{0.58, 0.40, 0.06}),
			cols(rgb3{0.86, 0.66, 0.22}, rgb3{1.0, 0.88, 0.45}, rgb3{1.0, 0.96, 0.72}, rgb3{0.92, 0.74, 0.30})),
	}
}

func blackGoldLook(light bool) tierLook {
	alpha := pick[float32](light, 0.92, 0.75)
	return tierLook{
		foreground: col(rgb3{0.93, 0.78, 0.38}),
		sheen:      colA(rgb3{1, 0.88, 0.48}, 0.70),
		fill: pick(light,
			cols(rgb3{0.12, 0.11, 0.10}, rgb3{0.26, 0.22, 0.16}, rgb3{0.10, 0.09, 0.08}),
			cols(rgb3{0.08, 0.07, 0.06}, rgb3{0.22, 0.18, 0.12}, rgb3{0.06, 0.05, 0.05})),
		stroke: colA(rgb3{0.78, 0.62, 0.24}, alpha),
		text: pick(light,
			cols(rgb3{0.42, 0.30, 0.06}, rgb3{0.72, 0.54, 0.12}, rgb3{0.95, 0.78, 0.32}, rgb3{0.55, 0.40, 0.08}),
			cols(rgb3{0.78, 0.60, 0.22}, rgb3{0.96, 0.80, 0.38}, rgb3{1.0, 0.92, 0.58}, rgb3{0.88, 0.70, 0.28})),
	}
}

func platinumLook(light bool) tierLook {
	return tierLook{
		foreground: col(rgb3{0.98, 0.94, 0.82}),
		sheen:      colA(rgb3{1, 0.96, 0.82}, 0.75),
		fill: pick(light,
			cols(rgb3{0.08, 0.08, 0.10}, rgb3{0.72, 0.68, 0.58}, rgb3{0.14, 0.13, 0.12}),
			cols(rgb3{0.05, 0.05, 0.07}, rgb3{0.62, 0.58, 0.50}, rgb3{0.10, 0.09, 0.08})),
		stroke: pick(light, colA(rgb3{0.93, 0.84, 0.62}, 0.95), colA(rgb3{0.96, 0.90, 0.72}, 0.85)),
		text: pick(light,
			cols(rgb3{0.32, 0.30, 0.24}, rgb3{0.52, 0.48, 0.36}, rgb3{0.74, 0.68, 0.50}, rgb3{0.40, 0.36, 0.26}),
			cols(rgb3{0.82, 0.78, 0.68}, rgb3{0.96, 0.93, 0.84}, rgb3{1.0, 0.98, 0.94}, rgb3{0.88, 0.82, 0.68})),
	}
}

func scale3(v rgb3, k float64) rgb3 { return rgb3{v[0] * k, v[1] * k, v[2] * k} }

// steelLook is the colored metal of the business and API tiers.
func steelLook(light bool, base, highlight, shadow, textLight, textDark rgb3) tierLook {
	mid := rgb3{(base[0] + highlight[0]) / 2, (base[1] + highlight[1]) / 2, (base[2] + highlight[2]) / 2}
	if light {
		return tierLook{
			foreground: col(textLight),
			sheen:      ui.RGBA(255, 255, 255, 0.55),
			fill:       cols(base, highlight, shadow),
			stroke:     colA(base, 0.85),
			text:       cols(textLight, scale3(base, 0.85), mid, textLight),
		}
	}
	return tierLook{
		foreground: col(textDark),
		sheen:      ui.RGBA(255, 255, 255, 0.42),
		fill:       cols(scale3(shadow, 0.85), scale3(mid, 0.9), scale3(shadow, 0.7)),
		stroke:     colA(highlight, 0.45),
		text:       cols(scale3(textDark, 0.85), mid, highlight, textDark),
	}
}

type steelSpec struct{ base, highlight, shadow, textLight, textDark rgb3 }

var (
	steelBusiness = steelSpec{rgb3{0.10, 0.55, 0.52}, rgb3{0.45, 0.82, 0.78}, rgb3{0.06, 0.38, 0.36}, rgb3{0.04, 0.28, 0.26}, rgb3{0.72, 0.96, 0.93}}
	steelTeam     = steelSpec{rgb3{0.28, 0.32, 0.72}, rgb3{0.58, 0.62, 0.95}, rgb3{0.18, 0.20, 0.52}, rgb3{0.12, 0.14, 0.40}, rgb3{0.82, 0.85, 1.0}}
	steelEnt      = steelSpec{rgb3{0.55, 0.14, 0.22}, rgb3{0.82, 0.38, 0.42}, rgb3{0.38, 0.08, 0.14}, rgb3{0.32, 0.06, 0.10}, rgb3{1.0, 0.78, 0.80}}
	steelEdu      = steelSpec{rgb3{0.18, 0.52, 0.30}, rgb3{0.48, 0.82, 0.55}, rgb3{0.10, 0.36, 0.20}, rgb3{0.08, 0.28, 0.14}, rgb3{0.72, 0.96, 0.78}}
	steelAPI      = steelSpec{rgb3{0.12, 0.52, 0.68}, rgb3{0.42, 0.82, 0.95}, rgb3{0.08, 0.36, 0.48}, rgb3{0.06, 0.28, 0.38}, rgb3{0.70, 0.94, 1.0}}
	steelLite     = steelSpec{rgb3{0.28, 0.52, 0.72}, rgb3{0.58, 0.78, 0.95}, rgb3{0.16, 0.36, 0.52}, rgb3{0.10, 0.28, 0.42}, rgb3{0.78, 0.92, 1.0}}
	steelXBasic   = steelSpec{rgb3{0.18, 0.42, 0.78}, rgb3{0.48, 0.68, 0.98}, rgb3{0.10, 0.28, 0.58}, rgb3{0.08, 0.22, 0.48}, rgb3{0.75, 0.88, 1.0}}
	steelXPremium = steelSpec{rgb3{0.42, 0.22, 0.72}, rgb3{0.72, 0.52, 0.95}, rgb3{0.28, 0.12, 0.52}, rgb3{0.24, 0.10, 0.42}, rgb3{0.90, 0.80, 1.0}}
	steelXPlus    = steelSpec{rgb3{0.62, 0.16, 0.42}, rgb3{0.92, 0.42, 0.68}, rgb3{0.42, 0.08, 0.28}, rgb3{0.38, 0.06, 0.22}, rgb3{1.0, 0.78, 0.90}}
	// steelCoral is the terracotta of the Claude Pro plan.
	steelCoral = steelSpec{rgb3{0.80, 0.42, 0.30}, rgb3{0.96, 0.68, 0.54}, rgb3{0.58, 0.26, 0.16}, rgb3{0.42, 0.16, 0.08}, rgb3{1.0, 0.86, 0.78}}
)

func (s steelSpec) look(light bool) tierLook {
	return steelLook(light, s.base, s.highlight, s.shadow, s.textLight, s.textDark)
}

// codexTierLook is SubscriptionTierLook.resolve for a Codex tier.
func codexTierLook(t *ui.Theme, tier codex.Tier) tierLook {
	light := !t.Dark
	switch tier {
	case codex.TierPlus:
		return silverLook(light)
	case codex.TierPro100:
		return goldLook(light)
	case codex.TierPro200:
		return blackGoldLook(light)
	case codex.TierPro500:
		return platinumLook(light)
	case codex.TierBusiness:
		return steelBusiness.look(light)
	case codex.TierBusinessPremium:
		return goldLook(light)
	case codex.TierTeam:
		return steelTeam.look(light)
	case codex.TierEnterprise:
		return steelEnt.look(light)
	case codex.TierEdu:
		return steelEdu.look(light)
	case codex.TierAPI:
		return steelAPI.look(light)
	}
	return plainLook(t)
}

// grokTierLook is SubscriptionTierLook.resolve for a Grok tier.
func grokTierLook(t *ui.Theme, tier grok.TierKind) tierLook {
	light := !t.Dark
	switch tier {
	case grok.TierSuperGrokLite, grok.TierXSubscriptionPlus:
		return steelLite.look(light)
	case grok.TierSuperGrok, grok.TierXSubscriptionPremium:
		return silverLook(light)
	case grok.TierSuperGrokPlus, grok.TierXSubscriptionSuper:
		return goldLook(light)
	case grok.TierSuperGrokHeavy, grok.TierXSubscriptionUltra:
		return blackGoldLook(light)
	case grok.TierXBasic:
		return steelXBasic.look(light)
	case grok.TierXPremium:
		return steelXPremium.look(light)
	case grok.TierXPremiumPlus:
		return steelXPlus.look(light)
	case grok.TierAPIKey:
		return steelAPI.look(light)
	}
	return plainLook(t)
}

// claudeTierLook follows the other providers: Max 5x is gold like Pro 100,
// Max 20x black gold like Pro 200, and Pro has the Claude terracotta.
func claudeTierLook(t *ui.Theme, tier claude.TierKind) tierLook {
	light := !t.Dark
	switch tier {
	case claude.TierPro:
		return steelCoral.look(light)
	case claude.TierMax, claude.TierMax5x:
		return goldLook(light)
	case claude.TierMax20x:
		return blackGoldLook(light)
	case claude.TierTeam:
		return steelTeam.look(light)
	case claude.TierEnterprise:
		return steelEnt.look(light)
	}
	return plainLook(t)
}

// fillStops paints a 2- or 3-stop left-to-right gradient into r.
func fillStops(p *ui.Painter, r ui.Rect, stops []ui.Color, radius float32) {
	switch len(stops) {
	case 0:
	case 1:
		p.Fill(r, stops[0], radius)
	case 2:
		p.FillGradient(r, ui.LinearGradient{From: stops[0], To: stops[1], Angle: 100}, radius)
	default:
		// Clip only the gradient halves; each fill owns its rounded shape.
		// The GPU renderer retains only the innermost rounded clip.
		half := r.W / 2
		p.Clip(ui.Rect{X: r.X, Y: r.Y, W: half, H: r.H}, 0, func() {
			p.FillGradient(r, ui.LinearGradient{From: stops[0], To: stops[1], Angle: 100, End: 0.5}, radius)
		})
		p.Clip(ui.Rect{X: r.X + half, Y: r.Y, W: r.W - half, H: r.H}, 0, func() {
			p.FillGradient(r, ui.LinearGradient{From: stops[1], To: stops[2], Angle: 100, Start: 0.5, End: 1}, radius)
		})
	}
}

// tierBadge is SubscriptionTierBadge: a capsule with the label at 10pt
// semibold, 7pt by 3pt padding.
func tierBadge(c *ui.Context, look tierLook, label string) { tierBadgePadded(c, look, label, 3, 7) }

// tierBadgePadded is tierBadge with its own padding; the account cards use
// the tighter 1pt by 5pt of SubscriptionTierTag.
func tierBadgePadded(c *ui.Context, look tierLook, label string, vertical, horizontal float32) ui.Element {
	reduceMotion := c.Preferences().ReduceMotion
	return ui.Row(c).Shrink(0).Padding(vertical, horizontal).Radius(999).AlignItems(ui.Center).Label(label).Draw(func(p *ui.Painter, r ui.Rect) {
		fillStops(p, r, look.fill, r.H/2)
		if look.sheen.A > 0 && !reduceMotion {
			drawTierSheen(p, r, look.sheen)
		}
	}).DrawOver(func(p *ui.Painter, r ui.Rect) {
		p.Stroke(r, look.stroke, r.H/2, 1)
	}).Children(func() {
		ui.Text(c, label).FontSize(fsCaption).FontWeight(600).TextColor(look.foreground).SingleLine().MinWidth(0).Shrink(1).Ellipsis("…")
	})
}

// gradientColor samples the stops at fraction f in [0, 1].
func gradientColor(stops []ui.Color, f float32) ui.Color {
	if len(stops) == 1 {
		return stops[0]
	}
	pos := f * float32(len(stops)-1)
	i := min(len(stops)-2, max(0, int(pos)))
	return stops[i].Mix(stops[i+1], pos-float32(i))
}

// gradientSpans colors a text rune by rune along the gradient.
func gradientSpans(text string, stops []ui.Color, size float32, weight int) []ui.Span {
	if len(stops) == 1 || text == "" {
		return []ui.Span{{Text: text, Size: size, Weight: weight, Color: stops[0]}}
	}
	n := utf8.RuneCountInString(text)
	spans := make([]ui.Span, 0, n)
	i := 0
	for _, r := range text {
		f := float32(i) / float32(max(1, n-1))
		spans = append(spans, ui.Span{Text: string(r), Size: size, Weight: weight, Color: gradientColor(stops, f)})
		i++
	}
	return spans
}

// tierText is the identity text in the metallic gradient of its tier.
func tierText(c *ui.Context, look tierLook, text string) ui.Element {
	return ui.RichText(c, gradientSpans(text, look.text, fsCallout, 600)...).SingleLine().Ellipsis("…").Label(text)
}
