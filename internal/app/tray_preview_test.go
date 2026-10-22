package app

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	xdraw "golang.org/x/image/draw"
)

func TestTrayStylePreview(t *testing.T) {
	dir := os.Getenv("RUNWAY_TRAY_QA_DIR")
	if dir == "" {
		t.Skip("set RUNWAY_TRAY_QA_DIR to export tray previews")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, lang := range l10n.Languages() {
		writeTrayPreview(t, dir, lang)
	}
}

// trayPreviewVariant is one row of the preview sheet.
type trayPreviewVariant struct {
	name      string
	style     store.StatusBarStyle
	customize func(*store.Preferences)
}

func trayPreviewVariants() []trayPreviewVariant {
	codex, claude, grok := store.ProviderCodex, store.ProviderClaude, store.ProviderGrok
	apart := trayGroups([]store.Provider{codex}, []store.Provider{claude}, []store.Provider{grok})
	paired := trayGroups([]store.Provider{codex, claude}, []store.Provider{grok})
	vertical := func(p *store.Preferences) { p.StatusBar.Battery = store.BatteryVertical }
	var out []trayPreviewVariant
	for _, style := range store.StatusBarStyles() {
		single := func(p *store.Preferences) { apart(p); p.StatusBar.Window = store.StatusBarWindowFiveHour }
		out = append(out, trayPreviewVariant{string(style), style, nil}, trayPreviewVariant{string(style) + " ×3", style, apart},
			trayPreviewVariant{string(style) + " 5h ×3", style, single})
		if style.Combines() {
			out = append(out, trayPreviewVariant{string(style) + " 2+1", style, paired})
		}
		if style == store.StatusBarStyleRings {
			values := func(p *store.Preferences) { p.StatusBar.RingValues = true }
			out = append(out,
				trayPreviewVariant{"rings values", style, values},
				trayPreviewVariant{"values ×3", style, func(p *store.Preferences) { apart(p); values(p) }},
				trayPreviewVariant{"values 5h ×3", style, func(p *store.Preferences) { single(p); values(p) }},
				trayPreviewVariant{"values no icons", style, func(p *store.Preferences) { values(p); p.StatusBar.HideIcons = true }})
		}
		if style == store.StatusBarStyleBattery {
			out = append(out,
				trayPreviewVariant{"battery vertical", style, vertical},
				trayPreviewVariant{"vertical ×3", style, func(p *store.Preferences) { apart(p); vertical(p) }},
				trayPreviewVariant{"vertical 2+1", style, func(p *store.Preferences) { paired(p); vertical(p) }},
				trayPreviewVariant{"vertical 5h ×3", style, func(p *store.Preferences) { apart(p); vertical(p); p.StatusBar.Window = store.StatusBarWindowFiveHour }},
				trayPreviewVariant{"battery 5h % ×3", style, func(p *store.Preferences) {
					apart(p)
					p.StatusBar.Window = store.StatusBarWindowFiveHour
					p.StatusBarBatteryDetailStyle = store.BatteryDetailRemainingPercent
				}})
		}
	}
	return out
}

func previewTrayStatus(v trayPreviewVariant, lang l10n.Language) TrayRender {
	prefs := store.DefaultPreferences()
	prefs.StatusBarDisplayStyle = v.style
	if v.customize != nil {
		v.customize(&prefs)
	}
	return TrayFromStatus(runway.BuildStatusBar(l10n.New(lang), trayTestInputs(prefs)))
}

func writeTrayPreview(t *testing.T, dir string, lang l10n.Language) {
	t.Helper()
	variants := trayPreviewVariants()
	// Reserve the measured full content width plus a separate square icon
	columnWidth := 0
	for _, v := range variants {
		img := decodeTrayPreview(t, previewTrayStatus(v, lang), true, false)
		columnWidth = max(columnWidth, img.Bounds().Dx()+90)
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, 170+2*columnWidth, 44+len(variants)*64))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.NRGBA{240, 240, 240, 255}}, image.Point{}, draw.Src)
	label := ui.Render(func(c *ui.Context) { ui.Text(c, lang.NativeTitle()).FontSize(15) }, 150, 30, 1)
	draw.Draw(canvas, image.Rect(0, 0, 150, 30), label, image.Point{}, draw.Over)
	for i, v := range variants {
		r := previewTrayStatus(v, lang)
		y := 44 + i*64
		label = ui.Render(func(c *ui.Context) { ui.Text(c, v.name).FontSize(14) }, 150, 30, 1)
		draw.Draw(canvas, image.Rect(12, y+8, 162, y+38), label, image.Point{}, draw.Over)
		for j, light := range []bool{true, false} {
			x := 170 + j*columnWidth
			bg := color.NRGBA{236, 236, 238, 255}
			if !light {
				bg = color.NRGBA{32, 32, 35, 255}
			}
			draw.Draw(canvas, image.Rect(x-8, y-6, x+columnWidth-8, y+52), &image.Uniform{C: bg}, image.Point{}, draw.Src)
			drawTrayLayers(canvas, r, light, image.Pt(x, y))
			icon := decodeTrayPreview(t, r, light, true)
			xdraw.CatmullRom.Scale(canvas, image.Rect(x+columnWidth-64, y, x+columnWidth-20, y+44), icon, icon.Bounds(), draw.Over, nil)
		}
	}
	if lang == l10n.English {
		writeTrayZoom(t, dir, variants)
	}
	name := "tray-styles-" + string(lang) + ".png"
	if lang == l10n.English {
		name = "tray-styles.png"
	}
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, canvas); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// writeTrayZoom enlarges every variant on a light bar, to check the details.
func writeTrayZoom(t *testing.T, dir string, variants []trayPreviewVariant) {
	const zoom = 2
	width := 0
	for _, v := range variants {
		width = max(width, decodeTrayPreview(t, previewTrayStatus(v, l10n.English), true, false).Bounds().Dx())
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, width*zoom+16, len(variants)*(44*zoom+8)))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.NRGBA{236, 236, 238, 255}}, image.Point{}, draw.Src)
	for i, v := range variants {
		r := previewTrayStatus(v, l10n.English)
		row := image.NewNRGBA(image.Rect(0, 0, width, 44))
		draw.Draw(row, row.Bounds(), &image.Uniform{C: color.NRGBA{236, 236, 238, 255}}, image.Point{}, draw.Src)
		drawTrayLayers(row, r, true, image.Point{})
		y := i * (44*zoom + 8)
		xdraw.NearestNeighbor.Scale(canvas, image.Rect(8, y+4, 8+width*zoom, y+4+44*zoom), row, row.Bounds(), draw.Src, nil)
	}
	file, err := os.Create(filepath.Join(dir, "tray-zoom.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, canvas); err != nil {
		t.Fatal(err)
	}
}

