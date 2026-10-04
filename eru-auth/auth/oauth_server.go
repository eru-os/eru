package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const (
	OAuthBackendEru = "ERU"

	OAuthRegisterPath  = "/oauth2/register"
	OAuthAuthorizePath = "/oauth2/auth"
	OAuthTokenPath     = "/oauth2/token"
	OAuthRevokePath    = "/oauth2/revoke"
	OAuthUserInfoPath  = "/userinfo"
	OAuthJwksPath      = "/.well-known/jwks.json"
	OAuthLogoutPath    = "/oauth2/sessions/logout"
	OAuthLoginPath     = "/oauth2/login"
	OAuthConsentPath   = "/oauth2/consent"

	AuthorizationServerWellKnownPath = "/.well-known/oauth-authorization-server"
	OpenIdWellKnownPath              = "/.well-known/openid-configuration"
)

var (
	defaultOAuthScopes        = []string{"openid", "offline_access"}
	defaultOAuthGrantTypes    = []string{"authorization_code", "refresh_token"}
	defaultOAuthResponseTypes = []string{"code"}
)

// OAuthServerConfig is the authorization server eru-auth fronts for a given auth. With backend ERU
// eru-auth serves the grants itself; with a token backend the grants are the backend's, while
// eru-auth still owns discovery and client registration so that policy lives here. Which one holds
// the grants changes the endpoints the metadata advertises, not the clients or the metadata's shape.
type OAuthServerConfig struct {
	Enabled         bool              `json:"enabled"`
	Backend         string            `json:"backend"`
	Issuer          string            `json:"issuer"`
	ScopesSupported []string          `json:"scopes_supported"`
	Ui              OAuthServerUi     `json:"ui"`
	ClientPolicy    OAuthClientPolicy `json:"client_policy"`

	// Only used when Backend is ERU - a token backend issues its own tokens with its own configuration.
	SigningKid           string   `json:"signing_kid"`
	FirstPartyClientId   string   `json:"first_party_client_id"`
	AccessTokenAudience  []string `json:"access_token_audience"`
	AccessTokenLifespan  Lifespan `json:"access_token_lifespan_seconds"`
	IdTokenLifespan      Lifespan `json:"id_token_lifespan_seconds"`
	RefreshTokenLifespan Lifespan `json:"refresh_token_lifespan_seconds"`
	SessionLifespan      Lifespan `json:"session_lifespan_seconds"`

	// RefreshCookie moves the refresh token out of the login response body and into a cookie the
	// browser cannot read. Off by default, so an existing caller keeps getting it in the body.
	RefreshCookie RefreshCookieConfig `json:"refresh_cookie"`
}

// RefreshCookieConfig controls the http only refresh cookie issued to a first party browser. The
// refresh token is the long lived credential, so keeping it out of javascript is what stops an xss
// bug from turning into a lasting account takeover.
type RefreshCookieConfig struct {
	Enabled bool `json:"enabled"`
	// Domain should be the parent the api subdomains share, e.g. "dev.example.com", so one
	// cookie reaches every service of that project. A leading dot is accepted but dropped when the
	// header is written - RFC 6265 treats a bare domain as already covering its subdomains. Left
	// empty the cookie is host only.
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	// SameSite defaults to Lax, which keeps the cookie off cross site posts - that is what protects
	// the refresh endpoint from being driven by another origin.
	SameSite string `json:"same_site"`
	// OmitFromBody stops returning the refresh token in the json as well. Turn it on only once every
	// caller of this auth reads the cookie, since it is a breaking change for the others.
	OmitFromBody bool `json:"omit_from_body"`
}

// OAuthServerUi points the login and consent steps at an app of your own. Leave LoginUrl empty and
// eru-auth serves its own minimal screens, branded from Branding.
type OAuthServerUi struct {
	LoginUrl   string              `json:"login_url"`
	ConsentUrl string              `json:"consent_url"`
	Branding   OAuthServerBranding `json:"branding"`
}

