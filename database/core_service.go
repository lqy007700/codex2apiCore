package database

import "strings"

// EnableCodexOnly preserves legacy data but refuses new non-Codex credentials.
func (db *DB) EnableCodexOnly() { db.codexOnly.Store(true) }

func IsNativeCodexCredentials(credentials map[string]interface{}) bool {
	upstream, _ := credentials["upstream_type"].(string)
	upstream = strings.ToLower(strings.TrimSpace(upstream))
	return upstream == "" || upstream == "codex"
}
