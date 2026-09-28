package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/eru-os/eru/eru-cache/cache"
	db "github.com/eru-os/eru/eru-db/db"
	"github.com/eru-os/eru/eru-events/events"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	"github.com/eru-os/eru/eru-read-write/validator"
	repos "github.com/eru-os/eru/eru-repos/repos"
	scheduler "github.com/eru-os/eru/eru-scheduler/scheduler"
	kms "github.com/eru-os/eru/eru-secret-manager/kms"
	sm "github.com/eru-os/eru/eru-secret-manager/sm"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jmoiron/sqlx"
	"github.com/tidwall/gjson"
)

const (
	SELECT_REQUEST = "select * from eru_requests where project_id=??? and tenant_id=??? and resource_name=??? and (request_name=??? or 'ALL'=???)"
	SAVE_REQUEST   = "insert into eru_requests (request_id, request_name, resource_name, project_id, tenant_id, request_json) values (???, ???, ???, ???, ???, ???) on conflict (request_id) do update set request_json=EXCLUDED.request_json , request_name=EXCLUDED.request_name"
	DELETE_REQUEST = "delete from eru_requests where request_id=??? returning request_id"
)

var InstanceId = "unknown"
var ServiceName = "unknown"
var BaseUrl = "unknown"

type StoreI interface {
	LoadStore(fp string, ms StoreI) (err error)
	GetStoreByteArray(fp string) (b []byte, err error)
	SaveStore(ctx context.Context, projectId string, fp string, ms StoreI) (err error)
	SaveTenantStore(ctx context.Context, projectId string, tenantId string, fp string, tenantConfig interface{}) (err error)
	SaveTenantObject(ctx context.Context, tableName string, idColumn string, nameColumn string, projectId string, tenantId string, name string, config interface{}, ms StoreI) (err error)
	RemoveTenantObject(ctx context.Context, tableName string, idColumn string, nameColumn string, projectId string, tenantId string, name string, ms StoreI) (err error)
	SetStoreTenantLoadQuery(query string)
	SetDbType(dbtype string)
	CreateConn() error
	GetConn() *sqlx.DB
	GetDbType() string
	ExecuteDbSave(ctx context.Context, queries []Queries) (output [][]map[string]interface{}, err error)
	ExecuteDbFetch(ctx context.Context, query Queries) (output []map[string]interface{}, err error)
	SetStoreTableName(tablename string)
	SetStoreTenantTableName(tablename string)
	GetStoreTableName() (tablename string)
	GetStoreWithoutTenants(ctx context.Context, ms StoreI) (b []byte, err error)
	GetStoreTenantTableName() (tablename string)
	SetVars(ctx context.Context, variables map[string]Variables)
	SetTenantVars(ctx context.Context, tenantVariables map[string]map[string]Variables)
	SaveVar(ctx context.Context, projectId string, newVar Vars, s StoreI) (err error)
	RemoveVar(ctx context.Context, projectId string, key string, s StoreI) (err error)
	SaveEnvVar(ctx context.Context, projectId string, newEnvVar EnvVars, s StoreI) (err error)
	RemoveEnvVar(ctx context.Context, projectId string, key string, s StoreI) (err error)
	SaveSecret(ctx context.Context, projectId string, newSecret Secrets, s StoreI) (err error)
	RemoveSecret(ctx context.Context, projectId string, key string, s StoreI) (err error)
	FetchVars(ctx context.Context, projectId string) (variables Variables, err error)
	FetchTenantVars(ctx context.Context, projectId string) (variables map[string]Variables, err error)
	ReplaceVariables(ctx context.Context, projectId string, text []byte, varMap map[string]interface{}) (returnText []byte)
	ReplaceTenantVariables(ctx context.Context, projectId string, tenantId string, conversationId string, text []byte) (returnText []byte)
	SaveTenantSecret(ctx context.Context, projectId string, tenantId string, newSecret Secrets, s StoreI) (err error)
	RemoveTenantSecret(ctx context.Context, projectId string, tenantId string, key string, s StoreI) (err error)
	SaveRepo(ctx context.Context, projectId string, repo repos.RepoI, s StoreI, persist bool) (err error)
	SaveRepoToken(ctx context.Context, projectId string, repo repos.RepoToken, s StoreI) (err error)
	FetchRepo(ctx context.Context, projectId string) (repo repos.RepoI, err error)
	CommitRepo(ctx context.Context, projectId string, ms StoreI) (err error)
	GetProjectConfigForRepo(ctx context.Context, projectId string, ms StoreI) (repoData map[string]map[string]interface{}, accessToken string, err error)
	SaveSm(ctx context.Context, projectId string, secretManager sm.SmStoreI, s StoreI, persist bool) (err error)
	FetchSm(ctx context.Context, projectId string) (sm sm.SmStoreI, err error)
	LoadSmValue(ctx context.Context, projectId string) (err error)
	SetSmValue(ctx context.Context, projectId string, secretName string, secretValue map[string]string) (err error)
	UnsetSmValue(ctx context.Context, projectId string, secretName string, secretKey string) (err error)
	GetSmValue(ctx context.Context, projectId string, secretName string, secretKey string, force_delete bool) (secret_Value interface{}, err error)
	GetProjectSecret(ctx context.Context, projectId string, key string) (value string, err error)
	LoadEnvValue(ctx context.Context, projectId string) (err error)
	SetStoreFromBytes(ctx context.Context, storeBytes []byte, msi StoreI) (err error)
	GetMutex() *sync.RWMutex
	FetchKms(ctx context.Context, projectId string) (kms map[string]kms.KmsStoreI, err error)
	SaveKms(ctx context.Context, projectId string, kms kms.KmsStoreI, s StoreI, persist bool) (err error)
	RemoveKms(ctx context.Context, projectId string, kmsName string, cloudDelete bool, deleteDays int32, s StoreI) (err error)
	GetCacheValue(ctx context.Context, projectId string, key string) (value interface{}, err error)
	SetCacheValue(ctx context.Context, projectId string, key string, value interface{}) (err error)
	ValidateJSON(ctx context.Context, schema validator.Schema, data []interface{}) (records []interface{}, errRecords []interface{})
	FetchEvents(ctx context.Context, projectId string) (events map[string]events.EventI, err error)
	FetchEvent(ctx context.Context, projectId string, eventName string, s StoreI) (event events.EventI, err error)
	SaveEvent(ctx context.Context, projectId string, event events.EventI, s StoreI, persist bool) (err error)
	CloneEvent(ctx context.Context, projectId string, event events.EventI, s StoreI) (eClone events.EventI, err error)
	RemoveEvent(ctx context.Context, projectId string, eventName string, cloudDelete bool, s StoreI) (err error)
	PublishEvent(ctx context.Context, projectId string, eventName string, msg interface{}, s StoreI) (msgId string, err error)
	PollEvent(ctx context.Context, projectId string, eventName string, s StoreI) (err error)
	SaveScheduler(ctx context.Context, projectId string, schedulerObj scheduler.SchedulerI, s StoreI, persist bool) (err error)
	FetchScheduler(ctx context.Context, projectId string) (schedulerObj scheduler.SchedulerI, err error)
	InitScheduler(ctx context.Context, s StoreI) (err error)
	SaveRequest(ctx context.Context, request models.SampleRequest, projectId string, tenantId string, s StoreI) (err error)
	GetRequests(ctx context.Context, projectId string, tenantId string, resourceName string, s StoreI) (requests []models.SampleRequest, err error)
	RemoveRequest(ctx context.Context, requestId string, s StoreI) (err error)
	SetServiceName(serviceName string)
	SetInstanceId(instanceId string)
	SetBaseUrl(baseUrl string)
	GetUpdateTime() time.Time
}

