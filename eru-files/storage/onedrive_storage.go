package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	eruaes "github.com/eru-os/eru/eru-crypto/aes"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/segmentio/ksuid"
)

const (
	graphDriveBase = "https://graph.microsoft.com/v1.0/me/drive"
)

type OneDriveStorage struct {
	Storage
	AuthName     string `json:"auth_name" eru:"required"`
	RootFolderId string `json:"root_folder_id"`
}

func (o *OneDriveStorage) GetAttribute(attributeName string) (interface{}, error) {
	switch attributeName {
	case "storage_name":
		return o.StorageName, nil
	case "storage_type":
		return o.StorageType, nil
	case "key_pair":
		return o.KeyPair, nil
	case "key_id":
		return o.KmsId, nil
	case "auth_name":
		return o.AuthName, nil
	case "root_folder_id":
		return o.RootFolderId, nil
	default:
		return nil, errors.New("Attribute not found")
	}
}

func (o *OneDriveStorage) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	if err := json.Unmarshal(*rj, &o); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if o.EncryptFiles {
		logs.WithContext(ctx).Warn("encrypt_files is not supported for ONEDRIVE storage; flag will be ignored")
	}
	return nil
}

func (o *OneDriveStorage) Init(ctx context.Context) error {
	return nil
}

func (o *OneDriveStorage) getAccessToken(ctx context.Context, projectId string) (string, error) {
	if o.AuthName == "" {
		return "", errors.New("auth_name is required for ONEDRIVE storage")
	}
	if projectId == "" {
		return "", errors.New("projectId is required for ONEDRIVE getAccessToken")
	}
	baseUrl, _ := ctx.Value("eruauthbaseurl").(string)
	if baseUrl == "" {
		return "", errors.New("eruauthbaseurl not found in context")
	}
	getUrl := fmt.Sprintf("%s/%s/%s/gettoken", strings.TrimRight(baseUrl, "/"), projectId, o.AuthName)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	qParams := map[string]string{}
	if prefix, _ := ctx.Value("tokenkeyprefix").(string); prefix != "" {
		qParams["token_key_prefix"] = prefix
	}
	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodGet, getUrl, headers, map[string]string{}, nil, qParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("gettoken call failed (status %d): %s", statusCode, err.Error()))
		return "", err
	}
	resMap, ok := res.(map[string]interface{})
	if !ok {
		return "", errors.New("gettoken response is not an object")
	}
	at, _ := resMap["access_token"].(string)
	if at == "" {
		return "", errors.New("gettoken response missing access_token")
	}
	return at, nil
}

func (o *OneDriveStorage) graphCall(ctx context.Context, projectId, method, fullUrl string, headers http.Header, params map[string]string, postBody interface{}) (interface{}, http.Header, int, error) {
	tok, err := o.getAccessToken(ctx, projectId)
	if err != nil {
		return nil, nil, 0, err
	}
	if headers == nil {
		headers = http.Header{}
	}
	headers.Set("Authorization", "Bearer "+tok)
	res, respHeaders, _, status, err := utils.CallHttp(ctx, method, fullUrl, headers, map[string]string{}, nil, params, postBody)
	return res, respHeaders, status, err
}

func (o *OneDriveStorage) parentSegment(parentId string) string {
	if parentId == "" || parentId == "root" {
		return "root"
	}
	return "items/" + parentId
}

func (o *OneDriveStorage) rootSegment() string {
	return o.parentSegment(o.RootFolderId)
}

func (o *OneDriveStorage) findChild(ctx context.Context, projectId, parentId, name string) (id string, isFolder bool, err error) {
	seg := o.parentSegment(parentId)
	itemUrl := fmt.Sprintf("%s/%s:/%s", graphDriveBase, seg, url.PathEscape(name))
	res, _, status, callErr := o.graphCall(ctx, projectId, http.MethodGet, itemUrl, nil, map[string]string{"$select": "id,name,folder,file"}, nil)
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if callErr != nil {
		return "", false, callErr
	}
	if status >= 300 {
		return "", false, fmt.Errorf("graph get child returned %d", status)
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		return "", false, errors.New("graph response not an object")
	}
	id, _ = m["id"].(string)
	_, isFolder = m["folder"]
	return
}

