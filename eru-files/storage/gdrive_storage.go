package storage

import (
	"bytes"
	"context"
	"crypto/md5"
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
	gdriveApiBase         = "https://www.googleapis.com/drive/v3"
	gdriveUploadBase      = "https://www.googleapis.com/upload/drive/v3"
	gdriveFolderMime      = "application/vnd.google-apps.folder"
	gdriveDocsBase        = "https://docs.google.com"
	gdriveSheetsBase      = "https://sheets.googleapis.com/v4/spreadsheets"
	gdriveNativeSheetMime = "application/vnd.google-apps.spreadsheet"
)

type GdriveStorage struct {
	Storage
	AuthName     string `json:"auth_name" eru:"required"`
	RootFolderId string `json:"root_folder_id"`
}

func (g *GdriveStorage) GetAttribute(attributeName string) (interface{}, error) {
	switch attributeName {
	case "storage_name":
		return g.StorageName, nil
	case "storage_type":
		return g.StorageType, nil
	case "key_pair":
		return g.KeyPair, nil
	case "key_id":
		return g.KmsId, nil
	case "auth_name":
		return g.AuthName, nil
	case "root_folder_id":
		return g.RootFolderId, nil
	default:
		return nil, errors.New("Attribute not found")
	}
}

func (g *GdriveStorage) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	if err := json.Unmarshal(*rj, &g); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if g.EncryptFiles {
		logs.WithContext(ctx).Warn("encrypt_files is not supported for GDRIVE storage; flag will be ignored")
	}
	return nil
}

func (g *GdriveStorage) Init(ctx context.Context) error {
	return nil
}

func (g *GdriveStorage) getAccessToken(ctx context.Context, projectId string) (string, error) {
	if g.AuthName == "" {
		return "", errors.New("auth_name is required for GDRIVE storage")
	}
	if projectId == "" {
		return "", errors.New("projectId is required for GDRIVE getAccessToken")
	}
	baseUrl, _ := ctx.Value("eruauthbaseurl").(string)
	if baseUrl == "" {
		return "", errors.New("eruauthbaseurl not found in context")
	}
	url := fmt.Sprintf("%s/%s/%s/gettoken", strings.TrimRight(baseUrl, "/"), projectId, g.AuthName)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	qParams := map[string]string{}
	if prefix, _ := ctx.Value("tokenkeyprefix").(string); prefix != "" {
		qParams["token_key_prefix"] = prefix
	}
	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, nil, qParams, nil)
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

func (g *GdriveStorage) driveCall(ctx context.Context, projectId, method, fullUrl string, headers http.Header, params map[string]string, postBody interface{}) (interface{}, http.Header, int, error) {
	tok, err := g.getAccessToken(ctx, projectId)
	if err != nil {
		return nil, nil, 0, err
	}
	if headers == nil {
		headers = http.Header{}
	}
	headers.Set("Authorization", "Bearer "+tok)
	headers.Set("Content-Type", "application/json")
	res, respHeaders, _, status, err := utils.CallHttp(ctx, method, fullUrl, headers, map[string]string{}, nil, params, postBody)
	return res, respHeaders, status, err
}

func (g *GdriveStorage) parentFolderId() string {
	if g.RootFolderId != "" {
		return g.RootFolderId
	}
	return "root"
}

func escapeDriveQ(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

func (g *GdriveStorage) findChild(ctx context.Context, projectId, parentId, name string, folderOnly bool) (string, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", escapeDriveQ(name), escapeDriveQ(parentId))
	if folderOnly {
		q += " and mimeType = '" + gdriveFolderMime + "'"
	} else {
		q += " and mimeType != '" + gdriveFolderMime + "'"
	}
	params := map[string]string{
		"q":                         q,
		"fields":                    "files(id,name,mimeType)",
		"pageSize":                  "1",
		"spaces":                    "drive",
		"includeItemsFromAllDrives": "true",
		"supportsAllDrives":         "true",
	}
	res, _, status, err := g.driveCall(ctx, projectId, http.MethodGet, gdriveApiBase+"/files", http.Header{}, params, nil)
	if err != nil {
		return "", err
	}
	if status >= 300 {
		return "", fmt.Errorf("drive list returned status %d", status)
	}
	resMap, ok := res.(map[string]interface{})
	if !ok {
		return "", errors.New("drive list response not an object")
	}
	filesArr, _ := resMap["files"].([]interface{})
	if len(filesArr) == 0 {
		return "", nil
	}
	first, ok := filesArr[0].(map[string]interface{})
	if !ok {
		return "", nil
	}
	id, _ := first["id"].(string)
	return id, nil
}

func (g *GdriveStorage) ensureFolder(ctx context.Context, projectId, parentId, name string) (string, error) {
	id, err := g.findChild(ctx, projectId, parentId, name, true)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	body := map[string]interface{}{
		"name":     name,
		"mimeType": gdriveFolderMime,
		"parents":  []string{parentId},
	}
	res, _, status, err := g.driveCall(ctx, projectId, http.MethodPost, gdriveApiBase+"/files", headers, map[string]string{"supportsAllDrives": "true"}, body)
	if err != nil {
		return "", err
	}
	if status >= 300 {
		return "", fmt.Errorf("drive create folder returned %d", status)
	}
	resMap, _ := res.(map[string]interface{})
	id, _ = resMap["id"].(string)
	if id == "" {
		return "", errors.New("drive create folder missing id")
	}
	return id, nil
}

func (g *GdriveStorage) resolveFolderPath(ctx context.Context, projectId, folderPath string, createIfMissing bool) (string, error) {
	parent := g.parentFolderId()
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
			id, err = g.ensureFolder(ctx, projectId, parent, part)
		} else {
			id, err = g.findChild(ctx, projectId, parent, part, true)
			if err == nil && id == "" {
				err = fmt.Errorf("folder not found: %s", part)
			}
		}
		if err != nil {
			return "", err
		}
		parent = id
	}
	return parent, nil
}

