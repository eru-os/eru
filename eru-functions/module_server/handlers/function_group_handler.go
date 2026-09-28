package handlers

import (
	"bufio"
	"bytes"
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eru-os/eru/eru-events/events"
	"github.com/eru-os/eru/eru-functions/functions"

	//"github.com/eru-os/eru/eru-functions/module_model"
	"github.com/eru-os/eru/eru-functions/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	scheduler "github.com/eru-os/eru/eru-scheduler/scheduler"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

func WfHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("WfHandler - Start")
		// Close the body of the request
		//TODO to add request body close in all handlers across projects
		defer r.Body.Close()
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origin", r.Header.Get("Origin"))
		// Extract the host and url from incoming request
		host, url := extractHostUrl(r)
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		wfName := vars["wfname"]

		wfObj, err := sh.Store.GetWf(ctx, wfName, projectId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		logs.WithContext(r.Context()).Info(fmt.Sprint(wfObj))
		// Lookup a functions in a function based on host and url
		fn := ""
		for _, v := range wfObj.WfEvents {
			fn = v.Function_Name
		}
		funcGroup, err := sh.Store.GetAndValidateFunc(ctx, fn, projectId, tenantId, host, url, r.Method, r.Header, nil, sh.Store, false, "")
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		reqVars := make(map[string]*functions.TemplateVars)
		resVars := make(map[string]*functions.TemplateVars)
		response, _, err := funcGroup.Execute(ctx, r, module_store.FuncThreads, module_store.LoopThreads, "", "", false, reqVars, resVars)

		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		defer response.Body.Close()
		if response.StatusCode >= 300 && response.StatusCode <= 399 {
			http.Redirect(w, r, response.Header.Get("Location"), response.StatusCode)
		} else {

			for k, v := range response.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(response.StatusCode)
			_, err = io.Copy(w, response.Body)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			return
		}
	}
}

func AsyncFuncHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("AsyncFuncHandler - Start")
		//logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler started "))
		// Close the body of the request
		//TODO to add request body close in all handlers across projects
		startTime := time.Now()
		endTime := time.Now()
		defer r.Body.Close()
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origin", r.Header.Get("Origin"))
		// Extract the host and url from incoming request
		host, url := extractHostUrl(r)
		vars := mux.Vars(r)
		projectId := vars["project"]
		eventName := vars["eventname"]
		eventId := vars["eventid"]
		eventI, err := sh.Store.FetchEvent(r.Context(), projectId, eventName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "event not found"})
			return
		}
		endTime = time.Now()
		diff := endTime.Sub(startTime)
		logs.WithContext(r.Context()).Info(fmt.Sprint("total time taken for is FetchEvent ", diff.Milliseconds(), "seconds"))
		var eventMsgs []events.EventMsg
		if eventId == "" {
			logs.WithContext(ctx).Info("polling events")
			eventMsgs, err = eventI.Poll(r.Context())
			if err != nil {
				server_handlers.FormatResponse(w, http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "could not fetch messages from event queue"})
				return
			}
		} else {
			eventMsgs = append(eventMsgs, events.EventMsg{Msg: eventId})
		}

		processedCount := 0
		failedCount := 0
		aStatus := "PENDING"
		if eventId != "" {
			aStatus = "ALL"
		}
		for _, m := range eventMsgs {
			asyncStatus := "PROCESSED"
			var asyncFuncData module_store.AsyncFuncData
			asyncFuncData, err = sh.Store.FetchAsyncEvent(ctx, m.Msg, aStatus, sh.Store)
			eventResponseBytes := []byte("{}")
			//	logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler for FetchAsyncEvent "))
			if err != nil || asyncFuncData.AsyncId == "" {
				failedCount = failedCount + 1
				asyncStatus = "FAILED"
				errMsg := "failed to fetch async event"
				if err != nil {
					errMsg = err.Error()
				}
				eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": errMsg})
				logs.WithContext(ctx).Error(errMsg)
			} else {
				ctx, rowProjectId, rowTenantId := module_store.AsyncTenantContext(ctx, asyncFuncData, projectId)
				bodyMap := make(map[string]interface{})

				bodyMapOk := false
				if asyncFuncData.EventMsg.Vars != nil {
					if bodyMap, bodyMapOk = asyncFuncData.EventMsg.Vars.Body.(map[string]interface{}); !bodyMapOk {
						logs.WithContext(ctx).Error("Request Body count not be retrieved, setting it as blank")
					}
				}
				funcGroup, err := sh.Store.GetAndValidateFunc(ctx, asyncFuncData.FuncName, rowProjectId, rowTenantId, host, url, r.Method, r.Header, bodyMap, sh.Store, true, "")
				//	logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler for GetAndValidateFunc"))
				if err != nil {
					failedCount = failedCount + 1
					asyncStatus = "FAILED"
					eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
					logs.WithContext(ctx).Error(err.Error())
				} else {
					reqBytes := []byte("")
					reqBytes, err = b64.StdEncoding.DecodeString(asyncFuncData.EventRequest)
					if err != nil {
						failedCount = failedCount + 1
						asyncStatus = "FAILED"
						eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
						logs.WithContext(ctx).Error(err.Error())
					} else {
						var newReq *http.Request
						if len(reqBytes) > 0 {
							if newReq, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(reqBytes))); err != nil { // deserialize request
								failedCount = failedCount + 1
								asyncStatus = "FAILED"
								eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
								logs.WithContext(ctx).Error(err.Error())
							}
						} else {
							newReq = r
							body, err := json.Marshal(bodyMap)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								failedCount = failedCount + 1
								asyncStatus = "FAILED"
								eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
							}
							newReq.Body = io.NopCloser(bytes.NewBuffer(body))
						}
						reqVars := make(map[string]*functions.TemplateVars)
						resVars := make(map[string]*functions.TemplateVars)
						if asyncFuncData.EventMsg.ReqVars != nil {
							reqVars = asyncFuncData.EventMsg.ReqVars
						}
						if asyncFuncData.EventMsg.ResVars != nil {
							resVars = asyncFuncData.EventMsg.ResVars
						}
						//logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler Before funcGroup.Execute "))
						response, funcVarsMap, err := funcGroup.Execute(ctx, newReq, module_store.FuncThreads, module_store.LoopThreads, asyncFuncData.FuncStepName, "", true, reqVars, resVars)
						//logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler After funcGroup.Execute "))
						if err != nil {
							failedCount = failedCount + 1
							asyncStatus = "FAILED"
							eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
							logs.WithContext(ctx).Error(err.Error())
						} else {
							responseBytes := []byte("")
							responseBytes, err = io.ReadAll(response.Body)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								failedCount = failedCount + 1
								asyncStatus = "FAILED"
								eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
							} else {
								response.Body = io.NopCloser(bytes.NewBuffer(responseBytes))
								responseStr := string(responseBytes)
								eventResponse := make(map[string]interface{})
								eventResponse["response"] = responseStr
								eventResponse["func_vars"] = funcVarsMap
								eventResponseBytes, err = json.Marshal(eventResponse)
								if err != nil {
									logs.WithContext(ctx).Error(err.Error())
									failedCount = failedCount + 1
									asyncStatus = "FAILED"
									eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
								} else {
									logs.WithContext(ctx).Info(fmt.Sprint(response))
									processedCount = processedCount + 1
								}
							}
						}
						defer func() {
							if response != nil {
								response.Body.Close()
							}
						}()
					}
				}
				//	logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler Before UpdateAsyncEvent "))
				_ = sh.Store.UpdateAsyncEvent(ctx, m.Msg, asyncStatus, string(eventResponseBytes), sh.Store)
				//		logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler After UpdateAsyncEvent "))
				_ = eventI.DeleteMessage(ctx, m.MsgIdentifer)
				//		logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler After DeleteMessage "))

			}
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"processed": processedCount, "failed": failedCount})
		//logs.FileLogger.Info(fmt.Sprint("AsyncFuncHandler ended "))
		return
	}
}

