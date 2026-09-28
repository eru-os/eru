package module_model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/eru-os/eru/eru-auth/auth"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-secret-manager/sm"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/go-cmp/cmp"
	"net/http"
	"strings"
	"time"
)

type StoreCompare struct {
	store.StoreCompare
	DeleteAuth   []string               `json:"delete_auth"`
	NewAuth      []string               `json:"new_auth"`
	MismatchAuth map[string]interface{} `json:"mismatch_auth"`
}

type ModuleProjectI interface {
	AddAuth(ctx context.Context, authObj auth.AuthI)
	RemoveAuth(ctx context.Context, authType string) error
	CompareProject(ctx context.Context, compareProject Project) (StoreCompare, error)
}
type ExtendedProject struct {
	Project
	Variables     store.Variables `json:"variables"`
	SecretManager sm.SmStoreI     `json:"secret_manager"`
}

type Project struct {
	ProjectId       string                `json:"project_id" eru:"required"`
	Auth            map[string]auth.AuthI `json:"auth"`
	ProjectSettings ProjectSettings       `json:"project_settings"`
	Kids            Kids                  `json:"kids"`
}
type ProjectSettings struct {
	ClaimsKey string `json:"claims_key" eru:"required"`
	KidKey    string `json:"kid_key" eru:"required"`
}

// OAuthProtectedResource is the OAuth 2.0 protected resource metadata (RFC 9728) an MCP client
// reads to find out which authorization server guards the MCP endpoint.
type OAuthProtectedResource struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceName           string   `json:"resource_name,omitempty"`
}

const (
	McpWellKnownPath   = "/.well-known/oauth-protected-resource"
	defaultMcpResource = "/mcp"
)

var McpScopesSupported = []string{"openid", "offline_access"}

// McpResourceUrl rebuilds the MCP endpoint the metadata describes from the url the caller hit.
// The path after McpWellKnownPath is the resource path RFC 9728 inserts, so a request for
// /.well-known/oauth-protected-resource/mcp describes <base url>/mcp. The gateway forwards the
// host it was addressed on in X-Forwarded-Host, since it overwrites Host with the internal
// target before proxying.
func McpResourceUrl(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
		if r.TLS == nil {
			scheme = "http"
		}
	}
	resourcePath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, McpWellKnownPath), "/")
	if resourcePath == "" {
		resourcePath = defaultMcpResource
	}
	return fmt.Sprint(scheme, "://", host, resourcePath)
}

type ApiToken struct {
	TokenId     string `json:"kid"`
	IdentityId  string `json:"user_id"`
	TokenHash   string `json:"token_hash"`
	Token       string `json:"token"`
	TokenName   string `json:"token_name"`
	TokenStatus string `json:"token_status"`
}

const (
	KidStatusActive  = "ACTIVE"
	KidStatusRetired = "RETIRED"
)

