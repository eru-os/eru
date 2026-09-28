package file_model

import (
	"context"
	"encoding/json"
	eruaes "github.com/eru-os/eru/eru-crypto/aes"
	erursa "github.com/eru-os/eru/eru-crypto/rsa"
	"github.com/eru-os/eru/eru-files/storage"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-secret-manager/sm"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type StoreCompare struct {
	store.StoreCompare
	DeleteStorages   []string               `json:"delete_storages"`
	NewStorages      []string               `json:"new_storages"`
	MismatchStorages map[string]interface{} `json:"mismatch_storages"`
	DeleteKeys       []string               `json:"delete_keys"`
	NewKeys          []string               `json:"new_keys"`
	MismatchKeys     map[string]interface{} `json:"mismatch_keys"`
}

type FileProjectI interface {
	AddStorage(ctx context.Context, storageObj storage.StorageI)
	GenerateRsaKeyPair(ctx context.Context, bits int)
	CompareProject(ctx context.Context, compareProject Project) (StoreCompare, error)
}

type Project struct {
	ProjectId       string                       `json:"project_id" eru:"required"`
	Storages        map[string]storage.StorageI  `json:"storages"`
	RsaKeyPairs     map[string]erursa.RsaKeyPair `json:"rsa_keypairs"`
	AesKeys         map[string]eruaes.AesKey     `json:"aes_keys"`
	ProjectSettings ProjectSettings              `json:"project_settings"`
}

type ExtendedProject struct {
	Project
	Variables     store.Variables `json:"variables"`
	SecretManager sm.SmStoreI     `json:"secret_manager"`
}

