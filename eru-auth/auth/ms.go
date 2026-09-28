package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/eru-os/eru/eru-crypto/jwt"
	erupkce "github.com/eru-os/eru/eru-crypto/pkce"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	storepkg "github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/uuid"
)

type MsAuth struct {
	Auth
	MsConfig MsConfig `json:"ms_config" eru:"required"`
}

type MsConfig struct {
	AuthConfig
	PersistToken bool `json:"persist_token"`
}

func (msAuth *MsAuth) PerformPreSaveTask(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("PerformPreSaveTask - Start")
	return msAuth.saveTokenBackendClients(ctx)
}

func (msAuth *MsAuth) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &msAuth)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (msAuth *MsAuth) GetUrl(ctx context.Context, state string) (urlStr string, msParams OAuthParams, err error) {
	codeVerifier, codeChallenge, err := erupkce.NewPKCE(ctx)
	msParams.ClientId = msAuth.MsConfig.ClientId
	msParams.Scope = msAuth.MsConfig.Scope
	msParams.RedirectURI = msAuth.MsConfig.RedirectURI
	msParams.ResponseType = "code"
	msParams.ResponseMode = "fragment"
	msParams.ClientRequestId = uuid.New().String()
	msParams.CodeChallenge = codeChallenge
	msParams.CodeVerifier = codeVerifier
	msParams.CodeChallengeMethod = "S256"
	msParams.Nonce = uuid.New().String()
	msParams.State = state
	params := url.Values{}
	f := reflect.ValueOf(msParams)
	for i := 0; i < f.NumField(); i++ {
		tags := f.Type().Field(i).Tag.Get("json")
		if tags != "" {
			params.Add(tags, f.Field(i).String())
		}
	}

	urlStr = fmt.Sprint(msAuth.MsConfig.SsoBaseUrl, "?", params.Encode())
	msParams.Url = urlStr
	return
}

