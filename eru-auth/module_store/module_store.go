package module_store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_model"
	erujwt "github.com/eru-os/eru/eru-crypto/jwt"
	erursa "github.com/eru-os/eru/eru-crypto/rsa"
	erusha "github.com/eru-os/eru/eru-crypto/sha"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-store/store"
	"github.com/google/uuid"
)

const (
	INSERT_PKCE_EVENT      = "insert into eruauth_pkce_events (pkce_event_id,code_verifier,code_challenge,request_id,nonce,url) values ($1,$2,$3,$4,$5,$6)"
	SELECT_PKCE_EVENT      = "select * from eruauth_pkce_events where request_id = $1"
	INSERT_API_TOKEN       = "insert into eruauth_api_tokens (api_token_id,identity_id,project_id,api_token_hash,api_token_name,api_token) values ($1,$2,$3,$4,$5,$6)"
	SELECT_IDENTITY_EXISTS = "select identity_id from eruauth_identities where identity_id = $1"
	UPDATE_API_TOKEN       = "update eruauth_api_tokens set api_token_status='INACTIVE' , updated_date=CURRENT_TIMESTAMP where api_token_id=$1"
	SELECT_API_TOKEN       = "select api_token_id, project_id, identity_id, api_token_name, api_token, api_token_hash, api_token_status from eruauth_api_tokens where identity_id=$1"
	//SELECT_IDENTITY_SUB = "select * from eruauth_identities where identity_provider_id = $1"
)

var Erufuncbaseurl = "http://localhost:8083"

type StoreHolder struct {
	sync.RWMutex
	Store ModuleStoreI
}
type ModuleStoreI interface {
	store.StoreI
	SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error
	RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error
	GetProjectConfig(ctx context.Context, projectId string) (*module_model.Project, error)
	GetExtendedProjectConfig(ctx context.Context, projectId string, realStore ModuleStoreI) (module_model.ExtendedProject, error)
	GetProjectList(ctx context.Context) []map[string]interface{}
	SaveAuth(ctx context.Context, authObj auth.AuthI, projectId string, realStore ModuleStoreI, persist bool) error
	RemoveAuth(ctx context.Context, authType string, projectId string, realStore ModuleStoreI) error
	GetAuth(ctx context.Context, projectId string, authName string, s ModuleStoreI) (auth.AuthI, error)
	SavePkceEvent(ctx context.Context, msParams auth.OAuthParams, s ModuleStoreI) (err error)
	GetPkceEvent(ctx context.Context, requestId string, s ModuleStoreI) (msParams auth.OAuthParams, err error)
	SaveProjectSettings(ctx context.Context, projectId string, projectSettings module_model.ProjectSettings, realStore ModuleStoreI) error

	SaveKid(ctx context.Context, kid string, projectId string, realStore ModuleStoreI, persist bool) (erursa.RsaKeyPair, error)
	RemoveKid(ctx context.Context, kid string, projectId string, realStore ModuleStoreI) error
	GetKid(ctx context.Context, projectId string, kid string, s ModuleStoreI) (erursa.RsaKeyPair, error)
	SaveApiToken(ctx context.Context, identity_id string, kid string, projectId string, token_header map[string]interface{}, token_claims map[string]interface{}, tokenName string, realStore ModuleStoreI) (string, error)
	RevokeApiToken(ctx context.Context, token_id string, realStore ModuleStoreI) (err error)
	GetApiTokens(ctx context.Context, identity_id string, realStore ModuleStoreI) (tokens []module_model.ApiToken, err error)
	FetchJWKKeys(ctx context.Context, projectId string, kid string, realStore ModuleStoreI) (jwk []erursa.JWK, err error)
	FetchJWKKeySet(ctx context.Context, projectId string, realStore ModuleStoreI) (jwk []erursa.JWK, err error)
	SetKidStatus(ctx context.Context, projectId string, kid string, status string, realStore ModuleStoreI) error
	GetSigningKid(ctx context.Context, projectId string, kid string, realStore ModuleStoreI) (erursa.RsaKeyPair, error)
}

type ModuleStore struct {
	Projects map[string]*module_model.Project `json:"projects"` //ProjectId is the key
}

type ModuleFileStore struct {
	store.FileStore
	ModuleStore
}
type ModuleDbStore struct {
	store.DbStore
	ModuleStore
}

