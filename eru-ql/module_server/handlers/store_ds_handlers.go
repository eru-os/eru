package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	common_types "github.com/eru-os/eru/eru-ql/common_types"
	"github.com/eru-os/eru/eru-ql/ds"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
	//"../server"
)

func DefaultDriverConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("DefaultDriverConfigHandler - Start")
		vars := mux.Vars(r)
		dbType := vars["dbType"] // not used for now but we may have different defaults for different db type
		_ = dbType
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"driverconfig": ds.DefaultDriverConfig})
	}
}

func DefaultOtherDBConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("DefaultOtherDBConfigHandler - Start")
		vars := mux.Vars(r)
		dbType := vars["dbType"] // not used for now but we may have different defaults for different db type
		_ = dbType
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"OtherDbConfig": ds.DefaultOtherConfig})
	}
}

func DefaultDBSecurityRulesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("DefaultDBSecurityRulesHandler - Start")
		vars := mux.Vars(r)
		dbType := vars["dbType"] // not used for now but we may have different defaults for different db type
		_ = dbType
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"DbSecurityRules": ds.DefaultDbSecurityRules})
	}
}

func DefaultReadPolicyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("DefaultReadPolicyHandler - Start")
		vars := mux.Vars(r)
		dbType := vars["dbType"]
		_ = dbType
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ReadPolicy": ds.DefaultReadPolicy})
	}
}

func ProjectDataSourceConfigHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectDataSourceConfigHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		datasource, err := sh.Store.GetDataSource(r.Context(), projectId, vars["tenantId"], dbAlias)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"datasource": datasource})
		return
	}
}

func ProjectDataSourceSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		// Read the request body bytes
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "Failed to read request body"})
			return
		}

		// Validate JSON structure by decoding into map first
		dsFromReqMap := make(map[string]interface{})
		if err := json.Unmarshal(bodyBytes, &dsFromReqMap); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "Invalid JSON format"})
			return
		}
		var datasource module_model.DataSource

		// Unmarshal the body bytes directly into DataSource (this will call your custom UnmarshalJSON method)
		if err := json.Unmarshal(bodyBytes, &datasource); err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := eru_utils.ValidateStruct(r.Context(), datasource, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
			if verr := validateReadDbConfigs(datasource.ReadDbConfigs); verr != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": verr.Error()})
				return
			}
		}
		err = sh.Store.SaveDataSource(r.Context(), projectId, vars["tenantId"], &datasource, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{dbAlias: datasource})
		}
		return
	}
}

func ProjectDataSourceRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]

		err := sh.Store.RemoveDataSource(r.Context(), projectId, vars["tenantId"], dbAlias, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("datasource config for ", dbAlias, " removed successfully")})
		}
		return
	}
}

func ProjectDataSourceListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectDataSourceListHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		datasources, err := sh.Store.GetDataSourcesList(r.Context(), projectId, vars["tenantId"])
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"datasources": datasources})
		}
		return
	}
}
func ProjectDataSourceTableCheckHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceTableCheckHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]

		columns, schema, err := sh.Store.CheckTableExists(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, sh.Store)
		if err != nil && schema == "" {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "schema": schema})
		}
		return
	}
}
func ProjectDataSourceSchemaHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]

		datasource, err := sh.Store.UpdateSchemaTables(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"SchemaTables": datasource.SchemaTables, "OtherTables": datasource.OtherTables})
		}
		return
	}
}

func ProjectDataSourceSchemaAddTableHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaAddTableHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]

		res, err := sh.Store.AddSchemaTable(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		}
		return
	}
}

func ProjectDataSourceSchemaRemoveTableHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaRemoveTableHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]

		res, err := sh.Store.RemoveSchemaTable(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		}
		return
	}
}
func ProjectDataSourceSchemaAddJoinHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaAddJoinHandler - Start")
		vars := mux.Vars(r)

		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		var tj module_model.TableJoins
		if err := json.NewDecoder(r.Body).Decode(&tj); err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			logs.WithContext(r.Context()).Error(err.Error())
			return
		}

		res, err := sh.Store.AddSchemaJoin(r.Context(), projectId, vars["tenantId"], dbAlias, &tj, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		}
		return
	}
}
func ProjectDataSourceSchemaRemoveJoinHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaRemoveJoinHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		var tj module_model.TableJoins
		if err := json.NewDecoder(r.Body).Decode(&tj); err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			logs.WithContext(r.Context()).Error(err.Error())
			return
		}

		res, err := sh.Store.RemoveSchemaJoin(r.Context(), projectId, vars["tenantId"], dbAlias, &tj, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		}
		return
	}
}

func ProjectDataSourceSchemaSaveTableHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaSaveTableHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]
		addInSchemaStr := vars["addInSchema"]
		tenantId := vars["tenantId"]
		tableName = strings.Replace(tableName, "___", ".", 1)

		tableFromReq := json.NewDecoder(r.Body)
		tableFromReq.DisallowUnknownFields()

		var tableObj map[string]common_types.TableColsMetaData

		if err := tableFromReq.Decode(&tableObj); err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			for _, v := range tableObj {
				err := eru_utils.ValidateStruct(r.Context(), v, "")
				if err != nil {
					server_handlers.FormatResponse(w, 400)
					json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
					return
				}
			}
		}

		addInSchema := false
		if addInSchemaStr == "true" {
			addInSchema = true
		}
		err := sh.Store.SaveSchemaTable(r.Context(), projectId, tenantId, dbAlias, tableName, tableObj, sh.Store, addInSchema)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Table ", tableName, " created successfully")})
		}
		return
	}
}
func ProjectDataSourceSchemaTransformTableHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaTransformTableHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]
		tableName = strings.Replace(tableName, "___", ".", 1)

		transformRulesFromReq := json.NewDecoder(r.Body)
		transformRulesFromReq.DisallowUnknownFields()

		var transformRules module_model.TransformRules

		if err := transformRulesFromReq.Decode(&transformRules); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := eru_utils.ValidateStruct(r.Context(), transformRules, "")
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		err := sh.Store.SaveTableTransformation(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, transformRules, sh.Store)

		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Table Transformation for ", tableName, " set successfully")})
		}
		return
	}
}

func ProjectDataSourceSchemaMasColumnHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaMasColumnHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]
		colName := vars["colname"]
		tableName = strings.Replace(tableName, "___", ".", 1)

		columnMaskingFromReq := json.NewDecoder(r.Body)
		columnMaskingFromReq.DisallowUnknownFields()

		var columnMasking common_types.ColumnMasking

		if err := columnMaskingFromReq.Decode(&columnMasking); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := eru_utils.ValidateStruct(r.Context(), columnMasking, "")
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		err := sh.Store.SaveColumnMasking(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, colName, columnMasking, sh.Store)

		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("column masking for ", colName, " set successfully")})
		}
		return
	}
}

func ProjectDataSourceRemoveSecureTableHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceRemoveSecureTableHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]
		tableName = strings.Replace(tableName, "___", ".", 1)

		securityRulesFromReq := json.NewDecoder(r.Body)
		securityRulesFromReq.DisallowUnknownFields()

		err := sh.Store.RemoveTableSecurity(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Table Security for ", tableName, " removed successfully")})
		}
	}
}

func ProjectDataSourceGetSecureTableHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceGetSecureTableHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]
		tableName = strings.Replace(tableName, "___", ".", 1)

		securityRulesFromReq := json.NewDecoder(r.Body)
		securityRulesFromReq.DisallowUnknownFields()

		sr, err := sh.Store.GetTableSecurity(r.Context(), projectId, vars["tenantId"], dbAlias, tableName)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(sr)
		}
	}
}

func ProjectDataSourceSchemaSecureTableHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaSecureTableHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]
		tableName = strings.Replace(tableName, "___", ".", 1)

		securityRulesFromReq := json.NewDecoder(r.Body)
		securityRulesFromReq.DisallowUnknownFields()

		var securityRules module_model.SecurityRules

		if err := securityRulesFromReq.Decode(&securityRules); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := eru_utils.ValidateStruct(r.Context(), securityRules, "")
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		err := sh.Store.SaveTableSecurity(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, securityRules, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Table Security for ", tableName, " set successfully")})
		}
		return
	}
}

func ProjectDataSourceSchemaDropTableHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectDataSourceSchemaDropTableHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		dbAlias := vars["dbalias"]
		tableName := vars["tablename"]
		tableName = strings.Replace(tableName, "___", ".", 1)

		err := sh.Store.DropSchemaTable(r.Context(), projectId, vars["tenantId"], dbAlias, tableName, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Table ", tableName, " dropped successfully")})
		}
		return
	}
}
func validateReadDbConfigs(replicas []*module_model.ReadDbConfig) error {
	if len(replicas) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(replicas))
	for i, r := range replicas {
		if r == nil {
			return fmt.Errorf("read_db_configs[%d] is nil", i)
		}
		name := strings.TrimSpace(r.Name)
		if name == "" {
			return fmt.Errorf("read_db_configs[%d] name is required", i)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("read_db_configs duplicate name %q", name)
		}
		seen[name] = struct{}{}
		if r.DbConfig.Host == "" {
			return fmt.Errorf("read_db_configs[%q] db_config.host is required", name)
		}
	}
	return nil
}
