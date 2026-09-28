package module_store

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
	"net/url"
	"os"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"strconv"

	"github.com/eru-os/eru/eru-db/db"
	"github.com/eru-os/eru/eru-events/events"
	"github.com/eru-os/eru/eru-functions/functions"
	"github.com/eru-os/eru/eru-functions/module_model"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	"github.com/eru-os/eru/eru-scheduler/scheduler"
	server "github.com/eru-os/eru/eru-server/server"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/eru-os/eru/eru-store/store"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var Eruqlbaseurl = "http://localhost:8087"
var Eruaibaseurl = "http://localhost:8088"
var FuncThreads = 3
var LoopThreads = 3
var EventThreads = 3

// FuncListItem is one entry of the func list, with the tenant the function was resolved
// from: the route tenant, the default tenant carried on the route, or "" for project level.
type FuncListItem struct {
	FuncName string `json:"func_name"`
	Source   string `json:"source"`
}

type ContextKey string

const (
	ContextKeyEruqlbaseurl ContextKey = "eruqlbaseurl"
	ContextKeyEruaibaseurl ContextKey = "eruaibaseurl"
)

func getEruqlbaseurl(ctx context.Context) string {
	if ctx != nil {
		if baseurl, ok := ctx.Value(ContextKeyEruqlbaseurl).(string); ok && baseurl != "" {
			return baseurl
		}
	}
	return Eruqlbaseurl
}

func getEruaibaseurl(ctx context.Context) string {
	if ctx != nil {
		if baseurl, ok := ctx.Value(ContextKeyEruaibaseurl).(string); ok && baseurl != "" {
			return baseurl
		}
	}
	return Eruaibaseurl
}

const (
	UPDATE_FUNC_ASYNC    = "update erufunctions_async_loop set async_status=???, processed_date=now(), event_response=??? where async_id = ???"
	SELECT_FUNC_ASYNC    = "update erufunctions_async_loop x set async_status='IN PROGRESS', processed_date=now() from (select a.async_id, b.event_id, b.func_group_name func_name, b.func_step_name,  jsonb_set(jsonb_set(b.event_msg , ARRAY['ReqVars', b.func_step_name, 'LoopVar'] , a.loop_var::jsonb),ARRAY['Vars','LoopVar'],a.loop_var::jsonb) event_msg, b.event_request, b.request_id, b.project_id, b.tenant_id from erufunctions_async_loop a left join erufunctions_async b on a.event_id = b.event_id where a.async_id=??? and (async_status=??? or 'ALL'=???) for update of a skip locked) y where x.async_id=y.async_id returning y.*"
	INSERT_FUNC_SCHEDULE = "insert into erufunctions_schedules (schedule_id, project_id, tenant_id, func_group_name, func_step_name, event_msg, scheduler_name, scheduler_label,job_id, start_date, end_date) values (???, ???, ???, ???, ???, ???, ???, ???, ???, ???, ???)"
	DELETE_FUNC_SCHEDULE = "delete from erufunctions_schedules where job_id=???"
	SELECT_FUNC_SCHEDULE = "select project_id, tenant_id from erufunctions_schedules where job_id=???"
)

type StoreHolder struct {
	sync.RWMutex
	Store ModuleStoreI
}

