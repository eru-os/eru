package module_store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/eru-os/eru/eru-gateway/module_model"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-secret-manager/sm"
	"github.com/eru-os/eru/eru-server/server"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/go-cmp/cmp"
)

type StoreHolder struct {
	sync.RWMutex
	Store ModuleStoreI
}
type ModuleStoreI interface {
	store.StoreI
	SaveListenerRule(ctx context.Context, istenerRule *module_model.ListenerRule, realStore ModuleStoreI, persist bool) error
	ReplaceListenerRule(ctx context.Context, listenerRule *module_model.ListenerRule) error
	RemoveListenerRule(ctx context.Context, listenerRuleName string, realStore ModuleStoreI) error
	GetListenerRules(ctx context.Context) []*module_model.ListenerRule
	SortListenerRules(ctx context.Context)
	GetListenerRule(ctx context.Context, listenerRuleName string) (*module_model.ListenerRule, error)
	GetTargetGroupAuthorizer(ctx context.Context, r *http.Request) (module_model.TargetHost, module_model.Authorizer, []module_model.MapStructCustom, string, error)
	SaveAuthorizer(ctx context.Context, authorizer module_model.Authorizer, realStore ModuleStoreI, persist bool) error
	RemoveAuthorizer(ctx context.Context, authorizerName string, realStore ModuleStoreI) error
	GetAuthorizer(ctx context.Context, authorizerName string) (module_model.Authorizer, error)
	GetAuthorizers(ctx context.Context) map[string]module_model.Authorizer
	CompareModuleStore(ctx context.Context, ms ExendedModuleStore, realStore ModuleStoreI) (module_model.StoreCompare, error)
	SaveProjectSettings(ctx context.Context, projectSettings module_model.ProjectSettings, realStore ModuleStoreI, persist bool) error
	GetProjectSettings(ctx context.Context) module_model.ProjectSettings
	GetGatewayConfig(ctx context.Context) ModuleStore
	GetExtendedGatewayConfig(ctx context.Context, realStore ModuleStoreI) ExendedModuleStore
}

const MatchTypePrefix = "PREFIX"
const MatchTypeExact = "EXACT"

type ExendedModuleStore struct {
	ModuleStore
	Variables     store.Variables `json:"variables"`
	SecretManager sm.SmStoreI     `json:"secret_manager"`
}

type ModuleStore struct {
	ListenerRules   []*module_model.ListenerRule       `json:"listener_rules" eru:"required"`
	Authorizers     map[string]module_model.Authorizer `json:"authorizers"`
	ProjectSettings module_model.ProjectSettings       `json:"project_settings"`
}

type ModuleFileStore struct {
	store.FileStore
	ModuleStore
}
type ModuleDbStore struct {
	store.DbStore
	ModuleStore
}