func (ms *ModuleStore) SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error {
	//TODO to handle edit project once new project attributes are finalized
	logs.WithContext(ctx).Debug("SaveProject - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	if _, ok := ms.Projects[projectId]; !ok {
		project := new(module_model.Project)
		project.ProjectId = projectId
		if ms.Projects == nil {
			ms.Projects = make(map[string]*module_model.Project)
		}
		if project.Auth == nil {
			project.Auth = make(map[string]auth.AuthI)
		}

		ms.Projects[projectId] = project
		if persist == true {
			logs.WithContext(ctx).Info("SaveStore called from SaveProject")
			return realStore.SaveStore(ctx, projectId, "", realStore)
		} else {
			return nil
		}
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " already exists"))
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}

func (ms *ModuleStore) RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveProject - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if _, ok := ms.Projects[projectId]; ok {
		delete(ms.Projects, projectId)
		logs.WithContext(ctx).Info("SaveStore called from RemoveProject")
		return realStore.SaveStore(ctx, projectId, "", realStore)
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetExtendedProjectConfig(ctx context.Context, projectId string, realStore ModuleStoreI) (ePrj module_model.ExtendedProject, err error) {
	logs.WithContext(ctx).Debug("GetExtendedProjectConfig - Start")
	ePrj = module_model.ExtendedProject{}
	if prj, ok := ms.Projects[projectId]; ok {
		ePrj.Variables, err = realStore.FetchVars(ctx, projectId)
		ePrj.SecretManager, err = realStore.FetchSm(ctx, projectId)
		ePrj.ProjectId = prj.ProjectId
		ePrj.ProjectSettings = prj.ProjectSettings
		ePrj.Auth = prj.Auth
		ePrj.Kids = prj.Kids
		return ePrj, nil
	} else {
		err = errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		return module_model.ExtendedProject{}, err
	}
}

func (ms *ModuleStore) GetProjectConfig(ctx context.Context, projectId string) (*module_model.Project, error) {
	logs.WithContext(ctx).Debug("GetProjectConfig - Start")
	if _, ok := ms.Projects[projectId]; ok {
		return ms.Projects[projectId], nil
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	}
}

func (ms *ModuleStore) GetProjectList(ctx context.Context) []map[string]interface{} {
	logs.WithContext(ctx).Debug("GetProjectList - Start")
	projects := make([]map[string]interface{}, len(ms.Projects))
	i := 0
	for k := range ms.Projects {
		project := make(map[string]interface{})
		project["project_name"] = k
		projects[i] = project
		i++
	}
	return projects
}

func (ms *ModuleStore) SaveAuth(ctx context.Context, authObj auth.AuthI, projectId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveAuth - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}

	//cloning authObj to replace variables and execute PerformPreSaveTask with actual values
	authObjClone, err := ms.GetAuthCloneObject(ctx, projectId, authObj, realStore)
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return err
	}
	authName, err := authObjClone.GetAttribute(ctx, "auth_name")
	if err != nil {
		return err
	}
	if persist == true {
		err = authObjClone.PerformPreSaveTask(ctx)
		if err != nil {
			return err
		}
	}
	//save original authObj with variables
	err = prj.AddAuth(ctx, authName.(string), authObj)
	if err != nil {
		return err
	}

	if persist == true {
		return realStore.SaveStore(ctx, projectId, "", realStore)
	}
	return nil
}
func (ms *ModuleStore) RemoveAuth(ctx context.Context, authName string, projectId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveAuth - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if prg, ok := ms.Projects[projectId]; ok {
		if authObj, ok := prg.Auth[authName]; ok {
			err = authObj.PerformPreDeleteTask(ctx)
			if err != nil {
				return
			}
		} else {
			err = errors.New(fmt.Sprint("Auth ", authName, " does not exists"))
			logs.WithContext(ctx).Info(err.Error())
			return err
		}
		err = prg.RemoveAuth(ctx, authName)
		if err != nil {
			return err
		}
	} else {
		err = errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
	return realStore.SaveStore(ctx, projectId, "", realStore)
}

// storeTokenSigner signs and verifies with the project's rsa keys - the same keys eru-auth publishes
// at its jwks endpoints, so a resource server validates these tokens against a key set it can
// already fetch.
type storeTokenSigner struct {
	store ModuleStoreI
}

func (signer storeTokenSigner) SignToken(ctx context.Context, projectId string, kid string, claims map[string]interface{}) (string, error) {
	if kid == "" {
		return "", errors.New("oauth_server.signing_kid is not set")
	}
	keyPair, err := signer.store.GetSigningKid(ctx, projectId, fmt.Sprint("ERUAUTH_KID_", kid), signer.store)
	if err != nil {
		return "", err
	}
	header := map[string]interface{}{"alg": "RS256", "typ": "JWT", "kid": kid}
	return erujwt.CreateJWT(ctx, keyPair.PrivateKey, claims, header)
}

// VerifyToken reads the key named in the token header rather than the configured signing key, so a
// token signed by a since retired key still verifies - which is what makes rotation possible.
func (signer storeTokenSigner) VerifyToken(ctx context.Context, projectId string, token string) (map[string]interface{}, error) {
	kid := erujwt.TokenKid(ctx, token)
	if kid == "" {
		return nil, errors.New("token has no kid")
	}
	keyPair, err := signer.store.GetKid(ctx, fmt.Sprint("ERUAUTH_KID_", kid), projectId, signer.store)
	if err != nil {
		return nil, err
	}
	return erujwt.VerifyTokenWithPublicKey(ctx, token, keyPair.PublicKey)
}

func (ms *ModuleStore) GetAuthClone(ctx context.Context, projectId string, authName string, s ModuleStoreI) (authObjClone auth.AuthI, err error) {
	logs.WithContext(ctx).Debug("GetAuthClone - Start")
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	}

	if authObj, ok := prj.Auth[authName]; !ok {
		err = errors.New(fmt.Sprint("auth ", authName, " not found"))
		logs.WithContext(ctx).Error(err.Error())
		return
	} else {
		authObjClone, err = ms.GetAuthCloneObject(ctx, projectId, authObj, s)
		authObjClone.SetAuthDb(GetAuthDb(s.GetDbType()))
		// The built in token backend mints with the project's keys, so it needs to know which
		// project this auth was loaded for and how to sign for it.
		authObjClone.SetTokenContext(projectId, storeTokenSigner{store: s}, authObjClone)
		var kmsIdI interface{}
		kmsIdI, err = authObjClone.GetAttribute(ctx, "key_id")
		if err == nil && kmsIdI != nil {
			kmsMap, kmsErr := s.FetchKms(ctx, projectId)
			if kmsErr != nil {
				logs.WithContext(ctx).Error(kmsErr.Error())
				//return
			} else {
				authObjClone.SetKms(ctx, kmsMap[kmsIdI.(string)])
			}
		}
		return
	}
}