func (msAuth *MsAuth) Login(ctx context.Context, loginPostBody LoginPostBody, projectId string, withTokens bool, s storepkg.StoreI) (identity Identity, loginSuccess LoginSuccess, err error) {
	logs.WithContext(ctx).Debug("Login - Start")

	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")

	msLoginFormBody := make(map[string]string)
	msLoginFormBody["client_id"] = msAuth.MsConfig.ClientId
	msLoginFormBody["client_secret"] = msAuth.MsConfig.ClientSecret
	msLoginFormBody["redirect_uri"] = msAuth.MsConfig.RedirectURI
	msLoginFormBody["grant_type"] = "authorization_code"
	msLoginFormBody["scope"] = msAuth.MsConfig.Scope
	msLoginFormBody["code"] = loginPostBody.IdpCode
	msLoginFormBody["code_verifier"] = loginPostBody.CodeVerifier

	loginRes, _, _, _, loginErr := utils.CallHttp(ctx, http.MethodPost, msAuth.MsConfig.TokenUrl, headers, msLoginFormBody, nil, nil, nil)
	if loginErr != nil {
		logs.WithContext(ctx).Error(fmt.Sprint(map[string]interface{}{"request_id": loginPostBody.IdpRequestId, "error": fmt.Sprint(loginErr)}))
		return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
	}

	if msAuth.MsConfig.PersistToken {
		if at, rt, ei, ok := parseIdpTokens(loginRes); ok {
			keyName := resolveTokenKeyPrefix(loginPostBody.TokenKeyPrefix, msAuth.AuthName)
			if perr := persistIdpTokens(ctx, s, projectId, keyName, at, rt, ei); perr != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("persistIdpTokens: ", perr.Error()))
			}
		}
	}

	idToken := ""
	if lMap, lMapOk := loginRes.(map[string]interface{}); lMapOk {

		if lToken, lTokensOk := lMap["id_token"]; lTokensOk {
			logs.WithContext(ctx).Info(fmt.Sprint(loginRes))
			idToken = lToken.(string)
		}
	}

	tokens, tokensErr := jwt.DecryptTokenJWK(ctx, idToken, msAuth.MsConfig.JwkUrl)
	if tokensErr != nil {
		logs.WithContext(ctx).Error(tokensErr.Error())
		return Identity{}, LoginSuccess{}, tokensErr
	}

	nonce := ""
	sub := ""
	userTraits := UserTraits{}
	userAttrs := make(map[string]string)

	if tokenMap, tokenMapOk := tokens.(map[string]interface{}); tokenMapOk {
		if tokenNonce, tokenNonceOk := tokenMap["nonce"]; tokenNonceOk {
			nonce = tokenNonce.(string)
		}
		if nonce != loginPostBody.Nonce {
			logs.WithContext(ctx).Error(fmt.Sprint("incorrect nonce : ", nonce, " expected nonce : ", loginPostBody.Nonce))
			return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
		}

		if tokenSub, tokenSubOk := tokenMap["sub"]; tokenSubOk {
			sub = tokenSub.(string)
		}
		query := models.Queries{}
		query.Query = msAuth.AuthDb.GetDbQuery(ctx, SELECT_IDENTITY_SUB)
		query.Vals = append(query.Vals, sub)
		logs.WithContext(ctx).Info(fmt.Sprint(tokenMap))
		if tokenEmail, tokenEmailOk := tokenMap[msAuth.MsConfig.Identifiers.Email.IdpMapper]; tokenEmailOk {
			query.Vals = append(query.Vals, tokenEmail)
		} else {
			query.Vals = append(query.Vals, "")
		}

		query.Vals = append(query.Vals, sub)
		query.Vals = append(query.Vals, sub)

		output, outputErr := utils.ExecuteDbFetch(ctx, msAuth.AuthDb.GetConn(), query)
		if outputErr != nil {
			err = outputErr
			logs.WithContext(ctx).Error(err.Error())
			return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
		}

		if identity.Attributes == nil {
			identity.Attributes = make(map[string]interface{})
		}
		//creating Just-In-Time user if not found in eru database
		if len(output) == 0 {

			var insertQueries []*models.Queries
			identity.Id = uuid.New().String()
			identifierFound := false
			var requiredIdentifiers []string
			logs.WithContext(ctx).Info(fmt.Sprint(msAuth.MsConfig.Identifiers))
			if msAuth.MsConfig.Identifiers.Email.Enable {
				if tokenEmail, tokenEmailOk := tokenMap[msAuth.MsConfig.Identifiers.Email.IdpMapper]; tokenEmailOk {
					userTraits.Email = tokenEmail.(string)
					userTraits.EmailVerified = true
					identity.Attributes["email"] = userTraits.Email
					identity.Attributes["email_verified"] = userTraits.EmailVerified // for sso email is considered as verified

					identifierFound = true
					insertQueryIcEmail := models.Queries{}
					insertQueryIcEmail.Query = msAuth.AuthDb.GetDbQuery(ctx, INSERT_IDENTITY_CREDENTIALS)
					insertQueryIcEmail.Vals = append(insertQueryIcEmail.Vals, uuid.New().String(), identity.Id, userTraits.Email, "email")
					insertQueryIcEmail.Rank = 2

					insertQueries = append(insertQueries, &insertQueryIcEmail)
				} else {
					userTraits.Email = ""
					identity.Attributes["email"] = ""
					requiredIdentifiers = append(requiredIdentifiers, "email")
				}
			}
			if msAuth.MsConfig.Identifiers.Mobile.Enable {
				if tokenMobile, tokenMobileOk := tokenMap[msAuth.MsConfig.Identifiers.Mobile.IdpMapper]; tokenMobileOk {
					userTraits.Mobile = tokenMobile.(string)
					identity.Attributes["mobile"] = userTraits.Mobile

					identifierFound = true
					insertQueryIcMobile := models.Queries{}
					insertQueryIcMobile.Query = msAuth.AuthDb.GetDbQuery(ctx, INSERT_IDENTITY_CREDENTIALS)
					insertQueryIcMobile.Vals = append(insertQueryIcMobile.Vals, uuid.New().String(), identity.Id, userTraits.Mobile, "mobile")
					insertQueryIcMobile.Rank = 3
					insertQueries = append(insertQueries, &insertQueryIcMobile)
				} else {
					userTraits.Mobile = ""
					identity.Attributes["mobile"] = ""
					requiredIdentifiers = append(requiredIdentifiers, "mobile")
				}
			}
			if msAuth.MsConfig.Identifiers.Username.Enable {
				if tokenUsername, tokenUsernameOk := tokenMap[msAuth.MsConfig.Identifiers.Username.IdpMapper]; tokenUsernameOk {
					userTraits.Username = tokenUsername.(string)
					identity.Attributes["username"] = userTraits.Username
					identifierFound = true
					insertQueryIcUsername := models.Queries{}
					insertQueryIcUsername.Query = msAuth.AuthDb.GetDbQuery(ctx, INSERT_IDENTITY_CREDENTIALS)
					insertQueryIcUsername.Vals = append(insertQueryIcUsername.Vals, uuid.New().String(), identity.Id, userTraits.Username, "userName")
					insertQueryIcUsername.Rank = 4
					insertQueries = append(insertQueries, &insertQueryIcUsername)
				} else {
					userTraits.Username = ""
					identity.Attributes["username"] = ""
					requiredIdentifiers = append(requiredIdentifiers, "username")
				}
			}

			if !identifierFound {
				err = errors.New(fmt.Sprint("missing mandatory indentifiers : ", strings.Join(requiredIdentifiers, " , ")))
				logs.WithContext(ctx).Error(err.Error())
				return Identity{}, LoginSuccess{}, err
			}

			name := ""
			if tokenName, tokenNameOk := tokenMap["name"]; tokenNameOk {
				name = tokenName.(string)
			}
			nameArray := strings.Split(name, " ")
			userTraits.FirstName = nameArray[0]
			identity.Attributes["first_name"] = userTraits.FirstName
			if len(nameArray) > 1 {
				userTraits.LastName = nameArray[len(nameArray)-1]
				identity.Attributes["last_name"] = userTraits.LastName
			}
			logs.WithContext(ctx).Info(fmt.Sprint(userTraits))
			userTraitsBytes, userTraitsBytesErr := json.Marshal(userTraits)
			if userTraitsBytesErr != nil {
				err = userTraitsBytesErr
				logs.WithContext(ctx).Error(err.Error())
				return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
			}
			logs.WithContext(ctx).Info(fmt.Sprint(string(userTraitsBytes)))
			identity.Status = "ACTIVE"
			identity.AuthDetails = IdentityAuth{}
			identity.Attributes["sub"] = identity.Id
			identity.Attributes["idp"] = msAuth.AuthName
			identity.Attributes["idp_sub"] = sub

			userAttrs["sub"] = identity.Id
			userAttrs["idp"] = msAuth.AuthName
			userAttrs["idp_sub"] = sub

			userAttrsBytes, userAttrsBytesErr := json.Marshal(userAttrs)
			if userAttrsBytesErr != nil {
				err = userAttrsBytesErr
				logs.WithContext(ctx).Error(err.Error())
				return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
			}

			insertQuery := models.Queries{}
			insertQuery.Query = msAuth.AuthDb.GetDbQuery(ctx, INSERT_IDENTITY)
			insertQuery.Vals = append(insertQuery.Vals, identity.Id, msAuth.AuthName, sub, string(userTraitsBytes), string(userAttrsBytes))
			insertQuery.Rank = 1
			insertQueries = append(insertQueries, &insertQuery)

			sort.Sort(models.QueriesSorter(insertQueries))

			insertOutput, err := utils.ExecuteDbSave(ctx, msAuth.AuthDb.GetConn(), insertQueries)
			_ = insertOutput
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
			}

			if msAuth.Hooks.SWEF != "" {
				msAuth.sendWelcomeEmail(ctx, userTraits.Email, userTraits.FirstName, "", "email")
			} else {
				logs.WithContext(ctx).Info("SWEF hook not defined")
			}

		} else {
			logs.WithContext(ctx).Info(fmt.Sprint(output))
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
		}
	} else {
		logs.WithContext(ctx).Error("token recevied from IDP is not a map")
		return Identity{}, LoginSuccess{}, errors.New("something went wrong - please try again")
	}

	if withTokens {
		eruTokens, tokensErr := msAuth.makeTokens(ctx, identity)
		if tokensErr != nil {
			err = tokensErr
			return
		}
		logs.WithContext(ctx).Info(fmt.Sprint(identity))
		logs.WithContext(ctx).Info(fmt.Sprint(eruTokens))
		return identity, eruTokens, nil
	}
	return identity, LoginSuccess{}, nil
}

