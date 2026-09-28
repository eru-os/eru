package ql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/ds"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
	"github.com/eru-os/eru/eru-ql/qlcache"
	"github.com/eru-os/eru/eru-read-write/eru_writes"
	"github.com/eru-os/eru/eru-templates/gotemplate"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
)

type GraphQLData struct {
	QLData
	Operation   string                 `json:"operation"`
	QueryObject map[string]QueryObject `json:"_"`
}

var KeyWords = []string{"null"}

func (gqd *GraphQLData) SetQLData(ctx context.Context, mq module_model.MyQuery, vars map[string]interface{}, executeFlag bool, tokenObj map[string]interface{}, isPublic bool, outputType string) {
	logs.WithContext(ctx).Debug("SetQLData - Start")
	gqd.SetQLDataCommon(ctx, mq, vars, executeFlag, tokenObj, isPublic, outputType)
}

func (gqd *GraphQLData) parseGraphQL(ctx context.Context) (d *ast.Document, err error) {
	logs.WithContext(ctx).Debug("parseGraphQL - Start")
	s := source.NewSource(&source.Source{
		Body: []byte(gqd.Query),
		Name: gqd.Operation,
	})
	d, err = parser.Parse(parser.ParseParams{Source: s})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return d, err
}

var graphQLColAliasRegex = regexp.MustCompile(`"([^"]*)"\s*$`)

func groupByColMap(cols SQLCols) (colMap map[string]string) {
	colMap = make(map[string]string)
	count := len(cols.ColNames)
	if len(cols.ColWithAlias) < count {
		count = len(cols.ColWithAlias)
	}
	for i := 0; i < count; i++ {
		match := graphQLColAliasRegex.FindStringSubmatch(cols.ColWithAlias[i])
		if match == nil {
			continue
		}
		colMap[cols.ColNames[i]] = fmt.Sprint("\"", match[1], "\"")
	}
	return colMap
}

func renameResultKey(result map[string]interface{}, from string, to string) (renamed map[string]interface{}) {
	if from == to {
		return result
	}
	v, ok := result[from]
	if !ok {
		return result
	}
	renamed = make(map[string]interface{}, len(result))
	for k, rv := range result {
		if k != from {
			renamed[k] = rv
		}
	}
	renamed[to] = v
	return renamed
}

func (gqd *GraphQLData) getSqlForQuery(ctx context.Context, projectId string, datasources map[string]*module_model.DataSource, query string, s module_store.ModuleStoreI, tokenObj map[string]interface{}, isPublic bool) (err error) {
	logs.WithContext(ctx).Debug("getSqlForQuery - Start")
	mq, err := s.GetMyQuery(ctx, projectId, gqd.TenantId, query)
	if err != nil {
		return err
	}
	//mq == nil changed to below
	if mq.QueryName == "" {
		err = errors.New(fmt.Sprint("Query ", query, " not found"))
		//logs.WithContext(ctx).Info(err.Error())
		return err
	}
	logs.WithContext(ctx).Info(fmt.Sprint("Query ", query, " found. Executing the same inside current GraphQL routine"))
	qlInterface := GetQL(mq.QueryType)
	if qlInterface == nil {
		err = errors.New("Invalid Query Type")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}

	qlInterface.SetQLData(ctx, mq, gqd.FinalVariables, false, tokenObj, isPublic, gqd.OutputType) //passing false as we only need the query in execute function and not actual result
	_, queryObjs, err := qlInterface.Execute(ctx, projectId, datasources, s, gqd.OutputType)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	for i, q := range queryObjs {
		queryObjs[i].Type = strings.ToUpper(strings.Split(q.Query, " ")[0])
	}
	if gqd.QueryObject == nil {
		gqd.QueryObject = make(map[string]QueryObject)
	}
	gqd.QueryObject[query] = queryObjs[0] // setting first result as query used in mutation select will usually be single query
	return nil
}

