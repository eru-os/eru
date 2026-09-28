package module_model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"
	"github.com/eru-os/eru/eru-cache/cache"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	common_types "github.com/eru-os/eru/eru-ql/common_types"
	sqlengine "github.com/eru-os/eru/eru-ql/sql_engine"
	eru_writes "github.com/eru-os/eru/eru-read-write/eru_writes"
	"github.com/eru-os/eru/eru-secret-manager/sm"
	"github.com/eru-os/eru/eru-security-rule/security_rule"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jmoiron/sqlx"
)

const (
	RULETYPE_NONE          = "none"
	RULETYPE_ALWAYS        = "always"
	RULETYPE_CUSTOM        = "custom"
	RULEPREFIX_TOKEN       = "token"
	RULEPREFIX_DOCS        = "docs"
	RULEPREFIX_NONE        = "none"
	RULEINFIX_NONE         = "none."
	QUERY_TYPE_INSERT      = "insert"
	QUERY_TYPE_UPDATE      = "update"
	QUERY_TYPE_DELETE      = "delete"
	QUERY_TYPE_SELECT      = "select"
	COLUMN_MASKING_NONE    = "none"
	COLUMN_MASKING_ENCRYPT = "encrypt"
	COLUMN_MASKING_HASH    = "hash"
	MAKE_JSON_ARRAY_FN     = "$make_json_array_fn"
	MAKE_JSON_ARRAY_FN_STR = "$make_json_array_fn_str"
)

type TableInQuery struct {
	TableName      string
	AliasName      string
	TableKey       string
	TableKeyPrefix string
	TableKeySuffix string
}
type TablesInQuery struct {
	Tables []TableInQuery
}

type ModuleProjectI interface {
	CompareProject(ctx context.Context, compareProject ExtendedProject) (StoreCompare, error)
}

type StoreCompare struct {
	store.StoreCompare
	DeleteQueries               []string               `json:"delete_queries"`
	NewQueries                  []string               `json:"new_queries"`
	MismatchQueries             map[string]interface{} `json:"mismatch_queries"`
	DeleteDataSources           []string               `json:"delete_data_sources"`
	NewDataSources              []string               `json:"new_data_sources"`
	MismatchDataSources         map[string]interface{} `json:"mismatch_data_sources"`
	DeleteTables                []string               `json:"delete_tables"`
	NewTables                   []string               `json:"new_tables"`
	MismatchTables              map[string]interface{} `json:"mismatch_tables"`
	DeleteJoins                 []string               `json:"delete_joins"`
	NewJoins                    []string               `json:"new_joins"`
	MismatchJoins               map[string]interface{} `json:"mismatch_joins"`
	DeleteTableSecurity         []string               `json:"delete_table_security"`
	NewTableSecurity            []string               `json:"new_table_security"`
	MismatchTableSecurity       map[string]interface{} `json:"mismatch_table_security"`
	DeleteTableTransformation   []string               `json:"delete_table_transformation"`
	NewTableTransformation      []string               `json:"new_table_transformation"`
	MismatchTableTransformation map[string]interface{} `json:"mismatch_table_transformation"`
}

type ExtendedProject struct {
	Project
	Variables       store.Variables            `json:"variables"`
	SecretManager   sm.SmStoreI                `json:"secret_manager"`
	TenantVariables map[string]store.Variables `json:"tenant_variables"`
}

type Project struct {
	ProjectId       string                  `json:"project_id" eru:"required"`
	DataSources     map[string]*DataSource  `json:"data_sources"` //DB alias is the key
	MyQueries       map[string]*MyQuery     `json:"my_queries"`   //queryName is key
	Tenants         map[string]TenantConfig `json:"tenants"`      //tenantId is the key
	ProjectSettings ProjectSettings         `json:"project_settings"`
}

type TenantConfig struct {
	TenantId    string                 `json:"tenant_id" eru:"required"`
	DataSources map[string]*DataSource `json:"data_sources"` //DB alias is the key
	MyQueries   map[string]*MyQuery    `json:"my_queries"`   //queryName is key
}
type ProjectSettings struct {
	ClaimsKey string `json:"claims_key" eru:"required"`
}