func (o *OneDriveStorage) ensureFolder(ctx context.Context, projectId, parentId, name string) (string, error) {
	id, isFolder, err := o.findChild(ctx, projectId, parentId, name)
	if err != nil {
		return "", err
	}
	if id != "" {
		if !isFolder {
			return "", fmt.Errorf("path conflict: %s exists but is not a folder", name)
		}
		return id, nil
	}
	seg := o.parentSegment(parentId)
	createUrl := fmt.Sprintf("%s/%s/children", graphDriveBase, seg)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	body := map[string]interface{}{
		"name":                              name,
		"folder":                            map[string]interface{}{},
		"@microsoft.graph.conflictBehavior": "fail",
	}
	res, _, status, err := o.graphCall(ctx, projectId, http.MethodPost, createUrl, headers, nil, body)
	if err != nil {
		return "", err
	}
	if status >= 300 {
		return "", fmt.Errorf("graph create folder returned %d", status)
	}
	m, _ := res.(map[string]interface{})
	id, _ = m["id"].(string)
	if id == "" {
		return "", errors.New("graph create folder missing id")
	}
	return id, nil
}

func (o *OneDriveStorage) resolveFolderPath(ctx context.Context, projectId, folderPath string, createIfMissing bool) (string, error) {
	parent := o.RootFolderId
	if parent == "" {
		parent = "root"
	}
	folderPath = strings.Trim(folderPath, "/")
	if folderPath == "" {
		return parent, nil
	}
	for _, part := range strings.Split(folderPath, "/") {
		if part == "" {
			continue
		}
		var id string
		var err error
		if createIfMissing {
			id, err = o.ensureFolder(ctx, projectId, parent, part)
		} else {
			var isFolder bool
			id, isFolder, err = o.findChild(ctx, projectId, parent, part)
			if err == nil && id == "" {
				err = fmt.Errorf("folder not found: %s", part)
			}
			if err == nil && !isFolder {
				err = fmt.Errorf("path component is not a folder: %s", part)
			}
		}
		if err != nil {
			return "", err
		}
		parent = id
	}
	return parent, nil
}

func buildOneDriveFileName(docType, baseName string) string {
	id := ksuid.New().String()
	if docType != "" {
		return fmt.Sprint(docType, "_", id, "_", baseName)
	}
	return fmt.Sprint(id, "_", baseName)
}

func (o *OneDriveStorage) uploadBytes(ctx context.Context, projectId, parentId, fileName string, data []byte, contentType string) (string, error) {
	tok, err := o.getAccessToken(ctx, projectId)
	if err != nil {
		return "", err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	seg := o.parentSegment(parentId)
	uploadUrl := fmt.Sprintf("%s/%s:/%s:/content", graphDriveBase, seg, url.PathEscape(fileName))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadUrl, strings.NewReader(string(data)))
	if err != nil {
		return "", err
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("graph upload failed: status %d body %s", resp.StatusCode, string(respBytes))
	}
	var respObj map[string]interface{}
	if err := json.Unmarshal(respBytes, &respObj); err != nil {
		return "", err
	}
	id, _ := respObj["id"].(string)
	return id, nil
}

func (o *OneDriveStorage) UploadFile(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	logs.WithContext(ctx).Debug("UploadFile - Start (ONEDRIVE)")
	if o.EncryptFiles {
		logs.WithContext(ctx).Warn("encrypt_files ignored for ONEDRIVE")
	}
	byteContainer, err := io.ReadAll(file)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	finalFileName := buildOneDriveFileName(docType, header.Filename)
	parentId, err := o.resolveFolderPath(ctx, projectId, folderPath, true)
	if err != nil {
		return "", err
	}
	if _, err = o.uploadBytes(ctx, projectId, parentId, finalFileName, byteContainer, header.Header.Get("Content-Type")); err != nil {
		return "", err
	}
	return joinDocId(folderPath, finalFileName), nil
}

func (o *OneDriveStorage) UploadFileB64(ctx context.Context, projectId string, file []byte, fileName string, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	logs.WithContext(ctx).Debug("UploadFileB64 - Start (ONEDRIVE)")
	if o.EncryptFiles {
		logs.WithContext(ctx).Warn("encrypt_files ignored for ONEDRIVE")
	}
	finalFileName := buildOneDriveFileName(docType, fileName)
	parentId, err := o.resolveFolderPath(ctx, projectId, folderPath, true)
	if err != nil {
		return "", err
	}
	if _, err = o.uploadBytes(ctx, projectId, parentId, finalFileName, file, ""); err != nil {
		return "", err
	}
	return joinDocId(folderPath, finalFileName), nil
}

