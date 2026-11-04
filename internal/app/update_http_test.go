package app

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/store"
)

// proxyAt starts an HTTP proxy that counts the requests it is asked to
// forward and refuses them, and returns the configuration that uses it.
func proxyAt(t *testing.T) (store.NetworkProxyConfiguration, *atomic.Int32) {
	t.Helper()
	var seen atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return store.NetworkProxyConfiguration{Mode: store.ProxyModeHTTP, Host: host, Port: n}, &seen
}

// readAll returns the reader of a fetched body: its text, or the test fails.
func readAll(t *testing.T) func(io.ReadCloser, error) string {
	return func(body io.ReadCloser, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()
		b, _ := io.ReadAll(body)
		return string(b)
	}
}

func TestUpdateFeedFollowsTheChannelOfTheVersion(t *testing.T) {
	for version, want := range map[string]string{
		"1.11.1":        "https://github.com/Licoy/Quosparr/releases/latest/download/update-darwin-arm64.json",
		"1.12.0-beta.1": "https://github.com/Licoy/Quosparr/releases/download/v1-beta/update-darwin-arm64.json",
	} {
		if got := updateFeed(version, "darwin", "arm64"); got != want {
			t.Errorf("updateFeed(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestUpdateSourceFetchesTheFeedAndItsFiles(t *testing.T) {
	var agents []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents = append(agents, r.UserAgent())
		switch r.URL.Path {
		case "/update.json":
			_, _ = io.WriteString(w, "manifest")
		case "/app.tar.gz":
			_, _ = io.WriteString(w, "archive")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	src := newUpdateSource(srv.URL+"/update.json", func(context.Context) (*http.Client, error) { return srv.Client(), nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if got := readAll(t)(src.Manifest(ctx, false)); got != "manifest" {
		t.Errorf("manifest %q", got)
	}
	if got := readAll(t)(src.Open(ctx, srv.URL+"/app.tar.gz")); got != "archive" {
		t.Errorf("archive %q", got)
	}
	if _, err := src.Open(ctx, srv.URL+"/missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing file: err = %v", err)
	}
	for _, a := range agents {
		if a != "mygo-updater" {
			t.Errorf("User-Agent %q", a)
		}
	}
	if _, err := src.Open(ctx, "http://updates.example.invalid/app.tar.gz"); err == nil {
		t.Error("a plain HTTP download was allowed")
	}
}

func TestUpdateSourceSendsACycleThroughTheNetworkContext(t *testing.T) {
	cfg, seen := proxyAt(t)
	c, err := netx.NewContext(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	src := newUpdateSource("https://updates.example.invalid/update.json", netxUpdateClient)
	// The request context carries the cycle's network context, like the
	// update cycle's own context does after a settings change.
	ctx, cancel := context.WithCancel(netx.WithContext(context.Background(), c))
	defer cancel()
	// "check" and "install" of one cycle: same ctx, same proxy.
	if _, err := src.Manifest(ctx, false); err == nil {
		t.Error("the refusing proxy let the manifest through")
	}
	if _, err := src.Open(ctx, "https://updates.example.invalid/app.tar.gz"); err == nil {
		t.Error("the refusing proxy let the archive through")
	}
	if seen.Load() != 2 {
		t.Errorf("proxy saw %d requests, want 2", seen.Load())
	}
}

func TestUpdateSourceUsesNetxForAContextThatIsNoCycle(t *testing.T) {
	var asked atomic.Int32
	src := newUpdateSource("https://x.invalid/update.json", func(ctx context.Context) (*http.Client, error) {
		asked.Add(1)
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: http.Header{}}, nil
		})}, nil
	})
	for range 2 {
		readAll(t)(src.Manifest(context.Background(), false)) // Background: never ends
	}
	if asked.Load() != 2 {
		t.Errorf("provider asked %d times, want once per request", asked.Load())
	}
}

func TestUpdateSourceFailsInsteadOfConnectingDirectly(t *testing.T) {
	var reached atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	defer srv.Close()
	boom := errors.New("proxy settings are broken")
	src := newUpdateSource(srv.URL, func(context.Context) (*http.Client, error) { return nil, boom })
	if _, err := src.Manifest(context.Background(), false); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the provider's error", err)
	}
	// A client without a transport must not mean the default one.
	src = newUpdateSource(srv.URL, func(context.Context) (*http.Client, error) { return &http.Client{}, nil })
	if _, err := src.Manifest(context.Background(), false); !errors.Is(err, ErrUpdateNoTransport) {
		t.Errorf("err = %v, want ErrUpdateNoTransport", err)
	}
	if reached.Load() != 0 {
		t.Error("a request went out directly")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestNoBusinessCodeUsesTheGlobalHTTPClient: every request in the app takes
// an explicit client from internal/netx.
func TestNoBusinessCodeUsesTheGlobalHTTPClient(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..") // internal/
	banned := map[string]bool{
		"DefaultClient": true, "DefaultTransport": true,
		"Get": true, "Post": true, "Head": true, "PostForm": true,
	}
	// Known exceptions, to be removed: the fallback of a nil client in the
	// pricing fetcher (the app always passes a netx client).
	allowed := map[string]bool{}

	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if allowed[rel] {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		name := ""
		for _, imp := range f.Imports {
			if imp.Path.Value == `"net/http"` {
				name = "http"
				if imp.Name != nil {
					name = imp.Name.Name
				}
			}
		}
		if name == "" {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == name && banned[sel.Sel.Name] {
					found = append(found, rel+": http."+sel.Sel.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Errorf("business code uses the global HTTP client (use internal/netx):\n%s", strings.Join(found, "\n"))
	}
}
