package handlers

import (
	"bytes"
	"context"
	b64 "encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
	"github.com/eru-os/eru/eru-ql/ql"
	eru_writes "github.com/eru-os/eru/eru-read-write/eru_writes"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	//"../server"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // TODO to check origin as per env variable
	},
}

type Client struct {
	conn *websocket.Conn
	id   string
	// other relevant fields like subscribed channels or user preferences
}

var clients = make(map[string]*Client)
var lock = sync.RWMutex{}

func ProjectMyQuerySaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectMyQuerySaveHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		queryName := vars["queryname"]
		queryType := vars["querytype"]

		var err error
		if queryType == "graphql" {
			var gqd ql.GraphQLData
			if err := json.NewDecoder(r.Body).Decode(&gqd); err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				logs.WithContext(r.Context()).Error(err.Error())
				return
			}
			if strings.Contains(gqd.Query, "query IntrospectionQuery") {
				server_handlers.FormatResponse(w, 200)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "Query Introspection not implemented"})
				return
			}
			err = sh.Store.SaveMyQuery(r.Context(), projectID, vars["tenantId"], queryName, queryType, "", gqd.Query, gqd.Variables, sh.Store, "", gqd.SecurityRule, 0, false, false)
		} else if queryType == "sql" {
			var sqd ql.SQLData
			if err := json.NewDecoder(r.Body).Decode(&sqd); err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				logs.WithContext(r.Context()).Error(err.Error())
				return
			}

			err = sh.Store.SaveMyQuery(r.Context(), projectID, vars["tenantId"], queryName, queryType, sqd.DBAlias, sqd.Query, sqd.Variables, sh.Store, sqd.Cols, sqd.SecurityRule, sqd.CacheTTL, sqd.CacheSkip, sqd.CacheLock)
		} else {
			err = errors.New("Incorrect query type")
		}
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})

		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprint("Query ", queryName, " saved successfully")})
		}
		return
	}
}

func ProjectMyQueryRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectMyQueryRemoveHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		queryName := vars["queryname"]

		err := sh.Store.RemoveMyQuery(r.Context(), projectID, vars["tenantId"], queryName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprint("Query ", queryName, " removed successfully")})
		}
		return
	}
}

func ProjectMyQueryListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectMyQueryListHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		queryType := vars["querytype"]

		myqueries, err := sh.Store.GetMyQueries(r.Context(), projectID, vars["tenantId"], queryType)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"myqueries": myqueries})
		}
		return
	}
}

func ProjectMyQueryListNamesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectMyQueryListNamesHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]

		myqueries, err := sh.Store.GetMyQueriesNames(r.Context(), projectID, vars["tenantId"])
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"myqueries": myqueries})
		}
		return
	}
}

func ProjectMyQueryConfigHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectMyQueryConfigHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		queryName := vars["queryname"]

		myquery, err := sh.Store.GetMyQuery(r.Context(), projectID, vars["tenantId"], queryName)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"myquery": myquery})
		}
		return
	}
}

func ProjectMyQueryASTHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectMyQueryASTHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		queryName := vars["queryname"]

		projectSettings, err := sh.Store.GetProjectSettingsObject(r.Context(), projectID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		tokenObj := make(map[string]interface{})
		tokenStr := r.Header.Get(projectSettings.ClaimsKey)
		if tokenStr != "" {
			err = json.Unmarshal([]byte(tokenStr), &tokenObj)
			if err != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprint("error while unmarshalling token claim : ", err.Error()))
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}

		postBody := make(map[string]interface{})

		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			logs.WithContext(r.Context()).Error(err.Error())
			return
		}

		datasources, err := sh.Store.GetDataSources(r.Context(), projectID, vars["tenantId"])
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		var res []map[string]interface{}
		//var queries []string
		myQuery, err := sh.Store.GetMyQuery(r.Context(), projectID, vars["tenantId"], queryName)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			logs.WithContext(r.Context()).Error(err.Error())
			return
		}
		// overwriting variables with same names
		if myQuery.QueryName != "" {
			qlInterface := ql.GetQL(myQuery.QueryType)
			if qlInterface == nil {
				server_handlers.FormatResponse(w, 400)
				err = errors.New("Invalid Query Type")
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				logs.WithContext(r.Context()).Error(err.Error())
				return
			}
			isPublic := false
			isPublic, err = strconv.ParseBool(r.Header.Get("is_public"))
			if err != nil {
				// do nothing - silently execute with is_public as false
			}
			qlInterface.SetQLData(r.Context(), myQuery, postBody, false, tokenObj, isPublic, "ast")
			qlInterface.SetTenantId(vars["tenantId"])
			res, _, err = qlInterface.Execute(r.Context(), projectID, datasources, sh.Store, "ast")
			/*
				if err != nil {
					server_handlers.FormatResponse(w, 400)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					logs.WithContext(r.Context()).Error(err.Error())
					return
				}
			*/
		} else {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": errors.New(fmt.Sprint("query ", queryName, " not found")).Error()})
			return
		}
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			if res == nil {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			server_handlers.FormatResponse(w, 200)
		}
		_ = json.NewEncoder(w).Encode(res)
		return
	}
}

func remarshal(data interface{}, target interface{}) (err error) {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

type contextKey string

const groupByModeKey contextKey = "groupByMode"

func ProjectMyQueryExecuteGroupHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	h := ProjectMyQueryExecuteHandler(sh)
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r.WithContext(context.WithValue(r.Context(), groupByModeKey, true)))
	}
}

func ProjectMyQueryExecuteHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectMyQueryExecuteHandler - Start")
		//logs.WithContext(r.Context()).Info(time.Now(), " Start --------------------------------------------------- ")
		//time.Sleep(time.Duration(3000) * time.Millisecond)
		//logs.WithContext(r.Context()).Info(time.Now(), " End --------------------------------------------------- ")
		//claims := r.Header.Get("claims")

		vars := mux.Vars(r)
		projectID := vars["project"]
		queryName := vars["queryname"]
		outputType := vars["outputtype"]
		encode := vars["encode"]

		projectSettings, err := sh.Store.GetProjectSettingsObject(r.Context(), projectID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		tokenObj := make(map[string]interface{})
		tokenStr := r.Header.Get(projectSettings.ClaimsKey)
		if tokenStr != "" {
			err = json.Unmarshal([]byte(tokenStr), &tokenObj)
			if err != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprint("error while unmarshalling token claim : ", err.Error()))
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}

		postBody := make(map[string]interface{})

		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			logs.WithContext(r.Context()).Error(err.Error())
			return
		}

		datasources, err := sh.Store.GetDataSources(r.Context(), projectID, vars["tenantId"])
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		var res []map[string]interface{}
		//var queries []string
		myQuery, err := sh.Store.GetMyQuery(r.Context(), projectID, vars["tenantId"], queryName)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			logs.WithContext(r.Context()).Error(err.Error())
			return
		}
		var qobjs []ql.QueryObject
		excelStyles := make(map[string]eru_writes.CellFormatter)
		if excelStylesData, excelStylesOk := postBody["excel_styles"]; excelStylesOk {
			// Marshal and unmarshal to convert interface{} to proper type
			excelStylesBytes, err := json.Marshal(excelStylesData)
			if err == nil {
				err = json.Unmarshal(excelStylesBytes, &excelStyles)
				if err != nil {
					logs.WithContext(r.Context()).Error(err.Error())
				}
			} else {
				logs.WithContext(r.Context()).Error(err.Error())
			}
			delete(postBody, "excel_styles")
		}
		columns := make(map[string]eru_writes.ColumnarSettings)
		if columnsData, columnsOk := postBody["columns"]; columnsOk {
			columnsBytes, err := json.Marshal(columnsData)
			if err == nil {
				err = json.Unmarshal(columnsBytes, &columns)
				if err != nil {
					logs.WithContext(r.Context()).Error(err.Error())
				}
			} else {
				logs.WithContext(r.Context()).Error(err.Error())
			}
			delete(postBody, "columns")
		}
		var excludeColumns []string
		if excludeColumnsData, excludeColumnsOk := postBody["exclude_columns"]; excludeColumnsOk {
			excludeColumnsBytes, err := json.Marshal(excludeColumnsData)
			if err == nil {
				err = json.Unmarshal(excludeColumnsBytes, &excludeColumns)
				if err != nil {
					logs.WithContext(r.Context()).Error(err.Error())
				}
			} else {
				logs.WithContext(r.Context()).Error(err.Error())
			}
			delete(postBody, "exclude_columns")
		}
		pivotConfig := make(map[string]eru_writes.PivotTableConfig)
		if pivotConfigData, pivotConfigOk := postBody["pivot_config"]; pivotConfigOk {
			pivotConfigBytes, err := json.Marshal(pivotConfigData)
			if err == nil {
				err = json.Unmarshal(pivotConfigBytes, &pivotConfig)
				if err != nil {
					logs.WithContext(r.Context()).Error(err.Error())
				}
			} else {
				logs.WithContext(r.Context()).Error(err.Error())
			}
			delete(postBody, "pivot_config")
		}
		imageConfig := myQuery.ImageConfig
		if imageConfigData, imageConfigOk := postBody["image_config"]; imageConfigOk {
			imageConfigBytes, err := json.Marshal(imageConfigData)
			if err == nil {
				err = json.Unmarshal(imageConfigBytes, &imageConfig)
				if err != nil {
					logs.WithContext(r.Context()).Error(err.Error())
				}
			} else {
				logs.WithContext(r.Context()).Error(err.Error())
			}
			delete(postBody, "image_config")
		}
		// overwriting variables with same names
		if myQuery.QueryName != "" {
			qlInterface := ql.GetQL(myQuery.QueryType)
			if qlInterface == nil {
				server_handlers.FormatResponse(w, 400)
				err = errors.New("Invalid Query Type")
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				logs.WithContext(r.Context()).Error(err.Error())
				return
			}
			isPublic := false
			isPublic, err = strconv.ParseBool(r.Header.Get("is_public"))
			if err != nil {
				// do nothing - silently execute with is_public as false
			}

			groupMode, _ := r.Context().Value(groupByModeKey).(bool)
			if groupMode {
				groupByConfig := module_model.GroupByConfig{Active: true}
				if gbData, gbOk := postBody["group_by"]; gbOk {
					if gErr := remarshal(gbData, &groupByConfig.GroupBy); gErr != nil {
						server_handlers.FormatResponse(w, 400)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("invalid group_by : ", gErr.Error())})
						logs.WithContext(r.Context()).Error(gErr.Error())
						return
					}
					delete(postBody, "group_by")
				}
				if aggData, aggOk := postBody["aggregations"]; aggOk {
					if gErr := remarshal(aggData, &groupByConfig.Aggregations); gErr != nil {
						server_handlers.FormatResponse(w, 400)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("invalid aggregations : ", gErr.Error())})
						logs.WithContext(r.Context()).Error(gErr.Error())
						return
					}
					delete(postBody, "aggregations")
				}
				if gobData, gobOk := postBody["group_order_by"]; gobOk {
					if gErr := remarshal(gobData, &groupByConfig.GroupOrderBy); gErr != nil {
						server_handlers.FormatResponse(w, 400)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("invalid group_order_by : ", gErr.Error())})
						logs.WithContext(r.Context()).Error(gErr.Error())
						return
					}
					delete(postBody, "group_order_by")
				}
				qlInterface.SetGroupBy(groupByConfig)
			}

			var wrapConfig module_model.QueryWrapConfig
			if filterData, fOk := postBody["qfilter"]; fOk {
				if fm, fmOk := filterData.(map[string]interface{}); fmOk {
					wrapConfig.Filter = fm
				}
				delete(postBody, "qfilter")
			}
			if sortData, sOk := postBody["qsort"]; sOk {
				sortBytes, mErr := json.Marshal(sortData)
				if mErr == nil {
					if uErr := json.Unmarshal(sortBytes, &wrapConfig.Sort); uErr != nil {
						logs.WithContext(r.Context()).Error(uErr.Error())
					}
				}
				delete(postBody, "qsort")
			}
			if limitData, lOk := postBody["qlimit"]; lOk {
				switch n := limitData.(type) {
				case float64:
					wrapConfig.Limit = int(n)
				case int:
					wrapConfig.Limit = n
				}
				delete(postBody, "qlimit")
			}
			if skipData, skOk := postBody["qskip"]; skOk {
				switch n := skipData.(type) {
				case float64:
					wrapConfig.Skip = int(n)
				case int:
					wrapConfig.Skip = n
				}
				delete(postBody, "qskip")
			}
			qlInterface.SetWrapConfig(wrapConfig)

			qlInterface.SetQLData(r.Context(), myQuery, postBody, true, tokenObj, isPublic, outputType)
			qlInterface.SetTenantId(vars["tenantId"])
			res, qobjs, err = qlInterface.Execute(r.Context(), projectID, datasources, sh.Store, outputType)
			/*
				if err != nil {
					server_handlers.FormatResponse(w, 400)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					logs.WithContext(r.Context()).Error(err.Error())
					return
				}
			*/
		} else {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": errors.New(fmt.Sprint("query ", queryName, " not found")).Error()})
			return
		}
		_ = qobjs
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			if res == nil {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else if outputType == eru_writes.OutputTypeExcel {
			columns = eru_writes.MergeColumnarSettings(columns, myQuery.Columns)
			excelStyles = eru_writes.MergeCellFormattersMap(excelStyles, myQuery.ExcelStyles)
			pivotConfig = eru_writes.MergePivotConfigs(pivotConfig, myQuery.PivotConfig)
			if len(excludeColumns) == 0 {
				excludeColumns = myQuery.ExcludeColumns
			}

			wd, wdErr := buildColumnarWriteData(r.Context(), res, qobjs, columns, excludeColumns, fmt.Sprintf("%s.xlsx", queryName))
			if wdErr != nil {
				err = wdErr
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}

			ewd := eru_writes.ExcelWriteData{
				WriteData:   wd,
				CellFormat:  excelStyles,
				PivotConfig: pivotConfig,
			}
			var b []byte // creates IO Writer
			b, err = ewd.WriteColumnar(r.Context())
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			if encode == "encode" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"file": b64.StdEncoding.EncodeToString(b)})
			} else {
				w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
				w.Header().Set("Content-Disposition", "attachment; filename=query.xlsx")
				_, _ = io.Copy(w, bytes.NewReader(b))
			}
			w.WriteHeader(http.StatusOK)
			return
		} else if outputType == eru_writes.OutputTypeHtml || outputType == eru_writes.OutputTypeImage {
			columns = eru_writes.MergeColumnarSettings(columns, myQuery.Columns)
			excelStyles = eru_writes.MergeCellFormattersMap(excelStyles, myQuery.ExcelStyles)
			if len(excludeColumns) == 0 {
				excludeColumns = myQuery.ExcludeColumns
			}

			wd, wdErr := buildColumnarWriteData(r.Context(), res, qobjs, columns, excludeColumns, queryName)
			if wdErr != nil {
				err = wdErr
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}

			hwd := eru_writes.HtmlWriteData{
				WriteData:  wd,
				CellFormat: excelStyles,
			}
			var b []byte
			b, err = hwd.WriteColumnar(r.Context())
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}

			contentType := "text/html; charset=utf-8"
			fileName := fmt.Sprint("query.", eru_writes.OutputTypeHtml)
			if outputType == eru_writes.OutputTypeImage {
				b, err = HtmlToImage(r.Context(), b, imageConfig)
				if err != nil {
					server_handlers.FormatResponse(w, 400)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
					return
				}
				imageFormat := imageConfig.Format
				if imageFormat == "" {
					imageFormat = ImageFormatPng
				}
				contentType = ImageContentType(imageFormat)
				fileName = fmt.Sprint("query.", strings.ToLower(imageFormat))
			}

			if encode == "encode" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"file": b64.StdEncoding.EncodeToString(b)})
			} else {
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Content-Disposition", fmt.Sprint("inline; filename=", fileName))
				_, _ = io.Copy(w, bytes.NewReader(b))
			}
			w.WriteHeader(http.StatusOK)
			return
		} else if outputType == eru_writes.OutputTypeCsv {
			if len(excludeColumns) == 0 {
				excludeColumns = myQuery.ExcludeColumns
			}
			b := &bytes.Buffer{} // creates IO Writer
			ww := csv.NewWriter(b)
			for _, v := range res {
				for _, csvData := range v {
					if records, ok := csvData.([][]interface{}); ok {
						if len(records[0]) > 0 {
							var csvStrData [][]string
							tmpArray, tmpErr := json.Marshal(records)
							if tmpErr != nil {
								err = tmpErr
								server_handlers.FormatResponse(w, 400)
								_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
							}
							tmpErr = json.Unmarshal(tmpArray, &csvStrData)
							if tmpErr != nil {
								err = tmpErr
								server_handlers.FormatResponse(w, 400)
								_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
							}
							excludedCols := eru_writes.ExcludedColumnIndices(records[0], excludeColumns)
							if excludedCols == nil {
								ww.WriteAll(csvStrData)
							} else {
								for _, row := range csvStrData {
									outRow := make([]string, 0, len(row))
									for ci, c := range row {
										if excludedCols[ci] {
											continue
										}
										outRow = append(outRow, c)
									}
									ww.Write(outRow)
								}
							}
						}
					} else {
						err = errors.New(fmt.Sprint("inccorect csv data format"))
						server_handlers.FormatResponse(w, 400)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
					}
				}
			}
			if encode == "encode" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"file": b64.StdEncoding.EncodeToString(b.Bytes())})
			} else {
				w.Header().Set("Content-Type", "text/csv")
				w.Header().Set("Content-Disposition", "attachment; filename=query.csv")
				_, _ = io.Copy(w, bytes.NewReader(b.Bytes()))
			}
			w.WriteHeader(http.StatusOK)
			return
		} else {
			server_handlers.FormatResponse(w, 200)
		}
		_ = json.NewEncoder(w).Encode(res)
		//logs.WithContext(r.Context()).Info(fmt.Sprint("---------------------------"))
		//logs.WithContext(r.Context()).Info(fmt.Sprint(w.Header()))
		return
	}
}

func GraphqlWsExecuteHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("GraphqlExecuteHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		outputType := vars["outputtype"]
		logs.WithContext(r.Context()).Info(fmt.Sprint(projectID))
		logs.WithContext(r.Context()).Info(fmt.Sprint(outputType))
		respHeader := make(http.Header)
		respHeader.Add("Sec-WebSocket-Protocol", "graphql-ws")
		conn, err := upgrader.Upgrade(w, r, respHeader) // Upgrade HTTP to WebSocket
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			if err = conn.WriteMessage(websocket.TextMessage, []byte("connection error")); err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				return
			}
		}
		defer conn.Close()

		client := &Client{conn: conn, id: uuid.New().String()}
		lock.Lock()
		clients[client.id] = client
		lock.Unlock()

		defer func() {
			lock.Lock()
			delete(clients, client.id)
			lock.Unlock()
		}()
		m := make(map[string]interface{})
		for {
			if err = conn.ReadJSON(&m); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logs.WithContext(r.Context()).Error("IsUnexpectedCloseError")
					logs.WithContext(r.Context()).Error(err.Error())
					break
				} else {
					logs.WithContext(r.Context()).Error(err.Error())
					_ = conn.WriteJSON(map[string]interface{}{"error": err.Error()})
					return
				}
			}

			// Logic to check database for updates
			time.Sleep(10 * time.Second) // Polling every 10 seconds
			// Suppose we have an update
			wmessage := "Update detected in the database"
			if err := conn.WriteMessage(websocket.TextMessage, []byte(wmessage)); err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				return
			}
		}
	}
}

func GraphqlExecuteHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("GraphqlExecuteHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		outputType := vars["outputtype"]
		projectSettings, err := sh.Store.GetProjectSettingsObject(r.Context(), projectID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		tokenObj := make(map[string]interface{})
		tokenStr := r.Header.Get(projectSettings.ClaimsKey)

		if tokenStr != "" {
			err = json.Unmarshal([]byte(tokenStr), &tokenObj)
			if err != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprint("error while unmarshalling token claim : ", err.Error()))
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}
		datasources, err := sh.Store.GetDataSources(r.Context(), projectID, vars["tenantId"])
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		var gqd ql.GraphQLData
		gqd.IsPublic = false
		gqd.IsPublic, err = strconv.ParseBool(r.Header.Get("is_public"))
		if err != nil {
			// do nothing - silently execute with is_public as false
		}

		if err := json.NewDecoder(r.Body).Decode(&gqd); err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			logs.WithContext(r.Context()).Error(err.Error())
			return
		}

		if gqd.Variables == nil {
			gqd.Variables = make(map[string]interface{})
		}
		gqd.Variables[module_model.RULEPREFIX_TOKEN] = tokenObj
		gqd.FinalVariables = gqd.Variables
		gqd.ExecuteFlag = true
		gqd.TenantId = vars["tenantId"]

		res, queryObjs, err := gqd.Execute(r.Context(), projectID, datasources, sh.Store, outputType)
		_ = queryObjs
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			if res == nil {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			server_handlers.FormatResponse(w, 200)
		}
		_ = json.NewEncoder(w).Encode(res)
		return
	}
}

func SqlExecuteHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SqlExecuteHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		outputType := vars["outputtype"]
		projectSettings, err := sh.Store.GetProjectSettingsObject(r.Context(), projectID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		tokenObj := make(map[string]interface{})
		tokenStr := r.Header.Get(projectSettings.ClaimsKey)
		if tokenStr != "" {
			err = json.Unmarshal([]byte(tokenStr), &tokenObj)
			if err != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprint("error while unmarshalling token claim : ", err.Error()))
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}

		datasources, err := sh.Store.GetDataSources(r.Context(), projectID, vars["tenantId"])
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		var sqd ql.SQLData
		sqd.IsPublic = false
		sqd.IsPublic, err = strconv.ParseBool(r.Header.Get("is_public"))
		if err != nil {
			// do nothing - silently execute with is_public as false
		}

		if err := json.NewDecoder(r.Body).Decode(&sqd); err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			logs.WithContext(r.Context()).Error(err.Error())
			return
		}

		if sqd.Variables == nil {
			sqd.Variables = make(map[string]interface{})
		}
		sqd.Variables[module_model.RULEPREFIX_TOKEN] = tokenObj
		sqd.FinalVariables = sqd.Variables
		sqd.ExecuteFlag = true
		sqd.TenantId = vars["tenantId"]
		res, queryObjs, err := sqd.Execute(r.Context(), projectID, datasources, sh.Store, outputType)
		_ = queryObjs
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			if res == nil {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			server_handlers.FormatResponse(w, 200)
		}
		_ = json.NewEncoder(w).Encode(res)
		return
	}
}

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/hello" {
		http.Error(w, "404 not found.", http.StatusNotFound)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}
	res := make(map[string]string)
	res["Hellow"] = "World"
	server_handlers.FormatResponse(w, 200)
	_ = json.NewEncoder(w).Encode(res)
}

func EchoHandler(w http.ResponseWriter, r *http.Request) {
	res := make(map[string]interface{})
	res["Host"] = r.Host
	res["Header"] = r.Header
	res["URL"] = r.URL
	res["Body"] = r.Body
	res["Method"] = r.Method
	res["MultipartForm"] = r.MultipartForm
	res["RequestURI"] = r.RequestURI
	server_handlers.FormatResponse(w, 200)
	_ = json.NewEncoder(w).Encode(res)

}
