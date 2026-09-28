package ql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/ds"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
	eru_writes "github.com/eru-os/eru/eru-read-write/eru_writes"
	"github.com/eru-os/eru/eru-security-rule/security_rule"
	"github.com/eru-os/eru/eru-templates/gotemplate"
	eru_utils "github.com/eru-os/eru/eru-utils"
)

type QLData struct {
	Query          string                       `json:"query"`
	QueryName      string                       `json:"-"`
	TenantId       string                       `json:"-"`
	Variables      map[string]interface{}       `json:"variables"`
	FinalVariables map[string]interface{}       `json:"-"`
	ExecuteFlag    bool                         `json:"-"`
	SecurityRule   security_rule.SecurityRule   `json:"security_rule"`
	IsPublic       bool                         `json:"is_public"`
	OutputType     string                       `json:"output_type"`
	PivotConfig    QLPivotConfig                `json:"pivot_config,omitempty"`
	Formatter      eru_writes.CellFormatter     `json:"formatter,omitempty"`
	UseWriter      bool                         `json:"use_writer,omitempty"`
	GroupBy        module_model.GroupByConfig   `json:"-"`
	GroupByColMap  map[string]string            `json:"-"`
	WrapConfig     module_model.QueryWrapConfig `json:"-"`
}

type QLPivotConfig struct {
	Columns      []string `json:"columns"`
	Rows         []string `json:"rows"`
	Aggregations []struct {
		AggregationFunction string `json:"aggregation_function"`
		Field_Name          string `json:"field_name"`
		Filed_Label         string `json:"field_label"`
	} `json:"aggregations"`
}

type QueryObject struct {
	Query     string
	Cols      string
	Type      string
	DataTypes []ds.ResultDataTypes
}

type QL interface {
	Execute(ctx context.Context, projectId string, datasources map[string]*module_model.DataSource, s module_store.ModuleStoreI, outputType string) (res []map[string]interface{}, queryObjs []QueryObject, err error)
	SetQLData(ctx context.Context, mq module_model.MyQuery, vars map[string]interface{}, executeFlag bool, tokenObj map[string]interface{}, isPublic bool, outputType string)
	SetTenantId(tenantId string)
	SetGroupBy(gb module_model.GroupByConfig)
	SetGroupByColMap(colMap map[string]string)
	SetWrapConfig(w module_model.QueryWrapConfig)
	ProcessTransformRule(ctx context.Context, tr module_model.TransformRule, docs interface{}) (outputObj map[string]interface{}, err error)
}

func (qld *QLData) SetTenantId(tenantId string) {
	qld.TenantId = tenantId
}

func (qld *QLData) SetGroupBy(gb module_model.GroupByConfig) {
	qld.GroupBy = gb
}

func (qld *QLData) SetGroupByColMap(colMap map[string]string) {
	qld.GroupByColMap = colMap
}

func groupBySelectPart(expr string, name string) string {
	if groupByAliasRegex.MatchString(name) {
		return fmt.Sprint(expr, " \"", name, "\"")
	}
	return expr
}

func (qld *QLData) resolveGroupByCol(ctx context.Context, name string) (expr string, err error) {
	if mapped, ok := qld.GroupByColMap[name]; ok {
		return mapped, nil
	}
	if err = validateGroupByIdentifier(ctx, name); err != nil {
		return "", err
	}
	return name, nil
}

func (qld *QLData) SetWrapConfig(w module_model.QueryWrapConfig) {
	qld.WrapConfig = w
}

var groupByIdentifierRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
var groupByAliasRegex = regexp.MustCompile(`^[A-Za-z0-9_ ]+$`)
var groupByAggFuncs = map[string]bool{"count": true, "sum": true, "avg": true, "min": true, "max": true}

func validateGroupByIdentifier(ctx context.Context, name string) (err error) {
	if !groupByIdentifierRegex.MatchString(name) {
		err = errors.New(fmt.Sprint("invalid group by identifier : ", name))
		logs.WithContext(ctx).Error(err.Error())
	}
	return
}

func validateGroupByAlias(ctx context.Context, alias string) (err error) {
	if !groupByAliasRegex.MatchString(alias) {
		err = errors.New(fmt.Sprint("invalid group by alias : ", alias))
		logs.WithContext(ctx).Error(err.Error())
	}
	return
}

