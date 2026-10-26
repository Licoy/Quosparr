package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/claudedesktop"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/platform"
)

// Construction does not discover or control Desktop, or read its data.
func newClaudeDesktopManager(home string, module *claude.Module) *claudedesktop.Manager {
	return claudedesktop.NewManager(platform.RunwayDir(home), claudedesktop.NewNativeHost(home), func(ctx context.Context, id string) (claude.ManagedAccount, error) {
		state, err := module.Load(ctx)
		if err != nil {
			return claude.ManagedAccount{}, err
		}
		for _, a := range state.Accounts {
			if a.ID == id {
				return a, nil
			}
		}
		return claude.ManagedAccount{}, errors.New("claude account unavailable")
	})
}

func desktopBound(rt *liveApp, id string) bool {
	if rt == nil || rt.claudeDesktop == nil {
		return false
	}
	p, err := rt.claudeDesktop.Status(context.Background(), id)
	return err == nil && p.IsBound() && !p.RecoveryRequired
}

func (rt *liveApp) switchClaudeWithDesktop(ctx context.Context, l l10n.Localizer, id string, desktop bool) string {
	message, _ := rt.switchClaudeDesktopResult(ctx, l, id, desktop)
	return message
}

func (rt *liveApp) switchClaudeDesktopResult(ctx context.Context, l l10n.Localizer, id string, desktop bool) (string, bool) {
	if !desktop {
		message := rt.switchClaude(l, id)
		return message, message == ""
	}
	if rt == nil || rt.claudeDesktop == nil || rt.claudeMod == nil {
		return tx(l, xDesktopFailed), false
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result, err := rt.claudeDesktop.Switch(ctx, id, func(after func() error) error {
		return rt.claudeMod.SwitchWithAfter(ctx, id, true, after)
	})
	if rt.claudeB != nil {
		rt.claudeB.reload(true)
	}
	return desktopResultText(l, result, err), err == nil
}

func desktopResultText(l l10n.Localizer, r claudedesktop.SwitchResult, err error) string {
	text := ""
	if err != nil {
		text = desktopFailureText(l, err) + "\n"
	}
	cli := xDesktopCLIUnchanged
	if r.CLISwitched {
		cli = xDesktopCLIChanged
	}
	if r.ManualCLIRecoveryRequired {
		cli = xDesktopCLIRecovery
	}
	text += tx(l, cli)
	state := xDesktopNotStarted
	if r.DesktopStarted {
		state = xDesktopStarted
	} else if r.PreviousDesktopRestored {
		state = xDesktopRestored
	}
	text += "\n" + tx(l, state)
	if r.PreviousDesktopRestored && r.DesktopStarted {
		text += "\n" + tx(l, xDesktopRestored)
	}
	if r.ManualReopenRequired {
		text += "\n" + tx(l, xDesktopRecovery)
	}
	return text
}

func desktopFailureText(l l10n.Localizer, err error) string {
	key := xDesktopFailed
	switch {
	case errors.Is(err, claudedesktop.ErrRollback), errors.Is(err, claudedesktop.ErrRecoveryRequired):
		key = xDesktopRecovery
	case errors.Is(err, claudedesktop.ErrStop):
		key = xDesktopStopFailed
	case errors.Is(err, claudedesktop.ErrLaunch):
		key = xDesktopLaunchFailed
	case errors.Is(err, claudedesktop.ErrBusy):
		key = xDesktopBusy
	case errors.Is(err, claudedesktop.ErrNotBound):
		key = xDesktopUnbound
	case errors.Is(err, claudedesktop.ErrCrossVolume):
		key = xDesktopCrossVolume
	case errors.Is(err, claudedesktop.ErrProfileChanged):
		key = xDesktopChanged
	case errors.Is(err, claudedesktop.ErrUnsafePath):
		key = xDesktopUnsafe
	case errors.Is(err, claudedesktop.ErrPersistence):
		key = xDesktopPersistence
	}
	return accountFailure(l, tx(l, key), err)
}
