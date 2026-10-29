package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

// skillAddPage is the add page: the back link, title and steps stay put
// over the step's scrolling body, and its actions stay at the bottom.
func (u *settingsUI) skillAddPage(c *ui.Context, acts SkillActions) {
	w, l := u.skills.add, u.L()
	ui.Column(c).Padding(0, 24).Gap(12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if detailBackButton(c, l.Text(l10n.KeyBack)).Clicked() {
				u.closeSkillAdd()
			}
			ui.Text(c, tx(l, xSkillAdd)).FontSize(15).FontWeight(650).SingleLine()
		})
		// Leaving may have just dropped the wizard the steps read.
		if w.open {
			u.skillAddSteps(c)
		}
	})
	if !w.open {
		return
	}
	ui.Scroll(c.Key(fmt.Sprint("skill-add-step-", w.step))).Grow(1).Basis(0).Padding(0, 24, 20, 24).Gap(16).Children(func() {
		switch w.step {
		case skillAddStepSource:
			u.skillAddSource(c, acts)
		case skillAddStepPick:
			u.skillAddPick(c)
		case skillAddStepReview:
			u.skillAddReview(c, acts)
		default:
			u.skillAddResults(c)
		}
	})
	ui.Column(c).Children(func() {
		ui.Box(c).Height(1).Background(pal(c.Theme()).Line)
		ui.Row(c).Padding(10, 24, 12, 24).Gap(8).AlignItems(ui.Center).MinHeight(52).Children(func() {
			u.skillAddFooter(c, acts)
		})
	})
}

// skillAddSteps numbers the three steps: done ones carry a tick and lead
// back to themselves, the current one is filled.
func (u *settingsUI) skillAddSteps(c *ui.Context) {
	w, l, t := u.skills.add, u.L(), c.Theme()
	titles := []textKey{xSkillAddStepSource, xSkillAddStepPick, xSkillAddStepReview}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
		for i, title := range titles {
			if i > 0 {
				ui.Box(c).Width(20).Height(1).Background(t.Border)
			}
			done, current := i < w.step, i == w.step
			if !done || w.step == skillAddStepResults || w.writing {
				ui.Row(c).Gap(6).Padding(3, 6).AlignItems(ui.Center).Children(func() { mcpAddStep(c, i, tx(l, title), done, current) })
				continue
			}
			b := ui.ButtonBase(c.Key(fmt.Sprint("skill-step-", i))).Label(tx(l, title)).Gap(6).Padding(3, 6).Radius(6).AlignItems(ui.Center).
				Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(pal(t).Hover)
			}
			b.Children(func() { mcpAddStep(c, i, tx(l, title), done, current) })
			if b.Clicked() {
				w.stopPrepare()
				w.step = i
			}
		}
	})
}

// skillAddFooter holds a step's actions, the leading one filled.
func (u *settingsUI) skillAddFooter(c *ui.Context, acts SkillActions) {
	w, l, t := u.skills.add, u.L(), c.Theme()
	back := func(step int) {
		if PillButton(c, nil, tx(l, xMCPPrevious), w.writing).Clicked() {
			w.stopPrepare()
			w.step = step
		}
	}
	switch w.step {
	case skillAddStepSource:
		if w.load.Running {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, skillAddLoadText(l, w.loadDone, w.loadTotal)).FontSize(11).TextColor(t.TextMuted).SingleLine()
			if PillButton(c.Key("skill-add-stop"), nil, l.Text(l10n.KeyCancel), false).Clicked() {
				w.stopLoad()
			}
		}
		ui.Spacer(c).Grow(1)
		_, err := w.source()
		if AccentPillButton(c.Key("skill-add-read"), nil, tx(l, xMCPNext), w.load.Running || err != nil || acts.LoadImport == nil).Clicked() {
			u.loadSkillSource(acts)
		}
	case skillAddStepPick:
		ui.Text(c, tx(l, xSkillAddSelected, len(w.pickedSkills()), len(skillAddUsable(w.found)))).FontSize(11).TextColor(t.TextMuted)
		ui.Spacer(c).Grow(1)
		back(skillAddStepSource)
		if AccentPillButton(c.Key("skill-add-next"), nil, tx(l, xMCPNext), len(w.pickedSkills()) == 0).Clicked() {
			w.step = skillAddStepReview
			u.prepareSkillInstall(acts, false)
		}
	case skillAddStepReview:
		if w.task.Running {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, tx(l, xSkillAddChecking)).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Spacer(c).Grow(1)
		back(skillAddStepPick)
		if AccentPillButton(c.Key("skill-add-submit"), nil, tx(l, xSkillAddSubmit, w.pendingAdds()), w.blocked() || acts.Install == nil).Clicked() {
			u.submitSkillInstall(acts)
		}
	default:
		ui.Spacer(c).Grow(1)
		if w.writing {
			return
		}
		if skillAddFailed(w.plans) && w.snap != nil {
			if PillButton(c.Key("skill-add-retry"), nil, tx(l, xSkillAddRetry), w.task.Running).Clicked() {
				w.step = skillAddStepReview
				u.prepareSkillInstall(acts, true)
			}
		}
		if AccentPillButton(c.Key("skill-add-done"), nil, tx(l, xMCPDone), false).Clicked() {
			u.finishSkillAdd()
		}
	}
}

