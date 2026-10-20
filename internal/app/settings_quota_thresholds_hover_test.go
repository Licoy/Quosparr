package app

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func quotaTagHoverPoints(t *testing.T, tt *ui.Tester, u *settingsUI) (ui.Rect, float32, float32) {
	t.Helper()
	r, ok := tt.Find(tx(u.L(), xQuotaThresholdRemove, 80))
	if !ok {
		t.Fatal("missing 80% tag")
	}
	return r, r.X + r.W - 16, r.Y + r.H/2
}

func exportQuotaHover(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	if dir := os.Getenv("RUNWAY_QUOTA_UI_QA"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		exportReviewPNG(t, tt, filepath.Join(dir, "hover-tags-"+name+".png"))
	}
}

func TestQuotaThresholdTagAndCloseHover(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint(dark), func(t *testing.T) {
			e := newEnv(t, nil)
			u, tt := openSettingsView(t, e, tabNotifications)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			e.s.SetQuotaAlerts(true)
			e.settle()
			tt.SetDark(dark)
			r, cx, cy := quotaTagHoverPoints(t, tt, u)
			fill := func() color.RGBA { return tt.Image().RGBAAt(int(r.X+8), int(cy)) }
			closeFill := func() color.RGBA { return tt.Image().RGBAAt(int(cx+6), int(cy)) }
			baseline := fill()
			before, writes := e.s.Prefs(), e.mem.Writes()
			exportQuotaHover(t, tt, fmt.Sprintf("dark-%t-normal", dark))
			tt.Move(r.X+15, cy)
			bodyHover, bodyClose := fill(), closeFill()
			if bodyHover == baseline || tt.Cursor() != ui.CursorPointer {
				t.Fatal("tag hover has no fill or pointer feedback")
			}
			exportQuotaHover(t, tt, fmt.Sprintf("dark-%t-tag-hover", dark))
			tt.Move(cx, cy)
			if fill() != bodyHover || closeFill() == bodyClose {
				t.Fatal("close hover did not highlight its own region independently")
			}
			exportQuotaHover(t, tt, fmt.Sprintf("dark-%t-close-hover", dark))
			closeHover := closeFill()
			tt.Press(cx, cy)
			if fill() == bodyHover || closeFill() == closeHover {
				t.Fatal("tag or close is missing pressed feedback")
			}
			exportQuotaHover(t, tt, fmt.Sprintf("dark-%t-close-pressed", dark))
			if e.mem.Writes() != writes || !reflect.DeepEqual(e.s.Prefs(), before) {
				t.Fatal("hover or press mutated preferences")
			}
			// The non-interactive close region must leave the single parent
			// click responsible for exactly one immediate deletion and write.
			tt.Release(cx, cy)
			e.settle()
			if e.mem.Writes() != writes+1 || !reflect.DeepEqual(e.stored().QuotaAlertThresholds, []int{95, 100}) {
				t.Fatal("close release did not delete exactly once")
			}
		})
	}
}

func TestQuotaThresholdDisabledTagHasNoHoverFeedback(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("readonly-%t/dark-%t", readOnly, dark), func(t *testing.T) {
				e := newEnv(t, nil)
				u, tt := openSettingsView(t, e, tabNotifications)
				tt.SetPreferences(ui.Preferences{ReduceMotion: true})
				e.s.SetQuotaAlerts(true)
				if readOnly {
					e.s.loadErr = errors.New("unreadable preferences")
				} else if err := e.s.SetQuotaAlertThresholds([]int{80}); err != nil {
					t.Fatal(err)
				}
				e.settle()
				tt.SetDark(dark)
				r, cx, cy := quotaTagHoverPoints(t, tt, u)
				baseline := bytes.Clone(tt.Image().Pix)
				before, writes := e.s.Prefs(), e.mem.Writes()
				for _, x := range []float32{r.X + 15, cx} {
					tt.Move(x, cy)
					if tt.Cursor() != ui.CursorDefault || !bytes.Equal(baseline, tt.Image().Pix) {
						t.Fatal("disabled tag changes on hover or shows pointer cursor")
					}
					tt.Press(x, cy)
					if !bytes.Equal(baseline, tt.Image().Pix) {
						t.Fatal("disabled tag changes when pressed")
					}
					tt.Release(x, cy)
				}
				e.settle()
				if e.mem.Writes() != writes || !reflect.DeepEqual(before, e.s.Prefs()) {
					t.Fatal("disabled tag changed preferences")
				}
				exportQuotaHover(t, tt, fmt.Sprintf("readonly-%t-dark-%t-disabled", readOnly, dark))
			})
		}
	}
}