func (o *OneDriveStorage) DownloadFile(ctx context.Context, projectId string, folderPath string, fileName string, keyName eruaes.AesKey) ([]byte, error) {
	logs.WithContext(ctx).Debug("DownloadFile - Start (ONEDRIVE)")
	parentId, err := o.resolveFolderPath(ctx, projectId, folderPath, false)
	if err != nil {
		return nil, err
	}
	fileId, isFolder, err := o.findChild(ctx, projectId, parentId, fileName)
	if err != nil {
		return nil, err
	}
	if fileId == "" {
		return nil, fmt.Errorf("file not found: %s", joinDocId(folderPath, fileName))
	}
	if isFolder {
		return nil, fmt.Errorf("path is a folder, not a file: %s", joinDocId(folderPath, fileName))
	}
	tok, err := o.getAccessToken(ctx, projectId)
	if err != nil {
		return nil, err
	}
	dlUrl := fmt.Sprintf("%s/items/%s/content", graphDriveBase, fileId)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("graph download failed: status %d body %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

func (o *OneDriveStorage) BucketExists(ctx context.Context) (bool, error) {
	return o.RootFolderId != "", nil
}

func (o *OneDriveStorage) CreateStorage(ctx context.Context, projectId string, cloneStorage StorageI, persist bool) error {
	if !persist {
		return nil
	}
	exists, err := cloneStorage.BucketExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		logs.WithContext(ctx).Info("skipping OneDrive folder creation as root_folder_id is already set")
		return nil
	}
	nameI, _ := cloneStorage.GetAttribute("storage_name")
	name, _ := nameI.(string)
	if name == "" {
		return errors.New("storage_name required to create OneDrive folder")
	}
	id, err := o.ensureFolder(ctx, projectId, "root", name)
	if err != nil {
		return err
	}
	o.RootFolderId = id
	if co, ok := cloneStorage.(*OneDriveStorage); ok {
		co.RootFolderId = id
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("created OneDrive root folder: %s (id=%s)", name, id))
	return nil
}

func (o *OneDriveStorage) DeleteStorage(ctx context.Context, projectId string, forceDelete bool, cloneStorage StorageI) error {
	cloneO, ok := cloneStorage.(*OneDriveStorage)
	if !ok {
		return errors.New("clone is not OneDriveStorage")
	}
	if cloneO.RootFolderId == "" {
		logs.WithContext(ctx).Info("OneDrive storage has no root_folder_id; nothing to delete")
		return nil
	}
	if forceDelete {
		if err := cloneStorage.EmptyBucket(ctx, projectId); err != nil {
			return err
		}
	}
	delUrl := fmt.Sprintf("%s/items/%s", graphDriveBase, cloneO.RootFolderId)
	_, _, status, err := cloneO.graphCall(ctx, projectId, http.MethodDelete, delUrl, http.Header{}, nil, nil)
	if err != nil && status != http.StatusNotFound {
		return err
	}
	if status >= 300 && status != http.StatusNotFound && status != http.StatusNoContent {
		return fmt.Errorf("graph delete folder returned %d", status)
	}
	return nil
}

func (o *OneDriveStorage) EmptyBucket(ctx context.Context, projectId string) error {
	if o.RootFolderId == "" {
		return errors.New("EmptyBucket refused: root_folder_id is not set (refusing to empty entire OneDrive)")
	}
	listUrl := fmt.Sprintf("%s/items/%s/children", graphDriveBase, o.RootFolderId)
	for listUrl != "" {
		res, _, status, err := o.graphCall(ctx, projectId, http.MethodGet, listUrl, http.Header{}, map[string]string{"$select": "id,name", "$top": "200"}, nil)
		if err != nil {
			return err
		}
		if status >= 300 {
			return fmt.Errorf("graph list returned %d", status)
		}
		m, _ := res.(map[string]interface{})
		filesArr, _ := m["value"].([]interface{})
		for _, f := range filesArr {
			fm, _ := f.(map[string]interface{})
			id, _ := fm["id"].(string)
			if id == "" {
				continue
			}
			delUrl := fmt.Sprintf("%s/items/%s", graphDriveBase, id)
			_, _, dStatus, dErr := o.graphCall(ctx, projectId, http.MethodDelete, delUrl, http.Header{}, nil, nil)
			if dErr != nil && dStatus != http.StatusNotFound {
				return dErr
			}
			if dStatus >= 300 && dStatus != http.StatusNotFound && dStatus != http.StatusNoContent {
				return fmt.Errorf("graph delete item returned %d", dStatus)
			}
		}
		nextLink, _ := m["@odata.nextLink"].(string)
		listUrl = nextLink
	}
	return nil
}