// skillAddSource is the first step: a GitHub repository, a folder or a
// zip file, and what reading one does.
func (u *settingsUI) skillAddSource(c *ui.Context, acts SkillActions) {
	w, l, t := u.skills.add, u.L(), c.Theme()
	ui.Row(c).Children(func() {
		if PillTabs(c.Key("skill-add-kind"), &w.kind, w.load.Running, "GitHub", tx(l, xSkillAddFolder), tx(l, xSkillAddZip)) {
			w.sourceChanged()
		}
	})
	message := ""
	if w.load.Err != nil {
		message = skillAddErrorText(l, w.load.Err)
	}
	Panel(c).Padding(12, 14).Children(func() {
		ui.Form(c, func() {
			switch w.sourceKind() {
			case skills.SourceGitHub:
				ui.Field(c, tx(l, xSkillAddRepo), func() {
					if ui.TextInput(c.Key("skill-add-github"), &w.github).Placeholder("owner/repo").Disabled(w.load.Running).Changed() {
						w.sourceChanged()
					}
				}).Description(tx(l, xSkillAddRepoHint)).Error(message)
			case skills.SourceFolder:
				ui.Field(c, tx(l, xSkillAddFolderPath), func() { u.skillAddPath(c, acts, true) }).Description(tx(l, xSkillAddFolderHint)).Error(message)
			default:
				ui.Field(c, tx(l, xSkillAddZipPath), func() { u.skillAddPath(c, acts, false) }).Description(tx(l, xSkillAddZipHint)).Error(message)
			}
		})
	})
	ui.Column(c).Gap(4).Children(func() {
		mcpInfoLine(c, icShield, tx(l, xSkillAddMemoryOnly), t.TextMuted)
		mcpInfoLine(c, icSearch, tx(l, xSkillAddScanHint), t.TextMuted)
		if w.sourceKind() == skills.SourceGitHub {
			mcpInfoLine(c, icGithub, tx(l, xSkillAddGitHubHint), t.TextMuted)
		}
	})
}

// skillAddLoadText is what reading a source has got to: a GitHub
// download's share of the repository, or its size when that is unknown.
func skillAddLoadText(l l10n.Localizer, done, total int64) string {
	switch {
	case done <= 0:
		return tx(l, xSkillAddReading)
	case total > 0:
		return tx(l, xSkillAddDownloading, min(done*100/total, 99))
	}
	return tx(l, xSkillAddDownloadingSize, skillBytes(done))
}

// skillAddPath is a path box with a button that asks the system for it.
func (u *settingsUI) skillAddPath(c *ui.Context, acts SkillActions, folder bool) {
	w, l := u.skills.add, u.L()
	value, title, key := &w.zip, tx(l, xSkillAddChooseZip), "skill-add-zip"
	if folder {
		value, title, key = &w.folder, tx(l, xSkillAddChooseFolder), "skill-add-folder"
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if ui.TextInput(c.Key(key), value).Placeholder(tx(l, xSkillAddPathHint)).Font("monospace").Grow(1).Basis(0).Disabled(w.load.Running).Changed() {
			w.sourceChanged()
		}
		if PillButton(c.Key(key+"-pick"), icFolder, tx(l, xSkillAddChoose), w.load.Running || acts.PickPath == nil).Clicked() {
			path, err := acts.PickPath(title, folder)
			switch {
			case err != nil:
				w.load = Task{Err: err}
			case path != "":
				*value = path
				w.sourceChanged()
			}
		}
	})
}

