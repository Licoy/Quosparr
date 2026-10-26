package claude

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/licoy/quosparr/internal/platform"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
)

type errorCommandRunner struct {
	code int
	err  error
}

func (r errorCommandRunner) Run(context.Context, string, []string, []byte) ([]byte, int, error) {
	return nil, r.code, r.err
}

func TestKeychainFailurePreservesSafeReason(t *testing.T) {
	cause := &os.PathError{Op: "read", Path: "/private/token-secret", Err: os.ErrPermission}
	_, err := (KeychainStore{Runner: errorCommandRunner{err: cause}}).Load(context.Background())
	if !errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "token-secret") {
		t.Fatalf("unsafe or lost reason: %v", err)
	}
}
func TestBrowserImportPreservesOfficialTimeout(t *testing.T) {
	r := newSyncRig(t)
	r.store.Official = KeychainStore{Runner: errorCommandRunner{err: context.DeadlineExceeded}}
	_, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("new", "browser-new"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout lost: %v", err)
	}
}

func requireOperation(t *testing.T, err error, stage ErrorStage, kind ErrorKind) *OperationError {
	t.Helper()
	var detail *OperationError
	if !errors.As(err, &detail) || detail.Stage != stage || detail.Kind != kind {
		t.Fatalf("error classification = %v, want %s/%s", err, stage, kind)
	}
	for _, secret := range []string{"token-secret", "private-secret", "secret-in-error"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error contains private text")
		}
	}
	return detail
}
func TestOperationErrorIOClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		kind ErrorKind
	}{{os.ErrPermission, KindPermission}, {syscall.EROFS, KindReadOnly}, {syscall.ENOSPC, KindDiskFull}, {context.DeadlineExceeded, KindTimeout}, {context.Canceled, KindCanceled}, {errors.New("private-secret"), KindUnknown}} {
		cause := &os.PathError{Op: "write", Path: "/private-secret", Err: tc.err}
		got := operationError(StageWriteIndex, cause)
		requireOperation(t, got, StageWriteIndex, tc.kind)
		if !errors.Is(got, tc.err) {
			t.Fatal("cause lost")
		}
	}
}
func TestOperationErrorIndexFailures(t *testing.T) {
	r := newSyncRig(t)
	if err := r.store.ensureRoot(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		data string
		kind ErrorKind
	}{{`{"private-secret":`, KindInvalidData}, {`{"version":999}`, KindUnsupportedVersion}} {
		if err := os.WriteFile(r.store.IndexPath(), []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := r.store.LoadIndex()
		requireOperation(t, err, StageReadIndex, tc.kind)
	}
}
func TestBrowserCommitFailureStages(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		r := newSyncRig(t)
		result := browserResult("new", "browser-new")
		idx := NewAccountIndex()
		doc, _ := ParseAuthDocument(result.Credential)
		acc := browserAccount(idx, result.Identity.withCredential(doc), doc, r.now)
		idx.Upsert(acc)
		calls := 0
		cause := &os.PathError{Op: "write", Path: "/private-secret", Err: syscall.ENOSPC}
		err := r.store.commitBrowser(browserCommit{acc, idx, result.Credential}, func(path string, data []byte, mode os.FileMode) error {
			calls++
			if calls == failAt {
				return cause
			}
			return platform.WriteFileAtomic(path, data, mode)
		})
		stage := StageWriteCredential
		if failAt == 2 {
			stage = StageWriteIndex
		}
		requireOperation(t, err, stage, KindDiskFull)
		if !errors.Is(err, syscall.ENOSPC) {
			t.Fatal("write cause lost")
		}
	}
}
func TestOAuthHTTPAndTransportReasons(t *testing.T) {
	for _, status := range []int{401, 403, 429, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("private-secret"))
		}))
		_, err := loginRequest(context.Background(), server.Client(), http.MethodGet, server.URL+"?private-secret", nil, "")
		detail := requireOperation(t, err, StageOAuthExchange, KindHTTP)
		if detail.HTTPStatus != status || !errors.Is(err, ErrLoginExchange) {
			t.Fatal("HTTP status or sentinel lost")
		}
		server.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := loginRequest(ctx, http.DefaultClient, http.MethodGet, "http://example.invalid/?private-secret", nil, "")
	requireOperation(t, err, StageOAuthExchange, KindCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancel lost")
	}
}
func TestKeychainExitAndDeadlineReasons(t *testing.T) {
	_, err := (KeychainStore{Runner: errorCommandRunner{code: 51}}).Load(context.Background())
	detail := requireOperation(t, err, StageReadKeychain, KindCommand)
	if detail.ExitCode != 51 {
		t.Fatal("exit code lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (KeychainStore{Runner: errorCommandRunner{code: -1}}).Load(ctx)
	requireOperation(t, err, StageReadKeychain, KindCanceled)
}

type deadlineCommandRunner struct{}

func (deadlineCommandRunner) Run(ctx context.Context, _ string, _ []string, _ []byte) ([]byte, int, error) {
	<-ctx.Done()
	return nil, -1, nil
}
func TestKeychainOwnTimeoutWinsExitStatus(t *testing.T) {
	_, err := (KeychainStore{Runner: deadlineCommandRunner{}}).Load(context.Background())
	requireOperation(t, err, StageReadKeychain, KindTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline cause lost")
	}
}

type rollbackErrorRunner struct {
	calls                 int
	writeErr, rollbackErr error
}

func (r *rollbackErrorRunner) Run(context.Context, string, []string, []byte) ([]byte, int, error) {
	r.calls++
	switch r.calls {
	case 1:
		return nil, 44, nil
	case 2:
		return nil, 0, r.writeErr
	default:
		return nil, 0, r.rollbackErr
	}
}
func TestKeychainWriteRetainsRollbackFailure(t *testing.T) {
	writeErr := &os.PathError{Op: "write", Path: "/token-secret", Err: syscall.ENOSPC}
	rollbackErr := errors.New("private-secret")
	runner := &rollbackErrorRunner{writeErr: writeErr, rollbackErr: rollbackErr}
	err := (KeychainStore{Service: "test", Account: "test", Runner: runner}).Save(context.Background(), []byte(`{"claudeAiOauth":{"accessToken":"test-access-token"}}`))
	detail := requireOperation(t, err, StageRollbackKeychain, KindRollbackFailed)
	if detail.Failure == nil || detail.Failure.Stage != StageWriteKeychain || detail.Failure.Kind != KindDiskFull || detail.Rollback == nil || detail.Rollback.Stage != StageRollbackKeychain {
		t.Fatal("write and rollback summaries missing")
	}
	if !errors.Is(err, writeErr) || !errors.Is(err, rollbackErr) {
		t.Fatal("write or rollback cause lost")
	}
}

func TestBrowserInvalidSettingsAndIdentityStages(t *testing.T) {
	t.Run("settings", func(t *testing.T) {
		r := newSyncRig(t)
		r.login(t, "test-access-token", "official")
		if err := os.WriteFile(r.store.SettingsPath(), []byte(`{"env":"private-secret"}`), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := r.mod.browserOfficialProof(context.Background())
		requireOperation(t, err, StageReadSettings, KindInvalidData)
	})
	t.Run("oauthAccount", func(t *testing.T) {
		r := newSyncRig(t)
		r.login(t, "test-access-token", "official")
		if err := os.WriteFile(GlobalConfigPath(r.config, r.home), []byte(`{"oauthAccount":"private-secret"}`), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := r.mod.browserOfficialProof(context.Background())
		requireOperation(t, err, StageReadIdentity, KindInvalidData)
		if !errors.Is(err, ErrIdentityRequired) || !errors.Is(err, ErrAuthInvalidRoot) {
			t.Fatal("identity sentinel lost")
		}
	})
	for _, payload := range []string{`{"private-secret":`, `{"account":{"uuid":"official"}}`} {
		t.Run("profile", func(t *testing.T) {
			r := newSyncRig(t)
			r.login(t, "test-access-token", "")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(payload)) }))
			defer server.Close()
			r.mod.Usage = &UsageClient{HTTP: server.Client(), ProfileEndpoint: server.URL}
			_, err := r.mod.browserOfficialProof(context.Background())
			requireOperation(t, err, StageReadIdentity, KindInvalidData)
			if !errors.Is(err, ErrIdentityRequired) {
				t.Fatal("identity sentinel lost")
			}
		})
	}
}