type OAuthServerBranding struct {
	AppName         string `json:"app_name"`
	LogoUrl         string `json:"logo_url"`
	PrimaryColor    string `json:"primary_color"`
	BackgroundColor string `json:"background_color"`
	StylesheetUrl   string `json:"stylesheet_url"`
	PrivacyUrl      string `json:"privacy_url"`
	TermsUrl        string `json:"terms_url"`
	SupportUrl      string `json:"support_url"`
}

// OAuthClientPolicy bounds what a client may register for. An empty AllowedRedirectHosts means no
// host is accepted, so dynamic registration has to be opened deliberately rather than by default.
type OAuthClientPolicy struct {
	AllowDynamicRegistration bool     `json:"allow_dynamic_registration"`
	AllowedRedirectHosts     []string `json:"allowed_redirect_hosts"`
	AllowedGrantTypes        []string `json:"allowed_grant_types"`
	AllowedScopes            []string `json:"allowed_scopes"`
	DefaultScopes            []string `json:"default_scopes"`
	PublicClientsOnly        bool     `json:"public_clients_only"`
}

// OAuthClient is the registry's own view of a client, so the registry behind it can change without
// the handlers changing.
type OAuthClient struct {
	ClientId                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// ClientRegistryI is the seam between client management and whoever stores the clients: eru-auth's
// own table, or a token backend.
type ClientRegistryI interface {
	CreateClient(ctx context.Context, client OAuthClient) (OAuthClient, error)
	GetClient(ctx context.Context, clientId string) (OAuthClient, error)
	UpdateClient(ctx context.Context, clientId string, client OAuthClient) (OAuthClient, error)
	DeleteClient(ctx context.Context, clientId string) error
}

// OAuthError is an RFC 6749 / RFC 7591 error body. Registration and the grant endpoints answer with
// these rather than eru's usual {"error": "..."} so that a spec compliant client can read them.
type OAuthError struct {
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
	StatusCode       int    `json:"-"`
}

func (oAuthError OAuthError) Error() string {
	if oAuthError.ErrorDescription == "" {
		return oAuthError.ErrorCode
	}
	return fmt.Sprint(oAuthError.ErrorCode, ": ", oAuthError.ErrorDescription)
}

func NewOAuthError(statusCode int, errorCode string, description string) OAuthError {
	return OAuthError{ErrorCode: errorCode, ErrorDescription: description, StatusCode: statusCode}
}

// OAuthServerMetadata is the RFC 8414 authorization server metadata document. It doubles as the
// openid-configuration - the fields openid adds are already here.
type OAuthServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	JwksUri                           string   `json:"jwks_uri"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	UserInfoEndpoint                  string   `json:"userinfo_endpoint,omitempty"`
	RevocationEndpoint                string   `json:"revocation_endpoint,omitempty"`
	EndSessionEndpoint                string   `json:"end_session_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IdTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
}

// Lifespan is a duration held in seconds. It accepts a plain number of seconds, and also a duration
// string like "60m" or "30d", because that is how the rest of eru writes lifespans - and a bare
// number invites the wrong unit.
//
// Days are handled here: time.ParseDuration has no "d", so "30d" would otherwise be rejected.
type Lifespan int

func (lifespan Lifespan) Seconds() int { return int(lifespan) }

func (lifespan *Lifespan) UnmarshalJSON(b []byte) error {
	var asSeconds int
	if err := json.Unmarshal(b, &asSeconds); err == nil {
		*lifespan = Lifespan(asSeconds)
		return nil
	}

	var asDuration string
	if err := json.Unmarshal(b, &asDuration); err != nil {
		return fmt.Errorf("a lifespan must be a number of seconds or a duration such as \"60m\"")
	}
	asDuration = strings.TrimSpace(asDuration)
	if asDuration == "" {
		*lifespan = 0
		return nil
	}

	seconds, err := parseLifespan(asDuration)
	if err != nil {
		return err
	}
	*lifespan = Lifespan(seconds)
	return nil
}

