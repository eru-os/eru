package storage

import (
	"context"
	"encoding/json"
	"mime/multipart"

	eruaes "github.com/eru-os/eru/eru-crypto/aes"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type GcpStorage struct {
	Storage
}

func (gcpStorage *GcpStorage) UploadFile(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	_ = projectId
	return docId, err
}

func (gcpStorage *GcpStorage) UploadFileB64(ctx context.Context, projectId string, file []byte, fileName string, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	_ = projectId
	return docId, err
}

func (gcpStorage *GcpStorage) DownloadFile(ctx context.Context, projectId string, folderPath string, fileName string, keyName eruaes.AesKey) (file []byte, err error) {
	_ = projectId
	return
}

func (gcpStorage *GcpStorage) Init(ctx context.Context) error {
	return nil
}

func (gcpStorage *GcpStorage) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	err := json.Unmarshal(*rj, &gcpStorage)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