func buildGdriveFileName(docType, baseName string) string {
	id := ksuid.New().String()
	if docType != "" {
		return fmt.Sprint(docType, "_", id, "_", baseName)
	}
	return fmt.Sprint(id, "_", baseName)
}

func joinDocId(folderPath, fileName string) string {
	folderPath = strings.Trim(folderPath, "/")
	if folderPath == "" {
		return fileName
	}
	return folderPath + "/" + fileName
}

func (g *GdriveStorage) uploadBytes(ctx context.Context, projectId, parentId, fileName string, data []byte, contentType string) (string, error) {
	tok, err := g.getAccessToken(ctx, projectId)
	if err != nil {
		return "", err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	meta := map[string]interface{}{
		"name":    fileName,
		"parents": []string{parentId},
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	boundary := "eru-gdrive-" + ksuid.New().String()
	var body bytes.Buffer
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Type: application/json; charset=UTF-8\r\n\r\n")
	body.Write(metaBytes)
	body.WriteString("\r\n--" + boundary + "\r\n")
	body.WriteString("Content-Type: " + contentType + "\r\n\r\n")
	body.Write(data)
	body.WriteString("\r\n--" + boundary + "--")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gdriveUploadBase+"/files?uploadType=multipart&supportsAllDrives=true", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("drive upload failed: status %d body %s", resp.StatusCode, string(respBytes))
	}
	var respObj map[string]interface{}
	if err := json.Unmarshal(respBytes, &respObj); err != nil {
		return "", err
	}
	id, _ := respObj["id"].(string)
	return id, nil
}

func (g *GdriveStorage) UploadFile(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	logs.WithContext(ctx).Debug("UploadFile - Start (GDRIVE)")
	if g.EncryptFiles {
		logs.WithContext(ctx).Warn("encrypt_files ignored for GDRIVE")
	}
	byteContainer, err := io.ReadAll(file)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	finalFileName := buildGdriveFileName(docType, header.Filename)
	parentId, err := g.resolveFolderPath(ctx, projectId, folderPath, true)
	if err != nil {
		return "", err
	}
	if _, err = g.uploadBytes(ctx, projectId, parentId, finalFileName, byteContainer, header.Header.Get("Content-Type")); err != nil {
		return "", err
	}
	return joinDocId(folderPath, finalFileName), nil
}

func (g *GdriveStorage) UploadFileB64(ctx context.Context, projectId string, file []byte, fileName string, docType string, folderPath string, keyName eruaes.AesKey) (docId string, err error) {
	logs.WithContext(ctx).Debug("UploadFileB64 - Start (GDRIVE)")
	if g.EncryptFiles {
		logs.WithContext(ctx).Warn("encrypt_files ignored for GDRIVE")
	}
	finalFileName := buildGdriveFileName(docType, fileName)
	parentId, err := g.resolveFolderPath(ctx, projectId, folderPath, true)
	if err != nil {
		return "", err
	}
	if _, err = g.uploadBytes(ctx, projectId, parentId, finalFileName, file, ""); err != nil {
		return "", err
	}
	return joinDocId(folderPath, finalFileName), nil
}

func (g *GdriveStorage) DownloadFile(ctx context.Context, projectId string, folderPath string, fileName string, keyName eruaes.AesKey) ([]byte, error) {
	logs.WithContext(ctx).Debug("DownloadFile - Start (GDRIVE)")
	parentId, err := g.resolveFolderPath(ctx, projectId, folderPath, false)
	if err != nil {
		return nil, err
	}
	fileId, err := g.findChild(ctx, projectId, parentId, fileName, false)
	if err != nil {
		return nil, err
	}
	if fileId == "" {
		return nil, fmt.Errorf("file not found: %s", joinDocId(folderPath, fileName))
	}
	tok, err := g.getAccessToken(ctx, projectId)
	if err != nil {
		return nil, err
	}
	data, _, _, _, err := g.downloadByDriveId(ctx, tok, fileId, "")
	return data, err
}

type GdriveSearchFilters struct {
	FileName      string
	SharedWithMe  bool
	OwnerEmail    string
	ModifiedAfter string
	MimeType      string
	MaxResults    int
}

func gdriveBuildSearchQuery(f GdriveSearchFilters) string {
	parts := []string{"trashed = false"}
	if f.FileName != "" {
		parts = append(parts, fmt.Sprintf("name = '%s'", escapeDriveQ(f.FileName)))
	}
	if f.SharedWithMe {
		parts = append(parts, "sharedWithMe = true")
	}
	if f.OwnerEmail != "" {
		parts = append(parts, fmt.Sprintf("'%s' in owners", escapeDriveQ(f.OwnerEmail)))
	}
	if f.ModifiedAfter != "" {
		parts = append(parts, fmt.Sprintf("modifiedTime > '%s'", escapeDriveQ(f.ModifiedAfter)))
	}
	if f.MimeType != "" {
		parts = append(parts, fmt.Sprintf("mimeType = '%s'", escapeDriveQ(f.MimeType)))
	}
	return strings.Join(parts, " and ")
}

func (g *GdriveStorage) SearchFiles(ctx context.Context, projectId string, f GdriveSearchFilters) ([]map[string]interface{}, error) {
	pageSize := f.MaxResults
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	params := map[string]string{
		"q":                         gdriveBuildSearchQuery(f),
		"fields":                    "files(id,name,mimeType,owners(emailAddress,displayName),parents,sharedWithMeTime,modifiedTime,size)",
		"pageSize":                  fmt.Sprint(pageSize),
		"spaces":                    "drive",
		"includeItemsFromAllDrives": "true",
		"supportsAllDrives":         "true",
	}
	res, _, status, err := g.driveCall(ctx, projectId, http.MethodGet, gdriveApiBase+"/files", http.Header{}, params, nil)
	if err != nil {
		return nil, fmt.Errorf("drive search failed (status %d): %w", status, err)
	}
	rm, ok := res.(map[string]interface{})
	if !ok {
		return nil, errors.New("drive search response not an object")
	}
	filesArr, _ := rm["files"].([]interface{})
	out := make([]map[string]interface{}, 0, len(filesArr))
	for _, fi := range filesArr {
		if fm, ok := fi.(map[string]interface{}); ok {
			out = append(out, fm)
		}
	}
	return out, nil
}

func gdriveDefaultExportMime(googleMime string) string {
	switch googleMime {
	case "application/vnd.google-apps.document":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "application/vnd.google-apps.spreadsheet":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "application/vnd.google-apps.presentation":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case "application/vnd.google-apps.drawing":
		return "application/pdf"
	case "application/vnd.google-apps.script":
		return "application/vnd.google-apps.script+json"
	default:
		return "application/pdf"
	}
}

func gdriveResolveExportMime(googleMime, requested string) string {
	if requested == "" {
		return gdriveDefaultExportMime(googleMime)
	}
	switch strings.ToLower(requested) {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case "odt":
		return "application/vnd.oasis.opendocument.text"
	case "ods":
		return "application/vnd.oasis.opendocument.spreadsheet"
	case "csv":
		return "text/csv"
	case "txt":
		return "text/plain"
	case "html":
		return "text/html"
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "svg":
		return "image/svg+xml"
	default:
		return requested
	}
}

func gdriveDocsExportUrl(fileId, srcMime string) (string, string) {
	switch srcMime {
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return fmt.Sprintf("%s/spreadsheets/d/%s/export?format=xlsx", gdriveDocsBase, fileId), srcMime
	case "application/vnd.ms-excel":
		return fmt.Sprintf("%s/spreadsheets/d/%s/export?format=xlsx", gdriveDocsBase, fileId), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return fmt.Sprintf("%s/document/d/%s/export?format=docx", gdriveDocsBase, fileId), srcMime
	case "application/msword":
		return fmt.Sprintf("%s/document/d/%s/export?format=docx", gdriveDocsBase, fileId), "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return fmt.Sprintf("%s/presentation/d/%s/export/pptx", gdriveDocsBase, fileId), srcMime
	case "application/vnd.ms-powerpoint":
		return fmt.Sprintf("%s/presentation/d/%s/export/pptx", gdriveDocsBase, fileId), "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		return "", ""
	}
}

func gdriveFetch(ctx context.Context, accessToken, dlUrl string) ([]byte, error) {
	req, rErr := http.NewRequestWithContext(ctx, http.MethodGet, dlUrl, nil)
	if rErr != nil {
		return nil, rErr
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Cache-Control", "no-cache, no-store, max-age=0")
	req.Header.Set("Pragma", "no-cache")
	resp, rErr := http.DefaultClient.Do(req)
	if rErr != nil {
		return nil, rErr
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("drive download failed: status %d body %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (g *GdriveStorage) latestRevision(ctx context.Context, accessToken, fileId string) (revId string, revModifiedTime string, exportLinks map[string]interface{}) {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-cache, no-store, max-age=0")
	h.Set("Pragma", "no-cache")
	params := map[string]string{
		"fields":   "revisions(id,modifiedTime,mimeType,exportLinks)",
		"pageSize": "1000",
	}
	res, _, _, status, err := utils.CallHttp(ctx, http.MethodGet, fmt.Sprintf("%s/files/%s/revisions", gdriveApiBase, fileId), h, map[string]string{}, nil, params, nil)
	if err != nil {
		logs.WithContext(ctx).Warn(fmt.Sprintf("drive revisions list failed for %s (status %d): %s", fileId, status, err.Error()))
		return "", "", nil
	}
	rm, ok := res.(map[string]interface{})
	if !ok {
		return "", "", nil
	}
	revs, _ := rm["revisions"].([]interface{})
	if len(revs) == 0 {
		return "", "", nil
	}
	last, _ := revs[len(revs)-1].(map[string]interface{})
	if last == nil {
		return "", "", nil
	}
	revId, _ = last["id"].(string)
	revModifiedTime, _ = last["modifiedTime"].(string)
	exportLinks, _ = last["exportLinks"].(map[string]interface{})
	return revId, revModifiedTime, exportLinks
}

func (g *GdriveStorage) downloadByDriveId(ctx context.Context, accessToken, fileId, exportMime string) (data []byte, mime string, name string, meta map[string]interface{}, err error) {
	metaParams := map[string]string{
		"fields":            "id,name,mimeType,size,version,modifiedTime,headRevisionId,md5Checksum",
		"supportsAllDrives": "true",
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-cache, no-store, max-age=0")
	h.Set("Pragma", "no-cache")
	metaRes, _, _, mStatus, mErr := utils.CallHttp(ctx, http.MethodGet, fmt.Sprintf("%s/files/%s", gdriveApiBase, fileId), h, map[string]string{}, nil, metaParams, nil)
	if mErr != nil {
		return nil, "", "", nil, fmt.Errorf("drive metadata failed (status %d): %w", mStatus, mErr)
	}
	srcMime := ""
	version := ""
	modifiedTime := ""
	revisionId := ""
	blobMd5 := ""
	if mm, ok := metaRes.(map[string]interface{}); ok {
		name, _ = mm["name"].(string)
		srcMime, _ = mm["mimeType"].(string)
		version = fmt.Sprint(mm["version"])
		modifiedTime, _ = mm["modifiedTime"].(string)
		revisionId, _ = mm["headRevisionId"].(string)
		blobMd5, _ = mm["md5Checksum"].(string)
	}
	meta = map[string]interface{}{
		"version":       version,
		"revision_id":   revisionId,
		"modified_time": modifiedTime,
	}
	latestRevId, latestRevTime, latestRevExportLinks := g.latestRevision(ctx, accessToken, fileId)
	if latestRevId != "" {
		meta["latest_revision_id"] = latestRevId
		meta["latest_revision_time"] = latestRevTime
	}
	cacheBuster := url.QueryEscape(strings.Join([]string{version, revisionId, latestRevId, modifiedTime, latestRevTime}, "_"))
	logs.WithContext(ctx).Info(fmt.Sprintf("drive download %s : version %s headRevision %s modifiedTime %s latestRevision %s latestRevisionTime %s", fileId, version, revisionId, modifiedTime, latestRevId, latestRevTime))

	var dlUrl string
	if strings.HasPrefix(srcMime, "application/vnd.google-apps.") {
		targetMime := gdriveResolveExportMime(srcMime, exportMime)
		mime = targetMime
		dlUrl = fmt.Sprintf("%s/files/%s/export?mimeType=%s&supportsAllDrives=true&eruv=%s", gdriveApiBase, fileId, url.QueryEscape(targetMime), cacheBuster)
		if latestRevId != "" && latestRevTime > modifiedTime {
			if link, lOk := latestRevExportLinks[targetMime].(string); lOk && link != "" {
				dlUrl = link
				meta["downloaded_from"] = "latest_revision_export_link"
			}
		}
	} else {
		mime = srcMime
		dlUrl = fmt.Sprintf("%s/files/%s?alt=media&supportsAllDrives=true&eruv=%s", gdriveApiBase, fileId, cacheBuster)
		if latestRevId != "" && (latestRevTime > modifiedTime || latestRevId != revisionId) {
			dlUrl = fmt.Sprintf("%s/files/%s/revisions/%s?alt=media&eruv=%s", gdriveApiBase, fileId, url.PathEscape(latestRevId), cacheBuster)
			meta["downloaded_from"] = "latest_revision"
		} else if latestRevTime != "" && modifiedTime > latestRevTime {
			docsUrl, docsMime := gdriveDocsExportUrl(fileId, srcMime)
			if docsUrl != "" {
				logs.WithContext(ctx).Info(fmt.Sprintf("drive download %s : file modified %s after last revision %s - fetching live editor export", fileId, modifiedTime, latestRevTime))
				docsData, docsErr := gdriveFetch(ctx, accessToken, docsUrl)
				if docsErr != nil {
					logs.WithContext(ctx).Warn(fmt.Sprintf("drive live editor export failed for %s, falling back to stored blob : %s", fileId, docsErr.Error()))
				} else {
					meta["downloaded_from"] = "live_editor_export"
					meta["blob_md5"] = blobMd5
					meta["downloaded_md5"] = fmt.Sprintf("%x", md5.Sum(docsData))
					return docsData, docsMime, name, meta, nil
				}
			}
		}
	}
	if _, dOk := meta["downloaded_from"]; !dOk {
		meta["downloaded_from"] = "head"
	}

	body, dErr := gdriveFetch(ctx, accessToken, dlUrl)
	if dErr != nil {
		return nil, "", "", nil, dErr
	}
	if blobMd5 != "" {
		meta["blob_md5"] = blobMd5
		meta["downloaded_md5"] = fmt.Sprintf("%x", md5.Sum(body))
	}
	return body, mime, name, meta, nil
}

func (g *GdriveStorage) DownloadById(ctx context.Context, projectId, fileId, exportMime string) (data []byte, mime string, name string, meta map[string]interface{}, err error) {
	tok, err := g.getAccessToken(ctx, projectId)
	if err != nil {
		return nil, "", "", nil, err
	}
	return g.downloadByDriveId(ctx, tok, fileId, exportMime)
}

func (g *GdriveStorage) BucketExists(ctx context.Context) (bool, error) {
	return g.RootFolderId != "", nil
}

func (g *GdriveStorage) CreateStorage(ctx context.Context, projectId string, cloneStorage StorageI, persist bool) error {
	if !persist {
		return nil
	}
	exists, err := cloneStorage.BucketExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		logs.WithContext(ctx).Info("skipping GDrive folder creation as root_folder_id is already set")
		return nil
	}
	nameI, _ := cloneStorage.GetAttribute("storage_name")
	name, _ := nameI.(string)
	if name == "" {
		return errors.New("storage_name required to create GDrive folder")
	}
	id, err := g.ensureFolder(ctx, projectId, "root", name)
	if err != nil {
		return err
	}
	g.RootFolderId = id
	if cg, ok := cloneStorage.(*GdriveStorage); ok {
		cg.RootFolderId = id
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("created GDrive root folder: %s (id=%s)", name, id))
	return nil
}

func (g *GdriveStorage) DeleteStorage(ctx context.Context, projectId string, forceDelete bool, cloneStorage StorageI) error {
	cloneG, ok := cloneStorage.(*GdriveStorage)
	if !ok {
		return errors.New("clone is not GdriveStorage")
	}
	if cloneG.RootFolderId == "" {
		logs.WithContext(ctx).Info("GDrive storage has no root_folder_id; nothing to delete")
		return nil
	}
	if forceDelete {
		if err := cloneStorage.EmptyBucket(ctx, projectId); err != nil {
			return err
		}
	}
	_, _, status, err := cloneG.driveCall(ctx, projectId, http.MethodDelete, fmt.Sprintf("%s/files/%s", gdriveApiBase, cloneG.RootFolderId), http.Header{}, map[string]string{"supportsAllDrives": "true"}, nil)
	if err != nil && status != http.StatusNotFound {
		return err
	}
	if status >= 300 && status != http.StatusNotFound {
		return fmt.Errorf("drive delete folder returned %d", status)
	}
	return nil
}

func (g *GdriveStorage) EmptyBucket(ctx context.Context, projectId string) error {
	if g.RootFolderId == "" {
		return errors.New("EmptyBucket refused: root_folder_id is not set (refusing to empty entire Drive)")
	}
	parent := g.RootFolderId
	pageToken := ""
	for {
		params := map[string]string{
			"q":                         fmt.Sprintf("'%s' in parents and trashed = false", escapeDriveQ(parent)),
			"fields":                    "nextPageToken, files(id,name)",
			"pageSize":                  "100",
			"spaces":                    "drive",
			"includeItemsFromAllDrives": "true",
			"supportsAllDrives":         "true",
		}
		if pageToken != "" {
			params["pageToken"] = pageToken
		}
		res, _, status, err := g.driveCall(ctx, projectId, http.MethodGet, gdriveApiBase+"/files", http.Header{}, params, nil)
		if err != nil {
			return err
		}
		if status >= 300 {
			return fmt.Errorf("drive list returned %d", status)
		}
		m, _ := res.(map[string]interface{})
		filesArr, _ := m["files"].([]interface{})
		for _, f := range filesArr {
			fm, _ := f.(map[string]interface{})
			id, _ := fm["id"].(string)
			if id == "" {
				continue
			}
			_, _, dStatus, dErr := g.driveCall(ctx, projectId, http.MethodDelete, fmt.Sprintf("%s/files/%s", gdriveApiBase, id), http.Header{}, map[string]string{"supportsAllDrives": "true"}, nil)
			if dErr != nil && dStatus != http.StatusNotFound {
				return dErr
			}
			if dStatus >= 300 && dStatus != http.StatusNotFound {
				return fmt.Errorf("drive delete file returned %d", dStatus)
			}
		}
		nt, _ := m["nextPageToken"].(string)
		if nt == "" {
			return nil
		}
		pageToken = nt
	}
}

func (g *GdriveStorage) StartPageToken(ctx context.Context, projectId string) (string, error) {
	tok, err := g.getAccessToken(ctx, projectId)
	if err != nil {
		return "", err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+tok)
	h.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, gdriveApiBase+"/changes/startPageToken", h, map[string]string{}, nil, map[string]string{"supportsAllDrives": "true"}, nil)
	if err != nil {
		return "", err
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		return "", errors.New("startPageToken response not an object")
	}
	out, _ := m["startPageToken"].(string)
	if out == "" {
		return "", errors.New("startPageToken response missing token")
	}
	return out, nil
}

func (g *GdriveStorage) WatchChanges(ctx context.Context, projectId, channelId, pushEndpoint string, expirationMs int64) (resourceId, startPageToken, expiration string, err error) {
	startPageToken, err = g.StartPageToken(ctx, projectId)
	if err != nil {
		return
	}
	body := map[string]interface{}{
		"id":      channelId,
		"type":    "web_hook",
		"address": pushEndpoint,
	}
	if expirationMs > 0 {
		body["expiration"] = fmt.Sprint(expirationMs)
	}
	watchUrl := fmt.Sprintf("%s/changes/watch?pageToken=%s&supportsAllDrives=true", gdriveApiBase, url.QueryEscape(startPageToken))
	res, _, status, cErr := g.driveCall(ctx, projectId, http.MethodPost, watchUrl, http.Header{}, map[string]string{}, body)
	if cErr != nil {
		err = fmt.Errorf("drive changes.watch failed (status %d): %w", status, cErr)
		return
	}
	m, _ := res.(map[string]interface{})
	resourceId, _ = m["resourceId"].(string)
	expiration, _ = m["expiration"].(string)
	return
}

func (g *GdriveStorage) WatchFile(ctx context.Context, projectId, fileId, channelId, pushEndpoint string, expirationMs int64) (resourceId, expiration string, err error) {
	if fileId == "" {
		err = errors.New("file_id is required")
		return
	}
	body := map[string]interface{}{
		"id":      channelId,
		"type":    "web_hook",
		"address": pushEndpoint,
	}
	if expirationMs > 0 {
		body["expiration"] = fmt.Sprint(expirationMs)
	}
	watchUrl := fmt.Sprintf("%s/files/%s/watch?supportsAllDrives=true", gdriveApiBase, fileId)
	res, _, status, cErr := g.driveCall(ctx, projectId, http.MethodPost, watchUrl, http.Header{}, map[string]string{}, body)
	if cErr != nil {
		err = fmt.Errorf("drive files.watch failed (status %d): %w", status, cErr)
		return
	}
	m, _ := res.(map[string]interface{})
	resourceId, _ = m["resourceId"].(string)
	expiration, _ = m["expiration"].(string)
	return
}

func (g *GdriveStorage) StopWatch(ctx context.Context, projectId, channelId, resourceId string) error {
	if channelId == "" || resourceId == "" {
		return errors.New("channel_id and resource_id are required")
	}
	body := map[string]interface{}{"id": channelId, "resourceId": resourceId}
	_, _, status, err := g.driveCall(ctx, projectId, http.MethodPost, gdriveApiBase+"/channels/stop", http.Header{}, map[string]string{}, body)
	if err != nil && status != http.StatusNotFound {
		return fmt.Errorf("drive channels.stop failed (status %d): %w", status, err)
	}
	return nil
}

func (g *GdriveStorage) ListChanges(ctx context.Context, projectId, pageToken string) (changes []map[string]interface{}, newStartPageToken, nextPageToken string, err error) {
	if pageToken == "" {
		err = errors.New("page_token is required")
		return
	}
	params := map[string]string{
		"pageToken":                 pageToken,
		"includeRemoved":            "true",
		"includeItemsFromAllDrives": "true",
		"supportsAllDrives":         "true",
		"fields":                    "newStartPageToken,nextPageToken,changes(fileId,removed,file(id,name,parents,mimeType,modifiedTime))",
	}
	res, _, status, cErr := g.driveCall(ctx, projectId, http.MethodGet, gdriveApiBase+"/changes", http.Header{}, params, nil)
	if cErr != nil {
		err = fmt.Errorf("drive changes list failed (status %d): %w", status, cErr)
		return
	}
	m, _ := res.(map[string]interface{})
	if arr, ok := m["changes"].([]interface{}); ok {
		changes = make([]map[string]interface{}, 0, len(arr))
		for _, c := range arr {
			if cm, ok := c.(map[string]interface{}); ok {
				changes = append(changes, cm)
			}
		}
	}
	newStartPageToken, _ = m["newStartPageToken"].(string)
	nextPageToken, _ = m["nextPageToken"].(string)
	return
}

func (g *GdriveStorage) sheetsHeaders(accessToken string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-cache, no-store, max-age=0")
	h.Set("Pragma", "no-cache")
	return h
}

func (g *GdriveStorage) sheetsMeta(ctx context.Context, accessToken, fileId string) (map[string]interface{}, error) {
	res, _, _, status, err := utils.CallHttp(ctx, http.MethodGet, fmt.Sprintf("%s/%s", gdriveSheetsBase, fileId), g.sheetsHeaders(accessToken), map[string]string{}, nil, map[string]string{
		"fields": "spreadsheetId,properties(title,timeZone),sheets(properties(sheetId,title,index,gridProperties(rowCount,columnCount)))",
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("sheets api metadata failed (status %d): %w", status, err)
	}
	rm, _ := res.(map[string]interface{})
	return rm, nil
}

func gdriveIsOfficeFileError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "must not be an Office file")
}

func (g *GdriveStorage) convertToSheet(ctx context.Context, accessToken, fileId, copyName string) (string, error) {
	if copyName == "" {
		copyName = "eru-tmp-sheet-" + ksuid.New().String()
	}
	body := map[string]interface{}{
		"name":     copyName,
		"mimeType": gdriveNativeSheetMime,
	}
	res, _, _, status, err := utils.CallHttp(ctx, http.MethodPost, fmt.Sprintf("%s/files/%s/copy", gdriveApiBase, fileId), g.sheetsHeaders(accessToken), map[string]string{}, nil, map[string]string{
		"supportsAllDrives": "true",
		"fields":            "id,mimeType",
	}, body)
	if err != nil {
		return "", fmt.Errorf("drive convert copy failed (status %d): %w", status, err)
	}
	rm, _ := res.(map[string]interface{})
	copyId, _ := rm["id"].(string)
	if copyId == "" {
		return "", errors.New("drive convert copy returned no file id")
	}
	return copyId, nil
}

func (g *GdriveStorage) deleteFile(ctx context.Context, accessToken, fileId string) {
	_, _, _, status, err := utils.CallHttp(ctx, http.MethodDelete, fmt.Sprintf("%s/files/%s", gdriveApiBase, fileId), g.sheetsHeaders(accessToken), map[string]string{}, nil, map[string]string{"supportsAllDrives": "true"}, nil)
	if err != nil {
		logs.WithContext(ctx).Warn(fmt.Sprintf("failed to delete temp drive file %s (status %d): %s", fileId, status, err.Error()))
	}
}

func (g *GdriveStorage) ReadSheetValues(ctx context.Context, projectId, fileId string, ranges []string, convertIfOffice bool) (map[string]interface{}, error) {
	accessToken, err := g.getAccessToken(ctx, projectId)
	if err != nil {
		return nil, err
	}
	readId := fileId
	readSource := "native_sheet"
	meta, mErr := g.sheetsMeta(ctx, accessToken, fileId)
	if mErr != nil {
		if !convertIfOffice || !gdriveIsOfficeFileError(mErr) {
			return nil, mErr
		}
		logs.WithContext(ctx).Info(fmt.Sprintf("file %s is an Office file - converting a temp copy to a native sheet to read live values", fileId))
		copyId, cErr := g.convertToSheet(ctx, accessToken, fileId, "")
		if cErr != nil {
			return nil, fmt.Errorf("%s ; conversion fallback failed: %w", mErr.Error(), cErr)
		}
		defer g.deleteFile(ctx, accessToken, copyId)
		readId = copyId
		readSource = "converted_copy"
		meta, mErr = g.sheetsMeta(ctx, accessToken, copyId)
		if mErr != nil {
			return nil, mErr
		}
	}
	if len(ranges) == 0 {
		sheets, _ := meta["sheets"].([]interface{})
		for _, sh := range sheets {
			shm, _ := sh.(map[string]interface{})
			if shm == nil {
				continue
			}
			props, _ := shm["properties"].(map[string]interface{})
			if props == nil {
				continue
			}
			if title, ok := props["title"].(string); ok && title != "" {
				ranges = append(ranges, title)
			}
		}
	}
	if len(ranges) == 0 {
		return nil, errors.New("no sheets found to read")
	}
	params := map[string]string{
		"valueRenderOption":    "UNFORMATTED_VALUE",
		"dateTimeRenderOption": "FORMATTED_STRING",
		"majorDimension":       "ROWS",
	}
	qs := ""
	for _, r := range ranges {
		if qs == "" {
			qs = "?ranges=" + url.QueryEscape(r)
		} else {
			qs = qs + "&ranges=" + url.QueryEscape(r)
		}
	}
	valUrl := fmt.Sprintf("%s/%s/values:batchGet%s", gdriveSheetsBase, readId, qs)
	res, _, _, status, vErr := utils.CallHttp(ctx, http.MethodGet, valUrl, g.sheetsHeaders(accessToken), map[string]string{}, nil, params, nil)
	if vErr != nil {
		return nil, fmt.Errorf("sheets api values.batchGet failed (status %d): %w", status, vErr)
	}
	rm, _ := res.(map[string]interface{})
	out := map[string]interface{}{
		"spreadsheet":  meta,
		"ranges":       ranges,
		"read_source":  readSource,
		"read_file_id": readId,
	}
	if rm != nil {
		out["value_ranges"] = rm["valueRanges"]
	}
	return out, nil
}

func (g *GdriveStorage) CreateSheetMirror(ctx context.Context, projectId, fileId, copyName string) (map[string]interface{}, error) {
	accessToken, err := g.getAccessToken(ctx, projectId)
	if err != nil {
		return nil, err
	}
	copyId, cErr := g.convertToSheet(ctx, accessToken, fileId, copyName)
	if cErr != nil {
		return nil, cErr
	}
	out := map[string]interface{}{
		"source_file_id": fileId,
		"mirror_file_id": copyId,
		"mirror_url":     fmt.Sprintf("%s/spreadsheets/d/%s/edit", gdriveDocsBase, copyId),
		"next_step":      "open mirror_url once in a desktop browser as the storage account and click Allow access on the IMPORTRANGE prompt; after that read_sheet_values on mirror_file_id returns live data",
	}
	if sm, smErr := g.sheetsMeta(ctx, accessToken, copyId); smErr == nil {
		out["spreadsheet"] = sm
	}
	return out, nil
}

func (g *GdriveStorage) InspectFile(ctx context.Context, projectId, fileId string) (map[string]interface{}, error) {
	tok, err := g.getAccessToken(ctx, projectId)
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+tok)
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-cache, no-store, max-age=0")
	h.Set("Pragma", "no-cache")

	fileFields := "id,name,mimeType,size,md5Checksum,version,createdTime,modifiedTime,modifiedByMeTime,viewedByMeTime,headRevisionId,trashed,driveId,parents,owners(emailAddress,displayName),lastModifyingUser(emailAddress,displayName),sharingUser(emailAddress,displayName),shortcutDetails,capabilities(canEdit,canDownload),webViewLink"
	fileRes, _, _, fStatus, fErr := utils.CallHttp(ctx, http.MethodGet, fmt.Sprintf("%s/files/%s", gdriveApiBase, fileId), h, map[string]string{}, nil, map[string]string{
		"fields":            fileFields,
		"supportsAllDrives": "true",
	}, nil)
	if fErr != nil {
		return nil, fmt.Errorf("drive metadata failed (status %d): %w", fStatus, fErr)
	}
	fileMeta, _ := fileRes.(map[string]interface{})

	out := map[string]interface{}{"file": fileMeta}

	revRes, _, _, rStatus, rErr := utils.CallHttp(ctx, http.MethodGet, fmt.Sprintf("%s/files/%s/revisions", gdriveApiBase, fileId), h, map[string]string{}, nil, map[string]string{
		"fields":   "revisions(id,modifiedTime,mimeType,size,lastModifyingUser(emailAddress,displayName))",
		"pageSize": "1000",
	}, nil)
	if rErr != nil {
		out["revisions_error"] = fmt.Sprintf("status %d : %s", rStatus, rErr.Error())
	} else if rm, ok := revRes.(map[string]interface{}); ok {
		out["revisions"] = rm["revisions"]
	}

	if sm, smErr := g.sheetsMeta(ctx, tok, fileId); smErr != nil {
		out["sheets_api_error"] = smErr.Error()
	} else {
		out["sheets_api"] = sm
	}

	if fileMeta != nil {
		if name, ok := fileMeta["name"].(string); ok && name != "" {
			sameName, sErr := g.SearchFiles(ctx, projectId, GdriveSearchFilters{FileName: name, MaxResults: 50})
			if sErr != nil {
				out["same_name_error"] = sErr.Error()
			} else {
				out["same_name_files"] = sameName
			}
		}
	}
	return out, nil
}
