package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-server/registration"
	handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/eru-os/eru/eru-store/store"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Server struct {
	Store store.StoreI
}

func Launch(serverRouter *mux.Router, port string, store store.StoreI) {
	LaunchWithContext(context.Background(), serverRouter, port, store)
}

const DefaultHeartbeatInterval = 30 * time.Second

func LaunchWithContext(ctx context.Context, serverRouter *mux.Router, port string, store store.StoreI) {

	// Initialize logger with instance ID
	logs.LogInit(handlers.ServerName, handlers.InstanceId)

	// Get singleton goroutine manager
	gm := GetGlobalGoroutineManager(ctx)

	// Channel to listen for OS signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	var regClient *registration.RegistryClient
	var serverCleanupOnce sync.Once

	// Setup HTTP server with graceful shutdown
	server := &http.Server{
		Addr:    ":" + port,
		Handler: nil,
	}

	// Service Registration
	if handlers.ServerName != "eru-gateway" {
		registryURL := os.Getenv("ERUGATEWAY_URL")

		if registryURL != "" {
			var err error
			regClient, err = registration.NewRegistryClient(registryURL, handlers.ServerName, port, handlers.InstanceId)
			if err != nil {
				logs.Logger.Error(fmt.Sprintf("Failed to create registry client: %v", err))
				err = nil
			} else {
				err := regClient.Register(gm.Context())
				if err != nil {
					logs.Logger.Error(fmt.Sprintf("Failed to register service: %v", err))
					err = nil
				} else {
					// Start heartbeating with non-critical restart behavior - keep service alive even if heartbeat fails
					gm.SafeGoWithRestartBehavior("heartbeat", func(ctx context.Context) {
						interval := DefaultHeartbeatInterval
						if os.Getenv("HEARTBEAT_INTERVAL") != "" {
							//expected format: 30s, 1m, 1h, 1d
							//if not valid, use default 1h
							interval, err = time.ParseDuration(os.Getenv("HEARTBEAT_INTERVAL"))
							if err != nil {
								logs.Logger.Error(fmt.Sprintf("Failed to parse heartbeat interval: %v", err))
								err = nil
							}
						}
						err = regClient.StartHeartbeat(ctx, interval)
						if err != nil {
							logs.Logger.Error(fmt.Sprintf("Failed to start heartbeat: %v", err))
							err = nil
						}
					}, ContinueOnMaxRetries)
				}
			}
		} else {
			logs.Logger.Warn("ERUGATEWAY_URL not set. Skipping service registration.")
		}
	}

	// Setup CORS and middleware
	handlers.AllowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	logs.Logger.Info(fmt.Sprint("AllowedOrigins = ", handlers.AllowedOrigins))
	corsObj := handlers.MakeCorsObject()

	//r := otelhttp.NewHandler(corsObj.Handler(panicRecoveryMiddleware(concurrencyLimitMiddleware(contextCancellationMiddleware(requestIdMiddleWare(otelMiddleWare(configChangeMiddleware(serverRouter))))))), handlers.ServerName)
	r := otelhttp.NewHandler(corsObj.Handler(requestIdMiddleWare(panicRecoveryMiddleware(contextCancellationMiddleware(otelMiddleWare(configChangeMiddleware(serverRouter)))))), handlers.ServerName)
	http.Handle("/", r)

	// Start HTTP server with critical restart behavior - shutdown service if server fails
	gm.SafeGoWithRestartBehavior("http-server", func(ctx context.Context) {
		logs.Logger.Info(fmt.Sprint("Starting server ", handlers.ServerName, " on ", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			//if err := server.ListenAndServeTLS("localhost.pem", "localhost-key.pem"); err != nil && err != http.ErrServerClosed {
			logs.Logger.Error(fmt.Sprintf("HTTP server error: %v", err))
		}
	}, ShutdownOnMaxRetries)

	// Wait for shutdown signal
	select {
	case sig := <-stop:
		logs.Logger.Info(fmt.Sprintf("Received signal %v, initiating graceful shutdown", sig))
	case <-gm.Context().Done():
		logs.Logger.Info("Context cancelled, initiating graceful shutdown")
	}

	// Cleanup function
	cleanup := func() {
		// Shutdown HTTP server
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logs.Logger.Error(fmt.Sprintf("HTTP server shutdown error: %v", err))
		}

		// Deregister from gateway if applicable
		if regClient != nil {
			logs.Logger.Info("Attempting to deregister service")
			deregisterCtx, deregisterCancel := context.WithTimeout(ctx, 5*time.Second)
			defer deregisterCancel()

			if err := regClient.Deregister(deregisterCtx); err != nil {
				logs.Logger.Error(fmt.Sprintf("Deregistration error: %v", err))
			}
		}

		// Shutdown all goroutines
		if err := gm.Shutdown(15 * time.Second); err != nil {
			logs.Logger.Error(fmt.Sprintf("Goroutine shutdown error: %v", err))
		}

		// Reset singleton for potential restart
		ResetGlobalGoroutineManager()
	}

	serverCleanupOnce.Do(cleanup)
	logs.Logger.Info("Server shutdown completed")
}

func Init(ctx context.Context, store store.StoreI) (*mux.Router, *Server, error) {
	_ = store.LoadSmValue(ctx, "")
	_ = store.LoadEnvValue(ctx, "")

	//ignore error from LoadSmValue and LoadEnvValue as server has to start even if load has failed.
	store.SetServiceName(handlers.ServerName)
	store.SetInstanceId(handlers.InstanceId)
	store.SetBaseUrl(handlers.BaseUrl)
	logs.Logger.Info(fmt.Sprintf("Base url: %s", handlers.BaseUrl))

	s := new(Server)
	s.Store = store
	serverRouter := s.GetRouter()
	serverRouter.Use(tenantMiddleWare)
	return serverRouter, s, nil
}