func (qld *QLData) wrapGroupBy(ctx context.Context, query string) (wrappedQuery string, err error) {
	logs.WithContext(ctx).Debug("wrapGroupBy - Start")
	if !qld.GroupBy.Active {
		return query, nil
	}
	aggregations := qld.GroupBy.Aggregations
	if len(aggregations) == 0 {
		aggregations = []module_model.AggregationConfig{{Func: "count", Field: "1", Alias: "cnt"}}
	}
	var selectParts []string
	aggAliases := make(map[string]bool)
	groupCols := make(map[string]bool)
	var groupByCols []string
	for _, g := range qld.GroupBy.GroupBy {
		g = strings.TrimSpace(g)
		var expr string
		expr, err = qld.resolveGroupByCol(ctx, g)
		if err != nil {
			return "", err
		}
		selectParts = append(selectParts, groupBySelectPart(expr, g))
		if !groupCols[expr] {
			groupCols[expr] = true
			groupByCols = append(groupByCols, expr)
		}
	}
	for _, agg := range aggregations {
		fn := strings.ToLower(strings.TrimSpace(agg.Func))
		if !groupByAggFuncs[fn] {
			err = errors.New(fmt.Sprint("invalid aggregation function : ", agg.Func))
			logs.WithContext(ctx).Error(err.Error())
			return "", err
		}
		field := strings.TrimSpace(agg.Field)
		alias := strings.TrimSpace(agg.Alias)
		if alias == "" {
			alias = fmt.Sprint(fn, "_", field)
		}
		if err = validateGroupByAlias(ctx, alias); err != nil {
			return "", err
		}
		if !(fn == "count" && field == "1") {
			field, err = qld.resolveGroupByCol(ctx, field)
			if err != nil {
				return "", err
			}
		}
		selectParts = append(selectParts, fmt.Sprint(fn, "(", field, ") \"", alias, "\""))
		aggAliases[alias] = true
	}
	orderByClause := ""
	if len(qld.GroupBy.GroupOrderBy) > 0 {
		var sorts []string
		for _, o := range qld.GroupBy.GroupOrderBy {
			dir := ""
			col := strings.TrimSpace(o)
			if strings.HasPrefix(col, "-") {
				dir = " desc"
				col = strings.TrimSpace(col[1:])
			}
			if aggAliases[col] {
				sorts = append(sorts, fmt.Sprint("\"", col, "\"", dir))
				continue
			}
			var expr string
			expr, err = qld.resolveGroupByCol(ctx, col)
			if err != nil {
				return "", err
			}
			if !groupCols[expr] {
				groupCols[expr] = true
				groupByCols = append(groupByCols, expr)
				selectParts = append(selectParts, groupBySelectPart(expr, col))
			}
			sorts = append(sorts, fmt.Sprint(expr, dir))
		}
		orderByClause = fmt.Sprint(" order by ", strings.Join(sorts, " , "))
	}
	groupByClause := ""
	if len(groupByCols) > 0 {
		groupByClause = fmt.Sprint(" group by ", strings.Join(groupByCols, " , "))
	}
	wrappedQuery = fmt.Sprint("select ", strings.Join(selectParts, " , "), " from (", query, ") eru_grp", groupByClause, orderByClause)
	return wrappedQuery, nil
}

func (qld *QLData) hasWrapConfig() bool {
	w := qld.WrapConfig
	return len(w.Filter) > 0 || len(w.Sort) > 0 || w.Limit != 0 || w.Skip != 0
}

func (qld *QLData) verifyWrapApplicable(ctx context.Context) (err error) {
	if qld.GroupBy.Active {
		err = errors.New("group by is not supported for a query which is not a plain select")
	} else if qld.hasWrapConfig() {
		err = errors.New("qfilter , qsort , qlimit and qskip are not supported for a query which is not a plain select")
	}
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return
}