/*
	type AesKey struct {
		Key string
		//Bits int
	}

	type TokenSecret struct {
		HeaderKey  string
		SecretAlgo string
		SecretKey  string
		JwkUrl     string
		Audience   []string
		Issuer     []string
	}
*/
type MyQuery struct {
	QueryName      string                                 `json:"query_name"`
	Query          string                                 `json:"query"`
	Vars           map[string]interface{}                 `json:"vars"`
	QueryType      string                                 `json:"query_type"`
	DBAlias        string                                 `json:"db_alias"`
	ReadWrite      string                                 `json:"read_write"`
	Cols           string                                 `json:"cols"`
	SecurityRule   security_rule.SecurityRule             `json:"security_rule"`
	ExcelStyles    map[string]eru_writes.CellFormatter    `json:"excel_styles"`
	Columns        map[string]eru_writes.ColumnarSettings `json:"columns"`
	PivotConfig    map[string]eru_writes.PivotTableConfig `json:"pivot_config"`
	ImageConfig    eru_writes.ImageConfig                 `json:"image_config"`
	ExcludeColumns []string                               `json:"exclude_columns"`
	CacheTTL       int                                    `json:"cache_ttl,omitempty"`
	CacheSkip      bool                                   `json:"cache_skip,omitempty"`
	CacheLock      bool                                   `json:"cache_lock,omitempty"`
}

type AggregationConfig struct {
	Func  string `json:"func"`
	Field string `json:"field"`
	Alias string `json:"alias"`
}

type GroupByConfig struct {
	Active       bool                `json:"-"`
	GroupBy      []string            `json:"group_by"`
	Aggregations []AggregationConfig `json:"aggregations"`
	GroupOrderBy []string            `json:"group_order_by"`
}

type QueryWrapConfig struct {
	Filter map[string]interface{} `json:"filter"`
	Sort   []string               `json:"sort"`
	Limit  int                    `json:"limit"`
	Skip   int                    `json:"skip"`
}

type QueryCacheConfig struct {
	Enabled        bool     `json:"enabled"`
	DefaultTTLSec  int      `json:"default_ttl_sec"`
	MaxValueBytes  int      `json:"max_value_bytes"`
	VolatileTables []string `json:"volatile_tables"`
	LockHotQueries bool     `json:"lock_hot_queries"`
}

type DataSource struct {
	ProjectId                  string                                               `json:"-"`
	DbAlias                    string                                               `json:"db_alias" eru:"required"`
	DbType                     string                                               `json:"db_type" eru:"required"`
	DbName                     string                                               `json:"db_name" eru:"required"`
	DbConfig                   DbConfig                                             `json:"db_config" eru:"optional"`
	ResolvedDbConfig           DbConfig                                             `json:"-" eru:"optional"`
	IcebergConfig              IcebergConfig                                        `json:"iceberg_config" eru:"optional"`
	SqlEngine                  sqlengine.SQLEngineI                                 `json:"sql_engine"`
	SchemaTables               map[string]map[string]common_types.TableColsMetaData `json:"schema_tables"` //tableName is the key
	OtherTables                map[string]map[string]common_types.TableColsMetaData `json:"other_tables"`  //tableName is the key
	SchemaTablesSecurity       map[string]SecurityRules                             `json:"schema_tables_security"`
	SchemaTablesTransformation map[string]TransformRules                            `json:"schema_tables_transformation"`
	TableJoins                 map[string]*TableJoins                               `json:"table_joins"`
	Con                        *sqlx.DB                                             `json:"-"`
	ConStatus                  bool                                                 `json:"con_status"`
	ReadDbConfigs              []*ReadDbConfig                                      `json:"read_db_configs" eru:"optional"`
	ReadPolicy                 ReadPolicy                                           `json:"read_policy" eru:"optional"`
	ReadCounter                uint64                                               `json:"-"`
	DbSecurityRules            SecurityRules                                        `json:"db_security_rules"`
	QueryCache                 cache.CacheStoreI                                    `json:"query_cache"`
	QueryCacheClone            cache.CacheStoreI                                    `json:"-"`
	QueryCacheConfig           QueryCacheConfig                                     `json:"query_cache_config"`
}

