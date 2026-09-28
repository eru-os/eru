package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-server/server"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
)

//go:embed oauth_templates/*.html
var oauthTemplatesFS embed.FS

const (
	oauthCsrfCookieName    = "eru_oauth_csrf"
	oauthSessionCookieName = "eru_oauth_session"
	defaultOAuthAppName    = "Sign in"
	defaultAccentColor     = "#2f6fed"
	defaultBgColor         = "#f4f6f9"
)

// scopeDescriptions turn the scope names a client asks for into something a person can weigh up.
// An unknown scope is shown by name rather than hidden, so consent is never granted for something
// the screen did not name.
var scopeDescriptions = map[string]string{
	"openid":         "Confirm who you are",
	"profile":        "See your basic profile details",
	"email":          "See your email address",
	"offline_access": "Stay signed in when you are away",
}

type oauthScopeView struct {
	Name        string
	Description string
}

type oauthPageData struct {
	Branding        auth.OAuthServerBranding
	AccentColor     string
	BackgroundColor string
	Challenge       string
	CsrfToken       string
	FormAction      string
	ClientName      string
	Scopes          []oauthScopeView
	Error           string
}

// LoginPageHandler is where the authorization server sends the browser when a grant needs a human.
// With ui.login_url configured the request is handed to that app; otherwise the built in screen is
// rendered from the project's branding.
func LoginPageHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("LoginPageHandler - Start")
		authObj, flow, err := oauthFlowFromRequest(sh, r)
		if err != nil {
			writeOAuthPageError(w, r, auth.OAuthServerConfig{}, err)
			return
		}
		oAuthServer := authObj.OAuthServer(r.Context())
		challenge := r.URL.Query().Get("login_challenge")

		loginRequest, err := flow.LoginRequest(r.Context(), challenge)
		if err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}

		// The authorization server already has a session it will reuse, so there is nobody to ask.
		if loginRequest.Skip {
			redirectTo, acceptErr := flow.AcceptLogin(r.Context(), challenge, loginRequest.Subject, true)
			if acceptErr != nil {
				writeOAuthPageError(w, r, oAuthServer, acceptErr)
				return
			}
			http.Redirect(w, r, redirectTo, http.StatusFound)
			return
		}

		if oAuthServer.IsExternalUi() {
			http.Redirect(w, r, externalUiUrl(oAuthServer.Ui.LoginUrl, "login_challenge", challenge), http.StatusFound)
			return
		}

		renderOAuthPage(w, r, "login.html", oauthPageData{
			Branding:   brandingWithDefaults(oAuthServer),
			Challenge:  challenge,
			CsrfToken:  issueCsrfToken(w, r),
			FormAction: r.URL.Path,
			ClientName: loginRequest.ClientName,
		})
	}
}

// LoginSubmitHandler verifies credentials and accepts the login request. It answers with a redirect
// for the built in form, or with the redirect url as json when an external ui asked for it.
func LoginSubmitHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("LoginSubmitHandler - Start")
		authObj, flow, err := oauthFlowFromRequest(sh, r)
		if err != nil {
			writeOAuthPageError(w, r, auth.OAuthServerConfig{}, err)
			return
		}
		oAuthServer := authObj.OAuthServer(r.Context())

		credentials, err := readLoginSubmission(r)
		if err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}
		challenge := credentials.Challenge

		// The built in form sets the cookie this checks. An external ui posts cross origin and never
		// carries it, so there the unguessable single use challenge is the only capability required.
		if !oAuthServer.IsExternalUi() && !csrfTokenValid(r, credentials.CsrfToken) {
			renderLoginError(w, r, oAuthServer, challenge, "Your session expired - please try again")
			return
		}

		identity, _, err := authObj.Login(r.Context(),
			auth.LoginPostBody{Username: credentials.Username, Password: credentials.Password},
			server.RequestProject(r), false, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Info(fmt.Sprint("oauth login failed : ", err.Error()))
			renderLoginError(w, r, oAuthServer, challenge, "Invalid credentials - please try again")
			return
		}

		redirectTo, err := flow.AcceptLogin(r.Context(), challenge, identity.Id, true)
		if err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}
		// With our own backend the browser gets a session so the next authorization does not ask for
		// credentials again. A token backend keeps its own session and needs nothing here.
		issueOAuthSession(w, r, oAuthServer, flow, identity.Id)
		respondWithRedirect(w, r, redirectTo)
	}
}

