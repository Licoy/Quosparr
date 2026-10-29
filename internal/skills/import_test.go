package skills

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func importSkillText(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\nBody of " + name + ".\n"
}

func importNames(found []Candidate) []string {
	var names []string
	for _, c := range found {
		names = append(names, c.Name)
	}
	return names
}

func importFind(t *testing.T, found []Candidate, name string) Candidate {
	t.Helper()
	for _, c := range found {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("candidate %q not found in %v", name, importNames(found))
	return Candidate{}
}

func importPaths(c Candidate) []string {
	var paths []string
	for _, f := range c.Files {
		paths = append(paths, f.Path)
	}
	return paths
}

func TestParseGitHubSource(t *testing.T) {
	cases := []struct {
		raw                         string
		owner, repo, ref, sub, tree string
	}{
		{raw: "acme/tools", owner: "acme", repo: "tools"},
		{raw: " acme/tools/skills/pdf ", owner: "acme", repo: "tools", sub: "skills/pdf"},
		{raw: "https://github.com/acme/tools.git", owner: "acme", repo: "tools"},
		{raw: "github.com/acme/tools?tab=readme", owner: "acme", repo: "tools"},
		{raw: "https://github.com/acme/tools/tree/main/skills/pdf", owner: "acme", repo: "tools", ref: "main", sub: "skills/pdf", tree: "main/skills/pdf"},
		{raw: "https://github.com/acme/tools/blob/v1/skills/pdf/SKILL.md", owner: "acme", repo: "tools", ref: "v1", sub: "skills/pdf", tree: "v1/skills/pdf"},
	}
	for _, tc := range cases {
		got, err := ParseGitHubSource(tc.raw)
		if err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		if got.Owner != tc.owner || got.Repo != tc.repo || got.Ref != tc.ref || got.Subpath != tc.sub || got.treePath != tc.tree {
			t.Fatalf("%q = %+v", tc.raw, got)
		}
	}
	for _, raw := range []string{"", "acme", "http://github.com/a/b", "https://gitlab.com/a/b", "git@github.com:a/b.git",
		"https://user:pw@github.com/a/b", "https://github.com/a/b/issues/1", "a/b/../c", "./a/b", "https://github.com/a/b/tree/main/x/../y"} {
		if _, err := ParseGitHubSource(raw); importCode(err) != ImportInvalidSource {
			t.Fatalf("%q: error %v", raw, err)
		}
	}
}

func TestDiscoverRepositoryLayout(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) { commonWriteFile(t, filepath.Join(dir, filepath.FromSlash(rel)), content) }
	write("README.md", "repo readme")
	write("skills/pdf/SKILL.md", importSkillText("pdf", "Fill PDF forms"))
	write("skills/pdf/reference.md", "ref")
	write("skills/pdf/.DS_Store", "litter")
	write("skills/pdf/node_modules/x/index.js", "dependency")
	write("skills/pdf/.github/workflows/ci.yml", "ci")
	write("skills/pdf/inner/SKILL.md", importSkillText("inner", "Part of pdf"))
	write("skills/docs/word/SKILL.md", importSkillText("word", "Edit documents"))
	write("skills/hidden/SKILL.md", "---\nname: hidden\ndescription: Internal\nmetadata:\n  internal: true\n---\n")
	write("skills/zz-again/SKILL.md", importSkillText("PDF", "Same name as pdf"))
	write("skills/nodesc/SKILL.md", "---\nname: nodesc\n---\nBody\n")
	write("examples/deep/demo/SKILL.md", importSkillText("demo", "Not in a container"))
	script := filepath.Join(dir, "skills", "pdf", "scripts", "run.sh")
	commonWriteFile(t, script, "#!/bin/sh\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink("reference.md", filepath.Join(dir, "skills", "pdf", "alias.md")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../README.md", filepath.Join(dir, "skills", "pdf", "outside.md")); err != nil {
			t.Fatal(err)
		}
	}

	snap, err := LoadFolder(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	found, err := Discover(snap)
	if err != nil {
		t.Fatal(err)
	}
	if got := importNames(found); !slices.Equal(got, []string{"word", "nodesc", "pdf"}) {
		t.Fatalf("names = %v", got)
	}
	pdf := importFind(t, found, "pdf")
	want := []string{"SKILL.md", "inner/SKILL.md", "reference.md", "scripts/run.sh"}
	if runtime.GOOS != "windows" {
		want = []string{"SKILL.md", "alias.md", "inner/SKILL.md", "reference.md", "scripts/run.sh"}
		if !slices.Equal(pdf.Skipped, []string{"outside.md"}) {
			t.Fatalf("skipped = %v", pdf.Skipped)
		}
		if pdf.Executables != 1 {
			t.Fatalf("executables = %d", pdf.Executables)
		}
	}
	if got := importPaths(pdf); !slices.Equal(got, want) {
		t.Fatalf("files = %v", got)
	}
	if pdf.Folder != "skills/pdf" || pdf.DirName != "pdf" || pdf.Description != "Fill PDF forms" || !strings.Contains(pdf.Body, "Body of pdf") {
		t.Fatalf("pdf = %+v", pdf)
	}
	if importFind(t, found, "nodesc").Problem != ProblemMissingDesc {
		t.Fatal("missing description not reported")
	}
}