// MarshalJSON writes seconds, so a value read back from the store is unambiguous whichever form it
// was written in.
func (lifespan Lifespan) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(lifespan))
}

func parseLifespan(value string) (int, error) {
	// "30d" and the like, which time.ParseDuration does not know.
	if days, found := strings.CutSuffix(strings.ToLower(value), "d"); found {
		dayCount, err := strconv.Atoi(strings.TrimSpace(days))
		if err == nil {
			return dayCount * 24 * 3600, nil
		}
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid lifespan - use seconds or a duration such as \"60m\", \"1h\" or \"30d\"", value)
	}
	return int(duration.Seconds()), nil
}

const defaultRefreshCookieName = "eru_refresh"

func (cookie RefreshCookieConfig) CookieName() string {
	if cookie.Name != "" {
		return cookie.Name
	}
	return defaultRefreshCookieName
}

func (cookie RefreshCookieConfig) CookiePath() string {
	if cookie.Path != "" {
		return cookie.Path
	}
	return "/"
}

// SameSiteMode maps the configured value, defaulting to Lax. None is only meaningful on a secure
// cookie and is what a cross site caller would need, so it has to be asked for explicitly.
func (cookie RefreshCookieConfig) SameSiteMode() http.SameSite {
	switch strings.ToUpper(cookie.SameSite) {
	case "STRICT":
		return http.SameSiteStrictMode
	case "NONE":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (oAuthServerConfig OAuthServerConfig) IsExternalUi() bool {
	return oAuthServerConfig.Ui.LoginUrl != ""
}

func (oAuthServerConfig OAuthServerConfig) Scopes() []string {
	if len(oAuthServerConfig.ScopesSupported) == 0 {
		return defaultOAuthScopes
	}
	return oAuthServerConfig.ScopesSupported
}

// OAuthServer returns the authorization server config of this auth.
func (auth *Auth) OAuthServer(ctx context.Context) OAuthServerConfig {
	return auth.OAuthServerConfig
}

// OAuthIssuer is the identifier clients discover this authorization server by. It falls back to
// the token backend's public url so an auth that has not been given an issuer still resolves to the
// server actually minting its tokens.
func (auth *Auth) OAuthIssuer(ctx context.Context) string {
	if auth.OAuthServerConfig.Issuer != "" {
		return strings.TrimSuffix(auth.OAuthServerConfig.Issuer, "/")
	}
	if backend, err := auth.tokenBackend(ctx); err == nil {
		return strings.TrimSuffix(backend.PublicUrl(), "/")
	}
	return ""
}

// ClientRegistry resolves the backend holding this auth's clients. The project is passed in because
// a client row is keyed by project and auth, while the auth object itself does not know which
// project it was loaded for.
func (auth *Auth) ClientRegistry(ctx context.Context, projectId string) (ClientRegistryI, error) {
	if auth.usesTokenBackend() {
		tokenBackend, err := auth.tokenBackend(ctx)
		if err != nil {
			return nil, err
		}
		return tokenBackend.ClientRegistry(), nil
	}
	backend := auth.oauthBackend()
	switch backend {
	case OAuthBackendEru:
		return EruClientRegistry{AuthDb: auth.AuthDb, ProjectId: projectId, AuthName: auth.AuthName}, nil
	default:
		err := errors.New(fmt.Sprint("unknown oauth server backend : ", backend))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

// AuthorizationServerMetadata builds the discovery document. While a token backend holds the grants
// the grant endpoints are its own, but registration is eru-auth's so that ClientPolicy is enforced.
func (auth *Auth) AuthorizationServerMetadata(ctx context.Context) (OAuthServerMetadata, error) {
	logs.WithContext(ctx).Debug("AuthorizationServerMetadata - Start")
	if !auth.OAuthServerConfig.Enabled {
		err := errors.New(fmt.Sprint("oauth server is not enabled for auth ", auth.AuthName))
		logs.WithContext(ctx).Info(err.Error())
		return OAuthServerMetadata{}, err
	}

	issuer := auth.OAuthIssuer(ctx)
	// With the ERU backend every endpoint is ours. While a token backend holds the grants, only
	// discovery and registration are - the rest of the document points at the token backend.
	grantBase := issuer
	if auth.usesTokenBackend() {
		backend, err := auth.tokenBackend(ctx)
		if err != nil {
			return OAuthServerMetadata{}, err
		}
		grantBase = strings.TrimSuffix(backend.PublicUrl(), "/")
	}
	if grantBase == "" {
		err := errors.New(fmt.Sprint("no grant endpoint base resolved for auth ", auth.AuthName))
		logs.WithContext(ctx).Error(err.Error())
		return OAuthServerMetadata{}, err
	}

	metadata := OAuthServerMetadata{
		Issuer:                            issuer,
		AuthorizationEndpoint:             fmt.Sprint(grantBase, OAuthAuthorizePath),
		TokenEndpoint:                     fmt.Sprint(grantBase, OAuthTokenPath),
		JwksUri:                           fmt.Sprint(grantBase, OAuthJwksPath),
		UserInfoEndpoint:                  fmt.Sprint(grantBase, OAuthUserInfoPath),
		RevocationEndpoint:                fmt.Sprint(grantBase, OAuthRevokePath),
		EndSessionEndpoint:                fmt.Sprint(grantBase, OAuthLogoutPath),
		ScopesSupported:                   auth.OAuthServerConfig.Scopes(),
		ResponseTypesSupported:            defaultOAuthResponseTypes,
		GrantTypesSupported:               auth.OAuthServerConfig.EffectiveClientPolicy().GrantTypes(),
		TokenEndpointAuthMethodsSupported: auth.OAuthServerConfig.EffectiveClientPolicy().AuthMethods(),
		CodeChallengeMethodsSupported:     []string{"S256"},
		SubjectTypesSupported:             []string{"public"},
		IdTokenSigningAlgValuesSupported:  []string{"RS256"},
	}
	if auth.OAuthServerConfig.EffectiveClientPolicy().AllowDynamicRegistration {
		metadata.RegistrationEndpoint = fmt.Sprint(issuer, OAuthRegisterPath)
	}
	return metadata, nil
}

func (policy OAuthClientPolicy) GrantTypes() []string {
	if len(policy.AllowedGrantTypes) == 0 {
		return defaultOAuthGrantTypes
	}
	return policy.AllowedGrantTypes
}

func (policy OAuthClientPolicy) AuthMethods() []string {
	if policy.PublicClientsOnly {
		return []string{"none"}
	}
	return []string{"none", "client_secret_basic", "client_secret_post"}
}

// EffectiveClientPolicy is the policy with its scope ceiling resolved. allowed_scopes and
// scopes_supported were two lists that almost always had to say the same thing, and disagreeing
// silently changed what a client could ask for, so leaving allowed_scopes unset now means "whatever
// this server supports". Set it only to hold registered clients to less than first party login gets.
func (oAuthServerConfig OAuthServerConfig) EffectiveClientPolicy() OAuthClientPolicy {
	policy := oAuthServerConfig.ClientPolicy
	if len(policy.AllowedScopes) == 0 {
		policy.AllowedScopes = oAuthServerConfig.Scopes()
	}
	return policy
}

func (policy OAuthClientPolicy) Scopes() []string {
	if len(policy.AllowedScopes) == 0 {
		return defaultOAuthScopes
	}
	return policy.AllowedScopes
}

// ApplyPolicy validates a client against the policy and fills in what the caller left out. It is
// the only place a dynamically registered client is vetted, so it fails closed: a redirect uri host
// that is not listed is rejected even when the list is empty.
func (policy OAuthClientPolicy) ApplyPolicy(ctx context.Context, client OAuthClient) (OAuthClient, error) {
	logs.WithContext(ctx).Debug("ApplyPolicy - Start")

	if len(client.RedirectURIs) == 0 {
		return client, NewOAuthError(400, "invalid_redirect_uri", "at least one redirect_uri is required")
	}
	for _, redirectUri := range client.RedirectURIs {
		if err := policy.validateRedirectUri(ctx, redirectUri); err != nil {
			return client, err
		}
	}

	if len(client.GrantTypes) == 0 {
		client.GrantTypes = policy.GrantTypes()
	}
	for _, grantType := range client.GrantTypes {
		if !contains(policy.GrantTypes(), grantType) {
			return client, NewOAuthError(400, "invalid_client_metadata", fmt.Sprint("grant_type not allowed : ", grantType))
		}
	}

	if len(client.ResponseTypes) == 0 {
		client.ResponseTypes = defaultOAuthResponseTypes
	}
	for _, responseType := range client.ResponseTypes {
		if !contains(defaultOAuthResponseTypes, responseType) {
			return client, NewOAuthError(400, "invalid_client_metadata", fmt.Sprint("response_type not allowed : ", responseType))
		}
	}

	if client.Scope == "" {
		client.Scope = strings.Join(policy.defaultScopes(), " ")
	}
	for _, scope := range strings.Fields(client.Scope) {
		if !contains(policy.Scopes(), scope) {
			return client, NewOAuthError(400, "invalid_client_metadata", fmt.Sprint("scope not allowed : ", scope))
		}
	}

	if policy.PublicClientsOnly {
		// A public client cannot keep a secret, so it authenticates with pkce alone. Forcing the
		// method here means a client cannot register itself back onto a weaker one.
		client.TokenEndpointAuthMethod = "none"
		client.ClientSecret = ""
	} else if client.TokenEndpointAuthMethod == "" {
		client.TokenEndpointAuthMethod = "none"
	} else if !contains(policy.AuthMethods(), client.TokenEndpointAuthMethod) {
		return client, NewOAuthError(400, "invalid_client_metadata", fmt.Sprint("token_endpoint_auth_method not allowed : ", client.TokenEndpointAuthMethod))
	}

	return client, nil
}

func (policy OAuthClientPolicy) defaultScopes() []string {
	if len(policy.DefaultScopes) == 0 {
		return policy.Scopes()
	}
	return policy.DefaultScopes
}

// validateRedirectUri holds the rules an authorization code is only as safe as: an exact absolute
// https url, no fragment, and a host the project has listed.
func (policy OAuthClientPolicy) validateRedirectUri(ctx context.Context, redirectUri string) error {
	parsedUri, err := url.Parse(redirectUri)
	if err != nil {
		return NewOAuthError(400, "invalid_redirect_uri", fmt.Sprint("redirect_uri is not a url : ", redirectUri))
	}
	if !parsedUri.IsAbs() || parsedUri.Host == "" {
		return NewOAuthError(400, "invalid_redirect_uri", fmt.Sprint("redirect_uri must be absolute : ", redirectUri))
	}
	if parsedUri.Fragment != "" || strings.Contains(redirectUri, "#") {
		return NewOAuthError(400, "invalid_redirect_uri", fmt.Sprint("redirect_uri must not carry a fragment : ", redirectUri))
	}
	host := parsedUri.Hostname()
	isLoopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if parsedUri.Scheme != "https" && !isLoopback {
		return NewOAuthError(400, "invalid_redirect_uri", fmt.Sprint("redirect_uri must be https : ", redirectUri))
	}
	if isLoopback {
		return nil
	}
	if !contains(policy.AllowedRedirectHosts, host) {
		return NewOAuthError(400, "invalid_redirect_uri", fmt.Sprint("redirect_uri host is not allowed : ", host))
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func stringSlice(value interface{}) ([]string, bool) {
	rawValues, rawValuesOk := value.([]interface{})
	if !rawValuesOk {
		return nil, false
	}
	var values []string
	for _, rawValue := range rawValues {
		if v, ok := rawValue.(string); ok {
			values = append(values, v)
		}
	}
	return values, len(values) > 0
}
