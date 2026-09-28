package store

import (
	"context"
	"sync/atomic"
)

type configChangeKey struct{}

type ConfigChange struct {
	changed atomic.Bool
}

func (c *ConfigChange) Mark() {
	if c != nil {
		c.changed.Store(true)
	}
}

func (c *ConfigChange) Changed() bool {
	return c != nil && c.changed.Load()
}

func ContextWithConfigChange(ctx context.Context, change *ConfigChange) context.Context {
	return context.WithValue(ctx, configChangeKey{}, change)
}

func ConfigChangeFromContext(ctx context.Context) *ConfigChange {
	change, _ := ctx.Value(configChangeKey{}).(*ConfigChange)
	return change
}

func MarkConfigChanged(ctx context.Context) {
	ConfigChangeFromContext(ctx).Mark()
}