func (ms *ModuleStore) GetAuthCloneObject(ctx context.Context, projectId string, authObj auth.AuthI, s ModuleStoreI) (authObjClone auth.AuthI, err error) {
	logs.WithContext(ctx).Debug("GetAuGetAuthCloneObjectth - Start")

	authObjJson, authObjJsonErr := json.Marshal(authObj)
	if authObjJsonErr != nil {
		err = errors.New(fmt.Sprint("error while cloning authObj (marshal)"))
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(authObjJsonErr.Error())
		return
	}
	authObjJson = s.ReplaceVariables(ctx, projectId, authObjJson, nil)

	iCloneI := reflect.New(reflect.TypeOf(authObj))
	authObjCloneErr := json.Unmarshal(authObjJson, iCloneI.Interface())
	if authObjCloneErr != nil {
		err = errors.New(fmt.Sprint("error while cloning authObj(unmarshal)"))
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(authObjCloneErr.Error())
		return
	}
	return iCloneI.Elem().Interface().(auth.AuthI), nil
}

func (ms *ModuleStore) GetAuth(ctx context.Context, projectId string, authName string, s ModuleStoreI) (auth.AuthI, error) {
	logs.WithContext(ctx).Debug("GetAuth - Start")
	return ms.GetAuthClone(ctx, projectId, authName, s)

	/*
		if prg, ok := ms.Projects[projectId]; ok {
			if prg.Auth != nil {
				for k, v := range prg.Auth {
					if k == authName {

						return v, nil
					}
				}
			} else {
				err := errors.New(fmt.Sprint("No Auth Defined for the project : ", projectId))
				logs.WithContext(ctx).Info(err.Error())
				return nil, err
			}
		} else {
			err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
			logs.WithContext(ctx).Info(err.Error())
			return nil, err
		}
		err := errors.New(fmt.Sprint("Auth ", authName, " not found"))
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	*/
}

