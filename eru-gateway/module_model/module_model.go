package module_model

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/eru-os/eru/eru-crypto/jwt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
)

type StoreCompare struct {
	store.StoreCompare
	DeleteListenerRules   []string               `json:"delete_listener_rules"`
	NewListenerRules      []string               `json:"new_listener_rules"`
	MismatchListenerRules map[string]interface{} `json:"mismatch_listener_rules"`
	DeleteAuthorizer      []string               `json:"delete_authorizer"`
	NewAuthorizer         []string               `json:"new_authorizer"`
	MismatchAuthorizer    map[string]interface{} `json:"mismatch_authorizer"`
	MismatchSettings      map[string]interface{} `json:"mismatch_settings"`
}

type ModuleProjectI interface {
}

type Authorizer struct {
	AuthorizerName string   `json:"authorizer_name"`
	AuthName       string   `json:"auth_name"`
	TokenHeaderKey string   `json:"token_header_key"`
	KidHeaderKey   string   `json:"kid_header_key"`
	SecretAlgo     string   `json:"secret_algo"`
	JwkUrl         string   `json:"jwk_url"`
	TokenUrl       string   `json:"token_url"`
	TokenUrlKey    string   `json:"token_url_key"`
	TokenKey       string   `json:"token_key"`
	TokenJwkUrl    string   `json:"token_jwk_url"`
	Audience       []string `json:"audience"`
	Issuer         []string `json:"issuer"`
	VerifyClaims   bool     `json:"verify_claims"`
	RequiredScope  []string `json:"required_scope"`
	AccessTokenUrl string   `json:"access_token_url"`
	IdTokenKey     string   `json:"id_token_key"`
}

type ListenerRule struct {
	RuleRank              int64             `json:"rule_rank" eru:"required"`
	RuleName              string            `json:"rule_name" eru:"required"`
	Hosts                 []string          `json:"hosts"`
	Paths                 []PathStruct      `json:"paths"`
	Headers               []MapStruct       `json:"headers"`
	AddHeaders            []MapStructCustom `json:"add_headers"`
	Params                []MapStruct       `json:"params"`
	Methods               []string          `json:"methods"`
	SourceIP              []string          `json:"source_ip"`
	TargetHosts           []TargetHost      `json:"target_hosts" eru:"required"`
	AuthorizerName        string            `json:"authorizer_name"`
	AuthorizerException   []PathStruct      `json:"authorizer_exception"`
	AuthorizerExceptionIP []string          `json:"authorizer_exception_ip"`
}

type MapStruct struct {
	Key   string `json:"key" eru:"required"`
	Value string `json:"value" eru:"required"`
}

type MapStructCustom struct {
	MapStruct
	IsTemplate bool `json:"is_template" eru:"required"`
}

type PathStruct struct {
	MatchType string `json:"match_type" eru:"required"`
	Path      string `json:"path" eru:"required"`
}

type TargetHost struct {
	//Name       string `json:"name"`
	Host       string `json:"host" eru:"required"`
	Port       string `json:"port"`
	Method     string `json:"method"`
	Scheme     string `json:"scheme" eru:"required"`
	Allocation int64  `json:"allocation"`
}
type ProjectSettings struct {
	ClaimsKey string `json:"claims_key" eru:"required"`
}

// RequestToken reads the access token the caller presented. The configured header wins, so a
// deployment that sends its own header is unaffected. An Authorization: Bearer header is only read
// when that header is absent - which is how an oauth client that knows nothing of eru's header
// names presents a token, and which today is simply a 401.
func (authorizer Authorizer) RequestToken(r *http.Request) string {
	if token := r.Header.Get(authorizer.TokenHeaderKey); token != "" {
		return token
	}
	return BearerToken(r)
}

// BearerToken reads an RFC 6750 bearer token off the Authorization header.
func BearerToken(r *http.Request) string {
	authorization := r.Header.Get("Authorization")
	const bearerPrefix = "bearer "
	if len(authorization) <= len(bearerPrefix) || !strings.EqualFold(authorization[:len(bearerPrefix)], bearerPrefix) {
		return ""
	}
	return strings.TrimSpace(authorization[len(bearerPrefix):])
}