func (qld *QLData) wrapQuery(ctx context.Context, query string, sr ds.SqlMakerI) (wrappedQuery string, err error) {
	logs.WithContext(ctx).Debug("wrapQuery - Start")
	w := qld.WrapConfig
	if len(w.Filter) == 0 && len(w.Sort) == 0 && w.Limit == 0 && w.Skip == 0 {
		return query, nil
	}
	whereClause := ""
	if len(w.Filter) > 0 {
		fStr, fErr := gotemplate.MakeFilterFromMap(ctx, w.Filter, "", "")
		if fErr != nil {
			return "", fErr
		}
		if fStr != "" {
			whereClause = fmt.Sprint(" where ", fStr)
		}
	}
	orderClause := ""
	if len(w.Sort) > 0 {
		var sorts []string
		for _, s := range w.Sort {
			dir := ""
			col := s
			if strings.HasPrefix(col, "-") {
				dir = " desc"
				col = col[1:]
			}
			if err = validateGroupByIdentifier(ctx, col); err != nil {
				return "", err
			}
			sorts = append(sorts, fmt.Sprint(col, dir))
		}
		orderClause = fmt.Sprint(" order by ", strings.Join(sorts, " , "))
	}
	wrappedQuery = fmt.Sprint("select * from (", query, ") eru_wrap", whereClause, orderClause)
	if w.Limit > 0 || w.Skip > 0 {
		wrappedQuery = sr.AddLimitSkipClause(ctx, wrappedQuery, w.Limit, w.Skip, 1000)
	}
	return wrappedQuery, nil
}

func (qld *QLData) SetQLDataCommon(ctx context.Context, mq module_model.MyQuery, vars map[string]interface{}, executeFlag bool, tokenObj map[string]interface{}, isPublic bool, outputType string) (err error) {
	logs.WithContext(ctx).Debug("SetQLDataCommon - Start")
	if mq.Vars == nil {
		mq.Vars = make(map[string]interface{})
	}
	//mq.Vars[module_model.RULEPREFIX_TOKEN] = tokenObj
	qld.Query = mq.Query
	qld.QueryName = mq.QueryName
	qld.Variables = mq.Vars
	qld.ExecuteFlag = executeFlag
	qld.IsPublic = isPublic
	qld.OutputType = outputType
	err = qld.SetFinalVars(ctx, vars)
	if tokenObj != nil {
		qld.FinalVariables[module_model.RULEPREFIX_TOKEN] = tokenObj
	}
	return err
}
func (qld *QLData) SetFinalVars(ctx context.Context, vars map[string]interface{}) (err error) {
	logs.WithContext(ctx).Debug("SetFinalVars - Start")
	tmpVars, _ := json.Marshal(qld.Variables)
	finalVars := make(map[string]interface{})
	err = json.Unmarshal(tmpVars, &finalVars) // Marshall UnMarshall used to copy without referencing of map
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	//commented below copying and replaced with above Marshall/UnMarshall to copy without referencing of map
	//for k, v := range myQuery.Vars {
	//	finalVars[k] = v
	//}
	if finalVars == nil {
		finalVars = make(map[string]interface{})
	}
	for k, v := range vars {
		finalVars[k] = v
	}
	qld.FinalVariables = finalVars
	return nil
}
func (qld *QLData) ProcessTransformRule(ctx context.Context, tr module_model.TransformRule, docs interface{}) (outputObj map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ProcessTransformRule - Start")
	if tr.RuleType == module_model.RULETYPE_NONE {
		outputObj = make(map[string]interface{})
		return
	}
	if tr.RuleType == module_model.RULETYPE_ALWAYS {
		outputObj = make(map[string]interface{})
		if len(tr.Rules) > 0 {
			for k, v := range tr.Rules[0].ForceColumnValues {
				//todo to remove array and make it single object
				outputBytes, err := processTemplate(ctx, "xxx", v, qld.FinalVariables, "string", k, docs)
				if err != nil {
					return nil, err
				}
				if outputBytes != nil {
					outputObj[k] = string(outputBytes)
					//skipping to write to overwriteobj if there is no output
				}
				logs.WithContext(ctx).Info(fmt.Sprint(outputObj[k]))
			}
		}
	}
	return
}

