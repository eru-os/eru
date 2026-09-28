package storage

import (
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"

	eruaes "github.com/eru-os/eru/eru-crypto/aes"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-secret-manager/kms"
)

type StorageI interface {
	UploadFile(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error)
	UploadFileB64(ctx context.Context, projectId string, file []byte, fileName string, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error)
	DownloadFile(ctx context.Context, projectId string, folderPath string, fileName string, keyName eruaes.AesKey) (file []byte, err error)
	GetAttribute(attributeName string) (attributeValue interface{}, err error)
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	CreateStorage(ctx context.Context, projectId string, cloneStorage StorageI, persist bool) (err error)
	DeleteStorage(ctx context.Context, projectId string, forceDelete bool, cloneStorage StorageI) (err error)
	Init(ctx context.Context) error
	SetKms(ctx context.Context, kmsObj kms.KmsStoreI) (err error)
	BucketExists(ctx context.Context) (exists bool, err error)
	EmptyBucket(ctx context.Context, projectId string) (err error)
}

type Storage struct {
	StorageType  string        `json:"storage_type" eru:"required"`
	StorageName  string        `json:"storage_name" eru:"required"`
	EncryptFiles bool          `json:"encrypt_files" eru:"required"`
	KeyPair      string        `json:"key_pair"`
	KmsId        string        `json:"key_id"`
	KmsKey       kms.KmsStoreI `json:"-"`
}

func (storage *Storage) GetAttribute(attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "storage_name":
		return storage.StorageName, nil
	case "storage_type":
		return storage.StorageType, nil
	case "key_pair":
		return storage.KeyPair, nil
	case "key_id":
		return storage.KmsId, nil
	default:
		return nil, errors.New("Attribute not found")
	}
}

func GetStorage(storageType string) StorageI {
	switch storageType {
	case "AWS":
		return new(AwsStorage)
	case "GCP":
		return new(GcpStorage)
	case "AZURE":
		return new(AzureStorage)
	case "GDRIVE":
		return new(GdriveStorage)
	case "ONEDRIVE":
		return new(OneDriveStorage)

	default:
		return nil
	}
}

func (storage *Storage) CreateStorage(ctx context.Context, projectId string, cloneStorage StorageI, persist bool) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (storage *Storage) BucketExists(ctx context.Context) (exists bool, err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (storage *Storage) EmptyBucket(ctx context.Context, projectId string) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (storage *Storage) DeleteStorage(ctx context.Context, projectId string, forceDelete bool, cloneStorage StorageI) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (storage *Storage) SetKms(ctx context.Context, kmsObj kms.KmsStoreI) (err error) {
	storage.KmsKey = kmsObj
	return
}

func (storage *Storage) UploadFile(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}
