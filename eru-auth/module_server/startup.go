package module_server

import (
	"context"
	"os"

	"github.com/eru-os/eru/eru-auth/module_server/handlers"
	"github.com/eru-os/eru/eru-auth/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

func StartUp(ctx context.Context) (module_store.ModuleStoreI, error) {
	erufuncbaseurl := os.Getenv("ERUFUNCTIONS_BASEURL")
	if erufuncbaseurl == "" {
		erufuncbaseurl = "http://localhost:8083"
		logs.WithContext(ctx).Info("'ERUFUNCTIONS_BASEURL' environment variable not found - setting default value as http://localhost:8083")
	}
	module_store.Erufuncbaseurl = erufuncbaseurl

	return module_store.LoadStore(ctx, handlers.StoreTableName, handlers.StoreTenantTableName)
}
