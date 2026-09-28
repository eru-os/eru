package auth

import (
	"fmt"
	"sync"
)

var (
	factoryMu sync.RWMutex
	factory   = map[string]func() AuthI{}
)

func RegisterAuth(authType string, newAuth func() AuthI) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	if _, exists := factory[authType]; exists {
		panic(fmt.Sprintf("auth type %s registered twice", authType))
	}
	factory[authType] = newAuth
}

func NewAuth(authType string) AuthI {
	factoryMu.RLock()
	newAuth, ok := factory[authType]
	factoryMu.RUnlock()
	if !ok {
		return new(Auth)
	}
	return newAuth()
}
