package sm

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/eru-os/eru/eru-cache/cache"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const (
	AuthTypeSecret = "SECRET"
	AuthTypeIAM    = "IAM"
)

type SmStore struct {
	SmStoreType    string            `json:"sm_store_type" eru:"required"`
	CacheStoreType string            `json:"cache_store_type" eru:"required"`
	CacheStore     cache.CacheStoreI `json:"-"`
}

type SmStoreI interface {
	Init(ctx context.Context) (err error)
	FetchSmValue(ctx context.Context) (resultJson map[string]string, err error)
	SetSmValue(ctx context.Context, secretName string, secretJson map[string]string) (err error)
	UnsetSmValue(ctx context.Context, secretName string, secretKey string) (err error)
	GetSmValue(ctx context.Context, projectId string, secretName string, secretKey string, forceFetch bool) (secretValue interface{}, err error)
	GetSmValues(ctx context.Context, secretName string) (secretValues map[string]string, err error)
	GetSecretName() string
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	InitCache(ctx context.Context, projectId string) error
	GetCacheStore() cache.CacheStoreI
	SetCacheStore(cache.CacheStoreI)
}

func GetSm(storageType string) SmStoreI {
	switch storageType {
	case "AWS":
		return new(AwsSmStore)
	case "GCP":
		return new(GcpSmStore)

	default:
		return nil
	}
}

func (smStore *SmStore) Init(ctx context.Context) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (smStore *SmStore) GetCacheStore() cache.CacheStoreI {
	return smStore.CacheStore
}
func (smStore *SmStore) SetCacheStore(cs cache.CacheStoreI) {
	smStore.CacheStore = cs
}

func (smStore *SmStore) InitCache(ctx context.Context, projectId string) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (smStore *SmStore) FetchSmValue(ctx context.Context) (resultJson map[string]string, err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}
func (smStore *SmStore) SetSmValue(ctx context.Context, secretName string, secretJson map[string]string) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (smStore *SmStore) UnsetSmValue(ctx context.Context, secretName string, secretKey string) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (smStore *SmStore) GetSmValue(ctx context.Context, projectId string, secretName string, secretKey string, forceFetch bool) (secretValue interface{}, err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}
func (smStore *SmStore) GetSmValues(ctx context.Context, secretName string) (secretValues map[string]string, err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (smStore *SmStore) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &smStore)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
func (smStore *SmStore) GetSecretName() string {
	return ""
}
