package tools

import (
	"fmt"
	"sync"
)

var (
	factoryMu sync.RWMutex
	factory   = map[string]func() Tooling{}
)

func RegisterTool(toolType string, newTool func() Tooling) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	if _, exists := factory[toolType]; exists {
		panic(fmt.Sprintf("tool type %s registered twice", toolType))
	}
	factory[toolType] = newTool
}

func NewTool(toolType string) Tooling {
	factoryMu.RLock()
	newTool, ok := factory[toolType]
	factoryMu.RUnlock()
	if !ok {
		return new(Tool)
	}
	return newTool()
}
