package ds

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	common_types "github.com/eru-os/eru/eru-ql/common_types"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/qlcache"
  "github.com/eru-os/eru/eru-ql/derived"
	"github.com/eru-os/eru/eru-security-rule/security_rule"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"

	//"github.com/graphql-go/graphql/language/ast"
	//"github.com/graphql-go/graphql/language/kinds"
	"github.com/jmoiron/sqlx"
	//"log"
	//"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var blockedWords = []string{"SELECT * FROM", "SELECT*FROM", "INSERT INTO ", "DELETE FROM ", "CREATE TABLE ", "CREATE FUNCTION ", "CREATE PROCEDURE ", "CREATE VIEW ", "CREATE INDEX ", "CREATE SEQUENCE ", "DROP TABLE ", "DROP FUNCTION ", "DROP VIEW ", "DROP INDEX ", "DROP SEQUENCES", "ALTER TABLE", "TRUNCATE TABLE ", "RENAME TABLE ", "SAVEPOINT ", "\\U003CSCRIPT"}

// var blockedRegex = []string{"OR[ ]*'", "AND[ ]*'", "GRANT\\s+\\w+\\s+ON"}
var blockedRegex = []string{"GRANT\\s+\\w+\\s+ON"}

type tablesInQuery struct {
	name   string
	nested bool
}
type ResultDataTypes struct {
	ColName             string
	ColType             string
	ColDatabaseTypeName string
}
type SqlMaker struct {
	TestFlag      bool
	QueryType     string
	MainTableName string
	MainAliasName string
	TableNames    map[string]string
	//AllTableNamesNew [][]tablesInQuery
	MainTableDB     string
	WhereClause     string
	SortClause      string
	ColumnList      string
	DBColumns       string
	GroupList       string
	JoinClause      string
	ChildChange     string
	Limit           int
	Skip            int
	HasAggregate    bool
	DistinctResults bool
	queryLevel      int
	querySubLevel   []int
	tables          [][]module_model.Tables
	//resultHolder      []map[string]interface{}
	resultHolderNew [][]map[string]interface{}
	//resultIndexHolder []int
	resultIndexHolderNew [][]map[int]int
	result               map[string]interface{}
	ResultDataTypes      []ResultDataTypes
	//tempRA               []map[int]int
	//tempA                map[int]int
	//t                    int
	//InsertData     []map[string]interface{}
	MutationRecords     []module_model.MutationRecord
	TxnFlag             bool
	MutationReturn      module_model.MutationReturn
	MutationSelectQuery string
	MutationSelectCols  string
	SingleTxn           bool
	openTxn             bool
	closeTxn            bool
	tx                  *sqlx.Tx
	IsNested            bool
	DBQuery             string
	PreparedQuery       bool
	executedSQLs        []string
}

type GraphqlResult struct {
	gqlr map[string]interface{}
	idx  int
}

var DefaultDriverConfig = module_model.DriverConfig{MaxOpenConns: 10, MaxIdleConns: 2, ConnMaxLifetime: time.Hour}
var DefaultOtherConfig = module_model.OtherDbConfig{RowLimit: 1000, QueryTimeOut: 60}
var DefaultReadPolicy = module_model.ReadPolicy{
	Strategy:           StrategyRoundRobin,
	IncludeMainInReads: false,
	MainWeight:         0,
	FailoverToMain:     true,
}
var emptyCustomRule = security_rule.CustomRule{}
var DefaultDbSecurityRules = module_model.SecurityRules{
	Create:     security_rule.SecurityRule{RuleType: "Allow", CustomRule: emptyCustomRule},
	Drop:       security_rule.SecurityRule{RuleType: "Deny", CustomRule: emptyCustomRule},
	Alter:      security_rule.SecurityRule{RuleType: "Allow", CustomRule: emptyCustomRule},
	Insert:     security_rule.SecurityRule{RuleType: "Allow", CustomRule: emptyCustomRule},
	Update:     security_rule.SecurityRule{RuleType: "Allow", CustomRule: emptyCustomRule},
	Delete:     security_rule.SecurityRule{RuleType: "Deny", CustomRule: emptyCustomRule},
	Select:     security_rule.SecurityRule{RuleType: "Allow", CustomRule: emptyCustomRule},
	Query:      security_rule.SecurityRule{RuleType: "Allow", CustomRule: emptyCustomRule},
	IsTemplate: false,
}

type SqlMakerI interface {
	GetReturnAlias(ctx context.Context) string
	GetBaseSqlMaker(ctx context.Context) *SqlMaker
	//ProcessGraphQL(sel ast.Selection, vars map[string]interface{}, myself SqlMakerI, datasource *model.DataSource, ExecuteFlag bool) (res map[string]interface{}, query string, cols string , err error)
	//ProcessMutationGraphQL(sel ast.Selection, vars map[string]interface{}, myself SqlMakerI, datasource *model.DataSource, singleTxn bool, openTxn bool, closeTxn bool, query string, cols string) (res map[string]interface{}, err error)
	CheckMe(ctx context.Context)
	MakeQuery(ctx context.Context) (dbQuery string, err string)
	//MakeSQLQuery(sqlObject ql.SQLObject) (dbQuery string, err string)
	//MakeMutationQuery(idx int, docs []MutationRecord, tableName string) (dbQuery string)
	AddLimitSkipClause(ctx context.Context, query string, limit int, skip int, globalLimit int) (newQuery string)
	CreateConn(ctx context.Context, dataSource *module_model.DataSource) error
	ConnectReadReplica(ctx context.Context, dataSource *module_model.DataSource, idx int) error
	ExecuteQuery(ctx context.Context, datasource *module_model.DataSource, qrm module_model.QueryResultMaker) (res map[string]interface{}, err error)
	ExecuteMutationQuery(ctx context.Context, datasource *module_model.DataSource, myself SqlMakerI, mrm module_model.MutationResultMaker) (res []map[string]interface{}, err error)
	ExecutePreparedQuery(ctx context.Context, query string, datasource *module_model.DataSource) (res map[string]interface{}, err error)
	ExecuteQueryForCsv(ctx context.Context, query string, datasource *module_model.DataSource, aliasName string, myself SqlMakerI) (res map[string]interface{}, err error)
	RollbackQuery(ctx context.Context) (err error)
	GetTableList(ctx context.Context, datasource *module_model.DataSource, tableName string, myself SqlMakerI) (err error)
	GetTableMetaDataSQL(ctx context.Context, tableName string) string
	SaveTable(ctx context.Context, tableName string, tableStructure common_types.TableStructure, isEdit bool, dataSource *module_model.DataSource) (err error)
	DropTable(ctx context.Context, tableName string, dataSource *module_model.DataSource) (err error)
	MakeCreateTableSQL(ctx context.Context, tableName string, tableObj map[string]common_types.TableColsMetaData) (string, error)
	MakeDropTableSQL(ctx context.Context, tableName string) (string, error)
	getDataTypeMapping(ctx context.Context, dataType string) string
	getErutoDBDataTypeMapping(ctx context.Context, dataType string) string
	GetSqlResult(ctx context.Context) map[string]interface{}
	GetPreparedQueryPlaceholder(ctx context.Context, rowCount int, colCount int, single bool) string
	MakeUpsertQuery(ctx context.Context, tableName string, insertCols []string, colsPlaceholder string, conflictCols []string, updateCols []string, action string, returningStr string) (string, error)
	GetBlockedWords() []string
	GetBlockedRegex() []string
	VerifyForBlockedWords(ctx context.Context, key string, val interface{}, realSqr SqlMakerI) (err error)
	GetMakeJsonArrayFn() (string, error)
	GetMakeJsonArrayFnStr() (string, error)
	MakeJsonColumn(jsonField string, jsonKey string) string
	ExtractTableNames(ctx context.Context, query string) module_model.TablesInQuery
	ExtractDMLTargetTables(ctx context.Context, query string) []string
	DefaultSchemaName() string
	GetResultDataTypes(ctx context.Context) []ResultDataTypes
	GetCalcDialect(ctx context.Context) derived.Dialect
	RewriteDerivedFields(ctx context.Context, req DerivedRewriteRequest) (string, error)
}

// DerivedRewriteRequest is everything the SQL entry point needs to turn
// qualified references to derived fields into expressions.
type DerivedRewriteRequest struct {
	Query      string
	DataSource *module_model.DataSource
	Security   derived.SecuredSource
	Limits     module_model.DerivedFieldLimits
}

// RewriteDerivedFields is a no-op outside postgres: only PostgresSqlMaker can
// parse a statement, so derived fields in hand-written SQL are postgres-only.
// GraphQL mode has no such limit - it never parses SQL.
func (sqr *SqlMaker) RewriteDerivedFields(ctx context.Context, req DerivedRewriteRequest) (string, error) {
	if len(req.DataSource.DerivedFields) > 0 {
		logs.WithContext(ctx).Warn("derived fields in hand-written SQL are supported on postgres only - references will be passed through to the database")
	}
	return req.Query, nil
}

// GetCalcDialect returns the postgres baseline. Each maker overrides only its
// deltas - derived.Dialect has a postgres-shaped zero value.
func (sqr *SqlMaker) GetCalcDialect(ctx context.Context) derived.Dialect {
	return derived.PostgresDialect()
}

