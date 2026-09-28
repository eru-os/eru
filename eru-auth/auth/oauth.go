package auth

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	auth_utils "github.com/eru-os/eru/eru-auth/utils"
	"github.com/eru-os/eru/eru-crypto/jwt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	storepkg "github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/uuid"
)

type OAuth struct {
	Auth
	OAuthConfig OAuthConfig `json:"oauth_config" eru:"required"`
}

type OAuthConfig struct {
	SsoUrlParams        map[string]string `json:"sso_url_params"`
	ClientId            string            `json:"client_id"`
	ClientSecret        string            `json:"client_secret"`
	RedirectURI         string            `json:"redirect_uri" eru:"required"`
	CodeKey             string            `json:"code_key"`
	TokenUrlContentType string            `json:"token_url_content_type"`
	Scope               string            `json:"scope"`
	SsoBaseUrl          string            `json:"sso_base_url" eru:"required"`
	TokenUrl            string            `json:"token_url" eru:"required"`
	JwkUrl              string            `json:"jwk_url"`
	CheckSum            string            `json:"checksum"`
	Identifiers         Identifiers       `json:"identifiers"`
	TokenKey            string            `json:"token_key"`
}

func (oAuth *OAuth) PerformPreSaveTask(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("PerformPreSaveTask - Start")
	return oAuth.saveTokenBackendClients(ctx)
}

func (oAuth *OAuth) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &oAuth)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (oAuth *OAuth) GetUrl(ctx context.Context, state string) (urlStr string, oAuthParams OAuthParams, err error) {
	oAuthParams.ClientId = oAuth.OAuthConfig.ClientId
	oAuthParams.Scope = oAuth.OAuthConfig.Scope
	oAuthParams.RedirectURI = oAuth.OAuthConfig.RedirectURI
	oAuthParams.ResponseType = "code"
	oAuthParams.State = state

	params := url.Values{}
	f := reflect.ValueOf(oAuthParams)
	for i := 0; i < f.NumField(); i++ {
		tags := f.Type().Field(i).Tag.Get("json")
		if tags != "redirect_key" {
			if tags != "" && f.Field(i).String() != "" {
				params.Add(tags, f.Field(i).String())
			}
		}
	}

	for k, v := range oAuth.OAuthConfig.SsoUrlParams {
		params.Add(k, v)
	}

	urlStr = fmt.Sprint(oAuth.OAuthConfig.SsoBaseUrl, "?", params.Encode())
	oAuthParams.Url = urlStr
	return
}

