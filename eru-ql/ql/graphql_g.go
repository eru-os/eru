package ql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/ds"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"
)

type SQLObjectQ struct {
	ProjectId       string
	TenantId        string
	FinalVariables  map[string]interface{}
	MainTableName   string
	MainAliasName   string
	MainTableDB     string
	WhereClause     interface{}
	SortClause      interface{}
	JoinClause      []*OrderedMap //map[string]interface{}
	DistinctResults bool
	HasAggregate    bool
	Limit           int
	Skip            int
	UseWriter       bool
	Columns         SQLCols
	tables          [][]module_model.Tables
	tableNames      map[string]string
	queryLevel      int
	querySubLevel   []int
	DBQuery         string
	OverwriteDoc    map[string]map[string]interface{} `json:"-"`
	SecurityClause  map[string]string                 `json:"-"`
	WithQuery       string                            `json:"-"`
	GroupByMode     bool                              `json:"-"`
}

type SQLCols struct {
	ColWithAlias []string
	ColNames     []string
	GroupClause  []string
}

func (sqlObj *SQLObjectQ) ProcessGraphQL(ctx context.Context, sel ast.Selection, datasource *module_model.DataSource, sqlMaker ds.SqlMakerI, vars map[string]interface{}, s module_store.ModuleStoreI, withColAlias bool) (err error) {
	logs.WithContext(ctx).Debug("ProcessGraphQL - Start")
	field := sel.(*ast.Field)
	sqlObj.MainTableName = strings.Replace(field.Name.Value, "___", ".", -1) //replacing schema___tablename with schema.tablename
	if field.Alias != nil {
		sqlObj.MainAliasName = field.Alias.Value
	} else {
		sqlObj.MainAliasName = sqlObj.MainTableName
	}
	sqlObj.MainTableDB = field.Directives[0].Name.Value

	/* we will need below block for tenant ds alias
	for _, vv := range field.Directives[0].Arguments {
	}
	*/

	for _, ff := range field.Arguments { //TODO to add join to main table without having to add

		v, e := ParseAstValue(ctx, ff.Value, vars)
		if e != nil {
			logs.WithContext(ctx).Error(e.Error())
		}
		switch ff.Name.Value {
		case "where":
			sqlObj.WhereClause = v
		case "sort":
			sqlObj.SortClause = v
		case "distinct":
			if ff.Value.GetKind() != kinds.BooleanValue {
				err = errors.New("Non Boolean value received - distinct clause need boolean value")
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			sqlObj.DistinctResults = v.(bool)
		case "limit": //TODO to handle if variable not found
			if reflect.TypeOf(v).Kind() == reflect.Float64 {
				v = int(v.(float64))
			}
			if reflect.TypeOf(v).Kind() != reflect.Int {
				err = errors.New("Non Integer value received - limit clause need integer value")
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			sqlObj.Limit = v.(int)
		case "skip":
			if reflect.TypeOf(v).Kind() == reflect.Float64 {
				v = int(v.(float64))
			}
			if reflect.TypeOf(v).Kind() != reflect.Int {
				err = errors.New("Non Integer value received - skip clause need integer value")
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			//v, e := ParseAstValue(ff.Value, vars)
			sqlObj.Skip = v.(int)
		case "use_writer":
			if ff.Value.GetKind() != kinds.BooleanValue {
				err = errors.New("Non Boolean value received - use_writer clause need boolean value")
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			sqlObj.UseWriter = v.(bool)
		default:
		}
	}
	sqlCols := SQLCols{}
	errMsg := ""
	if field.SelectionSet == nil {
		var tmpSelSet []ast.Selection
		sqlCols, _ = sqlObj.processColumnList(ctx, tmpSelSet, sqlObj.MainTableName, vars, 0, 0, datasource, s, sqlMaker)
		sqlCols.ColWithAlias = append(sqlCols.ColWithAlias, " * ")
	} else {
		sqlCols, errMsg = sqlObj.processColumnList(ctx, field.SelectionSet.Selections, sqlObj.MainTableName, vars, 0, 0, datasource, s, sqlMaker)
		if errMsg != "" {
			return errors.New(errMsg)
		}
	}
	sqlObj.Columns = sqlCols
	err = sqlObj.MakeQuery(ctx, sqlMaker, withColAlias)
	logs.WithContext(ctx).Info(fmt.Sprint("query  : ", sqlObj.DBQuery))
	return err
}

func (sqlObj *SQLObjectQ) processColumnList(ctx context.Context, sel []ast.Selection, tableName string, vars map[string]interface{}, level int, sublevel int, datasource *module_model.DataSource, s module_store.ModuleStoreI, sqlMaker ds.SqlMakerI) (sqlCols SQLCols, err string) {
	logs.WithContext(ctx).Debug("processColumnList - Start")

	if sqlObj.queryLevel < level {
		sqlObj.queryLevel = level
	}
	if len(sqlObj.querySubLevel) <= level {
		sqlObj.querySubLevel = append(sqlObj.querySubLevel, 1)
	}
	sqlObj.querySubLevel[level] = sublevel
	mySublevel := 0
	//sqlCols.ColWithAlias = make([]string, len(sel))

	//tempArray := make([]string, len(sel))
	//var tempArrayG []string
	//var tempArrayC []string

	tiq := module_model.Tables{Name: strings.Replace(tableName, ".", "___", 1), Nested: false, SqlQuery: ""}

	if len(sqlObj.tables) == level {
		sqlObj.tables = append(sqlObj.tables, []module_model.Tables{})
	}
	if len(sqlObj.tables[level]) == sublevel {
		sqlObj.tables[level] = append(sqlObj.tables[level], tiq)
	}
	var newSel []ast.Selection
	for _, va := range sel {
		field := va.(*ast.Field)
		colStr := field.Name.Value
		if strings.HasPrefix(colStr, "VAR_") {
			rv, rvErr := replaceVariableValue(ctx, strings.Replace(field.Name.Value, "VAR_", "", -1), vars)
			if rvErr != nil {
				logs.WithContext(ctx).Error(rvErr.Error())
				newSel = append(newSel, va)
				//return SQLCols{}, rvErr.Error()
			} else {
				colStrArray := strings.Split(rv.(string), ",")
				for _, str := range colStrArray {
					n := ast.Name{Kind: field.Kind, Loc: field.Loc, Value: str}
					f := ast.Field{Kind: field.Kind, Loc: field.Loc, Alias: field.Alias, Name: &n, Arguments: field.Arguments, Directives: field.Directives, SelectionSet: field.SelectionSet}
					newSel = append(newSel, &f)
				}
			}
		} else {
			fv := field.Name.Value
			if len(field.Arguments) > 0 {
				for _, a := range field.Arguments {
					switch a.Name.Value {
					case "fields":
						v, err := ParseAstValue(ctx, a.Value, vars)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
						}
						colStrArray := strings.Split(v.(string), ",")
						for _, cs := range colStrArray {
							csArray := strings.Split(cs, ":")
							cs_a := ""
							cs_c := ""
							if len(csArray) > 1 {
								cs_a = csArray[0]
								cs_c = csArray[1]
							} else {
								cs_a = csArray[0]
								cs_c = csArray[0]
							}
							cscArray := strings.Split(cs_c, "~")
							dt := ""
							if len(cscArray) > 1 {
								cs_c = cscArray[0]
								dt = cscArray[1]
								cs_a = fmt.Sprint(cs_a, "~", dt)
							}

							jc := sqlMaker.MakeJsonColumn(fv, cs_c)
							n := ast.Name{Kind: field.Kind, Loc: field.Loc, Value: jc}
							al := ast.Name{Kind: field.Kind, Loc: field.Loc, Value: fmt.Sprint(cs_a)}
							f := ast.Field{Kind: field.Kind, Loc: field.Loc, Alias: &al, Name: &n, Arguments: field.Arguments, Directives: field.Directives, SelectionSet: field.SelectionSet}
							if jc != "" {
								newSel = append(newSel, &f)
							}
						}
						break
					default:
						newSel = append(newSel, va)
					}
				}
			} else {
				newSel = append(newSel, va)
			}
		}
	}

	for _, va := range newSel {
		joinFound := false
		colProcessed := false
		field := va.(*ast.Field)
		colStr := field.Name.Value
		logs.WithContext(ctx).Info(fmt.Sprint(colStr))

		temp1 := strings.Split(colStr, "___")
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
			temp2 = strings.Split(colStr, "__")
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
			cName = colStr
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
				v, err := ParseAstValue(ctx, a.Value, vars)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
				}
				//TODO to exit if error
				//sqlObj.processJoins(a.Value, nil, colTableName, vars)

				//if sqlObj.JoinClause == nil {
				//	sqlObj.JoinClause = make(map[string]interface{})
				//}
				mapObj := make(map[string]interface{})
				mapObj[colTableName] = v
				om := OrderedMap{Level: level, SubLevel: sublevel, Rank: len(sqlObj.JoinClause) + 1, Obj: mapObj}
				sqlObj.JoinClause = append(sqlObj.JoinClause, &om)

			case "calc":
				v, err := ParseAstValue(ctx, a.Value, vars)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
				}
				//TODO to exit if error
				//val = fmt.Sprint("'", v.(string), "'") //TODO to handle float value as variable value
				actualType := reflect.TypeOf(v).String()
				if actualType == "string" {
					//val = fmt.Sprint("'", v.(string), "'") //TODO commented this as formulas stopped working
					val = v.(string)
				} else {
					val = v.(string) //TODO calc numeric value thorws error here
				}
			default:
				// do nothing
			}
		}
		if field.SelectionSet != nil {
			colName = ""
			cName = ""
			//var tg string
			//var tc string
			tiq.Nested = true
			sqlObj.tables[level][sublevel] = tiq
			sqlChildCols := SQLCols{}
			sqlChildCols, err = sqlObj.processColumnList(ctx, field.SelectionSet.Selections, colTableName, vars, level+1, mySublevel, datasource, s, sqlMaker)
			sqlCols.ColNames = append(sqlCols.ColNames, sqlChildCols.ColNames...)
			sqlCols.ColWithAlias = append(sqlCols.ColWithAlias, sqlChildCols.ColWithAlias...)
			sqlCols.GroupClause = append(sqlCols.GroupClause, sqlChildCols.GroupClause...)
			colProcessed = true
			mySublevel = mySublevel + 1
			//if tg != "" {
			//tempArrayG = append(tempArrayG, tg)
			//	sqlCols.GroupClause = append(sqlCols.GroupClause, tg)
			//}
			//tempArrayC = append(tempArrayC, tc)
			//sqlCols.ColNames = append(sqlCols.ColNames, tc)
			if err != "" {
				return SQLCols{}, err
			}
		} else if len(field.Directives) > 0 {
			d := field.Directives[0] // do not support multiple directives for fields - thus picking up first one - rest if provided will be ignored
			switch d.Name.Value {
			case "sum", "count", "avg", "max", "min":
				//tempArray[i] = fmt.Sprint(d.Name.Value, "(", val, ") ", alias)
				sqlCols.ColWithAlias = append(sqlCols.ColWithAlias, fmt.Sprint(d.Name.Value, "(", val, ") ", alias))
				sqlCols.ColNames = append(sqlCols.ColNames, cName)
				sqlObj.HasAggregate = true
				colProcessed = true
			case "distinctcount":
				//tempArray[i] = fmt.Sprint("count(distinct ", val, ") ", alias)
				sqlCols.ColWithAlias = append(sqlCols.ColWithAlias, fmt.Sprint("count(distinct ", val, ") ", alias))
				sqlCols.ColNames = append(sqlCols.ColNames, cName)
				sqlObj.HasAggregate = true
				colProcessed = true
			default:
				// do nothing
			}
		}
		if !colProcessed {
			//tempArray[i] = fmt.Sprint(val, alias)
			//tempArrayG = append(tempArrayG, val)
			//tempArrayC = append(tempArrayC, cName)
			sqlCols.ColWithAlias = append(sqlCols.ColWithAlias, fmt.Sprint(val, alias))
			sqlCols.GroupClause = append(sqlCols.GroupClause, val)
			sqlCols.ColNames = append(sqlCols.ColNames, cName)
		}
		if !joinFound && colTableName != "" && colTableName != sqlObj.MainTableName {
			logs.WithContext(ctx).Info(fmt.Sprint("fetch joins for tables ", tableName, " and ", colTableName))
			if sqlObj.tableNames == nil {
				sqlObj.tableNames = make(map[string]string)
			}
			if _, ok := sqlObj.tableNames[colTableName]; !ok { //TODO to check simlar check of duplicate table in joins in join clause passed explicitly in query
				sqlObj.tableNames[colTableName] = ""
				tj, e := datasource.GetTableJoins(ctx, tableName, colTableName, sqlObj.tableNames)
				if e != nil {
					logs.WithContext(ctx).Error(e.Error())
					//TODO if join not found then consider not handling it as error - currrently sql is sent to db with no columns resulting in sql failing
					return SQLCols{}, e.Error()
				}
				if sqlObj.SecurityClause == nil {
					sqlObj.SecurityClause = make(map[string]string)
				}
				sqlObj.SecurityClause[colTableName], _, e = getTableSecurityRule(ctx, sqlObj.ProjectId, sqlObj.TenantId, datasource.DbAlias, colTableName, s, "query", sqlObj.FinalVariables, colTableName)
				if e != nil {
					logs.WithContext(ctx).Error(e.Error())
					//ignoring error if security rule not defined - simply execute without security rule
					if !strings.Contains(e.Error(), "TableSecurityRule not defined for") {
						return SQLCols{}, e.Error()
					} else {
						e = nil
					}
				}
				//if sqlObj.JoinClause == nil {
				//	sqlObj.JoinClause = make(map[string]interface{})
				//}
				onClause, er := processMapVariable(ctx, tj.GetOnClause(ctx), vars)
				if er != nil {
					logs.WithContext(ctx).Error(er.Error())
				}
				mapObj := make(map[string]interface{})
				mapObj[colTableName] = onClause
				om := OrderedMap{Level: level, SubLevel: sublevel, Rank: len(sqlObj.JoinClause) + 1, Obj: mapObj}
				sqlObj.JoinClause = append(sqlObj.JoinClause, &om)
				joinFound = true
			}
		}
	}
	return sqlCols, err
}