func TestDiscoverRootSkillAndFallback(t *testing.T) {
	dir := t.TempDir()
	commonWriteFile(t, filepath.Join(dir, "SKILL.md"), importSkillText("My Tool", "Root skill"))
	commonWriteFile(t, filepath.Join(dir, "skills", "other", "SKILL.md"), importSkillText("other", "Ignored"))
	snap, err := LoadFolder(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	found, err := Discover(snap)
	if err != nil || len(found) != 1 || found[0].Folder != "" || found[0].DirName != "my-tool" {
		t.Fatalf("found = %+v, %v", found, err)
	}

	deep := t.TempDir()
	commonWriteFile(t, filepath.Join(deep, "a", "b", "c", "SKILL.md"), importSkillText("deep", "Found by the fallback"))
	commonWriteFile(t, filepath.Join(deep, "a", "b", "c", "d", "SKILL.md"), importSkillText("nested", "Inside deep"))
	snap, err = LoadFolder(context.Background(), deep)
	if err != nil {
		t.Fatal(err)
	}
	found, err = Discover(snap)
	if err != nil || !slices.Equal(importNames(found), []string{"deep"}) {
		t.Fatalf("found = %v, %v", importNames(found), err)
	}

	empty := t.TempDir()
	commonWriteFile(t, filepath.Join(empty, "README.md"), "no skills")
	if _, err := LoadFolder(context.Background(), empty); importCode(err) != ImportNoSkills {
		t.Fatalf("err = %v", err) // found from the listing, before anything is read
	}
}

func TestLoadReadsOnlyTheSkills(t *testing.T) {
	dir := t.TempDir()
	commonWriteFile(t, filepath.Join(dir, "skills", "pdf", "SKILL.md"), importSkillText("pdf", "Fill PDF forms"))
	commonWriteFile(t, filepath.Join(dir, "skills", "pdf", "ref", "forms.md"), "forms")
	commonWriteFile(t, filepath.Join(dir, ".claude-plugin", "plugin.json"), `{"name":"kit"}`)
	big := filepath.Join(dir, "docs", "video.bin")
	commonWriteFile(t, big, "")
	if err := os.Truncate(big, MaxImportBytes+1); err != nil {
		t.Fatal(err)
	}
	snap, err := LoadFolder(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(snap.order, []string{".claude-plugin/plugin.json", "skills/pdf/SKILL.md", "skills/pdf/ref/forms.md"}) || snap.dirs["docs"] || !snap.dirs[".claude-plugin"] {
		t.Fatalf("kept %v", snap.order)
	}
	found, err := Discover(snap)
	if err != nil || len(found) != 1 || found[0].Plugin != "" || len(found[0].Files) != 2 {
		t.Fatalf("found = %+v, %v", found, err)
	}

	// A skill too large to keep is refused from the listing.
	if err := os.Truncate(filepath.Join(dir, "skills", "pdf", "ref", "forms.md"), MaxImportFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFolder(context.Background(), dir); importCode(err) != ImportTooLarge {
		t.Fatalf("large skill: %v", err)
	}

	archive := filepath.Join(t.TempDir(), "kit.zip")
	out, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for name, size := range map[string]int{"kit/skills/pdf/SKILL.md": 0, "kit/docs/video.bin": MaxImportFileBytes + 1} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if size == 0 {
			_, err = w.Write([]byte(importSkillText("pdf", "Fill PDF forms")))
		} else {
			_, err = w.Write(make([]byte, size))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()
	snap, err = LoadZip(context.Background(), archive)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(snap.order, []string{"kit/skills/pdf/SKILL.md"}) {
		t.Fatalf("zip kept %v", snap.order)
	}
}

func TestDiscoverPluginManifest(t *testing.T) {
	dir := t.TempDir()
	commonWriteFile(t, filepath.Join(dir, ".claude-plugin", "marketplace.json"),
		`{"metadata":{"pluginRoot":"./plugins"},"plugins":[{"name":"docs","source":"./docs","skills":["./custom/xlsx"]},{"name":"remote","source":{"source":"github"}}]}`)
	commonWriteFile(t, filepath.Join(dir, "plugins", "docs", "custom", "xlsx", "SKILL.md"), importSkillText("xlsx", "Spreadsheets"))
	commonWriteFile(t, filepath.Join(dir, "plugins", "docs", "skills", "pptx", "SKILL.md"), importSkillText("pptx", "Slides"))
	snap, err := LoadFolder(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	found, err := Discover(snap)
	if err != nil {
		t.Fatal(err)
	}
	if got := importNames(found); !slices.Equal(got, []string{"xlsx", "pptx"}) {
		t.Fatalf("names = %v", got)
	}
	if importFind(t, found, "xlsx").Plugin != "docs" {
		t.Fatalf("plugin = %q", importFind(t, found, "xlsx").Plugin)
	}
}

func TestLoadZip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bundle.skill")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(out)
	add := func(name, content string, mode os.FileMode) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(mode)
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	add("bundle/SKILL.md", importSkillText("bundle", "From a zip"), 0o644)
	add("bundle/run.py", "print()", 0o755)
	add("bundle/link.md", "SKILL.md", os.ModeSymlink|0o777)
	add("__MACOSX/bundle/._SKILL.md", "resource fork", 0o644)
	add("bundle/._run.py", "resource fork", 0o644)
	add("../evil.txt", "escape", 0o644)
	add("other/../../evil.txt", "escape", 0o644)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()

	snap, err := LoadZip(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.rejected) != 2 {
		t.Fatalf("rejected = %v", snap.rejected)
	}
	found, err := Discover(snap)
	if err != nil || len(found) != 1 {
		t.Fatalf("found = %v, %v", importNames(found), err)
	}
	if got := importPaths(found[0]); !slices.Equal(got, []string{"SKILL.md", "link.md", "run.py"}) {
		t.Fatalf("files = %v", got)
	}
	if found[0].Executables != 1 {
		t.Fatalf("executables = %d", found[0].Executables)
	}
}

func TestSnapshotRejectsCaseConflicts(t *testing.T) {
	snap := newSnapshot(Source{Kind: SourceZip})
	if err := snap.addFile("s/SKILL.md", []byte(importSkillText("s", "d")), false); err != nil {
		t.Fatal(err)
	}
	if err := snap.addFile("s/Notes.md", []byte("a"), false); err != nil {
		t.Fatal(err)
	}
	if err := snap.addFile("s/notes.md", []byte("b"), false); err != nil {
		t.Fatal(err)
	}
	if err := snap.finish(); err != nil {
		t.Fatal(err)
	}
	found, err := Discover(snap)
	if err != nil || found[0].Problem != ProblemInvalidPath {
		t.Fatalf("found = %+v, %v", found, err)
	}
}

func TestSanitizeImportName(t *testing.T) {
	for raw, want := range map[string]string{"My Tool": "my-tool", "../../etc": "etc", ".hidden": "hidden", "a__b.c": "a__b.c", "中文": "", "PDF—Forms!": "pdf-forms"} {
		if got := sanitizeImportName(raw); got != want {
			t.Fatalf("%q = %q, want %q", raw, got, want)
		}
	}
}
