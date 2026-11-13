package gemini

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreMCPUsesSharedConfigAndBacksUpBesideLibrary(t *testing.T) {
	home := t.TempDir()
	runwayHome := filepath.Join(home, ".quosparr")
	st := NewStore(runwayHome, home)
	path := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if st.MCPConfigPath() != path || st.GeminiHome() != filepath.Join(home, ".gemini") {
		t.Fatalf("paths = %q, %q", st.MCPConfigPath(), st.GeminiHome())
	}
	writeFile(t, path, []byte(`{"mcpServers":{"alpha":{"command":"tool"}}}`))
	snap, err := st.LoadMCP(context.Background())
	if err != nil || len(snap.Servers) != 1 || !snap.Servers[0].Toggleable || !snap.Servers[0].Enabled {
		t.Fatalf("load = %+v, %v", snap, err)
	}
	if err := st.SetMCPEnabled(context.Background(), snap.Servers[0], false, time.Now()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"disabled": true`) {
		t.Fatalf("config = %s, %v", data, err)
	}
	backups, err := os.ReadDir(filepath.Join(runwayHome, "backups"))
	if err != nil || len(backups) == 0 {
		t.Fatalf("backups = %v, %v", backups, err)
	}
	if _, err := NewStoreAt(t.TempDir(), nil).LoadMCP(context.Background()); !errors.Is(err, errMCPHomeUnknown) {
		t.Fatalf("store without home = %v", err)
	}
}
