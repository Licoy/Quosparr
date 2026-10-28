package claude

import (
	"context"
	"github.com/licoy/quosparr/internal/mcp"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPAddUsesIsolatedStoreSource(t *testing.T) {
	home := t.TempDir()
	store := NewStore(filepath.Join(home, "official"), filepath.Join(home, "app"), home, filepath.Join(home, "official"), "")
	ctx := context.Background()
	plan, err := store.PrepareMCPAdd(ctx, []mcp.Draft{{Name: "user-server", Transport: "stdio", Command: "server"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.AddMCP(ctx, plan, time.Now())
	if err != nil || len(result.Added) != 1 {
		t.Fatal("add failed", err)
	}
	snapshot, err := store.LoadMCP(ctx)
	if err != nil || len(snapshot.Servers) != 1 || snapshot.Servers[0].Name != "user-server" {
		t.Fatal("wrong target", err)
	}
	plan, err = store.PrepareMCPAdd(ctx, []mcp.Draft{{Name: "user-server", Transport: "stdio", Command: "server"}})
	if err != nil || len(plan.Items) != 1 || plan.Items[0].Status != "exists" {
		t.Fatal("existing config not detected", err)
	}
}
