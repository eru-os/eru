package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eru-os/eru/eru-functions/functions"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	"github.com/eru-os/eru/eru-secret-manager/kms"
	storepkg "github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/uuid"
)

type AuthI interface {
	//Login(req *http.Request) (res interface{}, cookies []*http.Cookie, err error)
	SetAuthDb(authDbI AuthDbI)
	SetTokenContext(projectId string, signer TokenSignerI, self AuthI)
	GetAuthDb() (authDbI AuthDbI)
	Login(ctx context.Context, loginPostBody LoginPostBody, projectId string, withTokens bool, s storepkg.StoreI) (identity Identity, loginSuccess LoginSuccess, err error)
	IdpToken(ctx context.Context, idpToken LoginPostBody, projectId string, withTokens bool, renewFlag bool, s storepkg.StoreI) (loginResI interface{}, err error)
	GetToken(ctx context.Context, projectId string, tokenKeyPrefix string, s storepkg.StoreI) (accessToken string, err error)
	Register(ctx context.Context, registerUser RegisterUser, projectId string) (identity Identity, loginSuccess LoginSuccess, err error)
	RemoveUser(ctx context.Context, removeUser RemoveUser) (err error)
	Logout(ctx context.Context, req *http.Request) (res interface{}, resStatusCode int, err error)
	VerifyToken(ctx context.Context, tokenType string, token string) (res interface{}, err error)
	GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error)
	OAuthServer(ctx context.Context) OAuthServerConfig
	OAuthIssuer(ctx context.Context) string
	ClientRegistry(ctx context.Context, projectId string) (ClientRegistryI, error)
	AuthorizationServerMetadata(ctx context.Context) (OAuthServerMetadata, error)
	AuthorizationFlow(ctx context.Context, projectId string) (AuthorizationFlowI, error)
	GetUserInfo(ctx context.Context, access_token string) (identity Identity, err error)
	FetchTokens(ctx context.Context, refresh_token string, userId string) (res interface{}, err error)
	GetTokens(ctx context.Context, code string) (res interface{}, err error)
	GenerateTempCode(ctx context.Context, id string, tokens map[string]interface{}) (code string, err error)
	LoginApi(ctx context.Context, refresh_token string, userId string) (res interface{}, err error)
	MakeFromJson(ctx context.Context, rj *json.RawMessage) (err error)
	PerformPreSaveTask(ctx context.Context) (err error)
	PerformPreDeleteTask(ctx context.Context) (err error)
	GetUser(ctx context.Context, userId string) (identity Identity, err error)
	UpdateUser(ctx context.Context, identityToUpdate Identity, userId string, token map[string]interface{}) (tokens interface{}, err error)
	ChangePassword(ctx context.Context, tokenObj map[string]interface{}, userId string, changePasswordObj ChangePassword) (err error)
	GenerateRecoveryCode(ctx context.Context, recoveryIdentifier RecoveryPostBody, projectId string, silentFlag bool) (msg string, err error)
	GenerateVerifyCode(ctx context.Context, verifyIdentifier VerifyPostBody, projectId string, silentFlag bool) (msg string, err error)
	CompleteRecovery(ctx context.Context, recoveryPassword RecoveryPassword, cookies []*http.Cookie) (msg string, err error)
	VerifyRecovery(ctx context.Context, recoveryPassword RecoveryPassword) (res map[string]string, cookies []*http.Cookie, err error)
	VerifyCode(ctx context.Context, verifyCode VerifyCode, tokenObj map[string]interface{}, withToken bool) (res interface{}, err error)
	VerifyCodeNoUser(ctx context.Context, verifyCode VerifyCode) (res interface{}, err error)
	GetUrl(ctx context.Context, state string) (url string, oAuthParams OAuthParams, err error)
	SetKms(ctx context.Context, kmsObj kms.KmsStoreI) (err error)
	ApiTokenToUserToken(ctx context.Context, projectId string, apiToken string) (identity Identity, loginSuccess LoginSuccess, err error)
	GetIdToken(ctx context.Context, projectId string, identityId string) (idToken string, err error)
}

const (
	OTP_PURPOSE_RECOVERY = "RECOVERY"
	OTP_PURPOSE_VERIFY   = "VERIFY"
)

