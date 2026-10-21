package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestTrayPaletteIgnoresApplicationAppearance(t *testing.T) {
	want := []ui.Color{ui.RGB(48, 209, 88), ui.RGB(255, 159, 10), ui.RGB(255, 69, 58)}
	for _, light := range []bool{true, false} {
		for health, expected := range want {
			if got := trayLevelColor(runway.Health(health), light); got != expected {
				t.Errorf("app light=%v health=%d color=%v, want %v", light, health, got, expected)
			}
		}
	}
}

func TestTrayLayersPreserveGlyphsAndAccents(t *testing.T) {
	for _, style := range store.StatusBarStyles() {
		r := TrayFromStatus(trayTestStatus(style, nil))
		var boundsWidth int
		for _, options := range []trayImageOptions{{light: true, foreground: true}, {colorsOnly: true}} {
			data, err := renderTrayImage(r, options)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if boundsWidth != 0 && boundsWidth != img.Bounds().Dx() {
				t.Fatal("layers have different widths")
			}
			boundsWidth = img.Bounds().Dx()
			glyphs, accents, visible := 0, 0, 0
			for y := 0; y < img.Bounds().Dy(); y++ {
				for x := 0; x < img.Bounds().Dx(); x++ {
					pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					if pixel.A == 0 {
						continue
					}
					visible++
					if options.foreground {
						if pixel.R != 0 || pixel.G != 0 || pixel.B != 0 {
							t.Fatalf("%s foreground contains baked color %v", style, pixel)
						}
						glyphs++
					} else if trayPixelMatchesAccent(pixel) {
						accents++
					} else if style != store.StatusBarStyleBattery || !trayPixelMatchesBatteryLabel(pixel) {
						t.Fatalf("%s color layer contains foreground glyph/track %v at %d,%d", style, pixel, x, y)
					}
				}
			}
			if options.foreground && glyphs < 20 {
				t.Fatalf("%s lost its template glyphs", style)
			}
			if options.colorsOnly && (style == store.StatusBarStyleText || style == store.StatusBarStyleCountdown) && visible != 0 {
				t.Fatalf("%s retained stale color pixels", style)
			}
			if !options.foreground && style != store.StatusBarStyleText && style != store.StatusBarStyleCountdown && accents < 20 {
				t.Fatalf("%s lost colored accents", style)
			}
		}
	}
}

func trayPixelMatchesBatteryLabel(pixel color.NRGBA) bool {
	channels := []uint8{pixel.R, pixel.G, pixel.B}
	for _, health := range []runway.Health{runway.HealthGreen, runway.HealthYellow, runway.HealthRed} {
		base := trayLevelColor(health, true)
		values := []uint8{base.R, base.G, base.B}
		lowest := 0
		for i := range values {
			if values[i] < values[lowest] {
				lowest = i
			}
		}
		fraction := float32(int(channels[lowest])-int(values[lowest])) / float32(255-values[lowest])
		if fraction < 0 || fraction > 1 {
			continue
		}
		for i := range values {
			values[i] = uint8(float32(values[i]) + float32(255-values[i])*fraction + 0.5)
		}
		// A glyph over the antialiased end of the fill rounds twice.
		if trayPixelNear(pixel, ui.RGB(values[0], values[1], values[2]), 2) {
			return true
		}
	}
	return false
}

func trayPixelMatchesAccent(pixel color.NRGBA) bool {
	for _, health := range []runway.Health{runway.HealthGreen, runway.HealthYellow, runway.HealthRed} {
		if trayPixelMatches(pixel, trayLevelColor(health, true)) {
			return true
		}
	}
	return false
}

// Render uses 8-bit premultiplied RGBA, so A=1 can lose all RGB components
// Compare in that space rather than amplifying rounding by unpremultiplying
func trayPixelMatches(pixel color.NRGBA, expected ui.Color) bool {
	return trayPixelNear(pixel, expected, 1)
}

func trayPixelNear(pixel color.NRGBA, expected ui.Color, tolerance int) bool {
	got := []uint8{pixel.R, pixel.G, pixel.B}
	want := []uint8{expected.R, expected.G, expected.B}
	for i := range got {
		a := int(got[i]) * int(pixel.A) / 255
		b := int(want[i]) * int(pixel.A) / 255
		if delta := a - b; delta < -tolerance || delta > tolerance {
			return false
		}
	}
	return true
}

func TestTrayBatteryWhiteGlyphsStayInsideColorFill(t *testing.T) {
	cell := runway.StatusBarCell{Provider: store.ProviderCodex, Remaining: 35, Health: runway.HealthYellow, Percent: "80%"}
	// The color layer of one battery, with and without its percentage.
	battery := func(text string) image.Image {
		return ui.Render(func(c *ui.Context) {
			theme := ui.LightTheme()
			theme.Background = ui.Transparent
			c.SetTheme(theme)
			ui.Box(c).Fill().Draw(func(p *ui.Painter, _ ui.Rect) {
				trayPen{p: p, paint: trayPaint{ink: ui.Transparent, colorOpacity: 1}}.hBattery(ui.Rect{X: 4, Y: 4.5, W: 32, H: 13}, cell, text, 9)
			})
		}, 44, trayContentHeight, 2)
	}
	labels, fill := battery(cell.Percent), battery("")
	white := 0
	for y := 0; y < labels.Bounds().Dy(); y++ {
		for x := 0; x < labels.Bounds().Dx(); x++ {
			p := color.NRGBAModel.Convert(labels.At(x, y)).(color.NRGBA)
			if p.A > 8 && trayPixelMatches(p, ui.RGB(255, 255, 255)) {
				white++
				if _, _, _, a := fill.At(x, y).RGBA(); a == 0 {
					t.Fatalf("white label outside battery fill at %d,%d", x, y)
				}
			}
		}
	}
	if white < 5 {
		t.Fatal("battery lost clipped white labels")
	}
}
