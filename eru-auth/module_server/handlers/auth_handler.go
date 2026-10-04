package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

func UserInfoHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("UserInfoHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		userInfoFromReq := json.NewDecoder(r.Body)
		userInfoFromReq.DisallowUnknownFields()
		userInfoObj := make(map[string]interface{})
		//storageObj := new(storage.Storage)
		if err := userInfoFromReq.Decode(&userInfoObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		accessTokenStr := ""
		if accessToken, ok := userInfoObj["access_token"]; !ok {
			atErr := errors.New("access_token attribute missing in request body")
			logs.WithContext(r.Context()).Error(atErr.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": atErr})
			return
		} else {
			if accessTokenStr, ok = accessToken.(string); !ok {
				atErr := errors.New("Incorrect access_token recevied in request body")
				logs.WithContext(r.Context()).Error(atErr.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": atErr})
				return
			}
		}

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}
		identity, err := authObjI.GetUserInfo(r.Context(), accessTokenStr)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(identity)
		return
	}
}

func LoginApiHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("LoginApiHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		fetchTokenFromReq := json.NewDecoder(r.Body)

		fetchTokenFromReq.DisallowUnknownFields()
		type fetchToken struct {
			RefreshToken string `json:"refresh_token"`
			Id           string `json:"id" eru:"required"`
		}
		var fetchTokenObj fetchToken

		if err := fetchTokenFromReq.Decode(&fetchTokenObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), fetchTokenObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}

		refreshToken := refreshTokenFromRequest(r, authObjI, fetchTokenObj.RefreshToken)
		if refreshToken == "" {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "missing field in object : RefreshToken"})
			return
		}
		loginSuccess, err := authObjI.LoginApi(r.Context(), refreshToken, fetchTokenObj.Id)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(setRefreshCookieFromResult(w, r, authObjI, loginSuccess))
		return
	}
}

func GetUserTokensHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("GetUserTokensHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		fetchTokenFromReq := json.NewDecoder(r.Body)

		fetchTokenFromReq.DisallowUnknownFields()
		type fetchToken struct {
			ApiToken string `json:"token" eru:"required"`
		}
		var fetchTokenObj fetchToken

		if err := fetchTokenFromReq.Decode(&fetchTokenObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), fetchTokenObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}

		_, loginSuccess, err := authObjI.ApiTokenToUserToken(r.Context(), projectId, fetchTokenObj.ApiToken)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(loginSuccess)
		return
	}
}

func GetIdTokenHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("GetUserTokensHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		fetchTokenFromReq := json.NewDecoder(r.Body)

		fetchTokenFromReq.DisallowUnknownFields()
		type fetchToken struct {
			Id string `json:"id" eru:"required"`
		}
		var fetchTokenObj fetchToken

		if err := fetchTokenFromReq.Decode(&fetchTokenObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), fetchTokenObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}

		idToken, err := authObjI.GetIdToken(r.Context(), projectId, fetchTokenObj.Id)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id_token": idToken})
		return
	}
}

func GetTokensHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("GetTokensHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		fetchTokenFromReq := json.NewDecoder(r.Body)

		fetchTokenFromReq.DisallowUnknownFields()
		type fetchToken struct {
			Code string `json:"code" eru:"required"`
		}
		var fetchTokenObj fetchToken

		if err := fetchTokenFromReq.Decode(&fetchTokenObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), fetchTokenObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}

		loginSuccess, err := authObjI.GetTokens(r.Context(), fetchTokenObj.Code)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(loginSuccess)
		return
	}
}

func GenerateTempCodeHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("GenerateTempCodeHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		fetchTokenFromReq := json.NewDecoder(r.Body)

		fetchTokenFromReq.DisallowUnknownFields()
		type fetchToken struct {
			Id     string                 `json:"id" eru:"required"`
			Tokens map[string]interface{} `json:"tokens" eru:"required"`
		}
		var fetchTokenObj fetchToken

		if err := fetchTokenFromReq.Decode(&fetchTokenObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), fetchTokenObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}

		code, err := authObjI.GenerateTempCode(r.Context(), fetchTokenObj.Id, fetchTokenObj.Tokens)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": code})
		return
	}
}

func FetchTokensHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("FetchTokensHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		fetchTokenFromReq := json.NewDecoder(r.Body)

		fetchTokenFromReq.DisallowUnknownFields()
		type fetchToken struct {
			RefreshToken string `json:"refresh_token"`
			Id           string `json:"id" eru:"required"`
		}
		var fetchTokenObj fetchToken

		if err := fetchTokenFromReq.Decode(&fetchTokenObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), fetchTokenObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}

		refreshToken := refreshTokenFromRequest(r, authObjI, fetchTokenObj.RefreshToken)
		if refreshToken == "" {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "missing field in object : RefreshToken"})
			return
		}
		loginSuccess, err := authObjI.FetchTokens(r.Context(), refreshToken, fetchTokenObj.Id)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(setRefreshCookieFromResult(w, r, authObjI, loginSuccess))
		return
	}
}

func VerifyTokenHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("VerifyTokenHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tokenType := vars["tokentype"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		tokenHeaderKey, err := authObjI.GetAttribute(r.Context(), "token_header_key")
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		tokenToVerify := r.Header.Get(tokenHeaderKey.(string))

		res, err := authObjI.VerifyToken(r.Context(), tokenType, tokenToVerify)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
		return
	}
}
func IdpTokenHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Info("IdpTokenHandler - Start")
		ctx := context.WithValue(r.Context(), "Erufuncbaseurl", module_store.Erufuncbaseurl)
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]
		renew := vars["renew"]
		renewFlag := false
		if renew == "renew" {
			renewFlag = true
		}
		authObjI, err := sh.Store.GetAuth(ctx, projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(ctx).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}
		loginPostBodyFromReq := json.NewDecoder(r.Body)
		loginPostBodyFromReq.DisallowUnknownFields()

		var loginPostBody auth.LoginPostBody

		if err = loginPostBodyFromReq.Decode(&loginPostBody); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		msParams := auth.OAuthParams{}
		pkceRequired := false
		pkceRequiredI, pkceErr := authObjI.GetAttribute(ctx, "pkce")
		if pkceErr != nil {
			logs.WithContext(ctx).Error(pkceErr.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": pkceErr.Error()})
			return
		}
		pkceRequired, ok := pkceRequiredI.(bool)
		if !ok {
			logs.WithContext(ctx).Error("pkceRequired is not a boolean")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "pkceRequired is not a boolean"})
			return
		}
		if pkceRequired {
			msParams, err = sh.Store.GetPkceEvent(ctx, loginPostBody.IdpRequestId, sh.Store)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}

		loginPostBody.CodeVerifier = msParams.CodeVerifier
		loginPostBody.Nonce = msParams.Nonce

		res, err := authObjI.IdpToken(ctx, loginPostBody, projectId, true, renewFlag, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, http.StatusOK)
			_ = json.NewEncoder(w).Encode(res)
			return
		}
	}
}

func GetTokenHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Info("GetTokenHandler - Start")
		ctx := r.Context()
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(ctx, projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		}

		tokenKeyPrefix := r.URL.Query().Get("token_key_prefix")
		accessToken, err := authObjI.GetToken(ctx, projectId, tokenKeyPrefix, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": accessToken})
	}
}

func LoginHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Info("LoginHandler - Start")
		ctx := context.WithValue(r.Context(), "Erufuncbaseurl", module_store.Erufuncbaseurl)
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]
		logs.WithContext(r.Context()).Info(projectId)
		logs.WithContext(r.Context()).Info(authName)
		authObjI, err := sh.Store.GetAuth(ctx, projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		logs.WithContext(r.Context()).Info(fmt.Sprint(authObjI))

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(ctx).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}
		logs.WithContext(r.Context()).Info(fmt.Sprint("auth conn is set"))
		utils.PrintRequestBody(r.Context(), r, "from login handler in eruauth")
		loginPostBodyFromReq := json.NewDecoder(r.Body)
		loginPostBodyFromReq.DisallowUnknownFields()

		var loginPostBody auth.LoginPostBody

		if err = loginPostBodyFromReq.Decode(&loginPostBody); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		logs.WithContext(r.Context()).Info(fmt.Sprint(loginPostBody))
		msParams := auth.OAuthParams{}
		pkceRequired := false
		pkceRequiredI, pkceErr := authObjI.GetAttribute(ctx, "pkce")
		if pkceErr != nil {
			logs.WithContext(ctx).Error(pkceErr.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": pkceErr.Error()})
			return
		}
		pkceRequired, ok := pkceRequiredI.(bool)
		if !ok {
			logs.WithContext(ctx).Error("pkceRequired is not a boolean")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "pkceRequired is not a boolean"})
			return
		}
		if pkceRequired {
			msParams, err = sh.Store.GetPkceEvent(ctx, loginPostBody.IdpRequestId, sh.Store)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}

		loginPostBody.CodeVerifier = msParams.CodeVerifier
		loginPostBody.Nonce = msParams.Nonce
		logs.WithContext(r.Context()).Info(fmt.Sprint("before login = ", loginPostBody))

		res, tokens, err := authObjI.Login(ctx, loginPostBody, projectId, true, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			setRefreshCookie(w, r, authObjI, tokens.RefreshToken)
			server_handlers.FormatResponse(w, http.StatusOK)
			if tokens.IdToken != "" {
				_ = json.NewEncoder(w).Encode(hideRefreshToken(r, authObjI, tokens))
			} else {
				_ = json.NewEncoder(w).Encode(res)
			}
			return
		}
	}
}

func GetRecoveryCodeHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("GetRecoveryCodeHandler - Start")
		ctx := context.WithValue(r.Context(), "Erufuncbaseurl", module_store.Erufuncbaseurl)
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		params := r.URL.Query()
		isSilentStr := params.Get("silent")
		silentFlag := false
		silentFlag, _ = strconv.ParseBool(isSilentStr)

		authObjI, err := sh.Store.GetAuth(ctx, projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(ctx).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}

		recoveryReq := json.NewDecoder(r.Body)
		recoveryReq.DisallowUnknownFields()

		var recoveryPostBody auth.RecoveryPostBody

		if err = recoveryReq.Decode(&recoveryPostBody); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		res, err := authObjI.GenerateRecoveryCode(ctx, recoveryPostBody, projectId, silentFlag)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, http.StatusOK)
			if authName == "ory" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": res})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "code sent successfully"})
			return
		}
	}
}

func GetVerifyCodeHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("GetVerifyCodeHandler - Start")
		ctx := context.WithValue(r.Context(), "Erufuncbaseurl", module_store.Erufuncbaseurl)
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		params := r.URL.Query()
		isSilentStr := params.Get("silent")
		silentFlag := false
		var silentFlagErr error
		silentFlag, silentFlagErr = strconv.ParseBool(isSilentStr)
		logs.WithContext(ctx).Info(fmt.Sprint("silentFlag = ", silentFlag))
		if silentFlagErr != nil {
			logs.WithContext(ctx).Info(silentFlagErr.Error())
		}

		verifyPostBodyFromReq := json.NewDecoder(r.Body)
		verifyPostBodyFromReq.DisallowUnknownFields()

		var verifyPostBody auth.VerifyPostBody

		if err := verifyPostBodyFromReq.Decode(&verifyPostBody); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		authObjI, err := sh.Store.GetAuth(ctx, projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(ctx).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}

		_, err = authObjI.GenerateVerifyCode(ctx, verifyPostBody, projectId, silentFlag)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "code sent successfully"})
			return
		}
	}
}

func CheckVerifyCodeHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("CheckVerifyCodeHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}

		verifyReq := json.NewDecoder(r.Body)
		verifyReq.DisallowUnknownFields()

		var verifyCode auth.VerifyCode

		if err = verifyReq.Decode(&verifyCode); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		fmt.Printf("verifyCode = %v\n", verifyCode)
		tokenKey, tokenKeyErr := authObjI.GetAttribute(r.Context(), "token_header_key")
		fmt.Printf("tokenKey = %v\n", tokenKey)
		if tokenKeyErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": tokenKeyErr.Error()})
			return
		}
		tokenStr := r.Header.Get(tokenKey.(string))
		tokenObj, tokenObjErr := getToken(r.Context(), tokenStr)
		if tokenObjErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": tokenObjErr.Error()})
			return
		}
		userId, userIdErr := getUserIdFromToken(tokenObj)
		if userIdErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": userIdErr.Error()})
			return
		}
		verifyCode.UserId = userId

		res, err := authObjI.VerifyCode(r.Context(), verifyCode, tokenObj, true)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "not verified"})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		if res != nil {
			_ = json.NewEncoder(w).Encode(res)
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "verified"})
		}
		return
	}
}

func VerifyCodeHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("VerifyCodeHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}

		verifyReq := json.NewDecoder(r.Body)
		verifyReq.DisallowUnknownFields()

		var verifyCode auth.VerifyCode

		if err = verifyReq.Decode(&verifyCode); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		res, err := authObjI.VerifyCodeNoUser(r.Context(), verifyCode)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "not verified"})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		if res != nil {
			_ = json.NewEncoder(w).Encode(res)
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "verified"})
		}
		return
	}
}

func VerifyRecoveryCodeHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("VerifyRecoveryCodeHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return
		}
		recoveryReq := json.NewDecoder(r.Body)
		recoveryReq.DisallowUnknownFields()

		var recoveryPassword auth.RecoveryPassword

		if err = recoveryReq.Decode(&recoveryPassword); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		var cookies []*http.Cookie
		res := make(map[string]string)
		res, cookies, err = authObjI.VerifyRecovery(r.Context(), recoveryPassword)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			for _, c := range cookies {
				http.SetCookie(w, c)
				logs.WithContext(r.Context()).Debug(fmt.Sprint(c))
			}
			server_handlers.FormatResponse(w, http.StatusOK)
			_ = json.NewEncoder(w).Encode(res)
			return
		}
	}
}
func CompleteRecoveryHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("CompleteRecoveryHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		recoveryReq := json.NewDecoder(r.Body)
		recoveryReq.DisallowUnknownFields()

		var recoveryPassword auth.RecoveryPassword

		if err = recoveryReq.Decode(&recoveryPassword); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		msg := ""
		for _, c := range r.Cookies() {
			logs.WithContext(r.Context()).Info(c.String())
		}
		msg, err = authObjI.CompleteRecovery(r.Context(), recoveryPassword, r.Cookies())
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": msg})
			return
		}
	}
}

func LogoutHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("LogoutHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		res, resStatusCode, err := authObjI.Logout(r.Context(), r)
		if err != nil {
			server_handlers.FormatResponse(w, resStatusCode)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			clearRefreshCookie(w, r, authObjI)
			server_handlers.FormatResponse(w, resStatusCode)
			_ = json.NewEncoder(w).Encode(res)
			return
		}
	}
}

func GetUserHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("GetUserHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		getUserReq := json.NewDecoder(r.Body)
		getUserReq.DisallowUnknownFields()
		getUserObj := make(map[string]interface{})
		//storageObj := new(storage.Storage)
		if err := getUserReq.Decode(&getUserObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		userIdStr := ""
		if userId, ok := getUserObj["id"]; !ok {
			rtErr := errors.New("id attribute missing in request body")
			logs.WithContext(r.Context()).Error(rtErr.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": rtErr})
			return
		} else {
			if userIdStr, ok = userId.(string); !ok {
				rtErr := errors.New("Incorrect refresh_token recevied in request body")
				logs.WithContext(r.Context()).Error(rtErr.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": rtErr})
				return
			}
		}

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		identity, err := authObjI.GetUser(r.Context(), userIdStr)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(identity)
		return
	}
}

func UpdateUserHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("UpdateUserHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		updateUserReq := json.NewDecoder(r.Body)
		updateUserReq.DisallowUnknownFields()
		updateUserObj := make(map[string]interface{})
		//storageObj := new(storage.Storage)
		if err := updateUserReq.Decode(&updateUserObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		identity := auth.Identity{}
		userAttributes := make(map[string]interface{})
		if userAttributesObj, ok := updateUserObj["attributes"]; !ok {
			rtErr := errors.New("attributes missing in request body")
			logs.WithContext(r.Context()).Error(rtErr.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": rtErr})
			return
		} else {
			if userAttributes, ok = userAttributesObj.(map[string]interface{}); !ok {
				rtErr := errors.New("incorrect post body")
				logs.WithContext(r.Context()).Error(rtErr.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": rtErr})
				return
			}
		}
		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}

		identity.Attributes = userAttributes

		tokenKey, tokenKeyErr := authObjI.GetAttribute(r.Context(), "token_header_key")
		if tokenKeyErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": tokenKeyErr.Error()})
			return
		}
		tokenStr := r.Header.Get(tokenKey.(string))
		tokenObj, tokenObjErr := getToken(r.Context(), tokenStr)
		if tokenObjErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": tokenObjErr.Error()})
			return
		}
		userId, userIdErr := getUserIdFromToken(tokenObj)
		if userIdErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": userIdErr.Error()})
			return
		}
		identity.Id = userId
		var tokens interface{}
		tokens, err = authObjI.UpdateUser(r.Context(), identity, userId, tokenObj)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(tokens)
		return
	}
}

func EditUserHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("UpdateUserHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		updateUserReq := json.NewDecoder(r.Body)
		updateUserReq.DisallowUnknownFields()
		updateUserObj := make(map[string]interface{})
		//storageObj := new(storage.Storage)
		if err := updateUserReq.Decode(&updateUserObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		identity := auth.Identity{}
		userAttributes := make(map[string]interface{})
		userId := ""
		if userAttributesObj, ok := updateUserObj["attributes"]; !ok {
			rtErr := errors.New("attributes missing in request body")
			logs.WithContext(r.Context()).Error(rtErr.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": rtErr})
			return
		} else {
			if userAttributes, ok = userAttributesObj.(map[string]interface{}); !ok {
				rtErr := errors.New("incorrect post body")
				logs.WithContext(r.Context()).Error(rtErr.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": rtErr})
				return
			}
		}
		if userIdObj, ok := updateUserObj["id"]; !ok {
			rtErr := errors.New("id missing in request body")
			logs.WithContext(r.Context()).Error(rtErr.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": rtErr})
			return
		} else {
			if userId, ok = userIdObj.(string); !ok {
				rtErr := errors.New("incorrect post body")
				logs.WithContext(r.Context()).Error(rtErr.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": rtErr})
				return
			}
		}

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}

		identity.Attributes = userAttributes
		identity.Id = userId

		var tokens interface{}
		tokens, err = authObjI.UpdateUser(r.Context(), identity, userId, nil)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(tokens)
		return
	}
}

func ChangePasswordHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ChangePasswordHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		changePasswordReq := json.NewDecoder(r.Body)
		changePasswordReq.DisallowUnknownFields()
		changePasswordObj := auth.ChangePassword{}
		//storageObj := new(storage.Storage)
		if err := changePasswordReq.Decode(&changePasswordObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}
		tokenKey, tokenKeyErr := authObjI.GetAttribute(r.Context(), "token_header_key")
		if tokenKeyErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": tokenKeyErr.Error()})
			return
		}
		tokenStr := r.Header.Get(tokenKey.(string))
		tokenObj, tokenObjErr := getToken(r.Context(), tokenStr)
		if tokenObjErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": tokenObjErr.Error()})
			return
		}
		userId, userIdErr := getUserIdFromToken(tokenObj)
		if userIdErr != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": userIdErr.Error()})
			return
		}

		err = authObjI.ChangePassword(r.Context(), tokenObj, userId, changePasswordObj)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": "password updated successfully"})
		return
	}
}

func GetSsoUrlHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("GetSsoUrl - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		params := r.URL.Query()
		state := params.Get("state")
		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		responseBody := make(map[string]string)
		url, msParams, err := authObjI.GetUrl(r.Context(), state)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		responseBody["url"] = url

		pkceRequired, err := authObjI.GetAttribute(r.Context(), "pkce")
		if pkceRequired.(bool) {
			err = sh.Store.SavePkceEvent(r.Context(), msParams, sh.Store)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			responseBody["request_id"] = msParams.ClientRequestId
		}

		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(responseBody)
		return
	}
}

func RegisterHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("RegisterHandler - Start")
		ctx := context.WithValue(r.Context(), "Erufuncbaseurl", module_store.Erufuncbaseurl)
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(ctx, projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(ctx).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}

		registerPostBodyFromReq := json.NewDecoder(r.Body)
		registerPostBodyFromReq.DisallowUnknownFields()

		var registerPostBody auth.RegisterUser

		if err = registerPostBodyFromReq.Decode(&registerPostBody); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		res, tokens, err := authObjI.Register(ctx, registerPostBody, projectId)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			setRefreshCookie(w, r, authObjI, tokens.RefreshToken)
			server_handlers.FormatResponse(w, http.StatusOK)
			if tokens.IdToken != "" {
				_ = json.NewEncoder(w).Encode(hideRefreshToken(r, authObjI, tokens))
			} else {
				_ = json.NewEncoder(w).Encode(res)
			}
			return
		}
	}
}

func getToken(ctx context.Context, tokenStr string) (tokenObj map[string]interface{}, err error) {
	if tokenStr != "" {
		err = json.Unmarshal([]byte(tokenStr), &tokenObj)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("error while unmarshalling token claim : ", err.Error()))
			return
		}
	} else {
		err = errors.New("token not found")
	}
	return
}

func getUserIdFromToken(tokenObj map[string]interface{}) (userId string, err error) {
	if iObj, iObjOk := tokenObj["identity"]; iObjOk {
		if iObjMap, iObjMapOk := iObj.(map[string]interface{}); iObjMapOk {
			if uid, userIdOk := iObjMap["id"]; userIdOk {
				userId = uid.(string)
			}
		}
	}
	if userId == "" {
		err = errors.New("userid not found")
		return
	}
	return
}

func RemoveIdentityHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("RemoveIdentityHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObjI, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if authObjI.GetAuthDb() != nil {
			authObjI.GetAuthDb().SetConn(sh.Store.GetConn())
		} else {
			logs.WithContext(r.Context()).Error("authObjI.GetAuthDb() is nil")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Something went wrong, Please try again."})
			return

		}

		removeUserPostBodyFromReq := json.NewDecoder(r.Body)
		removeUserPostBodyFromReq.DisallowUnknownFields()

		var removeUserPostBody auth.RemoveUser

		if err = removeUserPostBodyFromReq.Decode(&removeUserPostBody); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		err = authObjI.RemoveUser(r.Context(), removeUserPostBody)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "identity deleted successfully"})
		return
	}
}