type AuthConfig struct {
	ClientId     string      `json:"client_id" eru:"required"`
	ClientSecret string      `json:"client_secret" eru:"required"`
	RedirectURI  string      `json:"redirect_uri" eru:"required"`
	Scope        string      `json:"scope" eru:"required"`
	SsoBaseUrl   string      `json:"sso_base_url" eru:"required"`
	TokenUrl     string      `json:"token_url" eru:"required"`
	JwkUrl       string      `json:"jwk_url" eru:"required"`
	Identifiers  Identifiers `json:"identifiers" eru:"required"`
}
type OAuthParams struct {
	ClientId            string `json:"client_id"`
	Scope               string `json:"scope"`
	RedirectURI         string `json:"redirect_uri"`
	ClientRequestId     string `json:"client-request-id"`
	ResponseMode        string `json:"response_mode"`
	ResponseType        string `json:"response_type"`
	CodeChallenge       string `json:"code_challenge"`
	CodeVerifier        string
	CodeChallengeMethod string `json:"code_challenge_method"`
	Nonce               string `json:"nonce"`
	State               string `json:"state"`
	Url                 string
	Prompt              string `json:"prompt"`
	AccessType          string `json:"access_type"`
}

type ChangePassword struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type LoginPostBody struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	IdpCode        string `json:"code"`
	IdpRequestId   string `json:"request_id"`
	CodeVerifier   string `json:"-"`
	Nonce          string `json:"-"`
	RefreshToken   string `json:"refresh_token"`
	TokenKeyPrefix string `json:"token_key_prefix,omitempty"`
}

type RecoveryPostBody struct {
	Username string `json:"username"`
}

type VerifyPostBody struct {
	Username       string `json:"username"`
	CredentialType string `json:"credential_type"`
}

type RecoveryPassword struct {
	Code     string `json:"code"`
	Id       string `json:"id"`
	Password string `json:"password"`
}

type VerifyCode struct {
	Code   string `json:"code"`
	Id     string `json:"id"`
	UserId string `json:"-"`
}