// ConsentPageHandler shows what the client is asking for. A grant the authorization server is
// willing to reuse, or a scope set the project has already decided on, is accepted without a screen.
func ConsentPageHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ConsentPageHandler - Start")
		authObj, flow, err := oauthFlowFromRequest(sh, r)
		if err != nil {
			writeOAuthPageError(w, r, auth.OAuthServerConfig{}, err)
			return
		}
		oAuthServer := authObj.OAuthServer(r.Context())
		challenge := r.URL.Query().Get("consent_challenge")

		consentRequest, err := flow.ConsentRequest(r.Context(), challenge)
		if err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}

		if consentRequest.Skip {
			redirectTo, acceptErr := acceptConsent(r, flow, oAuthServer, consentRequest)
			if acceptErr != nil {
				writeOAuthPageError(w, r, oAuthServer, acceptErr)
				return
			}
			http.Redirect(w, r, redirectTo, http.StatusFound)
			return
		}

		if oAuthServer.IsExternalUi() && oAuthServer.Ui.ConsentUrl != "" {
			http.Redirect(w, r, externalUiUrl(oAuthServer.Ui.ConsentUrl, "consent_challenge", challenge), http.StatusFound)
			return
		}

		renderOAuthPage(w, r, "consent.html", oauthPageData{
			Branding:   brandingWithDefaults(oAuthServer),
			Challenge:  challenge,
			CsrfToken:  issueCsrfToken(w, r),
			FormAction: r.URL.Path,
			ClientName: consentRequest.ClientName,
			Scopes:     scopeViews(oAuthServer.GrantableScope(consentRequest.RequestedScope)),
		})
	}
}

func ConsentSubmitHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ConsentSubmitHandler - Start")
		authObj, flow, err := oauthFlowFromRequest(sh, r)
		if err != nil {
			writeOAuthPageError(w, r, auth.OAuthServerConfig{}, err)
			return
		}
		oAuthServer := authObj.OAuthServer(r.Context())

		if err = r.ParseForm(); err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}
		challenge := r.PostFormValue("consent_challenge")
		if !oAuthServer.IsExternalUi() && !csrfTokenValid(r, r.PostFormValue("csrf_token")) {
			writeOAuthPageError(w, r, oAuthServer, fmt.Errorf("your session expired - please try again"))
			return
		}

		if r.PostFormValue("decision") == "reject" {
			redirectTo, rejectErr := flow.RejectConsent(r.Context(), challenge, "access_denied", "the user denied the request")
			if rejectErr != nil {
				writeOAuthPageError(w, r, oAuthServer, rejectErr)
				return
			}
			respondWithRedirect(w, r, redirectTo)
			return
		}

		consentRequest, err := flow.ConsentRequest(r.Context(), challenge)
		if err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}
		redirectTo, err := acceptConsent(r, flow, oAuthServer, consentRequest)
		if err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}
		respondWithRedirect(w, r, redirectTo)
	}
}

// acceptConsent grants only what the project allows, and passes the requested audience through so
// the access token is bound to the resource it was asked for rather than to every resource.
func acceptConsent(r *http.Request, flow auth.AuthorizationFlowI, oAuthServer auth.OAuthServerConfig, consentRequest auth.ConsentRequestInfo) (string, error) {
	grantScope := oAuthServer.GrantableScope(consentRequest.RequestedScope)
	idTokenClaims := map[string]interface{}{}
	accessTokenClaims := map[string]interface{}{}
	if consentRequest.Subject != "" {
		accessTokenClaims["sub"] = consentRequest.Subject
	}
	return flow.AcceptConsent(r.Context(), consentRequest.Challenge, grantScope,
		consentRequest.RequestedAudience, idTokenClaims, accessTokenClaims, true)
}

