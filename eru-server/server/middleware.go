package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	claimsKey       string = eru_utils.ClaimsHeaderKey
	eruqlbaseurlKey string = "eruqlbaseurl"
)

var tenantRouteVars = []string{"tenant", "tenantId"}

func tenantMiddleWare(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultTenantId := ""
		vars := mux.Vars(r)
		var newVars map[string]string
		for _, varName := range tenantRouteVars {
			routeTenant, ok := vars[varName]
			if !ok {
				continue
			}
			tenantId, routeDefaultTenantId := eru_utils.ParseTenantRoute(routeTenant)
			if tenantId != routeTenant {
				if newVars == nil {
					newVars = make(map[string]string, len(vars))
					for k, v := range vars {
						newVars[k] = v
					}
				}
				newVars[varName] = tenantId
			}
			if routeDefaultTenantId != "" {
				defaultTenantId = routeDefaultTenantId
			}
		}
		// the tenant header is read but never rewritten: the gateway forwards this same
		// request on, so changing the header here would strip the default tenant before the
		// callee ever sees it. Handlers that need the split call RequestTenant.
		if _, headerDefaultTenantId := RequestTenant(r); headerDefaultTenantId != "" {
			defaultTenantId = headerDefaultTenantId
		}
		if newVars != nil {
			r = mux.SetURLVars(r, newVars)
		}
		if defaultTenantId != "" {
			r = r.WithContext(eru_utils.WithDefaultTenant(r.Context(), defaultTenantId))
		}
		next.ServeHTTP(w, r)
	})
}

func requestIdMiddleWare(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			return
		}
		requestID := r.Header.Get(server_handlers.RequestIdKey)
		if requestID == "" {
			// set a new request id header of request
			requestID = uuid.New().String()
			r.Header.Set(server_handlers.RequestIdKey, requestID)
		}
		spanId := oteltrace.SpanFromContext(r.Context()).SpanContext().SpanID().String()
		traceId := oteltrace.SpanFromContext(r.Context()).SpanContext().TraceID().String()
		if spanId == "0000000000000000" {
			spanId = ""
		}
		if traceId == "00000000000000000000000000000000" {
			traceId = ""
		}

		ctx := context.WithValue(r.Context(), server_handlers.RequestIdKey, requestID)
		r = r.WithContext(logs.NewContext(ctx, zap.String(server_handlers.RequestIdKey, requestID), zap.String("spanID", spanId), zap.String("traceID", traceId)))
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func otelMiddleWare(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, _ := r.Context().Value(server_handlers.RequestIdKey).(string)
		if requestID == "" {
			requestID = r.Header.Get(server_handlers.RequestIdKey)
		}
		pspan := oteltrace.SpanFromContext(r.Context())
		//if !span.IsRecording() {
		//	logs.WithContext(r.Context()).Info("Span not found - making new tracer")

		newCtx, span := otel.Tracer(server_handlers.ServerName).Start(r.Context(), "Initial", oteltrace.WithAttributes(attribute.String("requestID", requestID), attribute.String("traceID", pspan.SpanContext().TraceID().String()), attribute.String("spanID", pspan.SpanContext().SpanID().String())))
		defer span.End()

		newCtx = context.WithValue(newCtx, claimsKey, fmt.Sprint(r.Header.Get(eru_utils.ClaimsHeaderKey)))
		newCtx = context.WithValue(newCtx, eruqlbaseurlKey, fmt.Sprint(server_handlers.EruqlBaseUrl))
		r = r.WithContext(newCtx)
		//} else {
		//	logs.WithContext(r.Context()).Info("making child span")
		//	logs.WithContext(r.Context()).Info(fmt.Sprint(span.TracerProvider()))
		//	newCtx, span := span.TracerProvider().Tracer(server_handlers.ServerName).Start(r.Context(), "Initial", oteltrace.WithAttributes(attribute.String("requestId", requestID)))
		//	defer span.End()
		//	r = r.WithContext(newCtx)

		//}
		w.Header().Set("trace_id", span.SpanContext().TraceID().String())
		w.Header().Set(server_handlers.RequestIdKey, requestID)
		next.ServeHTTP(w, r)
	})
}