func (ms *ModuleStore) GetTargetGroupAuthorizer(ctx context.Context, r *http.Request) (module_model.TargetHost, module_model.Authorizer, []module_model.MapStructCustom, string, error) {
	logs.WithContext(ctx).Debug("GetTargetGroupAuthorizer - Start")
	listenerRuleFound := false
	instanceId := ""
	if ms.ListenerRules != nil {
		// ListenerRules is kept sorted by rule_rank, so the first rule that matches is the one the
		// configured precedence intends.
		for _, v := range ms.ListenerRules {
			// A rule matches when every criterion it sets matches, and a criterion that lists several
			// values matches when any one of them does. A criterion left empty places no constraint.
			// A rule that sets no criterion at all matches nothing, so an empty rule never swallows
			// traffic.
			if len(v.Hosts)+len(v.Methods)+len(v.Paths)+len(v.Headers)+len(v.Params)+len(v.SourceIP) == 0 {
				continue
			}
			listenerRuleFound = true

			//check for hosts
			if len(v.Hosts) > 0 {
				listenerRuleFound = false
				for _, host := range v.Hosts {
					if strings.Split(r.Host, ":")[0] == host {
						logs.WithContext(ctx).Info(fmt.Sprint("host match = ", host))
						listenerRuleFound = true
						break
					}
				}
			}

			//check for Methods
			if listenerRuleFound && len(v.Methods) > 0 {
				listenerRuleFound = false
				for _, method := range v.Methods {
					if r.Method == method {
						logs.WithContext(ctx).Info(fmt.Sprint("method match = ", method))
						listenerRuleFound = true
						break
					}
				}
			}

			//check for Paths
			if listenerRuleFound && len(v.Paths) > 0 {
				listenerRuleFound = false
				for _, path := range v.Paths {
					if matchPath(r.URL.Path, path) {
						logs.WithContext(ctx).Info(fmt.Sprint("path match = ", path.Path))
						listenerRuleFound = true
						break
					}
				}
			}

			//check for Headers
			if listenerRuleFound && len(v.Headers) > 0 {
				listenerRuleFound = false
				for _, header := range v.Headers {
					if r.Header.Get(header.Key) == header.Value {
						logs.WithContext(ctx).Info(fmt.Sprint("header match = ", header.Key, " = ", header.Value))
						listenerRuleFound = true
						break
					}
				}
			}
			if r.Header.Get("instance_id") != "" {
				instanceId = r.Header.Get("instance_id")
			}

			//check for Params
			reqParams := r.URL.Query()
			if listenerRuleFound && len(v.Params) > 0 {
				listenerRuleFound = false
				for _, param := range v.Params {
					if reqParams.Get(param.Key) == param.Value {
						logs.WithContext(ctx).Info(fmt.Sprint("param match = ", param.Key, " = ", param.Value))
						listenerRuleFound = true
						break
					}
				}
			}
			if reqParams.Get("instance_id") != "" {
				instanceId = reqParams.Get("instance_id")
			}
			r.URL.RawQuery = reqParams.Encode()

			//check for SourceIP
			if listenerRuleFound && len(v.SourceIP) > 0 {
				listenerRuleFound = false
				for _, sourceIP := range v.SourceIP {
					if strings.Split(r.RemoteAddr, ":")[0] == sourceIP {
						logs.WithContext(ctx).Info(fmt.Sprint("sourceIP match = ", sourceIP))
						listenerRuleFound = true
						break
					}
				}
			}
			logs.WithContext(ctx).Info(fmt.Sprint("listenerRuleFound = ", listenerRuleFound))
			if listenerRuleFound {
				pathExceptionFound := false
				for _, pathException := range v.AuthorizerException {
					if matchPath(r.URL.Path, pathException) {
						logs.WithContext(ctx).Info(fmt.Sprint("pathException ", pathException.MatchType, " = ", pathException.Path))
						pathExceptionFound = true
						r.Header.Set("is_public", "true")
						break
					}
				}
				// The auth this rule is guarded by travels with the request, so a service can resolve
				// its own auth config without the rule repeating it in add_headers. It is stamped even
				// on the exception path, where the request is not authorized but still belongs to the
				// same auth, and it is always overwritten so a caller cannot supply its own.
				ms.setAuthNameHeader(ctx, r, v.AuthorizerName)

				if pathExceptionFound || v.AuthorizerName == "" {
					return v.TargetHosts[0], module_model.Authorizer{}, v.AddHeaders, instanceId, nil
				} else {
					authorizer, err := ms.GetAuthorizer(ctx, v.AuthorizerName)
					if err != nil {
						return module_model.TargetHost{}, module_model.Authorizer{}, nil, instanceId, err
					}
					return v.TargetHosts[0], authorizer, v.AddHeaders, instanceId, nil
				}
			}
		}
	}
	r.Header.Del(server.AuthNameHeaderKey)
	err := errors.New(fmt.Sprint("Listener Rule not found for request host = ", r.Host, " and path = ", r.URL))
	logs.WithContext(ctx).Error(err.Error())
	//TODO add_headers
	return module_model.TargetHost{}, module_model.Authorizer{}, nil, instanceId, err
}

// setAuthNameHeader stamps the auth behind the listener rule's authorizer onto the request. The
// authorizer may name the auth explicitly through AuthName; otherwise the authorizer name is taken
// to be the auth name, which is the existing convention.
func (ms *ModuleStore) setAuthNameHeader(ctx context.Context, r *http.Request, authorizerName string) {
	r.Header.Del(server.AuthNameHeaderKey)
	if authorizerName == "" {
		return
	}
	authName := authorizerName
	if authorizer, err := ms.GetAuthorizer(ctx, authorizerName); err == nil && authorizer.AuthName != "" {
		authName = authorizer.AuthName
	}
	r.Header.Set(server.AuthNameHeaderKey, authName)
}