type ReadDbConfig struct {
	Name             string   `json:"name" eru:"required"`
	DbConfig         DbConfig `json:"db_config" eru:"required"`
	ResolvedDbConfig DbConfig `json:"-" eru:"optional"`
	Weight           int      `json:"weight"`
	Disabled         bool     `json:"disabled"`
	Con              *sqlx.DB `json:"-"`
	ConStatus        bool     `json:"con_status"`
}

type ReadPolicy struct {
	Strategy           string `json:"strategy"`
	IncludeMainInReads bool   `json:"include_main_in_reads"`
	MainWeight         int    `json:"main_weight"`
	FailoverToMain     bool   `json:"failover_to_main"`
	Disabled           bool   `json:"disabled"`
}

type TableJoins struct {
	Table1Name       string                 `json:"table1_name"`
	Table1Cols       []string               `json:"table1_cols"`
	Table2Name       string                 `json:"table2_name"`
	Table2Cols       []string               `json:"table2_cols"`
	IsActive         bool                   `json:"is_active"`
	IsCustom         bool                   `json:"is_custom"`
	ComplexCondition map[string]interface{} `json:"complex_condition"`
}

/*
	type CustomRule struct {
		AND []CustomRuleDetails `json:",omitempty"`
		OR  []CustomRuleDetails `json:",omitempty"`
	}

	type CustomRuleDetails struct {
		DataType  string              `json:",omitempty"`
		Variable1 string              `json:",omitempty"`
		Variable2 string              `json:",omitempty"`
		Operator  string              `json:",omitempty"`
		ErrorMsg  string              `json:",omitempty"`
		AND       []CustomRuleDetails `json:",omitempty"`
		OR        []CustomRuleDetails `json:",omitempty"`
	}

	type SecurityRule struct {
		RuleType   string
		CustomRule CustomRule
	}
*/
type SecurityRules struct {
	Create     security_rule.SecurityRule `json:"create"`
	Drop       security_rule.SecurityRule `json:"drop"`
	Alter      security_rule.SecurityRule `json:"alter"`
	Insert     security_rule.SecurityRule `json:"insert"`
	Update     security_rule.SecurityRule `json:"update"`
	Delete     security_rule.SecurityRule `json:"delete"`
	Select     security_rule.SecurityRule `json:"select"`
	Query      security_rule.SecurityRule `json:"query"`
	IsTemplate bool                       `json:"is_template"`
}
type TransformRules struct {
	TransformInput  TransformRule `json:"transform_input"`
	TransformOutput TransformRule `json:"transform_output"`
}
type TransformRule struct {
	RuleType string                `json:"rule_type"`
	ApplyOn  []string              `json:"apply_on"`
	Rules    []TransformRuleDetail `json:"rules"`
}

type TransformRuleDetail struct {
	CustomRule         security_rule.CustomRule `json:"custom_rule"`
	ForceColumnValues  map[string]string        `json:"force_column_values"`
	RemoveColumnValues []string                 `json:"remove_column_values"`
	ComplexScript      string                   `json:"complex_script"`
	RuleRank           int                      `json:"rule_rank"`
}

type DbConfig struct {
	Host          string        `json:"host" eru:"required"`
	Port          string        `json:"port" eru:"required"`
	User          string        `json:"user" eru:"required"`
	Password      string        `json:"password" eru:"required"`
	DefaultDB     string        `json:"default_db" eru:"required"`
	DefaultSchema string        `json:"default_schema" eru:"required"`
	DriverConfig  DriverConfig  `json:"driver_config" eru:"required"`
	OtherDbConfig OtherDbConfig `json:"other_db_config"`
}