func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// Log the panic with full context
				panicMsg := fmt.Sprintf("HTTP handler panic: %v\nStack trace:\n%s", rec, string(debug.Stack()))
				err := errors.New(panicMsg)
				if logs.Logger != nil {
					err = logs.Err(r.Context(), err, "")
				} else {
					fmt.Printf("ERROR: %s\n", err.Error())
				}

				// Return 500 error to client
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)

				errorResponse := map[string]interface{}{
					"error":   "Internal server error",
					"message": err.Error(),
				}

				requestID, _ := r.Context().Value(server_handlers.RequestIdKey).(string)
				if requestID == "" {
					requestID = r.Header.Get(server_handlers.RequestIdKey)
				}
				if requestID != "" {
					errorResponse["request_id"] = requestID
				}

				json.NewEncoder(w).Encode(errorResponse)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func webSocketMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a WebSocket upgrade request
		if isWebSocketUpgrade(r) {
			// Apply special handling for WebSocket connections
			ctx := r.Context()
			logs.WithContext(ctx).Info(fmt.Sprintf("WebSocket upgrade request from: %s", r.RemoteAddr))

			// Add WebSocket-specific context values
			ctx = context.WithValue(ctx, "connection_type", "websocket")
			ctx = context.WithValue(ctx, "upgrade_protocol", r.Header.Get("Sec-WebSocket-Protocol"))
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)
	})
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Connection")) == "upgrade" &&
		strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}

func contextCancellationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a context with timeout for the request
		//ctx, cancel := context.WithTimeout(r.Context(), 300*time.Second)
		//defer cancel()

		//r = r.WithContext(ctx)
		ctx := r.Context()
		// Check if context is already cancelled before processing
		select {
		case <-ctx.Done():
			logs.WithContext(ctx).Warn("Request cancelled before processing")
			http.Error(w, "Request cancelled", http.StatusRequestTimeout)
			return
		default:
		}

		// Use a channel to signal completion
		done := make(chan struct{})

		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					// Log the panic but don't crash the service
					panicMsg := fmt.Sprintf("HTTP handler panic in contextCancellationMiddleware: %v\nStack trace:\n%s", rec, string(debug.Stack()))
					logs.WithContext(ctx).Error(panicMsg)

					// Try to write error response. w is only ever touched by this
					// goroutine, so this is safe.
					if w.Header().Get("Content-Type") == "" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusInternalServerError)

						errorResponse := map[string]interface{}{
							"error":   "Internal server error",
							"message": "Request handler panicked",
						}

						requestID, _ := r.Context().Value(server_handlers.RequestIdKey).(string)
						if requestID == "" {
							requestID = r.Header.Get(server_handlers.RequestIdKey)
						}
						if requestID != "" {
							errorResponse["request_id"] = requestID
						}

						json.NewEncoder(w).Encode(errorResponse)
					}
				}
				close(done)
			}()
			next.ServeHTTP(w, r)
		}()

		// Wait for the handler goroutine to finish. Even when the request
		// context is cancelled we must NOT return here and leave the goroutine
		// running - that would race two goroutines on the same ResponseWriter
		// and leak the goroutine. Cancellation propagates to the handler via
		// r.Context() (outbound calls unwind), which closes done.
		select {
		case <-done:
			// Request completed normally
			return
		case <-ctx.Done():
			logs.WithContext(ctx).Warn("Request cancelled during processing - waiting for handler to unwind")
			<-done
			return
		}
	})
}

var requestSemaphore chan struct{}

func init() {
	// Initialize request semaphore with configurable max concurrent requests
	maxConcurrentRequests := 100 // Default
	if env := os.Getenv("MAX_CONCURRENT_REQUESTS"); env != "" {
		if val, err := strconv.Atoi(env); err == nil && val > 0 {
			maxConcurrentRequests = val
		}
	}
	requestSemaphore = make(chan struct{}, maxConcurrentRequests)
}

func concurrencyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Configure timeout for waiting in queue
		queueTimeout := 30 * time.Second
		if env := os.Getenv("REQUEST_QUEUE_TIMEOUT"); env != "" {
			if duration, err := time.ParseDuration(env); err == nil {
				queueTimeout = duration
			}
		}

		select {
		case requestSemaphore <- struct{}{}: // Acquire semaphore slot
			defer func() { <-requestSemaphore }() // Release slot when done

			logs.WithContext(r.Context()).Debug("Request acquired concurrency slot")
			next.ServeHTTP(w, r)

		case <-time.After(queueTimeout): // Timeout waiting for slot
			logs.WithContext(r.Context()).Warn("Request rejected due to concurrency limit timeout")

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)

			errorResponse := map[string]interface{}{
				"error":       "Service too busy",
				"message":     "Server is handling too many concurrent requests, please try again later",
				"retry_after": "5s",
			}

			requestID, _ := r.Context().Value(server_handlers.RequestIdKey).(string)
			if requestID == "" {
				requestID = r.Header.Get(server_handlers.RequestIdKey)
			}
			if requestID != "" {
				errorResponse["request_id"] = requestID
			}

			json.NewEncoder(w).Encode(errorResponse)
		}
	})
}
