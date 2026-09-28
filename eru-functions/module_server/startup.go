package module_server

import (
	"context"
	"os"
	"strconv"

	handlers "github.com/eru-os/eru/eru-functions/module_server/handlers"
	"github.com/eru-os/eru/eru-functions/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

func StartUp(ctx context.Context) (module_store.ModuleStoreI, error) {
	var err error

	eruqlbaseurl := os.Getenv("ERUQL_BASEURL")
	if eruqlbaseurl == "" {
		eruqlbaseurl = "http://localhost:8087"
		logs.WithContext(ctx).Info("'eruqlbaseurl' environment variable not found - setting default value as http://localhost:8087")
	}
	module_store.Eruqlbaseurl = eruqlbaseurl

	eruaibaseurl := os.Getenv("ERUAI_BASEURL")
	if eruaibaseurl == "" {
		eruaibaseurl = "http://localhost:8088"
		logs.WithContext(ctx).Info("'eruaibaseurl' environment variable not found - setting default value as http://localhost:8088")
	}
	module_store.Eruaibaseurl = eruaibaseurl

	funcThreads := os.Getenv("FUNC_THREADS")
	if funcThreads == "" {
		funcThreads = "3"
		logs.WithContext(ctx).Info("'FUNC_THREADS' environment variable not found - setting default value as 3")
	}
	module_store.FuncThreads, err = strconv.Atoi(funcThreads)
	if err != nil {
		err = nil
		logs.WithContext(ctx).Info("'FUNC_THREADS' environment variable is non numeric - setting default value as 3")
		module_store.FuncThreads = 3
	}

	loopThreads := os.Getenv("LOOP_THREADS")
	if loopThreads == "" {
		loopThreads = "3"
		logs.WithContext(ctx).Info("'LOOP_THREADS' environment variable not found - setting default value as 3")
	}
	module_store.LoopThreads, err = strconv.Atoi(loopThreads)
	if err != nil {
		err = nil
		logs.WithContext(ctx).Info("'LOOP_THREADS' environment variable is non numeric - setting default value as 3")
		module_store.LoopThreads = 3
	}

	eventThreads := os.Getenv("EVENT_THREADS")
	if eventThreads == "" {
		eventThreads = "3"
		logs.WithContext(ctx).Info("'EVENT_THREADS' environment variable not found - setting default value as 3")
	}
	module_store.EventThreads, err = strconv.Atoi(eventThreads)
	if err != nil {
		err = nil
		logs.WithContext(ctx).Info("'EVENT_THREADS' environment variable is non numeric - setting default value as 3")
		module_store.EventThreads = 3
	}

	return module_store.LoadStore(ctx, handlers.StoreTableName, handlers.StoreTenantTableName)
}
