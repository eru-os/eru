package file_server

import (
	"context"

	handlers "github.com/eru-os/eru/eru-files/file_server/handlers"
	"github.com/eru-os/eru/eru-files/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

func StartUp(ctx context.Context) (module_store.ModuleStoreI, error) {
	logs.WithContext(ctx).Debug("StartUp - Start")
	return module_store.LoadStore(ctx, handlers.StoreTableName, handlers.StoreTenantTableName)
}
