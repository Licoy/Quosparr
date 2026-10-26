package claude

import (
	"errors"
	"os"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

func browserAccount(idx AccountIndex, identity CredentialIdentity, doc AuthDocument, now time.Time) ManagedAccount {
	acc, exists := idx.Account(identity.StableID())
	if !exists {
		acc = ManagedAccount{ID: identity.StableID(), SortIndex: nextSortIndex(idx), CreatedAt: now}
	}
	identity.Email = firstNonEmpty(identity.Email, acc.Identity.Email)
	identity.DisplayName = firstNonEmpty(identity.DisplayName, acc.Identity.DisplayName)
	identity.OrganizationName = firstNonEmpty(identity.OrganizationName, acc.Identity.OrganizationName)
	acc.Identity = identity
	acc.SubscriptionType = firstNonEmpty(doc.OAuth().SubscriptionType, acc.SubscriptionType)
	acc.RateLimitTier = firstNonEmpty(doc.OAuth().RateLimitTier, acc.RateLimitTier)
	acc.RequiresReauth, acc.LastError = false, ""
	return acc
}

type browserCommit struct {
	account    ManagedAccount
	index      AccountIndex
	credential []byte
}

// commit writes private atomic files. The injected writer is only a boundary
// for deterministic write-failure tests; rollback always uses the real writer.
func (s *Store) commitBrowser(change browserCommit, write func(string, []byte, os.FileMode) error) error {
	path := s.CredentialPath(change.account.ID)
	original, mode, existed, err := readOptional(path)
	if err != nil {
		return errors.New("claude: cannot snapshot browser account credential")
	}
	dir := s.AccountDir(change.account.ID)
	info, dirErr := os.Stat(dir)
	if dirErr != nil && !errors.Is(dirErr, os.ErrNotExist) {
		return errors.New("claude: cannot inspect browser account directory")
	}
	if dirErr == nil {
		if !info.IsDir() || platform.VerifyOwnerOnly(dir, storeDirPerm) != nil {
			return errors.New("claude: browser account directory is not private")
		}
	}
	if existed && platform.VerifyOwnerOnly(path, storeFilePerm) != nil {
		return errors.New("claude: browser account credential is not private")
	}
	managed, err := ManagedDocument(change.credential)
	if err != nil {
		return err
	}
	index, err := marshalIndex(change.index)
	if err != nil {
		return errors.New("claude: cannot encode browser account index")
	}
	if err := rejectTokenKeys(index); err != nil {
		return err
	}
	if err := platform.EnsurePrivateDirectory(dir); err != nil {
		if errors.Is(dirErr, os.ErrNotExist) {
			if cleanup := os.Remove(dir); cleanup != nil && !errors.Is(cleanup, os.ErrNotExist) {
				return errors.Join(errors.New("claude: cannot prepare browser account directory"), ErrLoginRollback)
			}
		}
		return errors.New("claude: cannot prepare browser account directory")
	}
	if err = write(path, managed, storeFilePerm); err == nil {
		err = write(s.IndexPath(), index, storeFilePerm)
	}
	if err == nil {
		return nil
	}
	// Atomic writes leave the original index untouched on error. Restore only
	// the credential, and remove a newly created directory only when empty.
	var rollback error
	if existed {
		rollback = platform.WriteFileAtomic(path, original, mode)
	} else {
		rollback = os.Remove(path)
		if errors.Is(rollback, os.ErrNotExist) {
			rollback = nil
		}
	}
	if errors.Is(dirErr, os.ErrNotExist) {
		rollback = errors.Join(rollback, os.Remove(dir))
	}
	if rollback != nil {
		return errors.Join(errors.New("claude: browser account commit failed"), ErrLoginRollback)
	}
	return errors.New("claude: browser account commit failed; credential restored")
}

// browserOfficialCurrent reflects settings.json, rather than a possibly stale
// index marker. Store.mu is already held, so key reads use the locked helper.
func (s *Store) browserOfficialCurrent(idx AccountIndex, proof browserOfficialProof) (string, error) {
	if proof.api == nil {
		return proof.identity.StableID(), nil
	}
	for _, acc := range idx.Accounts {
		if !acc.IsAPI() || acc.API == nil || acc.API.BaseURL != proof.api.BaseURL {
			continue
		}
		key, err := s.loadAPIKey(acc.ID)
		if err != nil {
			return "", errors.New("claude: cannot verify active API account")
		}
		if key == proof.api.AuthToken {
			return acc.ID, nil
		}
	}
	// A valid relay not yet imported has no managed current account. Sync will
	// import it; do not label the retained subscription as current meanwhile.
	return "", nil
}