type Store struct {
	//Projects map[string]*model.Project //ProjectId is the key
	mu                sync.RWMutex
	Variables         map[string]Variables                `json:"variables"`
	TenantVariables   map[string]map[string]Variables     `json:"tenant_variables"`
	ProjectRepos      map[string]repos.RepoI              `json:"repos"`
	ProjectRepoTokens map[string]repos.RepoToken          `json:"repo_token"`
	SecretManager     map[string]sm.SmStoreI              `json:"secret_manager"`
	Scheduler         map[string]scheduler.SchedulerI     `json:"scheduler"`
	KMS               map[string]map[string]kms.KmsStoreI `json:"kms"`
	Events            map[string]map[string]events.EventI `json:"events"`
	CacheStore        map[string]cache.CacheStoreI        `json:"-"`
}

type StoreCompare struct {
	DeleteVariables       []string               `json:"delete_variables"`
	NewVariables          []string               `json:"new_variables"`
	DeleteEnvVariables    []string               `json:"delete_env_variables"`
	NewEnvVariables       []string               `json:"new_env_variables"`
	DeleteSecrets         []string               `json:"delete_secrets"`
	NewSecrets            []string               `json:"new_secrets"`
	MismatchSettings      map[string]interface{} `json:"mismatch_settings"`
	MismatchSecretManager map[string]interface{} `json:"mismatch_secret_manager"`
}

func (store *Store) GetUpdateTime() time.Time {
	return time.Time{}
}
func (storeCompare *StoreCompare) CompareSecretManager(ctx context.Context, orgSm sm.SmStoreI, compareSm sm.SmStoreI) {
	var diffR utils.DiffReporter
	if !cmp.Equal(orgSm, compareSm, cmpopts.IgnoreUnexported(sm.AwsSmStore{}), cmp.Reporter(&diffR)) {
		if storeCompare.MismatchSecretManager == nil {
			storeCompare.MismatchSecretManager = make(map[string]interface{})
		}
		storeCompare.MismatchSecretManager["sm"] = diffR.Output()
	}
}

func (storeCompare *StoreCompare) CompareVariables(ctx context.Context, orgVars Variables, compareVars Variables) {
	//variables
	for k, _ := range orgVars.Vars {
		varFound := false
		for ck, _ := range compareVars.Vars {
			if k == ck {
				varFound = true
				break
			}
		}
		if !varFound {
			storeCompare.DeleteVariables = append(storeCompare.DeleteVariables, k)
		}
	}

	for ck, _ := range compareVars.Vars {
		varFound := false
		for k, _ := range orgVars.Vars {
			if k == ck {
				varFound = true
				break
			}
		}
		if !varFound {
			storeCompare.NewVariables = append(storeCompare.NewVariables, ck)
		}
	}

	// env variables
	for k, _ := range orgVars.EnvVars {
		varFound := false
		for ck, _ := range compareVars.EnvVars {
			if k == ck {
				varFound = true
				break
			}
		}
		if !varFound {
			storeCompare.DeleteEnvVariables = append(storeCompare.DeleteEnvVariables, k)
		}
	}

	for ck, _ := range compareVars.EnvVars {
		varFound := false
		for k, _ := range orgVars.EnvVars {
			if k == ck {
				varFound = true
				break
			}
		}
		if !varFound {
			storeCompare.NewEnvVariables = append(storeCompare.NewEnvVariables, ck)
		}
	}

	// secrets
	for k, _ := range orgVars.Secrets {
		varFound := false
		for ck, _ := range compareVars.Secrets {
			if k == ck {
				varFound = true
				break
			}
		}
		if !varFound {
			storeCompare.DeleteSecrets = append(storeCompare.DeleteSecrets, k)
		}
	}

	for ck, _ := range compareVars.Secrets {
		varFound := false
		for k, _ := range orgVars.Secrets {
			if k == ck {
				varFound = true
				break
			}
		}
		if !varFound {
			storeCompare.NewSecrets = append(storeCompare.NewSecrets, ck)
		}
	}
	return
}

func (store *Store) GetMutex() *sync.RWMutex {
	return &store.mu
}

func (store *Store) LoadStore(fp string, ms StoreI) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(context.Background()).Error(err.Error())
	return
}

func (store *Store) GetStoreByteArray(fp string) (b []byte, err error) {
	err = errors.New("method not implemented")
	logs.WithContext(context.Background()).Error(err.Error())
	return
}

func (store *Store) SaveStore(ctx context.Context, projectId string, fp string, ms StoreI) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(context.Background()).Error(err.Error())
	return
}
func (store *Store) SaveTenantStore(ctx context.Context, projectId string, tenantId string, fp string, tenantConfig interface{}) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(context.Background()).Error(err.Error())
	return
}
func (store *Store) SaveTenantObject(ctx context.Context, tableName string, idColumn string, nameColumn string, projectId string, tenantId string, name string, config interface{}, ms StoreI) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(context.Background()).Error(err.Error())
	return
}
func (store *Store) RemoveTenantObject(ctx context.Context, tableName string, idColumn string, nameColumn string, projectId string, tenantId string, name string, ms StoreI) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(context.Background()).Error(err.Error())
	return
}

type Variables struct {
	Vars    map[string]Vars    `json:"vars"`
	EnvVars map[string]EnvVars `json:"env_vars"`
	Secrets map[string]Secrets `json:"secrets"`
}

type Vars struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type EnvVars struct {
	Key   string `json:"key"`
	Value string `json:"-"`
}

type Secrets struct {
	Key         string `json:"key"`
	Value       string `json:"-"`
	SecretValue string `json:"secret_value"`
}

func (store *Store) GetDbType() string {
	return ""
}

func (store *Store) SetVars(ctx context.Context, variables map[string]Variables) {
	store.GetMutex().Lock()
	defer store.GetMutex().Unlock()
	store.Variables = variables
}

func (store *Store) SetTenantVars(ctx context.Context, tenantVariables map[string]map[string]Variables) {
	store.GetMutex().Lock()
	defer store.GetMutex().Unlock()
	store.TenantVariables = tenantVariables
}

