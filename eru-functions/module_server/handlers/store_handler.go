package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/eru-os/eru/eru-functions/functions"
	"github.com/eru-os/eru/eru-functions/module_model"
	"github.com/eru-os/eru/eru-functions/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

const StoreTableName = "erufunctions_config"
const StoreTenantTableName = "erufunctions_config_tenant"

func StoreLoadHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("StoreLoadHandler - Start")

		// Load new store from DB
		newStore, err := module_store.LoadStore(r.Context(), StoreTableName, StoreTenantTableName)
		if err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprintf("Failed to load store: %v", err))
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		// Update the global StoreHolder with the new store
		sh.Store = newStore

		logs.WithContext(r.Context()).Info("Store loaded and replaced successfully")
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(newStore)
	}
}
func StoreCompareHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("StoreCompareHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		projectJson := json.NewDecoder(r.Body)
		projectJson.DisallowUnknownFields()
		var compareProject module_model.ExtendedProject
		storeCompare := module_model.StoreCompare{}

		if err := projectJson.Decode(&compareProject); err == nil {
			myPrj, err := sh.Store.GetExtendedProjectConfig(r.Context(), projectId, sh.Store)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			storeCompare, err = myPrj.CompareProject(r.Context(), compareProject)

		} else {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(storeCompare)

	}
}

func ProjectSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectSaveHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		err := sh.Store.SaveProject(r.Context(), projectID, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project ", projectID, " created successfully")})
		}
	}
}

func ProjectSetingsSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectSetingsSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		prjConfigFromReq := json.NewDecoder(r.Body)
		prjConfigFromReq.DisallowUnknownFields()

		var projectSettings module_model.ProjectSettings

		if err := prjConfigFromReq.Decode(&projectSettings); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			logs.WithContext(r.Context()).Info(fmt.Sprint(projectSettings))
			err := utils.ValidateStruct(r.Context(), projectSettings, "")
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}

		err := sh.Store.SaveProjectSettings(r.Context(), projectId, projectSettings, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project settings for ", projectId, " saved successfully")})
		}
		//}
		//	vars := mux.Vars(r)
		//	projectId := vars["project"]
		//
		//	prjConfigFromReq := json.NewDecoder(r.Body)
		//	prjConfigFromReq.DisallowUnknownFields()
		//
		//	var projectCOnfig module_model.ProjectConfig
		//
		//	if err := prjConfigFromReq.Decode(&projectCOnfig); err != nil {
		//		logs.WithContext(r.Context()).Error(err.Error())
		//		server_handlers.FormatResponse(w, 400)
		//		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		//		return
		//	} else {
		//		err := utils.ValidateStruct(r.Context(), projectCOnfig, "")
		//		if err != nil {
		//			server_handlers.FormatResponse(w, 400)
		//			json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
		//			return
		//		}
		//	}
		//
		//	err := s.SaveProjectConfig(r.Context(), projectId, projectCOnfig, s)
		//	if err != nil {
		//		server_handlers.FormatResponse(w, 400)
		//		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		//	} else {
		//		server_handlers.FormatResponse(w, 200)
		//		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project config for ", projectId, " saved successfully")})
		//	}
	}
}

//func ProjectAuthorizerSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
//	return func(w http.ResponseWriter, r *http.Request) {
//		logs.WithContext(r.Context()).Debug("ProjectAuthorizerSaveHandler - Start")
//		vars := mux.Vars(r)
//		projectId := vars["project"]
//
//		prjAuthorizerFromReq := json.NewDecoder(r.Body)
//		prjAuthorizerFromReq.DisallowUnknownFields()
//
//		var prjAuthorizer functions.Authorizer
//
//		if err := prjAuthorizerFromReq.Decode(&prjAuthorizer); err != nil {
//			logs.WithContext(r.Context()).Error(err.Error())
//			server_handlers.FormatResponse(w, 400)
//			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
//			return
//		} else {
//			err := utils.ValidateStruct(r.Context(), prjAuthorizer, "")
//			if err != nil {
//				server_handlers.FormatResponse(w, 400)
//				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
//				return
//			}
//		}
//
//		err := s.SaveProjectAuthorizer(r.Context(), projectId, prjAuthorizer, s)
//		if err != nil {
//			server_handlers.FormatResponse(w, 400)
//			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
//		} else {
//			server_handlers.FormatResponse(w, 200)
//			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Project Authorizer ", prjAuthorizer.AuthorizerName, " saved successfully")})
//		}
//	}
//}
//
//func ProjectAuthorizerRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
//	return func(w http.ResponseWriter, r *http.Request) {
//		logs.WithContext(r.Context()).Debug("ProjectAuthorizerRemoveHandler - Start")
//		vars := mux.Vars(r)
//		projectId := vars["project"]
//		authorizerName := vars["authorizername"]
//
//		err := s.RemoveProjectAuthorizer(r.Context(), projectId, authorizerName)
//		if err != nil {
//			server_handlers.FormatResponse(w, 400)
//			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
//		} else {
//			//TODO to check if save store is required here
//			s.SaveStore(r.Context(), projectId,"", s)
//			server_handlers.FormatResponse(w, 200)
//			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Project Authorizer ", authorizerName, " removed successfully")})
//		}
//		return
//	}
//}

func ProjectRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ProjectRemoveHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		err := sh.Store.RemoveProject(r.Context(), projectID, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project ", projectID, " removed successfully")})
		}
	}
}

func ProjectListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectListHandler - Start")
		projectIds := sh.Store.GetProjectList(r.Context())
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"projects": projectIds})
	}
}

func ProjectConfigHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectConfigHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		project, err := sh.Store.GetExtendedProjectConfig(r.Context(), projectId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"project": project})
		}
	}
}

func RouteSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("RouteSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		routeFromReq := json.NewDecoder(r.Body)
		routeFromReq.DisallowUnknownFields()

		var routeObj functions.Route
		if err := routeFromReq.Decode(&routeObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), routeObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		//err := storageObj.Save(s,projectId,storageName)
		err := sh.Store.SaveRoute(r.Context(), routeObj, projectId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("route config for ", routeObj.RouteName, " saved successfully")})
		}
		return
	}
}

func RouteRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("RouteRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		routeName := vars["routename"]

		err := sh.Store.RemoveRoute(r.Context(), routeName, projectId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("route config for ", routeName, " removed successfully")})
		}
		return
	}
}

func FuncValidateHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncValidateHandler - Start")

		funcFromReq := json.NewDecoder(r.Body)
		funcFromReq.DisallowUnknownFields()

		var funcObj functions.FuncGroup
		if err := funcFromReq.Decode(&funcObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), funcObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("function config for ", funcObj.FuncGroupName, " validated successfully")})
		return
	}
}
func FuncSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("FuncSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]

		funcFromReq := json.NewDecoder(r.Body)
		funcFromReq.DisallowUnknownFields()

		var funcObj functions.FuncGroup
		if err := funcFromReq.Decode(&funcObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), funcObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		//err := storageObj.Save(s,projectId,storageName)
		err := sh.Store.SaveFunc(r.Context(), funcObj, projectId, tenantId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("function config for ", funcObj.FuncGroupName, " saved successfully")})
		}
		return
	}
}

func FuncRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("FuncRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		funcName := vars["funcname"]

		err := sh.Store.RemoveFunc(r.Context(), funcName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("function ", funcName, " removed successfully")})
		return
	}
}

func ProjectMyQueryListNamesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectMyQueryListNamesHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]

		reqHeader := http.Header{}
		reqHeader.Set("Content-Type", "application/json")
		res, _, _, _, err := utils.CallHttp(r.Context(), http.MethodGet, fmt.Sprint(module_store.Eruqlbaseurl, "/store/", projectID, "/myquery/list"), reqHeader, nil, nil, nil, nil)

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
func ProjectAgentListNamesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectAgentListNamesHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		tenantID := vars["tenant"]
		if tenantID != "" {
			tenantID = fmt.Sprintf("/%s", tenantID)
		}
		reqHeader := http.Header{}
		reqHeader.Set("Content-Type", "application/json")
		res, _, _, _, err := utils.CallHttp(r.Context(), http.MethodGet, fmt.Sprint(module_store.Eruaibaseurl, "/store/", projectID, tenantID, "/agent/list"), reqHeader, nil, nil, nil, nil)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		}
	}
}

func ProjectToolListNamesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectToolListNamesHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		tenantID := vars["tenant"]
		if tenantID != "" {
			tenantID = fmt.Sprintf("/%s", tenantID)
		}
		reqHeader := http.Header{}
		reqHeader.Set("Content-Type", "application/json")
		res, _, _, _, err := utils.CallHttp(r.Context(), http.MethodGet, fmt.Sprint(module_store.Eruaibaseurl, "/store/", projectID, tenantID, "/tool/list"), reqHeader, nil, nil, nil, nil)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		}
	}
}

func ProjectFunctionListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectFunctionListHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		tenantID := vars["tenant"]

		functionNames, err := sh.Store.GetFunctionNames(r.Context(), projectID, tenantID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"functions": functionNames})
		}
		return
	}
}

func ProjectRouteListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ProjectRouteListHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]

		routeNames, err := sh.Store.GetRouteNames(r.Context(), projectID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"routes": routeNames})
		}
		return
	}
}

func WfSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("WfSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		wfFromReq := json.NewDecoder(r.Body)
		wfFromReq.DisallowUnknownFields()

		var wfObj functions.Workflow
		if err := wfFromReq.Decode(&wfObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), wfObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		//err := storageObj.Save(s,projectId,storageName)
		err := sh.Store.SaveWf(r.Context(), wfObj, projectId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("workflow config for ", wfObj.WfName, " saved successfully")})
		}
		return
	}
}

func WfRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("WfRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		wfName := vars["wfname"]

		err := sh.Store.RemoveWf(r.Context(), wfName, projectId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("workflow ", wfName, " removed successfully")})
		return
	}
}
