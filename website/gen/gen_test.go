package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const repoRoot = "../.."

func TestLocalesAreComplete(t *testing.T) {
	locales, err := loadLocales(filepath.Join(repoRoot, "website", "locales"))
	if err != nil {
		t.Fatal(err)
	}
	if len(locales) != 7 {
		t.Fatalf("got %d locales, want 7", len(locales))
	}
}

func TestCheckShapeRejectsShortList(t *testing.T) {
	en, err := loadStrings(filepath.Join(repoRoot, "website", "locales", "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	short := *en
	short.Privacy.Items = short.Privacy.Items[1:]
	err = checkShape(reflect.ValueOf(*en), reflect.ValueOf(short), "short")
	if err == nil || !strings.Contains(err.Error(), "Privacy.Items") {
		t.Fatalf("got %v, want Privacy.Items length error", err)
	}
}

func TestReleaseFromGitHub(t *testing.T) {
	gh := ghRelease{TagName: "v1.1.0", PublishedAt: "2026-10-14T11:18:06Z"}
	for _, name := range []string{
		"Quosparr-macos-arm64.dmg", "Quosparr-macos-arm64.zip", "Quosparr-macos-x86_64.dmg", "Quosparr-macos-x86_64.zip",
		"Quosparr.Setup.1.1.0.amd64.exe", "Quosparr.Setup.1.1.0.arm64.exe",
		"quosparr_1.1.0_amd64.deb", "quosparr_1.1.0_arm64.deb",
		"quosparr-1.1.0-linux-amd64.tar.gz", "quosparr-1.1.0-linux-arm64.tar.gz",
		"quosparr-1.1.0-windows-amd64.tar.gz", "quosparr-1.1.0-windows-arm64.tar.gz",
		"quosparr-1.1.0-darwin-arm64.tar.gz", "install.sh", "SHA256SUMS", "update-linux-amd64.json",
	} {
		gh.Assets = append(gh.Assets, struct {
			Name string
			Size int64
		}{name, 9 << 20})
	}
	rel, err := releaseFromGitHub(gh)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Date != "2026-10-14" || rel.Version != "1.1.0" {
		t.Fatalf("got %+v", rel)
	}
	want := "https://github.com/Licoy/Quosparr/releases/download/v1.1.0/Quosparr.Setup.1.1.0.arm64.exe"
	if rel.DL.WinArm64.URL != want {
		t.Fatalf("WinArm64 = %s, want %s", rel.DL.WinArm64.URL, want)
	}
	want = "https://github.com/Licoy/Quosparr/releases/download/v1.1.0/quosparr-1.1.0-windows-arm64.tar.gz"
	if rel.DL.WinArm64Portable.URL != want {
		t.Fatalf("WinArm64Portable = %s, want %s", rel.DL.WinArm64Portable.URL, want)
	}
	if got := rel.DL.LinuxX64Tar.HumanSize(); got != "9.0 MB" {
		t.Fatalf("size = %s", got)
	}

	gh.Assets = gh.Assets[1:]
	if _, err := releaseFromGitHub(gh); err == nil {
		t.Fatal("missing macOS DMG accepted")
	}
}

func TestWebpSize(t *testing.T) {
	for name, want := range map[string][2]int{"accounts.webp": {2408, 1704}, "panel.webp": {778, 2138}, "panel-en.webp": {780, 2126}} {
		data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "images", name))
		if err != nil {
			t.Fatal(err)
		}
		w, h, err := webpSize(data)
		if err != nil || w != want[0] || h != want[1] {
			t.Fatalf("%s: got %dx%d %v, want %dx%d", name, w, h, err, want[0], want[1])
		}
	}
}

func TestBuildWritesEveryLanguage(t *testing.T) {
	out := t.TempDir()
	if err := build(repoRoot, out, ""); err != nil {
		t.Fatal(err)
	}
	sitemap, err := os.ReadFile(filepath.Join(out, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range localeTable {
		page, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(l.Path()), "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		html := string(page)
		for _, want := range []string{
			`<html lang="` + l.Lang + `">`,
			`<link rel="canonical" href="https://quosparr.com` + l.Path() + `">`,
			`hreflang="x-default"`,
			`application/ld+json`,
			`data-asset="mac-arm64"`,
			`data-asset="win-x64" data-name="Windows (x64)"`,
			`data-asset="win-x64-setup"`,
			`role="tablist"`,
			`id="app-codex" role="tabpanel"`,
			`id="app-gemini" role="tabpanel" aria-labelledby="app-tab-gemini" hidden`,
			`id="app-grok" role="tabpanel" aria-labelledby="app-tab-grok" hidden`,
			`<rect `,
		} {
			if !strings.Contains(html, want) {
				t.Errorf("%s: missing %s", l.Code, want)
			}
		}
		// Feature cards follow the app's provider order.
		at := 0
		for _, name := range []string{"Codex", "Claude", "Gemini", "Grok"} {
			i := strings.Index(html[at:], "</svg>"+name+"</h3>")
			if i < 0 {
				t.Errorf("%s: feature card %s missing or out of order", l.Code, name)
				break
			}
			at += i
		}
		og := "img/og-" + l.Code + ".png"
		if !strings.Contains(html, `content="https://quosparr.com/`+og+`"`) {
			t.Errorf("%s: missing og:image %s", l.Code, og)
		}
		if _, err := os.Stat(filepath.Join(out, og)); err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(sitemap), "<loc>https://quosparr.com"+l.Path()+"</loc>") {
			t.Errorf("sitemap misses %s", l.Path())
		}
	}
	for _, name := range []string{"404.html", "robots.txt", "CNAME", ".nojekyll", "img/shot-accounts.webp", "img/shot-mcp.webp"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Error(err)
		}
	}
}