type ProjectSettings struct {
	ClaimsKey string `json:"claims_key" eru:"required"`
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

	var ak map[string]eruaes.AesKey
	if _, ok := ePrjMap["aes_keys"]; ok {
		if ePrjMap["aes_keys"] != nil {
			err = json.Unmarshal(*ePrjMap["aes_keys"], &ak)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.AesKeys = ak
		}
	}

	var rk map[string]erursa.RsaKeyPair
	if _, ok := ePrjMap["rsa_keypairs"]; ok {
		if ePrjMap["rsa_keypairs"] != nil {
			err = json.Unmarshal(*ePrjMap["rsa_keypairs"], &rk)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.RsaKeyPairs = rk
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

	var storages map[string]*json.RawMessage
	if _, ok := ePrjMap["storages"]; ok {
		if ePrjMap["storages"] != nil {
			err = json.Unmarshal(*ePrjMap["storages"], &storages)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for _, storageJson := range storages {
				var storageObj map[string]*json.RawMessage
				err = json.Unmarshal(*storageJson, &storageObj)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				var storageType string
				err = json.Unmarshal(*storageObj["storage_type"], &storageType)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				var storageName string
				err = json.Unmarshal(*storageObj["storage_name"], &storageName)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				storageI := storage.GetStorage(storageType)
				err = storageI.MakeFromJson(ctx, storageJson)
				if err == nil {
					if ePrj.Storages == nil {
						ePrj.Storages = make(map[string]storage.StorageI)
					}
					ePrj.Storages[storageName] = storageI
				} else {
					return err
				}
			}
		} else {
			logs.WithContext(ctx).Info("storage attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("storage attribute not found in store")
	}

	return nil
}

func (prj *Project) AddStorage(ctx context.Context, storageObjI storage.StorageI) error {
	logs.WithContext(ctx).Debug("AddStorage - Start")
	storageName, err := storageObjI.GetAttribute("storage_name")
	if err == nil {
		prj.Storages[storageName.(string)] = storageObjI
		return nil
	}
	return err
}

func (prj *Project) GenerateRsaKeyPair(ctx context.Context, bits int, keyPairName string) (erursa.RsaKeyPair, error) {
	logs.WithContext(ctx).Debug("GenerateRsaKeyPair - Start")
	if prj.RsaKeyPairs == nil {
		prj.RsaKeyPairs = make(map[string]erursa.RsaKeyPair)
	}
	var err error
	prj.RsaKeyPairs[keyPairName], err = erursa.GenerateKeyPair(ctx, bits)
	return prj.RsaKeyPairs[keyPairName], err
}

func (prj *Project) GenerateAesKey(ctx context.Context, bits int, keyName string) (eruaes.AesKey, error) {
	logs.WithContext(ctx).Debug("GenerateAesKey - Start")
	if prj.AesKeys == nil {
		prj.AesKeys = make(map[string]eruaes.AesKey)
	}
	var err error
	prj.AesKeys[keyName], err = eruaes.GenerateKey(ctx, bits)
	return prj.AesKeys[keyName], err
}

func (ePrj *ExtendedProject) CompareProject(ctx context.Context, compareProject ExtendedProject) (StoreCompare, error) {
	logs.WithContext(ctx).Debug("CompareProject - Start")
	storeCompare := StoreCompare{}
	storeCompare.CompareVariables(ctx, ePrj.Variables, compareProject.Variables)
	storeCompare.CompareSecretManager(ctx, ePrj.SecretManager, compareProject.SecretManager)

	var diffR utils.DiffReporter
	if !cmp.Equal(ePrj.ProjectSettings, compareProject.ProjectSettings, cmp.Reporter(&diffR)) {
		if storeCompare.MismatchSettings == nil {
			storeCompare.MismatchSettings = make(map[string]interface{})
		}
		storeCompare.MismatchSettings["settings"] = diffR.Output()
	}

	for _, ms := range ePrj.Storages {
		msNameI, _ := ms.GetAttribute("storage_name")
		msName := msNameI.(string)
		var diffR utils.DiffReporter
		sFound := false
		for _, cs := range compareProject.Storages {
			csNameI, _ := cs.GetAttribute("storage_name")
			csName := csNameI.(string)
			if msName == csName {
				sFound = true
				if !cmp.Equal(ms, cs, cmpopts.IgnoreUnexported(storage.AwsStorage{}), cmp.Reporter(&diffR)) {
					if storeCompare.MismatchStorages == nil {
						storeCompare.MismatchStorages = make(map[string]interface{})
					}
					storeCompare.MismatchStorages[msName] = diffR.Output()
				}
				break
			}
		}
		if !sFound {
			storeCompare.DeleteStorages = append(storeCompare.DeleteStorages, msName)
		}
	}

	for _, cs := range compareProject.Storages {
		csNameI, _ := cs.GetAttribute("storage_name")
		csName := csNameI.(string)
		sFound := false
		for _, ms := range ePrj.Storages {
			msNameI, _ := ms.GetAttribute("storage_name")
			msName := msNameI.(string)
			if msName == csName {
				sFound = true
				break
			}
		}
		if !sFound {
			storeCompare.NewStorages = append(storeCompare.NewStorages, csName)
		}
	}

	//compare Keys
	for mKey, mk := range ePrj.AesKeys {
		var diffR utils.DiffReporter
		kFound := false
		for cKey, ck := range compareProject.AesKeys {
			if mKey == cKey {
				kFound = true
				if !cmp.Equal(mk, ck, cmp.Reporter(&diffR)) {
					if storeCompare.MismatchKeys == nil {
						storeCompare.MismatchKeys = make(map[string]interface{})
					}
					storeCompare.MismatchKeys[mKey] = diffR.Output()
				}
				break
			}
		}
		if !kFound {
			storeCompare.DeleteKeys = append(storeCompare.DeleteKeys, mKey)
		}
	}

	for cKey, _ := range compareProject.AesKeys {
		kFound := false
		for mKey, _ := range ePrj.AesKeys {
			if mKey == cKey {
				kFound = true
				break
			}
		}
		if !kFound {
			storeCompare.NewKeys = append(storeCompare.NewKeys, cKey)
		}
	}

	return storeCompare, nil
}
