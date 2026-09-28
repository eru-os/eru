package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/eru-os/eru/eru-cache/cache"
	"github.com/eru-os/eru/eru-gateway/config_sync"
	"github.com/eru-os/eru/eru-gateway/module_server"
	"github.com/eru-os/eru/eru-gateway/module_server/handlers"
	"github.com/eru-os/eru/eru-gateway/module_store"
	"github.com/eru-os/eru/eru-gateway/registry"
	"github.com/eru-os/eru/eru-gateway/user_events"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eruotel "github.com/eru-os/eru/eru-logs/eru-otel"
	"github.com/eru-os/eru/eru-server/server"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
)

var port = "8086"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize service
	module_server.SetServiceName()
	server_handlers.SetInstanceId()
	logs.LogInit(server_handlers.ServerName, server_handlers.InstanceId)
	logs.WithContext(ctx).Info(fmt.Sprintf("inside main of %s-%s", server_handlers.ServerName, server_handlers.InstanceId))

	server_handlers.BaseUrl = os.Getenv("ERUGATEWAY_PUB_BASE_URL")

	// Setup OpenTelemetry tracing
	var tp interface{ Shutdown(context.Context) error }
	var userEventLogger *user_events.Logger

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
		userEventLogger.Close(context.Background())
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
	envPort := os.Getenv("ERUGATEWAYPORT")
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

	userEventLogger, e = user_events.New(ctx, sh.Store)
	if e != nil {
		logs.WithContext(ctx).Error(e.Error())
		logs.WithContext(ctx).Error("Failed to Start Server - error while setting up user event logger")
		return
	}
	handlers.SetUserEventLogger(userEventLogger)

	// Create the service registry
	registryType := os.Getenv("REGISTRY_TYPE")
	if registryType == "" {
		registryType = "INMEMORY" // Default to in-memory if not specified or invalid
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Using %s registry", registryType))

	registryCache := cache.GetCacheStore(registryType, "")
	logs.WithContext(ctx).Info(fmt.Sprintf("registryCache: %v", registryCache))
	if registryCache == nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("failed to connect to registry cache: %v - fallback to in-memory", registryType))
		registryCache = cache.GetCacheStore("INMEMORY", "")
	}
	serviceRegistry := registry.NewRegistry(registryCache, 90*time.Second)
	rh := &handlers.RegistryHandler{Registry: serviceRegistry}
	handlers.SetConfigSyncNotifier(config_sync.New(serviceRegistry))

	sr, _, e := server.Init(ctx, sh.Store)
	if e != nil {
		logs.WithContext(ctx).Error(e.Error())
		return
	}

	// Setup routes
	module_server.AddModuleRoutes(sr, sh, rh)

	// Let eru-server handle everything including signals, contexts, and graceful shutdown
	server.LaunchWithContext(ctx, sr, port, sh.Store)
}