// drawTrayLayers composes the two macOS layers as the menu bar does: the
// template tinted for the bar, the colors over it.
func drawTrayLayers(canvas draw.Image, r TrayRender, light bool, at image.Point) {
	ink := color.NRGBA{A: 255}
	if !light {
		ink = color.NRGBA{255, 255, 255, 255}
	}
	var layers [2]image.Image
	for i, options := range []trayImageOptions{{light: true, foreground: true}, {colorsOnly: true}} {
		data, err := renderTrayImage(r, options)
		if err != nil {
			panic(err)
		}
		layers[i], _ = png.Decode(bytes.NewReader(data))
	}
	target := layers[0].Bounds().Add(at)
	draw.DrawMask(canvas, target, &image.Uniform{C: ink}, image.Point{}, layers[0], image.Point{}, draw.Over)
	draw.Draw(canvas, target, layers[1], image.Point{}, draw.Over)
}

func decodeTrayPreview(t *testing.T, r TrayRender, light, compact bool) image.Image {
	t.Helper()
	data, err := TrayImage(r, light, compact)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestTrayLayerPreview(t *testing.T) {
	dir := os.Getenv("RUNWAY_TRAY_QA_DIR")
	if dir == "" {
		t.Skip("set RUNWAY_TRAY_QA_DIR to export native layer previews")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	styles := store.StatusBarStyles()
	width := 0
	for _, style := range styles {
		width = max(width, decodeTrayPreview(t, previewTrayStatus(trayPreviewVariant{style: style}, l10n.English), true, false).Bounds().Dx()+24)
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, 150+3*width, 390))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.NRGBA{240, 240, 240, 255}}, image.Point{}, draw.Src)
	for i, style := range styles {
		r := previewTrayStatus(trayPreviewVariant{style: style}, l10n.English)
		layers := make([]image.Image, 2)
		for layer, options := range []trayImageOptions{{light: true, foreground: true}, {colorsOnly: true}} {
			data, err := renderTrayImage(r, options)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			layers[layer] = img
			suffix := "foreground"
			if layer == 1 {
				suffix = "colors"
			}
			if err := os.WriteFile(filepath.Join(dir, string(style)+"-"+suffix+".png"), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
		y := 30 + i*70
		label := ui.Render(func(c *ui.Context) { ui.Text(c, string(style)).FontSize(15) }, 140, 30, 1)
		draw.Draw(canvas, image.Rect(5, y, 145, y+30), label, image.Point{}, draw.Over)
		for col, bg := range []color.NRGBA{{255, 255, 255, 255}, {32, 32, 35, 255}, {79, 138, 178, 255}} {
			x := 150 + col*width
			rect := image.Rect(x, y, x+width-5, y+52)
			draw.Draw(canvas, rect, &image.Uniform{C: bg}, image.Point{}, draw.Src)
			ink := color.NRGBA{A: 255}
			if col != 0 {
				ink = color.NRGBA{255, 255, 255, 255}
			}
			target := layers[0].Bounds().Add(image.Pt(x+4, y+4))
			draw.DrawMask(canvas, target, &image.Uniform{C: ink}, image.Point{}, layers[0], image.Point{}, draw.Over)
			draw.Draw(canvas, target, layers[1], image.Point{}, draw.Over)
		}
	}
	file, err := os.Create(filepath.Join(dir, "tray-native-layer-matrix.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, canvas); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