func (gqd *GraphQLData) Execute(ctx context.Context, projectId string, datasources map[string]*module_model.DataSource, s module_store.ModuleStoreI, outputType string) (res []map[string]interface{}, queryObjs []QueryObject, err error) {
	logs.WithContext(ctx).Debug("Execute of GraphQl - Start")
	singleTxn := false
	errFound := false
	doc, err := gqd.parseGraphQL(ctx)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, err
	}
	//doc := ast.Node(d).(*ast.Document)
	for _, docDef := range doc.Definitions {
		op := ast.Node(docDef).(*ast.OperationDefinition)
		graphQLs := make([]ds.SqlMakerI, len(op.SelectionSet.Selections))
		//graphQLs := make([]ds.SqlMakerI, len(op.SelectionSet.Selections))
		var result map[string]interface{}
		var results []map[string]interface{}

		for _, v := range op.Directives {
			if v.Name.Value == "singleTxn" {
				singleTxn = true
			}
		}
		var returnAliasStrings []string
		var mainAliasNames []string
		breakForLoop := false
		for i, v := range op.SelectionSet.Selections {
			if breakForLoop {
				break
			}
			errMsg := "-"
			openTxn := false
			closeTxn := false
			queryObj := QueryObject{}
			if singleTxn && i == 0 {
				openTxn = true
			}
			if singleTxn && i == len(op.SelectionSet.Selections)-1 {
				closeTxn = true
			}
			dbAlias := ""
			if len(v.(*ast.Field).Directives) > 0 {
				dbAlias = v.(*ast.Field).Directives[0].Name.Value
			} else {
				logs.WithContext(ctx).Info(fmt.Sprint("No dbAlias found for ", v.(*ast.Field).Name.Value))
			}
			datasource := datasources[dbAlias]
			if datasource == nil {
				return nil, nil, errors.New(fmt.Sprint("dbAlias ", dbAlias, " not found"))
			}

			if i == 0 {
				graphQLs[i] = ds.GetSqlMaker(datasource.DbName)
			} else if singleTxn {
				graphQLs[i] = graphQLs[0]
			} else {
				graphQLs[i] = ds.GetSqlMaker(datasource.DbName)
			}

			boundVars := make(map[string]bool)
			if fld, ok := v.(*ast.Field); ok {
				for _, ff := range fld.Arguments {
					if ff.Name.Value == "docs" && ff.Value.GetKind() == kinds.Variable {
						boundVars[strings.Replace(ff.Value.(*ast.Variable).Name.Value, "$", "", -1)] = true
					}
				}
			}

			for k, v := range gqd.FinalVariables {
				if k != module_model.RULEPREFIX_TOKEN && !boundVars[k] {
					err = graphQLs[i].VerifyForBlockedWords(ctx, k, v, graphQLs[i])
					if err != nil {
						return
					}
				}
			}

			switch op.Operation {
			case "query":
				sqlObj := SQLObjectQ{}
				sqlObj.ProjectId = projectId
				sqlObj.TenantId = gqd.TenantId
				sqlObj.FinalVariables = gqd.QLData.FinalVariables
				field := v.(*ast.Field)
				sqlObj.MainTableName = strings.Replace(field.Name.Value, "___", ".", -1) //replacing schema___tablename with schema.tablename
				if field.Alias == nil {
					sqlObj.MainAliasName = field.Name.Value
				} else {
					sqlObj.MainAliasName = field.Alias.Value
				}

				if sqlObj.OverwriteDoc == nil {
					sqlObj.OverwriteDoc = make(map[string]map[string]interface{})
				}

				sqlObj.OverwriteDoc[sqlObj.MainTableName], err = gqd.setOverwriteDoc(ctx, projectId, dbAlias, sqlObj.MainTableName, s, op.Operation, module_model.QUERY_TYPE_SELECT, nil)
				if err != nil {
					errMsg = err.Error()
					errFound = true
				}

				err = gqd.getSqlForQuery(ctx, projectId, datasources, sqlObj.MainTableName, s, nil, gqd.IsPublic)
				startTime := time.Now()
				sqlObj.WithQuery = gqd.secureSQL(ctx, gqd.QueryObject[sqlObj.MainTableName].Query, projectId, datasource, s, graphQLs[i])
				endTime := time.Now()
				logs.WithContext(ctx).Info(fmt.Sprint("Time taken for secureSQL for ", sqlObj.MainTableName, " : ", endTime.Sub(startTime).Seconds()))
				if err != nil {
					logs.WithContext(ctx).Info(err.Error())
				}
				logs.WithContext(ctx).Info(sqlObj.WithQuery)

				if sqlObj.SecurityClause == nil {
					sqlObj.SecurityClause = make(map[string]string)
				}
				sqlObj.SecurityClause[sqlObj.MainTableName], _, err = getTableSecurityRule(ctx, projectId, gqd.TenantId, dbAlias, sqlObj.MainTableName, s, op.Operation, gqd.FinalVariables, sqlObj.MainTableName)
				if err != nil {
					if !strings.Contains(err.Error(), "TableSecurityRule not defined for") {
						return nil, nil, err
					} else {
						err = nil
					}
				}

				sqlObj.GroupByMode = gqd.GroupBy.Active
				err = sqlObj.ProcessGraphQL(ctx, v, datasource, graphQLs[i], gqd.FinalVariables, s, gqd.ExecuteFlag) //TODO to handle if err recd.
				//TODO : test this error handling
				if err != nil {
					return nil, nil, err
				}

				queryObj.Query = sqlObj.DBQuery
				queryObj.Cols = strings.Join(sqlObj.Columns.ColNames, " , ")
				mainAliasNames = append(mainAliasNames, sqlObj.MainAliasName)

				if gqd.GroupBy.Active {
					gqd.SetGroupByColMap(groupByColMap(sqlObj.Columns))
					groupQuery, gErr := gqd.wrapGroupBy(ctx, sqlObj.DBQuery)
					if gErr != nil {
						return nil, nil, gErr
					}
					groupQuery, gErr = gqd.wrapQuery(ctx, groupQuery, graphQLs[i])
					if gErr != nil {
						return nil, nil, gErr
					}
					queryObj.Query = groupQuery
					queryObj.Cols = strings.Join(gqd.GroupBy.GroupBy, " , ")
					if gqd.ExecuteFlag {
						ctxw := ds.WithUseWriter(ctx, false)
						if eru_writes.IsColumnarOutput(gqd.OutputType) {
							result, err = graphQLs[i].ExecuteQueryForCsv(ctxw, groupQuery, datasource, sqlObj.MainAliasName, graphQLs[i])
							if err != nil {
								err = logs.Err(ctx, err, "")
							}
							queryObj.DataTypes = graphQLs[i].GetResultDataTypes(ctx)
						} else {
							loader := func(ctx context.Context) (map[string]interface{}, []string, error) {
								r, lerr := graphQLs[i].ExecutePreparedQuery(ctx, groupQuery, datasource)
								if lerr != nil {
									return nil, nil, lerr
								}
								tbls := graphQLs[i].ExtractTableNames(ctx, groupQuery)
								names := make([]string, 0, len(tbls.Tables))
								for _, t := range tbls.Tables {
									if t.TableName != "" {
										names = append(names, t.TableName)
									}
								}
								return r, names, nil
							}
							result, err = qlcache.ServeOrLoad(ctxw, datasource, gqd.TenantId, groupQuery, graphQLs[i].DefaultSchemaName(), loader, qlcache.Options{})
							if result != nil {
								result = renameResultKey(result, "Results", strings.Replace(sqlObj.MainAliasName, ".", "___", 1))
							}
						}
						if err != nil {
							err = logs.Err(ctx, err, "")
							errMsg = err.Error()
							errFound = true
						}
						if result != nil {
							res = append(res, result)
						}
					}
					queryObjs = append(queryObjs, queryObj)
					continue
				}
				if gqd.ExecuteFlag {
					qrm := module_model.QueryResultMaker{}
					qrm.MainTableName = sqlObj.MainTableName
					qrm.MainAliasName = sqlObj.MainAliasName
					qrm.Tables = sqlObj.tables
					qrm.QueryLevel = sqlObj.queryLevel
					qrm.QuerySubLevel = sqlObj.querySubLevel
					qrm.SQLQuery = sqlObj.DBQuery
					qrm.SQLQuery, err = gqd.wrapQuery(ctx, qrm.SQLQuery, graphQLs[i])
					if err != nil {
						return nil, nil, err
					}
					queryObj.Query = qrm.SQLQuery
					qrm.UseWriter = sqlObj.UseWriter
					ctx := ds.WithUseWriter(ctx, sqlObj.UseWriter)

					if eru_writes.IsColumnarOutput(gqd.OutputType) {
						result, err = graphQLs[i].ExecuteQueryForCsv(ctx, qrm.SQLQuery, datasource, mainAliasNames[i], graphQLs[i])
						if err != nil {
							err = logs.Err(ctx, err, "")
						}
						queryObj.DataTypes = graphQLs[i].GetResultDataTypes(ctx)
					} else {
						loader := func(ctx context.Context) (map[string]interface{}, []string, error) {
							r, lerr := graphQLs[i].ExecuteQuery(ctx, datasource, qrm)
							if lerr != nil {
								return nil, nil, lerr
							}
							tbls := graphQLs[i].ExtractTableNames(ctx, qrm.SQLQuery)
							names := make([]string, 0, len(tbls.Tables))
							for _, t := range tbls.Tables {
								if t.TableName != "" {
									names = append(names, t.TableName)
								}
							}
							return r, names, nil
						}
						result, err = qlcache.ServeOrLoad(ctx, datasource, gqd.TenantId, qrm.SQLQuery, graphQLs[i].DefaultSchemaName(), loader, qlcache.Options{})
					}
					if err != nil {
						err = logs.Err(ctx, err, "")
						errMsg = err.Error()
						errFound = true
					}
				}

				if result != nil {
					res = append(res, result)
				}

			case "mutation":
				sqlObj := SQLObjectM{}
				field := v.(*ast.Field)
				tempStr := strings.SplitN(field.Name.Value, "_", 2)
				sqlObj.QueryType = tempStr[0]
				sqlObj.MainTableName = strings.Replace(tempStr[1], "___", ".", -1)
				for _, ff := range field.Arguments {
					switch ff.Name.Value {
					case "docs":
						if sqlObj.OverwriteDoc == nil {
							sqlObj.OverwriteDoc = make(map[string]map[string]interface{})
						}
						docKeyword := ""
						switch ff.Value.GetKind() {
						case kinds.Variable:
							t := ff.Value.(*ast.Variable)
							docKeyword = strings.Replace(t.Name.Value, "$", "", -1)
						}
						sqlObj.OverwriteDoc[sqlObj.MainTableName], err = gqd.setOverwriteDoc(ctx, projectId, dbAlias, sqlObj.MainTableName, s, op.Operation, sqlObj.QueryType, gqd.FinalVariables[docKeyword])
						if err != nil {
							err = logs.Err(ctx, err, "")
							errMsg = err.Error()
							errFound = true
						}
					default:
						//do nothing
					}
				}

				err = gqd.getSqlForQuery(ctx, projectId, datasources, sqlObj.MainTableName, s, nil, gqd.IsPublic)
				if err == nil {
					sqlObj.PreparedQuery = true
				}
				selectQuery := ""
				for _, ff := range field.Arguments {
					if ff.Name.Value == "query" {
						selectQuery = ff.Value.GetValue().(string)
					}
				}
				if selectQuery != "" {
					err = gqd.getSqlForQuery(ctx, projectId, datasources, selectQuery, s, nil, gqd.IsPublic)
					//todo consider passing token from finalvariables
					if err != nil {
						errFound = true
						err = logs.Err(ctx, err, "")
						errMsg = err.Error()
					}
				}
				sqlObj.SingleTxn = singleTxn
				sqlObj.openTxn = openTxn
				sqlObj.closeTxn = closeTxn
				sqlObj.QueryObject = gqd.QueryObject
				err = sqlObj.ProcessMutationGraphQL(ctx, v, gqd.FinalVariables, datasource, graphQLs[i])
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
				}
				//TODO connection close on this error - to handle the same
				mainAliasNames = append(mainAliasNames, sqlObj.MainAliasName)
				//TODO to loop on MutationRecords and pass query
				if gqd.ExecuteFlag && !errFound {
					//TODO can remove this mrm object and directly set values to graphQLs[i]
					mrm := module_model.MutationResultMaker{}
					mrm.MainTableName = sqlObj.MainTableName
					mrm.MainAliasName = sqlObj.MainAliasName
					mrm.MutationReturn = sqlObj.MutationReturn
					mrm.MutationRecords = sqlObj.MutationRecords
					mrm.SingleTxn = sqlObj.SingleTxn
					mrm.OpenTxn = sqlObj.openTxn
					mrm.CloseTxn = sqlObj.closeTxn
					mrm.TxnFlag = sqlObj.TxnFlag
					mrm.IsNested = sqlObj.NestedDoc
					mrm.QueryType = sqlObj.QueryType
					mrm.DBQuery = sqlObj.DBQuery
					mrm.PreparedQuery = sqlObj.PreparedQuery
					results, err = graphQLs[i].ExecuteMutationQuery(ctx, datasource, graphQLs[i], mrm)
					if err != nil {
						errFound = true
						err = logs.Err(ctx, err, "")
						errMsg = err.Error()
						// no need to return here - error is returned as part of result - if asked in the query.
					}
				} else if errFound {
					rollBackErr := graphQLs[i].RollbackQuery(ctx)
					if rollBackErr != nil {
						rollBackErr = logs.Err(ctx, rollBackErr, "")
						errMsg = rollBackErr.Error()
					}
					breakForLoop = true
				}
				returnResult := make(map[string]interface{})
				if sqlObj.MutationReturn.ReturnError {
					returnResult[sqlObj.MutationReturn.ReturnErrorAlias] = errMsg
				}
				if sqlObj.MutationReturn.ReturnDoc {
					returnResult[sqlObj.MutationReturn.ReturnDocAlias] = results
				}
				resObj := make(map[string]interface{})
				returnAliasStrings = append(returnAliasStrings, sqlObj.MutationReturn.ReturnDocAlias)
				resObj[mainAliasNames[i]] = returnResult
				res = append(res, resObj)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
				}
			default:
				logs.WithContext(ctx).Info(fmt.Sprint("Unrecognized Operation : ", op.Operation))
				//do nothing
			}
			queryObjs = append(queryObjs, queryObj)
		}
		if errFound && singleTxn {
			for i, _ := range res {
				obj, check := res[i][mainAliasNames[i]].(map[string]interface{})
				if check {
					if obj[returnAliasStrings[i]] != nil {
						obj[returnAliasStrings[i]] = make(map[string]interface{})
					}
				}
			}
			return res, queryObjs, errors.New("ERROR")
		}
	}
	return res, queryObjs, err
}

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
				return nil, err
			}
			o[adjustObjectKey(v.Name.Value)] = temp
		}
		return o, nil

	case kinds.ListValue:
		listValue := value.(*ast.ListValue)
		array := make([]interface{}, len(listValue.Values))
		for i, v := range listValue.Values {
			val, err := ParseAstValue(ctx, v, vars)
			if err != nil {
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
		//TODO - also consider moving this vars loop for INT value
		for varsK, varsV := range vars {
			if str, ok := varsV.(string); ok {
				v = strings.ReplaceAll(v, fmt.Sprint("$", varsK), str)
			}
		}
		vBytes, err := processTemplate(ctx, "variable", v, vars, "string", "", "")
		if err != nil {
			return nil, err
		}
		if string(vBytes) != "" {
			v = string(vBytes)
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
			return nil, err
		}

		return val, nil

	case kinds.FloatValue:
		floatValue := value.(*ast.FloatValue)

		// Convert string to int
		val, err := strconv.ParseFloat(floatValue.Value, 64)
		if err != nil {
			return nil, err
		}

		return val, nil

	case kinds.BooleanValue:
		boolValue := value.(*ast.BooleanValue)
		return boolValue.Value, nil

	case kinds.Variable:
		t := value.(*ast.Variable)
		if strings.HasPrefix(t.Name.Value, "$") {
			logs.WithContext(ctx).Info(fmt.Sprint("key has $ prefix : ", t.Name.Value))
		}
		return replaceVariableValue(ctx, t.Name.Value, vars)

	default:
		err := errors.New("Invalid data type `" + value.GetKind() + "` for value " + string(value.GetLoc().Source.Body)[value.GetLoc().Start:value.GetLoc().End])
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

func replaceVariableValue(ctx context.Context, varName string, vars map[string]interface{}) (res interface{}, err error) {
	logs.WithContext(ctx).Debug("replaceVariableValue - Start")
	for _, kw := range KeyWords {
		if kw == varName {
			return fmt.Sprint("$", varName), nil
		}
	}

	if vars[varName] == nil {
		return nil, errors.New(fmt.Sprint("Variable value not found for '", varName, "'"))
	}
	switch reflect.TypeOf(vars[varName]).Kind() {
	case reflect.Slice:
		m := vars[varName].([]interface{})
		for i, v := range m {
			switch reflect.TypeOf(v).Kind() {
			case reflect.Map:
				m[i], err = processMapVariable(ctx, v.(map[string]interface{}), vars)
			default:
				// do nothing
			}
		}
		res = vars[varName]
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
		mapKey := k
		if strings.HasPrefix(k, "$") {
			tempI, err := replaceVariableValue(ctx, strings.Replace(mapKey, "$", "", 1), vars)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}
			if err == nil {
				switch reflect.TypeOf(tempI).Kind() {
				case reflect.String, reflect.Float64, reflect.Int64:
					mapKey = fmt.Sprint(tempI)
				default:
					mapKey = tempI.(string)
				}
				m[mapKey] = m[k]
				delete(m, k)
			}
		}

		if v != nil {
			switch reflect.TypeOf(v).Kind() {
			case reflect.Map:
				m[mapKey], err = processMapVariable(ctx, v.(map[string]interface{}), vars)
			case reflect.Slice:
				s := v.([]interface{})
				for ii, sv := range s {
					switch reflect.TypeOf(sv).Kind() {
					case reflect.Map:
						s[ii], err = processMapVariable(ctx, sv.(map[string]interface{}), vars)
					case reflect.String:
						if strings.HasPrefix(sv.(string), "$") {
							s[ii], err = replaceVariableValue(ctx, strings.Replace(sv.(string), "$", "", 1), vars)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
							}
						}
					default:
						// do nothing
					}
				}
			case reflect.String:
				if strings.HasPrefix(v.(string), "$") {
					tArray := strings.Split(v.(string), " ")
					tempI, err := replaceVariableValue(ctx, strings.Replace(tArray[0], "$", "", 1), vars)
					if err == nil {
						switch reflect.TypeOf(tempI).Kind() {
						case reflect.String, reflect.Float64, reflect.Int64:
							tArray[0] = fmt.Sprint(tempI)
							m[mapKey] = strings.Join(tArray, " ")
						default:
							m[mapKey] = tempI
						}
					} else {
						if err != nil {
							logs.WithContext(ctx).Info(err.Error())
						}
						logs.WithContext(ctx).Info(fmt.Sprint("removing key ", mapKey, " from list of variables"))
						delete(m, mapKey)
					}
					//logs.WithContext(ctx).Info(fmt.Sprint("variable ", v.(string), " replace with ", m[mapKey]))
				} else {
					vBytes, err := processTemplate(ctx, "variable", v.(string), vars, "string", "", "")
					if err != nil {
						return nil, err
					}
					if string(vBytes) == "" {
						m[mapKey] = v.(string)
					} else {
						m[mapKey] = string(vBytes)
					}

				}
			default:
				// do nothing - return as is
			}
		}
	}
	return m, err
}

