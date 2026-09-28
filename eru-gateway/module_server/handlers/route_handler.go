package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eru-os/eru/eru-gateway/config_sync"
	"github.com/eru-os/eru/eru-gateway/module_store"
	"github.com/eru-os/eru/eru-gateway/user_events"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/eru-os/eru/eru-templates/gotemplate"
	utils "github.com/eru-os/eru/eru-utils"
)

var userEventLogger *user_events.Logger

func SetUserEventLogger(l *user_events.Logger) {
	userEventLogger = l
}

var configSyncNotifier *config_sync.Notifier

func SetConfigSyncNotifier(n *config_sync.Notifier) {
	configSyncNotifier = n
}

var httpClient = http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func RouteHandler(sh *module_store.StoreHolder, rh *RegistryHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("RouteHandler - Start")

		logEvent := userEventLogger.ShouldLog(r)
		var userEvent user_events.UserEvent
		if logEvent {
			startTime := time.Now()
			userEvent = userEventLogger.NewEvent(r, w, startTime)
			recorder := user_events.NewRecorder(w)
			w = recorder
			defer func() {
				userEvent.Status = recorder.Status()
				userEvent.DurationMs = int(time.Since(startTime).Milliseconds())
				userEventLogger.Log(userEvent)
			}()
		}

		host, url := extractHostUrl(r)
		logs.WithContext(r.Context()).Info(host)
		logs.WithContext(r.Context()).Info(url)
		utils.PrintRequestBody(r.Context(), r, "request from gateway")
		tg, authorizer, addHeaders, instanceId, err := sh.Store.GetTargetGroupAuthorizer(r.Context(), r)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "suspicious activity"})
			return
		}
		if logEvent {
			userEvent.TargetHost = tg.Host
		}
		logs.WithContext(r.Context()).Info(fmt.Sprint("authorizer.AuthorizerName = ", authorizer.AuthorizerName))
		if authorizer.AuthorizerName != "" {
			accessToken := authorizer.RequestToken(r)
			idToken := r.Header.Get(authorizer.IdTokenKey)
			token := ""
			if idToken != "" {
				token = idToken
			} else {
				token = accessToken
			}
			if token == "" || accessToken == "" {
				logs.WithContext(r.Context()).Info("token = \"\"")
				respondUnauthorized(w, r, "", "Unauthorized Request")
				logs.WithContext(r.Context()).Info(fmt.Sprint(http.StatusUnauthorized))
				return
			}

			accessClaims, err := authorizer.VerifyToken(r.Context(), accessToken, r.Header.Get(authorizer.KidHeaderKey))
			if err != nil {
				logs.WithContext(r.Context()).Error("access token verification failed")
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}
			accessClaimsMap, accessClaimsMapOk := accessClaims.(map[string]interface{})
			if !accessClaimsMapOk {
				logs.WithContext(r.Context()).Error("access token is not a map")
				logs.WithContext(r.Context()).Error(err.Error())
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}
			// A valid signature only says who minted the token, not that it was minted for this
			// resource. Audience and scope are what separate a token meant for us from any other
			// token the same authorization server issued.
			if err = authorizer.VerifyClaimsRestrictions(r.Context(), accessClaimsMap); err != nil {
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}

			accessSub, accessSubOk := accessClaimsMap["sub"]
			if !accessSubOk {
				err = fmt.Errorf("access token sub is not set")
				logs.WithContext(r.Context()).Error(err.Error())
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}

			claims, err := authorizer.VerifyToken(r.Context(), token, r.Header.Get(authorizer.KidHeaderKey))
			if err != nil {
				logs.WithContext(r.Context()).Error("id token verification failed")
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}

			claimsMap, claimsMapOk := claims.(map[string]interface{})
			if !claimsMapOk {
				logs.WithContext(r.Context()).Error("id token is not a map")
				logs.WithContext(r.Context()).Error(err.Error())
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}
			idSub, idSubOk := claimsMap["sub"]
			if !idSubOk {
				err = fmt.Errorf("id token sub is not set")
				logs.WithContext(r.Context()).Error(err.Error())
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}

			if idSub.(string) != accessSub.(string) {
				err = fmt.Errorf("sub mismatch")
				logs.WithContext(r.Context()).Error(err.Error())
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}

			if logEvent {
				if sub, subOk := accessSub.(string); subOk {
					userEvent.UserId = sub
				}
			}

			claimsBytes, err := json.Marshal(claims)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				respondUnauthorized(w, r, "invalid_token", err.Error())
				return
			}
			r.Header.Set(utils.ClaimsHeaderKey, string(claimsBytes))

			if authorizer.KidHeaderKey == "" {
				// An opaque token carries no readable claims, so the audience and scope have to come
				// back from introspection rather than from the token itself.
				introspectedClaims, introspectErr := authorizer.IntrospectAccessToken(r.Context(), accessToken)
				if introspectErr != nil {
					logs.WithContext(r.Context()).Info("invalid access token")
					respondUnauthorized(w, r, "invalid_token", "Unauthorized Request")
					return
				}
				if err = authorizer.VerifyClaimsRestrictions(r.Context(), introspectedClaims); err != nil {
					respondUnauthorized(w, r, "invalid_token", err.Error())
					return
				}
			}
		}

		for _, v := range addHeaders {
			headerValue := ""
			if v.IsTemplate {
				goTmpl := gotemplate.GoTemplate{Name: v.Key, Template: v.Value}
				outputObj, err := goTmpl.Execute(r.Context(), *r, "string")
				if err != nil {
					err = logs.Err(r.Context(), err, "")
					server_handlers.FormatResponse(w, http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					return
				} else {
					output, err := json.Marshal(outputObj)
					if err != nil {
						err = logs.Err(r.Context(), err, "")
						server_handlers.FormatResponse(w, http.StatusBadRequest)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
						return
					}
					if str, err := strconv.Unquote(string(output)); err == nil {
						headerValue = str
					} else {
						headerValue = string(output)
					}
				}
			} else {
				headerValue = v.Value
			}
			r.Header.Set(v.Key, headerValue)
		}

		// Record the host and scheme the gateway was addressed on before they are swapped for the
		// target, so a service can rebuild the public url it was reached through.
		if r.Header.Get("X-Forwarded-Host") == "" {
			r.Header.Set("X-Forwarded-Host", r.Host)
		}
		if r.Header.Get("X-Forwarded-Proto") == "" {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			r.Header.Set("X-Forwarded-Proto", scheme)
		}

		port := ""
		if tg.Port != "" {
			port = fmt.Sprint(":", tg.Port)
		}
		r.RequestURI = ""
		r.Host = tg.Host
		r.URL.Host = fmt.Sprint(tg.Host, port)
		r.URL.Scheme = tg.Scheme
		if tg.Method != "" {
			r.Method = tg.Method
		}

		if instanceId != "" {
			instances, err := rh.Registry.ListAllServices(r.Context())
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			} else {
				instanceFound := false
				for _, instance := range instances {
					if instance.Id == instanceId {
						instanceFound = true
						tmpSplit1 := strings.Split(instance.Address, "://")
						if tmpSplit1[0] == "http" || tmpSplit1[0] == "https" {
							tmpSplit2 := strings.Split(tmpSplit1[1], ":")
							logs.WithContext(r.Context()).Info(fmt.Sprint("tmpSplit2 = ", tmpSplit2))
							logs.WithContext(r.Context()).Info(fmt.Sprint("tmpSplit1 = ", tmpSplit1))
							r.Host = tmpSplit2[0]
							if len(tmpSplit1) > 1 {
								r.URL.Host = fmt.Sprint(tmpSplit1[1])
							} else {
								r.URL.Host = fmt.Sprint(tmpSplit1[0])
							}
							r.URL.Scheme = tmpSplit1[0]
							break
						}
					}
				}
				if !instanceFound {
					err = fmt.Errorf("instance not found")
					logs.WithContext(r.Context()).Error(err.Error())
					server_handlers.FormatResponse(w, 400)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
					return
				}
			}
		}

		//response, err := httpClient.Do(r)
		response, err := utils.ExecuteHttp(r.Context(), r)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		//defer response.Body.Close()

		configService := response.Header.Get(config_sync.ConfigUpdatedHeader)
		configInstance := response.Header.Get(config_sync.ConfigInstanceHeader)
		response.Header.Del(config_sync.ConfigUpdatedHeader)
		response.Header.Del(config_sync.ConfigInstanceHeader)
		if configService != "" {
			configSyncNotifier.Notify(configService, configInstance)
		}

		corsHeaders := []string{
			"Access-Control-Allow-Credentials",
			"Access-Control-Allow-Origin",
			"Access-Control-Allow-Headers",
			"Access-Control-Allow-Methods",
		}
		savedCors := make(map[string][]string)
		for _, h := range corsHeaders {
			if v := w.Header()[h]; len(v) > 0 {
				savedCors[h] = v
			}
			w.Header().Del(h)
		}
		for k, v := range response.Header {
			//logs.WithContext(r.Context()).Info(fmt.Sprint(k, " - ", v))
			w.Header()[k] = v
		}
		for _, h := range corsHeaders {
			if len(w.Header()[h]) == 0 {
				if v, ok := savedCors[h]; ok {
					w.Header()[h] = v
				}
			}
		}
		w.WriteHeader(response.StatusCode)
		if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
			flusher, canFlush := w.(http.Flusher)
			if canFlush {
				flusher.Flush()
			}
			buf := make([]byte, 4096)
			for {
				n, readErr := response.Body.Read(buf)
				if n > 0 {
					if _, writeErr := w.Write(buf[:n]); writeErr != nil {
						logs.WithContext(r.Context()).Error(writeErr.Error())
						break
					}
					if canFlush {
						flusher.Flush()
					}
				}
				if readErr != nil {
					break
				}
			}
		} else {
			_, err = io.Copy(w, response.Body)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
		}
		//logs.WithContext(r.Context()).Info(fmt.Sprint("---------------------------"))
		//logs.WithContext(r.Context()).Info(fmt.Sprint(w.Header()))
	}
}
func extractHostUrl(request *http.Request) (string, string) {
	return strings.Split(request.Host, ":")[0], request.URL.Path
}

func authChallenge(r *http.Request, oauthError string) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	challenge := fmt.Sprintf(`Bearer resource_metadata="%s://%s/.well-known/oauth-protected-resource"`, scheme, r.Host)
	if oauthError != "" {
		challenge = fmt.Sprint(challenge, `, error="`, oauthError, `"`)
	}
	return challenge
}

func respondUnauthorized(w http.ResponseWriter, r *http.Request, oauthError string, errMsg string) {
	w.Header().Set("WWW-Authenticate", authChallenge(r, oauthError))
	server_handlers.FormatResponse(w, http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
}