// VerifyClaimsRestrictions enforces the audience, issuer and scope the authorizer was configured
// with. A token that is merely signed by the right key is not thereby meant for this resource:
// without an audience check, any token the same authorization server ever minted is accepted here.
//
// Audience and issuer are gated behind VerifyClaims because both fields have been accepted in
// config for a long time without ever being read. Enforcing them silently would start rejecting
// tokens that work today. RequiredScope is new, so it is enforced whenever it is set.
func (authorizer Authorizer) VerifyClaimsRestrictions(ctx context.Context, claims map[string]interface{}) error {
	if authorizer.VerifyClaims {
		if len(authorizer.Audience) > 0 && !claimMatchesAny(claims["aud"], authorizer.Audience) {
			err := fmt.Errorf("token audience is not accepted by this authorizer")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		if len(authorizer.Issuer) > 0 && !claimMatchesAny(claims["iss"], authorizer.Issuer) {
			err := fmt.Errorf("token issuer is not accepted by this authorizer")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
	}
	if len(authorizer.RequiredScope) > 0 {
		grantedScope := claimScopes(claims)
		for _, requiredScope := range authorizer.RequiredScope {
			if !claimContains(grantedScope, requiredScope) {
				err := fmt.Errorf("token is missing the required scope : %s", requiredScope)
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
		}
	}
	return nil
}

// IntrospectAccessToken returns what the authorization server says about an opaque token. An
// inactive token yields an error rather than empty claims, so a caller cannot mistake one for the
// other.
func (authorizer Authorizer) IntrospectAccessToken(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	logs.WithContext(ctx).Debug("IntrospectAccessToken - Start")
	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	postBody := make(map[string]string)
	postBody["token"] = accessToken
	introspectRes, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, authorizer.AccessTokenUrl, headers, postBody, nil, nil, nil)
	if err != nil {
		return nil, logs.Err(ctx, err, "")
	}
	resMap, resMapOk := introspectRes.(map[string]interface{})
	if !resMapOk {
		err = fmt.Errorf("token introspection response is not a map")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	active, activeOk := resMap["active"].(bool)
	if !activeOk || !active {
		err = fmt.Errorf("token is not active")
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	}
	return resMap, nil
}

// claimMatchesAny reports whether a claim that may be a string or a list of strings overlaps the
// accepted values.
func claimMatchesAny(claim interface{}, accepted []string) bool {
	switch claimValue := claim.(type) {
	case string:
		return claimContains(accepted, claimValue)
	case []interface{}:
		for _, value := range claimValue {
			if valueStr, ok := value.(string); ok && claimContains(accepted, valueStr) {
				return true
			}
		}
	case []string:
		for _, value := range claimValue {
			if claimContains(accepted, value) {
				return true
			}
		}
	}
	return false
}

// claimScopes reads the granted scopes, which arrive space delimited in scope or as a list in scp
// depending on the authorization server.
func claimScopes(claims map[string]interface{}) []string {
	switch scopeClaim := claims["scope"].(type) {
	case string:
		return strings.Fields(scopeClaim)
	case []interface{}:
		var scopes []string
		for _, value := range scopeClaim {
			if valueStr, ok := value.(string); ok {
				scopes = append(scopes, valueStr)
			}
		}
		return scopes
	}
	if scpClaim, scpClaimOk := claims["scp"].([]interface{}); scpClaimOk {
		var scopes []string
		for _, value := range scpClaim {
			if valueStr, ok := value.(string); ok {
				scopes = append(scopes, valueStr)
			}
		}
		return scopes
	}
	return nil
}

func claimContains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (authorizer Authorizer) VerifyAccessToken(ctx context.Context, accessToken string) (valid bool) {
	valid = false
	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	postBody := make(map[string]string)
	postBody["token"] = accessToken
	introspectRes, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, authorizer.AccessTokenUrl, headers, postBody, nil, nil, nil)
	if err != nil {
		_ = logs.Err(ctx, err, "")
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint(introspectRes))
	if resMap, resMapOk := introspectRes.(map[string]interface{}); resMapOk {
		if validI, validIOk := resMap["active"]; validIOk {
			if validBool, validBoolOk := validI.(bool); validBoolOk {
				valid = validBool
			}
		}
		return valid
	}
	return
}

func (authorizer Authorizer) VerifyToken(ctx context.Context, token string, kid string) (claims interface{}, err error) {
	jwkUrl := authorizer.JwkUrl
	if kid != "" {
		jwkUrl = fmt.Sprint(jwkUrl, "/", kid)
	}
	logs.WithContext(ctx).Info(jwkUrl)
	claims, err = jwt.DecryptTokenJWK(ctx, token, jwkUrl)
	if err != nil {
		return
	}
	logs.WithContext(ctx).Info(authorizer.TokenUrl)
	if authorizer.TokenUrl != "" {
		headers := http.Header{}
		headers.Set("Content-Type", "application/json")
		postBody := make(map[string]string)
		postBody[authorizer.TokenUrlKey] = token
		hookRes, hookResHeaders, _, _, hookErr := utils.CallHttp(ctx, http.MethodPost, authorizer.TokenUrl, headers, nil, nil, nil, postBody)
		if hookErr != nil {
			err = logs.Err(ctx, hookErr, "")
			return
		}
		// token_url is configured, so the caller's real identity is the exchanged token, not
		// the access token that was presented. Failing to read it back must not silently fall
		// through to the access token's claims - downstream would then authorise a different,
		// thinner identity than the operator configured.
		claimsToken, claimsTokenErr := authorizer.readExchangedToken(ctx, hookRes, hookResHeaders)
		if claimsTokenErr != nil {
			err = claimsTokenErr
			return
		}
		claims, err = jwt.DecryptTokenJWK(ctx, claimsToken, authorizer.TokenJwkUrl)
		if err != nil {
			return
		}
	}
	return
}

// readExchangedToken pulls the user token out of the token_url response, looking in the body
// and then in the response headers, since token_key names either. The error says what came
// back - key names only, never values, because they are credentials.
func (authorizer Authorizer) readExchangedToken(ctx context.Context, hookRes interface{}, hookResHeaders http.Header) (claimsToken string, err error) {
	if hookResMap, hookResMapOk := hookRes.(map[string]interface{}); hookResMapOk {
		if claimsToken, claimsTokenOk := hookResMap[authorizer.TokenKey].(string); claimsTokenOk && claimsToken != "" {
			return claimsToken, nil
		}
		if headerToken := hookResHeaders.Get(authorizer.TokenKey); headerToken != "" {
			return headerToken, nil
		}
		return "", logs.Err(ctx, missingExchangedTokenErr(authorizer.TokenKey, hookRes), "")
	}
	if headerToken := hookResHeaders.Get(authorizer.TokenKey); headerToken != "" {
		return headerToken, nil
	}
	return "", logs.Err(ctx, missingExchangedTokenErr(authorizer.TokenKey, hookRes), "")
}

// missingExchangedTokenErr describes what the token_url actually returned, so the mismatch is
// visible in the logs. It names keys only - the values are tokens and must never be logged.
func missingExchangedTokenErr(tokenKey string, hookRes interface{}) error {
	hookResMap, hookResMapOk := hookRes.(map[string]interface{})
	if !hookResMapOk {
		return fmt.Errorf("token_url response is a %T, expected an object carrying %q", hookRes, tokenKey)
	}
	bodyKeys := make([]string, 0, len(hookResMap))
	for key := range hookResMap {
		bodyKeys = append(bodyKeys, key)
	}
	sort.Strings(bodyKeys)
	return fmt.Errorf("token_url response carries no non empty string %q - body keys were [%s]", tokenKey, strings.Join(bodyKeys, ", "))
}
