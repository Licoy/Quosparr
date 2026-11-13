package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Image struct {
	Src  string
	W, H int
	Alt  string
}

type pageData struct {
	Locale     Locale
	T          *Strings
	Locales    []Locale
	Base       string
	URL        string
	Repo       string
	Release    Release
	CSS, JS    string
	Panel      Panel
	Wide       Image
	Gallery    []Image
	Features   []string
	Privacy    []string
	JSONLD     template.JS
	Year       int
	ReadmeURL  string
	UpgradeURL string
}

// Remix Icon names paired by index with Features.Common and Privacy.Items.
var (
	featureIcons = []string{"dashboard-3-line", "picture-in-picture-line", "checkbox-multiple-line", "coins-line", "line-chart-line", "magic-line", "plug-line", "chat-history-line", "palette-line", "font-size", "refresh-line", "global-line"}
	privacyIcons = []string{"folder-shield-2-line", "key-2-line", "eye-off-line", "database-2-line", "history-line"}
)

func render(root, out string, locales []Locale, rel Release) error {
	site := filepath.Join(root, "website")
	now := panelNow(rel)
	if n := len(locales[0].T.Features.Common); n != len(featureIcons) {
		return fmt.Errorf("features.common has %d items, %d icons", n, len(featureIcons))
	}
	if n := len(locales[0].T.Privacy.Items); n != len(privacyIcons) {
		return fmt.Errorf("privacy.items has %d items, %d icons", n, len(privacyIcons))
	}
	if err := copyDir(filepath.Join(site, "static", "img"), filepath.Join(out, "img")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(site, "static", "favicon.ico"), filepath.Join(out, "favicon.ico")); err != nil {
		return err
	}
	shots, err := copyShots(filepath.Join(root, "docs", "images"), filepath.Join(out, "img"))
	if err != nil {
		return err
	}
	css, err := hashedAsset(filepath.Join(site, "static", "site.css"), out)
	if err != nil {
		return err
	}
	js, err := hashedAsset(filepath.Join(site, "static", "site.js"), out)
	if err != nil {
		return err
	}
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"icon":     iconLoader(filepath.Join(site, "icons")),
		"provider": iconLoader(filepath.Join(root, "internal", "app", "assets", "providers")),
		"logo":     colorIconLoader(filepath.Join(root, "internal", "app", "assets", "providers")),
		"html":     func(s string) template.HTML { return template.HTML(s) },
		"dict":     dict,
	}).ParseGlob(filepath.Join(site, "templates", "*.html"))
	if err != nil {
		return err
	}
	for _, l := range locales {
		page := pageData{
			Locale: l, T: l.T, Locales: locales, Base: siteURL, URL: siteURL + l.Path(), Repo: repoURL,
			Release: rel, CSS: css, JS: js, Year: time.Now().UTC().Year(),
			Features: featureIcons, Privacy: privacyIcons,
			ReadmeURL:  repoURL + "/blob/main/" + l.Readme,
			UpgradeURL: repoURL + "/blob/main/" + l.Upgrade,
		}
		page.Panel = buildPanel(l, now)
		sh := l.T.Shots
		page.Wide = shots["accounts"].with(sh.Accounts)
		page.Gallery = []Image{
			shots["menubar"].with(sh.MenuBar), shots["floating"].with(sh.Floating), shots["pricing"].with(sh.Pricing),
			shots["sessions"].with(sh.Sessions), shots["skills"].with(sh.Skills), shots["mcp"].with(sh.MCP),
		}
		page.JSONLD, err = jsonLD(page)
		if err != nil {
			return err
		}
		if err := renderFile(tmpl, "index.html", filepath.Join(out, filepath.FromSlash(l.Path()), "index.html"), page); err != nil {
			return err
		}
	}
	notFound := pageData{Locale: locales[0], T: locales[0].T, Locales: locales, Base: siteURL, URL: siteURL + "/", Repo: repoURL, CSS: css, JS: js, Year: time.Now().UTC().Year()}
	if err := renderFile(tmpl, "404.html", filepath.Join(out, "404.html"), notFound); err != nil {
		return err
	}
	files := map[string]string{
		"sitemap.xml":      sitemap(locales, rel.Date),
		"robots.txt":       "User-agent: *\nAllow: /\n\nSitemap: " + siteURL + "/sitemap.xml\n",
		"CNAME":            strings.TrimPrefix(siteURL, "https://") + "\n",
		".nojekyll":        "",
		"site.webmanifest": manifest(),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(out, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (img Image) with(alt string) Image {
	img.Alt = alt
	return img
}

func copyShots(src, dst string) (map[string]Image, error) {
	shots := map[string]Image{}
	for _, n := range []string{"accounts", "menubar", "floating", "pricing", "sessions", "skills", "mcp"} {
		data, err := os.ReadFile(filepath.Join(src, n+".webp"))
		if err != nil {
			return nil, err
		}
		w, h, err := webpSize(data)
		if err != nil {
			return nil, fmt.Errorf("docs/images/%s.webp: %w", n, err)
		}
		name := "shot-" + n + ".webp"
		if err := os.WriteFile(filepath.Join(dst, name), data, 0o644); err != nil {
			return nil, err
		}
		shots[n] = Image{Src: "/img/" + name, W: w, H: h}
	}
	return shots, nil
}

// hashedAsset copies site.css / site.js to /assets/ under a content hash so
// browsers can cache them across deploys.
func hashedAsset(src, out string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	ext := filepath.Ext(src)
	name := strings.TrimSuffix(filepath.Base(src), ext) + "." + hex.EncodeToString(sum[:5]) + ext
	if err := os.MkdirAll(filepath.Join(out, "assets"), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(out, "assets", name), data, 0o644); err != nil {
		return "", err
	}
	return "/assets/" + name, nil
}

var (
	svgRoot  = regexp.MustCompile(`(?s)<svg\b([^>]*)>(.*)</svg>`)
	svgTitle = regexp.MustCompile(`(?s)<title>.*?</title>`)
	svgAttr  = regexp.MustCompile(`\b(viewBox|fill-rule)="([^"]*)"`)
)

// iconLoader inlines SVGs as currentColor glyphs, so icons follow the text
// color in both themes without an icon font.
func iconLoader(dir string) func(string) (template.HTML, error) {
	cache := map[string]template.HTML{}
	return func(name string) (template.HTML, error) {
		if svg, ok := cache[name]; ok {
			return svg, nil
		}
		data, err := os.ReadFile(filepath.Join(dir, name+".svg"))
		if err != nil {
			return "", fmt.Errorf("icon %s: %w", name, err)
		}
		m := svgRoot.FindSubmatch(data)
		if m == nil {
			return "", fmt.Errorf("icon %s: no <svg> root", name)
		}
		attrs := ""
		for _, a := range svgAttr.FindAllSubmatch(m[1], -1) {
			attrs += fmt.Sprintf(` %s="%s"`, a[1], a[2])
		}
		inner := strings.TrimSpace(string(svgTitle.ReplaceAll(m[2], nil)))
		svg := template.HTML(`<svg class="ico"` + attrs + ` fill="currentColor" aria-hidden="true" focusable="false">` + inner + `</svg>`)
		cache[name] = svg
		return svg, nil
	}
}

// colorIconLoader inlines a full-color provider logo as is, dropping only the
// size, style and title of the source file.
func colorIconLoader(dir string) func(string) (template.HTML, error) {
	return func(name string) (template.HTML, error) {
		data, err := os.ReadFile(filepath.Join(dir, name+".svg"))
		if err != nil {
			return "", fmt.Errorf("icon %s: %w", name, err)
		}
		m := svgRoot.FindSubmatch(data)
		if m == nil {
			return "", fmt.Errorf("icon %s: no <svg> root", name)
		}
		attrs := ""
		for _, a := range svgAttr.FindAllSubmatch(m[1], -1) {
			attrs += fmt.Sprintf(` %s="%s"`, a[1], a[2])
		}
		inner := strings.TrimSpace(string(svgTitle.ReplaceAll(m[2], nil)))
		return template.HTML(`<svg class="ico"` + attrs + ` aria-hidden="true" focusable="false">` + inner + `</svg>`), nil
	}
}

func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, fmt.Errorf("dict: odd argument count")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

func renderFile(tmpl *template.Template, name, path string, data pageData) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func jsonLD(p pageData) (template.JS, error) {
	app := map[string]any{
		"@type":               "SoftwareApplication",
		"name":                "Quosparr",
		"url":                 p.URL,
		"description":         p.T.Meta.Description,
		"inLanguage":          p.Locale.Lang,
		"applicationCategory": "UtilitiesApplication",
		"operatingSystem":     "macOS 12+, Windows 10, Windows 11, Linux",
		"softwareVersion":     p.Release.Version,
		"downloadUrl":         p.Release.Page,
		"image":               siteURL + "/img/logo-192.png",
		"screenshot":          siteURL + p.Wide.Src,
		"license":             "https://www.gnu.org/licenses/agpl-3.0.html",
		"offers":              map[string]any{"@type": "Offer", "price": "0", "priceCurrency": "USD"},
		"author":              map[string]any{"@type": "Person", "name": "Licoy", "url": "https://github.com/Licoy"},
		"sameAs":              []string{repoURL},
	}
	if p.Release.Date != "" {
		app["dateModified"] = p.Release.Date
	}
	graph := map[string]any{
		"@context": "https://schema.org",
		"@graph": []any{
			map[string]any{"@type": "WebSite", "@id": siteURL + "/#website", "url": siteURL + "/", "name": "Quosparr", "inLanguage": p.Locale.Lang},
			app,
		},
	}
	// json.Marshal escapes <, > and &, so the result cannot close the script.
	data, err := json.Marshal(graph)
	return template.JS(data), err
}

func sitemap(locales []Locale, lastmod string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	for _, l := range locales {
		fmt.Fprintf(&b, "  <url>\n    <loc>%s%s</loc>\n", siteURL, l.Path())
		if lastmod != "" {
			fmt.Fprintf(&b, "    <lastmod>%s</lastmod>\n", lastmod)
		}
		for _, alt := range locales {
			fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s%s\"/>\n", alt.Lang, siteURL, alt.Path())
		}
		fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s/\"/>\n  </url>\n", siteURL)
	}
	b.WriteString("</urlset>\n")
	return b.String()
}

func manifest() string {
	data, _ := json.MarshalIndent(map[string]any{
		"name":             "Quosparr",
		"short_name":       "Quosparr",
		"start_url":        "/",
		"display":          "browser",
		"background_color": "#F7F3EE",
		"theme_color":      "#B5582E",
		"icons":            []map[string]string{{"src": "/img/logo-192.png", "sizes": "192x192", "type": "image/png"}},
	}, "", "  ")
	return string(data) + "\n"
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
