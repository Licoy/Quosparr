package app

import (
	"context"

	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
)

// MCPActions are the settings MCP-management operations. A nil function
// leaves the corresponding control unavailable.
type MCPActions struct {
	Query      func(context.Context, store.Provider) (mcp.Snapshot, error)
	Delete     func(context.Context, store.Provider, []mcp.Server) ([]mcp.DeleteResult, error)
	SetEnabled func(context.Context, store.Provider, mcp.Server, bool) error
	Reveal     func(string) error
	Parse      func([]byte) ([]mcp.Draft, error)
	Probe      func(context.Context, mcp.Draft) (mcp.ProbeResult, error)
	PrepareAdd func(context.Context, store.Provider, []mcp.Draft) (mcp.AddPlan, error)
	Add        func(context.Context, store.Provider, mcp.AddPlan) (mcp.AddResult, error)
}