func (oAuth *OAuth) Login(ctx context.Context, loginPostBody LoginPostBody, projectId string, withTokens bool, s storepkg.StoreI) (identity Identity, loginSuccess LoginSuccess, err error) {
	logs.WithContext(ctx).Debug("Login - Start")

	idToken := ""
	sub := ""
	lMap := make(map[string]interface{})
	lMapOk := false
	_ = lMapOk

	headers := http.Header{}
	contentType := "application/x-www-form-urlencoded"
	if oAuth.OAuthConfig.TokenUrlContentType != "" {
		contentType = oAuth.OAuthConfig.TokenUrlContentType
	}
	headers.Set("Content-Type", contentType)

	glLoginFormBody := make(map[string]string)
	if oAuth.OAuthConfig.ClientId != "" {
		glLoginFormBody["client_id"] = oAuth.OAuthConfig.ClientId
	}

	if oAuth.OAuthConfig.ClientSecret != "" {
		glLoginFormBody["client_secret"] = oAuth.OAuthConfig.ClientSecret
	}

	if oAuth.OAuthConfig.RedirectURI != "" {
		glLoginFormBody["redirect_uri"] = oAuth.OAuthConfig.RedirectURI
	}

	if oAuth.OAuthConfig.CheckSum != "" {
		e := reflect.ValueOf(&oAuth.OAuthConfig).Elem()

		OAuthConfigMap := make(map[string]interface{})
		for i := 0; i < e.NumField(); i++ {
			k := e.Type().Field(i).Name
			kTag := e.Type().Field(i).Tag.Get("json")
			if kTag != "" {
				k = kTag
			}
			OAuthConfigMap[k] = e.Field(i).Interface()
		}
		OAuthConfigMap["authorization_token"] = loginPostBody.IdpCode
		cs, csErr := auth_utils.ExecuteTemplate(ctx, "oauth_checksum", oAuth.OAuthConfig.CheckSum, OAuthConfigMap, "string")
		if csErr != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("error in checksum template : ", csErr.Error()))
			return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
		}
		glLoginFormBody["checksum"] = string(cs)
	}

	if loginPostBody.IdpCode != "" {
		if oAuth.OAuthConfig.CodeKey == "" {
			glLoginFormBody["code"] = loginPostBody.IdpCode
		} else {
			glLoginFormBody[oAuth.OAuthConfig.CodeKey] = loginPostBody.IdpCode
		}

	}
	glLoginFormBody["grant_type"] = "authorization_code"

	var loginErr error
	var loginRes interface{}

	logs.WithContext(ctx).Info(fmt.Sprint(headers))
	logs.WithContext(ctx).Info(fmt.Sprint(glLoginFormBody))

	if contentType == "application/x-www-form-urlencoded" {
		loginRes, _, _, _, loginErr = utils.CallHttp(ctx, http.MethodPost, oAuth.OAuthConfig.TokenUrl, headers, glLoginFormBody, nil, nil, nil)
	} else if contentType == "application/json" {
		loginRes, _, _, _, loginErr = utils.CallHttp(ctx, http.MethodPost, oAuth.OAuthConfig.TokenUrl, headers, nil, nil, nil, glLoginFormBody)
	} else {
		loginErr = errors.New("invalid token api content type")
	}

	if loginErr != nil {
		logs.WithContext(ctx).Error(fmt.Sprint(map[string]interface{}{"request_id": loginPostBody.IdpRequestId, "error": fmt.Sprint(loginErr)}))
		return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
	}
	logs.WithContext(ctx).Info(fmt.Sprint(loginRes))
	if lMap, lMapOk = loginRes.(map[string]interface{}); lMapOk {
		vI, vIErr := utils.GetNestedFieldValue(ctx, lMap, oAuth.OAuthConfig.TokenKey)
		if vIErr != nil {
			logs.WithContext(ctx).Error(vIErr.Error())
			return Identity{}, LoginSuccess{}, vIErr
		}
		if lToken, lTokensOk := vI.(string); lTokensOk {
			idToken = lToken
		} else {
			logs.WithContext(ctx).Error("token could not be retrieved from IDP response")
		}

		subI, subIErr := utils.GetNestedFieldValue(ctx, lMap, oAuth.OAuthConfig.Identifiers.UserId.IdpMapper)
		if subIErr != nil {
			logs.WithContext(ctx).Error(vIErr.Error())
		} else {
			if subStr, subStrOk := subI.(string); subStrOk {
				sub = subStr
			} else {
				logs.WithContext(ctx).Warn("idp_sub could not be retrieved from IDP response")
			}
		}
	} else {
		logs.WithContext(ctx).Error("token API response received from IDP is not a map")
		return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
	}

	logs.WithContext(ctx).Info(idToken)
	tokenMap := make(map[string]interface{})
	tokenMapOk := false
	strTokenEmail := ""
	strTokenMobile := ""
	strTokenUserName := ""
	nonce := ""

	if idToken != "" && oAuth.OAuthConfig.JwkUrl != "" {
		tokens, tokensErr := jwt.DecryptTokenJWK(ctx, idToken, oAuth.OAuthConfig.JwkUrl)
		if tokensErr != nil {
			logs.WithContext(ctx).Error(tokensErr.Error())
			return Identity{}, LoginSuccess{}, tokensErr
		}
		if tokenMap, tokenMapOk = tokens.(map[string]interface{}); !tokenMapOk {
			logs.WithContext(ctx).Error("token recevied from IDP is not a map")
			return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
		}

	} else {
		if oAuth.Hooks.USRP != "" {
			logs.WithContext(ctx).Info(fmt.Sprint("calling USRP"))
			hookRes, hookErr := triggerHook(ctx, oAuth.Hooks.USRP, projectId, lMap)
			if hookErr != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("error while executing USRP hook :", hookErr.Error()))
				return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
			}
			logs.WithContext(ctx).Info(fmt.Sprint(hookRes))
			if tokenMap, tokenMapOk = hookRes.(map[string]interface{}); !tokenMapOk {
				logs.WithContext(ctx).Error(fmt.Sprint("USRP hook response is not a map"))
				return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
			}
		} else {
			logs.WithContext(ctx).Error(fmt.Sprint("USRP hook not defined"))
			//return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
		}
	}

	if tokenNonce, tokenNonceOk := tokenMap["nonce"]; tokenNonceOk {
		nonce = tokenNonce.(string)
	}
	if nonce != loginPostBody.Nonce {
		logs.WithContext(ctx).Error(fmt.Sprint("incorrect nonce : ", nonce, " expected nonce : ", loginPostBody.Nonce))
		return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
	}
	if tokenSub, tokenSubOk := tokenMap[oAuth.OAuthConfig.Identifiers.UserId.IdpMapper]; tokenSubOk {
		sub = tokenSub.(string)
	} else {
		tokenMap[oAuth.OAuthConfig.Identifiers.UserId.IdpMapper] = sub
	}
	if tokenEmail, tokenEmailOk := tokenMap[oAuth.OAuthConfig.Identifiers.Email.IdpMapper]; tokenEmailOk {
		strTokenEmail = tokenEmail.(string)
	}
	if tokenMobile, tokenMobileOk := tokenMap[oAuth.OAuthConfig.Identifiers.Mobile.IdpMapper]; tokenMobileOk {
		strTokenMobile = tokenMobile.(string)
	}
	if tokenUserName, tokenUserNameOk := tokenMap[oAuth.OAuthConfig.Identifiers.Username.IdpMapper]; tokenUserNameOk {
		strTokenUserName = tokenUserName.(string)
	}

	identity, err = oAuth.getIdpUser(ctx, sub, strTokenEmail, strTokenMobile, strTokenUserName, idToken)
	if err != nil {
		return
	}

	//creating Just-In-Time user if not found in eru database
	if identity.Id == "" {
		if oAuth.Hooks.USRR != "" {
			logs.WithContext(ctx).Info(fmt.Sprint(tokenMap))
			_, hookErr := triggerHook(ctx, oAuth.Hooks.USRR, projectId, tokenMap)
			if hookErr != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("error while executing USRR hook :", hookErr.Error()))
				return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
			}
		} else {
			logs.WithContext(ctx).Error(fmt.Sprint("USRR hook not defined"))
			return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
		}
		//execute query again
		identity, err = oAuth.getIdpUser(ctx, sub, strTokenEmail, strTokenMobile, strTokenUserName, idToken)
		if err != nil {
			return
		}

	}

	if withTokens {
		eruTokens, eruTokensErr := oAuth.makeTokens(ctx, identity)
		return identity, eruTokens, eruTokensErr
	}
	return identity, LoginSuccess{}, nil
}