// SortListenerRules orders the rules by rule_rank ascending, so the lowest rank is matched first.
// Ranks are allowed to repeat: a stable sort leaves rules that share a rank in the order they were
// already in, so a tie stays put rather than shuffling on every save.
// matchPath applies one path rule to a request path. MatchType is compared case insensitively so a
// rule saved with "prefix" rather than "PREFIX" matches rather than being silently ignored.
func matchPath(requestPath string, path module_model.PathStruct) bool {
	switch strings.ToUpper(path.MatchType) {
	case MatchTypePrefix:
		return strings.HasPrefix(requestPath, path.Path)
	case MatchTypeExact:
		return requestPath == path.Path
	default:
		return false
	}
}

func (ms *ModuleStore) SortListenerRules(ctx context.Context) {
	logs.WithContext(ctx).Debug("SortListenerRules - Start")
	sort.SliceStable(ms.ListenerRules, func(i, j int) bool {
		return ms.ListenerRules[i].RuleRank < ms.ListenerRules[j].RuleRank
	})
}

func (ms *ModuleStore) GetListenerRule(ctx context.Context, listenerRuleName string) (*module_model.ListenerRule, error) {
	logs.WithContext(ctx).Debug("GetListenerRule - Start")
	if ms.ListenerRules != nil {
		for _, v := range ms.ListenerRules {
			if v.RuleName == listenerRuleName {
				return v, nil
			}
		}
	}
	err := errors.New(fmt.Sprint("Listener Rule ", listenerRuleName, " not found"))
	logs.WithContext(ctx).Info(err.Error())
	return nil, err
}

func (ms *ModuleStore) ReplaceListenerRule(ctx context.Context, listenerRule *module_model.ListenerRule) error {
	logs.WithContext(ctx).Debug("ReplaceListenerRule - Start")
	if ms.ListenerRules != nil {
		for i, v := range ms.ListenerRules {
			if v.RuleName == listenerRule.RuleName {
				ms.ListenerRules[i] = listenerRule
				return nil
			}
		}
	}
	err := errors.New(fmt.Sprint("Listener Rule ", listenerRule.RuleName, " not found"))
	logs.WithContext(ctx).Info(err.Error())
	return err
}

func (ms *ModuleStore) SaveListenerRule(ctx context.Context, listenerRule *module_model.ListenerRule, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveListenerRule - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	err := ms.ReplaceListenerRule(ctx, listenerRule)
	if err != nil {
		ms.ListenerRules = append(ms.ListenerRules, listenerRule)
	}
	ms.SortListenerRules(ctx)
	if persist == true {
		logs.WithContext(ctx).Info("SaveStore called from SaveListenerRule")
		return realStore.SaveStore(ctx, "gateway", "", realStore)
	} else {
		return nil
	}
}
func (ms *ModuleStore) RemoveListenerRule(ctx context.Context, listenerRuleName string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveListenerRule - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if ms.ListenerRules != nil {
		for i, v := range ms.ListenerRules {
			if v.RuleName == listenerRuleName {
				ms.ListenerRules = append(ms.ListenerRules[:i], ms.ListenerRules[i+1:]...)
				return realStore.SaveStore(ctx, "gateway", "", realStore)
			}
		}
	}
	err := errors.New(fmt.Sprint("Listener Rule ", listenerRuleName, " not found"))
	logs.WithContext(ctx).Info(err.Error())
	return err
}

func (ms *ModuleStore) GetListenerRules(ctx context.Context) []*module_model.ListenerRule {
	logs.WithContext(ctx).Debug("GetListenerRules - Start")
	return ms.ListenerRules
}

func (ms *ModuleStore) GetGatewayConfig(ctx context.Context) ModuleStore {
	logs.WithContext(ctx).Debug("GetGatewayConfig - Start")
	return *ms
}

func (ms *ModuleStore) GetExtendedGatewayConfig(ctx context.Context, realStore ModuleStoreI) (ems ExendedModuleStore) {
	logs.WithContext(ctx).Debug("GetExtendedGatewayConfig - Start")
	var err error
	ems.Variables, err = realStore.FetchVars(ctx, "gateway")
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	ems.SecretManager, err = realStore.FetchSm(ctx, "gateway")
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	ems.Authorizers = ms.Authorizers
	ems.ListenerRules = ms.ListenerRules
	ems.ProjectSettings = ms.ProjectSettings
	return ems
}