func (ms *ModuleStore) SavePkceEvent(ctx context.Context, msParams auth.OAuthParams, s ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("SavePkceEvent - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()

	var queries []store.Queries
	query := store.Queries{}
	query.Query = INSERT_PKCE_EVENT
	var vals []interface{}
	vals = append(vals, uuid.New().String(), msParams.CodeVerifier, msParams.CodeChallenge, msParams.ClientRequestId, msParams.Nonce, msParams.Url)
	query.Vals = vals
	queries = append(queries, query)
	_, err = s.ExecuteDbSave(ctx, queries)
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
	}
	return
}

func (ms *ModuleStore) GetPkceEvent(ctx context.Context, requestId string, s ModuleStoreI) (msParams auth.OAuthParams, err error) {
	logs.WithContext(ctx).Debug("GetPkceEvent - Start")
	query := store.Queries{}
	query.Query = SELECT_PKCE_EVENT
	var vals []interface{}
	vals = append(vals, requestId)
	query.Vals = vals
	output, err := s.ExecuteDbFetch(ctx, query)
	if len(output) > 0 {
		msParams.CodeVerifier = output[0]["code_verifier"].(string)
		msParams.CodeChallenge = output[0]["code_challenge"].(string)
		msParams.ClientRequestId = output[0]["request_id"].(string)
		msParams.Nonce = output[0]["nonce"].(string)
		msParams.Url = output[0]["url"].(string)
	}
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
	}
	return
}

func GetAuthDb(dbType string) auth.AuthDbI {
	switch strings.ToUpper(dbType) {
	case "POSTGRES":
		return new(auth.AuthDbPostgres)
	case "MYSQL":
		return new(auth.AuthDbMysql)
	default:
		return new(auth.AuthDb)
	}
}

