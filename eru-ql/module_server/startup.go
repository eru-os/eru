package module_server

import (
	"context"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_server/handlers"
	"github.com/eru-os/eru/eru-ql/module_store"
)

func StartUp(ctx context.Context) (module_store.ModuleStoreI, error) {
	logs.WithContext(ctx).Debug("StartUp - Start")
	return module_store.LoadStore(ctx, handlers.StoreTableName, handlers.StoreTenantTableName)
}
