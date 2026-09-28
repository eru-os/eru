package module_store

import (
	"context"
	"errors"
	"fmt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-rules/module_model"
	"github.com/eru-os/eru/eru-store/store"
	"time"
)

type StoreHolder struct {
	Store ModuleStoreI
}
type ModuleStoreI interface {
	store.StoreI
	SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error
	RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error
	GetProjectConfig(ctx context.Context, projectId string) (*module_model.Project, error)
	GetProjectList(ctx context.Context) []map[string]interface{}
	SaveDataType(ctx context.Context, projectId string, dataType module_model.DataType, realStore ModuleStoreI, persist bool) error
	RemoveDataType(ctx context.Context, projectId string, dataTypeName string, realStore ModuleStoreI) error
	GetDataTypeList(ctx context.Context, projectId string) (map[string]module_model.DataType, error)
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
	//TODO to handle edit project once new project attributes are finalized
	if _, ok := ms.Projects[projectId]; !ok {
		project := new(module_model.Project)
		project.ProjectId = projectId
		if ms.Projects == nil {
			ms.Projects = make(map[string]*module_model.Project)
		}
		if project.DMNs == nil {
			project.DMNs = make(map[string]module_model.DMN)
		}
		if project.DataTypes == nil {
			project.DataTypes = make(map[string]module_model.DataType)
		}
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

func (ms *ModuleStore) RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveProject - Start")
	if _, ok := ms.Projects[projectId]; ok {
		delete(ms.Projects, projectId)
		logs.WithContext(ctx).Info("SaveStore called from RemoveProject")
		return realStore.SaveStore(ctx, projectId, "", realStore)
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " already exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetProjectConfig(ctx context.Context, projectId string) (*module_model.Project, error) {
	logs.WithContext(ctx).Debug("GetProjectConfig - Start")
	if _, ok := ms.Projects[projectId]; ok {
		return ms.Projects[projectId], nil
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " already exists"))
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
		project["projectName"] = k
		project["createdBy"] = "user 1"
		project["lastUpdateDate"] = time.Now()
		projects[i] = project
		i++
	}
	return projects
}

func (ms *ModuleStore) SaveDataType(ctx context.Context, projectId string, dataType module_model.DataType, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveDataType - Start")
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		return err
	}
	ms.Projects[projectId].DataTypes[dataType.Name] = dataType
	if persist == true {
		logs.WithContext(ctx).Info("SaveStore called from SaveDataType")
		return realStore.SaveStore(ctx, projectId, "", realStore)
	} else {
		return nil
	}
}

func (ms *ModuleStore) RemoveDataType(ctx context.Context, projectId string, dataTypeName string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveDataType - Start")
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		return err
	}
	if _, ok := ms.Projects[projectId].DataTypes[dataTypeName]; ok {
		delete(ms.Projects[projectId].DataTypes, dataTypeName)
		logs.WithContext(ctx).Info("SaveStore called from RemoveDataType")
		return realStore.SaveStore(ctx, projectId, "", realStore)
	} else {
		err = errors.New(fmt.Sprint("Datatype ", dataTypeName, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetDataTypeList(ctx context.Context, projectId string) (map[string]module_model.DataType, error) {
	logs.WithContext(ctx).Debug("GetDataTypeList - Start")
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		return nil, err
	}
	return ms.Projects[projectId].DataTypes, nil
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
