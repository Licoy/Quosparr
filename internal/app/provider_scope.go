package app

import "github.com/licoy/quosparr/internal/store"

// toolProviders are the providers with MCP, Skill, session and pricing
// pages, in store.Providers() order. Gemini has its accounts and quota only
// until those pages are wired for it.
func toolProviders() []store.Provider {
	var out []store.Provider
	for _, p := range store.Providers() {
		if p != store.ProviderGemini {
			out = append(out, p)
		}
	}
	return out
}