func (ms *ModuleStore) SaveProjectSettings(ctx context.Context, projectId string, projectSettings module_model.ProjectSettings, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("SaveProjectConfig - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	ms.Projects[projectId].ProjectSettings = projectSettings
	logs.WithContext(ctx).Info("SaveStore called from SaveProjectSettings")
	return realStore.SaveStore(ctx, projectId, "", realStore)
}

func (ms *ModuleStore) SaveKid(ctx context.Context, kid string, projectId string, realStore ModuleStoreI, persist bool) (erursa.RsaKeyPair, error) {
	logs.WithContext(ctx).Debug("SaveKid - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}

	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return erursa.RsaKeyPair{}, err
	}
	if _, kidOk := prj.Kids[kid]; kidOk {
		err = errors.New(fmt.Sprint("kid ", kid, " already exists"))
		logs.WithContext(ctx).Info(err.Error())
		return erursa.RsaKeyPair{}, err
	}
	rsaKeyPair, rsaErr := erursa.GenerateKeyPair(ctx, 2048)
	if rsaErr != nil {
		err = errors.New(fmt.Sprint("RSA key generation failed"))
		logs.WithContext(ctx).Info(err.Error())
		return erursa.RsaKeyPair{}, err
	}
	rsaKeyPairMap := make(map[string]string)
	rsaKeyPairMap["private_key"] = rsaKeyPair.PrivateKey
	rsaKeyPairMap["public_key"] = rsaKeyPair.PublicKey
	err = realStore.SetSmValue(ctx, projectId, kid, rsaKeyPairMap)
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		err = errors.New(fmt.Sprint("RSA key could not be saved"))
		return erursa.RsaKeyPair{}, err
	}
	err = prj.AddKid(ctx, kid)
	if err != nil {
		return erursa.RsaKeyPair{}, err
	}
	InvalidateJWKSet(projectId)
	if persist == true {
		err = realStore.SaveStore(ctx, projectId, "", realStore)
		if err != nil {
			return erursa.RsaKeyPair{}, err
		}
	}
	return rsaKeyPair, nil
}
func (ms *ModuleStore) RemoveKid(ctx context.Context, kid string, projectId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveKid - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if prg, ok := ms.Projects[projectId]; ok {
		if _, kOk := prg.Kids[kid]; kOk {
			err = prg.RemoveKid(ctx, kid)
			if err != nil {
				return err
			}
			InvalidateJWKSet(projectId)
			err = realStore.UnsetSmValue(ctx, projectId, kid, "public_key")
			if err != nil {
				err = errors.New(fmt.Sprint("Kid public key ", kid, " could not be removed"))
				logs.WithContext(ctx).Info(err.Error())
				return err
			}
			err = realStore.UnsetSmValue(ctx, projectId, kid, "private_key")
			if err != nil {
				err = errors.New(fmt.Sprint("Kid private key ", kid, " could not be removed"))
				logs.WithContext(ctx).Info(err.Error())
				return err
			}
		} else {
			err = errors.New(fmt.Sprint("Kid ", kid, " does not exists"))
			logs.WithContext(ctx).Info(err.Error())
			return err
		}
	} else {
		err = errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
	return realStore.SaveStore(ctx, projectId, "", realStore)
}

func (ms *ModuleStore) SaveApiToken(ctx context.Context, identity_id string, kid string, projectId string, token_header map[string]interface{}, token_claims map[string]interface{}, tokenName string, realStore ModuleStoreI) (string, error) {
	logs.WithContext(ctx).Debug("SaveApiToken - Start")

	// The token row has a foreign key to the identity. Checking first means a bad user_id is
	// reported as such, instead of generating and persisting a signing key and only then failing on
	// the insert - which left the key behind with no token to show for it.
	identityQuery := store.Queries{Query: SELECT_IDENTITY_EXISTS}
	identityQuery.Vals = append(identityQuery.Vals, identity_id)
	identityOutput, err := realStore.ExecuteDbFetch(ctx, identityQuery)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("Something went wrong, Please try again.")
	}
	if len(identityOutput) == 0 {
		err = fmt.Errorf("user_id %s does not exist", identity_id)
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}

	kidCreated := false
	rsaKeyPair, err := ms.GetKid(ctx, kid, projectId, realStore)
	if err != nil {
		logs.WithContext(ctx).Info(fmt.Sprint(kid, " not found, creating it : ", err.Error()))
		rsaKeyPair, err = ms.SaveKid(ctx, kid, projectId, realStore, true)
		if err != nil {
			// Previously this error was discarded by the next assignment, so a failed key creation
			// surfaced as a confusing signing failure instead.
			logs.WithContext(ctx).Error(err.Error())
			return "", errors.New("Something went wrong, Please try again.")
		}
		kidCreated = true
	}

	jwt, err := erujwt.CreateJWT(ctx, rsaKeyPair.PrivateKey, token_claims, token_header)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		ms.removeKidCreatedFor(ctx, kidCreated, kid, projectId, realStore)
		return "", errors.New("Something went wrong, Please try again.")
	}
	var queries []store.Queries
	query := store.Queries{}
	query.Query = INSERT_API_TOKEN
	var vals []interface{}
	jwtHash := hex.EncodeToString(erusha.NewSHA512([]byte(jwt)))
	jwtStr := fmt.Sprint(jwt[:20], "xxxxxxxxxx")
	vals = append(vals, uuid.New().String(), identity_id, projectId, jwtHash, tokenName, jwtStr)
	query.Vals = vals
	queries = append(queries, query)
	_, err = realStore.ExecuteDbSave(ctx, queries)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		// A key generated for a token that was never stored is an orphan, so it is taken back out.
		ms.removeKidCreatedFor(ctx, kidCreated, kid, projectId, realStore)
		return "", errors.New("Something went wrong, Please try again.")
	}
	return jwt, nil
}

// removeKidCreatedFor undoes a key this call created, when what it was created for did not happen.
// A key that already existed is left alone - other tokens may be signed with it.
func (ms *ModuleStore) removeKidCreatedFor(ctx context.Context, kidCreated bool, kid string, projectId string, realStore ModuleStoreI) {
	if !kidCreated {
		return
	}
	if err := realStore.RemoveKid(ctx, kid, projectId, realStore); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("could not remove the key created for a token that was not saved : ", err.Error()))
	}
}

func (ms *ModuleStore) RevokeApiToken(ctx context.Context, token_id string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RevokeApiToken - Start")
	var queries []store.Queries
	query := store.Queries{}
	query.Query = UPDATE_API_TOKEN
	var vals []interface{}
	vals = append(vals, token_id)
	query.Vals = vals
	queries = append(queries, query)
	_, err = realStore.ExecuteDbSave(ctx, queries)
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		err = errors.New(fmt.Sprint("Something went wrong, Please try again."))
		return err
	}
	return
}

