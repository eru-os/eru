package handlers

import (
	"context"
	"fmt"
	"time"

	//"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/eru-os/eru/eru-events/events"
	"github.com/eru-os/eru/eru-functions/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

func RouteForwardHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Info("RouteForwardHandler - Start")
		utils.PrintRequestBody(r.Context(), r, "RouteForwardHandler - Request")
		defer r.Body.Close()
		// Extract the host and url from incoming request
		host, url := extractHostUrl(r)
		logs.WithContext(r.Context()).Info(host)
		logs.WithContext(r.Context()).Info(url)
	}
}

func RouteAsyncTestHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("RouteAsyncTestHandler - Start")
		defer r.Body.Close()
		// Extract the host and url from incoming request
		host, url := extractHostUrl(r)
		logs.WithContext(r.Context()).Debug(host)
		logs.WithContext(r.Context()).Debug(url)
	}
}

func RouteHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("RouteHandler - Start")
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origins", r.Header.Get("Origin"))
		// Close the body of the request
		//defer file_utils.CloseTheCloser(request.Body)  //TODO to add request body close in all handlers across projects
		defer r.Body.Close()
		// Extract the host and url from incoming request
		host, url := extractHostUrl(r)
		vars := mux.Vars(r)
		projectId := vars["project"]
		routeName := vars["routename"]

		// Lookup a route based on host and url
		route, err := sh.Store.GetAndValidateRoute(ctx, routeName, projectId, host, url, r.Method, r.Header, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		/*
			if route.Authorizer != "" {

				authorizer, err := s.GetProjectAuthorizer(r.Context(), projectId, route.Authorizer)
				if err != nil {
					server_handlers.FormatResponse(w, http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					return
				}
				if !route.CheckPathException(r.URL.Path) {

					token := r.Header.Get(authorizer.TokenHeaderKey)
					if token == "" {
						server_handlers.FormatResponse(w, http.StatusUnauthorized)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized Request"})
						return
					}
					claims, err := authorizer.VerifyToken(r.Context(), r.Header.Get(authorizer.TokenHeaderKey))
					if err != nil {
						server_handlers.FormatResponse(w, http.StatusUnauthorized)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
						return
					}
					claimsBytes, err := json.Marshal(claims)
					if err != nil {
						logs.WithContext(r.Context()).Error(err.Error())
						server_handlers.FormatResponse(w, http.StatusUnauthorized)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
						return
					}
					r.Header.Add("claims", string(claimsBytes))
				}
			}
		*/
		logs.WithContext(ctx).Info(fmt.Sprint("allowed_origins = ", ctx.Value("allowed_origins")))
		response, _, err := route.Execute(ctx, r, url, false, "", nil, module_store.LoopThreads)
		if route.Redirect {
			logs.WithContext(ctx).Info(route.FinalRedirectUrl)
			http.Redirect(w, r, route.FinalRedirectUrl, http.StatusSeeOther)
		} else {
			if err != nil {
				server_handlers.FormatResponse(w, http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			for k, v := range response.Header {
				for _, h := range v {
					w.Header().Set(k, h)
				}
			}
			w.WriteHeader(response.StatusCode)
			_, err = io.Copy(w, response.Body)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}
		}
	}
}

func NotifyHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("NotifyHandler - Start")
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		channel := r.URL.Query().Get("channel")
		if channel == "" {
			channel = "demo" // default for testing
		}

		// SSE headers
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")

		// Tell client how quickly to retry
		fmt.Fprintf(w, "retry: 1000\n\n")
		flusher.Flush()

		ctx := r.Context()

		keepAlive := time.NewTicker(15 * time.Second)
		defer keepAlive.Stop()

		var eventIs []events.EventI
		if projectId != "" {
			event, err := sh.Store.FetchEvent(ctx, projectId, channel, sh.Store)
			if err == nil {
				eventType, _ := event.GetAttribute("event_type")
				if eventType == "DB" {
					event.SetCon(sh.Store.GetConn(), sh.Store.GetDbType())
				}
				eventIs = append(eventIs, event)
			}
		} else {
			// fallback to all projects if no projectId is provided
			projects := sh.Store.GetProjectList(ctx)
			for _, p := range projects {
				if pid, ok := p["ProjectId"].(string); ok {
					event, err := sh.Store.FetchEvent(ctx, pid, channel, sh.Store)
					if err == nil {
						eventType, _ := event.GetAttribute("event_type")
						if eventType == "DB" {
							event.SetCon(sh.Store.GetConn(), sh.Store.GetDbType())
						}
						eventIs = append(eventIs, event)
					}
				}
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-keepAlive.C:
				// Comment ping to keep connection alive
				fmt.Fprintf(w, ": ping %d\n\n", time.Now().Unix())
				flusher.Flush()
			default:
				foundMsg := false
				for _, eventI := range eventIs {
					evts, err := eventI.Poll(ctx)
					if err == nil {
						for _, e := range evts {
							// Stream as named event "message"
							writeSSE(w, "message", e.Msg)
							flusher.Flush()
							_ = eventI.DeleteMessage(ctx, e.MsgIdentifer)
							foundMsg = true
						}
					}
				}
				if !foundMsg {
					// Wait before next poll if no messages, while still checking for context cancellation
					select {
					case <-ctx.Done():
						return
					case <-time.After(2 * time.Second):
					}
				}
			}
		}
	}
}
func writeSSE(w http.ResponseWriter, event string, data string) {
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	// data lines must be line-safe
	for _, line := range splitLines(data) {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}
func splitLines(s string) []string {
	// simple split to ensure no raw newlines break SSE framing
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