func (oAuth *OAuth) GetUserInfo(ctx context.Context, access_token string) (identity Identity, err error) {
	logs.WithContext(ctx).Debug("GetUserInfo - Start")
	return oAuth.tokenBackendUserInfo(ctx, access_token)
}

func (oAuth *OAuth) Register(ctx context.Context, registerUser RegisterUser, projectId string) (identity Identity, tokens LoginSuccess, err error) {
	logs.WithContext(ctx).Debug("Register - Start")

	userTraits := UserTraits{}
	userAttrs := make(map[string]interface{})

	if identity.Attributes == nil {
		identity.Attributes = make(map[string]interface{})
	}
	identifierFound := false
	var requiredIdentifiers []string
	var insertQueries []*models.Queries
	identity.Id = uuid.New().String()

	userTraits.EmailVerified = true //for sso, consider email as verified if received from IDP
	identity.Attributes["email_verified"] = userTraits.EmailVerified
	if oAuth.OAuthConfig.Identifiers.Email.Enable {
		userTraits.Email = registerUser.Email

		identity.Attributes["email"] = userTraits.Email

		if registerUser.Email != "" {
			identifierFound = true
			insertQueryIcEmail := models.Queries{}
			insertQueryIcEmail.Query = oAuth.AuthDb.GetDbQuery(ctx, INSERT_IDENTITY_CREDENTIALS)
			insertQueryIcEmail.Vals = append(insertQueryIcEmail.Vals, uuid.New().String(), identity.Id, userTraits.Email, "email")
			insertQueryIcEmail.Rank = 2

			insertQueries = append(insertQueries, &insertQueryIcEmail)
		} else {
			requiredIdentifiers = append(requiredIdentifiers, "email")
		}
	}

	userTraits.MobileVerified = true //for sso, consider mobile as verified if received from IDP
	identity.Attributes["mobile_verified"] = userTraits.MobileVerified
	if oAuth.OAuthConfig.Identifiers.Mobile.Enable {
		userTraits.Mobile = registerUser.Mobile
		identity.Attributes["mobile"] = userTraits.Mobile
		if registerUser.Mobile != "" {
			identifierFound = true
			insertQueryIcMobile := models.Queries{}
			insertQueryIcMobile.Query = oAuth.AuthDb.GetDbQuery(ctx, INSERT_IDENTITY_CREDENTIALS)
			insertQueryIcMobile.Vals = append(insertQueryIcMobile.Vals, uuid.New().String(), identity.Id, userTraits.Mobile, "mobile")
			insertQueryIcMobile.Rank = 3
			insertQueries = append(insertQueries, &insertQueryIcMobile)
		} else {
			requiredIdentifiers = append(requiredIdentifiers, "mobile")
		}
	}
	if oAuth.OAuthConfig.Identifiers.Username.Enable {
		userTraits.Username = registerUser.Username
		identity.Attributes["username"] = userTraits.Username
		if registerUser.Username != "" {
			identifierFound = true
			insertQueryIcUsername := models.Queries{}
			insertQueryIcUsername.Query = oAuth.AuthDb.GetDbQuery(ctx, INSERT_IDENTITY_CREDENTIALS)
			insertQueryIcUsername.Vals = append(insertQueryIcUsername.Vals, uuid.New().String(), identity.Id, userTraits.Username, "userName")
			insertQueryIcUsername.Rank = 4
			insertQueries = append(insertQueries, &insertQueryIcUsername)
		} else {
			requiredIdentifiers = append(requiredIdentifiers, "userName")
		}
	}

	if !identifierFound {
		err = errors.New(fmt.Sprint("missing mandatory indentifiers : ", strings.Join(requiredIdentifiers, " , ")))
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, LoginSuccess{}, err
	}

	userTraits.FirstName = registerUser.FirstName
	identity.Attributes["first_name"] = userTraits.FirstName

	userTraits.LastName = registerUser.LastName
	identity.Attributes["last_name"] = userTraits.LastName

	userTraitsBytes, userTraitsBytesErr := json.Marshal(userTraits)
	if userTraitsBytesErr != nil {
		err = userTraitsBytesErr
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
	}

	identity.Status = "ACTIVE"
	identity.AuthDetails = IdentityAuth{}
	identity.Attributes["sub"] = identity.Id
	identity.Attributes["idp"] = oAuth.AuthName

	userAttrs["sub"] = identity.Id
	userAttrs["idp"] = oAuth.AuthName

	for k, v := range registerUser.UserAttributes {
		userAttrs[k] = v
		identity.Attributes[k] = v
	}

	userAttrsBytes, userAttrsBytesErr := json.Marshal(userAttrs)
	if userAttrsBytesErr != nil {
		err = userAttrsBytesErr
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
	}

	insertQuery := models.Queries{}
	insertQuery.Query = oAuth.AuthDb.GetDbQuery(ctx, INSERT_IDENTITY)
	insertQuery.Vals = append(insertQuery.Vals, identity.Id, oAuth.AuthName, identity.Attributes["idp_sub"], string(userTraitsBytes), string(userAttrsBytes))
	insertQuery.Rank = 1
	insertQueries = append(insertQueries, &insertQuery)

	sort.Sort(models.QueriesSorter(insertQueries))

	insertOutput, err := utils.ExecuteDbSave(ctx, oAuth.AuthDb.GetConn(), insertQueries)
	logs.WithContext(ctx).Info(fmt.Sprint(insertOutput))
	if err != nil {
		if strings.Contains(err.Error(), "unique_identity_credential") {
			return Identity{}, LoginSuccess{}, errors.New("username already exists")
		}
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
	}

	//tokens, err = oAuth.makeTokens(ctx, identity)

	if oAuth.Hooks.SWEF != "" {
		oAuth.sendWelcomeEmail(ctx, userTraits.Email, userTraits.FirstName, projectId, "email")
	} else {
		logs.WithContext(ctx).Info("SWEF hook not defined")
	}
	return identity, tokens, nil
}