func (msAuth *MsAuth) IdpToken(ctx context.Context, loginPostBody LoginPostBody, projectId string, withTokens bool, renewFlag bool, s storepkg.StoreI) (loginResI interface{}, err error) {
	logs.WithContext(ctx).Debug("IdpToken - Start")

	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")

	msLoginFormBody := make(map[string]string)
	msLoginFormBody["client_id"] = msAuth.MsConfig.ClientId
	msLoginFormBody["client_secret"] = msAuth.MsConfig.ClientSecret
	msLoginFormBody["redirect_uri"] = msAuth.MsConfig.RedirectURI
	msLoginFormBody["scope"] = msAuth.MsConfig.Scope
	if renewFlag {
		msLoginFormBody["grant_type"] = "refresh_token"
		msLoginFormBody["refresh_token"] = loginPostBody.RefreshToken
	} else {
		msLoginFormBody["code"] = loginPostBody.IdpCode
		msLoginFormBody["code_verifier"] = loginPostBody.CodeVerifier
		msLoginFormBody["grant_type"] = "authorization_code"
	}

	loginRes, _, _, _, loginErr := utils.CallHttp(ctx, http.MethodPost, msAuth.MsConfig.TokenUrl, headers, msLoginFormBody, nil, nil, nil)
	if loginErr != nil {
		logs.WithContext(ctx).Error(fmt.Sprint(map[string]interface{}{"request_id": loginPostBody.IdpRequestId, "error": fmt.Sprint(loginErr)}))
		return nil, errors.New("something went wrong - please try again")
	}
	if msAuth.MsConfig.PersistToken {
		if at, rt, ei, ok := parseIdpTokens(loginRes); ok {
			keyName := resolveTokenKeyPrefix(loginPostBody.TokenKeyPrefix, msAuth.AuthName)
			if perr := persistIdpTokens(ctx, s, projectId, keyName, at, rt, ei); perr != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("persistIdpTokens: ", perr.Error()))
			}
		}
	}
	return loginRes, nil
}