// skillAddPick is the second step: what was read and the skills found,
// each one checked to add and opened to show its files and SKILL.md.
func (u *settingsUI) skillAddPick(c *ui.Context) {
	w, l, t := u.skills.add, u.L(), c.Theme()
	if w.snap == nil {
		return
	}
	src := w.snap.Source
	Panel(c).Padding(10, 14).Gap(6).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, skillSourceIcon(src.Kind)).Size(16, 16).TextColor(t.TextMuted)
			ui.Text(c, src.Label()).FontSize(12).Font("monospace").SingleLine().Ellipsis("…").Shrink(1).Selectable()
			if w.snap.Commit != "" {
				ui.Text(c, shortCommit(w.snap.Commit)).FontSize(11).Font("monospace").TextColor(t.TextMuted).SingleLine()
			}
			ui.Spacer(c).Grow(1)
			ui.Text(c, tx(l, xSkillAddFound, len(w.found))).FontSize(11).TextColor(t.TextMuted).SingleLine().Shrink(0)
			if repo := src.Repository(); repo != "" && IconButton(c, icExternal, repo, false).Clicked() {
				c.OpenURL(repo)
			}
		})
		if w.snap.Truncated() {
			mcpInfoLine(c, icWarnCircle, tx(l, xSkillAddTruncated, skills.MaxImportSkills), t.Warning)
		}
		if n := len(w.snap.Rejected()); n > 0 {
			mcpInfoLine(c, icWarnCircle, tx(l, xSkillAddRejected, n), t.Warning)
		}
	})
	usable := skillAddUsable(w.found)
	if picked := len(w.pickedSkills()); len(usable) > 1 {
		listBar(c).Children(func() {
			all := picked == len(usable)
			if mixedCheckbox(c, "skill-add-all", all, picked > 0, tx(l, xSkillAddSelectAll), false) {
				for _, cand := range usable {
					w.picked[cand.Folder] = !all
				}
				w.invalidate()
			}
			ui.Text(c, tx(l, xSkillAddSelectAll)).FontSize(12).TextColor(t.TextMuted)
		})
	}
	listPanel(c).Children(func() {
		for i, cand := range w.found {
			settingsRule(c, i)
			u.skillAddCandidate(c, cand)
		}
	})
	if slices.ContainsFunc(w.pickedSkills(), func(cand skills.Candidate) bool { return cand.Executables > 0 }) {
		mcpInfoLine(c, icTerminal, tx(l, xSkillAddExecHint), t.Warning)
	}
}

func (u *settingsUI) skillAddCandidate(c *ui.Context, cand skills.Candidate) {
	w, l, t := u.skills.add, u.L(), c.Theme()
	key := func(name string) *ui.Context { return c.Key(name + "-" + cand.Folder) }
	ui.Column(key("skill-add-cand")).Gap(8).Padding(8, listRowPadX).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
			on := w.picked[cand.Folder]
			box := ui.Checkbox(key("skill-add-pick"), &on, "").Label(tx(l, xSkillAddPickOne, cand.Name)).Size(18, 18).Margin(1, 0, 0, 0).
				Disabled(cand.Problem != skills.ProblemNone)
			if cand.Problem == skills.ProblemNone {
				box.Cursor(ui.CursorPointer)
			}
			if box.Changed() {
				w.setPicked(cand, on)
			}
			ui.Column(c).Gap(3).Grow(1).Basis(0).Children(func() {
				ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
					ui.Text(c, cand.Name).FontSize(13).FontWeight(600).Selectable()
					if cand.Problem != skills.ProblemNone {
						settingsBadge(c, icWarnCircle, skillAddProblemText(l, cand.Problem), t.Danger)
					}
					if cand.Plugin != "" {
						settingsBadge(c, nil, tx(l, xSkillAddPlugin, cand.Plugin), t.TextMuted)
					}
					if cand.Executables > 0 {
						settingsBadge(c, icTerminal, tx(l, xSkillAddExec, cand.Executables), t.Warning)
					}
				})
				if cand.Description != "" {
					ui.Text(c, cand.Description).FontSize(12).LineHeight(1.35).Selectable()
				}
				folder := cand.Folder
				if folder == "" {
					folder = "."
				}
				ui.Text(c, folder+" · "+tx(l, xSkillAddFiles, len(cand.Files), skillBytes(cand.Size))).FontSize(11).Font("monospace").
					TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			})
			open := w.expanded[cand.Folder]
			icon, label := icChevDown, tx(l, xSkillAddShowDetails, cand.Name)
			if open {
				icon, label = icChevUp, tx(l, xSkillAddHideDetails, cand.Name)
			}
			if IconButton(key("skill-add-expand"), icon, label, false).Clicked() {
				w.expanded[cand.Folder] = !open
			}
		})
		if w.expanded[cand.Folder] {
			skillAddCandidateDetails(c, l, cand)
		}
	})
}

