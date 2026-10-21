// Package cursor reads the current desktop login and its quota without changing it.
package cursor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/licoy/quosparr/internal/sqliteruntime"
)

var (
	ErrNotLoggedIn      = errors.New("cursor: no readable desktop login")
	ErrCredentials      = errors.New("cursor: credential store unavailable")
	ErrUnsupportedLogin = errors.New("cursor: unsupported desktop login storage or deployment")
	ErrUnauthorized     = errors.New("cursor: authentication expired")
	ErrResponse         = errors.New("cursor: unsupported usage response")
	ErrUnavailable      = errors.New("cursor: usage unavailable")
)

// Auth is transient; it must never be serialized, cached or logged.
type Auth struct {
	Token string `json:"-"`
	Email string `json:"-"`
	Plan  string `json:"-"`
}

func (Auth) String() string   { return "cursor credentials (redacted)" }
func (Auth) GoString() string { return "cursor credentials (redacted)" }

// ID identifies the signed-in account, so routine access-token rotation keeps
// it stable. Without a cached email the token is the only identity available.
func (a Auth) ID() string {
	key := "email:" + a.Email
	if a.Email == "" {
		key = "token:" + a.Token
	}
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// UserDataDir resolves the documented desktop defaults and inherited overrides.
// A running editor's --user-data-dir is deliberately not discovered by scanning processes.
func UserDataDir(goos, home string, getenv func(string) string) string {
	if p := getenv("VSCODE_PORTABLE"); filepath.IsAbs(p) {
		return filepath.Join(p, "user-data")
	}
	if p := getenv("VSCODE_APPDATA"); filepath.IsAbs(p) {
		return filepath.Join(p, "Cursor")
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor")
	case "windows":
		p := getenv("APPDATA")
		if !filepath.IsAbs(p) {
			p = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(p, "Cursor")
	default:
		p := getenv("XDG_CONFIG_HOME")
		if !filepath.IsAbs(p) {
			p = filepath.Join(home, ".config")
		}
		return filepath.Join(p, "Cursor")
	}
}

// ReadAuth only selects the required keys. SQLite remains read-only and observes WAL.
// Missing databases are never created; encrypted tokens and isolated deployments
// cannot be sent to the production service by this integration.
func ReadAuth(ctx context.Context, userDataDir string) (Auth, error) {
	path := filepath.Join(userDataDir, "User", "globalStorage", "state.vscdb")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Auth{}, ErrNotLoggedIn
	}
	if err != nil || !info.Mode().IsRegular() {
		return Auth{}, ErrCredentials
	}
	if err := sqliteruntime.Load(); err != nil {
		return Auth{}, ErrCredentials
	}
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{ReadOnly: true, BusyTimeout: time.Second})
	if err != nil {
		return Auth{}, ErrCredentials
	}
	defer db.Close()
	rows, err := db.Query(ctx, `SELECT key, CASE WHEN length(value) <= 16384 THEN value ELSE NULL END FROM ItemTable WHERE key IN (?, ?, ?, ?)`,
		"cursorAuth/accessToken", "cursorAuth/cachedEmail", "cursorAuth/stripeMembershipType", "cursorAuth/activeDeploymentId")
	if err != nil {
		return Auth{}, ErrCredentials
	}
	values := make(map[string]string, 4)
	for _, row := range rows.Values {
		if len(row) != 2 {
			return Auth{}, ErrCredentials
		}
		key, ok := row[0].(string)
		if !ok {
			return Auth{}, ErrCredentials
		}
		switch v := row[1].(type) {
		case string:
			values[key] = v
		case []byte:
			values[key] = string(v)
		default:
			return Auth{}, ErrCredentials
		}
	}
	if deployment := values["cursorAuth/activeDeploymentId"]; deployment != "" && deployment != "0" {
		return Auth{}, ErrUnsupportedLogin
	}
	a := Auth{Token: values["cursorAuth/accessToken"], Email: cleanLabel(values["cursorAuth/cachedEmail"]), Plan: cleanLabel(values["cursorAuth/stripeMembershipType"])}
	if a.Token == "" {
		return Auth{}, ErrNotLoggedIn
	}
	if strings.HasPrefix(a.Token, "cursor-safe-storage:") {
		return Auth{}, ErrUnsupportedLogin
	}
	if !validToken(a.Token) {
		return Auth{}, ErrCredentials
	}
	return a, nil
}

func validToken(s string) bool {
	if s == "" || len(s) > 16384 {
		return false
	}
	for _, c := range s {
		if c <= 32 || c >= 127 {
			return false
		}
	}
	return true
}

func cleanLabel(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > 120 {
		return ""
	}
	for _, c := range r {
		if c < 32 || c == 127 {
			return ""
		}
	}
	return string(r)
}