func (ms *ModuleStore) GetApiTokens(ctx context.Context, identity_id string, realStore ModuleStoreI) (tokens []module_model.ApiToken, err error) {
	logs.WithContext(ctx).Debug("RevokeApiToken - Start")
	query := store.Queries{}
	query.Query = SELECT_API_TOKEN
	var vals []interface{}
	vals = append(vals, identity_id)
	query.Vals = vals
	output, err := realStore.ExecuteDbFetch(ctx, query)
	if len(output) > 0 {
		for i, _ := range output {
			token := module_model.ApiToken{}
			token.TokenId = output[i]["api_token_id"].(string)
			token.IdentityId = output[i]["identity_id"].(string)
			token.Token = output[i]["api_token"].(string)
			token.TokenName = output[i]["api_token_name"].(string)
			token.TokenStatus = output[i]["api_token_status"].(string)
			tokens = append(tokens, token)
		}
	}
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
	}
	return
}

func (ms *ModuleStore) GetKid(ctx context.Context, kid string, projectId string, realStore ModuleStoreI) (rsakeyPair erursa.RsaKeyPair, err error) {
	logs.WithContext(ctx).Debug("GetKid - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if prg, ok := ms.Projects[projectId]; ok {
		if _, kOk := prg.Kids[kid]; kOk {
			publickKeyStr := ""
			privateKeyStr := ""
			strOk := false
			publickKey, publickKeyErr := realStore.GetSmValue(ctx, projectId, kid, "public_key", false)
			if publickKeyErr != nil {
				logs.WithContext(ctx).Info(publickKeyErr.Error())
				err = errors.New(fmt.Sprint("Kid public key ", kid, " was not found"))
				return erursa.RsaKeyPair{}, err
			}
			privateKey, privateKeyErr := realStore.GetSmValue(ctx, projectId, kid, "private_key", false)
			if privateKeyErr != nil {
				logs.WithContext(ctx).Info(privateKeyErr.Error())
				err = errors.New(fmt.Sprint("Kid private key ", kid, " was not found"))
				return erursa.RsaKeyPair{}, err
			}
			if publickKeyStr, strOk = publickKey.(string); !strOk {
				err = errors.New(fmt.Sprint("Kid public key ", kid, " is not a string"))
				logs.WithContext(ctx).Info(err.Error())
				return erursa.RsaKeyPair{}, err
			}
			if privateKeyStr, strOk = privateKey.(string); !strOk {
				err = errors.New(fmt.Sprint("Kid private key ", kid, " is not a string"))
				logs.WithContext(ctx).Info(err.Error())
				return erursa.RsaKeyPair{}, err
			}
			return erursa.RsaKeyPair{PublicKey: publickKeyStr, PrivateKey: privateKeyStr}, nil
		} else {
			err = errors.New(fmt.Sprint("Kid ", kid, " does not exists"))
			logs.WithContext(ctx).Info(err.Error())
			return erursa.RsaKeyPair{}, err
		}

	} else {
		err = errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Info(err.Error())
		return erursa.RsaKeyPair{}, err
	}
}

// jwkSetCache keeps the assembled key set for a short while. Without it every fetch of jwks_uri
// makes one secret manager call per key, on an endpoint every client polls.
var (
	jwkSetCacheMutex sync.RWMutex
	jwkSetCache      = map[string]jwkSetCacheEntry{}
)

const jwkSetCacheTtl = 5 * time.Minute

type jwkSetCacheEntry struct {
	keys      []erursa.JWK
	expiresAt time.Time
}

func cachedJWKSet(projectId string) ([]erursa.JWK, bool) {
	jwkSetCacheMutex.RLock()
	defer jwkSetCacheMutex.RUnlock()
	entry, ok := jwkSetCache[projectId]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.keys, true
}

func cacheJWKSet(projectId string, keys []erursa.JWK) {
	jwkSetCacheMutex.Lock()
	defer jwkSetCacheMutex.Unlock()
	jwkSetCache[projectId] = jwkSetCacheEntry{keys: keys, expiresAt: time.Now().Add(jwkSetCacheTtl)}
}

// InvalidateJWKSet drops the cached set so a key added, retired or removed is published at once
// rather than after the ttl.
func InvalidateJWKSet(projectId string) {
	jwkSetCacheMutex.Lock()
	defer jwkSetCacheMutex.Unlock()
	delete(jwkSetCache, projectId)
}