func processWhereClause(ctx context.Context, val interface{}, parentKey string, mainTableName string, isJoinClause bool, jsonOp bool) (whereClause string, err string) { //, gqr *graphQLRead
	logs.WithContext(ctx).Debug("processWhereClause - Start")

	if val != nil {
		if strings.HasPrefix(parentKey, "CONST_") {
			parentKey = fmt.Sprint("'", strings.Replace(parentKey, "CONST_", "", 1), "'")
		} else if strings.HasPrefix(parentKey, "FIELD_") {
			parentKey = fmt.Sprint(strings.Replace(parentKey, "FIELD_", "", 1))
		} else if !(strings.Contains(parentKey, ".")) {
			if jsonOp {
				parentKey = fmt.Sprint(mainTableName, "->>'", parentKey, "'")
			} else {
				parentKey = fmt.Sprint(mainTableName, ".", parentKey)
			}

		}
		switch reflect.TypeOf(val).Kind() {
		case reflect.Map:
			var tempArray []string
			var keyList []string

			for _, key := range reflect.ValueOf(val).MapKeys() {
				keyList = append(keyList, key.Interface().(string))
			}
			sort.Strings(keyList)
			if valMap, valMapOk := val.(map[string]interface{}); !valMapOk {
				err = "error in where clause - map keys are not strings"
				logs.WithContext(ctx).Error(err)
				return "false", err
			} else {
				//tempArray := make([]string, len(reflect.ValueOf(val).MapKeys()))
				for _, v := range keyList {
					newVal := valMap[v]
					logs.WithContext(ctx).Info(fmt.Sprint(v, " : ", newVal))
					if newVal != nil {
						var valPrefix, valSuffix = "", ""
						if reflect.TypeOf(newVal).Kind().String() == "string" {
							if !strings.Contains(newVal.(string), ".") {
								valPrefix = "'"
								valSuffix = "'"
							}
							if strings.Contains(newVal.(string), "\\.") {
								valPrefix = "'"
								valSuffix = "'"
								newVal = strings.Replace(newVal.(string), "\\.", ".", -1)
							}
						}
						if v == "$or" || v == "or" || v == "_or" {
							if reflect.TypeOf(newVal).Kind().String() != "slice" {
								errStr := "Error : or clause has single element"
								logs.WithContext(ctx).Error(errStr)
								return "", errStr
							}
							s := reflect.ValueOf(newVal)
							innerTempArray := make([]string, s.Len())
							for ii := 0; ii < s.Len(); ii++ {
								innerTempArray[ii], err = processWhereClause(ctx, s.Index(ii).Interface(), v, mainTableName, isJoinClause, jsonOp)
								if err != "" {
									return "", err
								}
							}
							tempArray = append(tempArray, fmt.Sprint("( ", strings.Join(innerTempArray, " or "), " )"))
						} else if v == "json" {
							logs.WithContext(ctx).Info(fmt.Sprint("json operator found for :", parentKey))
							logs.WithContext(ctx).Info(fmt.Sprint(newVal))
							str := ""
							str, err = processWhereClause(ctx, newVal, "", parentKey, isJoinClause, true)
							if str == "" {
								logs.WithContext(ctx).Warn(fmt.Sprint("skipping whereclause for ", newVal, " as there is no value provided by user  : ", str))
							} else {
								tempArray = append(tempArray, str)
							}
							if err != "" {
								logs.WithContext(ctx).Error(err)
								return "", err
							}
						} else {
							op := ""
							switch v {
							case "$btw", "_btw":
								op = " between "
							case "$gte", "_gte":
								op = " >= "
							case "$lte", "_lte":
								op = " <= "
							case "$gt", "_gt":
								op = " > "
							case "$lt", "_lt":
								op = " < "
							case "$eq", "_eq":
								op = " = "
							case "$ne", "_ne":
								op = " <> "
							case "$in", "_in":
								op = " in "
							case "$nin", "_nin":
								op = " not in "
							case "$inc", "_inc":
								op = " in "
							case "$ninc", "_ninc":
								op = " not in "
							case "$jin", "_jin":
								op = module_model.MAKE_JSON_ARRAY_FN
							case "$jnin", "_jnin":
								op = module_model.MAKE_JSON_ARRAY_FN
							case "$like", "_like":
								op = " like "
							case "$nlike", "_nlike":
								op = " not like "
							default:
								op = ""
							}
							switch v {
							case "$gte", "$lte", "$gt", "$lt", "$eq", "$ne", "_gte", "_lte", "_gt", "_lt", "_eq", "_ne":
								valType := reflect.ValueOf(newVal).Kind()
								logs.WithContext(ctx).Info(fmt.Sprint(valType.String()))
								switch valType {
								case reflect.Float64, reflect.Float32, reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
									//TODO get casting from db specific syntax
									parentKey = fmt.Sprint("(", parentKey, ")::numeric")
								case reflect.Bool:
									//TODO get casting from db specific syntax
									parentKey = fmt.Sprint("(", parentKey, ")::boolean")
								default:
									//do nothing
								}
								logs.WithContext(ctx).Info(fmt.Sprint(newVal))
								valTmp := reflect.ValueOf(newVal).String()
								logs.WithContext(ctx).Info(fmt.Sprint(valTmp))
								if strings.HasPrefix(valTmp, "FIELD_") {
									valTmp = strings.Replace(valTmp, "FIELD_", "", -1)
									valPrefix = ""
									valSuffix = ""
								}
								tempArray = append(tempArray, fmt.Sprint(parentKey, op, valPrefix, fmt.Sprint(newVal), valSuffix))
							case "$like", "$nlike", "_like", "_nlike":
								tempArray = append(tempArray, fmt.Sprint(parentKey, op, valPrefix, "%", reflect.ValueOf(newVal), "%", valSuffix))
							case "$btw", "_btw":
								btwClause, ok := reflect.ValueOf(newVal).Interface().(map[string]interface{})
								if !ok {
									logs.WithContext(ctx).Warn("between clause is not a map")
								}
								preFix := "'"
								//checking only "from" value to determine with values recevied are int/float to avoid adding single quote in sql
								fromVal := btwClause["from"]
								_, intOk := fromVal.(int)
								_, int64Ok := fromVal.(int64)
								_, float32Ok := fromVal.(float32)
								_, float64Ok := fromVal.(float64)
								isNumber := intOk || int64Ok || float32Ok || float64Ok
								if isNumber {
									preFix = ""
								} else {
									_, Interr := strconv.Atoi(btwClause["from"].(string))
									if Interr == nil {
										preFix = ""
									}
									if _, flErr := strconv.ParseFloat(btwClause["from"].(string), 64); flErr == nil {
										preFix = ""
									}
								}
								btwClauseStr := fmt.Sprint(preFix, btwClause["from"], preFix, " and ", preFix, btwClause["to"], preFix)
								tempArray = append(tempArray, fmt.Sprint(parentKey, op, btwClauseStr))
							case "$null", "_null":
								nullValue := fmt.Sprint(reflect.ValueOf(newVal))
								if nullValue == "true" {
									tempArray = append(tempArray, fmt.Sprint(parentKey, " IS NULL "))
								} else {
									tempArray = append(tempArray, fmt.Sprint(parentKey, " IS NOT NULL "))
								}
							case "$inc", "$ninc", "$in", "$nin", "$jin", "$jnin", "_inc", "_ninc", "_in", "_nin", "_jin", "_jnin": //TODO to pass json variable aaray and check if the replaced array is passed as single string or string of values to sql
								switch reflect.TypeOf(newVal).Kind() {
								case reflect.String:
									s := reflect.ValueOf(newVal)
									//TODO - do we need this then implement variable replacement for $jin and $jnin
									/* if strings.HasPrefix(s.String(), "$") {

									} */
									op = module_model.MAKE_JSON_ARRAY_FN_STR
									valPrefix = "'"
									valSuffix = "'"
									if strings.HasPrefix(s.String(), "FIELD_") {
										valPrefix = ""
										valSuffix = ""
									}
									str := ""
									//TODO to make this from db specific syntax
									if v == "$jnin" || v == "$jin" || v == "_jnin" || v == "_jin" {
										parentKey = strings.Replace(parentKey, "->>", "->", -1)
										str = fmt.Sprint(parentKey, op, valPrefix, strings.Replace(s.String(), "FIELD_", "", -1), valSuffix)
									} else {
										var temp []string
										temp = append(temp, s.String())
										str = fmt.Sprint(parentKey, op, "(", valPrefix, strings.Join(temp, " , "), valSuffix, ")")
									}

									if v == "$jnin" || v == "_jnin" {
										str = fmt.Sprint(" not (", str, ") ")
									}
									tempArray = append(tempArray, str)
								case reflect.Slice:
									s := reflect.ValueOf(newVal)
									temp := make([]string, s.Len())
									for i := 0; i < s.Len(); i++ {
										ss := s.Index(i).Interface()
										if reflect.TypeOf(ss).Kind().String() == "string" {
											temp[i] = fmt.Sprint("'", ss, "'")
										} else {
											temp[i] = fmt.Sprint(ss)
										}
									}
									str := ""
									//TODO to make this from db specific syntax
									if v == "$jnin" || v == "$jin" || v == "_jnin" || v == "_jin" {
										parentKey = strings.Replace(parentKey, "->>", "->", -1)
										str = fmt.Sprint(parentKey, op, "[", strings.Join(temp, " , "), "]")
									} else {
										str = fmt.Sprint(parentKey, op, "(", strings.Join(temp, " , "), ")")
									}

									if v == "$jnin" || v == "_jnin" {
										str = fmt.Sprint(" not (", str, ") ")
									}
									tempArray = append(tempArray, str)
								default:
									logs.WithContext(ctx).Warn(fmt.Sprint("skipping $inc, $ninc, $in, $nin, $jin and $jnin clause as it needs array as a value but received ", newVal))
								}
							default:
								str := ""
								str, err = processWhereClause(ctx, newVal, eru_utils.ReplaceUnderscoresWithDots(v), mainTableName, isJoinClause, jsonOp)
								if str == "" {
									logs.WithContext(ctx).Warn(fmt.Sprint("skipping whereclause for ", newVal, " as there is no value provided by user  : ", str))
								} else {
									tempArray = append(tempArray, str)
								}
								if err != "" {
									logs.WithContext(ctx).Error(err)
									return "", err
								}
							}
						}
					}
				}
			}
			if len(tempArray) > 0 {
				return fmt.Sprint("( ", strings.Join(tempArray, " and "), " )"), ""
			} else {
				return "", ""
			}

		case reflect.String:
			var newVal, valPrefix, valSuffix = "", "", ""
			//TODO due to below statement - 2022-07-27T18:30:00.000Z date in filter is failing if passed in this format
			//parse for date
			if !strings.Contains(reflect.ValueOf(val).String(), ".") || !isJoinClause {
				valPrefix = "'"
				valSuffix = "'"
			}
			newVal = reflect.ValueOf(val).String()
			if strings.HasPrefix(newVal, "FIELD_") {
				valPrefix = ""
				valSuffix = ""
				newVal = strings.Replace(newVal, "FIELD_", "", -1)
			}
			return fmt.Sprint(parentKey, " = ", valPrefix, newVal, valSuffix), ""
		case reflect.Int, reflect.Float32, reflect.Float64:
			if jsonOp {
				//TODO database specific syntax
				parentKey = fmt.Sprint("(", parentKey, ")::numeric ")
			}
			return fmt.Sprint(parentKey, " = ", reflect.ValueOf(val)), ""
		case reflect.Bool:
			if jsonOp {
				//TODO database specific syntax
				parentKey = fmt.Sprint("(", parentKey, ")::boolean ")
			}
			return fmt.Sprint(parentKey, " = ", reflect.ValueOf(val)), ""
		default:
			return "", ""
		}
	}
	return "", ""
}