func (oAuth *OAuth) getIdpUser(ctx context.Context, userId string, userEmail string, userMobile string, userName string, idToken string) (identity Identity, err error) {
	var output []map[string]interface{}
	var outputErr error
	if userId == "" {
		userId = "xx"
	}
	if userEmail == "" {
		userEmail = "xx"
	}
	if userMobile == "" {
		userMobile = "xx"
	}
	if userName == "" {
		userName = "xx"
	}
	query := models.Queries{}
	query.Query = oAuth.AuthDb.GetDbQuery(ctx, SELECT_IDENTITY_SUB)
	query.Vals = append(query.Vals, userId)
	query.Vals = append(query.Vals, userEmail)
	query.Vals = append(query.Vals, userMobile)
	query.Vals = append(query.Vals, userName)

	output, outputErr = utils.ExecuteDbFetch(ctx, oAuth.AuthDb.GetConn(), query)
	if outputErr != nil {
		err = outputErr
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, errors.New("something went wrong - please try again")
	}
	if identity.Attributes == nil {
		identity.Attributes = make(map[string]interface{})
	}
	logs.WithContext(ctx).Info(fmt.Sprint(output))
	if len(output) > 0 {
		identity.Id = output[0]["identity_id"].(string)
		if output[0]["is_active"].(bool) {
			identity.Status = "ACTIVE"
		} else {
			identity.Status = "INACTIVE"
		}
		identity.AuthDetails = IdentityAuth{}
		if traitsMap, traitsMapOk := output[0]["traits"].(*map[string]interface{}); traitsMapOk {
			for k, v := range *traitsMap {
				identity.Attributes[k] = v
			}
		}
		if attrMap, attrMapOk := output[0]["attributes"].(*map[string]interface{}); attrMapOk {
			for k, v := range *attrMap {
				identity.Attributes[k] = v
			}
		}

		idTokenToSave := []byte(idToken)
		if oAuth.KmsId != "" {
			idTokenToSave, err = oAuth.KmsKey.Encrypt(ctx, idTokenToSave)
		}
		idTokenToSaveStr := b64.StdEncoding.EncodeToString(idTokenToSave)

		idptoken_query := models.Queries{}
		idptoken_query.Query = oAuth.AuthDb.GetDbQuery(ctx, UPDATE_IDP_TOKEN)
		idptoken_query.Vals = append(idptoken_query.Vals, idTokenToSaveStr)
		idptoken_query.Vals = append(idptoken_query.Vals, identity.Id)
		_, outputErr = utils.ExecuteDbFetch(ctx, oAuth.AuthDb.GetConn(), idptoken_query)
		if outputErr != nil {
			err = outputErr
			logs.WithContext(ctx).Error(err.Error())
			return Identity{}, errors.New("something went wrong - please try again")
		}

		identity.Attributes["idp_token"] = idTokenToSaveStr
	}
	return
}
func (oAuth *OAuth) GetTokens(ctx context.Context, code string) (res interface{}, err error) {
	query := models.Queries{}
	query.Query = oAuth.AuthDb.GetDbQuery(ctx, SELECT_TEMP_CODE)
	query.Vals = append(query.Vals, code)
	output, outputErr := utils.ExecuteDbFetch(ctx, oAuth.AuthDb.GetConn(), query)
	if outputErr != nil {
		err = outputErr
		logs.WithContext(ctx).Error(err.Error())
		return Identity{}, err
	}
	if len(output) > 0 {
		if tokens, tokensOk := output[0]["tokens"].(*map[string]interface{}); !tokensOk {
			logs.WithContext(ctx).Error("tokens not found in GetTokens")
			return res, errors.New("something went wrong - please try again")
		} else {
			return tokens, nil
		}
	}
	return res, errors.New("invalid code")
}

