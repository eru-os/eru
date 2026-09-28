package store

import (
	"context"
	"encoding/json"
	"fmt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"io/ioutil"
	"os"
)

var filePath = "/config/config.json"

type FileStore struct {
	Store
}

func getStoreSaveFilePath() string {
	wd, err := os.Getwd()
	if err != nil {
		logs.Logger.Error(err.Error())
	}
	logs.Logger.Info(fmt.Sprint("wd = ", wd))
	logs.Logger.Info(fmt.Sprint("filePath = ", filePath))
	return fmt.Sprint(wd, filePath)
}

func (store *FileStore) GetStoreByteArray(fp string) (b []byte, err error) {
	logs.Logger.Debug("GetStoreByteArray - Start")
	if fp == "" {
		fp = getStoreSaveFilePath()
	}
	storeData, err := ioutil.ReadFile(fp)
	if err != nil {
		logs.Logger.Error(err.Error())
		//err = store.SaveStore(context.Background(), fp, store)
		//if err == nil {
		//	storeData, err = json.Marshal(store)
		//	if err != nil {
		//		logs.Logger.Error(err.Error())
		//		return nil, err
		//	}
		//}
		return
	}
	return storeData, err
}
func (store *FileStore) LoadStore(fp string, ms StoreI) (err error) {
	logs.Logger.Debug("LoadStore - Start")
	if fp == "" {
		fp = getStoreSaveFilePath()
	}
	storeData, err := ioutil.ReadFile(fp)
	if err != nil {
		logs.Logger.Error(err.Error())
		//store.SaveStore(context.Background(), projectId, fp, nil)
		return err
	}
	err = json.Unmarshal(storeData, ms)
	if err != nil {
		logs.WithContext(context.Background()).Error(err.Error())
		return err
	}
	return nil
}

func (store *FileStore) SaveStore(ctx context.Context, projectId string, fp string, ms StoreI) error {
	logs.WithContext(ctx).Debug("SaveStore - Start")
	if fp == "" {
		fp = getStoreSaveFilePath()
	}
	var err error
	var storeData []byte
	storeData, err = json.Marshal(ms)
	// Check for error during marshaling
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	err = ioutil.WriteFile(fp, storeData, 0644)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return err
}

func (store *FileStore) SaveTenantObject(ctx context.Context, tableName string, idColumn string, nameColumn string, projectId string, tenantId string, name string, config interface{}, ms StoreI) (err error) {
	logs.WithContext(ctx).Debug("SaveTenantObject - Start")
	return store.SaveStore(ctx, projectId, "", ms)
}

func (store *FileStore) RemoveTenantObject(ctx context.Context, tableName string, idColumn string, nameColumn string, projectId string, tenantId string, name string, ms StoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveTenantObject - Start")
	return store.SaveStore(ctx, projectId, "", ms)
}
