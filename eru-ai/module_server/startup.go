package module_server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/eru-os/eru/eru-cache/cache"
	"github.com/eru-os/eru/eru-server/server"

	"github.com/eru-os/eru/eru-ai/module_server/handlers"
	"github.com/eru-os/eru/eru-ai/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
)

func StartUp(ctx context.Context) (module_store.ModuleStoreI, error) {
	erufuncbaseurl := os.Getenv("ERUFUNCTIONS_BASEURL")
	if erufuncbaseurl == "" {
		erufuncbaseurl = "http://localhost:8083"
		logs.WithContext(ctx).Info("'ERUFUNCTIONS_BASEURL' environment variable not found - setting default value as http://localhost:8083")
	}
	// The default is only real once it is in the environment: other modules
	// (the cache's persistence, for one) read this with os.Getenv, and a value
	// that lives only in a package variable leaves them silently unconfigured.
	if os.Getenv("ERUFUNCTIONS_BASEURL") == "" {
		_ = os.Setenv("ERUFUNCTIONS_BASEURL", erufuncbaseurl)
	}
	module_store.Erufuncbaseurl = erufuncbaseurl

	eruauthbaseurl := os.Getenv("ERUAUTH_BASEURL")
	if eruauthbaseurl == "" {
		eruauthbaseurl = "http://localhost:8085"
		logs.WithContext(ctx).Info("'ERUAUTH_BASEURL' environment variable not found - setting default value as http://localhost:8085")
	}
	// The default is only real once it is in the environment: other modules
	// (the cache's persistence, for one) read this with os.Getenv, and a value
	// that lives only in a package variable leaves them silently unconfigured.
	if os.Getenv("ERUAUTH_BASEURL") == "" {
		_ = os.Setenv("ERUAUTH_BASEURL", eruauthbaseurl)
	}
	module_store.Eruauthbaseurl = eruauthbaseurl

	eruqlbaseurl := os.Getenv("ERUQL_BASEURL")
	if eruqlbaseurl == "" {
		eruqlbaseurl = "http://localhost:8087"
		logs.WithContext(ctx).Info("'ERUQL_BASEURL' environment variable not found - setting default value as http://localhost:8087")
	}
	// The default is only real once it is in the environment: other modules
	// (the cache's persistence, for one) read this with os.Getenv, and a value
	// that lives only in a package variable leaves them silently unconfigured.
	if os.Getenv("ERUQL_BASEURL") == "" {
		_ = os.Setenv("ERUQL_BASEURL", eruqlbaseurl)
	}
	server_handlers.EruqlBaseUrl = eruqlbaseurl
	module_store.Eruqlbaseurl = eruqlbaseurl

	eruaibaseurl := os.Getenv("ERUAI_BASEURL")
	if eruaibaseurl == "" {
		eruaibaseurl = "http://localhost:8088"
		logs.WithContext(ctx).Info("'ERUAI_BASEURL' environment variable not found - setting default value as http://localhost:8088")
	}
	// The default is only real once it is in the environment: other modules
	// (the cache's persistence, for one) read this with os.Getenv, and a value
	// that lives only in a package variable leaves them silently unconfigured.
	if os.Getenv("ERUAI_BASEURL") == "" {
		_ = os.Setenv("ERUAI_BASEURL", eruaibaseurl)
	}
	module_store.Eruaibaseurl = eruaibaseurl
	logs.WithContext(ctx).Info(fmt.Sprintf("ERUAI_BASEURL: %s", module_store.Eruaibaseurl))

	erufilesbaseurl := os.Getenv("ERUFILES_BASEURL")
	if erufilesbaseurl == "" {
		erufilesbaseurl = "http://localhost:8082"
		logs.WithContext(ctx).Info("'ERUFILES_BASEURL' environment variable not found - setting default value as http://localhost:8082")
	}
	// The default is only real once it is in the environment: other modules
	// (the cache's persistence, for one) read this with os.Getenv, and a value
	// that lives only in a package variable leaves them silently unconfigured.
	if os.Getenv("ERUFILES_BASEURL") == "" {
		_ = os.Setenv("ERUFILES_BASEURL", erufilesbaseurl)
	}
	module_store.Erufilesbaseurl = erufilesbaseurl

	// Expired entries in a cache store are the cache's business, not the
	// agent's. Nothing here configures a second store: agents already carry
	// their own chat_memory configuration and that is the only store there is.
	gm := server.GetGlobalGoroutineManager(ctx)
	gm.SafeGo("CacheStoreSweeper", func(ctx context.Context) {
		cache.StartSharedStoreSweeper(ctx, 10*time.Minute)
	})

	return module_store.LoadStore(ctx, handlers.StoreTableName, handlers.StoreTenantTableName)
}
