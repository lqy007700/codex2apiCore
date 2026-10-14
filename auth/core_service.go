package auth

import (
	"context"
	"github.com/codex2api/security/promptfilter"
)

// EnableCodexOnly must run before Init; it is immutable for the process lifetime.
func (s *Store) EnableCodexOnly() { s.codexOnly.Store(true) }
func (s *Store) CodexOnly() bool  { return s != nil && s.codexOnly.Load() }

func (s *Store) loadCoreCompatibleBindings(ctx context.Context) error {
	if s.CodexOnly() {
		return nil
	}
	return s.LoadPromptFilterNewAPIBindings(ctx)
}

func corePromptConfig() promptfilter.Config {
	cfg := promptfilter.DefaultConfig()
	cfg.Enabled = false
	cfg.Review.Enabled = false
	cfg.LogMatches = false
	cfg.Advanced = promptfilter.AdvancedConfig{}
	return cfg
}
