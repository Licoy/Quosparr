package claude

import (
	"context"
	"errors"
)

func (m *Module) reload(id string) (ManagedAccount, error) {
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	acc, ok := idx.Account(id)
	if !ok {
		return ManagedAccount{}, &AccountNotFoundError{ID: id}
	}
	return acc, nil
}

func (m *Module) refreshAccount(ctx context.Context, id string, force bool) error {
	acc, err := m.reload(id)
	if err != nil {
		return err
	}
	if acc.IsAPI() {
		return m.refreshAPI(ctx, acc, force)
	}
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return err
	}
	isCurrent := idx.CurrentAccountID == id
	var data []byte
	if isCurrent {
		login, err := m.syncOfficial(ctx)
		if err != nil {
			return err
		}
		isCurrent = login.ID == id
		if isCurrent && login.Known {
			data = login.Data
		}
	}
	if data == nil {
		if data, err = m.Store.LoadCredentialData(id); err != nil {
			return err
		}
	}
	doc, err := ParseAuthDocument(data)
	if err != nil {
		return err
	}
	if isCurrent && doc.AccessExpired(m.now()) {
		acc, err := m.reload(id)
		if err != nil {
			return err
		}
		if err := m.Store.UpdateMetadata(acc.ApplyError(ErrorCodeTokenExpired, false)); err != nil {
			return err
		}
		return ErrTokenExpired
	}
	if !isCurrent && doc.AccessExpired(m.now()) {
		if _, doc, err = m.refreshExpired(ctx, id); err != nil {
			return err
		}
	}
	return m.fetchUsage(ctx, id, doc, force)
}

func (m *Module) refreshExpired(ctx context.Context, id string) ([]byte, AuthDocument, error) {
	if err := m.Store.RefreshManaged(ctx, id, m.Tokens, m.now()); err != nil {
		if !errors.Is(err, ErrInvalidGrant) {
			if acc, reloadErr := m.reload(id); reloadErr == nil {
				_ = m.Store.UpdateMetadata(acc.ApplyError(ErrorCodeRefresh, false))
			}
		}
		return nil, AuthDocument{}, err
	}
	data, err := m.Store.LoadCredentialData(id)
	if err != nil {
		return nil, AuthDocument{}, err
	}
	doc, err := ParseAuthDocument(data)
	if err != nil {
		return nil, AuthDocument{}, err
	}
	return data, doc, nil
}

func (m *Module) fetchUsage(ctx context.Context, id string, doc AuthDocument, force bool) error {
	if m.Usage == nil {
		return nil
	}
	token := doc.OAuth().AccessToken
	wait, ok := m.throttle().Allow(token, m.now(), force)
	if !ok {
		acc, err := m.reload(id)
		if err != nil {
			return err
		}
		// A read skipped only for spacing keeps the cached quota without an error.
		if len(acc.CachedQuota) > 0 && !m.throttle().Blocked(token, m.now()) {
			return nil
		}
		_ = m.Store.UpdateMetadata(acc.ApplyError(ErrorCodeRateLimited, acc.RequiresReauth))
		return &RateLimitedError{RetryAfter: wait, HasRetryAfter: wait > 0}
	}
	m.throttle().Commit(token, m.now())
	raw, fetched, err := m.Usage.Fetch(ctx, token)
	if err != nil {
		return m.noteUsageError(id, token, err)
	}
	if _, err := ParseUsageSnapshot(raw); err != nil {
		acc, reloadErr := m.reload(id)
		if reloadErr != nil {
			return reloadErr
		}
		_ = m.Store.UpdateMetadata(acc.ApplyError(ErrorCodeUsageParse, false))
		return err
	}
	acc, err := m.reload(id)
	if err != nil {
		return err
	}
	if err := m.Store.UpdateMetadata(acc.ApplyQuota(raw, fetched)); err != nil {
		return err
	}
	if err := m.completeSubscription(ctx, id, doc); err != nil {
		code := ErrorCodeUsage
		var limited *RateLimitedError
		if errors.As(err, &limited) {
			m.throttle().ObserveRateLimit(token, m.now(), limited.RetryAfter, limited.HasRetryAfter)
			code = ErrorCodeRateLimited
		}
		acc, reloadErr := m.reload(id)
		if reloadErr != nil {
			return errors.Join(err, reloadErr)
		}
		return errors.Join(err, m.Store.UpdateMetadata(acc.ApplyError(code, false)))
	}
	return nil
}

func (m *Module) noteUsageError(id, token string, err error) error {
	var limited *RateLimitedError
	if errors.As(err, &limited) {
		m.throttle().ObserveRateLimit(token, m.now(), limited.RetryAfter, limited.HasRetryAfter)
		if acc, reloadErr := m.reload(id); reloadErr == nil {
			_ = m.Store.UpdateMetadata(acc.ApplyError(ErrorCodeRateLimited, false))
		}
		return err
	}
	if acc, reloadErr := m.reload(id); reloadErr == nil {
		_ = m.Store.UpdateMetadata(acc.ApplyError(ErrorCodeUsage, false))
	}
	return err
}