func (ms *ModuleStore) SaveAuthorizer(ctx context.Context, authorizer module_model.Authorizer, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveAuthorizer - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	if ms.Authorizers == nil {
		ms.Authorizers = make(map[string]module_model.Authorizer)
	}
	ms.Authorizers[authorizer.AuthorizerName] = authorizer
	if persist == true {
		logs.WithContext(ctx).Info("SaveStore called from SaveAuthorizer")
		return realStore.SaveStore(ctx, "gateway", "", realStore)
	} else {
		return nil
	}

}

func (ms *ModuleStore) RemoveAuthorizer(ctx context.Context, authorizerName string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveAuthorizer - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if _, authOk := ms.Authorizers[authorizerName]; authOk {
		delete(ms.Authorizers, authorizerName)
		return realStore.SaveStore(ctx, "gateway", "", realStore)
	} else {
		return errors.New(fmt.Sprint("Authorizer ", authorizerName, " not found"))
	}

}
func (ms *ModuleStore) GetAuthorizer(ctx context.Context, authorizerName string) (module_model.Authorizer, error) {
	logs.WithContext(ctx).Debug("GetAuthorizer - Start")
	if _, authOk := ms.Authorizers[authorizerName]; authOk {
		return ms.Authorizers[authorizerName], nil
	} else {
		return module_model.Authorizer{}, errors.New(fmt.Sprint("Authorizer ", authorizerName, " not found"))
	}

}

func (ms *ModuleStore) GetAuthorizers(ctx context.Context) map[string]module_model.Authorizer {
	logs.WithContext(ctx).Debug("GetAuthorizers - Start")
	return ms.Authorizers
}

func (ms *ModuleStore) GetProjectSettings(ctx context.Context) module_model.ProjectSettings {
	logs.WithContext(ctx).Debug("GetProjectSettings - Start")
	return ms.ProjectSettings
}

func (ms *ModuleStore) CompareModuleStore(ctx context.Context, cms ExendedModuleStore, realStore ModuleStoreI) (module_model.StoreCompare, error) {
	storeCompare := module_model.StoreCompare{}
	vars, err := realStore.FetchVars(ctx, "gateway")
	if err != nil {
		logs.WithContext(ctx).Warn(fmt.Sprint("ignoring variables to compare : ", err.Error()))
	}
	storeCompare.CompareVariables(ctx, vars, cms.Variables)

	sm, smerr := realStore.FetchSm(ctx, "gateway")
	if smerr != nil {
		logs.WithContext(ctx).Warn(fmt.Sprint("ignoring secret manager to compare : ", smerr.Error()))
	}
	storeCompare.CompareSecretManager(ctx, sm, cms.SecretManager)

	var oDiffR utils.DiffReporter
	if !cmp.Equal(ms.ProjectSettings, cms.ProjectSettings, cmp.Reporter(&oDiffR)) {
		if storeCompare.MismatchSettings == nil {
			storeCompare.MismatchSettings = make(map[string]interface{})
		}
		storeCompare.MismatchSettings["settings"] = oDiffR.Output()
	}

	for _, mlr := range ms.ListenerRules {
		var diffR utils.DiffReporter
		lrFound := false
		for _, clr := range cms.ListenerRules {
			if mlr.RuleName == clr.RuleName {
				lrFound = true
				logs.Logger.Info(fmt.Sprint(mlr))
				logs.Logger.Info(fmt.Sprint(clr))
				if !cmp.Equal(*mlr, *clr, cmp.Reporter(&diffR)) {
					if storeCompare.MismatchListenerRules == nil {
						storeCompare.MismatchListenerRules = make(map[string]interface{})
					}
					storeCompare.MismatchListenerRules[mlr.RuleName] = diffR.Output()
				}
				break
			}
		}
		if !lrFound {
			storeCompare.DeleteListenerRules = append(storeCompare.DeleteListenerRules, mlr.RuleName)
		}
	}

	for _, clr := range cms.ListenerRules {
		lrFound := false
		for _, mlr := range ms.ListenerRules {
			if mlr.RuleName == clr.RuleName {
				lrFound = true
				break
			}
		}
		if !lrFound {
			storeCompare.NewListenerRules = append(storeCompare.NewListenerRules, clr.RuleName)
		}
	}

	//compare authorizer
	for _, mlr := range ms.Authorizers {
		var diffR utils.DiffReporter
		authFound := false
		for _, auth := range cms.Authorizers {
			if mlr.AuthorizerName == auth.AuthorizerName {
				authFound = true
				if !cmp.Equal(mlr, auth, cmp.Reporter(&diffR)) {
					if storeCompare.MismatchAuthorizer == nil {
						storeCompare.MismatchAuthorizer = make(map[string]interface{})
					}
					storeCompare.MismatchAuthorizer[mlr.AuthorizerName] = diffR.Output()
				}
				break
			}
		}
		if !authFound {
			storeCompare.DeleteAuthorizer = append(storeCompare.DeleteAuthorizer, mlr.AuthorizerName)
		}
	}

	for _, auth := range cms.Authorizers {
		authFound := false
		for _, mlr := range ms.Authorizers {
			if mlr.AuthorizerName == auth.AuthorizerName {
				authFound = true
				break
			}
		}
		if !authFound {
			storeCompare.NewAuthorizer = append(storeCompare.NewAuthorizer, auth.AuthorizerName)
		}
	}
	return storeCompare, nil
}