type loginSubmission struct {
	Challenge string `json:"login_challenge"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	CsrfToken string `json:"csrf_token"`
}

// readLoginSubmission accepts the built in form post and a json body from an external ui. Password
// is returned base64 encoded either way, which is the encoding the rest of eru's login api uses.
func readLoginSubmission(r *http.Request) (loginSubmission, error) {
	credentials := loginSubmission{}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			return credentials, err
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return credentials, err
		}
		credentials.Challenge = r.PostFormValue("login_challenge")
		credentials.Username = r.PostFormValue("username")
		credentials.CsrfToken = r.PostFormValue("csrf_token")
		// A browser form posts the password as typed. Everywhere else in eru it travels base64
		// encoded - which is what EruAuth.Login decodes - so encode it here rather than leaving the
		// auth layer to guess which form it was handed. A json body is already in that shape, the
		// same as the /{project}/{authname}/login api, and is passed through untouched.
		credentials.Password = base64.StdEncoding.EncodeToString([]byte(r.PostFormValue("password")))
	}
	if credentials.Challenge == "" {
		return credentials, fmt.Errorf("login_challenge is missing")
	}
	return credentials, nil
}

func oauthFlowFromRequest(sh *module_store.StoreHolder, r *http.Request) (auth.AuthI, auth.AuthorizationFlowI, error) {
	authObj, err := authFromRequest(sh, r)
	if err != nil {
		return nil, nil, err
	}
	if !authObj.OAuthServer(r.Context()).Enabled {
		return nil, nil, fmt.Errorf("oauth server is not enabled")
	}
	// GetAuth hands back a clone with no database connection, so verifying credentials needs the
	// store's connection wired in first - the same thing LoginHandler does before calling Login.
	if authObj.GetAuthDb() != nil {
		authObj.GetAuthDb().SetConn(sh.Store.GetConn())
	}
	flow, err := authObj.AuthorizationFlow(r.Context(), server.RequestProject(r))
	if err != nil {
		return nil, nil, err
	}
	return authObj, flow, nil
}

// respondWithRedirect gives a browser a 302 and an external ui the url to follow itself.
func respondWithRedirect(w http.ResponseWriter, r *http.Request, redirectTo string) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"redirect_to": redirectTo})
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

func externalUiUrl(baseUrl string, challengeKey string, challenge string) string {
	separator := "?"
	if strings.Contains(baseUrl, "?") {
		separator = "&"
	}
	return fmt.Sprint(baseUrl, separator, challengeKey, "=", url.QueryEscape(challenge))
}

func brandingWithDefaults(oAuthServer auth.OAuthServerConfig) auth.OAuthServerBranding {
	branding := oAuthServer.Ui.Branding
	if branding.AppName == "" {
		branding.AppName = defaultOAuthAppName
	}
	return branding
}

func scopeViews(scopes []string) []oauthScopeView {
	var views []oauthScopeView
	for _, scope := range scopes {
		description, described := scopeDescriptions[scope]
		if !described {
			description = scope
		}
		views = append(views, oauthScopeView{Name: scope, Description: description})
	}
	return views
}

func renderOAuthPage(w http.ResponseWriter, r *http.Request, page string, data oauthPageData) {
	if data.AccentColor == "" {
		data.AccentColor = data.Branding.PrimaryColor
	}
	if data.AccentColor == "" {
		data.AccentColor = defaultAccentColor
	}
	if data.BackgroundColor == "" {
		data.BackgroundColor = data.Branding.BackgroundColor
	}
	if data.BackgroundColor == "" {
		data.BackgroundColor = defaultBgColor
	}

	pageTemplate, err := template.ParseFS(oauthTemplatesFS, "oauth_templates/layout.html", fmt.Sprint("oauth_templates/", page))
	if err != nil {
		logs.WithContext(r.Context()).Error(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// Consent is a clickjacking target: a framed screen could be clicked through by an overlay.
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if err = pageTemplate.ExecuteTemplate(w, "layout", data); err != nil {
		logs.WithContext(r.Context()).Error(err.Error())
	}
}

func renderLoginError(w http.ResponseWriter, r *http.Request, oAuthServer auth.OAuthServerConfig, challenge string, message string) {
	renderOAuthPage(w, r, "login.html", oauthPageData{
		Branding:   brandingWithDefaults(oAuthServer),
		Challenge:  challenge,
		CsrfToken:  issueCsrfToken(w, r),
		FormAction: r.URL.Path,
		Error:      message,
	})
}

// writeOAuthPageError keeps failures on this leg away from the client's redirect uri - there is no
// safe redirect to make when the challenge itself could not be resolved.
func writeOAuthPageError(w http.ResponseWriter, r *http.Request, oAuthServer auth.OAuthServerConfig, err error) {
	logs.WithContext(r.Context()).Error(err.Error())
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		server_handlers.FormatResponse(w, http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	renderOAuthPage(w, r, "login.html", oauthPageData{
		Branding: brandingWithDefaults(oAuthServer),
		Error:    err.Error(),
	})
}

// oauthSessionId reads the browser session cookie, if there is one.
func oauthSessionId(r *http.Request) string {
	cookie, err := r.Cookie(oauthSessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// issueOAuthSession records the session and sets its cookie. A failure here is not fatal: the
// authorization still completes, the next one just asks for credentials again.
func issueOAuthSession(w http.ResponseWriter, r *http.Request, oAuthServer auth.OAuthServerConfig, flowI auth.AuthorizationFlowI, identityId string) {
	flow, flowOk := flowI.(auth.EruAuthorizationFlow)
	if !flowOk {
		return
	}
	lifespan := oAuthServer.SessionLifespanSeconds()
	sessionId, err := flow.CreateSession(r.Context(), identityId, lifespan)
	if err != nil {
		logs.WithContext(r.Context()).Error(fmt.Sprint("oauth session could not be created : ", err.Error()))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthSessionCookieName,
		Value:    sessionId,
		Path:     "/",
		MaxAge:   lifespan,
		HttpOnly: true,
		Secure:   isRequestSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// clearOAuthSession removes the cookie whether or not the session could be revoked, so a browser is
// never left holding one it cannot use.
func clearOAuthSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isRequestSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// OAuthLogoutHandler ends the browser session. It does not revoke issued tokens - those have their own
// lifetime and their own revocation endpoint.
func OAuthLogoutHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("OAuthLogoutHandler - Start")
		_, flowI, err := oauthFlowFromRequest(sh, r)
		if err == nil {
			if flow, flowOk := flowI.(auth.EruAuthorizationFlow); flowOk {
				_ = flow.RevokeSession(r.Context(), oauthSessionId(r))
			}
		}
		clearOAuthSession(w, r)

		if redirectTo := r.URL.Query().Get("post_logout_redirect_uri"); redirectTo != "" {
			http.Redirect(w, r, redirectTo, http.StatusFound)
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": "signed out"})
	}
}

func issueCsrfToken(w http.ResponseWriter, r *http.Request) string {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		logs.WithContext(r.Context()).Error(err.Error())
		return ""
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	http.SetCookie(w, &http.Cookie{
		Name:     oauthCsrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isRequestSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

func csrfTokenValid(r *http.Request, submitted string) bool {
	cookie, err := r.Cookie(oauthCsrfCookieName)
	if err != nil || cookie.Value == "" || submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(submitted)) == 1
}

func isRequestSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