type IcebergConfig struct {
	S3TablesConfig S3TablesConfig `json:"s3_tables_config" eru:"optional"`
	Uri            string         `json:"-"`
	Warehouse      string         `json:"-"`
	Database       string         `json:"database" eru:"required"`
	TenantId       string         `json:"-"`
	CatalogType    string         `json:"catalog_type" eru:"required"`
}
type S3TablesConfig struct {
	BucketName     string           `json:"bucket_name" eru:"required"`
	Region         string           `json:"region" eru:"required"`
	Authentication string           `json:"authentication" eru:"required"`
	Key            string           `json:"key" eru:"required"`
	Secret         string           `json:"secret" eru:"required"`
	BucketArn      string           `json:"bucket_arn"`
	UseForDDL      bool             `json:"use_for_ddl"`
	Session        *s3tables.Client `json:"-"`
	S3Session      *s3.Client       `json:"-"`
}

type DriverConfig struct {
	MaxOpenConns    int           `json:"max_open_conns" eru:"required"`
	MaxIdleConns    int           `json:"max_idle_conns" eru:"required"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" eru:"required"`
}

type OtherDbConfig struct {
	RowLimit     int `json:"row_limit"`
	QueryTimeOut int `json:"query_time_out"`
}

type QueryResultMaker struct {
	QueryLevel    int
	QuerySubLevel []int
	MainTableName string
	MainAliasName string
	Tables        [][]Tables
	SQLQuery      string
	UseWriter     bool
}

type MutationResultMaker struct {
	MainTableName   string
	MainAliasName   string
	MutationRecords []MutationRecord
	MutationReturn  MutationReturn
	QueryType       string
	SingleTxn       bool
	OpenTxn         bool
	CloseTxn        bool
	TxnFlag         bool
	IsNested        bool
	DBQuery         string
	PreparedQuery   bool
}

type MutationRecord struct {
	Cols            string
	NonNestedCols   string
	NonNestedValues []interface{}
	UpdatedCols     string
	ColsPlaceholder string
	Values          []interface{}
	ChildRecords    map[string][]MutationRecord
	TableJoins      map[string]TableJoins
	DBQuery         string
}

type MutationReturn struct {
	ReturnError      bool
	ReturnDoc        bool
	ReturnErrorAlias string
	ReturnDocAlias   string
	ReturnFields     string
}

// tables used in query
type Tables struct {
	Name     string
	Nested   bool
	SqlQuery string
}

