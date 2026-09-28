package module_store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	authtype "github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_model"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-repos/repos"
	"github.com/eru-os/eru/eru-secret-manager/kms"
	"github.com/eru-os/eru/eru-secret-manager/sm"
	"github.com/eru-os/eru/eru-store/store"
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

func UnMarshalStore(ctx context.Context, b []byte, msi ModuleStoreI) error {
	logs.WithContext(ctx).Debug("UnMarshalStore - Start")
	var storeMap map[string]*json.RawMessage
	err := json.Unmarshal(b, &storeMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	var vars map[string]store.Variables
	if _, ok := storeMap["variables"]; ok {
		if storeMap["variables"] != nil {
			err = json.Unmarshal(*storeMap["variables"], &vars)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			msi.SetVars(ctx, vars)
		}
	}

	var prjSm map[string]*json.RawMessage
	if _, ok := storeMap["secret_manager"]; ok {
		if storeMap["secret_manager"] != nil {
			err = json.Unmarshal(*storeMap["secret_manager"], &prjSm)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for prj, smJson := range prjSm {
				var smObj map[string]*json.RawMessage
				err = json.Unmarshal(*smJson, &smObj)
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
						err = msi.SaveSm(ctx, prj, smI, msi, false)
						if err != nil {
							return err
						}
					} else {
						return err
					}
				} else {
					logs.WithContext(ctx).Info("ignoring secret manager as sm_store_type attribute not found")
				}
			}
		} else {
			logs.WithContext(ctx).Info("secret manager attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("secret manager attribute not found in store")
	}

	var prjKms map[string]*json.RawMessage
	if _, ok := storeMap["kms"]; ok {
		if storeMap["kms"] != nil {
			err = json.Unmarshal(*storeMap["kms"], &prjKms)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for prj, kmsJson := range prjKms {
				var kmsObj map[string]*json.RawMessage
				err = json.Unmarshal(*kmsJson, &kmsObj)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				for _, kJson := range kmsObj {
					var kObj map[string]*json.RawMessage
					err = json.Unmarshal(*kJson, &kObj)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					var kmsType string
					if _, stOk := kObj["kms_store_type"]; stOk {
						err = json.Unmarshal(*kObj["kms_store_type"], &kmsType)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
						kmsI := kms.GetKms(kmsType)
						err = kmsI.MakeFromJson(ctx, kJson)
						if err == nil {
							err = msi.SaveKms(ctx, prj, kmsI, msi, false)
							if err != nil {
								return err
							}
						} else {
							return err
						}
					} else {
						logs.WithContext(ctx).Info("ignoring kms as kms_store_type attribute not found")
					}
				}
			}
		} else {
			logs.WithContext(ctx).Info("kms attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("kms attribute not found in store")
	}

	var prjRepo map[string]*json.RawMessage
	if _, ok := storeMap["repos"]; ok {
		if storeMap["repos"] != nil {
			err = json.Unmarshal(*storeMap["repos"], &prjRepo)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for prj, repoJson := range prjRepo {
				var repoObj map[string]*json.RawMessage
				err = json.Unmarshal(*repoJson, &repoObj)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				var repoType string
				if _, rtOk := repoObj["repo_type"]; rtOk {
					err = json.Unmarshal(*repoObj["repo_type"], &repoType)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					repoI := repos.GetRepo(repoType)
					err = repoI.MakeFromJson(ctx, repoJson)
					if err == nil {
						err = msi.SaveRepo(ctx, prj, repoI, msi, false)
						if err != nil {
							return err
						}
					} else {
						return err
					}
				} else {
					logs.WithContext(ctx).Info("ignoring repo as repo type not found")
				}
			}
		} else {
			logs.WithContext(ctx).Info("repos attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("repos attribute not found in store")
	}

	var prjs map[string]*json.RawMessage
	if _, ok := storeMap["projects"]; ok {

		err = json.Unmarshal(*storeMap["projects"], &prjs)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		for prj, prjJson := range prjs {
			err = msi.SaveProject(ctx, prj, nil, false)
			if err != nil {
				return err
			}
			var prjObjs map[string]*json.RawMessage
			err = json.Unmarshal(*prjJson, &prjObjs)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			p, e := msi.GetProjectConfig(ctx, prj)
			if e != nil {
				return err
			}
			var ps module_model.ProjectSettings
			if _, ok := prjObjs["project_settings"]; ok {
				if prjObjs["project_settings"] != nil {
					err = json.Unmarshal(*prjObjs["project_settings"], &ps)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					p.ProjectSettings = ps
				}
			}

			var kids module_model.Kids
			if _, ok := prjObjs["kids"]; ok {
				if prjObjs["kids"] != nil {
					err = json.Unmarshal(*prjObjs["kids"], &kids)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					p.Kids = kids
				}
			}

			var auths map[string]*json.RawMessage
			if _, ok = prjObjs["auth"]; ok {
				err = json.Unmarshal(*prjObjs["auth"], &auths)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				for _, authJson := range auths {
					var authObj map[string]*json.RawMessage
					err = json.Unmarshal(*authJson, &authObj)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return err
					}
					var authType string
					if at, ok := authObj["auth_type"]; ok {
						err = json.Unmarshal(*at, &authType)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
					}
					authI := authtype.GetAuth(authType)
					err = authI.MakeFromJson(ctx, authJson)
					if err == nil {
						err = msi.SaveAuth(ctx, authI, prj, msi, false)
						if err != nil {
							return err
						}
					} else {
						return err
					}
				}
			}
		}
	}
	return nil
}

func GetStore(storeType string) ModuleStoreI {
	switch storeType {
	case "POSTGRES":
		return new(ModuleDbStore)
	case "STANDALONE":
		return new(ModuleFileStore)
	default:
		return nil
	}
}