func (store *Store) FetchVars(ctx context.Context, projectId string) (variables Variables, err error) {
	logs.WithContext(ctx).Debug("FetchVars - Start")
	if store.Variables == nil {
		err = errors.New("no variables defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return Variables{}, err
	}
	ok := false
	if variables, ok = store.Variables[projectId]; !ok {
		err = errors.New(fmt.Sprint("Variables not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return Variables{}, err
	}
	return
}

func (store *Store) SaveVar(ctx context.Context, projectId string, newVar Vars, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("SaveVar - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Variables == nil {
		store.Variables = make(map[string]Variables)
	}
	var v Variables
	ok := false
	if v, ok = store.Variables[projectId]; !ok {
		logs.WithContext(ctx).Info(fmt.Sprint("making new variable object for project : ", projectId))
		store.Variables[projectId] = Variables{}
		v = store.Variables[projectId]
	}
	if v.Vars == nil {
		v.Vars = make(map[string]Vars)
	}
	v.Vars[newVar.Key] = newVar
	store.Variables[projectId] = v
	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) RemoveVar(ctx context.Context, projectId string, key string, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveVar - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Variables == nil {
		err = errors.New("No variables defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.Variables[projectId]; !ok {
		err = errors.New(fmt.Sprint("Variables not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.Variables[projectId].Vars[key]; !ok {
		err = errors.New(fmt.Sprint("Variable key not defined :", key))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	delete(store.Variables[projectId].Vars, key)
	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) FetchTenantVars(ctx context.Context, projectId string) (variables map[string]Variables, err error) {
	logs.WithContext(ctx).Debug("FetchTenantVars - Start")
	if store.TenantVariables == nil {
		err = errors.New("no variables defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return map[string]Variables{}, err
	}
	ok := false
	if variables, ok = store.TenantVariables[projectId]; !ok {
		err = errors.New(fmt.Sprint("Variables not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return map[string]Variables{}, err
	}
	return
}

func (store *Store) SaveEnvVar(ctx context.Context, projectId string, newEnvVar EnvVars, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("SaveEnvVar - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Variables == nil {
		store.Variables = make(map[string]Variables)
	}
	var v Variables
	ok := false
	if v, ok = store.Variables[projectId]; !ok {
		logs.WithContext(ctx).Info(fmt.Sprint("making new variable object for project : ", projectId))
		store.Variables[projectId] = Variables{}
		v = store.Variables[projectId]
	}
	if v.EnvVars == nil {
		v.EnvVars = make(map[string]EnvVars)
	}
	v.EnvVars[newEnvVar.Key] = newEnvVar
	store.Variables[projectId] = v
	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) RemoveEnvVar(ctx context.Context, projectId string, key string, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveEnvVar - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Variables == nil {
		err = errors.New("No variables defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.Variables[projectId]; !ok {
		err = errors.New(fmt.Sprint("Variables not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.Variables[projectId].EnvVars[key]; !ok {
		err = errors.New(fmt.Sprint("Env. Variable key not defined :", key))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	delete(store.Variables[projectId].EnvVars, key)
	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) SaveSecret(ctx context.Context, projectId string, newSecret Secrets, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("SaveSecret - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Variables == nil {
		store.Variables = make(map[string]Variables)
	}
	var v Variables
	ok := false
	if v, ok = store.Variables[projectId]; !ok {
		logs.WithContext(ctx).Info(fmt.Sprint("making new variable object for project : ", projectId))
		store.Variables[projectId] = Variables{}
		v = store.Variables[projectId]
	}
	if v.Secrets == nil {
		v.Secrets = make(map[string]Secrets)
	}

	sv := newSecret.SecretValue
	newSecret.SecretValue = ""
	v.Secrets[newSecret.Key] = newSecret

	store.Variables[projectId] = v
	err = s.SaveStore(ctx, projectId, "", s)

	if sm, ok := store.SecretManager[projectId]; ok {
		smValue := make(map[string]string)
		smValue[newSecret.Key] = sv
		store.SetSmValue(ctx, projectId, sm.GetSecretName(), smValue)
	}

	return
}

func (store *Store) RemoveSecret(ctx context.Context, projectId string, key string, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveSecret - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Variables == nil {
		err = errors.New("no variables defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.Variables[projectId]; !ok {
		err = errors.New(fmt.Sprint("Variables not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.Variables[projectId].Secrets[key]; !ok {
		err = errors.New(fmt.Sprint("Secret key not defined :", key))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	delete(store.Variables[projectId].Secrets, key)
	err = s.SaveStore(ctx, projectId, "", s)

	if sm, ok := store.SecretManager[projectId]; ok {
		store.UnsetSmValue(ctx, projectId, sm.GetSecretName(), key)
	}

	return
}

func (store *Store) SaveTenantSecret(ctx context.Context, projectId string, tenantId string, newSecret Secrets, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("SaveSecret - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()

	if store.TenantVariables == nil {
		store.TenantVariables = make(map[string]map[string]Variables)
	}

	if _, vOk := store.TenantVariables[projectId]; !vOk {
		logs.WithContext(ctx).Info(fmt.Sprint("making new variable object for project : ", projectId))
		store.TenantVariables[projectId] = map[string]Variables{}
	}
	if _, tvOk := store.TenantVariables[projectId][tenantId]; !tvOk {
		logs.WithContext(ctx).Info(fmt.Sprint("making new variable object for tenant : ", tenantId))
		store.TenantVariables[projectId][tenantId] = Variables{}

	}
	tv := store.TenantVariables[projectId][tenantId]

	if tv.Secrets == nil {
		tv.Secrets = make(map[string]Secrets)
	}
	tv.Secrets[newSecret.Key] = newSecret
	store.TenantVariables[projectId][tenantId] = tv
	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) RemoveTenantSecret(ctx context.Context, projectId string, tenantId string, key string, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveSecret - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.TenantVariables == nil {
		err = errors.New("no variables defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.TenantVariables[projectId]; !ok {
		err = errors.New(fmt.Sprint("Variables not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.TenantVariables[projectId][tenantId]; !ok {
		err = errors.New(fmt.Sprint("Variables not defined for tenant :", tenantId))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if _, ok := store.TenantVariables[projectId][tenantId].Secrets[key]; !ok {
		err = errors.New(fmt.Sprint("Secret key not defined :", key))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	delete(store.TenantVariables[projectId][tenantId].Secrets, key)
	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) SetDbType(dbtype string) {
	//do nothing
}

func (store *Store) SetStoreTableName(tablename string) {
	//do nothing
}

func (store *Store) GetStoreTableName() (tablename string) {
	return
}

func (store *Store) SetStoreTenantTableName(tablename string) {
	//do nothing
}

func (store *Store) SetStoreTenantLoadQuery(query string) {
	//do nothing
}

func (store *Store) GetStoreTenantTableName() (tablename string) {
	return
}

func (store *Store) CreateConn() error {
	logs.Logger.Info("CreateConn not implemented")
	return nil
}

func (store *Store) GetConn() *sqlx.DB {
	logs.Logger.Info("GetConn not implemented")
	return nil
}

func (store *Store) ExecuteDbFetch(ctx context.Context, query Queries) (output []map[string]interface{}, err error) {
	logs.Logger.Info("ExecuteDbFetch not implemented")
	return
}

func (store *Store) ExecuteDbSave(ctx context.Context, queries []Queries) (output [][]map[string]interface{}, err error) {
	logs.Logger.Info("ExecuteDbFetch not implemented")
	return
}

func (store *Store) ReplaceVariables(ctx context.Context, projectId string, text []byte, varMap map[string]interface{}) (returnText []byte) {
	logs.WithContext(ctx).Debug("ReplaceVariables - Start")
	textStr := string(text)
	textStr = strings.Replace(textStr, "$VAR_project_id", projectId, -1)
	if _, prjVarsOk := store.Variables[projectId]; prjVarsOk {
		for k, v := range store.Variables[projectId].Vars {
			textStr = strings.Replace(textStr, fmt.Sprint("$VAR_", k), v.Value, -1)
		}
		for k, v := range store.Variables[projectId].EnvVars {
			textStr = strings.Replace(textStr, fmt.Sprint("$ENV_", k), v.Value, -1)
		}
		for k, v := range store.Variables[projectId].Secrets {
			textStr = strings.Replace(textStr, fmt.Sprint("$SECRET_", k), v.Value, -1)
		}
	}
	if varMap != nil {
		for k, v := range varMap {
			if vStr, vStrOk := v.(string); vStrOk {
				textStr = strings.Replace(textStr, fmt.Sprint("$VAR_", k), vStr, -1)
			}

		}
	}
	return []byte(textStr)
}
func (store *Store) ReplaceTenantVariables(ctx context.Context, projectId string, tenantId string, conversationId string, text []byte) (returnText []byte) {
	logs.WithContext(ctx).Debug("ReplaceTenantVariables - Start")
	textStr := string(text)
	textStr = strings.Replace(textStr, "$VAR_tenant_id", tenantId, -1)
	textStr = strings.Replace(textStr, "$VAR_conversation_id", conversationId, -1)
	if _, prjVarsOk := store.TenantVariables[projectId]; prjVarsOk {
		if _, tenantVarsOk := store.TenantVariables[projectId][tenantId]; tenantVarsOk {
			for k, v := range store.TenantVariables[projectId][tenantId].Secrets {
				textStr = strings.Replace(textStr, fmt.Sprint("$SECRET_", k), v.Value, -1)
			}
		}
		if _, prgTenantVarsOk := store.TenantVariables[projectId][projectId]; prgTenantVarsOk {
			for k, v := range store.TenantVariables[projectId][projectId].Secrets {
				textStr = strings.Replace(textStr, fmt.Sprint("$SECRET_", k), v.Value, -1)
			}
		}
	}
	return []byte(textStr)
}

func (store *Store) FetchRepo(ctx context.Context, projectId string) (repo repos.RepoI, err error) {
	logs.WithContext(ctx).Debug("FetchRepo - Start")
	if store.ProjectRepos == nil {
		err = errors.New("no repo defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	ok := false
	if repo, ok = store.ProjectRepos[projectId]; !ok {
		err = errors.New(fmt.Sprint("Repo not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return
}

func (store *Store) CommitRepo(ctx context.Context, projectId string, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("CommitRepo - Start")
	repo, err := store.FetchRepo(ctx, projectId)
	if err != nil {
		return
	}

	repoConfigBytes, err := json.Marshal(repo)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	repoConfigBytes = s.ReplaceVariables(ctx, projectId, repoConfigBytes, nil)

	cloneRepo := repos.GetRepo(repo.GetAttribute("repo_type").(string))

	err = json.Unmarshal(repoConfigBytes, &cloneRepo)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	repoData, token, err := store.GetProjectConfigForRepo(ctx, projectId, s)
	if err != nil {
		return
	}
	_ = token
	repoBytes, err := json.MarshalIndent(repoData, "", " ")
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if token != "" {
		cloneRepo.SetAuthKey(ctx, token)
	}
	err = cloneRepo.Commit(ctx, repoBytes, fmt.Sprint(s.GetStoreTableName(), ".json"))
	if err != nil {
		return
	}
	repo.SetLastCommitAt()
	return
}

func (store *Store) SaveRepo(ctx context.Context, projectId string, repo repos.RepoI, s StoreI, persist bool) (err error) {
	logs.WithContext(ctx).Debug("SaveRepo - Start")
	if persist {
		s.GetMutex().Lock()
		defer s.GetMutex().Unlock()
	}
	if store.ProjectRepos == nil {
		store.ProjectRepos = make(map[string]repos.RepoI)
	}
	store.ProjectRepos[projectId] = repo
	if persist {
		err = s.SaveStore(ctx, projectId, "", s)
	}
	return
}

func (store *Store) SaveRepoToken(ctx context.Context, projectId string, repoToken repos.RepoToken, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("SaveRepoToken - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.ProjectRepoTokens == nil {
		store.ProjectRepoTokens = make(map[string]repos.RepoToken)
	}
	var prjRepoToken repos.RepoToken
	_ = prjRepoToken
	ok := false
	if prjRepoToken, ok = store.ProjectRepoTokens[projectId]; !ok {
		logs.WithContext(ctx).Info(fmt.Sprint("making new repo token object for project : ", projectId))
		store.ProjectRepoTokens[projectId] = repos.RepoToken{}
	}
	store.ProjectRepoTokens[projectId] = repoToken
	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) FetchSm(ctx context.Context, projectId string) (smObj sm.SmStoreI, err error) {
	logs.WithContext(ctx).Debug("FetchSm - Start")
	if store.SecretManager == nil {
		err = errors.New("No secret manager defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	ok := false
	if smObj, ok = store.SecretManager[projectId]; !ok {
		err = errors.New(fmt.Sprint("Secret Manager not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return
}

func (store *Store) LoadEnvValue(ctx context.Context, projectId string) (err error) {
	logs.WithContext(ctx).Info("LoadEnvValue - Start")
	if store.Variables != nil {
		for prjId, _ := range store.Variables {
			if projectId == prjId || projectId == "" {
				if _, prjVarsOk := store.Variables[prjId]; prjVarsOk {
					if store.Variables[prjId].EnvVars != nil {
						for k, v := range store.Variables[prjId].EnvVars {
							envValue := os.Getenv(k)
							if envValue != "" {
								v.Value = envValue
								store.Variables[prjId].EnvVars[k] = v
							} else {
								logs.WithContext(ctx).Warn(fmt.Sprint("no environment value found for ", k))
							}
						}
					} else {
						err = errors.New(fmt.Sprint("environment variables not defined for project : ", prjId))
						return
					}
				}
			}
		}
	}
	return
}
func (store *Store) LoadSmValue(ctx context.Context, projectId string) (err error) {
	logs.WithContext(ctx).Info("LoadSmValue - Start")
	smFound := true
	var smObjI interface{}
	if store.Variables != nil {
		for prjId, _ := range store.Variables {
			if projectId == prjId || projectId == "" {
				logs.WithContext(ctx).Info(fmt.Sprint("loading secrets for :", prjId))
				smFound = true
				if _, prjVarsOk := store.Variables[prjId]; prjVarsOk {
					if store.Variables[prjId].Secrets != nil {
						if store.SecretManager == nil {
							err = errors.New("no secret manager defined in store")
							smFound = false
							logs.WithContext(ctx).Error(err.Error())
						} else if smObj, smObjOk := store.SecretManager[prjId]; !smObjOk {
							err = errors.New(fmt.Sprint("Secret Manager not defined for project :", prjId))
							smFound = false
							logs.WithContext(ctx).Error(err.Error())
						} else {
							if smObj != nil {
								result, resultErr := smObj.FetchSmValue(ctx)
								if resultErr != nil {
									smFound = false
								}
								smObjI, err = utils.CloneInterface(ctx, smObj)
								if err != nil {
									logs.WithContext(ctx).Error(err.Error())

								}

								if smFound {
									for k, v := range store.Variables[prjId].Secrets {
										if _, seretOk := result[k]; seretOk {
											v.Value = result[k]
											store.Variables[prjId].Secrets[k] = v
										} else {
											logs.WithContext(ctx).Warn(fmt.Sprint("secret manager does not have any secret value for ", k, ", trying to load from environment variables"))
											v.Value = os.Getenv(k)
											store.Variables[prjId].Secrets[k] = v
										}
									}
								}
							}
						}
					} else {
						err = errors.New(fmt.Sprint("secret not defined for project : ", prjId))
						smFound = false
						logs.WithContext(ctx).Error(err.Error())
					}
				} else {
					err = errors.New(fmt.Sprint("variables not defined for project : ", prjId))
					smFound = false
					logs.WithContext(ctx).Error(err.Error())
				}
				if !smFound {
					logs.WithContext(ctx).Warn("no secret manager found, trying to load from environment variables")
					for k, v := range store.Variables[prjId].Secrets {
						v.Value = os.Getenv(k)
						store.Variables[prjId].Secrets[k] = v
					}
				}
			}
		}
	}

	//if smObjClone, smObjCloneOk := smObjI.(sm.SmStoreI); smObjCloneOk {
	if store.TenantVariables != nil {
		for prjId, _ := range store.TenantVariables {
			if projectId == prjId || projectId == "" {

				if store.SecretManager == nil {
					err = errors.New("no secret manager defined in store")
					smFound = false
					logs.WithContext(ctx).Error(err.Error())
				} else if smObj, smObjOk := store.SecretManager[prjId]; !smObjOk {
					err = errors.New(fmt.Sprint("Secret Manager not defined for project :", prjId))
					smFound = false
					logs.WithContext(ctx).Error(err.Error())
				} else {
					if smObj != nil {
						smObjI, err = utils.CloneInterface(ctx, smObj)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())

						}
					}
				}
				if smObjClone, smObjCloneOk := smObjI.(sm.SmStoreI); smObjCloneOk {
					logs.WithContext(ctx).Info(fmt.Sprint("loading tenant secrets for :", prjId))
					for tenantId, _ := range store.TenantVariables[prjId] {
						smValues, err := smObjClone.GetSmValues(ctx, tenantId)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
						for tsk := range store.TenantVariables[prjId][tenantId].Secrets {
							if secretValue, secretOk := smValues[tsk]; secretOk {
								v := store.TenantVariables[prjId][tenantId].Secrets[tsk]
								v.Value = secretValue
								store.TenantVariables[prjId][tenantId].Secrets[tsk] = v
							}
						}
					}
				}
			}
		}
	}
	//} else {
	//	logs.WithContext(ctx).Error("failed to clone secret manager")
	//}

	return
}

func (store *Store) SetSmValue(ctx context.Context, projectId string, secretName string, secretValue map[string]string) (err error) {
	logs.WithContext(ctx).Info("SetSmValue - Start")
	if store.SecretManager == nil {
		err = errors.New("no secret manager defined in store")
		logs.WithContext(ctx).Error(err.Error())
	} else if smObj, smObjOk := store.SecretManager[projectId]; !smObjOk {
		err = errors.New(fmt.Sprint("Secret Manager not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
	} else {
		if smObj != nil {
			smObjClone, smObjCloneErr := utils.CloneInterface(ctx, smObj)
			if smObjCloneErr != nil {
				return
			}
			err = smObjClone.(sm.SmStoreI).SetSmValue(ctx, secretName, secretValue)
			if err != nil {
				return
			}
		}
	}
	return
}

func (store *Store) UnsetSmValue(ctx context.Context, projectId string, secretName string, secretKey string) (err error) {
	logs.WithContext(ctx).Info("UnsetSmValue - Start")
	if store.SecretManager == nil {
		err = errors.New("no secret manager defined in store")
		logs.WithContext(ctx).Error(err.Error())
	} else if smObj, smObjOk := store.SecretManager[projectId]; !smObjOk {
		err = errors.New(fmt.Sprint("Secret Manager not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
	} else {
		if smObj != nil {
			smObjClone, smObjCloneErr := utils.CloneInterface(ctx, smObj)
			if smObjCloneErr != nil {
				return
			}
			err = smObjClone.(sm.SmStoreI).UnsetSmValue(ctx, secretName, secretKey)
			if err != nil {
				return
			}

		}
	}
	return
}
func (store *Store) GetSmValue(ctx context.Context, projectId string, secretName string, secretKey string, force_delete bool) (secret_value interface{}, err error) {
	logs.WithContext(ctx).Info("GetSmValue - Start")
	if store.SecretManager == nil {
		err = errors.New("No secret manager defined in store")
		logs.WithContext(ctx).Error(err.Error())
	} else if smObj, smObjOk := store.SecretManager[projectId]; !smObjOk {
		err = errors.New(fmt.Sprint("Secret Manager not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
	} else {
		if smObj != nil {
			//smObjClone, smObjCloneErr := utils.CloneInterface(ctx, smObj)
			//if smObjCloneErr != nil {
			//	return
			//}
			//smCacheObjClone, smCacheObjCloneErr := utils.CloneInterface(ctx, smObj.GetCacheStore())
			//if smCacheObjCloneErr != nil {
			//	return
			//}
			//smObjClone.(sm.SmStoreI).SetCacheStore(smCacheObjClone.(cache.CacheStoreI))
			//logs.WithContext(ctx).Info(fmt.Sprint(smObjClone.(sm.SmStoreI).GetCacheStore()))
			secret_value, err = smObj.GetSmValue(ctx, projectId, secretName, secretKey, force_delete)
			if err != nil {
				return
			}
		}
	}
	return
}

func (store *Store) GetProjectSecret(ctx context.Context, projectId string, key string) (value string, err error) {
	logs.WithContext(ctx).Debug("GetProjectSecret - Start")
	if store.SecretManager == nil {
		err = errors.New("no secret manager defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	smObj, ok := store.SecretManager[projectId]
	if !ok || smObj == nil {
		err = fmt.Errorf("Secret Manager not defined for project : %s", projectId)
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	v, err := store.GetSmValue(ctx, projectId, smObj.GetSecretName(), key, false)
	if err != nil {
		return
	}
	if v == nil {
		return "", nil
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	err = fmt.Errorf("secret value for key %s is not a string", key)
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (store *Store) SaveSm(ctx context.Context, projectId string, secretManager sm.SmStoreI, s StoreI, persist bool) (err error) {
	logs.WithContext(ctx).Debug("SaveSm - Start")
	if persist {
		s.GetMutex().Lock()
		defer s.GetMutex().Unlock()
	}
	if store.SecretManager == nil {
		store.SecretManager = make(map[string]sm.SmStoreI)
	}
	store.SecretManager[projectId] = secretManager
	if persist {
		err = s.SaveStore(ctx, projectId, "", s)
	}
	return
}

func (store *Store) GetProjectConfigForRepo(ctx context.Context, projectId string, ms StoreI) (repoData map[string]map[string]interface{}, accessToken string, err error) {
	logs.WithContext(ctx).Debug("GetProjectConfigForRepo - Start")
	repoData = make(map[string]map[string]interface{})
	repoInnerData := make(map[string]interface{})
	repoData[projectId] = repoInnerData
	storeBytes, err := json.Marshal(ms)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, "", err
	}
	storeMap := make(map[string]interface{})
	err = json.Unmarshal(storeBytes, &storeMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, "", err
	}
	td := 0.0
	authMode := ""
	at := ""
	for k, v := range storeMap {
		if k == "projects" {
			if prjMap, prjMapOk := v.(map[string]interface{}); prjMapOk {
				if prj, ok := prjMap[projectId]; ok {
					repoInnerData["config"] = prj
				} else {
					return nil, "", errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
				}
			} else {
				logs.WithContext(ctx).Info("map failed")
			}
		} else if k == "variables" {
			if VarsMap, VarsMapOk := v.(map[string]interface{}); VarsMapOk {
				if vars, ok := VarsMap[projectId]; ok {
					repoInnerData["variables"] = vars
				}
			}
		} else if k == "repos" {
			if ReposMap, ReposMapOk := v.(map[string]interface{}); ReposMapOk {
				if repo, ok := ReposMap[projectId]; ok {
					repoBytes, repoBytesErr := json.Marshal(repo)
					if repoBytesErr != nil {
						return nil, "", repoBytesErr
					}
					repoStruct := repos.Repo{}
					repoMapErr := json.Unmarshal(repoBytes, &repoStruct)
					if repoMapErr != nil {
						return nil, "", repoMapErr
					}
					repoInnerData["repo"] = repoStruct
					authMode = repoStruct.AuthMode
				}
			}
		} else if k == "repo_token" {
			if RepoTokenMap, RepoTokenMapOk := v.(map[string]interface{}); RepoTokenMapOk {
				if repoToken, ok := RepoTokenMap[projectId]; ok {
					repoTokenBytes, repoTokenBytesErr := json.Marshal(repoToken)
					if repoTokenBytesErr != nil {
						return nil, "", repoTokenBytesErr
					}
					repoTokenStruct := repos.RepoToken{}
					repoMapErr := json.Unmarshal(repoTokenBytes, &repoTokenStruct)
					if repoMapErr != nil {
						return nil, "", repoMapErr
					}

					t, e := time.Parse("2006-01-02T15:04:05Z", repoTokenStruct.RepoTokenExpiry)
					if e != nil {
						logs.WithContext(ctx).Error(e.Error())
						return nil, "", e
					}
					td = t.Sub(time.Now()).Seconds()
					at = repoTokenStruct.RepoToken
				}
			}
		}
	}
	if authMode == "GITHUBAPP" {
		if td > 0 {
			accessToken = at
		} else {
			err = errors.New("token expired")
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	}
	return
}

func (store *Store) SetStoreFromBytes(ctx context.Context, storeBytes []byte, msi StoreI) (err error) {
	logs.WithContext(ctx).Debug("SetStoreFromBytes - Start")
	msi.GetMutex().Lock()
	defer msi.GetMutex().Unlock()

	var storeMap map[string]*json.RawMessage
	err = json.Unmarshal(storeBytes, &storeMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	var prjEvents map[string]*json.RawMessage
	if _, ok := storeMap["events"]; ok {
		if storeMap["events"] != nil {
			logs.WithContext(ctx).Info("inside event loop")
			err = json.Unmarshal(*storeMap["events"], &prjEvents)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for prj, eventJson := range prjEvents {
				if eventJson != nil {
					var eventObj map[string]*json.RawMessage
					err = json.Unmarshal(*eventJson, &eventObj)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					for _, eJson := range eventObj {
						var eObj map[string]*json.RawMessage
						err = json.Unmarshal(*eJson, &eObj)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
						var eventType string
						if _, stOk := eObj["event_type"]; stOk {
							err = json.Unmarshal(*eObj["event_type"], &eventType)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							eventI := events.GetEvent(eventType)
							if eventI != nil {
								err = eventI.MakeFromJson(ctx, eJson)
								if err == nil {
									err = msi.SaveEvent(ctx, prj, eventI, msi, false)
									if err != nil {
										return err
									}
								} else {
									return err
								}
							}
						} else {
							logs.WithContext(ctx).Info("ignoring event as event_type attribute not found")
						}
					}
				}
			}
		} else {
			logs.WithContext(ctx).Info("event attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("event attribute not found in store")
	}

	var prjKms map[string]*json.RawMessage
	if _, ok := storeMap["kms"]; ok {
		if storeMap["kms"] != nil {
			logs.WithContext(ctx).Info("inside kms loop")
			err = json.Unmarshal(*storeMap["kms"], &prjKms)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for prj, kmsJson := range prjKms {
				if kmsJson != nil {
					var kmsObj map[string]*json.RawMessage
					err = json.Unmarshal(*kmsJson, &kmsObj)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					for _, kJson := range kmsObj {
						var kObj map[string]*json.RawMessage
						err = json.Unmarshal(*kJson, &kObj)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
						var kmsType string
						if _, stOk := kObj["kms_store_type"]; stOk {
							err = json.Unmarshal(*kObj["kms_store_type"], &kmsType)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							kmsI := kms.GetKms(kmsType)
							if kmsI != nil {
								err = kmsI.MakeFromJson(ctx, kJson)
								if err == nil {
									err = msi.SaveKms(ctx, prj, kmsI, msi, false)
									if err != nil {
										return err
									}
								} else {
									return err
								}
							}
						} else {
							logs.WithContext(ctx).Info("ignoring kms as kms_store_type attribute not found")
						}
					}
				}
			}
		} else {
			logs.WithContext(ctx).Info("kms attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("kms attribute not found in store")
	}

	var prjScheduler map[string]*json.RawMessage
	if _, ok := storeMap["scheduler"]; ok {
		if storeMap["scheduler"] != nil {
			err = json.Unmarshal(*storeMap["scheduler"], &prjScheduler)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for prj, schedulerJson := range prjScheduler {
				var schedulerObj map[string]*json.RawMessage
				err = json.Unmarshal(*schedulerJson, &schedulerObj)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				var schedulerType string
				if _, stOk := schedulerObj["scheduler_type"]; stOk {
					err = json.Unmarshal(*schedulerObj["scheduler_type"], &schedulerType)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					schedulerI := scheduler.GetScheduler(schedulerType)
					err = schedulerI.MakeFromJson(ctx, schedulerJson)
					if err == nil {
						err = msi.SaveScheduler(ctx, prj, schedulerI, msi, false)
						if err != nil {
							return err
						}
					} else {
						return err
					}
				} else {
					logs.WithContext(ctx).Info("ignoring scheduler as scheduler_type attribute not found")
				}
			}
		} else {
			logs.WithContext(ctx).Info("scheduler attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("scheduler attribute not found in store")
	}

	var prjSm map[string]*json.RawMessage
	if _, ok := storeMap["secret_manager"]; ok {
		if storeMap["secret_manager"] != nil {
			err = json.Unmarshal(*storeMap["secret_manager"], &prjSm)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for prj, smJson := range prjSm {
				if smJson != nil {
					var smObj map[string]*json.RawMessage
					err = json.Unmarshal(*smJson, &smObj)
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
						if smI != nil {
							err = smI.MakeFromJson(ctx, smJson)
							if err == nil {
								logs.WithContext(ctx).Info(fmt.Sprint("________________________ inii cache called while lodaing for ", prj))
								err = smI.InitCache(ctx, prj)
								err = msi.SaveSm(ctx, prj, smI, msi, false)
								if err != nil {
									return err
								}
							} else {
								return err
							}
						}
					} else {
						logs.WithContext(ctx).Info("ignoring secret manager as sm_store_type attribute not found")
					}
				}
			}
		} else {
			logs.WithContext(ctx).Info("secret manager attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("secret manager attribute not found in store")
	}

	var prjRepo map[string]*json.RawMessage
	if _, ok := storeMap["repos"]; ok {
		if storeMap["repos"] != nil {
			err = json.Unmarshal(*storeMap["repos"], &prjRepo)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for prj, repoJson := range prjRepo {
				if repoJson != nil {
					var repoObj map[string]*json.RawMessage
					err = json.Unmarshal(*repoJson, &repoObj)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					var repoType string
					if _, rtOk := repoObj["repo_type"]; rtOk {
						err = json.Unmarshal(*repoObj["repo_type"], &repoType)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
						repoI := repos.GetRepo(repoType)
						err = repoI.MakeFromJson(ctx, repoJson)
						if err == nil {
							err = msi.SaveRepo(ctx, prj, repoI, msi, false)
							if err != nil {
								return err
							}
						} else {
							return err
						}
					} else {
						logs.WithContext(ctx).Info("ignoring repo as repo type not found")
					}
				} else {
					logs.WithContext(ctx).Info(fmt.Sprint("repo not defined for projct ", prj))
				}
			}
		} else {
			logs.WithContext(ctx).Info("repos attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("repos attribute not found in store")
	}
	logs.WithContext(ctx).Error("SetStoreFromBytes before return")
	return
}
func (store *Store) FetchKms(ctx context.Context, projectId string) (kms map[string]kms.KmsStoreI, err error) {
	logs.WithContext(ctx).Debug("FetchKms - Start")
	if store.KMS == nil {
		err = errors.New("no kms defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	ok := false
	if kms, ok = store.KMS[projectId]; !ok {
		err = errors.New(fmt.Sprint("kms not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return
}
func (store *Store) SaveKms(ctx context.Context, projectId string, k kms.KmsStoreI, s StoreI, persist bool) (err error) {
	logs.WithContext(ctx).Debug("SaveKms - Start")
	if persist {
		s.GetMutex().Lock()
		defer s.GetMutex().Unlock()
	}
	if store.KMS == nil {
		store.KMS = make(map[string]map[string]kms.KmsStoreI)
	}
	if store.KMS[projectId] == nil {
		store.KMS[projectId] = make(map[string]kms.KmsStoreI)
	}
	if persist {
		err = k.CreateKey(ctx)
		if err != nil {
			return
		}
	}
	kName, err := k.GetAttribute(ctx, "kms_name")
	if err != nil {
		return
	}
	store.KMS[projectId][kName.(string)] = k

	if persist {
		err = s.SaveStore(ctx, projectId, "", s)
	}
	return
}

func (store *Store) RemoveKms(ctx context.Context, projectId string, kmsName string, cloudDelete bool, deleteDays int32, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveKms - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.KMS == nil {
		err = errors.New("kms not defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if store.KMS[projectId] == nil {
		err = errors.New(fmt.Sprint("kms not defined for project ", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if store.KMS[projectId][kmsName] == nil {
		err = errors.New(fmt.Sprint("key not found"))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if cloudDelete {
		err = store.KMS[projectId][kmsName].DeleteKey(ctx, kmsName, deleteDays)
		if err != nil {
			return
		}
	}
	delete(store.KMS[projectId], kmsName)

	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) FetchEvent(ctx context.Context, projectId string, eventName string, s StoreI) (eventClone events.EventI, err error) {
	logs.WithContext(ctx).Debug("FetchEvent - Start")
	if store.Events == nil {
		err = errors.New("no event defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	if eventMap, ok := store.Events[projectId]; !ok {
		err = errors.New(fmt.Sprint("event not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	} else {
		ok = false
		if event, ok := eventMap[eventName]; !ok {
			err = errors.New(fmt.Sprint("event ", eventName, " not found for project :", projectId))
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		} else {
			eventClone, err = s.CloneEvent(ctx, projectId, event, s)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, err
			}
		}
	}
	return
}

func (store *Store) FetchEvents(ctx context.Context, projectId string) (event map[string]events.EventI, err error) {
	logs.WithContext(ctx).Debug("FetchEvents - Start")
	if store.Events == nil {
		err = errors.New("no event defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	ok := false
	if event, ok = store.Events[projectId]; !ok {
		err = errors.New(fmt.Sprint("event not defined for project :", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return
}
func (store *Store) CloneEvent(ctx context.Context, projectId string, event events.EventI, s StoreI) (eClone events.EventI, err error) {
	logs.WithContext(ctx).Debug("CloneEvent - Start")
	eventJson, err := json.Marshal(event)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	eCloneBytes := s.ReplaceVariables(ctx, projectId, eventJson, nil)
	eventType, err := event.GetAttribute("event_type")
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	eClone = events.GetEvent(eventType.(string))
	if eClone == nil {
		err = errors.New("unknown event_type: " + eventType.(string))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	err = json.Unmarshal(eCloneBytes, eClone)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}
func (store *Store) SaveEvent(ctx context.Context, projectId string, e events.EventI, s StoreI, persist bool) (err error) {
	logs.WithContext(ctx).Debug("SaveEvent - Start")
	if persist {
		s.GetMutex().Lock()
		defer s.GetMutex().Unlock()
	}
	if store.Events == nil {
		store.Events = make(map[string]map[string]events.EventI)
	}
	if store.Events[projectId] == nil {
		store.Events[projectId] = make(map[string]events.EventI)
	}
	if persist {
		eClone, cloneErr := s.CloneEvent(ctx, projectId, e, store)
		if cloneErr != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		err = eClone.CreateEvent(ctx)
		if err != nil {
			return
		}
	}

	eName, err := e.GetAttribute("event_name")
	if err != nil {
		return
	}
	store.Events[projectId][eName.(string)] = e

	if persist {
		err = s.SaveStore(ctx, projectId, "", s)
	}
	return
}

func (store *Store) RemoveEvent(ctx context.Context, projectId string, eventName string, cloudDelete bool, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveEvent - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Events == nil {
		err = errors.New("event not defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if store.Events[projectId] == nil {
		err = errors.New(fmt.Sprint("event not defined for project ", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if store.Events[projectId][eventName] == nil {
		err = errors.New(fmt.Sprint("event ", eventName, " not found"))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if cloudDelete {
		eClone, cloneErr := s.CloneEvent(ctx, projectId, store.Events[projectId][eventName], store)
		if cloneErr != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		err = eClone.DeleteEvent(ctx)
		if err != nil {
			return
		}
	}
	delete(store.Events[projectId], eventName)

	err = s.SaveStore(ctx, projectId, "", s)
	return
}

func (store *Store) GetCacheValue(ctx context.Context, projectId string, key string) (value interface{}, err error) {
	if store.CacheStore != nil {
		if pcv, pcvOk := store.CacheStore[projectId]; pcvOk {
			return pcv.Get(ctx, key)
		} else {
			err = errors.New(fmt.Sprint("cache store not found for project ", projectId))
			return
		}
	} else {
		err = errors.New("cache store is not defined")
		return
	}
}
func (store *Store) PollEvent(ctx context.Context, projectId string, eventName string, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("PollEvent - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Events == nil {
		err = errors.New("event not defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if store.Events[projectId] == nil {
		err = errors.New(fmt.Sprint("event not defined for project ", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if store.Events[projectId][eventName] == nil {
		err = errors.New(fmt.Sprint("event not found"))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	_, err = store.Events[projectId][eventName].Poll(ctx)
	return
}

func (store *Store) PublishEvent(ctx context.Context, projectId string, eventName string, msg interface{}, s StoreI) (msgId string, err error) {
	logs.WithContext(ctx).Debug("PublishEvent - Start")
	s.GetMutex().Lock()
	defer s.GetMutex().Unlock()
	if store.Events == nil {
		err = errors.New("event not defined in store")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if store.Events[projectId] == nil {
		err = errors.New(fmt.Sprint("event not defined for project ", projectId))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if store.Events[projectId][eventName] == nil {
		err = errors.New(fmt.Sprint("event not found"))
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	msgId, err = store.Events[projectId][eventName].Publish(ctx, msg, store.Events[projectId][eventName])
	return
}
func (store *Store) SetCacheValue(ctx context.Context, projectId string, key string, value interface{}) (err error) {
	if store.CacheStore != nil {
		if pcv, pcvOk := store.CacheStore[projectId]; pcvOk {
			return pcv.Set(ctx, key, value)
		} else {
			err = errors.New(fmt.Sprint("cache store not found for project ", projectId))
			return
		}
	} else {
		err = errors.New("cache store is not defined")
		return
	}
}

func (store *Store) ValidateJSON(ctx context.Context, schema validator.Schema, data []interface{}) (records []interface{}, errRecords []interface{}) {
	var jsonData gjson.Result
	dataBytes, err := json.Marshal(data)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	jsonData = gjson.ParseBytes(dataBytes)
	return schema.Validate(ctx, jsonData)
}
func (store *Store) GetStoreWithoutTenants(ctx context.Context, ms StoreI) (b []byte, err error) {
	logs.WithContext(ctx).Debug("GetStoreByteArrayWithoutTenants - Start")
	logs.WithContext(ctx).Info("calling default get store byte array without tenants")
	b, err = json.Marshal(ms)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (store *Store) SaveScheduler(ctx context.Context, projectId string, schedulerObj scheduler.SchedulerI, s StoreI, persist bool) (err error) {
	logs.WithContext(ctx).Debug("SaveScheduler - Start")
	if persist {
		s.GetMutex().Lock()
		defer s.GetMutex().Unlock()
	}
	if store.Scheduler == nil {
		store.Scheduler = make(map[string]scheduler.SchedulerI)
	}
	store.Scheduler[projectId] = schedulerObj
	if persist {
		/* err = store.InitScheduler(ctx, s)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		} */
		err = s.SaveStore(ctx, projectId, "", s)
	}
	return
}
func (store *Store) FetchScheduler(ctx context.Context, projectId string) (schedulerObj scheduler.SchedulerI, err error) {
	logs.WithContext(ctx).Debug("FetchScheduler - Start")
	if store.Scheduler == nil {
		err = errors.New("no scheduler defined in store")
		logs.Err(ctx, err, "no scheduler defined in store")
		return nil, err
	}
	ok := false
	if schedulerObj, ok = store.Scheduler[projectId]; !ok {
		err = errors.New(fmt.Sprint("scheduler not defined for project :", projectId))
		logs.Err(ctx, err, "scheduler not defined for project")
		return nil, err
	}
	return
}

func (store *Store) InitScheduler(ctx context.Context, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("InitScheduler - Start")
	for projectId, sch := range store.Scheduler {
		schJson, err := json.Marshal(sch)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}

		schJsonStr := store.ReplaceVariables(ctx, projectId, schJson, nil)
		rawMsg := json.RawMessage(schJsonStr)
		err = sch.Init(ctx, &rawMsg)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			//return err
		}
	}
	return nil
}
func (store *Store) SaveRequest(ctx context.Context, sampleRequest models.SampleRequest, projectId string, tenantId string, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("SaveRequest - Start")
	sampleRequestBytes, err := json.Marshal(sampleRequest.RequestBody)
	if err != nil {
		err = logs.Err(ctx, err, "error marshalling sample request")
		return
	}
	var saveQueries []*models.Queries
	saveQueryFuncRequest := models.Queries{}
	saveQueryFuncRequest.Query = db.GetDb(s.GetDbType()).GetDbQuery(ctx, SAVE_REQUEST)
	saveQueryFuncRequest.Vals = append(saveQueryFuncRequest.Vals, sampleRequest.RequestId, sampleRequest.RequestName, sampleRequest.ResourceName, projectId, tenantId, (string)(sampleRequestBytes))
	saveQueryFuncRequest.Rank = 1
	saveQueries = append(saveQueries, &saveQueryFuncRequest)
	_, err = utils.ExecuteDbSave(ctx, s.GetConn(), saveQueries)
	if err != nil {
		return
	}
	return
}

func (store *Store) RemoveRequest(ctx context.Context, requestId string, s StoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveRequest - Start")
	var deleteQueries []*models.Queries
	deleteQueryFuncRequest := models.Queries{}
	deleteQueryFuncRequest.Query = db.GetDb(s.GetDbType()).GetDbQuery(ctx, DELETE_REQUEST)
	deleteQueryFuncRequest.Vals = append(deleteQueryFuncRequest.Vals, requestId)
	deleteQueryFuncRequest.Rank = 1
	deleteQueries = append(deleteQueries, &deleteQueryFuncRequest)
	var delResult [][]map[string]interface{}
	delResult, err = utils.ExecuteDbSave(ctx, s.GetConn(), deleteQueries)
	if err != nil {
		return
	}
	if len(delResult[0]) == 0 {
		err = logs.Err(ctx, fmt.Errorf("func request not found %s", requestId), "")
		return
	}
	return
}

func (store *Store) GetRequests(ctx context.Context, projectId string, tenantId string, resourceName string, s StoreI) (requests []models.SampleRequest, err error) {
	logs.WithContext(ctx).Debug("GetRequests - Start")
	selectQueryFuncRequest := models.Queries{}
	selectQueryFuncRequest.Query = db.GetDb(s.GetDbType()).GetDbQuery(ctx, SELECT_REQUEST)
	selectQueryFuncRequest.Vals = append(selectQueryFuncRequest.Vals, projectId, tenantId, resourceName, "ALL", "ALL")
	selectQueryFuncRequest.Rank = 1
	output, err := utils.ExecuteDbFetch(ctx, s.GetConn(), selectQueryFuncRequest)
	if err != nil {
		return
	}
	requests = []models.SampleRequest{}
	for _, request := range output {
		requests = append(requests, models.SampleRequest{
			RequestId:    utils.GetStringField(request, "request_id"),
			RequestName:  utils.GetStringField(request, "request_name"),
			RequestBody:  utils.GetMapField(request, "request_json"),
			ResourceName: utils.GetStringField(request, "resource_name"),
		})
	}
	return
}
func (store *Store) SetServiceName(serviceName string) {
	ServiceName = serviceName
}
func (store *Store) SetInstanceId(instanceId string) {
	InstanceId = instanceId
}
func (store *Store) SetBaseUrl(baseUrl string) {
	BaseUrl = baseUrl
}