func (msAuth *MsAuth) GetToken(ctx context.Context, projectId string, tokenKeyPrefix string, s storepkg.StoreI) (accessToken string, err error) {
	logs.WithContext(ctx).Debug("GetToken - Start")
	if !msAuth.MsConfig.PersistToken {
		err = errors.New("persist_token not enabled for this auth config")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	keyName := resolveTokenKeyPrefix(tokenKeyPrefix, msAuth.AuthName)
	mu := tokenLockFor(projectId, keyName)
	mu.Lock()
	defer mu.Unlock()

	if at, ok := cachedAccessTokenIfValid(ctx, s, projectId, keyName); ok {
		return at, nil
	}
	rt, rerr := storedRefreshToken(ctx, s, projectId, keyName)
	if rerr != nil {
		err = rerr
		return
	}
	res, ierr := msAuth.IdpToken(ctx, LoginPostBody{RefreshToken: rt, TokenKeyPrefix: tokenKeyPrefix}, projectId, false, true, s)
	if ierr != nil {
		err = ierr
		return
	}
	at, _, _, ok := parseIdpTokens(res)
	if !ok || at == "" {
		err = errors.New("refresh did not return an access_token")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return at, nil
}

func (msAuth *MsAuth) GetUserInfo(ctx context.Context, access_token string) (identity Identity, err error) {
	logs.WithContext(ctx).Debug("GetUserInfo - Start")
	return msAuth.tokenBackendUserInfo(ctx, access_token)
}

func init() {
	RegisterAuth("MICROSOFT", func() AuthI { return new(MsAuth) })
}
