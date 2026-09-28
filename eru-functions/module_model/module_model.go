package module_model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/eru-os/eru/eru-functions/functions"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	scheduler "github.com/eru-os/eru/eru-scheduler/scheduler"
	"github.com/eru-os/eru/eru-secret-manager/sm"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/go-cmp/cmp"
)

type StoreCompare struct {
	store.StoreCompare
	DeleteRoutes   []string               `json:"delete_routes"`
	NewRoutes      []string               `json:"new_routes"`
	MismatchRoutes map[string]interface{} `json:"mismatch_routes"`
	DeleteFuncs    []string               `json:"delete_funcs"`
	NewFuncs       []string               `json:"new_funcs"`
	MismatchFuncs  map[string]interface{} `json:"mismatch_funcs"`
}
type SampleRequest struct {
	RequestId     string                 `json:"request_id" eru:"required"`
	RequestName   string                 `json:"request_name" eru:"required"`
	RequestBody   map[string]interface{} `json:"request_body" eru:"required"`
	FuncGroupName string                 `json:"func_group_name" eru:"required"`
}
type ModuleProjectI interface {
	AddFunc(ctx context.Context, funcObj functions.FuncGroup) error
	AddRoute(ctx context.Context, routeObj functions.Route) error
	CompareProject(ctx context.Context, compareProject Project) (StoreCompare, error)
	AddWf(ctx context.Context, wfObj functions.Workflow) error
}

//type ProjectConfig struct {
//	//AesKey         AesKey
//	TokenSecret functions.TokenSecret
//	//ProjectGitRepo ProjectGitRepo
//}

type ProjectSettings struct {
	ClaimsKey           string `json:"claims_key" eru:"required"`
	AsyncRepollWaitTime int32  `json:"async_repoll_wait_time"`
}
type ExtendedProject struct {
	Project
	Variables     store.Variables      `json:"variables"`
	SecretManager sm.SmStoreI          `json:"secret_manager"`
	Scheduler     scheduler.SchedulerI `json:"scheduler"`
}

type Project struct {
	ProjectId       string                         `json:"project_id" eru:"required"`
	Routes          map[string]functions.Route     `json:"routes" eru:"required"`
	FuncGroups      map[string]functions.FuncGroup `json:"func_groups" eru:"required"`
	Workflows       map[string]functions.Workflow  `json:"workflows" eru:"required"`
	Tenants         map[string]TenantConfig        `json:"tenants"`
	ProjectSettings ProjectSettings                `json:"project_settings"`
	//Authorizers   map[string]functions.Authorizer
}

type TenantConfig struct {
	TenantId   string                         `json:"tenant_id" eru:"required"`
	FuncGroups map[string]functions.FuncGroup `json:"func_groups"`
}

func (prj *Project) AddTenantFunc(ctx context.Context, tenantId string, funcObj functions.FuncGroup) error {
	logs.WithContext(ctx).Debug("AddTenantFunc - Start")
	if prj.Tenants == nil {
		prj.Tenants = make(map[string]TenantConfig)
	}
	tc, ok := prj.Tenants[tenantId]
	if !ok {
		tc = TenantConfig{TenantId: tenantId}
	}
	if tc.FuncGroups == nil {
		tc.FuncGroups = make(map[string]functions.FuncGroup)
	}
	tc.FuncGroups[funcObj.FuncGroupName] = funcObj
	prj.Tenants[tenantId] = tc
	return nil
}

func (prj *Project) RemoveTenantFunc(ctx context.Context, tenantId string, funcName string) error {
	logs.WithContext(ctx).Debug("RemoveTenantFunc - Start")
	tc, ok := prj.Tenants[tenantId]
	if !ok {
		return errors.New(fmt.Sprint("tenant ", tenantId, " does not exists"))
	}
	if _, ok := tc.FuncGroups[funcName]; !ok {
		return errors.New(fmt.Sprint("Function ", funcName, " does not exists"))
	}
	delete(tc.FuncGroups, funcName)
	prj.Tenants[tenantId] = tc
	return nil
}

func (prj *Project) AddRoute(ctx context.Context, routeObj functions.Route) error {
	logs.WithContext(ctx).Debug("AddRoute - Start")
	if prj.Routes == nil {
		prj.Routes = make(map[string]functions.Route)
	}
	prj.Routes[routeObj.RouteName] = routeObj
	return nil
}

func (prj *Project) AddFunc(ctx context.Context, funcObj functions.FuncGroup) error {
	logs.WithContext(ctx).Debug("AddFunc - Start")
	if prj.FuncGroups == nil {
		prj.FuncGroups = make(map[string]functions.FuncGroup)
	}
	prj.FuncGroups[funcObj.FuncGroupName] = funcObj
	return nil
}