func (sqr *SqlMaker) GetBlockedWords() []string {
	return blockedWords
}
func (sqr *SqlMaker) CreateDatabase(ctx context.Context, databaseName string) error {
	return errors.New("CreateDatabase not implemented")
}
func (sqr *SqlMaker) GetBlockedRegex() []string {
	return blockedRegex
}

func (sqr *SqlMaker) GetMakeJsonArrayFn() (string, error) {
	return "", errors.New("GetMakeJsonArrayFn not implemented")
}
func (sqr *SqlMaker) GetMakeJsonArrayFnStr() (string, error) {
	return "", errors.New("GetMakeJsonArrayFnStr not implemented")
}
func (sqr *SqlMaker) MakeJsonColumn(jsonField string, jsonKey string) string {
	return ""
}
func (sqr *SqlMaker) DefaultSchemaName() string {
	return ""
}

func (sqr *SqlMaker) VerifyForBlockedWords(ctx context.Context, key string, valI interface{}, realSqr SqlMakerI) (err error) {
	valB, errB := json.Marshal(valI)
	if errB != nil {
		logs.WithContext(ctx).Error(errB.Error())
		return errB
	}
	vWords := strings.ToUpper(string(valB))
	for _, v := range realSqr.GetBlockedWords() {
		if strings.Contains(vWords, v) {
			err = errors.New(fmt.Sprint("Suspicious value found for ", key, " ", vWords, " ", v))
			logs.WithContext(ctx).Error(err.Error())
			return
		}

	}
	for _, v := range realSqr.GetBlockedRegex() {
		match, mErr := regexp.MatchString(v, vWords)
		if mErr != nil {
			err = mErr
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		if match {
			err = errors.New(fmt.Sprint("Suspicious value found for ", key, " ", vWords, " ", v))
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	}
	return nil
}

func (sqr *SqlMaker) GetReturnAlias(ctx context.Context) string {
	return sqr.MutationReturn.ReturnDocAlias
}

func (sqr *SqlMaker) GetPreparedQueryPlaceholder(ctx context.Context, rowCount int, colCount int, single bool) string {
	return strings.Repeat(" ? ", colCount*rowCount)
}

func (sqr *SqlMaker) MakeUpsertQuery(ctx context.Context, tableName string, insertCols []string, colsPlaceholder string, conflictCols []string, updateCols []string, action string, returningStr string) (string, error) {
	logs.WithContext(ctx).Debug("MakeUpsertQuery - Start")
	return "", errors.New("upsert not supported for this database dialect")
}

func buildUpsertSets(insertCols []string, conflictSet map[string]bool, updateCols []string, formatSet func(col string) string) []string {
	insertSet := make(map[string]bool)
	for _, c := range insertCols {
		cc := strings.TrimSpace(c)
		if cc != "" {
			insertSet[cc] = true
		}
	}
	var sets []string
	if len(updateCols) > 0 {
		seen := make(map[string]bool)
		for _, c := range updateCols {
			cc := strings.TrimSpace(c)
			if cc == "" || conflictSet[cc] || !insertSet[cc] || seen[cc] {
				continue
			}
			seen[cc] = true
			sets = append(sets, formatSet(cc))
		}
		return sets
	}
	for _, c := range insertCols {
		cc := strings.TrimSpace(c)
		if cc == "" || conflictSet[cc] {
			continue
		}
		sets = append(sets, formatSet(cc))
	}
	return sets
}

func (sqr *SqlMaker) GetBaseSqlMaker(ctx context.Context) *SqlMaker {
	return sqr
}
func (sqr *SqlMaker) GetResultDataTypes(ctx context.Context) []ResultDataTypes {
	return sqr.ResultDataTypes
}
func (sqr *SqlMaker) GetTableMetaDataSQL(ctx context.Context, tableName string) string {
	return ""
}

func (sqr *SqlMaker) SaveTable(ctx context.Context, tableName string, tableStructure common_types.TableStructure, isEdit bool, dataSource *module_model.DataSource) (err error) {
	return nil
}
func (sqr *SqlMaker) DropTable(ctx context.Context, tableName string, dataSource *module_model.DataSource) (err error) {
	query, err := sqr.MakeDropTableSQL(ctx, tableName)
	if err != nil {
		return err
	}
	_, err = sqr.ExecutePreparedQuery(ctx, query, dataSource)
	if err != nil {
		return err
	}
	return nil
}

func (sqr *SqlMaker) MakeCreateTableSQL(ctx context.Context, tableName string, tableObj map[string]common_types.TableColsMetaData) (string, error) {
	return "", nil
}
func (sqr *SqlMaker) MakeDropTableSQL(ctx context.Context, tableName string) (string, error) {
	logs.WithContext(ctx).Debug("MakeDropTableSQL - Start")
	return fmt.Sprint("drop table ", tableName), nil
}

func (sqr *SqlMaker) GetSqlResult(ctx context.Context) map[string]interface{} {
	return sqr.result
}

func (sqr *SqlMaker) CreateConn(ctx context.Context, dataSource *module_model.DataSource) error {
	return errors.New("CreateConn not implemented")
}

func (sqr *SqlMaker) ConnectReadReplica(ctx context.Context, dataSource *module_model.DataSource, idx int) error {
	return errors.New("ConnectReadReplica not implemented")

}

func (sqr *SqlMaker) CheckMe(ctx context.Context) {
	logs.WithContext(ctx).Info("I am SqlMaker")
}

func (sqr *SqlMaker) ExtractTableNames(ctx context.Context, query string) module_model.TablesInQuery {
	logs.WithContext(ctx).Error("ExtractTableNames Not Implemented")
	return module_model.TablesInQuery{}
}

func (sqr *SqlMaker) ExtractDMLTargetTables(ctx context.Context, query string) []string {
	return nil
}

// extractTargetsFromSQLs parses each executed SQL via the dialect's
// ExtractDMLTargetTables and returns the deduplicated union. Used by
// mutation paths so cache invalidation matches what the SQL DML path does.
func extractTargetsFromSQLs(ctx context.Context, myself SqlMakerI, sqls []string) []string {
	if len(sqls) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, q := range sqls {
		for _, t := range myself.ExtractDMLTargetTables(ctx, q) {
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

/*
func (sqr *SqlMaker) processMutationDoc(d interface{}, myself SqlMakerI, datasource *module_model.DataSource, parentTableName string) (mr []MutationRecord, e error) {
	docs, err := d.([]interface{})
	if !err {
		_, er := d.(map[string]interface{}) // checking if docs is a single document without array
		if !er {
			return nil, errors.New("error while parsing value of 'docs'")
		}
		docs = append(docs, d)
	} else if sqr.QueryType == "update" {
		return nil, errors.New("value of 'docs' cannot be an array")
	}

	mr = make([]MutationRecord, len(docs))
	for i, doc := range docs {
		var cols []string
		var updateCols []string
		var colsPlaceholder []string
		var values []interface{}
		var childRecords []MutationRecord
		insertDoc, err := doc.(map[string]interface{})
		if !err {
			return nil, errors.New(fmt.Sprintf("error while parsing document at index ", i))
		}
		ii := 1
		mr[i] = MutationRecord{}
		for k, kv := range insertDoc {
			childTableName := strings.Replace(k, "___", ".", -1)
			a1, b1 := kv.([]interface{})          //check if value is array of nested object
			a2, b2 := kv.(map[string]interface{}) // check if value is single nested object (not an array)
			if b2 {
				a1 = append(a1, a2) // making single nested object into array of one element - to have common code to execute both conditions
				b1 = true
			}
			if b1 {
				tj, e := datasource.GetTableJoins(parentTableName, childTableName)
				//TODO ensure parent query has returning fields required for child join values
				if e != nil {
					return nil, e
				}
				childRecords, e = sqr.processMutationDoc(a1, myself, datasource, childTableName)
				if e != nil {
					logs.WithContext(ctx).Info(e.Error())
					return nil, e
				}
				if mr[i].childRecords == nil {
					mr[i].childRecords = make(map[string][]MutationRecord)
				}
				mr[i].childRecords[childTableName] = childRecords
				if mr[i].tableJoins == nil {
					mr[i].tableJoins = make(map[string]module_model.TableJoins)
				}
				mr[i].tableJoins[childTableName] = tj
			} else {
				cols = append(cols, k)
				updateCols = append(updateCols, fmt.Sprint(k, " = ", myself.GetPreparedQueryPlaceholder(ii)))
				colsPlaceholder = append(colsPlaceholder, myself.GetPreparedQueryPlaceholder(ii))
				values = append(values, kv)
				ii++
			}
		}
		mr[i].cols = strings.Join(cols, ",")
		mr[i].updatedCols = strings.Join(updateCols, ",")
		mr[i].colsPlaceholder = strings.Join(colsPlaceholder, ",")
		mr[i].values = values
	}
	return mr, nil
}

func (sqr *SqlMaker) ProcessMutationGraphQL(sel ast.Selection, vars map[string]interface{}, myself SqlMakerI, datasource *module_model.DataSource, singleTxn bool, openTxn bool, closeTxn bool, query string, cols string) (returnResult map[string]interface{}, err error) {
	//myself.CheckMe()
	logs.WithContext(ctx).Info("inside ProcessMutationGraphQL")
	sqr.SingleTxn = singleTxn
	sqr.openTxn = openTxn
	sqr.closeTxn = closeTxn
	field := sel.(*ast.Field)
	docsFound := false
	errMsg := "-"
	tempStr := strings.SplitN(field.Name.Value, "_", 2)
	sqr.QueryType = tempStr[0]
	sqr.MainTableName = strings.Replace(tempStr[1], "___", ".", -1)
	sqr.TxnFlag = true //default to true

	// resetting below variables as it is called in loop and
	// in case of SingleTxn, same sqr object is used for all records in loop
	sqr.MutationSelectQuery = ""
	sqr.MutationSelectCols = ""
	sqr.MutationRecords = nil
	for _, ff := range field.Arguments {
		switch ff.Name.Value {

		case "docs":
			docsFound = true
			varValue, e := ParseAstValue(ff.Value, vars)
			if e != nil {
				return nil, e
			}
			sqr.MutationRecords, err = sqr.processMutationDoc(varValue, myself, datasource, sqr.MainTableName)
			if err != nil {
				errMsg = err.Error()
				// return nil, err
				// no need to return here
			}
		case "query":
			//varValue, err := ParseAstValue(ff.Value, vars)
			//if err != nil {
			//	return nil, err
			//}

			sqr.MutationSelectQuery = query
			sqr.MutationSelectCols = cols
			docsFound = true
		case "txn":
			if !sqr.SingleTxn { //ignore txn flag for each query if singleTxn directive exists
				varValue, err := ParseAstValue(ff.Value, vars)
				if err != nil {
					return nil, err
				}
				v, e := varValue.(bool)
				if !e {
					return nil, errors.New("error while parsing value of 'txn'")
				}
				sqr.TxnFlag = v
			}
		case "where":
			v, e := ParseAstValue(ff.Value, vars)
			_ = v
			if e != nil {
				// TODO: return returnresult error
				return nil, e
			}
			///////////////////////////////////// TODO to open up wc, _ := sqr.processWhereClause(v, "", false)
			///////////////////////////////////// TODO sqr.WhereClause = fmt.Sprint(" where ", wc)
		default:
			//do nothing
		}
	}
	logs.WithContext(ctx).Info(sqr.MutationSelectQuery)
	if sqr.QueryType == "delete" {
		sqr.MutationRecords = make([]MutationRecord, 1) // dummy record added so that it enters for loop in ExecuteMutationQuery function
	} else if !docsFound {
		return nil, errors.New("missing 'docs' keyword - document to mutate not found") //TODO this error is not returned in graphql error
	}

	sqr.MutationReturn.ReturnError = false  //default to false
	sqr.MutationReturn.ReturnDoc = false    //default to false
	sqr.MutationReturn.ReturnFields = " * " //set default to * - below code will override it if user has asked for specific fields only
	for _, ss := range field.SelectionSet.Selections {
		field := ss.(*ast.Field)
		switch field.Name.Value {
		case "error":
			sqr.MutationReturn.ReturnError = true
			if field.Alias != nil {
				sqr.MutationReturn.ReturnErrorAlias = field.Alias.Value
			} else {
				sqr.MutationReturn.ReturnErrorAlias = field.Name.Value
			}
		case "returning":
			sqr.MutationReturn.ReturnDoc = true
			if field.Alias != nil {
				sqr.MutationReturn.ReturnDocAlias = field.Alias.Value
			} else {
				sqr.MutationReturn.ReturnDocAlias = field.Name.Value
			}
			retField := ss.(*ast.Field)
			//TODO to handle nested returning clause - to pass in insert query only fields of respective object.
			// Currently all fields are passing causing sql to fail
			if retField.SelectionSet != nil {
				var retFieldArray []string
				for _, rf := range retField.SelectionSet.Selections {
					rfNew := rf.(*ast.Field)
					retFieldArray = append(retFieldArray, rfNew.Name.Value)
				}
				sqr.MutationReturn.ReturnFields = strings.Join(retFieldArray, ",")
			}
			//else {
			//	sqr.MutationReturn.ReturnFields = " * "
			//}
		default:
			//do nothing
		}
	}
	res, er := sqr.ExecuteMutationQuery(datasource, myself)

	if er != nil {
		logs.WithContext(ctx).Info(er.Error())
		errMsg = er.Error()
		//return nil, er
		// no need to return here - error is returned as part of result - if asked in the query.
	}
	returnResult = make(map[string]interface{})
	if sqr.MutationReturn.ReturnError {
		returnResult[sqr.MutationReturn.ReturnErrorAlias] = errMsg
	}
	if sqr.MutationReturn.ReturnDoc {
		returnResult[sqr.MutationReturn.ReturnDocAlias] = res
	}
	//TODO : query alias to be returned for mutation
	return returnResult, er
}
*/
/*
func (sqr *SqlMaker) ProcessGraphQL(sel ast.Selection, vars map[string]interface{}, myself SqlMakerI, datasource *module_model.DataSource, executeFlag bool) (res map[string]interface{}, query string, cols string, err error) {
	//myself.CheckMe()
	field := sel.(*ast.Field)

	//logs.WithContext(ctx).Info("field.GetKind() = " + field.GetKind())
	//logs.WithContext(ctx).Info("field.Name.Value = " + field.Name.Value)
	sqr.MainTableName = strings.Replace(field.Name.Value, "___", ".", -1)
	if field.Alias != nil {
		sqr.MainAliasName = field.Alias.Value
	} else {
		sqr.MainAliasName = sqr.MainTableName
	}
	//sqr.AllTableNames = append(sqr.AllTableNames, field.Name.Value)
	//sqr.AllTableNamesNew = append(sqr.AllTableNamesNew,[]tablesInQuery{})
	//sqr.graphqlResult.gqlr = make(map[string]interface{})
	sqr.MainTableDB = field.Directives[0].Name.Value
	______________________________________________________________________
	we will need below block for tenant ds alias
	   logs.WithContext(ctx).Info("field.Directives[0].Name.Value = " + field.Directives[0].Name.Value)
	   	logs.WithContext(ctx).Info("loop on field.Directives[0].Arguments starts")
	   	for _, vv := range field.Directives[0].Arguments {
	   		logs.WithContext(ctx).Info("vv.Name.Value = "+vv.Name.Value)
	   		logs.WithContext(ctx).Info("vv.Value.GetValue().(string)" + vv.Value.GetValue().(string))
	   	}
	   	logs.WithContext(ctx).Info("loop on field.Directives[0].Arguments ends")
	______________________________________________________________________

	//logs.WithContext(ctx).Info("len(field.Arguments) = " + string(len(field.Arguments)))
	for _, ff := range field.Arguments { //TODO to add join to main table without having to add
		switch ff.Name.Value {
		case "where":
			v, _ := ParseAstValue(ff.Value, vars)
			wc, _ := sqr.processWhereClause(v, "", false)
			//logs.WithContext(ctx).Info("final where clause = " + wc)
			sqr.WhereClause = fmt.Sprint(" where ", wc)
		case "sort":
			sqr.processSortClause(ff.Value, vars)
		case "distinct":
			if ff.Value.GetKind() != kinds.BooleanValue {
				return nil, "", "", errors.New("Non Boolean value received - distinct clause need integer value")
			}
			v, _ := ParseAstValue(ff.Value, vars)
			sqr.DistinctResults = v.(bool)
		case "limit": //TODO to handle if variable not found
			v, _ := ParseAstValue(ff.Value, vars)
			if reflect.TypeOf(v).Kind() == reflect.Float64 {
				v = int(v.(float64))
			}
			if reflect.TypeOf(v).Kind() != reflect.Int {
				return nil, "", "" , errors.New("Non Integer value received - limit clause need integer value")
			}
			sqr.Limit = v.(int)
		case "skip":
			if ff.Value.GetKind() != kinds.IntValue {
				return nil, "", "", errors.New("Non Integer value received - skip clause need integer value")
			}
			v, _ := ParseAstValue(ff.Value, vars)
			sqr.Skip = v.(int)
		default:
		}
	}
	//logs.WithContext(ctx).Info("loop on field.Arguments ends")
	//logs.WithContext(ctx).Info("len(field.SelectionSet.Selections) = "+string(len(field.SelectionSet.Selections)))
	if field.SelectionSet == nil {
		var tmpSelSet []ast.Selection
		sqr.ColumnList, sqr.DBColumns, sqr.GroupList, _ = sqr.processColumnList(tmpSelSet, sqr.MainTableName, vars, 0, 0, datasource)
		sqr.ColumnList = " * "
	} else {
		sqr.ColumnList, sqr.DBColumns, sqr.GroupList, _ = sqr.processColumnList(field.SelectionSet.Selections, sqr.MainTableName, vars, 0, 0, datasource)
	}
	query, errStr := sqr.MakeQuery()
	if errStr != "" {
		return nil, "", "", errors.New(errStr)
	}
	query = myself.AddLimitSkipClause(query, sqr.Limit, sqr.Skip, 1000)
	if executeFlag {
		res, err = myself.ExecuteQuery(query, datasource)
	}
	return res, query, sqr.DBColumns, err
}
*/

func (sqr *SqlMaker) ExecuteQueryForCsv(ctx context.Context, query string, datasource *module_model.DataSource, aliasName string, myself SqlMakerI) (res map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ExecuteQueryForCsv - Start")
	con, replica, rerr := ReadCon(ctx, datasource, RouterHint{})
	if rerr != nil {
		return nil, logs.Err(ctx, rerr, "")
	}
	rows, e := con.Queryx(query)
	if e != nil {
		MarkReadFailed(ctx, replica, e)
		e = logs.Err(ctx, e, "")
		return nil, e
	}
	defer rows.Close()

	mapping := make(map[string]interface{})
	colsType, ee := rows.ColumnTypes()
	if ee != nil {
		ee = logs.Err(ctx, ee, "")
		return nil, ee
	}
	sqr.result = make(map[string]interface{})
	var innerResult [][]interface{}
	firstRow := true
	var innerResultDataTypes []ResultDataTypes
	var innerResultDataTypesMap = make(map[string]ResultDataTypes)
	for rows.Next() {
		var innerResultRow []interface{}
		var innerResultLabel []interface{}

		ee = rows.MapScan(mapping)
		if ee != nil {
			ee = logs.Err(ctx, ee, "")
			return nil, ee
		}
		for _, colType := range colsType {
			if firstRow {
				colHeader := colType.Name()
				colHeaderArray := strings.Split(colType.Name(), "**")
				if len(colHeaderArray) > 1 {
					colHeader = colHeaderArray[1]
				}
				colHeaderArray = strings.Split(colHeader, "~")
				dt := colType.DatabaseTypeName()
				if len(colHeaderArray) > 1 {
					colType.DatabaseTypeName()
					colHeader = colHeaderArray[0]
					dt = colHeaderArray[1]
				}
				innerResultLabel = append(innerResultLabel, colHeader)
				ct := ""
				if mapping[colType.Name()] != nil {
					ct = reflect.TypeOf(mapping[colType.Name()]).String()
				}
				if dt == "b64" {
					innerResultDataTypesMap[colType.Name()] = ResultDataTypes{ColName: colHeader, ColType: ct, ColDatabaseTypeName: dt}
					dt = colType.DatabaseTypeName()
				} else {
					innerResultDataTypesMap[colType.Name()] = ResultDataTypes{ColName: colHeader, ColType: ct, ColDatabaseTypeName: myself.getDataTypeMapping(ctx, dt)}
				}
				innerResultDataTypes = append(innerResultDataTypes, ResultDataTypes{ColName: colHeader, ColType: ct, ColDatabaseTypeName: myself.getDataTypeMapping(ctx, dt)})

			}
			if mapping[colType.Name()] != nil {
				if reflect.TypeOf(mapping[colType.Name()]).String() == "[]uint8" && colType.DatabaseTypeName() == "NUMERIC" {
					f := 0.0
					f, err = strconv.ParseFloat(string(mapping[colType.Name()].([]byte)), 64)
					mapping[colType.Name()] = strconv.FormatFloat(f, 'f', -1, 64)
					if err != nil {
						err = logs.Err(ctx, err, "")
						return nil, ee
					}
				} else if reflect.TypeOf(mapping[colType.Name()]).String() == "float64" {
					f := 0.0
					f = mapping[colType.Name()].(float64)
					mapping[colType.Name()] = strconv.FormatFloat(f, 'E', -1, 64)
				} else if reflect.TypeOf(mapping[colType.Name()]).String() == "bool" {
					mapping[colType.Name()] = strconv.FormatBool(mapping[colType.Name()].(bool))
				} else if reflect.TypeOf(mapping[colType.Name()]).String() == "time.Time" {
					if colType.DatabaseTypeName() == "DATE" {
						mapping[colType.Name()] = mapping[colType.Name()].(time.Time).Format("02-Jan-2006")
					} else {
						mapping[colType.Name()] = mapping[colType.Name()].(time.Time).String()
					}
				} else if strings.HasPrefix(reflect.TypeOf(mapping[colType.Name()]).String(), "int") {
					//if reflect.TypeOf(mapping[colType.Name()]).String() == "int64" {
					n := mapping[colType.Name()].(int64)
					mapping[colType.Name()] = fmt.Sprintf("%d", n)
					//}
				} else if innerResultDataTypesMap[colType.Name()].ColDatabaseTypeName == "b64" {
					strV, err := b64.StdEncoding.DecodeString(mapping[colType.Name()].(string))
					if err != nil {
						err = logs.Err(ctx, err, "")
						err = nil // silently proceed even if base64 decoding fails
					} else {
						mapping[colType.Name()] = string(strV)
					}
				} else if innerResultDataTypesMap[colType.Name()].ColDatabaseTypeName == "JSON" {
					var v interface{}
					strValue, strValueOk := mapping[colType.Name()].(string)
					if strValueOk {
						err = json.Unmarshal([]byte(strValue), &v)
						if err != nil {
							err = logs.Err(ctx, err, "")
							return nil, err
						}
						var newV []string
						if reflect.TypeOf(v).Kind() == reflect.Slice {
							for _, vv := range v.([]interface{}) {
								strVV, err := json.Marshal(vv)
								if err != nil {
									err = logs.Err(ctx, err, "")
									return nil, err
								}
								uqStrVV, err := strconv.Unquote(string(strVV))
								if err != nil {
									uqStrVV = string(strVV)
								}
								if uqStrVV == "null" {
									uqStrVV = ""
								}
								newV = append(newV, uqStrVV)
							}
							mapping[colType.Name()] = strings.Join(newV, ",")
						}
					}
				} else if (colType.DatabaseTypeName() == "JSONB" || colType.DatabaseTypeName() == "JSON") || colType.DatabaseTypeName() == "BPCHAR" {
					mapping[colType.Name()] = string(mapping[colType.Name()].([]byte))
				}
			}

			if val, ok := mapping[colType.Name()].(string); ok {
				innerResultRow = append(innerResultRow, val)
			} else if mapping[colType.Name()] == nil {
				innerResultRow = append(innerResultRow, "")
			} else if mapBytes, mapErr := json.Marshal(mapping[colType.Name()]); mapErr == nil {
				innerResultRow = append(innerResultRow, string(mapBytes))
			} else {
				err = errors.New(fmt.Sprint("value of ", colType.Name(), " is not a string"))
				err = logs.Err(ctx, err, "")
				return nil, err
			}
		}
		if firstRow {
			innerResult = append(innerResult, innerResultLabel)
			firstRow = false
		}
		innerResult = append(innerResult, innerResultRow)
	}
	if len(innerResult) == 0 {
		innerResult = append(innerResult, []interface{}{})
	}
	sqr.result[aliasName] = innerResult
	sqr.ResultDataTypes = innerResultDataTypes
	return sqr.result, nil
}

func (sqr *SqlMaker) ExecutePreparedQuery(ctx context.Context, query string, datasource *module_model.DataSource) (res map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ExecutePreparedQuery - Start")
	logs.WithContext(ctx).Info(query)
	con, replica, rerr := ReadCon(ctx, datasource, RouterHint{})
	if rerr != nil {
		return nil, logs.Err(ctx, rerr, "")
	}
	rows, e := con.Queryx(query)
	if e != nil {
		MarkReadFailed(ctx, replica, e)
		e = logs.Err(ctx, e, "")
		return nil, e
	}
	defer rows.Close()

	mapping := make(map[string]interface{})
	colsType, ee := rows.ColumnTypes()
	if ee != nil {
		ee = logs.Err(ctx, ee, "")
		return nil, ee
	}
	sqr.result = make(map[string]interface{})
	var innerResult []map[string]interface{}
	for rows.Next() {
		innerResultRow := make(map[string]interface{})
		ee = rows.MapScan(mapping)
		if ee != nil {
			ee = logs.Err(ctx, ee, "")
			return nil, ee
		}
		for _, colType := range colsType {
			if colType.DatabaseTypeName() == "NUMERIC" && mapping[colType.Name()] != nil {
				f := 0.0
				if reflect.TypeOf(mapping[colType.Name()]).String() == "[]uint8" {
					f, err = strconv.ParseFloat(string(mapping[colType.Name()].([]byte)), 64)
					mapping[colType.Name()] = f
				} else if reflect.TypeOf(mapping[colType.Name()]).String() == "float64" {
					f = mapping[colType.Name()].(float64)
					mapping[colType.Name()] = f
				}
				if err != nil {
					err = logs.Err(ctx, err, "")
					return nil, err
				}
			} else if (colType.DatabaseTypeName() == "JSONB" || colType.DatabaseTypeName() == "JSON") && mapping[colType.Name()] != nil {
				var bytesToUnmarshal []byte
				switch mv := mapping[colType.Name()].(type) {
				case []byte:
					bytesToUnmarshal = mv
				case string:
					bytesToUnmarshal = []byte(mv)
				}
				if bytesToUnmarshal != nil {
					var v interface{}
					err = json.Unmarshal(bytesToUnmarshal, &v)
					if err != nil {
						err = logs.Err(ctx, err, "")
						return nil, err
					}
					mapping[colType.Name()] = &v
				}
			}
			innerResultRow[colType.Name()] = mapping[colType.Name()]
			//innerResult[colType.Name()+"_DT"] = reflect.TypeOf(r).Kind().String()
		}
		innerResult = append(innerResult, innerResultRow)
	}
	if len(innerResult) == 0 {
		innerResult = append(innerResult, make(map[string]interface{}))
	}
	sqr.result["Results"] = innerResult

	return sqr.result, nil

}

func (sqr *SqlMaker) ExecuteMutationQuery(ctx context.Context, datasource *module_model.DataSource, myself SqlMakerI, mrm module_model.MutationResultMaker) (res []map[string]interface{}, err error) {
	//TODO single column with same values without distinct flag returns only one row - behaves like distinct
	logs.WithContext(ctx).Debug("ExecuteMutationQuery - Start")

	sqr.MainTableName = mrm.MainTableName
	sqr.MainAliasName = mrm.MainAliasName
	sqr.IsNested = mrm.IsNested
	sqr.MutationRecords = mrm.MutationRecords
	sqr.MutationReturn = mrm.MutationReturn
	sqr.SingleTxn = mrm.SingleTxn
	sqr.openTxn = mrm.OpenTxn
	sqr.closeTxn = mrm.CloseTxn
	sqr.TxnFlag = mrm.TxnFlag
	sqr.QueryType = mrm.QueryType
	sqr.DBQuery = mrm.DBQuery
	sqr.PreparedQuery = mrm.PreparedQuery
	var errMsgs []string
	//ctx, cancel := context.WithTimeout(context.Background(), 100000*time.Millisecond) //TODO: to get context as argument
	//defer cancel()
	if sqr.openTxn || (sqr.TxnFlag && !sqr.SingleTxn) {
		writeCon, werr := WriteCon(ctx, datasource)
		if werr != nil {
			return nil, logs.Err(ctx, werr, "")
		}
		logs.WithContext(ctx).Debug("datasource.Con.MustBegin() called in ExecuteMutationQuery")
		sqr.tx = writeCon.MustBegin() //begin txn only once for all queries OR begin txn outside for loop to insert all docs as single txn
	}
	if len(sqr.MutationRecords) > 0 || sqr.QueryType == "insertselect" || sqr.QueryType == "delete" || sqr.PreparedQuery {
		res, err = sqr.iterateDocsForMutation(ctx, sqr.MutationRecords, sqr.MainTableName, datasource, myself, false, -1)
		if err != nil {
			err = logs.Err(ctx, err, "")
			errMsgs = append(errMsgs, err.Error())
		}
	}
	if sqr.closeTxn || (sqr.TxnFlag && !sqr.SingleTxn) {
		logs.WithContext(ctx).Info("sqr.tx.Commit() called in ExecuteMutationQuery")
		err = sqr.tx.Commit()
		if err != nil {
			err = logs.Err(ctx, err, "")
			errMsgs = append(errMsgs, fmt.Sprint("DB error :", err.Error()))
			sqr.tx.Rollback()
		} else {
			qlcache.EnqueueInvalidate(ctx, datasource, qlcache.QualifyTables(extractTargetsFromSQLs(ctx, myself, sqr.executedSQLs), myself.DefaultSchemaName()))
		}
	}
	if len(errMsgs) > 0 {
		if sqr.TxnFlag {
			res = make([]map[string]interface{}, 0)
		}
		err = logs.Err(ctx, errors.New(strings.Join(errMsgs, " , ")), "")
		return res, err
	}
	return res, nil
}

func (sqr *SqlMaker) RollbackQuery(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("RollbackQuery - Start")
	if sqr.tx != nil {
		err = sqr.tx.Rollback()
		if err != nil {
			err = logs.Err(ctx, err, "")
		}
	}
	return err
}

func (sqr *SqlMaker) iterateDocsForMutation(ctx context.Context, docs []module_model.MutationRecord, tableName string, datasource *module_model.DataSource, myself SqlMakerI, isNested bool, docNo int) (res []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("iterateDocsForMutation - Start")
	var errMsgs []string
	var finalValues []interface{}
	query := ""
	logs.WithContext(ctx).Info(fmt.Sprint("sqr.IsNested = ", sqr.IsNested))
	if !sqr.IsNested {
		if sqr.QueryType == "insertselect" || sqr.QueryType == "delete" || sqr.PreparedQuery {
			query = sqr.DBQuery
		} else if sqr.QueryType == "update" {
			finalValues = docs[0].Values
			query = sqr.MutationRecords[0].DBQuery
			for i, _ := range sqr.MutationRecords[0].UpdatedCols {
				query = strings.Replace(query, fmt.Sprint("$UpdateColPlaceholder", i), myself.GetPreparedQueryPlaceholder(ctx, 1, i, true), 1)
			}
		} else {
			//TODO to handle if sqr.MutationRecords is nil - one of the reason it is passed as nil is when table or table join is not found
			for _, d := range docs {
				finalValues = append(finalValues, d.NonNestedValues...)
			}
			query = strings.Replace(sqr.MutationRecords[0].DBQuery, "$ColsPlaceholder", myself.GetPreparedQueryPlaceholder(ctx, len(sqr.MutationRecords), len(sqr.MutationRecords[0].Values), false), 1)
		}
		res, err = sqr.executeMutationQueriesinDB(ctx, query, tableName, datasource, myself, isNested, docNo, 0, finalValues)
		if err != nil {
			err = logs.Err(ctx, err, "")
			errMsgs = append(errMsgs, err.Error())
			return res, errors.New(strings.Join(errMsgs, " , "))
		}
		sqr.executedSQLs = append(sqr.executedSQLs, query)
	} else {

		for i, v := range docs {
			query = strings.Replace(v.DBQuery, "$ColsPlaceholder", myself.GetPreparedQueryPlaceholder(ctx, 1, len(v.Values), false), 1)
			resDocs, err := sqr.executeMutationQueriesinDB(ctx, query, tableName, datasource, myself, isNested, docNo, i, v.Values)
			if err != nil {
				err = logs.Err(ctx, err, "")
				errMsgs = append(errMsgs, err.Error())
				return res, errors.New(strings.Join(errMsgs, " , "))
			}
			sqr.executedSQLs = append(sqr.executedSQLs, query)
			if sqr.QueryType == "insert" && len(resDocs) > 0 {
				resDoc := resDocs[0]
				var childError bool
				for ck, cv := range v.ChildRecords {
					childError = false
					tj := v.TableJoins[ck]
					var colsPlaceholder []int
					//var values []interface{}
					//for ii := 0; ii < len(tj.Table1Cols); ii++ {
					//	cols = append(cols, tj.Table1Cols[ii])
					//	colsPlaceholder = append(colsPlaceholder, 999+ii)
					//	values = append(values, resDoc[tj.Table1Cols[ii]])
					//}
					for icv, childRec := range cv {
						cvColsArray := strings.Split(childRec.Cols, ",")
						for _, tjColVal := range tj.Table1Cols { //TODO test parent-child insertion extensively.
							colFound := false
							for colIdx, colVal := range cvColsArray {
								if tjColVal == colVal {
									colFound = true
									childRec.Values[colIdx] = resDoc[tjColVal]
									break
								}
							}
							if !colFound {
								cvColsArray = append(cvColsArray, tjColVal)
								//childRec.Values = append(childRec.Values, resDoc[tjColVal])
								cv[icv].Values = append(cv[icv].Values, resDoc[tjColVal])
								colsPlaceholder = append(colsPlaceholder, len(cvColsArray))
							}
						}
						cv[icv].Cols = strings.Join(cvColsArray, ",")
						/* to check if below block is needed
						var colsPlaceholderStr []string

						for iiii := 0; iiii < len(colsPlaceholder); iiii++ {
							//colsPlaceholderStr = append(colsPlaceholderStr, myself.GetPreparedQueryPlaceholder(colsPlaceholder[iiii]-999+len(strings.Split(cv[iii].colsPlaceholder, ","))+1))
							colsPlaceholderStr = append(colsPlaceholderStr, myself.GetPreparedQueryPlaceholder(colsPlaceholder[iiii]))
						}
						childRec.colsPlaceholder = fmt.Sprint(childRec.colsPlaceholder, " , ", strings.Join(colsPlaceholderStr, ","))
						//cv[iii].values = append(cv[iii].values, values...)
						*/
					}

					resDoc[ck], err = sqr.iterateDocsForMutation(ctx, cv, ck, datasource, myself, true, docNo)
					if err != nil {
						childError = true
						err = logs.Err(ctx, err, "")
						errMsgs = append(errMsgs, err.Error())
						return res, errors.New(strings.Join(errMsgs, " , "))
					}
				}
				if !childError {
					res = append(res, resDoc) //remember parent result only if there are no errors in child/nested records
				} else {
					res = append(res, make(map[string]interface{}))
				}
			} else {
				res = resDocs
			}
		}
	}
	if len(errMsgs) > 0 {
		return res, errors.New(strings.Join(errMsgs, " , "))
	}
	return res, nil
}

func (sqr *SqlMaker) executeMutationQueriesinDB(ctx context.Context, query string, tableName string, datasource *module_model.DataSource, myself SqlMakerI, isNested bool, docNo int, idx int, vals []interface{}) (res []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("executeMutationQueriesinDB - Start")
	if len(query) > 1000 {
		logs.WithContext(ctx).Info(query[:1000])
	} else {
		logs.WithContext(ctx).Info(query)
	}
	var errMsgs []string
	if !isNested {
		docNo = idx + 1 //to ensure error message always refer to parent document number
	}
	errFound := false
	if !sqr.TxnFlag && !isNested {
		writeCon, werr := WriteCon(ctx, datasource)
		if werr != nil {
			return nil, logs.Err(ctx, werr, "")
		}
		logs.WithContext(ctx).Info("datasource.Con.MustBegin() called")
		sqr.tx = writeCon.MustBegin() // begin txn inside for loop to insert every doc as seperate txn
	}
	stmt, err := sqr.tx.PreparexContext(ctx, query) // TODO: to fetch con after locking
	if err != nil {
		err = logs.Err(ctx, err, "")
		errFound = true
		errMsgs = append(errMsgs, fmt.Sprint("DB error for Document No ", docNo, " : ", err.Error()))
		sqr.tx.Rollback()
		if sqr.TxnFlag || isNested {
			res = make([]map[string]interface{}, 0)
			return nil, errors.New(strings.Join(errMsgs, " , "))
		}
		//return nil, errors.New(strings.Join(errMsgs, " , "))
	}
	if !errFound {
		/*if sqr.QueryType == "insert" {
			resDoc := make(map[string]interface{})
			rw := stmt.QueryRowxContext(ctx, vals...).MapScan(resDoc)

			//rw, ee := stmt.QueryxContext(ctx, v.values...)
			if rw != nil {
				if rw.Error() != "" {
					errMsgs = append(errMsgs, fmt.Sprint("DB error for Document No ", docNo, " : ", rw.Error()))
					sqr.tx.Rollback()
					if sqr.TxnFlag || isNested {
						res = make([]map[string]interface{}, 0)
						return nil, errors.New(strings.Join(errMsgs, " , "))
					}
				}
			}

			for k, v := range resDoc {
				if v != nil {
					if reflect.TypeOf(v).String() == "[]uint8" {
						logs.WithContext(ctx).Info("string(v.([]byte) == ", string(v.([]byte)))
						f, err := strconv.ParseFloat(string(v.([]byte)), 64)
						if err != nil {
							logs.WithContext(ctx).Info(err.Error(), " : ", k, " : ", v)
							resDoc[k] = string(v.([]byte))
						} else {
							resDoc[k] = f
						}
					}
				}
			}
			res = append(res, resDoc)
		} else {

		*/
		rw, ee := stmt.QueryxContext(ctx, vals...)
		if ee != nil {
			ee = logs.Err(ctx, ee, "")
			errMsgs = append(errMsgs, fmt.Sprint("DB error for Document No ", docNo, " : ", ee.Error()))
			logs.WithContext(ctx).Error(strings.Join(errMsgs, " , "))
			sqr.tx.Rollback()
			//if sqr.TxnFlag || isNested {
			return nil, errors.New(strings.Join(errMsgs, " , "))
			//}
		}
		for rw.Rows.Next() {
			resDoc := make(map[string]interface{})
			colsType, ee := rw.ColumnTypes()
			if ee != nil {
				ee = logs.Err(ctx, ee, "")
				return nil, ee
			}
			e := rw.MapScan(resDoc)
			if e != nil {
				e = logs.Err(ctx, e, "")
				return nil, e
			}

			for _, colType := range colsType {
				if colType.DatabaseTypeName() == "JSONB" || colType.DatabaseTypeName() == "JSON" {
					var tmpv interface{}
					json.Unmarshal(resDoc[colType.Name()].([]byte), &tmpv)
					resDoc[colType.Name()] = tmpv
				} else if colType.DatabaseTypeName() == "NUMERIC" && resDoc[colType.Name()] != nil {
					f, err := strconv.ParseFloat(string(resDoc[colType.Name()].([]byte)), 64)
					if err != nil {
						err = logs.Err(ctx, err, "")
						return nil, err
					}
					resDoc[colType.Name()] = f
				}
			}
			res = append(res, resDoc)
		}
	}
	if !sqr.TxnFlag && !isNested {
		logs.WithContext(ctx).Info("sqr.tx.Commit() called")
		err = sqr.tx.Commit()
		if err != nil {
			err = logs.Err(ctx, err, "")
			errMsgs = append(errMsgs, fmt.Sprint("DB error for Document No ", docNo, " : ", err.Error()))
			sqr.tx.Rollback()
		} else {
			qlcache.EnqueueInvalidate(ctx, datasource, qlcache.QualifyTables(extractTargetsFromSQLs(ctx, myself, sqr.executedSQLs), myself.DefaultSchemaName()))
			sqr.executedSQLs = nil
		}
	}
	return res, nil
}

func (sqr *SqlMaker) ExecuteQuery(ctx context.Context, datasource *module_model.DataSource, qrm module_model.QueryResultMaker) (res map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ExecuteQuery - Start")

	sqr.queryLevel = qrm.QueryLevel
	sqr.querySubLevel = qrm.QuerySubLevel
	sqr.tables = qrm.Tables
	sqr.MainAliasName = qrm.MainAliasName
	sqr.MainTableName = qrm.MainTableName

	con, replica, rerr := ReadCon(ctx, datasource, RouterHint{UseWriter: qrm.UseWriter})
	if rerr != nil {
		return nil, logs.Err(ctx, rerr, "")
	}
	rows, e := con.Queryx(qrm.SQLQuery)
	if e != nil {
		MarkReadFailed(ctx, replica, e)
		e = logs.Err(ctx, e, "")
		return nil, e
	}

	defer rows.Close()
	mapping := make(map[string]interface{})
	colsType, ee := rows.ColumnTypes()
	if ee != nil {
		ee = logs.Err(ctx, ee, "")
		return nil, ee
	}
	rowNo := 0
	sqr.result = make(map[string]interface{})
	for rows.Next() {
		rowNo = rowNo + 1
		resultRowHolderNew := make([][]map[string]interface{}, sqr.queryLevel+1)

		e = rows.MapScan(mapping)
		if e != nil {
			e = logs.Err(ctx, e, "")
			return nil, e
		}

		var colLevelI, colSubLevelI int
		var er error
		for _, colType := range colsType {
			colLevelStrArray := strings.Split(colType.Name(), "**")
			colLevelStr := "0~~0"
			cn := colType.Name()
			if len(colLevelStrArray) > 1 {
				colLevelStr = strings.Replace(colLevelStrArray[0], "L", "", 1)
				cn = strings.Split(colType.Name(), "**")[1]
			}
			colLevel := strings.Split(colLevelStr, "~~")[0]
			colSubLevel := ""

			if len(strings.Split(colLevelStr, "~~")) > 1 {
				colSubLevel = strings.Split(colLevelStr, "~~")[1]
				colSubLevelI, er = strconv.Atoi(colSubLevel)
			}
			colLevelI, er = strconv.Atoi(colLevel) //TODO to handle select * (no column names)
			if er != nil {
				return nil, er
			}

			//colLevelI = colLevelI - 1
			//colSubLevelI = colSubLevelI - 1

			//if resultRowHolder[colLevelI] == nil {
			//	resultRowHolder[colLevelI] = make(map[string]interface{})
			//}
			if resultRowHolderNew[colLevelI] == nil {
				resultRowHolderNew[colLevelI] = []map[string]interface{}{0: make(map[string]interface{})}
			} else if len(resultRowHolderNew[colLevelI])-1 < colSubLevelI {
				resultRowHolderNew[colLevelI] = append(resultRowHolderNew[colLevelI], make(map[string]interface{}))
			}

			actualColType := ""
			if mapping[colType.Name()] != nil {
				actualColType = reflect.TypeOf(mapping[colType.Name()]).String()
			}
			_ = actualColType

			// TODO : json boolean/numeric values are returned as TEXT by postgres - need to check if we can return BOOLEAN
			if colType.DatabaseTypeName() == "NUMERIC" && mapping[colType.Name()] != nil {
				f := 0.0
				if reflect.TypeOf(mapping[colType.Name()]).String() == "[]uint8" {
					f, err = strconv.ParseFloat(string(mapping[colType.Name()].([]byte)), 64)
					mapping[colType.Name()] = f
				} else if reflect.TypeOf(mapping[colType.Name()]).String() == "float64" {
					f = mapping[colType.Name()].(float64)
					mapping[colType.Name()] = f
				}
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return nil, err
				}
			} else if (colType.DatabaseTypeName() == "JSONB" || colType.DatabaseTypeName() == "JSON") && mapping[colType.Name()] != nil {
				var bytesToUnmarshal []byte
				switch mv := mapping[colType.Name()].(type) {
				case []byte:
					bytesToUnmarshal = mv
				case string:
					bytesToUnmarshal = []byte(mv)
				}
				if bytesToUnmarshal != nil {
					var v interface{}
					err = json.Unmarshal(bytesToUnmarshal, &v)
					if err != nil {
						return nil, err
					}
					mapping[colType.Name()] = &v
				}
			}
			resultRowHolderNew[colLevelI][colSubLevelI][cn] = mapping[colType.Name()]

			/*
				if actualColType == "[]uint8" && mapping[colType.Name()] != nil {
					f, err := strconv.ParseFloat(string(mapping[colType.Name()].([]byte)), 64)
					if err != nil {
						logs.WithContext(ctx).Info("error from strconv.ParseFloat")
						logs.WithContext(ctx).Error(err.Error())
						return nil, err
					}
					mapping[colType.Name()] = f
				}
				resultRowHolderNew[colLevelI][colSubLevelI][cn] = mapping[colType.Name()]
			*/

			//logs.WithContext(ctx).Info(resultRowHolderNew)
		}

		r, rf, err := sqr.processRows(ctx, resultRowHolderNew, 0, rowNo, true, -1) //sqr.result[sqr.MainTableName].([]interface{})
		if err != nil {
			err = logs.Err(ctx, err, "")
			return nil, err
		}
		mtn := strings.Replace(sqr.MainTableName, ".", "___", 1)
		man := strings.Replace(sqr.MainAliasName, ".", "___", 1)
		if !rf[mtn] {
			if sqr.result[man] == nil {
				sqr.result[man] = r[mtn]
			} else {
				sqr.result[man] = append(sqr.result[man].([]interface{}), r[mtn].([]interface{})...)
			}
		}

	}
	return sqr.result, nil
}

func (sqr *SqlMaker) processRows(ctx context.Context, vrh [][]map[string]interface{}, curLevel int, rowNo int, parentRecordFound bool, parentIndexNo int) (r map[string]interface{}, recf map[string]bool, err error) {
	logs.WithContext(ctx).Debug("processRows - Start")
	recordFound := false
	recf = make(map[string]bool)
	//r = append(r,[]interface{}{})
	r = make(map[string]interface{})

	for i := 0; i <= sqr.querySubLevel[curLevel]; i++ {
		curLevelValue := make(map[string]interface{})
		vrh[curLevel][i]["parentIndexNo"] = parentIndexNo
		v, e := json.Marshal(vrh[curLevel][i])
		if e != nil {
			e = logs.Err(ctx, e, "")
			return nil, nil, e
		}

		delete(vrh[curLevel][i], "parentIndexNo")
		var curLevelRecord map[string]interface{}
		indexNo := 1

		if len(sqr.resultHolderNew) <= curLevel {
			sqr.resultHolderNew = append(sqr.resultHolderNew, []map[string]interface{}{0: make(map[string]interface{})})
			sqr.resultIndexHolderNew = append(sqr.resultIndexHolderNew, []map[int]int{map[int]int{parentIndexNo: 0}})
		}
		if len(sqr.resultHolderNew[curLevel]) <= i {
			sqr.resultHolderNew[curLevel] = append(sqr.resultHolderNew[curLevel], make(map[string]interface{}))
			sqr.resultIndexHolderNew[curLevel] = append(sqr.resultIndexHolderNew[curLevel], map[int]int{parentIndexNo: 0})
		}
		if sqr.resultHolderNew[curLevel][i][string(v)] != nil {
			curLevelRecord = sqr.resultHolderNew[curLevel][i][string(v)].(map[string]interface{})
			if curLevelValue[sqr.tables[curLevel][i].Name] == nil {
				curLevelValue[sqr.tables[curLevel][i].Name] = []interface{}{curLevelRecord}
			} else {
				curLevelValue[sqr.tables[curLevel][i].Name] = append(curLevelValue[sqr.tables[curLevel][i].Name].([]interface{}), curLevelRecord)
			}
			if !parentRecordFound {
				recordFound = false
				sqr.resultIndexHolderNew[curLevel][i][parentIndexNo] = 1
				indexNo = sqr.resultIndexHolderNew[curLevel][i][parentIndexNo]
			} else {
				recordFound = true
				indexNo = curLevelRecord["rowIndex"].(int)
			}
		} else {
			recordFound = false
			if !parentRecordFound {
				sqr.resultIndexHolderNew[curLevel][i][parentIndexNo] = 1
			} else {
				sqr.resultIndexHolderNew[curLevel][i][parentIndexNo] = sqr.resultIndexHolderNew[curLevel][i][parentIndexNo] + 1
			}
			sqr.resultHolderNew[curLevel][i][string(v)] = vrh[curLevel][i]
			curLevelRecord = sqr.resultHolderNew[curLevel][i][string(v)].(map[string]interface{})
			//TODO remove rowindex - atleast from result.
			curLevelRecord["rowIndex"] = sqr.resultIndexHolderNew[curLevel][i][parentIndexNo]
			if curLevelValue[sqr.tables[curLevel][i].Name] == nil {
				curLevelValue[sqr.tables[curLevel][i].Name] = []interface{}{curLevelRecord}
			} else {
				curLevelValue[sqr.tables[curLevel][i].Name] = append(curLevelValue[sqr.tables[curLevel][i].Name].([]interface{}), curLevelRecord)
			}
			indexNo = sqr.resultIndexHolderNew[curLevel][i][parentIndexNo]
		}
		recf[sqr.tables[curLevel][i].Name] = recordFound
		if curLevel+1 <= sqr.queryLevel && sqr.tables[curLevel][i].Nested {
			cr, rf, ee := sqr.processRows(ctx, vrh, curLevel+1, rowNo, recordFound, indexNo)
			if ee != nil {
				ee = logs.Err(ctx, ee, "")
				return nil, nil, ee
			}
			idx := 0
			for k, v := range cr {
				if !rf[k] {
					if curLevelRecord[k] == nil {
						curLevelRecord[k] = v
					} else {
						curLevelRecord[k] = append(curLevelRecord[k].([]interface{}), v.([]interface{})...)
					}
				}
				idx = idx + 1
			}
		}
		r[sqr.tables[curLevel][i].Name] = curLevelValue[sqr.tables[curLevel][i].Name]
	}
	return r, recf, nil
}

/*
func (sqr *SqlMaker) processColumnList(sel []ast.Selection, tableName string, vars map[string]interface{}, level int, sublevel int, datasource *module_model.DataSource) (columnList string, cList string, groupList string, err string) {
	//logs.WithContext(ctx).Info(fmt.Sprint("level = ", level, " sublevel = ", sublevel))
	//logs.WithContext(ctx).Info("tableName === ", tableName)

	if sqr.queryLevel < level {
		sqr.queryLevel = level
	}
	if len(sqr.querySubLevel) <= level {
		sqr.querySubLevel = append(sqr.querySubLevel, 1)
	}
	sqr.querySubLevel[level] = sublevel
	mySublevel := 0
	tempArray := make([]string, len(sel))
	var tempArrayG []string
	var tempArrayC []string
	tiq := tablesInQuery{strings.Replace(tableName, ".", "___", 1), false}

	if len(sqr.AllTableNamesNew) == level {
		sqr.AllTableNamesNew = append(sqr.AllTableNamesNew, []tablesInQuery{})
	}
	if len(sqr.AllTableNamesNew[level]) == sublevel {
		sqr.AllTableNamesNew[level] = append(sqr.AllTableNamesNew[level], tiq)
	}

	for i, va := range sel {
		joinFound := false
		colProcessed := false
		field := va.(*ast.Field)
		temp1 := strings.Split(field.Name.Value, "___")
		var temp2 []string
		var colSchemaName, colTableName, colName string
		if len(temp1) > 1 {
			colSchemaName = temp1[0]
			temp2 = strings.Split(temp1[1], "__")
			if len(temp2) > 1 {
				if colSchemaName != "" {
					colTableName = fmt.Sprint(colSchemaName, ".", temp2[0])
				} else {
					colTableName = temp2[0]
				}
				colName = temp2[1]
			} else {
				colName = temp2[0]
			}
		} else {
			temp2 = strings.Split(field.Name.Value, "__")
			if len(temp2) > 1 {
				colTableName = temp2[0]
				colName = temp2[1]
			} else {
				colName = temp2[0]
			}
		}
		if field.SelectionSet != nil {
			if colSchemaName != "" {
				colTableName = fmt.Sprint(colSchemaName, ".", colName)
			} else {
				colTableName = colName
			}
			colName = ""
		}
		var val string
		if colTableName != "" {
			val = fmt.Sprint(colTableName, ".", colName)
		} else {
			val = colName
		}
		//orgVal := fmt.Sprint("L",level,"**",field.Name.Value)
		alias := ""
		cName := ""
		if field.Alias == nil {
			alias = fmt.Sprint(" \"L", level, "~~", sublevel, "**", field.Name.Value, "\" ") // TODO to add aggregate in column name"_", d.Name.Value,
			cName = field.Name.Value
			//alias1 = fmt.Sprint(" ",alias)
		} else {
			alias = fmt.Sprint(" \"L", level, "~~", sublevel, "**", field.Alias.Value, "\" ")
			//alias1 = fmt.Sprint(" ",field.Alias.Value)
			cName = field.Alias.Value
		}
		//alias1 := ""
		tn := ""
		if len(strings.Split(tableName, ".")) > 1 {
			tn = strings.Split(tableName, ".")[1]
		} else {
			tn = tableName
		}
		if !strings.Contains(val, ".") {
			val = fmt.Sprint(tn, ".", val)
		}
		for _, a := range field.Arguments { //TODO where clause for inner tables
			switch a.Name.Value {
			case "join":
				joinFound = true
				sqr.processJoins(a.Value, nil, colTableName, vars)
			case "calc":
				v, err := ParseAstValue(a.Value, vars)
				logs.WithContext(ctx).Error(err.Error())                         //TODO to exit if error
				val = fmt.Sprint("'", v.(string), "'") //TODO to handle float value as variable value
			default:
				// do nothing
			}
		}
		if field.SelectionSet != nil {
			colName = ""
			var tg string
			var tc string
			tiq.nested = true
			sqr.AllTableNamesNew[level][sublevel] = tiq
			tempArray[i], tc, tg, err = sqr.processColumnList(field.SelectionSet.Selections, colTableName, vars, level+1, mySublevel, datasource)
			colProcessed = true
			mySublevel = mySublevel + 1
			if tg != "" {
				tempArrayG = append(tempArrayG, tg)
			}
			tempArrayC= append(tempArrayC,tc)
			if err != "" {
				return "", "", "", err
			}

		} else if len(field.Directives) > 0 {
			d := field.Directives[0]

			switch d.Name.Value {
			case "sum", "count", "avg", "max", "min":
				tempArray[i] = fmt.Sprint(d.Name.Value, "(", val, ") ", alias)
				sqr.HasAggregate = true
				colProcessed = true
			case "distinctcount":
				tempArray[i] = fmt.Sprint("count(distinct ", val, ") ", alias)
				sqr.HasAggregate = true
				colProcessed = true
			default:
				// do nothing
			}
		}
		if !colProcessed {
			tempArray[i] = fmt.Sprint(val, alias)
			tempArrayG = append(tempArrayG, val)
			tempArrayC = append(tempArrayC, cName)
		}
		if !joinFound && colTableName != "" && colTableName != sqr.MainTableName {
			logs.WithContext(ctx).Info("fetch joins for tables ", tableName, " and ", colTableName)
			if sqr.TableNames == nil {
				sqr.TableNames = make(map[string]string)
			}
			if _, ok := sqr.TableNames[colTableName]; !ok { //TODO to check simlar check of duplicate table in joins in join clause passed explicitly in query
				sqr.TableNames[colTableName] = ""
				tj, e := datasource.GetTableJoins(tableName, colTableName)
				logs.WithContext(ctx).Info(e)
				if e != nil {
					return "", "","", e.Error()
				}
				sqr.processJoins(nil, tj.GetOnClause(), colTableName, vars)
				joinFound = true
			}
		}
	}
	//logs.WithContext(ctx).Info("loop on field.SelectionSet.Selections ends")
	return strings.Join(tempArray, " , "), strings.Join(tempArrayC, " , "), strings.Join(tempArrayG, " , "), ""
}
*/

// parseAstValue returns an interface that can be casted to string
func ParseAstValue(ctx context.Context, value ast.Value, vars map[string]interface{}) (interface{}, error) {
	logs.WithContext(ctx).Debug("ParseAstValue - Start")
	switch value.GetKind() {
	case kinds.ObjectValue:
		o := map[string]interface{}{}
		obj := value.(*ast.ObjectValue)
		for _, v := range obj.Fields {
			temp, err := ParseAstValue(ctx, v.Value, vars)
			if err != nil {
				err = logs.Err(ctx, err, "")
				return nil, err
			}
			o[adjustObjectKey(ctx, v.Name.Value)] = temp
		}
		return o, nil

	case kinds.ListValue:
		listValue := value.(*ast.ListValue)
		array := make([]interface{}, len(listValue.Values))
		for i, v := range listValue.Values {
			val, err := ParseAstValue(ctx, v, vars)
			if err != nil {
				err = logs.Err(ctx, err, "")
				return nil, err
			}
			array[i] = val
		}
		return array, nil

	case kinds.EnumValue:
		v := value.(*ast.EnumValue).Value
		if strings.Contains(v, "__") {
			v = strings.ReplaceAll(v, "__", ".")
		}
		/*
			val, err := LoadValue(v, store)
			if err == nil {
				return val, nil
			}
		*/

		return v, nil

	case kinds.StringValue:
		v := value.(*ast.StringValue).Value
		if strings.Contains(v, "__") {
			v = strings.ReplaceAll(v, "__", ".")
		}
		/*
			val, err := LoadValue(v, store)
			if err == nil {
				return val, nil
			}
		*/
		return v, nil

	case kinds.IntValue:
		intValue := value.(*ast.IntValue)

		// Convert string to int
		val, err := strconv.Atoi(intValue.Value)
		if err != nil {
			err = logs.Err(ctx, err, "")
			return nil, err
		}

		return val, nil

	case kinds.FloatValue:
		floatValue := value.(*ast.FloatValue)

		val, err := strconv.ParseFloat(floatValue.Value, 64)
		if err != nil {
			err = logs.Err(ctx, err, "")
			return nil, err
		}

		return val, nil

	case kinds.BooleanValue:
		boolValue := value.(*ast.BooleanValue)
		return boolValue.Value, nil

	case kinds.Variable:
		t := value.(*ast.Variable)
		return replaceVariableValue(ctx, t.Name.Value, vars)

	default:
		return nil, errors.New("Invalid data type `" + value.GetKind() + "` for value " + string(value.GetLoc().Source.Body)[value.GetLoc().Start:value.GetLoc().End])
	}
}

func replaceVariableValue(ctx context.Context, varName string, vars map[string]interface{}) (res interface{}, err error) {
	logs.WithContext(ctx).Debug("replaceVariableValue - Start")
	if vars[varName] == nil {
		return nil, errors.New(fmt.Sprint("Variable value not found for '", varName, "'"))
	}
	switch reflect.TypeOf(vars[varName]).Kind() {
	case reflect.Map:
		m := vars[varName].(map[string]interface{})
		res, err = processMapVariable(ctx, m, vars)
	default:
		res = vars[varName]
	}
	return res, err
}

func processMapVariable(ctx context.Context, m map[string]interface{}, vars map[string]interface{}) (interface{}, error) {
	logs.WithContext(ctx).Debug("processMapVariable - Start")
	var err error
	for k, v := range m {
		switch reflect.TypeOf(v).Kind() {
		case reflect.Map:
			m[k], err = processMapVariable(ctx, v.(map[string]interface{}), vars)
		case reflect.Slice:
			// do nothing - return as is
		default:
			if strings.HasPrefix(v.(string), "$") { // TODO handle non string values in DOC for update
				m[k], err = replaceVariableValue(ctx, strings.Replace(v.(string), "$", "", 1), vars)
			}
		}
	}
	return m, err
}

func adjustObjectKey(ctx context.Context, key string) string {
	logs.WithContext(ctx).Debug("adjustObjectKey - Start")
	if strings.HasPrefix(key, "_") && key != "_id" {
		key = "$" + key[1:]
	}

	key = strings.ReplaceAll(key, "__", ".")

	return key
}

func (sqr *SqlMaker) MakeQuery(ctx context.Context) (dbQuery string, err string) {
	logs.WithContext(ctx).Debug("MakeQuery - Start")
	var str string
	strDistinct := ""
	gs := ""
	if sqr.HasAggregate && sqr.GroupList != "" {
		gs = fmt.Sprint(" group by ", sqr.GroupList, " ")
	}
	if sqr.DistinctResults {
		strDistinct = " distinct "
	}
	str = fmt.Sprint("select ", strDistinct, sqr.ColumnList, " from ", sqr.MainTableName, " ", sqr.JoinClause, " ", sqr.WhereClause, " ", gs, sqr.SortClause)
	return str, ""
}

func (sqr *SqlMaker) AddLimitSkipClause(ctx context.Context, query string, limit int, skip int, globalLimit int) (newQuery string) {
	logs.WithContext(ctx).Debug("AddLimitSkipClause - Start")
	if limit > 0 {
		newQuery = fmt.Sprint(newQuery, " limit ", limit)
	} else {
		newQuery = fmt.Sprint(newQuery, " limit ", globalLimit)
	}
	if skip > 0 {
		newQuery = fmt.Sprint(newQuery, " offset ", skip)
	}
	return newQuery
}

func (sqr *SqlMaker) GetTableList(ctx context.Context, datasource *module_model.DataSource, tableName string, myself SqlMakerI) (err error) {
	logs.WithContext(ctx).Debug("GetTableList - Start")
	tableList := make(map[string]map[string]common_types.TableColsMetaData)
	query := myself.GetTableMetaDataSQL(ctx, tableName)
	writeCon, werr := WriteCon(ctx, datasource)
	if werr != nil {
		return logs.Err(ctx, werr, "")
	}
	rows, e := writeCon.Queryx(query)
	logs.WithContext(ctx).Info(fmt.Sprintf("GetTableList - query: %s", query))
	if e != nil {
		logs.WithContext(ctx).Error(e.Error())
		return e
	}
	defer rows.Close()
	for rows.Next() {
		innerResultRow := common_types.TableColsMetaData{}
		e = rows.StructScan(&innerResultRow)
		if e != nil {
			logs.WithContext(ctx).Error(e.Error())
		}

		innerResultRow.OwnDataType = myself.getDataTypeMapping(ctx, innerResultRow.DataType)
		tableKey := fmt.Sprint(innerResultRow.TblSchema, ".", innerResultRow.TblName)
		if tableList[tableKey] == nil {
			tableList[tableKey] = make(map[string]common_types.TableColsMetaData)
		}
		tableList[tableKey][innerResultRow.ColName] = innerResultRow
	}
	datasource.OtherTables = tableList
	return nil

}