// skillAddCandidateDetails lists what installing a skill writes, what it
// leaves out, and its SKILL.md as it will be installed.
func skillAddCandidateDetails(c *ui.Context, l l10n.Localizer, cand skills.Candidate) {
	t := c.Theme()
	inset := func(body func()) {
		ui.Column(c).Gap(3).Padding(8, 10).Radius(7).Background(pal(t).Inset).Border(1, pal(t).Line).Margin(0, 0, 0, 28).Children(body)
	}
	inset(func() {
		for _, f := range cand.Files {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, f.Path).FontSize(11).Font("monospace").Grow(1).Basis(0).SingleLine().Ellipsis("…").Selectable()
				if f.Exec {
					ui.Icon(c, icTerminal).Size(11, 11).TextColor(t.Warning)
				}
				ui.Text(c, skillBytes(f.Size)).FontSize(11).FontFeatures("tnum").TextColor(t.TextMuted).SingleLine()
			})
		}
	})
	if len(cand.Skipped) > 0 {
		inset(func() {
			ui.Text(c, tx(l, xSkillAddSkippedLinks)).FontSize(11).TextColor(t.Warning)
			for _, p := range cand.Skipped {
				ui.Text(c, p).FontSize(11).Font("monospace").TextColor(t.TextMuted).Selectable()
			}
		})
	}
	if body := strings.TrimSpace(cand.Body); body != "" {
		inset(func() {
			ui.Text(c, "SKILL.md").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
			ui.Text(c, body).FontSize(11).Font("monospace").LineHeight(1.4).Selectable()
			if cand.BodyTruncated {
				ui.Text(c, tx(l, xSkillAddBodyCut)).FontSize(11).TextColor(t.TextMuted)
			}
		})
	}
}

func skillSourceIcon(kind skills.SourceKind) *ui.SVG {
	switch kind {
	case skills.SourceGitHub:
		return icGithub
	case skills.SourceZip:
		return icFile
	}
	return icFolder
}

func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func skillAddProblemText(l l10n.Localizer, p skills.CandidateProblem) string {
	switch p {
	case skills.ProblemInvalidFront:
		return tx(l, xSkillAddProblemFront)
	case skills.ProblemMissingDesc:
		return tx(l, xSkillAddProblemDesc)
	case skills.ProblemInvalidName:
		return tx(l, xSkillAddProblemName)
	}
	return tx(l, xSkillAddProblemPath)
}

// skillTargetName names a folder skills can be added to.
func skillTargetName(l l10n.Localizer, target skills.InstallTarget) string {
	if target == skills.TargetShared {
		return tx(l, xSkillAddShared)
	}
	return l.Text(providerKey(store.Provider(target)))
}

func skillSourceKindName(l l10n.Localizer, kind skills.SourceKind) string {
	switch kind {
	case skills.SourceGitHub:
		return "GitHub"
	case skills.SourceZip:
		return tx(l, xSkillAddZip)
	}
	return tx(l, xSkillAddFolder)
}