func (sqlObj *SQLObjectQ) processSortClause(ctx context.Context, val interface{}) (sortClause string) {
	logs.WithContext(ctx).Debug("processSortClause - Start")
	if val != nil {
		isDesc := ""
		_ = isDesc
		//v, e := ParseAstValue(val, vars)
		switch reflect.TypeOf(val).Kind() {
		case reflect.Slice:
			s := reflect.ValueOf(val)
			var temp []string
			for i := 0; i < s.Len(); i++ {
				isDesc = ""
				switch reflect.TypeOf(s.Index(i).Interface()).Kind() {
				case reflect.Map:
					if sMap, ok := s.Index(i).Interface().(map[string]interface{}); ok {
						for k, v := range sMap {
							switch reflect.TypeOf(v).Kind() {
							case reflect.Slice:
								ss := reflect.ValueOf(v)
								for ii := 0; ii < ss.Len(); ii++ {
									isDesc = ""
									sss := fmt.Sprintf("%s", ss.Index(ii))
									if strings.HasPrefix(sss, "-") {
										isDesc = " desc"
										sss = strings.Replace(sss, "-", "", 1)
									}
									if strings.Contains(sss, ".") {
										temp = append(temp, sss+isDesc)
									} else {
										temp = append(temp, fmt.Sprint(sqlObj.MainTableName, ".", k, "->>'", sss, "'", isDesc))
									}
								}
							default:
								//do nothing

							}
						}
					}
				default:
					si, ok := s.Index(i).Interface().(int)
					if ok {
						if si < 0 {
							isDesc = " desc"
							si = si * -1
						}
						temp = append(temp, fmt.Sprint(si, " ", isDesc))
					} else if sf, sfOk := s.Index(i).Interface().(float64); sfOk {
						if sf < 0 {
							isDesc = " desc"
							sf = sf * -1
						}
						temp = append(temp, fmt.Sprint(sf, " ", isDesc))
					} else {
						ss := fmt.Sprintf("%s", s.Index(i))
						if strings.HasPrefix(ss, "-") {
							isDesc = " desc"
							ss = strings.Replace(ss, "-", "", 1)
						}
						if strings.Contains(ss, ".") {
							temp = append(temp, ss+isDesc)
						} else {
							temp = append(temp, fmt.Sprintf("%s%s%s%s", sqlObj.MainTableName, ".", ss, isDesc))
						}
					}
				}
			}
			sortStr := ""
			if len(temp) > 0 {
				sortStr = fmt.Sprint(" order by ", strings.Join(temp, " , "))
			}
			return sortStr
		case reflect.String:
			s := fmt.Sprintf("%s", reflect.ValueOf(val))
			if strings.HasPrefix(s, "-") {
				isDesc = " desc"
				s = strings.Replace(s, "-", "", 1)
			}
			if strings.Contains(eru_utils.ReplaceUnderscoresWithDots(s), ".") {
				return fmt.Sprint(" order by ", eru_utils.ReplaceUnderscoresWithDots(s), isDesc)
			} else {
				return fmt.Sprint(" order by ", sqlObj.MainTableName, ".", s, isDesc)
			}
		case reflect.Int:
			s := reflect.ValueOf(val).Int()
			if s < 0 {
				isDesc = " desc"
				s = s * -1
			}
			return fmt.Sprint(" order by ", s, isDesc)
		case reflect.Float64:
			s := reflect.ValueOf(val).Float()
			if s < 0 {
				isDesc = " desc"
				s = s * -1
			}
			return fmt.Sprint(" order by ", s, isDesc)
		default:
		}
	}
	return ""
}
func (sqlObj *SQLObjectQ) processJoins(ctx context.Context, val []*OrderedMap) (strJoinClause string) {
	logs.WithContext(ctx).Debug("processJoins - Start")

	sort.Sort(MapSorter(val))

	for _, obj := range val {
		for tableName, v := range obj.Obj {
			joinType := "LEFT" //default join value TODO schema joins has an option to define join type
			onClause := ""
			switch reflect.TypeOf(v).Kind() {
			case reflect.Map:
				for _, vv := range reflect.ValueOf(v).MapKeys() { //TODO remove reflect usage
					if vv.String() == "joinType" {
						jt, err := reflect.ValueOf(v).MapIndex(vv).Interface().(string)
						if !err {
							logs.WithContext(ctx).Warn("joinType value is not a string")
						}
						switch jt {
						case "LEFT", "RIGHT", "INNER":
							joinType = jt
						default:
							logs.WithContext(ctx).Warn("valid values for joinType are LEFT RIGHT and INNER ")
						}
					} else if vv.String() == "on" {
						oc, _ := processWhereClause(ctx, reflect.ValueOf(v).MapIndex(vv).Interface(), "", sqlObj.MainTableName, true, false)
						onClause = oc
					}
				}
				strJoinClause = fmt.Sprint(strJoinClause, " ", fmt.Sprint(joinType, " JOIN ", tableName, " on ", onClause))
			default:
				//do nothing
			}
		}
	}
	return strJoinClause
}