// SetKidStatus retires or reactivates a signing key. A retired key still verifies - it is published
// in the key set until it is removed - but is refused for signing.
func (ms *ModuleStore) SetKidStatus(ctx context.Context, projectId string, kid string, status string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("SetKidStatus - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return err
	}
	if err = prj.SetKidStatus(ctx, kid, status); err != nil {
		return err
	}
	InvalidateJWKSet(projectId)
	return realStore.SaveStore(ctx, projectId, "", realStore)
}

// GetSigningKid returns a key pair only if that key is still allowed to sign, so a retired key
// cannot be used to mint new tokens by leaving stale config pointing at it.
func (ms *ModuleStore) GetSigningKid(ctx context.Context, projectId string, kid string, realStore ModuleStoreI) (erursa.RsaKeyPair, error) {
	logs.WithContext(ctx).Debug("GetSigningKid - Start")
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return erursa.RsaKeyPair{}, err
	}
	if kidInfo, ok := prj.Kids[kid]; ok && kidInfo.Retired() {
		err = errors.New(fmt.Sprint("kid ", kid, " is retired and may not sign"))
		logs.WithContext(ctx).Error(err.Error())
		return erursa.RsaKeyPair{}, err
	}
	return ms.GetKid(ctx, kid, projectId, realStore)
}

// FetchJWKKeySet returns every published key of a project.// FetchJWKKeySet returns every published key of a project. A standard client fetches jwks_uri once
// and picks the key named by the token header's kid, so the whole set has to be reachable from one
// url - and that is also what lets a signing key be rotated: tokens signed by the retired key keep
// verifying until that key is removed.
func (ms *ModuleStore) FetchJWKKeySet(ctx context.Context, projectId string, realStore ModuleStoreI) (jwks []erursa.JWK, err error) {
	logs.WithContext(ctx).Debug("FetchJWKKeySet - Start")
	if keys, ok := cachedJWKSet(projectId); ok {
		return keys, nil
	}
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return nil, err
	}
	for storedKid := range prj.Kids {
		// Kids are indexed under their stored name; the kid a token carries is the name without the
		// prefix the store adds.
		kid := strings.TrimPrefix(storedKid, "ERUAUTH_KID_")
		keys, keyErr := realStore.FetchJWKKeys(ctx, projectId, kid, realStore)
		if keyErr != nil {
			// One unreadable key must not hide the rest, or rotating a key could take every token
			// down with it.
			logs.WithContext(ctx).Error(fmt.Sprint("jwk could not be read for kid ", kid, " : ", keyErr.Error()))
			continue
		}
		jwks = append(jwks, keys...)
	}
	if len(jwks) == 0 {
		err = errors.New(fmt.Sprint("no published keys found for project ", projectId))
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	}
	cacheJWKSet(projectId, jwks)
	return jwks, nil
}

func (ms *ModuleStore) FetchJWKKeys(ctx context.Context, projectId string, kid string, realStore ModuleStoreI) (jwks []erursa.JWK, err error) {
	rsakeyPair, err := ms.GetKid(ctx, fmt.Sprint("ERUAUTH_KID_", kid), projectId, realStore)
	if err != nil {
		return
	}
	jwk, err := erursa.RsaPublicKeyToJWK(ctx, rsakeyPair.PublicKey, kid)
	if err != nil {
		return
	}
	jwks = append(jwks, jwk)
	return
}
func LoadStore(ctx context.Context, StoreTableName string, StoreTenantTableName string) (ModuleStoreI, error) {
	logs.WithContext(ctx).Info("Loading store")
	storeType := strings.ToUpper(os.Getenv("STORE_TYPE"))
	if storeType == "" {
		storeType = "STANDALONE"
		logs.WithContext(ctx).Info("STORE_TYPE environment variable not found - loading default standlone store")
	}
	var myStore ModuleStoreI
	var err error
	switch storeType {
	case "POSTGRES":
		myStore = new(ModuleDbStore)
		myStore.SetDbType(storeType)
		myStore.SetStoreTableName(StoreTableName)
		//myStore.SetStoreTenantTableName(StoreTenantTableName)
		myStore.CreateConn()
	case "STANDALONE":
		// myStore, err = store.LoadStoreFromFile()
		myStore = new(ModuleFileStore)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New(fmt.Sprint("Invalid STORE_TYPE ", storeType))
	}
	storeBytes, err := myStore.GetStoreByteArray("")
	if err == nil {
		UnMarshalStore(ctx, storeBytes, myStore)
	} else {
		logs.WithContext(ctx).Error(err.Error())
	}
	//s.Store = myStore
	return myStore, err
}