func (ePrj *ExtendedProject) UnmarshalJSON(b []byte) error {
	logs.Logger.Info("UnMarshal ExtendedProject - Start")
	ctx := context.Background()
	var ePrjMap map[string]*json.RawMessage
	err := json.Unmarshal(b, &ePrjMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	projectId := ""
	if _, ok := ePrjMap["project_id"]; ok {
		if ePrjMap["project_id"] != nil {
			err = json.Unmarshal(*ePrjMap["project_id"], &projectId)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.ProjectId = projectId
		}
	}

	var ps ProjectSettings
	if _, ok := ePrjMap["project_settings"]; ok {
		if ePrjMap["project_settings"] != nil {
			err = json.Unmarshal(*ePrjMap["project_settings"], &ps)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.ProjectSettings = ps
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
			ePrj.Variables = vars
		}
	}

	var ds map[string]*DataSource
	if _, ok := ePrjMap["data_sources"]; ok {
		if ePrjMap["data_sources"] != nil {
			err = json.Unmarshal(*ePrjMap["data_sources"], &ds)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.DataSources = ds
		}
	}

	var mq map[string]*MyQuery
	if _, ok := ePrjMap["my_queries"]; ok {
		if ePrjMap["my_queries"] != nil {
			err = json.Unmarshal(*ePrjMap["my_queries"], &mq)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.MyQueries = mq
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
					ePrj.SecretManager = smI
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

func (ds *DataSource) GetTableJoins(ctx context.Context, parentTableName string, childTableName string, otherTables map[string]string) (TableJoins, error) {
	logs.WithContext(ctx).Debug("GetTableJoins - Start")
	// TODO if schema is not passed with table name then compare with default schema set at datasource level
	tj := TableJoins{}
	if _, ok := ds.SchemaTables[parentTableName]; !ok {
		return tj, errors.New(fmt.Sprint(parentTableName, " table not found"))
	}
	if _, ok := ds.SchemaTables[childTableName]; !ok {
		return tj, errors.New(fmt.Sprint(childTableName, " table not found"))
	}
	tempKey := fmt.Sprint(parentTableName, "___", childTableName)
	tempKey1 := fmt.Sprint(childTableName, "___", parentTableName)
	if val, ok := ds.TableJoins[tempKey]; !ok {
		if val, ok := ds.TableJoins[tempKey1]; !ok {
			logs.WithContext(ctx).Info(fmt.Sprint("table joins not found for ", parentTableName, " and ", childTableName))
			newOtherTables := make(map[string]string)
			for k, _ := range otherTables {
				if k != parentTableName && k != childTableName {
					newOtherTables[k] = ""
				}
			}
			newParentTableName := ""
			finalOtherTables := make(map[string]string)
			for k, _ := range newOtherTables {
				if newParentTableName == "" {
					newParentTableName = k
				} else {
					finalOtherTables[k] = ""
				}
			}
			return ds.GetTableJoins(ctx, newParentTableName, childTableName, finalOtherTables)
			//return tj, errors.New(fmt.Sprint("table joins not found for ", parentTableName, " and ", childTableName))

		} else {
			tj = *val
			//swaping so the consumer of this function will always get child details as table 2 details and parent details as table 1 details
			tempTableName := tj.Table1Name
			tempTableCols := tj.Table1Cols
			tj.Table1Name = tj.Table2Name
			tj.Table1Cols = tj.Table2Cols
			tj.Table2Name = tempTableName
			tj.Table2Cols = tempTableCols
		}
	} else {
		tj = *val
	}
	return tj, nil
}

func (ds *DataSource) AddTableJoins(ctx context.Context, tj *TableJoins) {
	logs.WithContext(ctx).Debug("AddTableJoins - Start")
	tempKey := fmt.Sprint(tj.Table1Name, "___", tj.Table2Name)
	if ds.TableJoins == nil {
		ds.TableJoins = make(map[string]*TableJoins)
	}
	ds.TableJoins[tempKey] = tj
}
func (ds *DataSource) RemoveTableJoins(ctx context.Context, tj *TableJoins) {
	logs.WithContext(ctx).Debug("RemoveTableJoins - Start")
	tempKey := fmt.Sprint(tj.Table1Name, "___", tj.Table2Name)
	delete(ds.TableJoins, tempKey)
}

func (tj *TableJoins) GetOnClause(ctx context.Context) (res map[string]interface{}) {
	logs.WithContext(ctx).Debug("GetOnClause - Start")
	onClause := make(map[string]interface{})
	for i := 0; i < len(tj.Table1Cols); i++ {
		k := fmt.Sprint(tj.Table1Name, ".", tj.Table1Cols[i])
		kk := fmt.Sprint(tj.Table2Name, ".", tj.Table2Cols[i])
		onClause[k] = kk
	}
	if tj.ComplexCondition != nil {
		for k, v := range tj.ComplexCondition {
			onClause[k] = v
		}
	}
	res = make(map[string]interface{})
	res["on"] = onClause

	return res
}

func (ds *DataSource) CreateTable(ctx context.Context, tableName string, tableObj map[string]common_types.TableColsMetaData) (err error) {
	logs.WithContext(ctx).Debug("CreateTable - Start")
	return
}

func (prj *ExtendedProject) CompareProject(ctx context.Context, compareProject ExtendedProject) (StoreCompare, error) {
	storeCompare := StoreCompare{}
	storeCompare.CompareVariables(ctx, prj.Variables, compareProject.Variables)
	storeCompare.CompareSecretManager(ctx, prj.SecretManager, compareProject.SecretManager)

	var diffR utils.DiffReporter
	if !cmp.Equal(prj.ProjectSettings, compareProject.ProjectSettings, cmp.Reporter(&diffR)) {
		if storeCompare.MismatchSettings == nil {
			storeCompare.MismatchSettings = make(map[string]interface{})
		}
		storeCompare.MismatchSettings["settings"] = diffR.Output()
	}

	for _, mq := range prj.MyQueries {
		var diffR utils.DiffReporter
		qFound := false
		for _, cq := range compareProject.MyQueries {
			if mq.QueryName == cq.QueryName {
				qFound = true
				if !cmp.Equal(mq, cq, cmp.Reporter(&diffR)) {
					if storeCompare.MismatchQueries == nil {
						storeCompare.MismatchQueries = make(map[string]interface{})
					}
					storeCompare.MismatchQueries[mq.QueryName] = diffR.Output()
				}
				break
			}
		}
		if !qFound {
			storeCompare.DeleteQueries = append(storeCompare.DeleteQueries, mq.QueryName)
		}
	}

	for _, cq := range compareProject.MyQueries {
		qFound := false
		for _, mq := range prj.MyQueries {
			if mq.QueryName == cq.QueryName {
				qFound = true
				break
			}
		}
		if !qFound {
			storeCompare.NewQueries = append(storeCompare.NewQueries, cq.QueryName)
		}
	}

	//compare datasources
	for _, md := range prj.DataSources {
		var diffR utils.DiffReporter
		dsFound := false
		for _, cd := range compareProject.DataSources {
			if md.DbAlias == cd.DbAlias {
				dsFound = true
				if !cmp.Equal(md, cd, cmpopts.IgnoreFields(DataSource{}, "Con", "ReadCounter", "ResolvedDbConfig", "SchemaTables", "SchemaTablesTransformation", "TableJoins"), cmpopts.IgnoreFields(ReadDbConfig{}, "Con", "ConStatus", "ResolvedDbConfig"), cmpopts.IgnoreFields(common_types.TableColsMetaData{}, "ColPosition"), cmp.Reporter(&diffR)) {
					if storeCompare.MismatchDataSources == nil {
						storeCompare.MismatchDataSources = make(map[string]interface{})
					}
					storeCompare.MismatchDataSources[md.DbAlias] = diffR.Output()
				}

				for mstKey, mst := range md.SchemaTables {
					var diffSt utils.DiffReporter
					stFound := false
					for cstKey, cst := range cd.SchemaTables {
						if mstKey == cstKey {
							stFound = true
							if !cmp.Equal(mst, cst, cmpopts.IgnoreFields(common_types.TableColsMetaData{}, "ColPosition"), cmp.Reporter(&diffSt)) {
								if storeCompare.MismatchTables == nil {
									storeCompare.MismatchTables = make(map[string]interface{})
								}
								storeCompare.MismatchTables[mstKey] = diffSt.Output()
							}
							break
						}
					}
					if !stFound {
						storeCompare.DeleteTables = append(storeCompare.DeleteTables, mstKey)
					}
				}
				for cstK, _ := range cd.SchemaTables {
					sFound := false
					for mstK, _ := range md.SchemaTables {
						if mstK == cstK {
							sFound = true
							break
						}
					}
					if !sFound {
						storeCompare.NewTables = append(storeCompare.NewTables, cstK)
					}
				}

				for mstKey, mst := range md.SchemaTablesSecurity {
					var diffSt utils.DiffReporter
					stFound := false
					for cstKey, cst := range cd.SchemaTablesSecurity {
						if mstKey == cstKey {
							stFound = true
							if !cmp.Equal(mst, cst, cmp.Reporter(&diffSt)) {
								if storeCompare.MismatchTableSecurity == nil {
									storeCompare.MismatchTableSecurity = make(map[string]interface{})
								}
								storeCompare.MismatchTableSecurity[mstKey] = diffSt.Output()
							}
							break
						}
					}
					if !stFound {
						storeCompare.DeleteTableSecurity = append(storeCompare.DeleteTableSecurity, mstKey)
					}
				}
				for cstK, _ := range cd.SchemaTablesSecurity {
					sFound := false
					for mstK, _ := range md.SchemaTablesSecurity {
						if mstK == cstK {
							sFound = true
							break
						}
					}
					if !sFound {
						storeCompare.NewTableSecurity = append(storeCompare.NewTableSecurity, cstK)
					}
				}

				for mstKey, mst := range md.SchemaTablesTransformation {
					var diffSt utils.DiffReporter
					stFound := false
					for cstKey, cst := range cd.SchemaTablesTransformation {
						if mstKey == cstKey {
							stFound = true
							if !cmp.Equal(mst, cst, cmp.Reporter(&diffSt)) {
								if storeCompare.MismatchTableTransformation == nil {
									storeCompare.MismatchTableTransformation = make(map[string]interface{})
								}
								storeCompare.MismatchTableTransformation[mstKey] = diffSt.Output()
							}
							break
						}
					}
					if !stFound {
						storeCompare.DeleteTableTransformation = append(storeCompare.DeleteTableTransformation, mstKey)
					}
				}
				for cstK, _ := range cd.SchemaTablesTransformation {
					sFound := false
					for mstK, _ := range md.SchemaTablesTransformation {
						if mstK == cstK {
							sFound = true
							break
						}
					}
					if !sFound {
						storeCompare.NewTableTransformation = append(storeCompare.NewTableTransformation, cstK)
					}
				}

				for mstKey, mst := range md.TableJoins {
					var diffSt utils.DiffReporter
					stFound := false
					for cstKey, cst := range cd.TableJoins {
						if mstKey == cstKey {
							stFound = true
							if !cmp.Equal(*mst, *cst, cmp.Reporter(&diffSt)) {
								if storeCompare.MismatchJoins == nil {
									storeCompare.MismatchJoins = make(map[string]interface{})
								}
								storeCompare.MismatchJoins[mstKey] = diffSt.Output()
							}
							break
						}
					}
					if !stFound {
						storeCompare.DeleteJoins = append(storeCompare.DeleteJoins, mstKey)
					}
				}
				for cstK, _ := range cd.TableJoins {
					sFound := false
					for mstK, _ := range md.TableJoins {
						if mstK == cstK {
							sFound = true
							break
						}
					}
					if !sFound {
						storeCompare.NewJoins = append(storeCompare.NewJoins, cstK)
					}
				}

				break
			}
		}
		if !dsFound {
			storeCompare.DeleteDataSources = append(storeCompare.DeleteDataSources, md.DbAlias)
		}
	}
	for _, cd := range compareProject.DataSources {
		dFound := false
		for _, md := range prj.DataSources {
			if md.DbAlias == cd.DbAlias {
				dFound = true
				break
			}
		}
		if !dFound {
			storeCompare.NewDataSources = append(storeCompare.NewDataSources, cd.DbAlias)
		}
	}

	return storeCompare, nil
}

type OrderedTableMap struct {
	Alias string
	Obj   TableInQuery
}

type MapSorterTable []*OrderedTableMap

func (a MapSorterTable) Len() int {
	return len(a)
}
func (a MapSorterTable) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a MapSorterTable) Less(i, j int) bool {
	return a[i].Alias == ""
}

// UnmarshalJSON implements the json.Unmarshaler interface
// This method will be called automatically when json.Unmarshal is used on DataSource
func (ds *DataSource) UnmarshalJSON(b []byte) error {
	logs.Logger.Info("DataSource UnmarshalJSON - Start")
	ctx := context.Background()
	type TempDataSource struct {
		DbAlias                    string                                               `json:"db_alias"`
		DbType                     string                                               `json:"db_type"`
		DbName                     string                                               `json:"db_name"`
		DbConfig                   DbConfig                                             `json:"db_config"`
		IcebergConfig              IcebergConfig                                        `json:"iceberg_config"`
		SqlEngineType              string                                               `json:"sql_engine_type"`
		SchemaTables               map[string]map[string]common_types.TableColsMetaData `json:"schema_tables"`
		OtherTables                map[string]map[string]common_types.TableColsMetaData `json:"other_tables"`
		SchemaTablesSecurity       map[string]SecurityRules                             `json:"schema_tables_security"`
		SchemaTablesTransformation map[string]TransformRules                            `json:"schema_tables_transformation"`
		TableJoins                 map[string]*TableJoins                               `json:"table_joins"`
		ConStatus                  bool                                                 `json:"con_status"`
		ReadDbConfigs              []*ReadDbConfig                                      `json:"read_db_configs"`
		ReadPolicy                 ReadPolicy                                           `json:"read_policy"`
		DbSecurityRules            SecurityRules                                        `json:"db_security_rules"`
		QueryCacheConfig           QueryCacheConfig                                     `json:"query_cache_config"`
	}
	var tempDs TempDataSource
	if err := json.Unmarshal(b, &tempDs); err != nil {
		err = logs.Err(ctx, err, "failed to unmarshal data source")
		return err
	}
	ds.DbAlias = tempDs.DbAlias
	ds.DbType = tempDs.DbType
	ds.DbName = tempDs.DbName
	ds.DbConfig = tempDs.DbConfig
	ds.IcebergConfig = tempDs.IcebergConfig
	ds.SchemaTables = tempDs.SchemaTables
	ds.OtherTables = tempDs.OtherTables
	ds.SchemaTablesSecurity = tempDs.SchemaTablesSecurity
	ds.SchemaTablesTransformation = tempDs.SchemaTablesTransformation
	ds.TableJoins = tempDs.TableJoins
	ds.ConStatus = tempDs.ConStatus
	ds.ReadDbConfigs = tempDs.ReadDbConfigs
	ds.ReadPolicy = tempDs.ReadPolicy
	ds.DbSecurityRules = tempDs.DbSecurityRules
	ds.QueryCacheConfig = tempDs.QueryCacheConfig

	var dsMap map[string]*json.RawMessage
	err := json.Unmarshal(b, &dsMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	var sqlEngineObj map[string]*json.RawMessage
	var sqlEngineJson *json.RawMessage
	if _, ok := dsMap["sql_engine"]; ok {
		if dsMap["sql_engine"] != nil {
			err = json.Unmarshal(*dsMap["sql_engine"], &sqlEngineObj)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			err = json.Unmarshal(*dsMap["sql_engine"], &sqlEngineJson)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			var sqlEngineType string
			if _, seOk := sqlEngineObj["sql_engine_type"]; seOk {
				err = json.Unmarshal(*sqlEngineObj["sql_engine_type"], &sqlEngineType)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				sqlEngineI := sqlengine.GetSQLEngine(sqlEngineType)
				err = sqlEngineI.MakeFromJson(ctx, sqlEngineJson)
				if err == nil {
					ds.SqlEngine = sqlEngineI
				} else {
					return err
				}
			} else {
				logs.WithContext(ctx).Info("ignoring secret manager as sm_store_type attribute not found")
			}
		}
	}

	var cacheStoreObj map[string]*json.RawMessage
	var cacheStoreJson *json.RawMessage
	if _, ok := dsMap["query_cache"]; ok {
		if dsMap["query_cache"] != nil {
			if err = json.Unmarshal(*dsMap["query_cache"], &cacheStoreObj); err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			if err = json.Unmarshal(*dsMap["query_cache"], &cacheStoreJson); err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			if _, seOk := cacheStoreObj["cache_store_type"]; seOk {
				var cacheStoreType string
				if err = json.Unmarshal(*cacheStoreObj["cache_store_type"], &cacheStoreType); err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				if cacheStoreType != "" {
					cacheStoreI := cache.GetCacheStore(cacheStoreType, "")
					if cacheStoreI != nil {
						if mfjErr := cacheStoreI.MakeFromJson(ctx, cacheStoreJson); mfjErr != nil {
							logs.WithContext(ctx).Warn("query_cache MakeFromJson failed; caching disabled for this datasource: " + mfjErr.Error())
						} else {
							ds.QueryCache = cacheStoreI
						}
					}
				}
			}
		}
	}
	return nil
}

func (ds *DataSource) SetQueryCache(ctx context.Context, cacheStoreI cache.CacheStoreI) error {
	logs.WithContext(ctx).Debug("SetQueryCache - Start")
	ds.QueryCache = cacheStoreI
	return nil
}

func (ds *DataSource) GetQueryCache() cache.CacheStoreI {
	return ds.QueryCacheClone
}

func (ds *DataSource) ValidateQueryCache(ctx context.Context, projectId string) error {
	logs.WithContext(ctx).Debug("ValidateQueryCache - Start")
	store := ds.GetQueryCache()
	if store == nil {
		return nil
	}
	return store.ValidatePersistence(ctx, projectId)
}