func (ePrj *ExtendedProject) CompareProject(ctx context.Context, compareProject ExtendedProject) (StoreCompare, error) {
	logs.WithContext(ctx).Debug("CompareProject - Start")
	storeCompare := StoreCompare{}
	storeCompare.CompareVariables(ctx, ePrj.Variables, compareProject.Variables)
	storeCompare.CompareSecretManager(ctx, ePrj.SecretManager, compareProject.SecretManager)

	var oDiffR utils.DiffReporter
	if !cmp.Equal(ePrj.ProjectSettings, compareProject.ProjectSettings, cmp.Reporter(&oDiffR)) {
		if storeCompare.MismatchSettings == nil {
			storeCompare.MismatchSettings = make(map[string]interface{})
		}
		storeCompare.MismatchSettings["settings"] = oDiffR.Output()
	}

	for _, mr := range ePrj.Routes {
		var diffR utils.DiffReporter
		rFound := false
		for _, cr := range compareProject.Routes {
			if mr.RouteName == cr.RouteName {
				rFound = true
				if !cmp.Equal(mr, cr, cmp.Reporter(&diffR)) {
					if storeCompare.MismatchRoutes == nil {
						storeCompare.MismatchRoutes = make(map[string]interface{})
					}
					storeCompare.MismatchRoutes[mr.RouteName] = diffR.Output()
				}
				break
			}
		}
		if !rFound {
			storeCompare.DeleteRoutes = append(storeCompare.DeleteRoutes, mr.RouteName)
		}
	}

	for _, cr := range compareProject.Routes {
		rFound := false
		for _, mr := range ePrj.Routes {
			if mr.RouteName == cr.RouteName {
				rFound = true
				break
			}
		}
		if !rFound {
			storeCompare.NewRoutes = append(storeCompare.NewRoutes, cr.RouteName)
		}
	}

	//compare funcs
	for _, mf := range ePrj.FuncGroups {
		var diffR utils.DiffReporter
		fFound := false
		for _, cf := range compareProject.FuncGroups {
			if mf.FuncGroupName == cf.FuncGroupName {
				fFound = true
				if !cmp.Equal(mf, cf, cmp.Reporter(&diffR)) {
					if storeCompare.MismatchFuncs == nil {
						storeCompare.MismatchFuncs = make(map[string]interface{})
					}
					storeCompare.MismatchFuncs[mf.FuncGroupName] = diffR.Output()

				}
				break
			}
		}
		if !fFound {
			storeCompare.DeleteFuncs = append(storeCompare.DeleteFuncs, mf.FuncGroupName)
		}
	}

	for _, cf := range compareProject.FuncGroups {
		fFound := false
		for _, mf := range ePrj.FuncGroups {
			if mf.FuncGroupName == cf.FuncGroupName {
				fFound = true
				break
			}
		}
		if !fFound {
			storeCompare.NewFuncs = append(storeCompare.NewFuncs, cf.FuncGroupName)
		}
	}
	return storeCompare, nil
}

func (ePrj *ExtendedProject) UnmarshalJSON(b []byte) error {
	logs.Logger.Info("UnMarshal ExtendedProject - Start")
	ctx := context.Background()
	var ePrjMap map[string]*json.RawMessage
	err := json.Unmarshal(b, &ePrjMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	projectId := ""
	if _, ok := ePrjMap["project_id"]; ok {
		if ePrjMap["project_id"] != nil {
			err = json.Unmarshal(*ePrjMap["project_id"], &projectId)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.ProjectId = projectId
		}
	}

	var ps ProjectSettings
	if _, ok := ePrjMap["project_settings"]; ok {
		if ePrjMap["project_settings"] != nil {
			err = json.Unmarshal(*ePrjMap["project_settings"], &ps)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.ProjectSettings = ps
		}
	}

	var vars store.Variables
	if _, ok := ePrjMap["variables"]; ok {
		if ePrjMap["variables"] != nil {
			err = json.Unmarshal(*ePrjMap["variables"], &vars)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.Variables = vars
		}
	}

	var rt map[string]functions.Route
	if _, ok := ePrjMap["functions"]; ok {
		if ePrjMap["functions"] != nil {
			err = json.Unmarshal(*ePrjMap["functions"], &rt)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.Routes = rt
		}
	}

	var fn map[string]functions.FuncGroup
	if _, ok := ePrjMap["func_groups"]; ok {
		if ePrjMap["func_groups"] != nil {
			err = json.Unmarshal(*ePrjMap["func_groups"], &fn)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.FuncGroups = fn
		}
	}

	var smObj map[string]*json.RawMessage
	var smJson *json.RawMessage
	if _, ok := ePrjMap["secret_manager"]; ok {
		if ePrjMap["secret_manager"] != nil {
			err = json.Unmarshal(*ePrjMap["secret_manager"], &smObj)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			err = json.Unmarshal(*ePrjMap["secret_manager"], &smJson)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}

			var smType string
			if _, stOk := smObj["sm_store_type"]; stOk {
				err = json.Unmarshal(*smObj["sm_store_type"], &smType)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				smI := sm.GetSm(smType)
				err = smI.MakeFromJson(ctx, smJson)
				if err == nil {
					ePrj.SecretManager = smI
				} else {
					return err
				}
			} else {
				logs.WithContext(ctx).Info("ignoring secret manager as sm_store_type attribute not found")
			}
		} else {
			logs.WithContext(ctx).Info("secret manager attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("secret manager attribute not found in store")
	}

	return nil
}

func (prj *Project) AddWf(ctx context.Context, wfObj functions.Workflow) error {
	logs.WithContext(ctx).Info("AddWf - Start")
	if prj.Workflows == nil {
		prj.Workflows = make(map[string]functions.Workflow)
	}
	prj.Workflows[wfObj.WfName] = wfObj
	return nil
}