var (
	scriptDomainPattern  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)
	scriptProjectPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func ScriptHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ScriptHandler - Start")
		defer r.Body.Close()

		vars := mux.Vars(r)
		projectId := vars["project"]
		domain := vars["domain"]
		if !scriptProjectPattern.MatchString(projectId) || !scriptDomainPattern.MatchString(domain) {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid project or domain"})
			return
		}

		w.Header().Set("Content-Type", "application/javascript")
		scriptUrl := fmt.Sprint("https://", domain, "/", projectId, "/func/crm_script?opu=<opu>")
		jsCode := `document.addEventListener("submit",function(t){let n=t.target,e=new FormData(n),i=n.action;return!function t(n,e){fetch("` + scriptUrl + `",{method:"POST",headers:{"Content-Type":"application/json","X-Original-Endpoint":e},body:JSON.stringify({formData:Object.fromEntries(n),timestamp:new Date().toISOString(),pageUrl:window.location.href,userId:"<opu>"})}).catch(t=>console.error("Error cloning submission:",t))}(e,i),!0},!0);`
		w.Write([]byte(jsCode))
	}
}
func FuncScheduleHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncScheduleHandler - Start")
		defer r.Body.Close()
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origin", r.Header.Get("Origin"))
		// Extract the host and url from incoming request
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		funcName := vars["funcname"]

		reqContentType := strings.Split(r.Header.Get("Content-type"), ";")[0]
		bodyMap := make(map[string]interface{})
		if reqContentType == "application/json" && r.ContentLength > 0 {
			tmplBodyFromReq := json.NewDecoder(r.Body)
			tmplBodyFromReq.DisallowUnknownFields()
			if err := tmplBodyFromReq.Decode(&bodyMap); err != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprint("error decode request body : ", err.Error()))
				server_handlers.FormatResponse(w, http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decode request body"})
				return
			}
		}

		var funcSchedule scheduler.ScheduleConfig
		if scheduleObj, scheduleObjOk := bodyMap["schedule"]; !scheduleObjOk {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "schedule not found"})
			return
		} else if scheduleMap, scheduleMapOk := scheduleObj.(map[string]interface{}); !scheduleMapOk {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "incorrect schedule map"})
			return
		} else if scheduleMapJson, scheduleMapJsonErr := json.Marshal(scheduleMap); scheduleMapJsonErr != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to marshal schedule map"})
			return
		} else if err := json.Unmarshal(scheduleMapJson, &funcSchedule); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to unmarshal schedule map"})
			return
		}
		// the tenant the schedule runs against comes from the route, which tenantMiddleWare
		// has already split into the tenant and the default tenant on the context.
		if err := utils.ValidateStruct(ctx, funcSchedule, ""); err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		delete(bodyMap, "schedule")

		projectSettings, err := sh.Store.GetProjectSettings(ctx, projectId)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		tokenStr := r.Header.Get(projectSettings.ClaimsKey)

		jobId, err := sh.Store.ScheduleFunc(ctx, funcSchedule, projectId, tenantId, funcName, bodyMap, tokenStr, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("function %s scheduled", funcName), "job_id": jobId})
	}
}

func FuncUnScheduleHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncScheduleHandler - Start")
		defer r.Body.Close()
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origin", r.Header.Get("Origin"))
		// Extract the host and url from incoming request
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		jobId := vars["jobid"]

		err := sh.Store.UnScheduleFunc(ctx, projectId, tenantId, jobId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("scheduler with job id %s unscheduled", jobId)})
		return
	}
}
func FuncHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncHandler - Start")
		// Close the body of the request
		//TODO to add request body close in all handlers across projects
		defer r.Body.Close()
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origin", r.Header.Get("Origin"))
		// Extract the host and url from incoming request
		host, url := extractHostUrl(r)
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		funcName := vars["funcname"]
		funcStepName := vars["funcstepname"]
		endfuncStepName := vars["endfuncstepname"]
		eventName := vars["eventname"]
		logs.WithContext(r.Context()).Info(fmt.Sprint("eventName : ", eventName))
		// Lookup a functions in a function based on host and url

		reqContentType := strings.Split(r.Header.Get("Content-type"), ";")[0]
		var bodyMap map[string]interface{}
		if reqContentType == "application/json" && r.ContentLength > 0 {

			body, err := io.ReadAll(r.Body)
			if err != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprint("error reading request body : ", err.Error()))
				server_handlers.FormatResponse(w, http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to read request body"})
				return
			}
			if err := json.Unmarshal(body, &bodyMap); err != nil {
				var bodyArray []interface{}
				if arrErr := json.Unmarshal(body, &bodyArray); arrErr != nil {
					logs.WithContext(r.Context()).Error(fmt.Sprint("error decode request body : ", err.Error()))
					server_handlers.FormatResponse(w, http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decode request body"})
					return
				}
				bodyMap = map[string]interface{}{"body": bodyArray}
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.Header.Set("Content-Length", strconv.Itoa(len(body)))
			r.ContentLength = int64(len(body))
		}

		funcGroup, err := sh.Store.GetAndValidateFunc(ctx, funcName, projectId, tenantId, host, url, r.Method, r.Header, bodyMap, sh.Store, false, eventName)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		errStatusCode := http.StatusBadRequest
		if funcGroup.ResponseStatusCode > 0 {
			errStatusCode = funcGroup.ResponseStatusCode
		}
		reqVars := make(map[string]*functions.TemplateVars)
		resVars := make(map[string]*functions.TemplateVars)
		response, _, err := funcGroup.Execute(ctx, r, module_store.FuncThreads, module_store.LoopThreads, funcStepName, endfuncStepName, false, reqVars, resVars)
		if err != nil {
			server_handlers.FormatResponse(w, errStatusCode)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer response.Body.Close()

		if response.StatusCode >= 300 && response.StatusCode <= 399 {
			http.Redirect(w, r, response.Header.Get("Location"), response.StatusCode)
		} else {
			for k, v := range response.Header {
				w.Header()[k] = v
			}
			if funcGroup.ResponseContentType != "" {
				w.Header().Del("Content-Type")
				w.Header().Set("Content-Type", funcGroup.ResponseContentType)
			}
			respStatusCode := response.StatusCode
			logs.WithContext(r.Context()).Info(fmt.Sprint(respStatusCode))
			if funcGroup.ResponseStatusCode > 0 {
				respStatusCode = funcGroup.ResponseStatusCode
			}
			logs.WithContext(r.Context()).Info(fmt.Sprint(respStatusCode))
			w.WriteHeader(respStatusCode)
			_, err = io.Copy(w, response.Body)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				w.WriteHeader(errStatusCode)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			logs.WithContext(ctx).Info(fmt.Sprint(w.Header()))
			return
		}
	}
}

func FuncFetchHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncFetchHandler - Start")
		// Close the body of the request
		//TODO to add request body close in all handlers across projects
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		funcName := vars["funcname"]

		funcGroup, err := sh.Store.GetFunc(r.Context(), funcName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(funcGroup)
	}
}

func SFuncHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncHandler - Start")
		// Close the body of the request
		//TODO to add request body close in all handlers across projects
		defer r.Body.Close()
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origin", r.Header.Get("Origin"))
		// Extract the host and url from incoming request
		host, url := extractHostUrl(r)
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		funcName := vars["funcname"]
		funcStepName := vars["funcstepname"]
		endfuncStepName := vars["endfuncstepname"]
		// Lookup a functions in a function based on host and url

		reqContentType := strings.Split(r.Header.Get("Content-type"), ";")[0]

		type reqBody struct {
			Body    map[string]interface{}             `json:"body"`
			ReqVars map[string]*functions.TemplateVars `json:"req_vars"`
			ResVars map[string]*functions.TemplateVars `json:"res_vars"`
		}
		type resBody struct {
			Body    interface{}                        `json:"body"`
			ReqVars map[string]*functions.TemplateVars `json:"req_vars"`
			ResVars map[string]*functions.TemplateVars `json:"res_vars"`
		}

		bodyMap := reqBody{}
		if reqContentType == "application/json" && r.ContentLength > 0 {

			tmplBodyFromReq := json.NewDecoder(r.Body)
			tmplBodyFromReq.DisallowUnknownFields()
			if err := tmplBodyFromReq.Decode(&bodyMap); err != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprint("error decode request body : ", err.Error()))
				server_handlers.FormatResponse(w, http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decode request body"})
				return
			}

			if bodyMap.Body == nil {
				bodyMap.Body = make(map[string]interface{})
			}

			if bodyMap.ReqVars == nil {
				bodyMap.ReqVars = make(map[string]*functions.TemplateVars)
			}

			if bodyMap.ResVars == nil {
				bodyMap.ResVars = make(map[string]*functions.TemplateVars)
			}

			body, err := json.Marshal(bodyMap.Body)
			if err != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("json.Marshal(vars.Body) error : ", err.Error()))
				server_handlers.FormatResponse(w, http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to marshal request body"})
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.Header.Set("Content-Length", strconv.Itoa(len(body)))
			r.ContentLength = int64(len(body))
		}

		funcGroup, err := sh.Store.GetAndValidateFunc(ctx, funcName, projectId, tenantId, host, url, r.Method, r.Header, bodyMap.Body, sh.Store, false, "")
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		errStatusCode := http.StatusBadRequest
		if funcGroup.ResponseStatusCode > 0 {
			errStatusCode = funcGroup.ResponseStatusCode
		}
		reqVars := bodyMap.ReqVars
		resVars := bodyMap.ResVars
		response, varsMap, err := funcGroup.Execute(ctx, r, module_store.FuncThreads, module_store.LoopThreads, funcStepName, endfuncStepName, false, reqVars, resVars)
		if err != nil {
			server_handlers.FormatResponse(w, errStatusCode)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		defer response.Body.Close()
		if response.StatusCode >= 300 && response.StatusCode <= 399 {
			http.Redirect(w, r, response.Header.Get("Location"), response.StatusCode)
		} else {

			for k, v := range response.Header {
				w.Header()[k] = v
			}
			if funcGroup.ResponseContentType != "" {
				w.Header().Del("Content-Type")
				w.Header().Set("Content-Type", funcGroup.ResponseContentType)
			}
			respStatusCode := response.StatusCode
			if funcGroup.ResponseStatusCode > 0 {
				respStatusCode = funcGroup.ResponseStatusCode
			}
			responseBytes, err := io.ReadAll(response.Body)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				server_handlers.FormatResponse(w, errStatusCode)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			responseData := resBody{
				ReqVars: map[string]*functions.TemplateVars{},
				ResVars: map[string]*functions.TemplateVars{},
				Body:    interface{}(nil),
			}
			var resBodyI interface{}
			err = json.Unmarshal(responseBytes, &resBodyI)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				server_handlers.FormatResponse(w, errStatusCode)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			responseData.Body = resBodyI

			// Get the first (and only) value from varsMap
			for _, v := range varsMap {
				for kk, vv := range v.ReqVars {
					responseData.ReqVars[kk] = vv
				}
				for kk, vv := range v.ResVars {
					responseData.ResVars[kk] = vv
				}
			}

			var finalResponseData interface{}
			if funcStepName != "" || endfuncStepName != "" {
				finalResponseData = responseData
			} else {
				finalResponseData = responseData.Body
			}
			jsonBytes, err := json.Marshal(finalResponseData)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				server_handlers.FormatResponse(w, errStatusCode)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to serialize response"})
				return
			}
			w.Header().Set("Content-Type", "application/json")

			// Set content length for the JSON response
			w.Header().Set("Content-Length", strconv.Itoa(len(jsonBytes)))

			// Write status code
			w.WriteHeader(respStatusCode)

			// Write the JSON response
			w.Write(jsonBytes)
			/* _, err = io.Copy(w, response.Body)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				w.WriteHeader(errStatusCode)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			} */
			return
		}
	}
}

func FuncRunHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("FuncRunHandler - Start")
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origin", r.Header.Get("Origin"))

		host, url := extractHostUrl(r)
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		funcStepName := vars["funcstepname"]
		endFuncStepName := vars["endfuncstepname"]

		funcFromReq := json.NewDecoder(r.Body)
		funcFromReq.DisallowUnknownFields()

		var funcMap map[string]interface{}

		if err := funcFromReq.Decode(&funcMap); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			if funcJson, funcJsonOk := funcMap["func"]; funcJsonOk {
				funcJsonBytes, funcJsonBytesErr := json.Marshal(funcJson)
				if funcJsonBytesErr != nil {
					server_handlers.FormatResponse(w, http.StatusBadRequest)
					logs.WithContext(ctx).Error(funcJsonBytesErr.Error())
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "function body could not be read from json"})
					return
				}
				var funcObj functions.FuncGroup
				funcObjD := json.NewDecoder(bytes.NewReader(funcJsonBytes))
				funcObjD.DisallowUnknownFields()

				if err = funcObjD.Decode(&funcObj); err == nil {
					err = utils.ValidateStruct(ctx, funcObj, "")
					if err != nil {
						server_handlers.FormatResponse(w, 400)
						json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
						return
					}
					if rBody, rBodyOk := funcMap["body"]; rBodyOk {
						rBodyBytes, rBodyBytesErr := json.Marshal(rBody)
						if rBodyBytesErr != nil {
							server_handlers.FormatResponse(w, http.StatusBadRequest)
							logs.WithContext(ctx).Error(rBodyBytesErr.Error())
							_ = json.NewEncoder(w).Encode(map[string]string{"error": "function body could not be read"})
							return
						}
						r.Body = io.NopCloser(bytes.NewReader(rBodyBytes))
						r.Header.Set("Content-Length", strconv.Itoa(len(rBodyBytes)))
						r.ContentLength = int64(len(rBodyBytes))
					} else {
						err = errors.New("function body not found")
						logs.WithContext(ctx).Error(err.Error())
						w.WriteHeader(http.StatusBadRequest)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					}

					funcGroup, err := sh.Store.ValidateFunc(ctx, funcObj, projectId, tenantId, host, url, r.Method, r.Header, nil, sh.Store, false, "")
					if err != nil {
						server_handlers.FormatResponse(w, http.StatusBadRequest)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
						return
					}
					reqVars := make(map[string]*functions.TemplateVars)
					resVars := make(map[string]*functions.TemplateVars)
					response, _, err := funcGroup.Execute(ctx, r, module_store.FuncThreads, module_store.LoopThreads, funcStepName, endFuncStepName, false, reqVars, resVars)
					if err != nil {
						server_handlers.FormatResponse(w, http.StatusBadRequest)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
						return
					}

					defer response.Body.Close()
					if response.StatusCode >= 300 && response.StatusCode <= 399 {
						http.Redirect(w, r, response.Header.Get("Location"), response.StatusCode)
					} else {

						for k, v := range response.Header {
							w.Header()[k] = v
						}
						w.WriteHeader(response.StatusCode)
						_, err = io.Copy(w, response.Body)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							w.WriteHeader(http.StatusBadRequest)
							_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
							return
						}
						return
					}

				} else {
					err := errors.New("function definition could not be read")
					logs.WithContext(ctx).Error(err.Error())
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				}
			} else {
				err := errors.New("function definition not found")
				logs.WithContext(ctx).Error(err.Error())
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			}
		}
	}
}

func SFuncRunHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncRunHandler - Start")
		ctx := context.WithValue(r.Context(), "allowed_origins", server_handlers.AllowedOrigins)
		ctx = context.WithValue(ctx, "origin", r.Header.Get("Origin"))

		host, url := extractHostUrl(r)
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		funcStepName := vars["funcstepname"]
		endFuncStepName := vars["endfuncstepname"]

		funcFromReq := json.NewDecoder(r.Body)
		funcFromReq.DisallowUnknownFields()

		var funcMap map[string]interface{}
		type resBody struct {
			Body    interface{}                        `json:"body"`
			ReqVars map[string]*functions.TemplateVars `json:"req_vars"`
			ResVars map[string]*functions.TemplateVars `json:"res_vars"`
		}
		if err := funcFromReq.Decode(&funcMap); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			if funcJson, funcJsonOk := funcMap["func"]; funcJsonOk {
				funcJsonBytes, funcJsonBytesErr := json.Marshal(funcJson)
				if funcJsonBytesErr != nil {
					server_handlers.FormatResponse(w, http.StatusBadRequest)
					logs.WithContext(ctx).Error(funcJsonBytesErr.Error())
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "function body could not be read from json"})
					return
				}
				var funcObj functions.FuncGroup
				funcObjD := json.NewDecoder(bytes.NewReader(funcJsonBytes))
				funcObjD.DisallowUnknownFields()

				if err = funcObjD.Decode(&funcObj); err == nil {
					err = utils.ValidateStruct(ctx, funcObj, "")
					if err != nil {
						server_handlers.FormatResponse(w, 400)
						json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
						return
					}
					type reqBody struct {
						Body    interface{}                        `json:"body"`
						ReqVars map[string]*functions.TemplateVars `json:"req_vars"`
						ResVars map[string]*functions.TemplateVars `json:"res_vars"`
					}

					bodyMap := reqBody{}
					bodyNewMap := make(map[string]interface{})

					if rBody, rBodyOk := funcMap["body"]; rBodyOk {
						rBodyBytes, rBodyBytesErr := json.Marshal(rBody)
						if rBodyBytesErr != nil {
							server_handlers.FormatResponse(w, http.StatusBadRequest)
							logs.WithContext(ctx).Error(rBodyBytesErr.Error())
							_ = json.NewEncoder(w).Encode(map[string]string{"error": "function body could not be read"})
							return
						}

						reqContentType := strings.Split(r.Header.Get("Content-type"), ";")[0]
						if reqContentType == "application/json" && r.ContentLength > 0 {

							tmplBodyFromReq := json.NewDecoder(bytes.NewReader(rBodyBytes))
							tmplBodyFromReq.DisallowUnknownFields()
							if err := tmplBodyFromReq.Decode(&bodyMap); err != nil {
								logs.WithContext(r.Context()).Error(fmt.Sprint("error decode request body : ", err.Error()))
								server_handlers.FormatResponse(w, http.StatusBadRequest)
								_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decode request body"})
								return
							}
							if bodyMap.Body == nil {
								bodyMap.Body = make(map[string]interface{})
							}

							if bodyArray, ok := bodyMap.Body.([]interface{}); ok {
								for _, v := range bodyArray {
									if vMap, ok := v.(map[string]interface{}); ok {
										for kk, vv := range vMap {
											bodyNewMap[kk] = vv
										}
									}
								}
							} else {
								if bodyM, ok := bodyMap.Body.(map[string]interface{}); ok {
									bodyNewMap = bodyM
								}
							}

							if bodyMap.ReqVars == nil {
								bodyMap.ReqVars = make(map[string]*functions.TemplateVars)
							}

							if bodyMap.ResVars == nil {
								bodyMap.ResVars = make(map[string]*functions.TemplateVars)
							}
							body, err := json.Marshal(bodyNewMap)
							if err != nil {
								logs.WithContext(ctx).Error(fmt.Sprint("json.Marshal(vars.Body) error : ", err.Error()))
								server_handlers.FormatResponse(w, http.StatusBadRequest)
								_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to marshal request body"})
								return
							}
							r.Body = io.NopCloser(bytes.NewReader(body))
							r.Header.Set("Content-Length", strconv.Itoa(len(body)))
							r.ContentLength = int64(len(body))
						}
					} else {
						err = errors.New("function body not found")
						logs.WithContext(ctx).Error(err.Error())
						w.WriteHeader(http.StatusBadRequest)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					}

					funcGroup, err := sh.Store.ValidateFunc(ctx, funcObj, projectId, tenantId, host, url, r.Method, r.Header, bodyNewMap, sh.Store, false, "")
					if err != nil {
						server_handlers.FormatResponse(w, http.StatusBadRequest)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
						return
					}
					errStatusCode := http.StatusBadRequest
					if funcGroup.ResponseStatusCode > 0 {
						errStatusCode = funcGroup.ResponseStatusCode
					}
					reqVars := bodyMap.ReqVars
					resVars := bodyMap.ResVars
					if reqVars == nil {
						reqVars = make(map[string]*functions.TemplateVars)
					}
					if resVars == nil {
						resVars = make(map[string]*functions.TemplateVars)
					}
					response, varsMap, err := funcGroup.Execute(ctx, r, module_store.FuncThreads, module_store.LoopThreads, funcStepName, endFuncStepName, false, reqVars, resVars)
					if err != nil {
						server_handlers.FormatResponse(w, errStatusCode)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
						return
					}

					defer response.Body.Close()
					if response.StatusCode >= 300 && response.StatusCode <= 399 {
						http.Redirect(w, r, response.Header.Get("Location"), response.StatusCode)
					} else {

						for k, v := range response.Header {
							w.Header()[k] = v
						}
						if funcGroup.ResponseContentType != "" {
							w.Header().Del("Content-Type")
							w.Header().Set("Content-Type", funcGroup.ResponseContentType)
						}
						respStatusCode := response.StatusCode
						if funcGroup.ResponseStatusCode > 0 {
							respStatusCode = funcGroup.ResponseStatusCode
						}

						responseBytes, err := io.ReadAll(response.Body)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							server_handlers.FormatResponse(w, errStatusCode)
							_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
							return
						}
						responseData := resBody{
							ReqVars: map[string]*functions.TemplateVars{},
							ResVars: map[string]*functions.TemplateVars{},
							Body:    interface{}(nil),
						}
						var resBodyI interface{}
						err = json.Unmarshal(responseBytes, &resBodyI)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							server_handlers.FormatResponse(w, errStatusCode)
							_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
							return
						}
						responseData.Body = resBodyI

						for _, v := range varsMap {
							for kk, vv := range v.ReqVars {
								responseData.ReqVars[kk] = vv
							}
							for kk, vv := range v.ResVars {
								responseData.ResVars[kk] = vv
							}
						}

						var finalResponseData interface{}
						if funcStepName != "" || endFuncStepName != "" {
							finalResponseData = responseData
						} else {
							finalResponseData = responseData.Body
						}
						jsonBytes, err := json.Marshal(finalResponseData)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							server_handlers.FormatResponse(w, errStatusCode)
							_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to serialize response"})
							return
						}
						w.Header().Set("Content-Type", "application/json")

						// Set content length for the JSON response
						w.Header().Set("Content-Length", strconv.Itoa(len(jsonBytes)))

						// Write status code
						w.WriteHeader(respStatusCode)
						// Write the JSON response
						w.Write(jsonBytes)
						/* _, err = io.Copy(w, response.Body)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							w.WriteHeader(errStatusCode)
							_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
							return
						} */
						return
					}
				} else {
					err := errors.New("function definition could not be read")
					logs.WithContext(ctx).Error(err.Error())
					server_handlers.FormatResponse(w, http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				}
			} else {
				err := errors.New("function definition not found")
				logs.WithContext(ctx).Error(err.Error())
				server_handlers.FormatResponse(w, http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			}
		}
	}
}
