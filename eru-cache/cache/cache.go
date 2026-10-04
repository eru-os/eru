package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	DEFAULT_CACHE_TABLE_NAME = "eru_cache"
)

type CacheTableColumn struct {
	TblSchema         string `json:"tbl_schema" eru:"required"`
	TblName           string `json:"tbl_name" eru:"required"`
	ColName           string `json:"col_name" eru:"required"`
	OwnDataType       string `json:"own_data_type" eru:"required"`
	PrimaryKey        bool   `json:"primary_key" eru:"required"`
	IsNullable        bool   `json:"is_nullable" eru:"required"`
	DefaultValue      string `json:"default_value"`
	AutoIncrement     bool   `json:"auto_increment"`
	CharMaxLength     int    `json:"char_max_length"`
	NumericPrecision  string `json:"numeric_precision"`
	NumericScale      int    `json:"numeric_scale"`
	DatetimePrecision int    `json:"datetime_precision"`
}

var ExpectedCacheTableSchema = map[string]CacheTableColumn{
	"cache_sk":      {TblName: "eru_cache", ColName: "cache_sk", OwnDataType: "integer", IsNullable: false, PrimaryKey: true, AutoIncrement: true},
	"project_id":    {TblName: "eru_cache", ColName: "project_id", OwnDataType: "varchar", IsNullable: false, PrimaryKey: false, CharMaxLength: 100},
	"tenant_id":     {TblName: "eru_cache", ColName: "tenant_id", OwnDataType: "varchar", IsNullable: false, PrimaryKey: false, CharMaxLength: 100},
	"cache_key":     {TblName: "eru_cache", ColName: "cache_key", OwnDataType: "varchar", IsNullable: false, PrimaryKey: false, CharMaxLength: 500},
	"cache_value":   {TblName: "eru_cache", ColName: "cache_value", OwnDataType: "string", IsNullable: false, PrimaryKey: false},
	"created_at":    {TblName: "eru_cache", ColName: "created_at", OwnDataType: "datetime", IsNullable: false, PrimaryKey: false, DefaultValue: "CURRENT_TIMESTAMP"},
	"updated_at":    {TblName: "eru_cache", ColName: "updated_at", OwnDataType: "datetime", IsNullable: false, PrimaryKey: false, DefaultValue: "CURRENT_TIMESTAMP"},
	"expires_at":    {TblName: "eru_cache", ColName: "expires_at", OwnDataType: "datetime", IsNullable: true, PrimaryKey: false},
	"access_count":  {TblName: "eru_cache", ColName: "access_count", OwnDataType: "biginteger", IsNullable: false, PrimaryKey: false, DefaultValue: "0"},
	"last_accessed": {TblName: "eru_cache", ColName: "last_accessed", OwnDataType: "datetime", IsNullable: false, PrimaryKey: false, DefaultValue: "CURRENT_TIMESTAMP"},
	"created_by":    {TblName: "eru_cache", ColName: "created_by", OwnDataType: "varchar", IsNullable: false, PrimaryKey: false, CharMaxLength: 100},
	"agent_name":    {TblName: "eru_cache", ColName: "agent_name", OwnDataType: "varchar", IsNullable: false, PrimaryKey: false, CharMaxLength: 100},
}

