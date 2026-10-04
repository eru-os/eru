package ql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
  "github.com/eru-os/eru/eru-ql/derived"
	"github.com/eru-os/eru/eru-ql/ds"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
	"github.com/eru-os/eru/eru-ql/qlcache"
	"github.com/eru-os/eru/eru-read-write/eru_writes"
)

type SQLData struct {
	QLData
	DBAlias   string `json:"db_alias"`
	Cols      string `json:"cols"`
	CacheTTL  int    `json:"cache_ttl,omitempty"`
	CacheSkip bool   `json:"cache_skip,omitempty"`
	CacheLock bool   `json:"cache_lock,omitempty"`
}

func (sqd *SQLData) SetQLData(ctx context.Context, mq module_model.MyQuery, vars map[string]interface{}, executeFlag bool, tokenObj map[string]interface{}, isPublic bool, outputType string) {
	logs.WithContext(ctx).Debug("SetQLData - Start")
	sqd.SetQLDataCommon(ctx, mq, vars, executeFlag, tokenObj, isPublic, outputType)
	//sqd.Query=mq.Query
	//sqd.Variables=mq.Vars
	sqd.DBAlias = mq.DBAlias
	sqd.Cols = mq.Cols
	sqd.CacheTTL = mq.CacheTTL
	sqd.CacheSkip = mq.CacheSkip
	sqd.CacheLock = mq.CacheLock
	if vars != nil {
		if v, ok := vars["cache_ttl"]; ok {
			switch n := v.(type) {
			case float64:
				sqd.CacheTTL = int(n)
			case int:
				sqd.CacheTTL = n
			}
			delete(vars, "cache_ttl")
		}
		if v, ok := vars["cache_skip"]; ok {
			if b, bok := v.(bool); bok {
				sqd.CacheSkip = b
			}
			delete(vars, "cache_skip")
		}
		if v, ok := vars["cache_lock"]; ok {
			if b, bok := v.(bool); bok {
				sqd.CacheLock = b
			}
			delete(vars, "cache_lock")
		}
	}
	//sqd.SetFinalVars(vars)
}
func (sqd *SQLData) Execute(ctx context.Context, projectId string, datasources map[string]*module_model.DataSource, s module_store.ModuleStoreI, outputType string) (res []map[string]interface{}, queryObjs []QueryObject, err error) {
	logs.WithContext(ctx).Debug("Execute of Sql - Start")
	datasource := datasources[sqd.DBAlias]
	if datasource == nil {
		return nil, nil, errors.New(fmt.Sprint("dbAlias ", sqd.DBAlias, " not found"))
	}
	var result map[string]interface{}
	sr := ds.GetSqlMaker(datasource.DbName)

	var keyList []string
	for key, _ := range sqd.FinalVariables {
		keyList = append(keyList, key)
	}
	sort.Strings(keyList)
	for _, k := range keyList {
		v := sqd.FinalVariables[k]
		var str string
		switch tp := v.(type) {
		case []interface{}:
			isMap := false
			if iArray, ok := v.([]interface{}); ok {
				for i, strText := range iArray {
					isMap = false
					sep := ""
					if i > 0 {
						sep = " , "
					}
					if txt, txtOk := strText.(string); txtOk {
						str = fmt.Sprint(str, sep, "'", txt, "'")
					} else if _, mapVOk := strText.(map[string]interface{}); mapVOk {
						isMap = true
						break
					}
				}
				if isMap {
					mapJ, mapJErr := json.Marshal(iArray)
					if mapJErr != nil {
						logs.WithContext(ctx).Error(mapJErr.Error())
						return nil, nil, mapJErr
					}
					str = string(mapJ)
				}
			}
			sqd.FinalVariables[k] = str
			sqd.Query = strings.Replace(sqd.Query, fmt.Sprint("$", k), str, -1)
			break
		default:
			// do noting
			_ = tp
			break
		}
	}

	for _, k := range keyList {
		v := sqd.FinalVariables[k]
		//ignoring processing token variable
		if k != module_model.RULEPREFIX_TOKEN {

			err = sr.VerifyForBlockedWords(ctx, k, v, sr)
			if err != nil {
				return
			}

			var str string
			switch tp := v.(type) {
			case float64:
				str = fmt.Sprint(v.(float64))
				break
			case string:
				str = v.(string)
				vBytes, err := processTemplate(ctx, "variable", str, sqd.FinalVariables, "string", "", nil)
				if err != nil {
					return nil, nil, err
				}
				if string(vBytes) != "" {
					str = string(vBytes)
				}
				break
			case map[string]interface{}:
				strBytes, strBytesErr := json.Marshal(v)
				err = strBytesErr
				str = string(strBytes)
				break
			default:
				logs.WithContext(ctx).Warn(fmt.Sprint("Unhandled type for : ", tp))
				// do noting
				break
			}
			//logs.WithContext(ctx).Info(fmt.Sprint(k, " = ", str))
			sqd.FinalVariables[k] = str
			sqd.Query = strings.Replace(sqd.Query, fmt.Sprint("$", k), str, -1)
		}
	}

	// Rewrite qualified references to derived fields before anything else looks
	// at the SQL, so secureSQL sees the final table set including hop targets
	// and cache tags come out right.
	sqd.Query, err = sqd.rewriteDerivedFields(ctx, projectId, sqd.Query, datasource, s, sr)
	if err != nil {
		return nil, nil, err
	}

	queryObj := QueryObject{}
	queryObj.Query = sqd.Query
	queryObj.Cols = sqd.Cols
	if sqd.ExecuteFlag {
		sqd.Query = sqd.secureSQL(ctx, sqd.Query, projectId, datasource, s, sr)
		if !qlcache.IsDML(sqd.Query) {
			sqd.Query, err = sqd.wrapQuery(ctx, sqd.Query, sr)
			if err != nil {
				return nil, nil, err
			}
		} else if err = sqd.verifyWrapApplicable(ctx); err != nil {
			return nil, nil, err
		}
		logs.WithContext(ctx).Info(sqd.Query)
		ctx = ds.WithUseWriter(ctx, sqd.UseWriter || qlcache.IsDML(sqd.Query))
		if eru_writes.IsColumnarOutput(sqd.OutputType) {
			result, err = sr.ExecuteQueryForCsv(ctx, sqd.Query, datasource, "Results", sr)
			if err != nil {
				err = logs.Err(ctx, err, "")
			}
			queryObj.DataTypes = sr.GetResultDataTypes(ctx)
			if result != nil {
				res = append(res, result)
			}
		} else {
			if qlcache.IsDML(sqd.Query) {
				result, err = sr.ExecutePreparedQuery(ctx, sqd.Query, datasource)
				if err != nil {
					err = logs.Err(ctx, err, "")
				} else {
					targets := sr.ExtractDMLTargetTables(ctx, sqd.Query)
					qlcache.EnqueueInvalidate(ctx, datasource, qlcache.QualifyTables(targets, sr.DefaultSchemaName()))
				}
			} else {
				queryDesc := sqd.QueryName
				if queryDesc == "" {
					queryDesc = sqd.Query
				}
				loader := func(ctx context.Context) (map[string]interface{}, []string, error) {
					logs.WithContext(ctx).Info(fmt.Sprintf("cache not found, executing query %s", queryDesc))
					r, lerr := sr.ExecutePreparedQuery(ctx, sqd.Query, datasource)
					if lerr != nil {
						return nil, nil, lerr
					}
					tbls := sr.ExtractTableNames(ctx, sqd.Query)
					names := make([]string, 0, len(tbls.Tables))
					for _, t := range tbls.Tables {
						if t.TableName != "" {
							names = append(names, t.TableName)
						}
					}
					return r, names, nil
				}
				result, err = qlcache.ServeOrLoad(ctx, datasource, sqd.TenantId, sqd.Query, sr.DefaultSchemaName(), loader, qlcache.Options{
					TTLSec:    sqd.CacheTTL,
					SkipCache: sqd.CacheSkip,
					LockHot:   sqd.CacheLock,
					QueryName: sqd.QueryName,
				})
				if err != nil {
					err = logs.Err(ctx, err, "")
				}
			}
			if result != nil {
				res = append(res, result)
			}
		}
	} else if sqd.OutputType == "ast" {
		secureQuery := sqd.secureSQL(ctx, sqd.Query, projectId, datasource, s, sr)
		if !qlcache.IsDML(secureQuery) {
			secureQuery, err = sqd.wrapQuery(ctx, secureQuery, sr)
			if err != nil {
				return nil, nil, err
			}
		} else if err = sqd.verifyWrapApplicable(ctx); err != nil {
			return nil, nil, err
		}
		res = append(res, map[string]interface{}{"sql": secureQuery})
	}
	queryObjs = append(queryObjs, queryObj)
	return res, queryObjs, err
}

// rewriteDerivedFields turns table-qualified references to derived fields into
// their compiled expressions. Postgres only - the base maker cannot parse a
// statement.
func (sqd *SQLData) rewriteDerivedFields(ctx context.Context, projectId string, query string, datasource *module_model.DataSource, s module_store.ModuleStoreI, sr ds.SqlMakerI) (string, error) {
	if len(datasource.DerivedFields) == 0 {
		return query, nil
	}
	limits := module_model.DefaultDerivedFieldLimits()
	if ps, psErr := s.GetProjectSettingsObject(ctx, projectId); psErr == nil {
		limits = ps.DerivedFieldLimits.WithDefaults()
	}
	return sr.RewriteDerivedFields(ctx, ds.DerivedRewriteRequest{
		Query:      query,
		DataSource: datasource,
		// secureSQL runs on the rewritten statement and wraps every table it
		// finds, so the hop targets this introduces are secured there rather
		// than per hop. Applying the rule in both places produces invalid SQL.
		Security: derived.SourceSecuredDownstream,
		Limits:   limits,
	})
}