func processSecurityRule(ctx context.Context, sr security_rule.SecurityRule, vars map[string]interface{}, mainTableName string, ctjMap map[string]string) (outputStr string, templates []string, ptables []string, err error) {
	logs.WithContext(ctx).Debug("processSecurityRule - Start")
	if sr.RuleType == module_model.RULETYPE_NONE {
		err = errors.New("Security Rule Set to NONE")
		return
	}
	if sr.RuleType == module_model.RULETYPE_ALWAYS {
		//do nothing
		return
	} else if sr.RuleType == module_model.RULETYPE_CUSTOM {
		outputStr, templates, ptables, err = sr.Stringify(ctx, vars, false, mainTableName, ctjMap)

	}
	return
}

// func processTemplate(ctx context.Context, templateName string, templateString string, vars map[string]interface{}, outputType string, key string, d interface{}) (output []byte, err error) {
// 	logs.WithContext(ctx).Debug("processTemplate - Start")
// 	ruleValue := strings.SplitN(templateString, ".", 2)
// 	templateStr := ""
// 	if len(ruleValue) > 1 {
// 		templateStr = ruleValue[1]
// 	} else {
// 		templateStr = ruleValue[0]
// 	}
// 	if ruleValue[0] == module_model.RULEPREFIX_TOKEN {
// 		templateStr = ruleValue[1]
// 		if !(strings.HasPrefix(ruleValue[1], "{{")) {
// 			templateStr = fmt.Sprint("{{ .", ruleValue[1], " }}")
// 		}
// 		templateStr = eru_utils.ReplaceVariables(ctx, templateStr, vars)
// 		return executeTemplate(ctx, templateName, templateStr, vars[module_model.RULEPREFIX_TOKEN], outputType)
// 	} else if ruleValue[0] == module_model.RULEPREFIX_DOCS {
// 		var docs []interface{}
// 		isArray := false

// 		docs, isArray = d.([]interface{})
// 		if !isArray {
// 			dd, er := d.(map[string]interface{}) // checking if docs is a single document without array
// 			if !er {
// 				return nil, errors.New("error while parsing value of 'docs'")
// 			}
// 			docs = append(docs, dd)
// 		}

// 		for i, doc := range docs {
// 			dd, er := doc.(map[string]interface{}) // checking if docs is a single document without array
// 			if !er {
// 				return nil, errors.New("error while parsing value of 'docs'")
// 			}
// 			outputBytes, ptErr := executeTemplate(ctx, templateName, templateStr, dd, outputType)
// 			if err != nil {
// 				err = ptErr
// 				logs.WithContext(ctx).Error(err.Error())
// 				return
// 			}
// 			dd[key] = string(outputBytes)
// 			docs[i] = dd
// 			outputBytes = nil
// 		}
// 	} else if ruleValue[0] == module_model.RULEPREFIX_NONE {
// 		return executeTemplate(ctx, templateName, templateStr, vars, outputType)
// 	} else {
// 		return executeTemplate(ctx, templateName, templateStr, vars, outputType)
// 	}
// 	return
// }

