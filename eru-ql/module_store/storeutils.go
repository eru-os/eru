package module_store

import (
	"context"
	"errors"
	"fmt"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

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

func (ms *ModuleStore) checkProjectDataSourceExists(ctx context.Context, projectId string, dbAlias string) error {
	logs.WithContext(ctx).Debug("checkProjectDataSourceExists - Start")
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	_, ok := ms.Projects[projectId].DataSources[dbAlias]
	if !ok {
		err = errors.New(fmt.Sprint("datasource ", dbAlias, " not found"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