func (ms *ModuleStore) SaveProjectSettings(ctx context.Context, projectSettings module_model.ProjectSettings, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveProjectConfig - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}

	ms.ProjectSettings = projectSettings
	if persist == true {
		logs.WithContext(ctx).Info("SaveStore called from SaveAuthorizer")
		return realStore.SaveStore(ctx, "gateway", "", realStore)
	} else {
		return nil
	}
}

func (eMs *ExendedModuleStore) UnmarshalJSON(b []byte) error {
	logs.Logger.Info("UnMarshal ExendedModuleStore - Start")
	ctx := context.Background()
	var ePrjMap map[string]*json.RawMessage
	err := json.Unmarshal(b, &ePrjMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	var ps module_model.ProjectSettings
	if _, ok := ePrjMap["project_settings"]; ok {
		if ePrjMap["project_settings"] != nil {
			err = json.Unmarshal(*ePrjMap["project_settings"], &ps)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			eMs.ProjectSettings = ps
		}
	}

	var vars store.Variables
	if _, ok := ePrjMap["variables"]; ok {
		if ePrjMap["variables"] != nil {
			err = json.Unmarshal(*ePrjMap["variables"], &vars)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			eMs.Variables = vars
		}
	}

	//ListenerRules   []*module_model.ListenerRule       `json:"listener_rules" eru:"required"`
	//Authorizers     map[string]module_model.Authorizer `json:"authorizers"`

	var ak map[string]module_model.Authorizer
	if _, ok := ePrjMap["authorizers"]; ok {
		if ePrjMap["authorizers"] != nil {
			err = json.Unmarshal(*ePrjMap["authorizers"], &ak)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			eMs.Authorizers = ak
		}
	}

	var lr []*module_model.ListenerRule
	if _, ok := ePrjMap["listener_rules"]; ok {
		if ePrjMap["listener_rules"] != nil {
			err = json.Unmarshal(*ePrjMap["listener_rules"], &lr)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			eMs.ListenerRules = lr
		}
	}

	var smObj map[string]*json.RawMessage
	var smJson *json.RawMessage
	if _, ok := ePrjMap["secret_manager"]; ok {
		if ePrjMap["secret_manager"] != nil {
			err = json.Unmarshal(*ePrjMap["secret_manager"], &smObj)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			err = json.Unmarshal(*ePrjMap["secret_manager"], &smJson)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}

			var smType string
			if _, stOk := smObj["sm_store_type"]; stOk {
				err = json.Unmarshal(*smObj["sm_store_type"], &smType)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				smI := sm.GetSm(smType)
				err = smI.MakeFromJson(ctx, smJson)
				if err == nil {
					eMs.SecretManager = smI
				} else {
					return err
				}
			} else {
				logs.WithContext(ctx).Info("ignoring secret manager as sm_store_type attribute not found")
			}
		} else {
			logs.WithContext(ctx).Info("secret manager attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("secret manager attribute not found in store")
	}

	return nil
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
		//myStore.CreateConn()
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
		err = json.Unmarshal(storeBytes, myStore)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		err = myStore.SetStoreFromBytes(ctx, storeBytes, myStore)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
		myStore.SortListenerRules(ctx)
	} else {
		logs.WithContext(ctx).Error(err.Error())
	}
	//s.Store = myStore
	return myStore, err
}
