package tools

import (
	"sync"

	eru_models "github.com/eru-os/eru/eru-models"
)

type ToolCatalogEntry struct {
	ToolType     string                `json:"tool_type"`
	Public       bool                  `json:"public"`
	Category     string                `json:"category"`
	Description  string                `json:"description"`
	Actions      []ActionInfo          `json:"actions"`
	OAuthEnabled bool                  `json:"oauth_enabled"`
	Icon         string                `json:"icon"`
	IconType     string                `json:"icon_type"`
	ToolSchema   eru_models.JSONSchema `json:"tool_schema"`
}

var (
	catalogMu      sync.RWMutex
	catalogEntries []ToolCatalogEntry
)

func RegisterToolCatalog(entry ToolCatalogEntry) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	catalogEntries = append(catalogEntries, entry)
}

func GetToolCatalog() []ToolCatalogEntry {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	result := make([]ToolCatalogEntry, len(catalogEntries))
	copy(result, catalogEntries)
	return result
}