func adjustObjectKey(key string) string {
	if strings.HasPrefix(key, "_") && key != "_id" {
		key = "$" + key[1:]
	}
	key = strings.ReplaceAll(key, "__", ".")

	return key
}

func (gqd *GraphQLData) setOverwriteDoc(ctx context.Context, projectId string, dbAlias string, tableName string, s module_store.ModuleStoreI, op string, queryType string, docs interface{}) (overwriteDoc map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("setOverwriteDoc - Start")
	tr, err := s.GetTableTransformation(ctx, projectId, gqd.TenantId, dbAlias, tableName)
	if err != nil {
		err = errors.New(fmt.Sprint("error from GetTableTransformation = ", err.Error()))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	overwriteDoc = make(map[string]interface{})
	if op == "query" {
		overwriteDoc, err = gqd.ProcessTransformRule(ctx, tr.TransformOutput, docs)
	} else if op == "mutation" {
		processTransformRule := false
		for _, v := range tr.TransformInput.ApplyOn {
			if v == queryType {
				processTransformRule = true
				break
			}
		}

		var docArray []interface{}
		isArray := false
		docArray, isArray = docs.([]interface{})
		if !isArray {
			dd, er := docs.(map[string]interface{}) // checking if docs is a single document without array
			if !er {
				return nil, errors.New("error while parsing value of 'docs'")
			}
			docArray = append(docArray, dd)
		}
		childOverwriteDoc := make(map[string]interface{})

		for _, doc := range docArray {
			dd, er := doc.(map[string]interface{})
			if !er {
				return nil, errors.New("error while parsing value of 'docs'")
			}

			for k, v := range dd {
				if strings.Contains(k, "___") {
					tn := strings.Replace(k, "___", ".", -1)
					childOverwriteDoc[tn], err = gqd.setOverwriteDoc(ctx, projectId, dbAlias, tn, s, op, queryType, v)
					childOverwriteDoc[tn] = v
				}
			}
		}
		if processTransformRule {
			overwriteDoc, err = gqd.ProcessTransformRule(ctx, tr.TransformInput, docs)
			if docMap, docMapOk := docs.(map[string]interface{}); docMapOk {
				overwriteDoc = docMap
			}
		}
		for k, v := range childOverwriteDoc {
			overwriteDoc[k] = v
		}
	} else {
		err = errors.New(fmt.Sprint("Invalid Operation : ", op))
		logs.WithContext(ctx).Error(err.Error())
	}

	if err != nil {
		err = errors.New(fmt.Sprint("TransformRule failed : ", err.Error()))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return
}

func getTableSecurityRule(ctx context.Context, projectId string, tenantId string, dbAlias string, tableName string, s module_store.ModuleStoreI, op string, vars map[string]interface{}, mainTableName string) (ruleOutput string, ruleJoinTables []string, err error) {
	logs.WithContext(ctx).Debug("getTableSecurityRule - Start")
	var templates []string
	var ruleJoinChildTables []string
	sr, err := s.GetTableSecurity(ctx, projectId, tenantId, dbAlias, tableName)
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		err = errors.New(fmt.Sprint("TableSecurityRule not defined for ", tableName))
		logs.WithContext(ctx).Info(err.Error())
		return "", make([]string, 0), err
	}
	//var ruleJoinChildTables []string
	if op == "query" {

		if sr.Select.IgnoreRecord != "" {
			ignoreRecordBytes, irErr := processTemplate(ctx, "security_rule_ignore_record", sr.Select.IgnoreRecord, vars, "json", "", "")
			if irErr != nil {
				irErr = logs.Err(ctx, irErr, "")
				return "", make([]string, 0), irErr
			}
			ignoreRecord := make(map[string]interface{})
			irErr = json.Unmarshal(ignoreRecordBytes, &ignoreRecord)
			if irErr != nil {
				irErr = logs.Err(ctx, irErr, "")
				return "", make([]string, 0), irErr
			}

			evalStr, evalErr := gotemplate.EvalFilter(ctx, sr.Select.IgnoreFilter, ignoreRecord)
			if evalErr != nil {
				evalErr = logs.Err(ctx, evalErr, "")
				return "", make([]string, 0), evalErr
			}
			if evalStr {
				return "", nil, nil
			}
		}
		if sr.Select.RuleType == "deny" {
			err = errors.New("access denied for table " + tableName)
			err = logs.Err(ctx, err, "")
			return "", nil, err
		}

		for _, v := range sr.Select.CustomRule.AND {
			if v.Template == "" {
				tableNameParts := strings.Split(v.Variable1, ".")
				if len(tableNameParts) == 1 {
					tableNameParts = strings.Split(fmt.Sprint(mainTableName, ".", v.Variable1), ".")
				}
				ruleTableName := tableNameParts[0]
				if len(tableNameParts) > 1 {
					ruleTableName = strings.Join(tableNameParts[:len(tableNameParts)-1], ".")
				}
				if mainTableName != ruleTableName && !strings.HasPrefix(ruleTableName, "token") {
					if !(v.Operator == "ex_in" || v.Operator == "nex_in" || v.Operator == "ex_jin" || v.Operator == "nex_jin" || v.Operator == "ex_pj" || v.Operator == "nex_pj") {
						ruleJoinTables = append(ruleJoinTables, ruleTableName)
					} else {
						ruleJoinChildTables = append(ruleJoinChildTables, ruleTableName)
					}
				}
			}
		}
		for _, v := range sr.Select.CustomRule.OR {
			if v.Template == "" {
				tableNameParts := strings.Split(v.Variable1, ".")
				if len(tableNameParts) == 1 {
					tableNameParts = strings.Split(fmt.Sprint(mainTableName, ".", v.Variable1), ".")
				}
				ruleTableName := tableNameParts[0]
				if len(tableNameParts) > 1 {
					ruleTableName = strings.Join(tableNameParts[:len(tableNameParts)-1], ".")
				}
				if mainTableName != ruleTableName && !strings.HasPrefix(ruleTableName, "token") {
					if !(v.Operator == "ex_in" || v.Operator == "nex_in" || v.Operator == "ex_jin" || v.Operator == "nex_jin" || v.Operator == "ex_pj" || v.Operator == "nex_pj") {
						ruleJoinTables = append(ruleJoinTables, ruleTableName)
					} else {
						ruleJoinChildTables = append(ruleJoinChildTables, ruleTableName)
					}
				}
			}
		}

		ds, dsErr := s.GetDataSource(ctx, projectId, tenantId, dbAlias)
		if dsErr != nil {
			dsErr = logs.Err(ctx, dsErr, "")
			return "", nil, dsErr
		}
		ctjObjMap := make(map[string]string)
		for _, ctj := range ruleJoinChildTables {
			ctjObj, e := ds.GetTableJoins(ctx, mainTableName, ctj, make(map[string]string))
			if e != nil {
				e = logs.Err(ctx, e, "")
				return "", nil, e
			}
			onClause, er := processMapVariable(ctx, ctjObj.GetOnClause(ctx), vars)
			if er != nil {
				er = logs.Err(ctx, er, "")
				return "", nil, er
			}
			oc, _ := processWhereClause(ctx, onClause, "", mainTableName, true, false)
			ctjObjMap[ctj] = oc
		}
		var ptables []string
		ruleOutput, templates, ptables, err = processSecurityRule(ctx, sr.Select, vars, mainTableName, ctjObjMap)

		for _, t := range templates {
			ro, rj, rerr := getTableSecurityRule(ctx, projectId, tenantId, dbAlias, t, s, op, vars, tableName)
			if rerr != nil {
				if !strings.Contains(rerr.Error(), "TableSecurityRule not defined for") {
					return "", nil, rerr
				} else {
					rerr = nil
				}
				return "", nil, rerr
			}
			ruleJoinTables = append(ruleJoinTables, rj...)
			ruleOutput = strings.Replace(ruleOutput, fmt.Sprint("$TEMPLATE_", t), ro, -1)
		}

		for _, p := range ptables {
			ro, rjt, roerr := getTableSecurityRule(ctx, projectId, tenantId, dbAlias, p, s, op, vars, p)
			q := fmt.Sprint("select  ", p, ".* from ", p)
			if roerr == nil && ro != "" {
				for _, srJoin := range rjt {
					tj, e := ds.GetTableJoins(ctx, p, srJoin, make(map[string]string))
					if e != nil {
						e = logs.Err(ctx, e, "")
						return "", nil, e
					}
					onClause, er := processMapVariable(ctx, tj.GetOnClause(ctx), vars)
					if er != nil {
						er = logs.Err(ctx, er, "")
						return "", nil, er
					}
					oc, _ := processWhereClause(ctx, onClause, "", p, true, false)
					q = fmt.Sprint(q, " left join ", srJoin, " on ", oc)
				}
				q = fmt.Sprint(q, " where ", ro)
			}
			ruleOutput = strings.Replace(ruleOutput, fmt.Sprint("$", p, "$"), fmt.Sprint("( ", q, " ) pj "), -1)
		}
	} else if op == "mutation" {
		ruleOutput, templates, _, err = processSecurityRule(ctx, sr.Insert, vars, mainTableName, nil) //todo to change it as per query type
		_ = templates
	} else {
		err = errors.New(fmt.Sprint("Invalid Query Type : ", op))
		err = logs.Err(ctx, err, "")
		return "", nil, err
	}

	if err != nil {
		err = errors.New(fmt.Sprint("SecurityRule failed : ", err.Error()))
		err = logs.Err(ctx, err, "")
		return "", make([]string, 0), err
	}
	return ruleOutput, eru_utils.UniqueStrings(ruleJoinTables), nil
}