type payloadCommandRunner struct{ payload []byte }

func (r payloadCommandRunner) Run(context.Context, string, []string, []byte) ([]byte, int, error) {
	return r.payload, 0, nil
}
func TestKeychainInvalidPayloadStage(t *testing.T) {
	for _, payload := range []string{"", "  \n", "private-secret", `{"private-secret":`, "7b22707269766174652d73656372657422"} {
		_, err := (KeychainStore{Runner: payloadCommandRunner{[]byte(payload)}}).Load(context.Background())
		requireOperation(t, err, StageReadKeychain, KindInvalidData)
	}
}

func TestKeychainPayloadDecodingPreservesValidFormats(t *testing.T) {
	payload := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token"}}`)
	for _, encoded := range [][]byte{payload, []byte(hex.EncodeToString(payload))} {
		data, err := (KeychainStore{Runner: payloadCommandRunner{encoded}}).Load(context.Background())
		if err != nil || string(data) != string(payload) {
			t.Fatal("valid keychain payload changed")
		}
	}
}

type rollbackReadTimeoutRunner struct{ calls int }

func (r *rollbackReadTimeoutRunner) Run(context.Context, string, []string, []byte) ([]byte, int, error) {
	r.calls++
	switch r.calls {
	case 1:
		return []byte(`{"claudeAiOauth":{"accessToken":"test-old"}}`), 0, nil
	case 2:
		return nil, 0, syscall.ENOSPC
	case 3:
		return nil, 0, nil
	default:
		return nil, 0, context.DeadlineExceeded
	}
}
func TestKeychainRollbackSummaryPreservesReadTimeout(t *testing.T) {
	err := (KeychainStore{Service: "test", Account: "test", Runner: &rollbackReadTimeoutRunner{}}).Save(context.Background(), []byte(`{"claudeAiOauth":{"accessToken":"test-new"}}`))
	detail := requireOperation(t, err, StageRollbackKeychain, KindRollbackFailed)
	if detail.Failure == nil || detail.Failure.Kind != KindDiskFull || detail.Rollback == nil || detail.Rollback.Kind != KindTimeout {
		t.Fatal("recovery cause was masked")
	}
	if !errors.Is(err, syscall.ENOSPC) || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrKeychainVerify) {
		t.Fatal("cause or verification sentinel lost")
	}
}

func TestProfileHTTPFailureUsesIdentityStage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("private-secret"))
	}))
	defer server.Close()
	client := &UsageClient{HTTP: server.Client(), ProfileEndpoint: server.URL}
	_, _, err := client.FetchProfile(context.Background(), "test-access-token")
	detail := requireOperation(t, err, StageReadIdentity, KindHTTP)
	if detail.HTTPStatus != 503 || !errors.Is(err, ErrUsageStatus) {
		t.Fatal("profile HTTP status or sentinel lost")
	}
}
