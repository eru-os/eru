package module_server

import (
	"context"

	"github.com/eru-os/eru/eru-html-image/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

func StartUp(ctx context.Context) (module_store.ModuleStoreI, error) {
	logs.WithContext(ctx).Debug("StartUp - Start")
	return new(module_store.ModuleStore), nil
}
