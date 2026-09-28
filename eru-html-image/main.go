package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/eru-os/eru/eru-html-image/html_image"
	"github.com/eru-os/eru/eru-html-image/module_server"
	"github.com/eru-os/eru/eru-html-image/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eruotel "github.com/eru-os/eru/eru-logs/eru-otel"
	"github.com/eru-os/eru/eru-server/server"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
)

var port = "8089"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	module_server.SetServiceName()
	server_handlers.SetInstanceId()
	logs.LogInit(server_handlers.ServerName, server_handlers.InstanceId)
	logs.WithContext(ctx).Info(fmt.Sprintf("inside main of %s-%s", server_handlers.ServerName, server_handlers.InstanceId))

	server_handlers.BaseUrl = os.Getenv("ERUHTMLIMAGE_PUB_BASE_URL")

	var tp interface{ Shutdown(context.Context) error }

	defer func() {
		if r := recover(); r != nil {
			logs.Logger.Error(fmt.Sprint("FATAL: Main goroutine panic: ", r, " : ", string(debug.Stack())))
			os.Exit(2)
		}
	}()

	cleanup := func() {
		if tp != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := tp.Shutdown(shutdownCtx); err != nil {
				logs.Logger.Error(fmt.Sprint("Error shutting down tracer provider: ", err.Error()))
			}
		}
	}
	defer cleanup()

	traceUrl := os.Getenv("TRACE_URL")
	if traceUrl != "" {
		var err error
		tp, err = eruotel.TracerTempoInit(traceUrl)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
	}

	envPort := os.Getenv("ERUHTMLIMAGEPORT")
	if envPort != "" {
		port = envPort
	}

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

	renderer, e := html_image.NewRenderer(ctx)
	if e != nil {
		logs.WithContext(ctx).Error(e.Error())
		logs.WithContext(ctx).Error("Failed to Start Server - error while starting chromium")
		return
	}
	defer renderer.Close()

	module_server.AddModuleRoutes(sr, sh, renderer)

	server.LaunchWithContext(ctx, sr, port, sh.Store)
}
