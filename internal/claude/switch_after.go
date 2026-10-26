package claude

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

// ErrSwitchRollback means the original CLI state could not be restored safely.
var ErrSwitchRollback = errors.New("claude: coordinated switch rollback failed")

// SwitchWithAfter installs a subscription login, then runs after while account
// operations remain serialized. A failed callback restores the actual previous
// official login, settings and current marker, including an API or absent login.
// Rotated managed credentials are intentionally retained. The callback must be
// bounded by its caller and must not call Module methods (the module is locked).
func (m *Module) SwitchWithAfter(ctx context.Context, id string, allow bool, after func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if after == nil {
		return errors.New("claude: coordinated switch requires a callback")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.runningBlocked(ctx, allow); err != nil {
		return err
	}
	target, _, err := m.targetCredential(id)
	if err != nil {
		return err
	}
	if target.IsAPI() {
		return ErrAPIAccount
	}
	if _, err = m.syncOfficial(ctx); err != nil {
		return err
	}
	target, data, err := m.targetCredential(id)
	if err != nil {
		return err
	}
	data, err = m.refreshTargetIfNeeded(ctx, target, data)
	if err != nil {
		return err
	}
	txn, err := m.prepareCoordinated(ctx, target, data)
	if err != nil {
		return err
	}
	if err = txn.apply(ctx); err == nil {
		err = after()
	}
	if err == nil {
		return nil
	}
	// Cancellation must not suppress restoration, but restoration is bounded.
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if rollbackErr := txn.restore(rollbackCtx); rollbackErr != nil {
		return errors.Join(err, ErrSwitchRollback, rollbackErr)
	}
	return err
}

type coordinatedSwitch struct {
	officialAttempted bool
	module            *Module
	target            ManagedAccount
	current           string
	official          *switchValue
	config, settings  *switchValue
	indexWritten      bool
	lastUsed          *time.Time
}

func (m *Module) prepareCoordinated(ctx context.Context, target ManagedAccount, data []byte) (*coordinatedSwitch, error) {
	official, had, err := m.readOfficial(ctx)
	if err != nil {
		return nil, err
	}
	if !had {
		if _, ok := m.Store.official().(interface{ Remove(context.Context) error }); !ok {
			return nil, errors.New("claude: credential store cannot restore absent login")
		}
	}
	updated, err := ReplaceOAuth(official, data)
	if err != nil {
		return nil, err
	}
	cfg, err := fileSwitchValue(m.configPath())
	if err != nil {
		return nil, err
	}
	cfg.next, err = replaceOAuthAccountBytes(cfg.old, target.Identity.OAuthAccountValue())
	if err != nil {
		return nil, err
	}
	settings, err := fileSwitchValue(m.Store.SettingsPath())
	if err != nil {
		return nil, err
	}
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return nil, err
	}
	if prev, key, _, ok := m.currentAPI(idx); ok {
		settings.next, _, err = RemoveAPISettings(settings.old, prev.appliedAPIEnv(key))
		if err != nil {
			return nil, err
		}
	}
	return &coordinatedSwitch{module: m, target: target, current: idx.CurrentAccountID,
		official: &switchValue{old: official, next: updated, existed: had}, config: cfg,
		settings: settings, lastUsed: target.LastUsedAt}, nil
}

func (t *coordinatedSwitch) apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m := t.module
	m.Store.mu.Lock()
	defer m.Store.mu.Unlock()
	if err := t.checkOriginal(ctx); err != nil {
		return err
	}
	if !bytes.Equal(t.settings.old, t.settings.next) {
		_, err := m.Store.editAPISettings(m.now(), func(old []byte) ([]byte, error) {
			if !bytes.Equal(old, t.settings.old) {
				return nil, ErrCredentialChanged
			}
			return t.settings.next, nil
		})
		if err != nil {
			return err
		}
		t.settings.written = true
	}
	t.officialAttempted = true
	if err := m.Store.SaveOfficial(ctx, t.official.next); err != nil {
		return err
	}
	t.official.written = true
	if m.Replacer.BeforeWrite != nil {
		if err := m.Replacer.BeforeWrite(t.config.path); err != nil {
			return err
		}
	}
	if err := t.config.check(false); err != nil {
		return err
	}
	if err := platform.WriteFileAtomic(t.config.path, t.config.next, t.config.mode); err != nil {
		return err
	}
	t.config.written = true
	if err := m.Store.setCurrent(t.target.ID, m.now()); err != nil {
		return err
	}
	t.indexWritten = true
	return nil
}

func (t *coordinatedSwitch) checkOriginal(ctx context.Context) error {
	current, exists, err := t.module.readOfficial(ctx)
	if err != nil {
		return err
	}
	if exists != t.official.existed || !bytes.Equal(current, t.official.old) {
		return ErrCredentialChanged
	}
	for _, f := range []*switchValue{t.config, t.settings} {
		if err := f.check(false); err != nil {
			return err
		}
	}
	idx, err := t.module.Store.loadIndex()
	if err != nil {
		return err
	}
	if idx.CurrentAccountID != t.current {
		return ErrCredentialChanged
	}
	return nil
}

func (t *coordinatedSwitch) restore(ctx context.Context) error {
	if !t.officialAttempted && !t.config.written && !t.settings.written && !t.indexWritten {
		return nil
	}
	s := t.module.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	// Compare every owned write before restoring any of them, so an external
	// login change cannot leave a mixture of old credentials and new identity.
	if t.officialAttempted {
		data, had, err := t.module.readOfficial(ctx)
		if err != nil {
			return err
		}
		if had == t.official.existed && bytes.Equal(data, t.official.old) {
			t.official.written = false
		} else if had && bytes.Equal(data, t.official.next) {
			t.official.written = true
		} else {
			return ErrCredentialChanged
		}
	}
	for _, f := range []*switchValue{t.config, t.settings} {
		if err := f.check(f.written); err != nil {
			return err
		}
	}
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	if t.indexWritten && idx.CurrentAccountID != t.target.ID {
		return ErrCredentialChanged
	}
	var errs []error
	if t.official.written {
		errs = append(errs, t.restoreOfficial(ctx))
	}
	for _, f := range []*switchValue{t.config, t.settings} {
		if f.written {
			errs = append(errs, f.restore())
		}
	}
	if t.indexWritten {
		idx.CurrentAccountID = t.current
		if a, ok := idx.Account(t.target.ID); ok {
			a.LastUsedAt = t.lastUsed
			idx.Upsert(a)
		}
		errs = append(errs, s.saveIndex(idx))
	}
	return errors.Join(errs...)
}

func (t *coordinatedSwitch) restoreOfficial(ctx context.Context) error {
	if t.official.existed {
		return t.module.Store.SaveOfficial(ctx, t.official.old)
	}
	remover, ok := t.module.Store.official().(interface{ Remove(context.Context) error })
	if !ok {
		return errors.New("claude: credential store cannot restore absent login")
	}
	return remover.Remove(ctx)
}