type CacheData struct {
	CacheKey     string    `json:"cache_key"`
	CacheValue   string    `json:"cache_value"`
	ProjectId    string    `json:"project_id"`
	TenantId     string    `json:"tenant_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	AccessCount  int       `json:"access_count"`
	LastAccessed time.Time `json:"last_accessed"`
	CreatedBy    string    `json:"created_by"`
	AgentName    string    `json:"agent_name"`
	// LastUpdated is when the conversation this row belongs to was last
	// touched. The list query has always computed it and the struct had nowhere
	// to put it, so it was dropped on unmarshal and every conversation reached
	// the UI with no time at all.
	//
	// It is read-only: eru_cache has no such column, the list query derives it
	// as max(updated_at). It has to be a pointer so a zero value is genuinely
	// omitted from the insert - omitempty does not omit a zero time.Time, and a
	// struct value here put last_updated in the column list of every write,
	// which postgres rejected and the sync goroutine only logged. No
	// conversation was persisted at all while that stood.
	LastUpdated *time.Time `json:"last_updated,omitempty"`
}

// CacheStoreI defines the interface for a generic cache.
type CacheStoreI interface {
	Init(ctx context.Context) error
	Get(ctx context.Context, key string) (value string, err error)
	Set(ctx context.Context, key string, value interface{}) (err error)
	SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) (err error)
	SetWithTagsTTL(ctx context.Context, key string, value interface{}, ttl time.Duration, tags []string) (err error)
	InvalidateByTags(ctx context.Context, tags []string) (deleted int, err error)
	AcquireLock(ctx context.Context, key string, ttl time.Duration, owner string) (acquired bool, err error)
	ReleaseLock(ctx context.Context, key string, owner string) (err error)
	GetKeys(ctx context.Context, pattern string) ([]string, error)
	Delete(ctx context.Context, key string) error
	ValidatePersistence(ctx context.Context, projectId string) error
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error)
	SyncPersistence(ctx context.Context, cacheStoreI CacheStoreI) error
	SyncToDatabase(ctx context.Context, projectId string, cacheData []CacheData) error
	LoadFromDatabase(ctx context.Context, projectId, tenantId, cacheKey string, agentName string, createdBy string) ([]CacheData, error)
	LoadListFromDatabase(ctx context.Context, projectId, tenantId, cacheKey string, agentName string, createdBy string, limit int, skip int) (cacheData []CacheData, err error)
}

// CacheStore is a base struct to be embedded by specific implementations.
//
// A configured store is two tiers, not one. What is held in the store itself -
// process memory, or Redis - is the short-term tier: fast, shared with whoever
// else is on that store, and allowed to expire. What PersistEnabled and
// CacheDbAlias send on to the database is the long-term tier: slower, smaller,
// and kept. A caller chooses a tier by choosing whether to hand a row to
// SyncToDatabase, not by reaching for a second store.
type CacheStore struct {
	CacheStoreType string `json:"cache_store_type"`
	CacheDbAlias   string `json:"cache_db_alias"`
	PersistEnabled bool   `json:"persist_enabled"`
	PersistError   bool   `json:"persist_error"`

	// SessionTtl is how long an entry that is never persisted may live in the
	// short-term tier - a Go duration such as "2h". Entries written without a
	// ttl are unaffected. Empty means DefaultSessionTtl.
	SessionTtl string `json:"session_ttl,omitempty"`
}

// DefaultSessionTtl is how long short-term entries live when the store's
// configuration does not say.
const DefaultSessionTtl = 2 * time.Hour

// SessionTtlOrDefault is the configured short-term lifetime.
func (cs *CacheStore) SessionTtlOrDefault() time.Duration {
	if cs.SessionTtl == "" {
		return DefaultSessionTtl
	}
	d, err := time.ParseDuration(cs.SessionTtl)
	if err != nil || d <= 0 {
		return DefaultSessionTtl
	}
	return d
}

func (cs *CacheStore) Init(ctx context.Context) error {
	return nil
}

func (cs *CacheStore) Delete(ctx context.Context, key string) error {
	return nil
}
func (cs *CacheStore) Get(ctx context.Context, key string) (value string, err error) {
	return "", nil
}
func (cs *CacheStore) Set(ctx context.Context, key string, value interface{}) (err error) {
	return nil
}
func (cs *CacheStore) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) (err error) {
	return nil
}
func (cs *CacheStore) SetWithTagsTTL(ctx context.Context, key string, value interface{}, ttl time.Duration, tags []string) (err error) {
	return nil
}
func (cs *CacheStore) InvalidateByTags(ctx context.Context, tags []string) (int, error) {
	return 0, nil
}
func (cs *CacheStore) AcquireLock(ctx context.Context, key string, ttl time.Duration, owner string) (bool, error) {
	return false, nil
}
func (cs *CacheStore) ReleaseLock(ctx context.Context, key string, owner string) error {
	return nil
}
func (cs *CacheStore) GetKeys(ctx context.Context, pattern string) ([]string, error) {
	return nil, nil
}

func (cs *CacheStore) GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "cache_store_type":
		return cs.CacheStoreType, nil
	case "cache_db_alias":
		return cs.CacheDbAlias, nil
	case "persist_enabled":
		return cs.PersistEnabled, nil
	case "persist_error":
		return cs.PersistError, nil
	case "session_ttl":
		return cs.SessionTtlOrDefault(), nil
	default:
		return nil, errors.New("attribute not found")
	}
}

func (cs *CacheStore) ValidatePersistence(ctx context.Context, projectId string) error {
	logs.WithContext(ctx).Info("ValidatePersistence - Start")

	if cs.CacheDbAlias == "" {
		logs.WithContext(ctx).Info("Cache DB alias not set, persistence disabled")
		cs.PersistEnabled = false
		cs.PersistError = true
		return nil
	}
	if cs.PersistEnabled && !cs.PersistError {
		return nil
	}

	eruqlURL := os.Getenv("ERUQL_BASEURL")
	if eruqlURL == "" {
		logs.WithContext(ctx).Warn("ERUQL_BASEURL environment variable not set, persistence could not be enabled")
		cs.PersistEnabled = false
		cs.PersistError = true
		return nil
	}

	tableExists, columns, schema, err := cs.checkCacheTableExists(ctx, eruqlURL, projectId)
	if err != nil {
		cs.PersistEnabled = false
		cs.PersistError = true
		return nil
	}

	if !tableExists {
		logs.WithContext(ctx).Info("Cache table does not exist, creating it")
		err = cs.createCacheTable(ctx, eruqlURL, projectId, schema)
		if err != nil {
			cs.PersistEnabled = false
			cs.PersistError = true
			return nil
		}
		return nil
	}

	isValidSchema := cs.validateTableColumns(ctx, columns)

	if !isValidSchema {
		_ = logs.Err(ctx, fmt.Errorf("cache table schema is invalid for alias: %s", cs.CacheDbAlias), "Cache table schema is invalid")
		cs.PersistEnabled = false
		cs.PersistError = true
		return nil
	}

	cs.PersistEnabled = true
	cs.PersistError = false
	return nil
}

func (cs *CacheStore) checkCacheTableExists(ctx context.Context, eruqlURL string, projectId string) (bool, map[string]CacheTableColumn, string, error) {
	logs.WithContext(ctx).Debug("checkCacheTableExists - Start")

	url := fmt.Sprintf("%s/store/%s/datasource/tablecheck/%s/%s",
		strings.TrimSuffix(eruqlURL, "/"),
		projectId,
		cs.CacheDbAlias,
		DEFAULT_CACHE_TABLE_NAME)

	logs.WithContext(ctx).Info(fmt.Sprintf("Checking cache table existence at: %s", url))
	tableExists := false
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, nil, nil, nil, nil)
	if err != nil {
		if strings.Contains(err.Error(), "Table not found") {
			return false, nil, "", nil
		}
		err = logs.Err(ctx, err, "error calling eru-ql API")
		return false, nil, "", err
	}
	if resMap, resMapOk := res.(map[string]interface{}); !resMapOk {
		err = logs.Err(ctx, fmt.Errorf("respons is not a map"), "fail to check cache table existence")
		return false, nil, "", err
	} else {
		columns := make(map[string]CacheTableColumn)
		schema := ""
		ok := false
		if columnsI, columnsIOk := resMap["columns"]; !columnsIOk {
			_ = logs.Err(ctx, fmt.Errorf("columns not found in response"), "fail to check cache table existence")
		} else {
			if columnsI != nil {
				columnsIBytes, err := json.Marshal(columnsI)
				if err != nil {
					_ = logs.Err(ctx, err, "fail to marshal columns")
				} else {
					err = json.Unmarshal(columnsIBytes, &columns)
					if err != nil {
						_ = logs.Err(ctx, err, "fail to unmarshal columns")
					} else {
						tableExists = true
					}
				}
			}
		}
		if schI, schIOk := resMap["schema"]; !schIOk {
			_ = logs.Err(ctx, fmt.Errorf("schema not found in response"), "fail to check cache table existence")
		} else {
			if schema, ok = schI.(string); !ok {
				_ = logs.Err(ctx, fmt.Errorf("schema is not a string"), "fail to check cache table existence")
			}
		}
		return tableExists, columns, schema, nil
	}
}

func (cs *CacheStore) validateTableColumns(ctx context.Context, actualColumns map[string]CacheTableColumn) bool {
	logs.WithContext(ctx).Debug("validateTableColumns - Start")

	var expectedCols []string
	var actualCols []string
	for colName := range ExpectedCacheTableSchema {
		expectedCols = append(expectedCols, colName)
	}

	for actualColName, actualCol := range actualColumns {
		expectedCol, exists := ExpectedCacheTableSchema[strings.ToLower(actualColName)]
		if !exists {
			_ = logs.Err(ctx, fmt.Errorf("unexpected column found: %s", actualColName), "Unexpected column found")
			return false
		}
		actualCols = append(actualCols, actualColName)

		if !strings.EqualFold(strings.ToLower(expectedCol.OwnDataType), strings.ToLower(actualCol.OwnDataType)) {
			_ = logs.Err(ctx, fmt.Errorf("column %s has incompatible data type. Expected: %s, Found: %s",
				actualColName, expectedCol.OwnDataType, actualCol.OwnDataType), "Column has incompatible data type")
			return false
		}

		if expectedCol.IsNullable != actualCol.IsNullable {
			_ = logs.Err(ctx, fmt.Errorf("column %s has incompatible nullable setting. Expected: %v, Found: %v",
				actualColName, expectedCol.IsNullable, actualCol.IsNullable), "Column has incompatible nullable setting")
			return false
		}
	}

	if len(expectedCols) != len(actualCols) {
		var missingCols []string
		for _, colName := range expectedCols {
			if !slices.Contains(actualCols, colName) {
				missingCols = append(missingCols, colName)
			}
		}
		_ = logs.Err(ctx, fmt.Errorf("missing required columns: %v", missingCols), "Missing required columns")
		return false
	}
	return true
}

func (cs *CacheStore) createCacheTable(ctx context.Context, eruqlURL string, projectId string, schema string) error {
	logs.WithContext(ctx).Debug("createCacheTable - Start")

	url := fmt.Sprintf("%s/store/%s/datasource/schema/%s/savetable/%s/true",
		strings.TrimSuffix(eruqlURL, "/"),
		projectId,
		cs.CacheDbAlias,
		DEFAULT_CACHE_TABLE_NAME)

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	tableObj := make(map[string]CacheTableColumn)
	for colName, col := range ExpectedCacheTableSchema {
		col.TblSchema = schema
		tableObj[colName] = col
	}
	tableObjBytes, _ := json.Marshal(tableObj)
	logs.WithContext(ctx).Info(fmt.Sprintf("tableObjBytes: %s", string(tableObjBytes)))
	logs.WithContext(ctx).Info(fmt.Sprintf("ExpectedCacheTableSchema: %v", ExpectedCacheTableSchema))
	res, _, _, statusCode, err := utils.CallHttp(ctx, "POST", url, headers, nil, nil, nil, tableObj)
	if err != nil {
		err = logs.Err(ctx, err, "error calling eru-ql create table API")
		return err
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("res: %v", res))
	if statusCode == http.StatusOK {
		logs.WithContext(ctx).Info("Cache table created successfully")
		return nil
	} else {
		err = logs.Err(ctx, fmt.Errorf("eru-ql create table API returned status: %d", statusCode), "Cache table creation failed")
		return err
	}
}

// GetCacheStore is a factory function that returns a cache implementation.
func GetCacheStore(cacheStoreType string, projectId string) (cs CacheStoreI) {
	ctx := context.Background()
	logs.WithContext(ctx).Info(fmt.Sprintf("GetCacheStore called for type: %s", cacheStoreType))
	switch strings.ToUpper(cacheStoreType) {
	case "REDIS":
		cs, _ = NewRedisCache()
		//ignore error as return blank cache store as returned by NewRedisCache
	case "REDIS_CLUSTER", "ELASTICACHE":
		cs, _ = NewRedisClusterCache()
		//ignore error as return blank cache store as returned by NewRedisClusterCache
	case "ETCD":
		cs, _ = NewEtcdCache()
		//ignore error as return blank cache store as returned by NewEtcdCache
	case "INMEMORY":
		cs = new(InMemoryCache)
	default:
		_ = logs.Err(ctx, fmt.Errorf("unsupported cache type: %s", cacheStoreType), "unsupported cache type")
		return nil
	}
	cs.ValidatePersistence(ctx, projectId)
	return cs
}
// A configured cache store is shared by everyone configured the same way.
//
// Without this an in-memory store is a cache in name only. Its owner - an agent,
// say - is rebuilt from its JSON on every request, and a store built during that
// rebuild starts empty and is thrown away when the request ends, so it can never
// return a hit to anything. Every read falls through to the database and every
// value written between requests is lost. Two callers configured identically
// want the same cache; two configured differently must not share one, which is
// what the configuration itself is keyed on.
var (
	sharedStores   = map[string]CacheStoreI{}
	sharedStoresMu sync.Mutex
)

// GetSharedCacheStore returns the store configured by rj, building it the first
// time and returning that same instance to every later caller with the same
// configuration.
func GetSharedCacheStore(ctx context.Context, cacheStoreType string, rj *json.RawMessage) (CacheStoreI, error) {
	key, err := storeConfigKey(cacheStoreType, rj)
	if err != nil {
		return nil, err
	}

	sharedStoresMu.Lock()
	defer sharedStoresMu.Unlock()

	if cs, ok := sharedStores[key]; ok {
		return cs, nil
	}

	cs := GetCacheStore(cacheStoreType, "")
	if cs == nil {
		return nil, fmt.Errorf("unsupported cache type: %s", cacheStoreType)
	}
	if err := cs.MakeFromJson(ctx, rj); err != nil {
		return nil, err
	}
	sharedStores[key] = cs
	logs.WithContext(ctx).Info(fmt.Sprintf("cache store %s created and shared for configuration %s", cacheStoreType, key))
	return cs, nil
}

// storeConfigKey identifies a configuration rather than the bytes that happened
// to express it, so the same settings serialised twice do not produce two stores.
func storeConfigKey(cacheStoreType string, rj *json.RawMessage) (string, error) {
	if rj == nil {
		return strings.ToUpper(cacheStoreType), nil
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(*rj, &cfg); err != nil {
		return "", err
	}
	// Values the store fills in for itself say nothing about which store was
	// asked for, and would otherwise split one configuration into two.
	delete(cfg, "cache_values")
	delete(cfg, "persist_error")
	canonical, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(cacheStoreType) + "|" + string(canonical), nil
}

func (cs *CacheStore) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	err := logs.Err(ctx, errors.New("not implemented"), "not implemented")
	return err
}
func (cs *CacheStore) SyncPersistence(ctx context.Context, cacheStoreI CacheStoreI) error {
	logs.WithContext(ctx).Debug("SyncPersistence - Start")

	cacheDbAliasI, cacheDbAliasErr := cacheStoreI.GetAttribute(ctx, "cache_db_alias")
	if cacheDbAliasErr != nil {
		return cacheDbAliasErr
	}
	cacheDbAlias, ok := cacheDbAliasI.(string)
	if !ok {
		return fmt.Errorf("cache_db_alias is not a string")
	}
	cacheStoreTypeI, cacheStoreTypeErr := cacheStoreI.GetAttribute(ctx, "cache_store_type")
	if cacheStoreTypeErr != nil {
		return cacheStoreTypeErr
	}
	cacheStoreType, ok := cacheStoreTypeI.(string)
	if !ok {
		return fmt.Errorf("cache_store_type is not a string")
	}
	if !(strings.EqualFold(cs.CacheDbAlias, cacheDbAlias) && strings.EqualFold(cs.CacheStoreType, cacheStoreType)) {
		//return with no error without syncing so validate will be called again with new dbalias and storetype
		return nil
	}

	peI, peErr := cacheStoreI.GetAttribute(ctx, "persist_enabled")
	if peErr != nil {
		return peErr
	}
	pe, ok := peI.(bool)
	if !ok {
		err := logs.Err(ctx, fmt.Errorf("persist_enabled is not a boolean"), "persist_enabled is not a boolean")
		return err
	}
	cs.PersistEnabled = pe

	peI, peErr = cacheStoreI.GetAttribute(ctx, "persist_error")
	if peErr != nil {
		return peErr
	}
	pe, ok = peI.(bool)
	if !ok {
		err := logs.Err(ctx, fmt.Errorf("persist_error is not a boolean"), "persist_error is not a boolean")
		return err
	}
	cs.PersistError = pe
	return nil
}

func (cs *CacheStore) SyncToDatabase(ctx context.Context, projectId string, cacheData []CacheData) error {
	logs.WithContext(ctx).Debug("SyncToDatabase - Start")

	if !cs.PersistEnabled {
		logs.WithContext(ctx).Info("Persistence not enabled, skipping cache data sync")
		return nil
	}

	if cs.CacheDbAlias == "" {
		// This ran in a background goroutine whose error nobody reads, so a
		// missing alias meant conversations silently stopped being persisted
		// with not one line to say so.
		logs.WithContext(ctx).Error("cache database alias not configured, so nothing was persisted")
		return fmt.Errorf("cache database alias not configured")
	}

	eruqlURL := os.Getenv("ERUQL_BASEURL")
	if eruqlURL == "" {
		logs.WithContext(ctx).Error("ERUQL_BASEURL is not set, so nothing was persisted")
		return fmt.Errorf("ERUQL_BASEURL environment variable not set")
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("persisting %d cache row(s) to %s", len(cacheData), cs.CacheDbAlias))

	mutation := `
	mutation {
  results : insert_public___eru_cache(docs: $docs) $$dbalias$$  {
   err : error
  }
}`
	mutation = strings.Replace(mutation, "$$dbalias$$", fmt.Sprintf("@%s", cs.CacheDbAlias), -1)

	variables := map[string]interface{}{
		"docs": cacheData,
	}

	requestBody := map[string]interface{}{
		"query":     mutation,
		"variables": variables,
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	url := fmt.Sprintf("%s/graphql/%s/execute", strings.TrimSuffix(eruqlURL, "/"), projectId)
	// Capped: a row can carry screenshots a client sent back, and the log is
	// not where they belong.
	b, _ := json.Marshal(requestBody)
	if len(b) > 2000 {
		logs.WithContext(ctx).Info(fmt.Sprintf("requestBody (%d bytes, first 2000): %s", len(b), string(b[:2000])))
	} else {
		logs.WithContext(ctx).Info(fmt.Sprintf("requestBody: %s", string(b)))
	}
	_, _, _, statusCode, err := utils.CallHttp(ctx, "POST", url, headers, nil, nil, nil, requestBody)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to sync cache data to database: %v", err))
		return err
	}

	if statusCode != http.StatusOK {
		logs.WithContext(ctx).Error(fmt.Sprintf("Cache data sync returned status: %d", statusCode))
		return fmt.Errorf("cache data sync failed with status: %d", statusCode)
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Successfully synced cache data to database: %d", len(cacheData)))
	return nil
}

func (cs *CacheStore) LoadFromDatabase(ctx context.Context, projectId, tenantId, cacheKey string, agentName string, createdBy string) (cacheData []CacheData, err error) {
	logs.WithContext(ctx).Debug("LoadFromDatabase - Start")

	if !cs.PersistEnabled {
		logs.WithContext(ctx).Info("Persistence not enabled, skipping database load")
		return nil, nil
	}

	if cs.CacheDbAlias == "" {
		return nil, fmt.Errorf("cache database alias not configured")
	}

	eruqlURL := os.Getenv("ERUQL_BASEURL")
	if eruqlURL == "" {
		return nil, fmt.Errorf("ERUQL_BASEURL environment variable not set")
	}

	query := `
	query {
  cache: public___eru_cache (where : $where) $$dbalias$$  {
    project_id
    tenant_id
    cache_key
    cache_value
    created_at
    updated_at
    expires_at
    access_count
    last_accessed
	created_by
	agent_name
  }
}`
	query = strings.Replace(query, "$$dbalias$$", fmt.Sprintf("@%s", cs.CacheDbAlias), -1)

	whereClause := map[string]interface{}{
		"project_id": projectId,
		"tenant_id":  tenantId,
		"created_by": createdBy,
		"agent_name": agentName,
	}
	if cacheKey != "" {
		whereClause["cache_key"] = cacheKey
	}

	variables := map[string]interface{}{
		"where": whereClause,
	}

	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	url := fmt.Sprintf("%s/graphql/%s/execute", strings.TrimSuffix(eruqlURL, "/"), projectId)

	res, _, _, statusCode, err := utils.CallHttp(ctx, "POST", url, headers, nil, nil, nil, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to query cache data from database: %v", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("database query returned status: %d", statusCode)
	}

	if res == nil {
		logs.WithContext(ctx).Info("No cache data found in database")
		return []CacheData{}, nil
	}
	if resArray, ok := res.([]interface{}); !ok {
		return nil, fmt.Errorf("unexpected response format from database")
	} else if len(resArray) > 0 {
		responseData, ok := resArray[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected response format from database")
		}
		cacheData, ok := responseData["cache"].([]CacheData)
		if !ok {
			// Try to convert each entry individually
			if cacheEntries, ok := responseData["cache"].([]interface{}); ok {
				cacheData = make([]CacheData, len(cacheEntries))
				for i, entry := range cacheEntries {
					if entryMap, ok := entry.(map[string]interface{}); ok {
						entryBytes, err := json.Marshal(entryMap)
						if err != nil {
							return nil, fmt.Errorf("failed to marshal cache entry: %v", err)
						}
						if err := json.Unmarshal(entryBytes, &cacheData[i]); err != nil {
							return nil, fmt.Errorf("failed to unmarshal cache entry: %v", err)
						}
					}
				}
				return cacheData, nil
			}
			return []CacheData{}, nil
		}
		return cacheData, nil

	} else {
		return []CacheData{}, nil
	}
}

func (cs *CacheStore) LoadListFromDatabase(ctx context.Context, projectId, tenantId, cacheKey string, agentName string, createdBy string, limit int, skip int) (cacheData []CacheData, err error) {
	logs.WithContext(ctx).Debug("LoadFromDatabase - Start")

	if !cs.PersistEnabled {
		logs.WithContext(ctx).Info("Persistence not enabled, skipping database load")
		return nil, nil
	}

	if cs.CacheDbAlias == "" {
		return nil, fmt.Errorf("cache database alias not configured")
	}

	eruqlURL := os.Getenv("ERUQL_BASEURL")
	if eruqlURL == "" {
		return nil, fmt.Errorf("ERUQL_BASEURL environment variable not set")
	}

	query := `with minsk as (select cache_key,agent_name, min(cache_sk) cache_sk, max(updated_at) last_updated from eru_cache group by cache_key,agent_name) select a.*, b.last_updated from eru_cache a inner join minsk b on a.cache_sk=b.cache_sk and a.agent_name=b.agent_name where a.project_id='$$project_id$$' and a.tenant_id='$$tenant_id$$' and a.agent_name='$$agent_name$$'`
	//and created_by = '$$userid$$'

	query = strings.Replace(query, "$$project_id$$", projectId, -1)
	query = strings.Replace(query, "$$tenant_id$$", tenantId, -1)
	query = strings.Replace(query, "$$agent_name$$", agentName, -1)
	query = strings.Replace(query, "$$userid$$", createdBy, -1)

	// Newest first. Without an order the rows arrive in whatever order the
	// database happens to return them, and a caller that then puts them in a map
	// loses even that - which is why the conversation list read as shuffled.
	query = query + " order by b.last_updated desc"
	if limit > 0 {
		query = fmt.Sprintf("%s limit %d", query, limit)
	}
	if skip > 0 {
		query = fmt.Sprintf("%s offset %d", query, skip)
	}

	requestBody := map[string]interface{}{
		"query":    query,
		"db_alias": cs.CacheDbAlias,
		"cols":     "",
		"vars":     map[string]interface{}{},
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	url := fmt.Sprintf("%s/sql/%s/execute", strings.TrimSuffix(eruqlURL, "/"), projectId)

	res, _, _, statusCode, err := utils.CallHttp(ctx, "POST", url, headers, nil, nil, nil, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to query cache data from database: %v", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("database query returned status: %d", statusCode)
	}

	if res == nil {
		logs.WithContext(ctx).Info("No cache data found in database")
		return []CacheData{}, nil
	}
	if resArray, ok := res.([]interface{}); !ok {
		return nil, fmt.Errorf("unexpected response format from database")
	} else if len(resArray) > 0 {
		responseData, ok := resArray[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected response format from database")
		}
		cacheData, ok := responseData["Results"].([]CacheData)
		if !ok {
			// Try to convert each entry individually
			if cacheEntries, ok := responseData["Results"].([]interface{}); ok {
				cacheData = make([]CacheData, len(cacheEntries))
				for i, entry := range cacheEntries {
					if entryMap, ok := entry.(map[string]interface{}); ok {
						entryBytes, err := json.Marshal(entryMap)
						if err != nil {
							return nil, fmt.Errorf("failed to marshal cache entry: %v", err)
						}
						if err := json.Unmarshal(entryBytes, &cacheData[i]); err != nil {
							return nil, fmt.Errorf("failed to unmarshal cache entry: %v", err)
						}
					}
				}
				return cacheData, nil
			}
			return []CacheData{}, nil
		}
		return cacheData, nil

	} else {
		return []CacheData{}, nil
	}
}

// expiringStore is a store that can reclaim its own expired entries. Redis does
// this for itself and does not implement it.
type expiringStore interface {
	PurgeExpired(ctx context.Context) int
}

// PurgeExpiredSharedStores reclaims expired entries across every shared store.
//
// Expiry in the in-memory store is lazy - a read of a stale key deletes it, and
// GetKeys hides stale keys rather than removing them. That reclaims nothing from
// an entry nobody comes back to, which is the only kind worth reclaiming: left
// alone, the working set of every conversation ever started stays resident for
// the life of the process.
func PurgeExpiredSharedStores(ctx context.Context) int {
	sharedStoresMu.Lock()
	stores := make([]CacheStoreI, 0, len(sharedStores))
	for _, cs := range sharedStores {
		stores = append(stores, cs)
	}
	sharedStoresMu.Unlock()

	purged := 0
	for _, cs := range stores {
		if p, ok := cs.(expiringStore); ok {
			purged += p.PurgeExpired(ctx)
		}
	}
	if purged > 0 {
		logs.WithContext(ctx).Info(fmt.Sprintf("reclaimed %d expired cache entr(ies)", purged))
	}
	return purged
}

// StartSharedStoreSweeper reclaims expired entries on an interval until the
// context is done.
func StartSharedStoreSweeper(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 10 * time.Minute
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			PurgeExpiredSharedStores(ctx)
		}
	}
}