type AsyncFuncData struct {
	AsyncId      string                     `json:"async_id"`
	EventId      string                     `json:"event_id"`
	FuncName     string                     `json:"func_group_name"`
	FuncStepName string                     `json:"func_step_name"`
	EventMsg     functions.FuncTemplateVars `json:"event_msg"`
	EventRequest string                     `json:"event_request"`
	RequestId    string                     `json:"request_id"`
	ProjectId    string                     `json:"project_id"`
	// RouteTenantId is the route form of the tenant the function was called for, so picking
	// the row up later resolves the function through the same tenant fallback order.
	RouteTenantId string `json:"tenant_id"`
}
type ModuleStoreI interface {
	store.StoreI
	SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error
	//SaveProjectConfig(ctx context.Context, projectId string, projectConfig module_model.ProjectConfig, realStore ModuleStoreI) error
	SaveProjectSettings(ctx context.Context, projectId string, projectConfig module_model.ProjectSettings, realStore ModuleStoreI) error
	SetProjectSettings(ctx context.Context, projectId string, projectSettings module_model.ProjectSettings) error
	GetProjectSettings(ctx context.Context, projectId string) (module_model.ProjectSettings, error)
	RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error
	//SaveProjectAuthorizer(ctx context.Context, projectId string, authorizer functions.Authorizer, realStore ModuleStoreI) error
	//RemoveProjectAuthorizer(ctx context.Context, projectId string, authorizerName string) error
	//GetProjectAuthorizer(ctx context.Context, projectId string, authorizerName string) (functions.Authorizer, error)
	GetProjectConfig(ctx context.Context, projectId string) (*module_model.Project, error)
	GetExtendedProjectConfig(ctx context.Context, projectId string, realStore ModuleStoreI) (module_model.ExtendedProject, error)
	GetProjectList(ctx context.Context) []map[string]interface{}
	SaveRoute(ctx context.Context, routeObj functions.Route, projectId string, realStore ModuleStoreI, persist bool) error
	RemoveRoute(ctx context.Context, routeName string, projectId string, realStore ModuleStoreI) error
	GetAndValidateRoute(ctx context.Context, routeName string, projectId string, host string, url string, method string, headers http.Header, s ModuleStoreI) (route functions.Route, err error)
	GetAndValidateFunc(ctx context.Context, funcName string, projectId string, tenantId string, host string, url string, method string, headers http.Header, reqBody map[string]interface{}, s ModuleStoreI, fromAsync bool, eventName string) (funcGroup functions.FuncGroup, err error)
	GetFunc(ctx context.Context, funcName string, projectId string, tenantId string, s ModuleStoreI) (funcGroup functions.FuncGroup, err error)
	ScheduleFunc(ctx context.Context, funcSchedule scheduler.ScheduleConfig, projectId string, tenantId string, funcName string, reqBody map[string]interface{}, tokenStr string, realStore ModuleStoreI) (jobId string, err error)
	UnScheduleFunc(ctx context.Context, projectId string, tenantId string, jobId string, realStore ModuleStoreI) error
	GetWf(ctx context.Context, wfName string, projectId string, s ModuleStoreI) (wfObj functions.Workflow, err error)
	ValidateFunc(ctx context.Context, funcObj functions.FuncGroup, projectId string, tenantId string, host string, url string, method string, headers http.Header, reqBody map[string]interface{}, s ModuleStoreI, fromAsync bool, eventName string) (funcGroup functions.FuncGroup, err error)
	SaveFunc(ctx context.Context, funcObj functions.FuncGroup, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error
	RemoveFunc(ctx context.Context, funcName string, projectId string, tenantId string, realStore ModuleStoreI) error
	GetFunctionNames(ctx context.Context, projectId string, tenantId string) (functions []FuncListItem, err error)
	GetRouteNames(ctx context.Context, projectId string) (routes []string, err error)
	SaveWf(ctx context.Context, wfObj functions.Workflow, projectId string, realStore ModuleStoreI, persist bool) error
	RemoveWf(ctx context.Context, wfName string, projectId string, realStore ModuleStoreI) error
	FetchAsyncEvent(ctx context.Context, asyncId string, asyncStatus string, realStore ModuleStoreI) (asyncFuncData AsyncFuncData, err error)
	UpdateAsyncEvent(ctx context.Context, asyncId string, asyncStatus string, eventResponse string, realStore ModuleStoreI) (err error)
	FetchProjectEvents(ctx context.Context, s ModuleStoreI, cnt int, asyncEventsList []string) (err error)
	StartPolling(ctx context.Context, projectId string, event events.EventI, s ModuleStoreI, cnt int) (err error)
	//SaveFuncRequest(ctx context.Context, sampleRequest module_model.SampleRequest, projectId string, tenantId string, realStore ModuleStoreI) error
	//RemoveFuncRequest(ctx context.Context, requestId string, realStore ModuleStoreI) error
	//GetFuncRequests(ctx context.Context, projectId string, tenantId string, funcName string, realStore ModuleStoreI) (requests []module_model.SampleRequest, err error)
}

type ModuleStore struct {
	Projects map[string]*module_model.Project `json:"projects"` //ProjectId is the key
}

type ModuleFileStore struct {
	store.FileStore
	ModuleStore
}
type ModuleDbStore struct {
	store.DbStore
	ModuleStore
}

func (ms *ModuleStore) SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveProject - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}

	//TODO to handle edit project once new project attributes are finalized
	if _, ok := ms.Projects[projectId]; !ok {
		project := new(module_model.Project)
		project.ProjectId = projectId
		if ms.Projects == nil {
			ms.Projects = make(map[string]*module_model.Project)
		}
		if project.Routes == nil {
			project.Routes = make(map[string]functions.Route)
		}
		if project.FuncGroups == nil {
			project.FuncGroups = make(map[string]functions.FuncGroup)
		}
		if project.Tenants == nil {
			project.Tenants = make(map[string]module_model.TenantConfig)
		}
		//if project.Authorizers == nil {
		//	project.Authorizers = make(map[string]functions.Authorizer)
		//}
		ms.Projects[projectId] = project
		if persist == true {
			logs.WithContext(ctx).Info("SaveStore called from SaveProject")
			return realStore.SaveStore(ctx, projectId, "", realStore)
		} else {
			return nil
		}
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " already exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}

//func (ms *ModuleStore) SaveProjectConfig(ctx context.Context, projectId string, projectConfig module_model.ProjectConfig, realStore ModuleStoreI) error {
//	logs.WithContext(ctx).Debug("SaveProjectConfig - Start")
//	if _, ok := ms.Projects[projectId]; ok {
//		ms.Projects[projectId].ProjectConfig = projectConfig
//		return realStore.SaveStore(ctx, projectId,"", realStore)
//	} else {
//		err := errors.New(fmt.Sprint("Project ", projectId, " not found"))
//		logs.WithContext(ctx).Error(err.Error())
//		return err
//	}
//}
//
//func (ms *ModuleStore) SaveProjectAuthorizer(ctx context.Context, projectId string, authorizer functions.Authorizer, realStore ModuleStoreI) error {
//	logs.WithContext(ctx).Debug("SaveProjectAuthorizer - Start")
//	if _, ok := ms.Projects[projectId]; ok {
//		if ms.Projects[projectId].Authorizers == nil {
//			ms.Projects[projectId].Authorizers = make(map[string]functions.Authorizer)
//		}
//		ms.Projects[projectId].Authorizers[authorizer.AuthorizerName] = authorizer
//		return realStore.SaveStore(ctx, projectId,"", realStore)
//	} else {
//		err := errors.New(fmt.Sprint("Project ", projectId, " not found"))
//		logs.WithContext(ctx).Error(err.Error())
//		return err
//	}
//}
//
//func (ms *ModuleStore) RemoveProjectAuthorizer(ctx context.Context, projectId string, authorizerName string) error {
//	logs.WithContext(ctx).Debug("RemoveProjectAuthorizer - Start")
//	if _, ok := ms.Projects[projectId]; ok {
//		if _, authOk := ms.Projects[projectId].Authorizers[authorizerName]; authOk {
//			delete(ms.Projects[projectId].Authorizers, authorizerName)
//			return nil
//		} else {
//			err := errors.New(fmt.Sprint("Authorizer ", authorizerName, " not found"))
//			logs.WithContext(ctx).Error(err.Error())
//			return err
//		}
//	} else {
//		err := errors.New(fmt.Sprint("Project ", projectId, " not found"))
//		logs.WithContext(ctx).Error(err.Error())
//		return err
//	}
//}
//func (ms *ModuleStore) GetProjectAuthorizer(ctx context.Context, projectId string, authorizerName string) (functions.Authorizer, error) {
//	logs.WithContext(ctx).Debug("GetProjectAuthorizer - Start")
//	if _, ok := ms.Projects[projectId]; ok {
//		if _, authOk := ms.Projects[projectId].Authorizers[authorizerName]; authOk {
//			return ms.Projects[projectId].Authorizers[authorizerName], nil
//		} else {
//			err := errors.New(fmt.Sprint("Authorizer ", authorizerName, " not found"))
//			logs.WithContext(ctx).Error(err.Error())
//			return functions.Authorizer{}, err
//		}
//	} else {
//		err := errors.New(fmt.Sprint("Project ", projectId, " not found"))
//		logs.WithContext(ctx).Error(err.Error())
//		return functions.Authorizer{}, err
//	}
//}

func (ms *ModuleStore) RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveProject - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if _, ok := ms.Projects[projectId]; ok {
		delete(ms.Projects, projectId)
		logs.WithContext(ctx).Info("SaveStore called from RemoveProject")
		return realStore.SaveStore(ctx, projectId, "", realStore)
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}
func (ms *ModuleStore) GetExtendedProjectConfig(ctx context.Context, projectId string, realStore ModuleStoreI) (ePrj module_model.ExtendedProject, err error) {
	logs.WithContext(ctx).Debug("GetExtendedProjectConfig - Start")
	ePrj = module_model.ExtendedProject{}
	if prj, ok := ms.Projects[projectId]; ok {
		ePrj.Variables, err = realStore.FetchVars(ctx, projectId)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		ePrj.SecretManager, err = realStore.FetchSm(ctx, projectId)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		ePrj.Scheduler, err = realStore.FetchScheduler(ctx, projectId)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		ePrj.ProjectId = prj.ProjectId
		ePrj.ProjectSettings = prj.ProjectSettings
		ePrj.Routes = prj.Routes
		ePrj.FuncGroups = prj.FuncGroups
		ePrj.Workflows = prj.Workflows
		return ePrj, nil
	} else {
		err = errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		return module_model.ExtendedProject{}, err
	}
}

func (ms *ModuleStore) GetProjectConfig(ctx context.Context, projectId string) (*module_model.Project, error) {
	logs.WithContext(ctx).Debug("GetProjectConfig - Start")
	if _, ok := ms.Projects[projectId]; ok {

		logs.WithContext(ctx).Info(fmt.Sprint(ms.Projects[projectId].Workflows))

		return ms.Projects[projectId], nil
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

func (ms *ModuleStore) GetProjectList(ctx context.Context) []map[string]interface{} {
	logs.WithContext(ctx).Debug("GetProjectList - Start")
	projects := make([]map[string]interface{}, len(ms.Projects))
	i := 0
	for k := range ms.Projects {
		project := make(map[string]interface{})
		project["project_name"] = k
		//project["lastUpdateDate"] = time.Now()
		projects[i] = project
		i++
	}
	return projects
}

func (ms *ModuleStore) SaveRoute(ctx context.Context, routeObj functions.Route, projectId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveRoute - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	err = prj.AddRoute(ctx, routeObj)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if persist == true {
		return realStore.SaveStore(ctx, projectId, "", realStore)
	}
	return nil
}

func (ms *ModuleStore) RemoveRoute(ctx context.Context, routeName string, projectId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveRoute - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if prg, ok := ms.Projects[projectId]; ok {
		if _, ok := prg.Routes[routeName]; ok {
			delete(prg.Routes, routeName)
			logs.WithContext(ctx).Info("SaveStore called from RemoveRoute")
			return realStore.SaveStore(ctx, projectId, "", realStore)
		} else {
			err := errors.New(fmt.Sprint("Route ", routeName, " does not exists"))
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetAndValidateRoute(ctx context.Context, routeName string, projectId string, host string, url string, method string, headers http.Header, s ModuleStoreI) (route functions.Route, err error) {
	logs.WithContext(ctx).Debug("GetAndValidateRoute - Start")
	cloneRoute := functions.Route{}
	if prg, ok := ms.Projects[projectId]; ok {
		if route, ok = prg.Routes[routeName]; !ok {
			err = errors.New(fmt.Sprint("Route ", routeName, " does not exists"))
			logs.WithContext(ctx).Error(err.Error())
			return cloneRoute, err
		}
		routeI, jmErr := json.Marshal(route)
		if jmErr != nil {
			err = errors.New("route marshal failed")
			logs.WithContext(ctx).Error(fmt.Sprint(err.Error(), " : ", jmErr.Error()))
			return cloneRoute, err
		}
		routeI = s.ReplaceVariables(ctx, projectId, routeI, nil)
		jmErr = json.Unmarshal(routeI, &cloneRoute)
		if jmErr != nil {
			err = errors.New("route unmarshal failed")
			logs.WithContext(ctx).Error(fmt.Sprint(err.Error(), " : ", jmErr.Error()))
			return cloneRoute, err
		}
		cloneRoute.TokenSecretKey = prg.ProjectSettings.ClaimsKey
	} else {
		err = errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return cloneRoute, err
	}
	err = cloneRoute.Validate(ctx, host, url, method, headers)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return cloneRoute, err
	}
	return cloneRoute, nil
}

func (ms *ModuleStore) resolveFunc(ctx context.Context, projectId string, tenantId string, funcName string) (funcGroup functions.FuncGroup, found bool) {
	prg, ok := ms.Projects[projectId]
	if !ok {
		return
	}
	for _, tid := range eru_utils.TenantLookupOrder(ctx, tenantId) {
		if tc, tcOk := prg.Tenants[tid]; tcOk {
			if fg, fgOk := tc.FuncGroups[funcName]; fgOk {
				return fg, true
			}
		}
	}
	funcGroup, found = prg.FuncGroups[funcName]
	return
}

func (ms *ModuleStore) GetFunc(ctx context.Context, funcName string, projectId string, tenantId string, s ModuleStoreI) (cloneFunc functions.FuncGroup, err error) {
	logs.WithContext(ctx).Debug("GetFunc - Start")
	if _, ok := ms.Projects[projectId]; ok {
		funcGroup, found := ms.resolveFunc(ctx, projectId, tenantId, funcName)
		if !found {
			return funcGroup, errors.New(fmt.Sprint("Function ", funcName, " does not exists"))
		}
		return funcGroup, nil
	}
	return
}
func (ms *ModuleStore) GetAndValidateFunc(ctx context.Context, funcName string, projectId string, tenantId string, host string, url string, method string, headers http.Header, reqBody map[string]interface{}, s ModuleStoreI, fromAsync bool, eventName string) (cloneFunc functions.FuncGroup, err error) {
	logs.WithContext(ctx).Debug("GetAndValidateFunc - Start")
	if _, ok := ms.Projects[projectId]; ok {
		funcGroup, found := ms.resolveFunc(ctx, projectId, tenantId, funcName)
		if !found {
			return funcGroup, errors.New(fmt.Sprint("Function ", funcName, " does not exists"))
		}
		return ms.ValidateFunc(ctx, funcGroup, projectId, tenantId, host, url, method, headers, reqBody, s, fromAsync, eventName)
	}
	return
}

func (ms *ModuleStore) ValidateFunc(ctx context.Context, funcGroup functions.FuncGroup, projectId string, tenantId string, host string, url string, method string, headers http.Header, reqBody map[string]interface{}, s ModuleStoreI, fromAsync bool, eventName string) (cloneFunc functions.FuncGroup, err error) {
	logs.WithContext(ctx).Debug("ValidateFunc - Start")
	if prg, ok := ms.Projects[projectId]; ok {
		FuncI, jmErr := json.Marshal(funcGroup)
		if jmErr != nil {
			err = errors.New("funcGroup marshal failed")
			logs.WithContext(ctx).Error(fmt.Sprint(err.Error(), " : ", jmErr.Error()))
			return cloneFunc, err
		}
		if tenantId != "" {
			FuncI = s.ReplaceTenantVariables(ctx, projectId, tenantId, "", FuncI)
		}
		FuncI = s.ReplaceVariables(ctx, projectId, FuncI, reqBody)
		jmErr = json.Unmarshal(FuncI, &cloneFunc)
		if jmErr != nil {
			err = errors.New("funcGroup unmarshal failed")
			logs.WithContext(ctx).Error(fmt.Sprint(err.Error(), " : ", jmErr.Error()))
			return cloneFunc, err
		}
		cloneFunc.TokenSecretKey = prg.ProjectSettings.ClaimsKey
	} else {
		return cloneFunc, errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
	}
	newCloneFunc, _ := cloneFunc.Clone(ctx)
	cloneFunc = *newCloneFunc

	var errArray []string
	for k, v := range cloneFunc.FuncSteps {
		fs := cloneFunc.FuncSteps[k]
		fs.ParentFuncGroupName = cloneFunc.FuncGroupName
		// if eventName is not empty, then set the async event name and message for all childing forcing it to run asyncronously
		if eventName != "" {
			fs.AsyncEventName = eventName
			fs.Async = true
			fs.AsyncMessage = fmt.Sprintf("{\"event_name\":\"%s\"}", eventName)
		}
		err = ms.LoadRoutesForFunction(ctx, fs, v.RouteName, projectId, tenantId, host, v.Path, method, headers, s, cloneFunc.TokenSecretKey, reqBody, fromAsync)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			errArray = append(errArray, err.Error())
		}
	}
	if len(errArray) > 0 {
		err = errors.New(strings.Join(errArray, " , "))
		logs.WithContext(ctx).Error(err.Error())
		return cloneFunc, err
	}
	return
}

// tenantSegment is the tenant part of a url path below a project. A call that carries no
// tenant gets no segment at all, addressing the project level route of the callee.
func tenantSegment(routeTenantId string) string {
	if routeTenantId == "" {
		return ""
	}
	return fmt.Sprint("/", routeTenantId)
}

// eruaiTenantId is the tenant eru-ai is addressed with. eru-ai keys a project's own tools and
// agents under the project id itself, so a call that carries no tenant passes the project id
// and resolves there.
func eruaiTenantId(projectId string, routeTenantId string) string {
	if routeTenantId == "" {
		return projectId
	}
	return routeTenantId
}

func (ms *ModuleStore) LoadRoutesForFunction(ctx context.Context, funcStep *functions.FuncStep, routeName string, projectId string, tenantId string, host string, url string, method string, headers http.Header, s ModuleStoreI, tokenHeaderKey string, reqBody map[string]interface{}, fromAsync bool) (err error) {
	logs.WithContext(ctx).Debug(fmt.Sprint("loadRoutesForFunction - Start : ", funcStep.GetRouteName()))
	var errArray []string
	r := functions.Route{}

	funcStep.FsDb = db.GetDb(s.GetDbType())
	funcStep.FsDb.SetConn(s.GetConn())
	funcStep.ProjectId = projectId
	funcStep.RouteTenantId = eru_utils.JoinTenantRoute(eru_utils.DefaultTenant(ctx), tenantId)
	if funcStep.AsyncEventName != "" {
		var eventI events.EventI
		eventI, err = s.FetchEvent(ctx, projectId, funcStep.AsyncEventName, s)
		if err != nil {
			return
		}
		funcStep.AsyncEvent = eventI
	}

	if funcStep.FunctionName != "" {
		loadNested := func(nctx context.Context, funcName string) (functions.FuncGroup, error) {
			funcGroup, fgErr := ms.GetAndValidateFunc(nctx, funcName, projectId, tenantId, host, url, method, headers, reqBody, s, fromAsync, "")
			if fgErr != nil {
				return functions.FuncGroup{}, fgErr
			}
			tsk := ms.Projects[projectId].ProjectSettings.ClaimsKey
			if funcStep.Async && !fromAsync {
				for k := range funcGroup.FuncSteps {
					funcGroup.FuncSteps[k].Async = true
					funcGroup.FuncSteps[k].AsyncEvent = funcStep.AsyncEvent
					funcGroup.FuncSteps[k].AsyncMessage = funcStep.AsyncMessage
					funcGroup.FuncSteps[k].AsyncEventName = funcStep.AsyncEventName
					funcGroup.FuncSteps[k].Route.TokenSecretKey = tsk
				}
			}
			return funcGroup, nil
		}

		// a templated function_name is only known once the request is being processed, so the
		// function it names is loaded then, through this resolver, instead of now.
		if strings.HasPrefix(funcStep.FunctionName, "{{") {
			funcStep.ResolveFunc = loadNested
		} else {
			funcGroup, fgErr := loadNested(ctx, funcStep.FunctionName)
			if fgErr != nil {
				err = fgErr
				return
			}
			funcStep.FuncGroup = funcGroup
		}

	} else {
		if funcStep.QueryName != "" {
			r.RouteName = funcStep.QueryName
			r.Url = "/"
			r.MatchType = "PREFIX"
			output := ""
			encode := ""
			if funcStep.QueryOutput == "csv" {
				output = "/csv"
			} else if funcStep.QueryOutput == "excel" {
				output = "/excel"
			}

			if funcStep.QueryOutputEncode {
				encode = "/encode"
			}

			r.RewriteUrl = fmt.Sprint("/store/", projectId, tenantSegment(funcStep.RouteTenantId), "/myquery/execute/", funcStep.QueryName, output, encode)
			tg := functions.TargetHost{}
			tg.Method = "POST"
			eruqlbaseurl := getEruqlbaseurl(ctx)
			tmpSplit := strings.Split(eruqlbaseurl, "://")
			tg.Host = eruqlbaseurl
			tg.Scheme = "https"
			if len(tmpSplit) > 0 {
				tg.Scheme = tmpSplit[0]
				tg.Host = tmpSplit[1]
			}
			tg.Allocation = 100
			r.LoopVariable = ""
			r.Condition = ""
			r.TargetHosts = append(r.TargetHosts, tg)
		} else if funcStep.Api.Host != "" {
			r.RouteName = strings.Replace(strings.Replace(funcStep.Api.Host, ".", "", -1), ":", "", -1)
			r.RouteName = funcStep.GetRouteName()
			r.Url = "/"
			r.MatchType = "PREFIX"
			r.RewriteUrl = funcStep.ApiPath
			r.LoopVariable = ""
			r.Condition = ""
			r.OnError = "IGNORE"
			r.TargetHosts = append(r.TargetHosts, funcStep.Api)
		} else if funcStep.ToolName != "" {
			r.RouteName = funcStep.ToolName
			r.Url = "/"
			r.MatchType = "PREFIX"
			toolAction := ""
			if funcStep.ToolAction != "" {
				toolAction = fmt.Sprint("/", funcStep.ToolAction)
			}
			r.RewriteUrl = fmt.Sprint("/", projectId, "/", eruaiTenantId(projectId, funcStep.RouteTenantId), "/execute/tool/", funcStep.ToolName, toolAction)
			r.OnError = "STOP"
			tg := functions.TargetHost{}
			tg.Method = "POST"
			eruaibaseurl := getEruaibaseurl(ctx)
			tmpSplit := strings.Split(eruaibaseurl, "://")
			tg.Host = eruaibaseurl
			tg.Scheme = "https"
			if len(tmpSplit) > 0 {
				tg.Scheme = tmpSplit[0]
				tg.Host = tmpSplit[1]
			}
			tg.Allocation = 100
			r.LoopVariable = ""
			r.Condition = ""
			r.TargetHosts = append(r.TargetHosts, tg)
		} else if funcStep.AgentName != "" {
			r.RouteName = funcStep.AgentName
			r.Url = "/"
			r.MatchType = "PREFIX"
			conversationId := ""
			if funcStep.ConversationId != "" {
				conversationId = fmt.Sprint("/", funcStep.ConversationId)
			}
			r.RewriteUrl = fmt.Sprint("/", projectId, "/", eruaiTenantId(projectId, funcStep.RouteTenantId), "/execute/agent/", funcStep.AgentName, conversationId)
			r.OnError = "STOP"
			tg := functions.TargetHost{}
			tg.Method = "POST"
			eruaibaseurl := getEruaibaseurl(ctx)
			tmpSplit := strings.Split(eruaibaseurl, "://")
			tg.Host = eruaibaseurl
			tg.Scheme = "https"
			if len(tmpSplit) > 0 {
				tg.Scheme = tmpSplit[0]
				tg.Host = tmpSplit[1]
			}
			tg.Allocation = 100
			r.LoopVariable = ""
			r.Condition = ""
			r.TargetHosts = append(r.TargetHosts, tg)
		} else {
			r, err = ms.GetAndValidateRoute(ctx, routeName, projectId, host, url, method, headers, s)
			if err != nil {
				return
			}
		}
		r.TokenSecretKey = ms.Projects[projectId].ProjectSettings.ClaimsKey
		if funcStep.OnError != "" {
			r.OnError = funcStep.OnError
		}
		funcStep.Route = r
	}
	for ck, cv := range funcStep.FuncSteps {
		fs := funcStep.FuncSteps[ck]
		fs.ParentFuncGroupName = funcStep.ParentFuncGroupName
		err = ms.LoadRoutesForFunction(ctx, fs, cv.RouteName, projectId, tenantId, host, cv.Path, method, headers, s, tokenHeaderKey, reqBody, fromAsync)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			errArray = append(errArray, err.Error())
		}
		if fs.AsyncEvent != nil {
			funcStep.AsyncEvent = fs.AsyncEvent
		}
	}
	if len(errArray) > 0 {
		return errors.New(strings.Join(errArray, " , "))
	}
	return
}

func (ms *ModuleStore) SaveFunc(ctx context.Context, funcObj functions.FuncGroup, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug(fmt.Sprint("SaveFunc - Start"))
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if tenantId != "" {
		err = prj.AddTenantFunc(ctx, tenantId, funcObj)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		if persist == true {
			return realStore.SaveTenantObject(ctx, realStore.GetStoreTenantTableName(), "func_id", "func_name", projectId, tenantId, funcObj.FuncGroupName, funcObj, realStore)
		}
		return nil
	}
	err = prj.AddFunc(ctx, funcObj)
	if persist == true {
		return realStore.SaveStore(ctx, projectId, "", realStore)
	}
	return nil
}

func (ms *ModuleStore) RemoveFunc(ctx context.Context, funcName string, projectId string, tenantId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug(fmt.Sprint("RemoveFunc - Start"))
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	prg, ok := ms.Projects[projectId]
	if !ok {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if tenantId != "" {
		err := prg.RemoveTenantFunc(ctx, tenantId, funcName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		logs.WithContext(ctx).Info(fmt.Sprint("RemoveTenantObject called from RemoveFunc"))
		return realStore.RemoveTenantObject(ctx, realStore.GetStoreTenantTableName(), "func_id", "func_name", projectId, tenantId, funcName, realStore)
	}
	if _, ok := prg.FuncGroups[funcName]; ok {
		delete(prg.FuncGroups, funcName)
		logs.WithContext(ctx).Info(fmt.Sprint("SaveStore called from RemoveFunc"))
		return realStore.SaveStore(ctx, projectId, "", realStore)
	} else {
		err := errors.New(fmt.Sprint("Function ", funcName, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}

// SetProjectSettings applies settings to an in memory project without persisting the store.
// It is for callers that build a throwaway store to run a function group in process - they
// still need the project's claims key, because that is the header a func step reads the
// caller's token from, and a project created on the fly carries none.
func (ms *ModuleStore) SetProjectSettings(ctx context.Context, projectId string, projectSettings module_model.ProjectSettings) error {
	logs.WithContext(ctx).Debug("SetProjectSettings - Start")
	if err := ms.checkProjectExists(ctx, projectId); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	ms.Projects[projectId].ProjectSettings = projectSettings
	return nil
}

func (ms *ModuleStore) SaveProjectSettings(ctx context.Context, projectId string, projectSettings module_model.ProjectSettings, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("SaveProjectConfig - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	ms.Projects[projectId].ProjectSettings = projectSettings
	logs.WithContext(ctx).Info("SaveStore called from SaveProjectSettings")
	return realStore.SaveStore(ctx, projectId, "", realStore)
}

func (ms *ModuleStore) GetProjectSettings(ctx context.Context, projectId string) (module_model.ProjectSettings, error) {
	logs.WithContext(ctx).Debug("SaveProjectConfig - Start")
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return module_model.ProjectSettings{}, err
	}
	return ms.Projects[projectId].ProjectSettings, nil
}

func (ms *ModuleStore) checkProjectExists(ctx context.Context, projectId string) error {
	logs.WithContext(ctx).Debug("checkProjectExists - Start")
	_, ok := ms.Projects[projectId]
	if !ok {
		err := errors.New(fmt.Sprint("project ", projectId, " not found"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (ms *ModuleStore) GetFunctionNames(ctx context.Context, projectId string, tenantId string) (functions []FuncListItem, err error) {
	logs.WithContext(ctx).Debug("GetFunctionNames - Start")
	prg, ok := ms.Projects[projectId]
	if !ok {
		err = errors.New(fmt.Sprint("Project ", projectId, " not found"))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	if tenantId != "" {
		funcSources := make(map[string]string)
		lookupOrder := eru_utils.TenantLookupOrder(ctx, tenantId)
		for i := len(lookupOrder) - 1; i >= 0; i-- {
			if tc, tcOk := prg.Tenants[lookupOrder[i]]; tcOk {
				for k := range tc.FuncGroups {
					funcSources[k] = lookupOrder[i]
				}
			}
		}
		for k, source := range funcSources {
			functions = append(functions, FuncListItem{FuncName: k, Source: source})
		}
		return
	}
	for k := range prg.FuncGroups {
		functions = append(functions, FuncListItem{FuncName: k})
	}
	return
}

func (ms *ModuleStore) GetRouteNames(ctx context.Context, projectId string) (routes []string, err error) {
	logs.WithContext(ctx).Debug("GetRouteNames - Start")
	if _, ok := ms.Projects[projectId]; ok {
		if ms.Projects[projectId].Routes == nil {
			return
		} else {
			for k, _ := range ms.Projects[projectId].Routes {
				routes = append(routes, k)
			}
			return
		}
	} else {
		err = errors.New(fmt.Sprint("Project ", projectId, " not found"))
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		return nil, err
	}
}

func (ms *ModuleStore) SaveWf(ctx context.Context, wfObj functions.Workflow, projectId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveWf - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(prj.Workflows))
	err = prj.AddWf(ctx, wfObj)
	logs.WithContext(ctx).Info(fmt.Sprint(prj.Workflows))
	if persist == true {
		return realStore.SaveStore(ctx, projectId, "", realStore)
	}
	return nil
}

func (ms *ModuleStore) RemoveWf(ctx context.Context, wfName string, projectId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug(fmt.Sprint("RemoveWf - Start"))
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if prg, ok := ms.Projects[projectId]; ok {
		if _, ok := prg.Workflows[wfName]; ok {
			delete(prg.Workflows, wfName)
			logs.WithContext(ctx).Info(fmt.Sprint("SaveStore called from RemoveWf"))
			return realStore.SaveStore(ctx, projectId, "", realStore)
		} else {
			err := errors.New(fmt.Sprint("Workflow ", wfName, " does not exists"))
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetWf(ctx context.Context, wfName string, projectId string, s ModuleStoreI) (cloneWf functions.Workflow, err error) {
	logs.WithContext(ctx).Debug("GetWf - Start")
	wfObj := functions.Workflow{}
	if prg, ok := ms.Projects[projectId]; ok {
		if wfObj, ok = prg.Workflows[wfName]; !ok {
			return wfObj, errors.New(fmt.Sprint("Workflow ", wfName, " does not exists"))
		}
		cloneWf, err = ms.GetWfCloneObject(ctx, projectId, wfObj, s)
		cloneWf.WfDb = db.GetDb(s.GetDbType())
		cloneWf.WfDb.SetConn(s.GetConn())
		return
	} else {
		return wfObj, errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
	}
}

func (ms *ModuleStore) GetWfCloneObject(ctx context.Context, projectId string, wfObj functions.Workflow, s ModuleStoreI) (cloneWf functions.Workflow, err error) {
	logs.WithContext(ctx).Debug("GetWfCloneObject - Start")

	wfObjJson, wfObjJsonErr := json.Marshal(wfObj)
	if wfObjJsonErr != nil {
		err = errors.New(fmt.Sprint("error while cloning wfObj (marshal)"))
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(wfObjJsonErr.Error())
		return
	}
	wfObjJson = s.ReplaceVariables(ctx, projectId, wfObjJson, nil)

	iCloneI := reflect.New(reflect.TypeOf(wfObj))
	wfObjCloneErr := json.Unmarshal(wfObjJson, iCloneI.Interface())
	if wfObjCloneErr != nil {
		err = errors.New(fmt.Sprint("error while cloning wfObj(unmarshal)"))
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(wfObjCloneErr.Error())
		return
	}
	return iCloneI.Elem().Interface().(functions.Workflow), nil
}

// dbString reads a text column, tolerating drivers that hand one back as raw bytes, and
// leaving a null column as the empty string.
func dbString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	}
	return ""
}

func (ms *ModuleStore) FetchAsyncEvent(ctx context.Context, asyncId string, asyncStatus string, s ModuleStoreI) (asyncFuncData AsyncFuncData, err error) {
	logs.WithContext(ctx).Debug("FetchAsyncEvent - Start")
	logs.WithContext(ctx).Info(fmt.Sprint("FetchAsyncEvent called for asyncId = ", asyncId, asyncStatus))
	var selectQueries []*models.Queries
	selectQueryFuncAsync := models.Queries{}
	selectQueryFuncAsync.Query = db.GetDb(s.GetDbType()).GetDbQuery(ctx, SELECT_FUNC_ASYNC)
	selectQueryFuncAsync.Vals = append(selectQueryFuncAsync.Vals, asyncId, asyncStatus, asyncStatus)
	selectQueryFuncAsync.Rank = 1
	selectQueries = append(selectQueries, &selectQueryFuncAsync)
	selectOutput, err := eru_utils.ExecuteDbSave(ctx, s.GetConn(), selectQueries)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint("length of selectOutput = ", len(selectOutput)))
	var fVars functions.FuncTemplateVars
	if selectOutput[0] != nil {
		if selectOutput[0][0] != nil {
			fVarsBytes := []byte("")
			fVarsBytesOk := false
			if fVarsBytes, fVarsBytesOk = selectOutput[0][0]["event_msg"].([]byte); !fVarsBytesOk {
				logs.WithContext(ctx).Error(err.Error())
				return
			}
			err = json.Unmarshal(fVarsBytes, &fVars)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return
			}
			asyncFuncData.FuncName = selectOutput[0][0]["func_name"].(string)
			asyncFuncData.FuncStepName = selectOutput[0][0]["func_step_name"].(string)
			asyncFuncData.AsyncId = selectOutput[0][0]["async_id"].(string)
			asyncFuncData.EventMsg = fVars
			asyncFuncData.EventRequest = selectOutput[0][0]["event_request"].(string)
			asyncFuncData.RequestId = selectOutput[0][0]["request_id"].(string)
			asyncFuncData.ProjectId = dbString(selectOutput[0][0]["project_id"])
			asyncFuncData.RouteTenantId = dbString(selectOutput[0][0]["tenant_id"])
		}
	}
	return
}

// AsyncTenantContext resolves who an async row has to be executed for. The row carries the
// route form of the tenant, so it is split back into the tenant and the default tenant and
// the default tenant is put on the context, restoring the same tenant -> default tenant ->
// project fallback the original call ran with. fallbackProjectId covers rows written before
// the project was recorded on the row.
func AsyncTenantContext(ctx context.Context, asyncFuncData AsyncFuncData, fallbackProjectId string) (asyncCtx context.Context, projectId string, tenantId string) {
	projectId = asyncFuncData.ProjectId
	if projectId == "" {
		projectId = fallbackProjectId
	}
	tenantId, defaultTenantId := eru_utils.ParseTenantRoute(asyncFuncData.RouteTenantId)
	return eru_utils.WithDefaultTenant(ctx, defaultTenantId), projectId, tenantId
}

func (ms *ModuleStore) UpdateAsyncEvent(ctx context.Context, asyncId string, asyncStatus string, eventResponse string, s ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("UpdateAsyncEvent - Start")
	var updateQueries []*models.Queries
	updateQueryFuncAsync := models.Queries{}
	updateQueryFuncAsync.Query = db.GetDb(s.GetDbType()).GetDbQuery(ctx, UPDATE_FUNC_ASYNC)
	updateQueryFuncAsync.Vals = append(updateQueryFuncAsync.Vals, asyncStatus, eventResponse, asyncId)
	updateQueryFuncAsync.Rank = 1
	updateQueries = append(updateQueries, &updateQueryFuncAsync)

	_, err = eru_utils.ExecuteDbSave(ctx, s.GetConn(), updateQueries)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (ms *ModuleStore) FetchProjectEvents(ctx context.Context, s ModuleStoreI, cnt int, asyncEventsList []string) (err error) {
	logs.WithContext(ctx).Debug("FetchProjectEvents - Start")
	for _, p := range ms.Projects {
		evts, err := s.FetchEvents(ctx, p.ProjectId)
		if err == nil {
			for _, e := range evts {
				eventName, err := e.GetAttribute("event_name")
				if err == nil {
					if slices.Contains(asyncEventsList, eventName.(string)) {
						gm := server.GetGlobalGoroutineManager(ctx)
						gm.SafeGoWithRestartBehavior("fetch-project-events-polling", func(bgCtx context.Context) {
							ms.StartPolling(bgCtx, p.ProjectId, e, s, cnt)
						}, server.ContinueOnMaxRetries)
					}
				}
			}
		}
	}
	return
}

func (ms *ModuleStore) StartPolling(ctx context.Context, projectId string, event events.EventI, s ModuleStoreI, jcnt int) (err error) {
	eventName, _ := event.GetAttribute("event_name")
	eventType, _ := event.GetAttribute("event_type")
	if eventType == "DB" {
		event.SetCon(s.GetConn(), s.GetDbType())
	}
	logs.WithContext(ctx).Info(fmt.Sprint("StartPolling - Start : ", eventName, " jcnt = ", jcnt))

	gm := server.GetGlobalGoroutineManager(ctx)

	for {
		select {
		case <-ctx.Done():
			logs.WithContext(ctx).Info(fmt.Sprint("Polling stopped for event: ", eventName, " jcnt = ", jcnt))
			return nil
		default:
			logs.WithContext(ctx).Info(fmt.Sprint("polling message for event : ", eventName, " jcnt = ", jcnt))

			eventJobs := make(chan functions.EventJob, 10)
			eventResults := make(chan functions.EventResult, 10)
			done := make(chan bool, 1)

			gm.SafeGo(fmt.Sprintf("polling-allocator-%s-%d", eventName, jcnt), func(allocCtx context.Context) {
				functions.AllocateEvent(allocCtx, event, eventJobs, EventThreads)
			})

			gm.SafeGo(fmt.Sprintf("polling-results-%s-%d", eventName, jcnt), func(bgCtx context.Context) {
				cnt := 0
				for res := range eventResults {
					startTime := time.Now()
					cnt = cnt + 1
					err = ms.ProcessEvents(bgCtx, projectId, res.EventMsgs, event, s, cnt, jcnt)
					if err != nil {
						logs.WithContext(bgCtx).Error(err.Error())
						err = nil
					}
					endTime := time.Now()
					diff := endTime.Sub(startTime)
					logs.WithContext(bgCtx).Info(fmt.Sprint("result processing ending for ", eventName, " job worker ", cnt, " of ", jcnt, " is ", diff.Seconds(), "seconds"))
				}
				done <- true
			})

			noOfWorkers := 1
			functions.CreateWorkerPoolEvent(ctx, noOfWorkers, eventJobs, eventResults, s.GetConn(), jcnt)
			<-done

			event.InitiatPollingInterval(ctx)
		}
	}
}

func (ms *ModuleStore) ProcessEvents(nctx context.Context, projectId string, eventMsgs []events.EventMsg, event events.EventI, s ModuleStoreI, cnt int, jcnt int) (err error) {
	logs.WithContext(nctx).Debug("ProcessEvents - Start")
	aStatus := "PENDING"
	for i, m := range eventMsgs {
		startTime := time.Now()
		asyncStatus := "PROCESSED"
		var asyncFuncData AsyncFuncData
		logs.WithContext(nctx).Info(fmt.Sprint(m.Msg, " ", aStatus))
		failedCount := 0
		processedCount := 0

		msgArray := strings.Split(m.Msg, ",")
		for _, async_id := range msgArray {
			startTimeaid := time.Now()
			ctx := context.WithoutCancel(nctx)
			ctx = logs.NewContext(ctx, zap.String(server_handlers.RequestIdKey, async_id))

			asyncFuncData, err = ms.FetchAsyncEvent(ctx, async_id, aStatus, s)
			if err == nil && asyncFuncData.AsyncId == "" {
				logs.WithContext(ctx).Info(fmt.Sprint("skipping async_id ", async_id, " - not in status ", aStatus, " (claimed by another worker)"))
				continue
			}
			if err != nil {
				failedCount = failedCount + 1
				asyncStatus = "FAILED"
			} else {
				ctx, rowProjectId, rowTenantId := AsyncTenantContext(ctx, asyncFuncData, projectId)
				bodyMap := make(map[string]interface{})
				eventResponseBytes := []byte("{}")
				bodyMapOk := false
				requestBytes := []byte("")
				_ = requestBytes
				requestBytes, err = b64.StdEncoding.DecodeString(asyncFuncData.EventRequest)
				if err != nil {
					failedCount = failedCount + 1
					asyncStatus = "FAILED"
					eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
					logs.WithContext(ctx).Error(err.Error())
				} else {
					var eventReq *http.Request
					if bodyMap, bodyMapOk = asyncFuncData.EventMsg.Vars.Body.(map[string]interface{}); !bodyMapOk {
						logs.WithContext(ctx).Error("Request Body could not be retrieved, setting it as blank")
					}

					if len(requestBytes) > 0 {
						r := bufio.NewReader(bytes.NewBuffer(requestBytes))
						if eventReq, err = http.ReadRequest(r); err != nil { // deserialize request
							failedCount = failedCount + 1
							asyncStatus = "FAILED"
							eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
							logs.WithContext(ctx).Error(err.Error())
						}
					} else {
						projectSettings, err := ms.GetProjectSettings(ctx, rowProjectId)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							err = nil //ignore error and continue
						}
						headers := http.Header{}
						headers.Set("Content-Type", "application/json")
						var tokenMap map[string]interface{}
						var tokenMapOk bool
						if tokenMap, tokenMapOk = asyncFuncData.EventMsg.Vars.Token.(map[string]interface{}); !tokenMapOk {
							logs.WithContext(ctx).Error("Request Toekn not be retrieved, setting it as blank")
						} else {
							if projectSettings.ClaimsKey != "" {
								tokenBytes, err := json.Marshal(tokenMap)
								if err != nil {
									logs.WithContext(ctx).Error(err.Error())
									err = nil //ignore error and continue
								} else {
									headers.Set(projectSettings.ClaimsKey, (string)(tokenBytes))
								}
							}
						}

						eventReq = &http.Request{
							Method: "POST",
							URL: &url.URL{
								Scheme: "http",
								Host:   "localhost",
								Path:   "/",
							},
							Header: headers,
						}
						body, err := json.Marshal(bodyMap)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							failedCount = failedCount + 1
							asyncStatus = "FAILED"
							eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
						} else {
							eventReq.Body = io.NopCloser(bytes.NewBuffer(body))
							eventReq.Header.Set("Content-Length", strconv.Itoa(len(body)))
							eventReq.ContentLength = int64(len(body))
						}
					}
					if asyncStatus != "FAILED" {
						eventReq = eventReq.WithContext(logs.NewContext(ctx, zap.String(server_handlers.RequestIdKey, async_id)))

						funcGroup, err := ms.GetAndValidateFunc(ctx, asyncFuncData.FuncName, rowProjectId, rowTenantId, strings.Split(eventReq.Host, ":")[0], eventReq.URL.Path, eventReq.Method, eventReq.Header, bodyMap, s, true, "")
						if err != nil {
							failedCount = failedCount + 1
							asyncStatus = "FAILED"
							eventResponseBytes, _ = json.Marshal(map[string]interface{}{"error": err.Error()})
							logs.WithContext(ctx).Error(err.Error())
						} else {
							/*reqBytes := []byte("")
							reqBytes, err = b64.StdEncoding.DecodeString(asyncFuncData.EventRequest)
							if err != nil {
								failedCount = failedCount + 1
								asyncStatus = "FAILED"
								logs.WithContext(ctx).Error("event request decoding failed")
							} else {

							var newReq *http.Request
							if newReq, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(reqBytes))); err != nil { // deserialize request
								failedCount = failedCount + 1
								asyncStatus = "FAILED"
								logs.WithContext(ctx).Error("event request deserialization failed")
							}
							*/
							reqVars := make(map[string]*functions.TemplateVars)
							resVars := make(map[string]*functions.TemplateVars)
							if asyncFuncData.EventMsg.ReqVars != nil {
								reqVars = asyncFuncData.EventMsg.ReqVars
							}
							if asyncFuncData.EventMsg.ResVars != nil {
								resVars = asyncFuncData.EventMsg.ResVars
							}
							var response *http.Response
							var funcVarsMap map[string]functions.FuncTemplateVars
							var err error
							func() {
								defer func() {
									if r := recover(); r != nil {
										err = fmt.Errorf("panic while executing func group %s : %v", asyncFuncData.FuncName, r)
										logs.WithContext(ctx).Error(fmt.Sprint(err.Error(), "\nStack trace:\n", string(debug.Stack())))
									}
								}()
								response, funcVarsMap, err = funcGroup.Execute(ctx, eventReq, FuncThreads, LoopThreads, asyncFuncData.FuncStepName, "", true, reqVars, resVars)
							}()
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
									logs.WithContext(ctx).Error(err.Error())
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
										logs.WithContext(ctx).Error(err.Error())
									} else {
										processedCount = processedCount + 1
									}
								}
							}
							defer func() {
								if response != nil {
									response.Body.Close()
								}
							}()
							//}
						}
					}
				}
				_ = s.UpdateAsyncEvent(ctx, async_id, asyncStatus, string(eventResponseBytes), s)
			}
			endTimeaid := time.Now()
			diffaid := endTimeaid.Sub(startTimeaid)
			logs.WithContext(ctx).Info(fmt.Sprint("total time taken for asyncid ", async_id, " is ", diffaid.Seconds(), "seconds"))
		}
		logs.WithContext(nctx).Info(fmt.Sprint("Delete msg called for ", m.MsgIdentifer))
		_ = event.DeleteMessage(nctx, m.MsgIdentifer)
		endTime := time.Now()
		diff := endTime.Sub(startTime)
		logs.WithContext(nctx).Info(fmt.Sprint("total time taken for message processing for job ", jcnt, " of ", cnt, " and msg ", i, " is ", diff.Seconds(), "seconds"))
	}
	return
}

func (ms *ModuleStore) GetStoreWithoutTenants(ctx context.Context, realStore store.StoreI) (b []byte, err error) {
	logs.WithContext(ctx).Debug("GetStoreWithoutTenants - Start")
	realStoreJson, err := json.Marshal(realStore)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	// strip the per-tenant config from the project blob without reconstructing
	// the typed store (interface fields like repos.RepoI cannot be unmarshaled)
	var storeMap map[string]interface{}
	if err = json.Unmarshal(realStoreJson, &storeMap); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if projects, ok := storeMap["projects"].(map[string]interface{}); ok {
		for _, p := range projects {
			if pm, ok := p.(map[string]interface{}); ok {
				delete(pm, "tenants")
			}
		}
	}
	b, err = json.Marshal(storeMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func getTenantLoadQuery(storeTableName string, storeTenantTableName string) string {
	return fmt.Sprint("with prj as (select b.* from ", storeTableName, " a, jsonb_each(config->'projects') b), ",
		"tc as (select project_id, tenant_id, jsonb_build_object('func_groups', jsonb_object_agg(func_name, config)) tenant_config, max(update_date) update_date from ", storeTenantTableName, " group by project_id, tenant_id), ",
		"pt as (select project_id, max(update_date) create_date, jsonb_object_agg(tenant_id, tenant_config) tenant_config from tc group by project_id), ",
		"fpt as (select max(create_date) create_date, jsonb_object_agg(a.key, a.value||jsonb_build_object('tenants', coalesce(b.tenant_config,'{}'::jsonb))) project_config from prj a left join pt b on a.key=b.project_id) ",
		"select a.config||jsonb_build_object('projects', coalesce(b.project_config,'{}'::jsonb)) config, greatest(a.create_date, b.create_date) create_date from ", storeTableName, " a left join fpt b on 1=1")
}

func LoadStore(ctx context.Context, StoreTableName string, StoreTenantTableName string) (ModuleStoreI, error) {
	logs.WithContext(ctx).Info("Loading store")
	storeType := strings.ToUpper(os.Getenv("STORE_TYPE"))
	if storeType == "" {
		storeType = "STANDALONE"
		logs.WithContext(ctx).Info("STORE_TYPE environment variable not found - loading default standlone store")
	}
	var myStore ModuleStoreI
	var err error
	switch storeType {
	case "POSTGRES":
		myStore = new(ModuleDbStore)
		myStore.SetDbType(storeType)
		myStore.SetStoreTableName(StoreTableName)
		myStore.SetStoreTenantTableName(StoreTenantTableName)
		myStore.SetStoreTenantLoadQuery(getTenantLoadQuery(StoreTableName, StoreTenantTableName))
		myStore.CreateConn()
	case "STANDALONE":
		// myStore, err = store.LoadStoreFromFile()
		myStore = new(ModuleFileStore)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New(fmt.Sprint("Invalid STORE_TYPE ", storeType))
	}
	storeBytes, err := myStore.GetStoreByteArray("")
	if err == nil {
		err = json.Unmarshal(storeBytes, myStore)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		err = myStore.SetStoreFromBytes(ctx, storeBytes, myStore)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
	} else {
		logs.WithContext(ctx).Error(err.Error())
	}
	//s.Store = myStore
	return myStore, err
}
func (ms *ModuleStore) ScheduleFunc(ctx context.Context, scheduleConfig scheduler.ScheduleConfig, projectId string, tenantId string, funcName string, reqBody map[string]interface{}, tokenStr string, realStore ModuleStoreI) (jobId string, err error) {
	logs.WithContext(ctx).Info("ScheduleFunc - Start")
	scheduleId := uuid.New().String()
	// the job has to resolve the function again when it fires, so it carries the route form
	// of the tenant - the same tenant___default pair this request came in with.
	routeTenantId := eru_utils.JoinTenantRoute(eru_utils.DefaultTenant(ctx), tenantId)
	scheduler, err := realStore.FetchScheduler(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	jobName := fmt.Sprintf("%s_%s_%s_%s_%s", projectId, tenantId, funcName, scheduleConfig.SchedulerName, scheduleId)
	cronStr := scheduleConfig.GetCronStr(ctx)
	schedulerCommand := scheduleProcedureCall(funcName, string(reqBodyBytes), scheduleConfig.SchedulerName, projectId, routeTenantId)
	jobId, err = scheduler.Schedule(ctx, jobName, schedulerCommand, cronStr)
	if err != nil {
		return "", err
	}
	logs.WithContext(ctx).Info(fmt.Sprint("ScheduleFunc - End : ", jobId))

	tokenObj := map[string]interface{}{}
	err = json.Unmarshal([]byte(tokenStr), &tokenObj)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}

	reqBody["job_id"] = jobId
	requestBody := map[string]interface{}{
		"Vars": map[string]interface{}{
			"Body":    reqBody,
			"OrgBody": reqBody,
			"Token":   tokenObj,
		},
		"ReqVars": map[string]interface{}{},
		"ResVars": map[string]interface{}{},
	}
	reqBodyBytes, err = json.Marshal(requestBody)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	//recalling schedule with same jobname to edit the body with job id
	schedulerCommand = scheduleProcedureCall(funcName, string(reqBodyBytes), scheduleConfig.SchedulerName, projectId, routeTenantId)
	jobId, err = scheduler.Schedule(ctx, jobName, schedulerCommand, cronStr)
	if err != nil {
		return "", err
	}
	ed := scheduleConfig.EndDate
	if ed == "" {
		ed = "2099-12-31"
	}
	var insertQueries []*models.Queries
	insertScheduleLog := models.Queries{}
	insertScheduleLog.Query = db.GetDb(realStore.GetDbType()).GetDbQuery(ctx, INSERT_FUNC_SCHEDULE)
	insertScheduleLog.Vals = []interface{}{scheduleId, projectId, routeTenantId, funcName, "", string(reqBodyBytes), scheduleConfig.SchedulerName, scheduleConfig.SchedulerLabel, jobId, scheduleConfig.StartDate, ed}
	insertScheduleLog.Rank = 2
	insertQueries = append(insertQueries, &insertScheduleLog)

	insertOutput, insertOutputErr := eru_utils.ExecuteDbSave(ctx, realStore.GetConn(), insertQueries)
	if insertOutputErr != nil {
		return "", insertOutputErr
	}
	logs.WithContext(ctx).Info(fmt.Sprint("insertOutput : ", insertOutput))

	return jobId, nil
}

// scheduleProcedureCall builds the pg_cron command that fires the function. project and
// tenant travel with it so the function is resolved for the same tenant when it fires - the
// tenant is the route form, so the schedule_procedure caller keeps the tenant -> default
// tenant -> project fallback.
func scheduleProcedureCall(funcName string, reqBody string, schedulerName string, projectId string, routeTenantId string) string {
	return fmt.Sprint("CALL schedule_procedure('", funcName, "','", reqBody, "','", schedulerName, "','", projectId, "','", routeTenantId, "')")
}

// scheduledJobTenant returns the tenant a job was scheduled under, and whether the job exists.
func (ms *ModuleStore) scheduledJobTenant(ctx context.Context, jobId string, realStore ModuleStoreI) (projectId string, routeTenantId string, found bool, err error) {
	selectSchedule := models.Queries{}
	selectSchedule.Query = db.GetDb(realStore.GetDbType()).GetDbQuery(ctx, SELECT_FUNC_SCHEDULE)
	selectSchedule.Vals = []interface{}{jobId}
	output, err := eru_utils.ExecuteDbFetch(ctx, realStore.GetConn(), selectSchedule)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", "", false, err
	}
	if len(output) == 0 {
		return "", "", false, nil
	}
	projectId = dbString(output[0]["project_id"])
	routeTenantId = dbString(output[0]["tenant_id"])
	return projectId, routeTenantId, true, nil
}

// canUnscheduleJob reports whether a caller on tenantId may remove a job that was scheduled
// under jobRouteTenantId. A tenant reaches its own schedules and those of its default tenant,
// the way a read falls back; a project level schedule is shared by every tenant of the
// project, so only a project level caller may remove it.
func canUnscheduleJob(ctx context.Context, tenantId string, jobRouteTenantId string) bool {
	jobTenantId, _ := eru_utils.ParseTenantRoute(jobRouteTenantId)
	if jobTenantId == "" {
		return tenantId == ""
	}
	return slices.Contains(eru_utils.TenantLookupOrder(ctx, tenantId), jobTenantId)
}

func (ms *ModuleStore) UnScheduleFunc(ctx context.Context, projectId string, tenantId string, jobId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Info("UnScheduleFunc - Start")

	// a job id says nothing about who owns it, so check the job was scheduled within the
	// tenants this caller can reach before touching it.
	jobProjectId, jobRouteTenantId, found, err := ms.scheduledJobTenant(ctx, jobId, realStore)
	if err != nil {
		return err
	}
	notFound := errors.New(fmt.Sprint("scheduled job ", jobId, " not found"))
	if !found || jobProjectId != projectId {
		logs.WithContext(ctx).Error(notFound.Error())
		return notFound
	}
	if !canUnscheduleJob(ctx, tenantId, jobRouteTenantId) {
		logs.WithContext(ctx).Error(fmt.Sprint(notFound.Error(), " for tenant ", tenantId))
		return notFound
	}

	scheduler, err := realStore.FetchScheduler(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	err = scheduler.Unschedule(ctx, jobId, "")
	if err != nil {
		// do nothing and continue
	}
	var deleteQueries []*models.Queries
	deleteScheduleLog := models.Queries{}
	deleteScheduleLog.Query = db.GetDb(realStore.GetDbType()).GetDbQuery(ctx, DELETE_FUNC_SCHEDULE)
	deleteScheduleLog.Vals = []interface{}{jobId}
	deleteScheduleLog.Rank = 1
	deleteQueries = append(deleteQueries, &deleteScheduleLog)

	deleteOutput, deleteOutputErr := eru_utils.ExecuteDbSave(ctx, realStore.GetConn(), deleteQueries)
	if deleteOutputErr != nil {
		return deleteOutputErr
	}
	logs.WithContext(ctx).Info(fmt.Sprint("deleteOutput : ", deleteOutput))

	return nil
}
