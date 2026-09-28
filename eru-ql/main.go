package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eruotel "github.com/eru-os/eru/eru-logs/eru-otel"
	"github.com/eru-os/eru/eru-ql/module_server"
	"github.com/eru-os/eru/eru-ql/module_store"
	"github.com/eru-os/eru/eru-ql/qlcache"
	"github.com/eru-os/eru/eru-server/server"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
)

var port = "8087"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize service
	module_server.SetServiceName()
	server_handlers.SetInstanceId()
	logs.LogInit(server_handlers.ServerName, server_handlers.InstanceId)
	logs.WithContext(ctx).Info(fmt.Sprintf("inside main of %s-%s", server_handlers.ServerName, server_handlers.InstanceId))

	server_handlers.BaseUrl = os.Getenv("ERUQL_PUB_BASE_URL")

	// Setup OpenTelemetry tracing
	var tp interface{ Shutdown(context.Context) error }

	// Production-ready panic handler (normal defer will handle cleanup)
	defer func() {
		if r := recover(); r != nil {
			logs.Logger.Error(fmt.Sprint("FATAL: Main goroutine panic: ", r, " : ", string(debug.Stack())))
			// Exit with specific code for orchestrator (2 = panic, 1 = error, 0 = normal)
			os.Exit(2)
		}
	}()

	// Common cleanup function for all exit scenarios
	cleanup := func() {
		if tp != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := tp.Shutdown(shutdownCtx); err != nil {
				logs.Logger.Error(fmt.Sprint("Error shutting down tracer provider: ", err.Error()))
			}
		}
	}
	defer cleanup() // Handles cleanup for both normal exit and panic recovery

	traceUrl := os.Getenv("TRACE_URL")
	if traceUrl != "" {
		var err error
		tp, err = eruotel.TracerTempoInit(traceUrl)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
	}

	// Configure port
	envPort := os.Getenv("ERUQLPORT")
	if envPort != "" {
		port = envPort
	}

	// Initialize store and server
	store, e := module_server.StartUp(ctx)
	if e != nil {
		logs.WithContext(ctx).Error(e.Error())
		logs.WithContext(ctx).Error("Failed to Start Server - error while setting up config store")
		return
	}

	sh := new(module_store.StoreHolder)
	sh.Store = store
	sr, _, e := server.Init(ctx, sh.Store)
	if e != nil {
		logs.WithContext(ctx).Error(e.Error())
		return
	}

	// Setup data source connections
	logs.WithContext(ctx).Info("calling SetDataSourceConnections")
	err := sh.Store.SetDataSourceConnections(ctx, sh.Store)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}

	qlcache.StartInvalidator(ctx, 0)

	// Setup routes
	module_server.AddModuleRoutes(sr, sh)

	// Let eru-server handle everything including signals, contexts, and graceful shutdown
	server.LaunchWithContext(ctx, sr, port, sh.Store)
}