// Kid is one signing key in the project's index. The key material itself never appears here - it
// lives in the secret manager. This carries only what is needed to reason about rotation: when the
// key appeared, and whether it is still allowed to sign.
type Kid struct {
	Kid       string    `json:"kid"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Kids indexes a project's signing keys by their stored name.
type Kids map[string]Kid

// UnmarshalJSON accepts both the shape this index used to have - a plain map of name to name - and
// the current one, so a project saved before keys carried metadata still loads. An older entry is
// read as active with no creation date, which is the truthful answer: nobody recorded one.
func (kids *Kids) UnmarshalJSON(b []byte) error {
	current := make(map[string]Kid)
	if err := json.Unmarshal(b, &current); err == nil {
		*kids = current
		return nil
	}
	legacy := make(map[string]string)
	if err := json.Unmarshal(b, &legacy); err != nil {
		return err
	}
	*kids = make(Kids, len(legacy))
	for name := range legacy {
		(*kids)[name] = Kid{Kid: name, Status: KidStatusActive}
	}
	return nil
}

// Retired reports whether a key may no longer sign. It may still verify - that is the whole point
// of retiring rather than removing.
func (kid Kid) Retired() bool {
	return strings.EqualFold(kid.Status, KidStatusRetired)
}

func (prj *Project) AddKid(ctx context.Context, kid string) error {
	logs.WithContext(ctx).Debug("AddKid - Start")
	if prj.Kids == nil {
		prj.Kids = make(Kids)
	}
	prj.Kids[kid] = Kid{Kid: kid, Status: KidStatusActive, CreatedAt: time.Now().UTC()}
	return nil
}

// SetKidStatus retires or reactivates a key without touching the key material.
func (prj *Project) SetKidStatus(ctx context.Context, kid string, status string) error {
	logs.WithContext(ctx).Debug("SetKidStatus - Start")
	existing, ok := prj.Kids[kid]
	if !ok {
		err := errors.New(fmt.Sprint("kid ", kid, " does not exists"))
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
	existing.Status = strings.ToUpper(status)
	prj.Kids[kid] = existing
	return nil
}
func (prj *Project) RemoveKid(ctx context.Context, kid string) error {
	logs.WithContext(ctx).Debug("RemoveKid - Start")
	delete(prj.Kids, kid)
	return nil
}
func (prj *Project) AddAuth(ctx context.Context, authType string, authObjI auth.AuthI) error {
	logs.WithContext(ctx).Debug("AddAuth - Start")
	prj.Auth[authType] = authObjI
	return nil
}
func (prj *Project) RemoveAuth(ctx context.Context, authType string) error {
	logs.WithContext(ctx).Debug("RemoveAuth - Start")
	delete(prj.Auth, authType)
	return nil
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

	for _, ma := range ePrj.Auth {
		maNameI, _ := ma.GetAttribute(ctx, "auth_name")
		maName := maNameI.(string)
		var diffR utils.DiffReporter
		aFound := false
		for _, ca := range compareProject.Auth {
			caNameI, _ := ca.GetAttribute(ctx, "auth_name")
			caName := caNameI.(string)
			if maName == caName {
				aFound = true
				if !cmp.Equal(ma, ca, cmp.Reporter(&diffR)) {
					if storeCompare.MismatchAuth == nil {
						storeCompare.MismatchAuth = make(map[string]interface{})
					}
					storeCompare.MismatchAuth[maName] = diffR.Output()
				}
				break
			}
		}
		if !aFound {
			storeCompare.DeleteAuth = append(storeCompare.DeleteAuth, maName)
		}
	}
	for _, ca := range compareProject.Auth {
		caNameI, _ := ca.GetAttribute(ctx, "auth_name")
		caName := caNameI.(string)
		rFound := false
		for _, ma := range ePrj.Auth {
			maNameI, _ := ma.GetAttribute(ctx, "auth_name")
			maName := maNameI.(string)
			if maName == caName {
				rFound = true
				break
			}
		}
		if !rFound {
			storeCompare.NewAuth = append(storeCompare.NewAuth, caName)
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
	var kids Kids
	if _, ok := ePrjMap["kids"]; ok {
		if ePrjMap["kids"] != nil {
			err = json.Unmarshal(*ePrjMap["kids"], &kids)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.Kids = kids
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

	var auths map[string]*json.RawMessage
	if _, ok := ePrjMap["auth"]; ok {
		if ePrjMap["auth"] != nil {
			err = json.Unmarshal(*ePrjMap["auth"], &auths)
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
				err = json.Unmarshal(*authObj["auth_type"], &authType)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				var authName string
				err = json.Unmarshal(*authObj["auth_name"], &authName)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				authI := auth.GetAuth(authType)
				err = authI.MakeFromJson(ctx, authJson)
				if err == nil {
					if ePrj.Auth == nil {
						ePrj.Auth = make(map[string]auth.AuthI)
					}
					ePrj.Auth[authName] = authI
				} else {
					return err
				}
			}
		} else {
			logs.WithContext(ctx).Info("auth attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("auth attribute not found in store")
	}

	return nil
}
