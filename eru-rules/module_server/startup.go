package module_server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-rules/module_store"
)

const StoreTableName = "erurules_config"

func StartUp(ctx context.Context) (module_store.ModuleStoreI, error) {
	logs.WithContext(context.Background()).Debug("Start - Start")
	storeType := strings.ToUpper(os.Getenv("STORE_TYPE"))
	if storeType == "" {
		storeType = "STANDALONE"
		logs.WithContext(context.Background()).Info("STORE_TYPE environment variable not found - loading default standlone store")
	}
	var myStore module_store.ModuleStoreI
	var err error
	switch storeType {
	case "POSTGRES":
		myStore = new(module_store.ModuleDbStore)
		myStore.SetDbType(storeType)
		myStore.SetStoreTableName(StoreTableName)
	case "STANDALONE":
		// myStore, err = store.LoadStoreFromFile()
		myStore = new(module_store.ModuleFileStore)
		if err != nil {
			logs.WithContext(context.Background()).Error(err.Error())
			return nil, err
		}
	default:
		err = errors.New(fmt.Sprint("Invalid STORE_TYPE ", storeType))
		logs.WithContext(context.Background()).Error(err.Error())
		return nil, err
	}
	storeBytes, err := myStore.GetStoreByteArray("")
	if err == nil {
		err = json.Unmarshal(storeBytes, myStore)
		//module_store.UnMarshalStore(storeBytes, myStore)
	} else {
		logs.WithContext(context.Background()).Error(err.Error())
	}
	//s.Store = myStore
	return myStore, err
}