type Identity struct {
	Id          string                 `json:"id"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Attributes  map[string]interface{} `json:"attributes"`
	AuthDetails IdentityAuth           `json:"auth_details"`
	OtherInfo   map[string]interface{} `json:"other_info"`
	Status      string                 `json:"status"`
}

type IdentityAuth struct {
	SessionToken                string        `json:"session_token"`
	SessionId                   string        `json:"session_id"`
	SessionStatus               bool          `json:"session_status"`
	ExpiresAt                   time.Time     `json:"expires_at"`
	AuthenticatedAt             time.Time     `json:"authenticated_at"`
	AuthenticatorAssuranceLevel string        `json:"authenticator_assurance_level"`
	AuthenticationMethods       []interface{} `json:"authentication_methods"`
	IssuedAt                    time.Time     `json:"issued_at"`
}

type LoginSuccess struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IdToken      string    `json:"id_token"`
	Expiry       time.Time `json:"expiry"`
	ExpiresIn    float64   `json:"expires_in"`
	Id           string    `json:"id"`
}

type Auth struct {
	AuthType           string          `json:"auth_type"`
	AuthName           string          `json:"auth_name"`
	TokenHeaderKey     string          `json:"token_header_key"`
	Hooks              AuthHooks       `json:"hooks" eru:"optional"`
	AuthDb             AuthDbI         `json:"-"`
	PKCE               bool            `json:"pkce"`
	TokenBackendConfig json.RawMessage `json:"hydra,omitempty"`

	// Set by the store when an auth is cloned for a request. They are not configuration - the
	// project an auth was loaded for, and the means to sign with that project's keys.
	ProjectId   string       `json:"-"`
	TokenSigner TokenSignerI `json:"-"`
	// Self is the concrete auth this Auth is embedded in. A method reached through the embedded
	// struct has no knowledge of its outer type, so calling GetUser on the embedded value gets the
	// stub rather than EruAuth's implementation. Anything needing an overridden method goes via
	// concrete().
	Self              AuthI             `json:"-"`
	OAuthServerConfig OAuthServerConfig `json:"oauth_server"`
	KmsId             string            `json:"key_id"`
	KmsKey            kms.KmsStoreI     `json:"-"`
}

type AuthHooks struct {
	SRCF string `json:"srcf"`
	SVCF string `json:"svcf"`
	SWEF string `json:"swef"`
	USRP string `json:"usrp"`
	USRR string `json:"usrr"`
}

type IdentifierConfig struct {
	Enable    bool   `json:"enable"`
	IdpMapper string `json:"idp_mapper"`
}

type Identifiers struct {
	Email    IdentifierConfig `json:"email"`
	Mobile   IdentifierConfig `json:"mobile"`
	Username IdentifierConfig `json:"username"`
	UserId   IdentifierConfig `json:"user_id"`
}

type UserTraits struct {
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Email          string `json:"email"`
	Mobile         string `json:"mobile"`
	Username       string `json:"username"`
	EmailVerified  bool   `json:"email_verified"`
	MobileVerified bool   `json:"mobile_verified"`
}

type RegisterUser struct {
	UserTraits
	Password       string                 `json:"password"`
	UserAttributes map[string]interface{} `json:"user_attributes"`
}

type RemoveUser struct {
	UserId string `json:"id"`
}

func (auth *Auth) SetAuthDb(authDbI AuthDbI) {
	auth.AuthDb = authDbI
}

// SetTokenContext records which project this auth was loaded for and how to sign with its keys, so
// the built in token backend can mint without reaching back into the store.
func (auth *Auth) SetTokenContext(projectId string, signer TokenSignerI, self AuthI) {
	auth.ProjectId = projectId
	auth.TokenSigner = signer
	auth.Self = self
}

// concrete returns the auth to call overridable methods on: the outer type when one was recorded,
// and otherwise this value, which keeps a bare Auth usable in tests.
func (auth *Auth) concrete() AuthI {
	if auth.Self != nil {
		return auth.Self
	}
	return auth
}

func (auth *Auth) GetAuthDb() (authDbI AuthDbI) {
	return auth.AuthDb
}

func (auth *Auth) GetUrl(ctx context.Context, state string) (url string, oAuthParams OAuthParams, err error) {
	err = errors.New("GetUrl Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (auth *Auth) GenerateTempCode(ctx context.Context, id string, tokens map[string]interface{}) (code string, err error) {
	err = errors.New("GetUrl Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (auth *Auth) GetOAuthUrl(ctx context.Context, state string) (url string, oAuthParams OAuthParams, err error) {
	err = errors.New("GetOAuthUrl Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (auth *Auth) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	err := errors.New("MakeFromJson Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return err
}

func (auth *Auth) GenerateRecoveryCode(ctx context.Context, recoveryIdentifier RecoveryPostBody, projectId string, silentFlag bool) (msg string, err error) {
	err = errors.New("GenerateRecoveryCode Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return "", err
}

func (auth *Auth) SendCode(ctx context.Context, credentialIdentifier string, recovery_code string, recovery_time string, name string, projectId string, purpose string, credentialType string) (err error) {
	logs.WithContext(ctx).Debug("sendRecoveryCode - Start")
	trReqVars := &functions.TemplateVars{}
	if trReqVars.Vars == nil {
		trReqVars.Vars = make(map[string]interface{})
	}
	trReqVars.Vars["credential_type"] = credentialType
	trReqVars.Vars[credentialType] = credentialIdentifier
	trReqVars.Vars["recovery_code"] = recovery_code
	trReqVars.Vars["recovery_time"] = recovery_time
	trReqVars.Vars["name"] = name

	srcHookFound := false
	if purpose == OTP_PURPOSE_RECOVERY {
		logs.WithContext(ctx).Info(auth.Hooks.SRCF)
		if auth.Hooks.SRCF != "" {
			_, err = triggerHook(ctx, auth.Hooks.SRCF, projectId, trReqVars.Vars)
			srcHookFound = true
		}
		if !srcHookFound {
			logs.WithContext(ctx).Warn("SRCF hook not defined for auth. Thus no email was triggered.")
		}
	}
	if purpose == OTP_PURPOSE_VERIFY {
		logs.WithContext(ctx).Info(auth.Hooks.SVCF)
		if auth.Hooks.SVCF != "" {
			_, err = triggerHook(ctx, auth.Hooks.SVCF, projectId, trReqVars.Vars)
			srcHookFound = true
		}
		if !srcHookFound {
			logs.WithContext(ctx).Warn("SVCF hook not defined for auth. Thus no email was triggered.")
		}
	}
	return
}

func triggerHook(ctx context.Context, functionName string, projectId string, funcBody map[string]interface{}) (res interface{}, err error) {
	urlArray := strings.Split(ctx.Value("Erufuncbaseurl").(string), "://")
	if len(urlArray) < 2 {
		err = errors.New("incorrect eru-functions url")
		return
	}
	srcfUrl := url.URL{
		Scheme: urlArray[0],
		Host:   urlArray[1],
		Path:   fmt.Sprint("/", projectId, "/func/", functionName),
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Content-Length", strconv.Itoa(0))
	logs.WithContext(ctx).Info(fmt.Sprint(srcfUrl.String()))
	logs.WithContext(ctx).Info(fmt.Sprint(headers))
	logs.WithContext(ctx).Info(fmt.Sprint(funcBody))
	hookRes, _, _, _, hookErr := utils.CallHttp(ctx, http.MethodPost, srcfUrl.String(), headers, nil, nil, nil, funcBody)
	if hookErr != nil {
		err = hookErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return hookRes, hookErr
}

func (auth *Auth) sendWelcomeEmail(ctx context.Context, credentialIdentifier string, name string, projectId string, credentialType string) (err error) {
	logs.WithContext(ctx).Debug("sendWelcomeEmail - Start")
	trReqVars := &functions.TemplateVars{}
	if trReqVars.Vars == nil {
		trReqVars.Vars = make(map[string]interface{})
	}
	trReqVars.Vars["credential_type"] = credentialType
	trReqVars.Vars[credentialType] = credentialIdentifier
	trReqVars.Vars["name"] = name

	logs.WithContext(ctx).Info(auth.Hooks.SWEF)
	if auth.Hooks.SWEF != "" {
		_, err = triggerHook(ctx, auth.Hooks.SWEF, projectId, trReqVars.Vars)
	} else {
		logs.WithContext(ctx).Warn("SWEF hook not defined for auth")
	}
	return
}

func (auth *Auth) CompleteRecovery(ctx context.Context, recoveryPassword RecoveryPassword, cookies []*http.Cookie) (msg string, err error) {
	err = errors.New("CompleteRecovery Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return "", err
}

func (auth *Auth) VerifyRecovery(ctx context.Context, recoveryPassword RecoveryPassword) (res map[string]string, cookies []*http.Cookie, err error) {
	err = errors.New("VerifyRecovery Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return nil, nil, err
}

func (auth *Auth) VerifyToken(ctx context.Context, tokenType string, token string) (res interface{}, err error) {
	err = errors.New("VerifyToken Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return nil, err
}

func (auth *Auth) PerformPreSaveTask(ctx context.Context) (err error) {
	err = errors.New("PerformPreSaveTask Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return err
}
func (auth *Auth) PerformPreDeleteTask(ctx context.Context) (err error) {
	logs.WithContext(ctx).Warn("PerformPreDeleteTask Method not implemented")
	return nil
}

func (auth *Auth) GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "auth_type":
		return auth.AuthType, nil
	case "auth_name":
		return auth.AuthName, nil
	case "token_header_key":
		return auth.TokenHeaderKey, nil
	case "pkce":
		return auth.PKCE, nil
	case "key_id":
		return auth.KmsId, nil
	case "oauth_issuer":
		return auth.OAuthIssuer(ctx), nil
	case "oauth_server":
		return auth.OAuthServerConfig, nil
	default:
		if backend, backendErr := auth.tokenBackend(ctx); backendErr == nil {
			if attributeValue, ok := backend.GetAttribute(attributeName); ok {
				return attributeValue, nil
			}
		}
		err := errors.New("Attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}
func (auth *Auth) GetUserInfo(ctx context.Context, access_token string) (identity Identity, err error) {
	logs.WithContext(ctx).Debug("GetUserInfo - Start")
	identity, err = auth.tokenBackendUserInfo(ctx, access_token)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, errors.New("something went wrong - please try again")
	}
	logs.WithContext(ctx).Info(fmt.Sprint(identity))
	return auth.getUserInfo(ctx, identity.Id)
}

func (auth *Auth) getUserInfo(ctx context.Context, id string) (identity Identity, err error) {
	loginQuery := models.Queries{}
	loginQuery.Query = auth.AuthDb.GetDbQuery(ctx, SELECT_IDENTITY)
	loginQuery.Vals = append(loginQuery.Vals, id)
	loginQuery.Rank = 1
	logs.WithContext(ctx).Info(fmt.Sprint(auth.AuthDb.GetConn()))
	loginOutput, err := utils.ExecuteDbFetch(ctx, auth.AuthDb.GetConn(), loginQuery)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, errors.New("something went wrong - please try again")
	}

	if len(loginOutput) == 0 {
		err = errors.New("user not found")
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, err
	}
	identity.Id = loginOutput[0]["identity_id"].(string)
	identity.Status = loginOutput[0]["status"].(string)
	identity.Attributes = make(map[string]interface{})

	if attrs, attrsOk := loginOutput[0]["attributes"].(*map[string]interface{}); attrsOk {
		for k, v := range *attrs {
			identity.Attributes[k] = v
		}
	}
	if loginOutput[0]["idp_token"] != nil {
		identity.Attributes["idp_token"] = loginOutput[0]["idp_token"].(string)
	} else {
		identity.Attributes["idp_token"] = ""
	}

	if traits, traitsOk := loginOutput[0]["traits"].(*map[string]interface{}); traitsOk {
		for k, v := range *traits {
			if v != "" {
				identity.Attributes[k] = v
			}
		}
	}
	return
}

func (auth *Auth) GetUser(ctx context.Context, userId string) (identity Identity, err error) {
	err = errors.New("GetUser Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return Identity{}, err
}

func (auth *Auth) UpdateUser(ctx context.Context, identityToUpdate Identity, userId string, token map[string]interface{}) (tokens interface{}, err error) {
	err = errors.New("UpdateUser Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return nil, err
}

func (auth *Auth) GetTokens(ctx context.Context, code string) (res interface{}, err error) {
	logs.WithContext(ctx).Debug("GetTokens - Start")
	err = errors.New("UpdateUser Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return nil, err
}

func (auth *Auth) FetchTokens(ctx context.Context, refreshToken string, userId string) (res interface{}, err error) {
	logs.WithContext(ctx).Debug("FetchTokens - Start")
	backend, err := auth.tokenBackend(ctx)
	if err != nil {
		return nil, err
	}
	return backend.FetchTokens(ctx, refreshToken, userId)
}

func (auth *Auth) LoginApi(ctx context.Context, refreshToken string, userId string) (res interface{}, err error) {
	logs.WithContext(ctx).Info("LoginApi - Start")
	backend, err := auth.tokenBackend(ctx)
	if err != nil {
		return nil, err
	}
	return backend.LoginApi(ctx, refreshToken)
}

func (auth *Auth) makeTokens(ctx context.Context, identity Identity) (eruTokens LoginSuccess, err error) {
	backend, err := auth.tokenBackend(ctx)
	if err != nil {
		return LoginSuccess{}, err
	}
	eruTokens, err = backend.MakeTokens(ctx, identity)
	if err != nil {
		return
	}
	eruTokens.Id = identity.Id
	return
}

func (auth *Auth) Login(ctx context.Context, loginPostBody LoginPostBody, projectId string, withTokens bool, s storepkg.StoreI) (identity Identity, loginSuccess LoginSuccess, err error) {
	err = errors.New("Login Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return Identity{}, LoginSuccess{}, err
}

func (auth *Auth) IdpToken(ctx context.Context, idpToken LoginPostBody, projectId string, withTokens bool, renewFlag bool, s storepkg.StoreI) (loginResI interface{}, err error) {
	err = errors.New("IdpToken Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return nil, err
}

func (auth *Auth) Register(ctx context.Context, registerUser RegisterUser, projectId string) (identity Identity, loginSuccess LoginSuccess, err error) {
	err = errors.New("Register Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return Identity{}, LoginSuccess{}, err
}

func (auth *Auth) Logout(ctx context.Context, req *http.Request) (res interface{}, resStatusCode int, err error) {
	err = errors.New("Logout Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return nil, 400, err
}

func (auth *Auth) ChangePassword(ctx context.Context, tokenObj map[string]interface{}, userId string, changePasswordObj ChangePassword) (err error) {
	err = errors.New("ChangePassword Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return err
}

func GetAuth(authType string) AuthI {
	return NewAuth(authType)
}

func getTokenAttributes(ctx context.Context, token map[string]interface{}) (tokenObj map[string]interface{}, tokenErr bool) {
	if tokenIdentity, tokenIdentityOk := token["identity"]; tokenIdentityOk {
		if tokenIdentityMap, tokenIdentityMapOk := tokenIdentity.(map[string]interface{}); tokenIdentityMapOk {
			if tokenAttrs, tokenAttrsOk := tokenIdentityMap["attributes"]; tokenAttrsOk {
				if tokenAttrsMap, tokenAttrsMapOK := tokenAttrs.(map[string]interface{}); tokenAttrsMapOK {
					tokenObj = tokenAttrsMap
				} else {
					logs.WithContext(ctx).Error("token attributes is not a map")
					tokenErr = true
				}
			} else {
				logs.WithContext(ctx).Error("token attributes not found")
				tokenErr = true
			}
		} else {
			logs.WithContext(ctx).Error("token identity is not a map")
			tokenErr = true
		}
	} else {
		logs.WithContext(ctx).Error("token identity not found")
		tokenErr = true
	}
	return
}

func (auth *Auth) generateOtp(ctx context.Context, identity_credential string, identity_credential_type string, purpose string, silentFlag bool) (otp string, err error) {
	logs.WithContext(ctx).Debug("generateOtp - Start")
	if silentFlag {
		otp = "777777"
	} else {
		otp = fmt.Sprint(rand.Intn(999999-100000) + 100000)
	}
	otpQuery := models.Queries{}
	otpQuery.Query = auth.AuthDb.GetDbQuery(ctx, INSERT_OTP)
	otpQuery.Vals = append(otpQuery.Vals, uuid.New().String(), otp, identity_credential, identity_credential_type, purpose)
	otpQuery.Rank = 1

	_, err = utils.ExecuteDbFetch(ctx, auth.AuthDb.GetConn(), otpQuery)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("something went wrong - please try again")
	}
	return
}

func (auth *Auth) GenerateVerifyCode(ctx context.Context, verifyIdentifier VerifyPostBody, projectId string, silentFlag bool) (msg string, err error) {
	logs.WithContext(ctx).Debug("GenerateVerifyCode - Start")
	logs.WithContext(ctx).Info("GenerateVerifyCode Method not implemented")
	return
}

func (auth *Auth) VerifyCode(ctx context.Context, verifyCode VerifyCode, tokenObj map[string]interface{}, withToken bool) (res interface{}, err error) {
	logs.WithContext(ctx).Debug("VerifyCode - Start")
	logs.WithContext(ctx).Info("VerifyCode Method not implemented")
	return
}

func (auth *Auth) VerifyCodeNoUser(ctx context.Context, verifyCode VerifyCode) (res interface{}, err error) {
	logs.WithContext(ctx).Debug("VerifyCodeNoUser - Start")
	logs.WithContext(ctx).Info("VerifyCodeNoUser Method not implemented")
	return
}

func (auth *Auth) ApiTokenToUserToken(ctx context.Context, projectId string, apiTokenHash string) (identiy Identity, loginSuccess LoginSuccess, err error) {
	logs.WithContext(ctx).Info("ApiTokenToUserToken Method not implemented")
	return
}
func (auth *Auth) GetIdToken(ctx context.Context, projectId string, identityId string) (idToken string, err error) {
	logs.WithContext(ctx).Info("GetIdToken Method not implemented")
	return
}

func (auth *Auth) RemoveUser(ctx context.Context, removeUser RemoveUser) (err error) {
	logs.WithContext(ctx).Debug("RemoveUser - Start")

	var queries []*models.Queries
	idiQuery := models.Queries{}
	idiQuery.Query = auth.AuthDb.GetDbQuery(ctx, INSERT_DELETED_IDENTITY)
	idiQuery.Vals = append(idiQuery.Vals, removeUser.UserId)
	idiQuery.Rank = 1
	queries = append(queries, &idiQuery)

	dipQuery := models.Queries{}
	dipQuery.Query = auth.AuthDb.GetDbQuery(ctx, DELETE_IDENTITY_PASSWORD)
	dipQuery.Vals = append(dipQuery.Vals, removeUser.UserId)
	dipQuery.Rank = 2
	queries = append(queries, &dipQuery)

	dicQuery := models.Queries{}
	dicQuery.Query = auth.AuthDb.GetDbQuery(ctx, DELETE_IDENTITY_CREDENTIALS_BY_ID)
	dicQuery.Vals = append(dicQuery.Vals, removeUser.UserId)
	dicQuery.Rank = 3
	queries = append(queries, &dicQuery)

	diQuery := models.Queries{}
	diQuery.Query = auth.AuthDb.GetDbQuery(ctx, DELETE_IDENTITY)
	diQuery.Vals = append(diQuery.Vals, removeUser.UserId)
	diQuery.Rank = 4
	queries = append(queries, &diQuery)

	_, err = utils.ExecuteDbSave(ctx, auth.AuthDb.GetConn(), queries)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return errors.New("something went wrong - please try again")
	}
	return
}

func (auth *Auth) SetKms(ctx context.Context, kmsObj kms.KmsStoreI) (err error) {
	auth.KmsKey = kmsObj
	logs.WithContext(ctx).Info(fmt.Sprint(auth.KmsKey))
	return
}

func (auth *Auth) GetToken(ctx context.Context, projectId string, tokenKeyPrefix string, s storepkg.StoreI) (accessToken string, err error) {
	err = errors.New("GetToken Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

var tokenRefreshMu sync.Map

func tokenLockFor(projectId string, authName string) *sync.Mutex {
	key := fmt.Sprint(projectId, "/", authName)
	if v, ok := tokenRefreshMu.Load(key); ok {
		return v.(*sync.Mutex)
	}
	nm := &sync.Mutex{}
	actual, _ := tokenRefreshMu.LoadOrStore(key, nm)
	return actual.(*sync.Mutex)
}

func parseIdpTokens(res interface{}) (accessToken string, refreshToken string, expiresIn int64, ok bool) {
	m, mok := res.(map[string]interface{})
	if !mok {
		return "", "", 0, false
	}
	ok = true
	if at, atOk := m["access_token"].(string); atOk {
		accessToken = at
	}
	if rt, rtOk := m["refresh_token"].(string); rtOk {
		refreshToken = rt
	}
	switch ev := m["expires_in"].(type) {
	case float64:
		expiresIn = int64(ev)
	case int64:
		expiresIn = ev
	case int:
		expiresIn = int64(ev)
	case string:
		if v, perr := strconv.ParseInt(ev, 10, 64); perr == nil {
			expiresIn = v
		}
	}
	return
}

func tokenSecretKeys(name string) (accessKey, refreshKey, expiryKey string) {
	return name + "_access_token", name + "_refresh_token", name + "_token_expiry"
}

func resolveTokenKeyPrefix(tokenKeyPrefix, authName string) string {
	if tokenKeyPrefix != "" {
		return tokenKeyPrefix
	}
	return authName
}

func persistIdpTokens(ctx context.Context, s storepkg.StoreI, projectId string, authName string, accessToken string, refreshToken string, expiresIn int64) (err error) {
	if s == nil {
		err = errors.New("nil store passed to persistIdpTokens")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if authName == "" {
		err = errors.New("empty authName passed to persistIdpTokens")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	accessKey, refreshKey, expiryKey := tokenSecretKeys(authName)
	if accessToken != "" {
		if err = s.SaveSecret(ctx, projectId, storepkg.Secrets{Key: accessKey, SecretValue: accessToken}, s); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	}
	if refreshToken != "" {
		if err = s.SaveSecret(ctx, projectId, storepkg.Secrets{Key: refreshKey, SecretValue: refreshToken}, s); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	}
	if expiresIn > 0 {
		exp := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)
		if err = s.SaveSecret(ctx, projectId, storepkg.Secrets{Key: expiryKey, SecretValue: exp}, s); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	}
	return nil
}

func cachedAccessTokenIfValid(ctx context.Context, s storepkg.StoreI, projectId string, authName string) (accessToken string, valid bool) {
	if s == nil || authName == "" {
		return "", false
	}
	accessKey, _, expiryKey := tokenSecretKeys(authName)
	expStr, err := s.GetProjectSecret(ctx, projectId, expiryKey)
	if err != nil || expStr == "" {
		return "", false
	}
	t, err := time.Parse(time.RFC3339, expStr)
	if err != nil {
		return "", false
	}
	if time.Now().UTC().Add(5 * time.Minute).After(t) {
		return "", false
	}
	at, err := s.GetProjectSecret(ctx, projectId, accessKey)
	if err != nil || at == "" {
		return "", false
	}
	return at, true
}

func storedRefreshToken(ctx context.Context, s storepkg.StoreI, projectId string, authName string) (refreshToken string, err error) {
	if s == nil {
		err = errors.New("nil store passed to storedRefreshToken")
		return
	}
	if authName == "" {
		err = errors.New("empty authName passed to storedRefreshToken")
		return
	}
	_, refreshKey, _ := tokenSecretKeys(authName)
	refreshToken, err = s.GetProjectSecret(ctx, projectId, refreshKey)
	if err != nil {
		return
	}
	if refreshToken == "" {
		err = errors.New("no refresh token stored; bootstrap idptoken first")
	}
	return
}
