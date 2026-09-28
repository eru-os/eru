package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
)

const (
	// ConfigApiKeyEnvKey holds the key that guards the gateway's own config routes. It is read from
	// the environment rather than the store, because the store is the thing being protected.
	ConfigApiKeyEnvKey = "ERUGATEWAY_CONFIG_API_KEY"

	ConfigApiKeyHeaderKey = "X-Config-Api-Key"
)

// ConfigApiKeyMiddleware guards the routes that read and write the gateway's own configuration.
// It is deliberately opt in: with the environment variable unset or empty the routes stay open, so
// an existing deployment behaves exactly as before until the key is introduced.
//
// This is only ever mounted on the config subrouters. The route that forwards requests to other
// services is not wrapped - those requests are authorised by their listener rule's authorizer, and
// a caller of the proxy has no reason to hold the gateway's config key.
func ConfigApiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedApiKey := os.Getenv(ConfigApiKeyEnvKey)
		if expectedApiKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		suppliedApiKey := r.Header.Get(ConfigApiKeyHeaderKey)
		// Compared in constant time so a caller cannot learn the key one byte at a time from how
		// long the comparison takes.
		if suppliedApiKey == "" || subtle.ConstantTimeCompare([]byte(expectedApiKey), []byte(suppliedApiKey)) != 1 {
			logs.WithContext(r.Context()).Info("config api key missing or incorrect")
			server_handlers.FormatResponse(w, http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid or missing config api key"})
			return
		}

		next.ServeHTTP(w, r)
	})
}
