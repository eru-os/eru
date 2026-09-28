package module_server

import (
	"fmt"
	"net/http"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_model"
	module_handlers "github.com/eru-os/eru/eru-auth/module_server/handlers"
	"github.com/eru-os/eru/eru-auth/module_store"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func SetServiceName() {
	server_handlers.ServerName = "eru-auth"
}
func AddModuleRoutes(serverRouter *mux.Router, sh *module_store.StoreHolder) {

	//store functions specific to files
	//serverRouter.Path("/auth/google/login").HandlerFunc(module_handlers.OauthGoogleLogin())
	//serverRouter.Path("/auth/google/callback").HandlerFunc(module_handlers.OauthGoogleCallback())
	//serverRouter.Path("/auth/openid/login").HandlerFunc(module_handlers.OpenIdLoginHandler(sh.Store))
	//serverRouter.Path("/auth/openid/callback").HandlerFunc(module_handlers.OpenIdCallbackHandler(sh.Store))
	//serverRouter.Path("/auth/openid/getloginflow/{loginchallenge}").HandlerFunc(module_handlers.GetLoginFlowHandlerandler(sh.Store))
	// OAuth 2.0 protected resource metadata (RFC 9728). An MCP client reads this from the root
	// of the host serving the MCP endpoint, so it is registered ahead of the /{project} subrouter
	// and takes the project from the header the gateway listener rule adds.
	oauthResourceHandler := module_handlers.OAuthProtectedResourceHandler(sh)
	serverRouter.Methods(http.MethodGet).Path(module_model.McpWellKnownPath).HandlerFunc(oauthResourceHandler)
	serverRouter.Methods(http.MethodGet).PathPrefix(fmt.Sprint(module_model.McpWellKnownPath, "/")).HandlerFunc(oauthResourceHandler)

	// Authorization server discovery (RFC 8414) and dynamic client registration (RFC 7591). Both are
	// read from the root of the issuer, so they sit beside the resource metadata rather than under
	// /{project}, and resolve the auth from the listener rule headers.
	asMetadataHandler := module_handlers.AuthorizationServerMetadataHandler(sh)
	serverRouter.Methods(http.MethodGet).Path(auth.AuthorizationServerWellKnownPath).HandlerFunc(asMetadataHandler)
	serverRouter.Methods(http.MethodGet).Path(auth.OpenIdWellKnownPath).HandlerFunc(asMetadataHandler)
	serverRouter.Methods(http.MethodPost).Path(auth.OAuthRegisterPath).HandlerFunc(module_handlers.RegisterOAuthClientHandler(sh))

	// Interactive leg of the authorization code grant. The authorization server redirects the
	// browser here, so these must stay public in the listener rule's authorizer exception.
	serverRouter.Methods(http.MethodGet).Path(auth.OAuthAuthorizePath).HandlerFunc(module_handlers.AuthorizeHandler(sh))
	serverRouter.Methods(http.MethodGet).Path(auth.OAuthLoginPath).HandlerFunc(module_handlers.LoginPageHandler(sh))
	serverRouter.Methods(http.MethodPost).Path(auth.OAuthLoginPath).HandlerFunc(module_handlers.LoginSubmitHandler(sh))
	serverRouter.Methods(http.MethodGet).Path(auth.OAuthConsentPath).HandlerFunc(module_handlers.ConsentPageHandler(sh))
	serverRouter.Methods(http.MethodPost).Path(auth.OAuthConsentPath).HandlerFunc(module_handlers.ConsentSubmitHandler(sh))
	serverRouter.Methods(http.MethodPost).Path(auth.OAuthTokenPath).HandlerFunc(module_handlers.TokenHandler(sh))
	serverRouter.Methods(http.MethodPost).Path(auth.OAuthRevokePath).HandlerFunc(module_handlers.RevokeTokenHandler(sh))
	serverRouter.Methods(http.MethodGet).Path(auth.OAuthLogoutPath).HandlerFunc(module_handlers.OAuthLogoutHandler(sh))
	serverRouter.Methods(http.MethodGet).Path(auth.OAuthJwksPath).HandlerFunc(module_handlers.OAuthJWKSetHandler(sh))
	serverRouter.Methods(http.MethodGet).Path(auth.OAuthUserInfoPath).HandlerFunc(module_handlers.OAuthUserInfoHandler(sh))
	serverRouter.Methods(http.MethodPost).Path(auth.OAuthUserInfoPath).HandlerFunc(module_handlers.OAuthUserInfoHandler(sh))

	storeRouter := serverRouter.PathPrefix("/store").Subrouter()
	storeRouter.Methods(http.MethodGet).Path("/load").HandlerFunc(module_handlers.StoreLoadHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/compare").HandlerFunc(module_handlers.StoreCompareHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/save").HandlerFunc(module_handlers.ProjectSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/remove").HandlerFunc(module_handlers.ProjectRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/project/list").HandlerFunc(module_handlers.ProjectListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/config").HandlerFunc(module_handlers.ProjectConfigHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{authname}/save/oauthclient").HandlerFunc(module_handlers.OAuthClientSaveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{authname}/oauthclient/{clientid}").HandlerFunc(module_handlers.OAuthClientGetHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{authname}/remove/oauthclient/{clientid}").HandlerFunc(module_handlers.OAuthClientRemoveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/save/auth").HandlerFunc(module_handlers.AuthSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/remove/auth/{authname}").HandlerFunc(module_handlers.AuthRemoveHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/save/kid").HandlerFunc(module_handlers.KidSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/remove/kid/{kid}").HandlerFunc(module_handlers.KidRemoveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/kid/{kid}/status/{status}").HandlerFunc(module_handlers.KidStatusHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/create/api_token").HandlerFunc(module_handlers.ApiTokenSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/revoke/api_token/{token_id}").HandlerFunc(module_handlers.ApiTokenRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/list/api_token/{identity_id}").HandlerFunc(module_handlers.ApiTokenListHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/settings/save").HandlerFunc(module_handlers.ProjectSetingsSaveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/func/list").HandlerFunc(module_handlers.ProjectFunctionListHandler(sh))

	// functions for file events
	authRouter := serverRouter.PathPrefix("/{project}").Subrouter()
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/getrecoverycode").HandlerFunc(module_handlers.GetRecoveryCodeHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/getverifycode").HandlerFunc(module_handlers.GetVerifyCodeHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/verifyrecoverycode").HandlerFunc(module_handlers.VerifyRecoveryCodeHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/checkverifycode").HandlerFunc(module_handlers.CheckVerifyCodeHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/verifycode").HandlerFunc(module_handlers.VerifyCodeHandler(sh))

	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/completerecovery").HandlerFunc(module_handlers.CompleteRecoveryHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/login/api").HandlerFunc(module_handlers.LoginApiHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/login").HandlerFunc(module_handlers.LoginHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/idptoken/{renew}").HandlerFunc(module_handlers.IdpTokenHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/idptoken").HandlerFunc(module_handlers.IdpTokenHandler(sh))
	authRouter.Methods(http.MethodGet).PathPrefix("/{authname}/gettoken").HandlerFunc(module_handlers.GetTokenHandler(sh))
	authRouter.Methods(http.MethodDelete).PathPrefix("/{authname}/logout").HandlerFunc(module_handlers.OAuthLogoutHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/verify/{tokentype}").HandlerFunc(module_handlers.VerifyTokenHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/userinfo").HandlerFunc(module_handlers.UserInfoHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/fetchtokens").HandlerFunc(module_handlers.FetchTokensHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/gettokens").HandlerFunc(module_handlers.GetTokensHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/getusertoken").HandlerFunc(module_handlers.GetUserTokensHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/getidtoken").HandlerFunc(module_handlers.GetIdTokenHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/generatetempcode").HandlerFunc(module_handlers.GenerateTempCodeHandler(sh))
	authRouter.Methods(http.MethodGet).PathPrefix("/{authname}/getuser").HandlerFunc(module_handlers.GetUserHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/updateuser").HandlerFunc(module_handlers.UpdateUserHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/edituser").HandlerFunc(module_handlers.EditUserHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/changepassword").HandlerFunc(module_handlers.ChangePasswordHandler(sh))
	authRouter.Methods(http.MethodGet).PathPrefix("/{authname}/getssourl").HandlerFunc(module_handlers.GetSsoUrlHandler(sh))
	authRouter.Methods(http.MethodPost).PathPrefix("/{authname}/register").HandlerFunc(module_handlers.RegisterHandler(sh))
	authRouter.Methods(http.MethodDelete).PathPrefix("/{authname}/removeidentity").HandlerFunc(module_handlers.RemoveIdentityHandler(sh))
	authRouter.Methods(http.MethodGet).Path("/.well-known/jwks.json").HandlerFunc(module_handlers.JWKSetHandler(sh))
	authRouter.Methods(http.MethodGet).Path("/.well-known/jwks.json/{kid}").HandlerFunc(module_handlers.JWKHandler(sh))
}
