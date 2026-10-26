package app

import (
	"errors"
	"sync/atomic"

	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/store"
)

// PlatformCredentials is the system credential store (Keychain, Credential
// Manager, Secret Service) behind CredentialStore. The username and password
// are never logged. The first Load may ask the system to show its prompt;
// every later Load, including background ones, does not.
type PlatformCredentials struct {
	raw  netx.CredentialStore
	once atomic.Int32
}

// NewPlatformCredentials opens the store under netx.DefaultService.
func NewPlatformCredentials() *PlatformCredentials {
	return newPlatformCredentials(netx.NewCredentialStore(netx.DefaultService))
}

func newPlatformCredentials(raw netx.CredentialStore) *PlatformCredentials {
	p := &PlatformCredentials{raw: raw}
	p.once.Store(1)
	return p
}

// Netx is the store netx.ConfigureFrom reads. allowInteraction is honored
// only for the one startup read; a later true is treated as false.
func (p *PlatformCredentials) Netx() netx.CredentialStore { return netxCredentials{p} }

// Save implements CredentialStore.
func (p *PlatformCredentials) Save(id string, c ProxyCredentials) error {
	return credStoreErr(p.raw.Save(netx.Credentials{Username: c.Username, Password: c.Password}, id))
}

// Load implements CredentialStore.
func (p *PlatformCredentials) Load(id string) (ProxyCredentials, error) {
	return p.read(id, p.allowOnce())
}

// Delete implements CredentialStore.
func (p *PlatformCredentials) Delete(id string) error {
	return credStoreErr(p.raw.Delete(id))
}

func (p *PlatformCredentials) allowOnce() bool {
	return p.once.CompareAndSwap(1, 0)
}

func (p *PlatformCredentials) read(id string, allow bool) (ProxyCredentials, error) {
	c, err := p.raw.Load(id, allow)
	if err != nil {
		return ProxyCredentials{}, credStoreErr(err)
	}
	return ProxyCredentials{Username: c.Username, Password: c.Password}, nil
}

type netxCredentials struct{ p *PlatformCredentials }

func (n netxCredentials) Load(id string, allow bool) (netx.Credentials, error) {
	if allow {
		allow = n.p.allowOnce()
	}
	c, err := n.p.read(id, allow)
	if err != nil {
		return netx.Credentials{}, err
	}
	return netx.Credentials{Username: c.Username, Password: c.Password}, nil
}

func (n netxCredentials) Save(c netx.Credentials, id string) error {
	return n.p.Save(id, ProxyCredentials{Username: c.Username, Password: c.Password})
}

func (n netxCredentials) Delete(id string) error { return n.p.Delete(id) }

// credStoreErr maps netx failures onto the sentinels the settings UI already
// shows, retaining the cause behind a fixed, secret-free message.
func credStoreErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, netx.ErrCredentialsUnavailable):
		return &netx.Error{Kind: store.ErrProxyCredentialsUnavailable, Cause: err}
	case errors.Is(err, netx.ErrInvalidCredentials):
		return &netx.Error{Kind: ErrProxyInvalidCredentials, Cause: err}
	case errors.Is(err, store.ErrProxyInvalidConfiguration),
		errors.Is(err, store.ErrProxyInvalidHost),
		errors.Is(err, store.ErrProxyInvalidPort):
		return err
	default:
		return &netx.Error{Kind: ErrCredentialStoreFailed, Cause: err}
	}
}