func processTemplate(ctx context.Context, templateName string, templateString string, vars map[string]interface{}, outputType string, key string, d interface{}) (output []byte, err error) {
	logs.WithContext(ctx).Debug("processTemplate - Start")
	ruleValue := strings.SplitN(templateString, ".", 2)
	templateStr := ""
	if ruleValue[0] == module_model.RULEPREFIX_TOKEN {
		templateStr = ruleValue[1]
		if !(strings.HasPrefix(ruleValue[1], "{{")) {
			templateStr = fmt.Sprint("{{ .", ruleValue[1], " }}")
		}
		templateStr = eru_utils.ReplaceVariables(ctx, templateStr, vars)
		return executeTemplate(ctx, templateName, templateStr, vars[module_model.RULEPREFIX_TOKEN], outputType)
	} else if ruleValue[0] == module_model.RULEPREFIX_DOCS {
		templateStr = ruleValue[1]
		var docs []interface{}
		isArray := false

		docs, isArray = d.([]interface{})
		if !isArray {
			dd, er := d.(map[string]interface{}) // checking if docs is a single document without array
			if !er {
				return nil, logs.Err(ctx, errors.New("error while parsing value of 'docs'"), "")
			}
			docs = append(docs, dd)
		}

		for i, doc := range docs {
			dd, er := doc.(map[string]interface{}) // checking if docs is a single document without array
			if !er {
				return nil, logs.Err(ctx, errors.New("error while parsing value of 'docs'"), "")
			}
			outputBytes, ptErr := executeTemplate(ctx, templateName, templateStr, dd, outputType)
			if err != nil {
				err = logs.Err(ctx, ptErr, "")
				return
			}
			dd[key] = string(outputBytes)
			docs[i] = dd
			outputBytes = nil
		}
	} else if ruleValue[0] == module_model.RULEPREFIX_NONE {
		if len(ruleValue) > 1 {
			templateStr = ruleValue[1]
		} else {
			templateStr = templateString
		}
		return executeTemplate(ctx, templateName, templateStr, vars, outputType)
	} else if strings.Contains(templateString, module_model.RULEINFIX_NONE) {
		templateStr = strings.Replace(templateString, module_model.RULEINFIX_NONE, "", -1)
		return executeTemplate(ctx, templateName, templateStr, vars, outputType)
	}
	return
}
func executeTemplate(ctx context.Context, templateName string, templateString string, vars interface{}, outputType string) (output []byte, err error) {
	logs.WithContext(ctx).Debug("executeTemplate - Start")
	goTmpl := gotemplate.GoTemplate{Name: templateName, Template: templateString}
	outputObj, err := goTmpl.Execute(ctx, vars, outputType)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	} else if outputType == "string" {
		return []byte(outputObj.(string)), nil
	} else {
		output, err = json.Marshal(outputObj)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
	}
	return
}
func (qld *QLData) secureSQL(ctx context.Context, query string, projectId string, datasource *module_model.DataSource, s module_store.ModuleStoreI, sr ds.SqlMakerI) (secureQuery string) {
	tableNames := sr.ExtractTableNames(ctx, query)
	for _, table := range tableNames.Tables {
		if !(strings.HasSuffix(table.TableName, "___ALL")) {
			if !(strings.Contains(table.TableName, ".")) {
				table.TableName = fmt.Sprint(sr.DefaultSchemaName(), table.TableName)
			}
			sRulesStr, srJoins, srErr := getTableSecurityRule(ctx, projectId, qld.TenantId, datasource.DbAlias, table.TableName, s, "query", qld.FinalVariables, table.TableName)
			if srErr != nil {
				logs.WithContext(ctx).Info(srErr.Error())
				if !strings.HasPrefix(srErr.Error(), "TableSecurityRule not defined for "+table.TableName) {
					return
				} else {
					srErr = nil
				}
			}
			if sRulesStr != "" {
				q := fmt.Sprint("select  ", table.TableName, ".* from ", table.TableName)
				for _, srJoin := range srJoins {
					tj, e := datasource.GetTableJoins(ctx, table.TableName, srJoin, make(map[string]string))
					if e != nil {
						logs.WithContext(ctx).Error(e.Error())
						return
					}
					onClause, er := processMapVariable(ctx, tj.GetOnClause(ctx), qld.FinalVariables)
					if er != nil {
						logs.WithContext(ctx).Error(er.Error())
						return
					}
					oc, _ := processWhereClause(ctx, onClause, "", table.TableName, true, false)
					q = fmt.Sprint(q, " left join ", srJoin, " on ", oc)
				}
				q = fmt.Sprint(q, " where ", sRulesStr)
				query = strings.Replace(query, fmt.Sprint(table.TableKeyPrefix, table.TableKey, table.TableKeySuffix), fmt.Sprint(table.TableKeyPrefix, " (", q, ") ", table.AliasName, " ", table.TableKeySuffix), -1)

				makeJsonArrayFnStrKeyWord, err := sr.GetMakeJsonArrayFnStr()
				if err != nil {
					makeJsonArrayFnStrKeyWord = ""
				}
				query = strings.Replace(query, module_model.MAKE_JSON_ARRAY_FN_STR, makeJsonArrayFnStrKeyWord, -1)

				makeJsonArrayFnKeyWord, err := sr.GetMakeJsonArrayFn()
				if err != nil {
					makeJsonArrayFnKeyWord = ""
				}
				query = strings.Replace(query, module_model.MAKE_JSON_ARRAY_FN, makeJsonArrayFnKeyWord, -1)
			}
		} else {
			query = strings.Replace(query, table.TableName, strings.Replace(table.TableName, "___ALL", "", -1), -1)
		}
	}
	secureQuery = query
	return
}