func (oAuth *OAuth) GenerateTempCode(ctx context.Context, id string, tokens map[string]interface{}) (code string, err error) {
	tokenBytes, tokenBytesErr := json.Marshal(tokens)
	if tokenBytesErr != nil {
		err = tokenBytesErr
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("something went wrong - please try again")
	}
	code = uuid.New().String()
	idptoken_query := models.Queries{}
	idptoken_query.Query = oAuth.AuthDb.GetDbQuery(ctx, INSERT_TEMP_CODE)
	idptoken_query.Vals = append(idptoken_query.Vals, id)
	idptoken_query.Vals = append(idptoken_query.Vals, code)
	idptoken_query.Vals = append(idptoken_query.Vals, string(tokenBytes))
	_, err = utils.ExecuteDbFetch(ctx, oAuth.AuthDb.GetConn(), idptoken_query)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("invalid code")
	}
	return
}
func (oAuth *OAuth) IdpToken(ctx context.Context, loginPostBody LoginPostBody, projectId string, withTokens bool, renewFlag bool, s storepkg.StoreI) (loginResI interface{}, err error) {
	logs.WithContext(ctx).Debug("IdpToken - Start")

	headers := http.Header{}

	contentType := "application/x-www-form-urlencoded"
	if oAuth.OAuthConfig.TokenUrlContentType != "" {
		contentType = oAuth.OAuthConfig.TokenUrlContentType
	}
	headers.Set("Content-Type", contentType)

	oAuthLoginFormBody := make(map[string]string)
	if oAuth.OAuthConfig.ClientId != "" {
		oAuthLoginFormBody["client_id"] = oAuth.OAuthConfig.ClientId
	}

	if oAuth.OAuthConfig.ClientSecret != "" {
		oAuthLoginFormBody["client_secret"] = oAuth.OAuthConfig.ClientSecret
	}

	if oAuth.OAuthConfig.RedirectURI != "" {
		oAuthLoginFormBody["redirect_uri"] = oAuth.OAuthConfig.RedirectURI
	}

	if oAuth.OAuthConfig.CheckSum != "" {
		e := reflect.ValueOf(&oAuth.OAuthConfig).Elem()

		OAuthConfigMap := make(map[string]interface{})
		for i := 0; i < e.NumField(); i++ {
			k := e.Type().Field(i).Name
			kTag := e.Type().Field(i).Tag.Get("json")
			if kTag != "" {
				k = kTag
			}
			OAuthConfigMap[k] = e.Field(i).Interface()
		}
		oAuthLoginFormBody["authorization_token"] = loginPostBody.IdpCode
		cs, csErr := auth_utils.ExecuteTemplate(ctx, "oauth_checksum", oAuth.OAuthConfig.CheckSum, OAuthConfigMap, "string")
		if csErr != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("error in checksum template : ", csErr.Error()))
			return nil, errors.New("something went wrong - please try again")
		}
		oAuthLoginFormBody["checksum"] = string(cs)
	}

	if renewFlag {
		oAuthLoginFormBody["grant_type"] = "refresh_token"
		oAuthLoginFormBody["refresh_token"] = loginPostBody.RefreshToken
	} else {
		if loginPostBody.IdpCode != "" {
			if oAuth.OAuthConfig.CodeKey == "" {
				oAuthLoginFormBody["code"] = loginPostBody.IdpCode
			} else {
				oAuthLoginFormBody[oAuth.OAuthConfig.CodeKey] = loginPostBody.IdpCode
			}
		}
		oAuthLoginFormBody["grant_type"] = "authorization_code"
	}

	var loginErr error
	var loginRes interface{}
	if contentType == "application/x-www-form-urlencoded" {
		loginRes, _, _, _, loginErr = utils.CallHttp(ctx, http.MethodPost, oAuth.OAuthConfig.TokenUrl, headers, oAuthLoginFormBody, nil, nil, nil)
	} else if contentType == "application/json" {
		loginRes, _, _, _, loginErr = utils.CallHttp(ctx, http.MethodPost, oAuth.OAuthConfig.TokenUrl, headers, nil, nil, nil, oAuthLoginFormBody)
	} else {
		loginErr = errors.New("invalid token api content type")
	}
	if loginErr != nil {
		logs.WithContext(ctx).Error(fmt.Sprint(map[string]interface{}{"request_id": loginPostBody.IdpRequestId, "error": fmt.Sprint(loginErr)}))
		return nil, errors.New("something went wrong - please try again")
	}
	return loginRes, nil
}

func init() {
	RegisterAuth("OAUTH", func() AuthI { return new(OAuth) })
}