func (sqlObj *SQLObjectQ) MakeQuery(ctx context.Context, sqlMaker ds.SqlMakerI, withColAlias bool) (err error) {
	logs.WithContext(ctx).Debug("MakeQuery - Start")
	strDistinct := ""
	strGroupClause := ""
	strColums := ""
	if withColAlias {
		strColums = strings.Join(sqlObj.Columns.ColWithAlias, " , ")
	} else {
		strColums = strings.Join(sqlObj.Columns.ColNames, " , ")
	}
	strJoinClause := sqlObj.processJoins(ctx, sqlObj.JoinClause)
	strWhereClause, e := processWhereClause(ctx, sqlObj.WhereClause, "", sqlObj.MainTableName, false, false)
	if e != "" {
		err = errors.New(e)
	}

	strAnd := ""
	strSecurityClause := ""
	for _, v := range sqlObj.SecurityClause {
		if v != "" {
			strSecurityClause = fmt.Sprint(strSecurityClause, strAnd, v)
			strAnd = " and "
		}
	}
	if strSecurityClause != "" {
		if strWhereClause != "" {
			strWhereClause = fmt.Sprint(strWhereClause, " and ", strSecurityClause)
		} else {
			strWhereClause = strSecurityClause
		}
	}
	if strWhereClause != "" {
		strWhereClause = fmt.Sprint(" where ", strWhereClause)
	}

	strSortClause := ""
	if !sqlObj.GroupByMode {
		strSortClause = sqlObj.processSortClause(ctx, sqlObj.SortClause)
	}
	if sqlObj.HasAggregate && len(sqlObj.Columns.GroupClause) > 0 {
		strGroupClause = fmt.Sprint(" group by ", strings.Join(sqlObj.Columns.GroupClause, " , "))
	}
	if sqlObj.DistinctResults {
		strDistinct = " distinct "
	}

	fromTable := sqlObj.MainTableName
	withClause := ""
	if sqlObj.WithQuery != "" {
		fromTable = fmt.Sprint("( ", sqlObj.WithQuery, " ) ", sqlObj.MainTableName)
	}
	sqlObj.DBQuery = fmt.Sprint(withClause, "select ", strDistinct, strColums, " from ", fromTable, " ", strJoinClause, " ", strWhereClause, " ", strGroupClause, strSortClause)

	if !sqlObj.GroupByMode || sqlObj.Limit > 0 || sqlObj.Skip > 0 {
		sqlObj.DBQuery = sqlMaker.AddLimitSkipClause(ctx, sqlObj.DBQuery, sqlObj.Limit, sqlObj.Skip, 1000)
	}

	makeJsonArrayFnStrKeyWord, err := sqlMaker.GetMakeJsonArrayFnStr()
	if err != nil {
		makeJsonArrayFnStrKeyWord = ""
	}
	sqlObj.DBQuery = strings.Replace(sqlObj.DBQuery, module_model.MAKE_JSON_ARRAY_FN_STR, makeJsonArrayFnStrKeyWord, -1)

	makeJsonArrayFnKeyWord, err := sqlMaker.GetMakeJsonArrayFn()
	if err != nil {
		makeJsonArrayFnKeyWord = ""
	}
	sqlObj.DBQuery = strings.Replace(sqlObj.DBQuery, module_model.MAKE_JSON_ARRAY_FN, makeJsonArrayFnKeyWord, -1)

	return err
}
