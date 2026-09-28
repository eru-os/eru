package module_store

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"

	eruaes "github.com/eru-os/eru/eru-crypto/aes"
	erursa "github.com/eru-os/eru/eru-crypto/rsa"
	"github.com/eru-os/eru/eru-files/file_model"
	"github.com/eru-os/eru/eru-files/storage"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_reads "github.com/eru-os/eru/eru-read-write/eru-reads"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gobwas/glob"
)

type StoreHolder struct {
	sync.RWMutex
	Store ModuleStoreI
}

type FileObj struct {
	FileType string      `json:"file_type"`
	File     interface{} `json:"file"`
}

type FileDownloadRequest struct {
	FileName        string                                       `json:"file_name" eru:"required"`
	FolderPath      string                                       `json:"folder_path" eru:"required"`
	InnerFileNames  []string                                     `json:"inner_file_names" eru:"required"`
	CsvAsJson       bool                                         `json:"csv_as_json"`
	CsvDelimited    int32                                        `json:"csv_delimited"`
	ExcelAsJson     bool                                         `json:"excel_as_json"`
	ExcelSheets     map[string]map[string]eru_reads.FileReadData `json:"excel_sheets"`
	LowerCaseHeader bool                                         `json:"lower_case_header"`
	MimeLimit       uint32                                       `json:"mime_limit"`
	FileId          string                                       `json:"file_id"`
	SharedWithMe    bool                                         `json:"shared_with_me"`
	OwnerEmail      string                                       `json:"owner_email"`
	ModifiedAfter   string                                       `json:"modified_after"`
	MimeType        string                                       `json:"mime_type"`
	ExportMimeType  string                                       `json:"export_mime_type"`
	MaxResults      int                                          `json:"max_results"`
}

const (
	MIME_TEXT = "text/plain"
	MIME_CSV  = "text/csv"
	MIME_XLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

type ModuleStoreI interface {
	store.StoreI
	SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error
	RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error
	GetProjectConfig(ctx context.Context, projectId string) (*file_model.Project, error)
	GetExtendedProjectConfig(ctx context.Context, projectId string, realStore ModuleStoreI) (file_model.ExtendedProject, error)
	GetProjectList(ctx context.Context) []map[string]interface{}
	SaveStorage(ctx context.Context, storageObj storage.StorageI, projectId string, realStore ModuleStoreI, persist bool) error
	RemoveStorage(ctx context.Context, storageName string, projectId string, cloudDelete bool, forceDelete bool, realStore ModuleStoreI) error
	GetStorageClone(ctx context.Context, projectId string, storageName string, s ModuleStoreI) (storageObjClone storage.StorageI, prj *file_model.Project, err error)
	GenerateRsaKeyPair(ctx context.Context, projectId string, keyPairName string, bits int, overwrite bool, realStore ModuleStoreI) (rsaKeyPair erursa.RsaKeyPair, err error)
	GenerateAesKey(ctx context.Context, projectId string, keyPairName string, bits int, overwrite bool, realStore ModuleStoreI) (aesKey eruaes.AesKey, err error)
	UploadFile(ctx context.Context, projectId string, storageName string, file multipart.File, header *multipart.FileHeader, docType string, fodlerPath string, s ModuleStoreI) (docId string, err error)
	UploadFileB64(ctx context.Context, projectId string, storageName string, file []byte, fileName string, docType string, fodlerPath string, s ModuleStoreI) (docId string, err error)
	UploadFileFromUrl(ctx context.Context, projectId string, storageName string, urlStr string, fileName string, docType string, fodlerPath string, fileType string, s ModuleStoreI) (docId string, err error)
	DownloadFile(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (file []byte, mimeType string, err error)
	DownloadFileSmart(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (fileB64 string, mimeType string, fileName string, fileId string, fileMeta map[string]interface{}, candidates []map[string]interface{}, err error)
	GdriveWatchChanges(ctx context.Context, projectId string, storageName string, channelId string, pushEndpoint string, expirationMs int64, s ModuleStoreI) (resourceId string, startPageToken string, expiration string, err error)
	GdriveInspectFile(ctx context.Context, projectId string, storageName string, fileId string, s ModuleStoreI) (map[string]interface{}, error)
	GdriveCreateSheetMirror(ctx context.Context, projectId string, storageName string, fileId string, copyName string, s ModuleStoreI) (map[string]interface{}, error)
	GdriveReadSheetValues(ctx context.Context, projectId string, storageName string, fileId string, ranges []string, convertIfOffice bool, s ModuleStoreI) (map[string]interface{}, error)
	GdriveWatchFile(ctx context.Context, projectId string, storageName string, fileId string, channelId string, pushEndpoint string, expirationMs int64, s ModuleStoreI) (resourceId string, expiration string, err error)
	GdriveStopWatch(ctx context.Context, projectId string, storageName string, channelId string, resourceId string, s ModuleStoreI) error
	GdriveListChanges(ctx context.Context, projectId string, storageName string, pageToken string, s ModuleStoreI) (changes []map[string]interface{}, newStartPageToken string, nextPageToken string, err error)
	DownloadFileAsJson(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (jsonData []map[string]interface{}, err error)
	DownloadFileB64(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (fileB64 string, mimeType string, err error)
	DownloadFileUnzip(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (files map[string]FileObj, err error)
	SaveProjectSettings(ctx context.Context, projectId string, projectSettings file_model.ProjectSettings, realStore ModuleStoreI) error
	ExcelToJson(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (jsonObj interface{}, err error)
	BytesToJson(ctx context.Context, projectId string, f []byte, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (jsonObj []map[string]interface{}, err error)
}

type ModuleStore struct {
	Projects map[string]*file_model.Project `json:"projects"` //ProjectId is the key
}

type ModuleFileStore struct {
	store.FileStore
	ModuleStore
}
type ModuleDbStore struct {
	store.DbStore
	ModuleStore
}

func (ms *ModuleStore) GenerateRsaKeyPair(ctx context.Context, projectId string, keyPairName string, bits int, overwrite bool, realStore ModuleStoreI) (rsaKeyPair erursa.RsaKeyPair, err error) {
	logs.WithContext(ctx).Debug("GenerateRsaKeyPair - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	} else {
		if _, ok := prj.RsaKeyPairs[keyPairName]; ok && !overwrite {
			err = errors.New(fmt.Sprint("keyPairName ", keyPairName, " already exists"))
			logs.WithContext(ctx).Info(err.Error())
			return
		} else {
			rsaKeyPair, err = prj.GenerateRsaKeyPair(ctx, bits, keyPairName)
			err = realStore.SaveStore(ctx, projectId, "", realStore)
		}
	}
	return
}

func (ms *ModuleStore) GenerateAesKey(ctx context.Context, projectId string, keyName string, bits int, overwrite bool, realStore ModuleStoreI) (aesKey eruaes.AesKey, err error) {
	logs.WithContext(ctx).Debug("GenerateAesKey - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	} else {
		if _, ok := prj.AesKeys[keyName]; ok && !overwrite {
			err = errors.New(fmt.Sprint("keyname ", keyName, " already exists"))
			logs.WithContext(ctx).Info(err.Error())
			return
		} else {
			aesKey, err = prj.GenerateAesKey(ctx, bits, keyName)
			err = realStore.SaveStore(ctx, projectId, "", realStore)
		}
	}
	return
}

func (ms *ModuleStore) SaveStorage(ctx context.Context, storageObj storage.StorageI, projectId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveStorage - Start")
	ctx = context.WithValue(ctx, "eruauthbaseurl", os.Getenv("ERUAUTH_BASEURL"))
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return err
	}

	if nameI, nameErr := storageObj.GetAttribute("storage_name"); nameErr == nil {
		if name, _ := nameI.(string); name != "" {
			if existing, ok := prj.Storages[name]; ok {
				switch ns := storageObj.(type) {
				case *storage.GdriveStorage:
					if ns.RootFolderId == "" {
						if es, ok := existing.(*storage.GdriveStorage); ok && es.RootFolderId != "" {
							ns.RootFolderId = es.RootFolderId
						}
					}
				case *storage.OneDriveStorage:
					if ns.RootFolderId == "" {
						if es, ok := existing.(*storage.OneDriveStorage); ok && es.RootFolderId != "" {
							ns.RootFolderId = es.RootFolderId
						}
					}
				}
			}
		}
	}

	if persist == true {
		storageObjClone, _, err := ms.GetStorageObjClone(ctx, projectId, storageObj, realStore)
		if err != nil {
			return err
		}

		err = storageObj.CreateStorage(ctx, projectId, storageObjClone, persist)
		//  TODO to create bucket here instead of createstorage
		if err != nil {
			return err
		}
	}

	err = prj.AddStorage(ctx, storageObj)

	if persist == true {
		return realStore.SaveStore(ctx, projectId, "", realStore)
	}
	return nil
}

func (ms *ModuleStore) GetStorageClone(ctx context.Context, projectId string, storageName string, s ModuleStoreI) (storageObjClone storage.StorageI, prj *file_model.Project, err error) {
	prj, err = ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	}

	if storageObj, ok := prj.Storages[storageName]; !ok {
		err = errors.New(fmt.Sprint("storage ", storageName, " not found"))
		logs.WithContext(ctx).Error(err.Error())
		return
	} else {
		storageObjJson, storageObjJsonErr := json.Marshal(storageObj)
		if storageObjJsonErr != nil {
			err = errors.New(fmt.Sprint("error while cloning storageObj (marshal)"))
			logs.WithContext(ctx).Error(err.Error())
			logs.WithContext(ctx).Error(storageObjJsonErr.Error())
			return
		}
		storageObjJson = s.ReplaceVariables(ctx, projectId, storageObjJson, nil)

		iCloneI := reflect.New(reflect.TypeOf(storageObj))
		storageObjCloneErr := json.Unmarshal(storageObjJson, iCloneI.Interface())
		if storageObjCloneErr != nil {
			err = errors.New(fmt.Sprint("error while cloning storageObj(unmarshal)"))
			logs.WithContext(ctx).Error(err.Error())
			logs.WithContext(ctx).Error(storageObjCloneErr.Error())
			return
		}
		return iCloneI.Elem().Interface().(storage.StorageI), prj, nil
	}
}

func (ms *ModuleStore) GetStorageObjClone(ctx context.Context, projectId string, storageObj storage.StorageI, s ModuleStoreI) (storageObjClone storage.StorageI, prj *file_model.Project, err error) {
	prj, err = ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	}

	storageObjJson, storageObjJsonErr := json.Marshal(storageObj)
	if storageObjJsonErr != nil {
		err = errors.New(fmt.Sprint("error while cloning storageObj (marshal)"))
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(storageObjJsonErr.Error())
		return
	}
	storageObjJson = s.ReplaceVariables(ctx, projectId, storageObjJson, nil)

	iCloneI := reflect.New(reflect.TypeOf(storageObj))
	storageObjCloneErr := json.Unmarshal(storageObjJson, iCloneI.Interface())
	if storageObjCloneErr != nil {
		err = errors.New(fmt.Sprint("error while cloning storageObj(unmarshal)"))
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(storageObjCloneErr.Error())
		return
	}
	return iCloneI.Elem().Interface().(storage.StorageI), prj, nil

}

func (ms *ModuleStore) UploadFile(ctx context.Context, projectId string, storageName string, file multipart.File, header *multipart.FileHeader, docType string, folderPath string, s ModuleStoreI) (docId string, err error) {
	logs.WithContext(ctx).Info("UploadFile - Start")
	ctx = context.WithValue(ctx, "eruauthbaseurl", os.Getenv("ERUAUTH_BASEURL"))
	storageObjClone, prj, sErr := ms.GetStorageClone(ctx, projectId, storageName, s)
	if sErr != nil {
		return
	}
	keyName, kpErr := storageObjClone.GetAttribute("key_pair")
	if kpErr != nil {
		err = kpErr
		return
	}
	kmsName, kpErr := storageObjClone.GetAttribute("key_id")
	if kpErr != nil {
		err = kpErr
		return
	}
	kmsMap, kmsErr := s.FetchKms(ctx, projectId)
	if kmsErr != nil {
		logs.WithContext(ctx).Error(kmsErr.Error())
		//err = kmsErr
		//return
	} else {
		storageObjClone.SetKms(ctx, kmsMap[kmsName.(string)])
	}
	docId, err = storageObjClone.UploadFile(ctx, projectId, file, header, docType, folderPath, prj.AesKeys[keyName.(string)])
	return
}

func (ms *ModuleStore) UploadFileB64(ctx context.Context, projectId string, storageName string, file []byte, fileName string, docType string, folderPath string, s ModuleStoreI) (docId string, err error) {
	logs.WithContext(ctx).Debug("UploadFileB64 - Start")
	ctx = context.WithValue(ctx, "eruauthbaseurl", os.Getenv("ERUAUTH_BASEURL"))
	storageObjClone, prj, sErr := ms.GetStorageClone(ctx, projectId, storageName, s)
	if sErr != nil {
		return
	}
	keyName, kpErr := storageObjClone.GetAttribute("key_pair")
	if err != nil {
		err = kpErr
		return
	}
	kmsName, kpErr := storageObjClone.GetAttribute("key_id")
	if kpErr != nil {
		err = kpErr
		return
	}
	kmsMap, kmsErr := s.FetchKms(ctx, projectId)
	if kmsErr != nil {
		logs.WithContext(ctx).Error(kmsErr.Error())
		//err = kmsErr
		//return
	} else {
		storageObjClone.SetKms(ctx, kmsMap[kmsName.(string)])
	}
	docId, err = storageObjClone.UploadFileB64(ctx, projectId, file, fileName, docType, folderPath, prj.AesKeys[keyName.(string)])
	return
}

func (ms *ModuleStore) UploadFileFromUrl(ctx context.Context, projectId string, storageName string, urlStr string, fileName string, docType string, folderPath string, fileType string, s ModuleStoreI) (docId string, err error) {
	logs.WithContext(ctx).Debug("UploadFileFromUrl - Start")
	reqHeaders := http.Header{}
	res, respHeaders, _, _, err := utils.CallHttp(ctx, http.MethodGet, urlStr, reqHeaders, nil, nil, nil, nil)
	_ = res
	if err != nil {
		return "", err
	}
	if respHeaders.Get("Content-Type") != fileType {
		logs.WithContext(ctx).Warn("mismatch file type")
	}
	respBody := ""
	if respMap, ok := res.(map[string]interface{}); ok {
		if respBodyI, okb := respMap["body"]; okb {
			respBody = respBodyI.(string)
			return ms.UploadFileB64(ctx, projectId, storageName, []byte(respBody), fileName, docType, folderPath, s)
		} else {
			err = errors.New("response body or file attribute not found")
			logs.WithContext(ctx).Error(err.Error())
			return "", err
		}
	} else {
		err = errors.New("response is not a map")
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
}

func (ms *ModuleStore) DownloadFileB64(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (fileB64 string, mimeType string, err error) {
	logs.WithContext(ctx).Debug("DownloadFileB64 - Start")
	f, mt, e := ms.DownloadFile(ctx, projectId, storageName, fileDownloadRequest, s)
	return base64.StdEncoding.EncodeToString(f), mt, e
}

func (ms *ModuleStore) DownloadFileSmart(ctx context.Context, projectId string, storageName string, req FileDownloadRequest, s ModuleStoreI) (fileB64 string, mimeType string, fileName string, fileId string, fileMeta map[string]interface{}, candidates []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("DownloadFileSmart - Start")
	ctx = context.WithValue(ctx, "eruauthbaseurl", os.Getenv("ERUAUTH_BASEURL"))
	storageObj, _, sErr := ms.GetStorageClone(ctx, projectId, storageName, s)
	if sErr != nil {
		err = sErr
		return
	}
	gd, ok := storageObj.(*storage.GdriveStorage)
	if !ok {
		err = errors.New("smart download is only supported for GDRIVE storage")
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	if req.FileId != "" {
		var data []byte
		data, mimeType, fileName, fileMeta, err = gd.DownloadById(ctx, projectId, req.FileId, req.ExportMimeType)
		if err != nil {
			return
		}
		fileB64 = base64.StdEncoding.EncodeToString(data)
		fileId = req.FileId
		return
	}

	if req.FileName == "" {
		err = errors.New("file_name or file_id is required")
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	matches, sErr2 := gd.SearchFiles(ctx, projectId, storage.GdriveSearchFilters{
		FileName:      req.FileName,
		SharedWithMe:  req.SharedWithMe,
		OwnerEmail:    req.OwnerEmail,
		ModifiedAfter: req.ModifiedAfter,
		MimeType:      req.MimeType,
		MaxResults:    req.MaxResults,
	})
	if sErr2 != nil {
		err = sErr2
		return
	}
	if len(matches) == 0 {
		err = fmt.Errorf("no file matches name %q with given filters", req.FileName)
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if len(matches) > 1 {
		candidates = matches
		return
	}
	only := matches[0]
	fid, _ := only["id"].(string)
	data, mt, nm, fm, dErr := gd.DownloadById(ctx, projectId, fid, req.ExportMimeType)
	if dErr != nil {
		err = dErr
		return
	}
	fileB64 = base64.StdEncoding.EncodeToString(data)
	mimeType = mt
	fileName = nm
	fileId = fid
	fileMeta = fm
	return
}

func (ms *ModuleStore) gdriveFromStorage(ctx context.Context, projectId string, storageName string, s ModuleStoreI) (context.Context, *storage.GdriveStorage, error) {
	ctx = context.WithValue(ctx, "eruauthbaseurl", os.Getenv("ERUAUTH_BASEURL"))
	storageObj, _, sErr := ms.GetStorageClone(ctx, projectId, storageName, s)
	if sErr != nil {
		return ctx, nil, sErr
	}
	gd, ok := storageObj.(*storage.GdriveStorage)
	if !ok {
		return ctx, nil, errors.New("storage is not GDRIVE")
	}
	return ctx, gd, nil
}

func (ms *ModuleStore) GdriveWatchChanges(ctx context.Context, projectId string, storageName string, channelId string, pushEndpoint string, expirationMs int64, s ModuleStoreI) (resourceId string, startPageToken string, expiration string, err error) {
	ctx, gd, gErr := ms.gdriveFromStorage(ctx, projectId, storageName, s)
	if gErr != nil {
		return "", "", "", gErr
	}
	return gd.WatchChanges(ctx, projectId, channelId, pushEndpoint, expirationMs)
}

func (ms *ModuleStore) GdriveWatchFile(ctx context.Context, projectId string, storageName string, fileId string, channelId string, pushEndpoint string, expirationMs int64, s ModuleStoreI) (resourceId string, expiration string, err error) {
	ctx, gd, gErr := ms.gdriveFromStorage(ctx, projectId, storageName, s)
	if gErr != nil {
		return "", "", gErr
	}
	return gd.WatchFile(ctx, projectId, fileId, channelId, pushEndpoint, expirationMs)
}

func (ms *ModuleStore) GdriveStopWatch(ctx context.Context, projectId string, storageName string, channelId string, resourceId string, s ModuleStoreI) error {
	ctx, gd, gErr := ms.gdriveFromStorage(ctx, projectId, storageName, s)
	if gErr != nil {
		return gErr
	}
	return gd.StopWatch(ctx, projectId, channelId, resourceId)
}

func (ms *ModuleStore) GdriveListChanges(ctx context.Context, projectId string, storageName string, pageToken string, s ModuleStoreI) (changes []map[string]interface{}, newStartPageToken string, nextPageToken string, err error) {
	ctx, gd, gErr := ms.gdriveFromStorage(ctx, projectId, storageName, s)
	if gErr != nil {
		return nil, "", "", gErr
	}
	return gd.ListChanges(ctx, projectId, pageToken)
}

func (ms *ModuleStore) GdriveInspectFile(ctx context.Context, projectId string, storageName string, fileId string, s ModuleStoreI) (map[string]interface{}, error) {
	ctx, gd, gErr := ms.gdriveFromStorage(ctx, projectId, storageName, s)
	if gErr != nil {
		return nil, gErr
	}
	return gd.InspectFile(ctx, projectId, fileId)
}

func (ms *ModuleStore) GdriveCreateSheetMirror(ctx context.Context, projectId string, storageName string, fileId string, copyName string, s ModuleStoreI) (map[string]interface{}, error) {
	ctx, gd, gErr := ms.gdriveFromStorage(ctx, projectId, storageName, s)
	if gErr != nil {
		return nil, gErr
	}
	return gd.CreateSheetMirror(ctx, projectId, fileId, copyName)
}

func (ms *ModuleStore) GdriveReadSheetValues(ctx context.Context, projectId string, storageName string, fileId string, ranges []string, convertIfOffice bool, s ModuleStoreI) (map[string]interface{}, error) {
	ctx, gd, gErr := ms.gdriveFromStorage(ctx, projectId, storageName, s)
	if gErr != nil {
		return nil, gErr
	}
	return gd.ReadSheetValues(ctx, projectId, fileId, ranges, convertIfOffice)
}

func (ms *ModuleStore) DownloadFileUnzip(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (files map[string]FileObj, err error) {
	logs.WithContext(ctx).Debug("DownloadFileUnzip - Start")
	f, _, e := ms.DownloadFile(ctx, projectId, storageName, fileDownloadRequest, s)
	zipReader, err := zip.NewReader(bytes.NewReader(f), int64(len(f)))
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	// Read all the files from zip archive
	fo := FileObj{}
	files = make(map[string]FileObj)
	for _, zipFile := range zipReader.File {
		fileToUnzip := false
		for _, ifn := range fileDownloadRequest.InnerFileNames {
			g := glob.MustCompile(ifn)
			if g.Match(zipFile.Name) {
				fileToUnzip = true
			}
		}
		if len(fileDownloadRequest.InnerFileNames) == 0 {
			fileToUnzip = true
		}
		if fileToUnzip {
			logs.WithContext(ctx).Info(fmt.Sprint("Reading file:", zipFile.Name))
			unzippedFileBytes, ziperr := readZipFile(ctx, zipFile)
			if ziperr != nil {
				err = ziperr
			}
			mimetype.SetLimit(2000)
			if fileDownloadRequest.MimeLimit > 0 {
				mimetype.SetLimit(fileDownloadRequest.MimeLimit)
			}
			fMime := mimetype.Detect(unzippedFileBytes)
			logs.WithContext(ctx).Info(fmt.Sprint("fileDownloadRequest.CsvAsJson = ", fileDownloadRequest.CsvAsJson))
			logs.WithContext(ctx).Info(fmt.Sprint("fMime = ", fMime))
			logs.WithContext(ctx).Info(fmt.Sprint("fileDownloadRequest.Mime_Limit = ", fileDownloadRequest.MimeLimit))

			if fileDownloadRequest.CsvAsJson && fMime.Is(MIME_CSV) {
				jsonData, jsonErr := csvToJson(ctx, unzippedFileBytes, fileDownloadRequest)
				if jsonErr != nil {
					err = jsonErr
					return
				}
				fo.File = jsonData
			} else if fileDownloadRequest.ExcelAsJson && fMime.Is(MIME_XLSX) {
				logs.WithContext(ctx).Info("inside MIME_XLSX")
				logs.WithContext(ctx).Info(fmt.Sprint("fileDownloadRequest.ExcelAsJson = ", fileDownloadRequest.ExcelAsJson))
				if !fileDownloadRequest.ExcelAsJson {
					fo.File = base64.StdEncoding.EncodeToString(unzippedFileBytes)
				} else {
					var sheets map[string]eru_reads.FileReadData
					if fileDownloadRequest.ExcelSheets != nil {
						for fn, v := range fileDownloadRequest.ExcelSheets {
							if fn == zipFile.Name || fn == "*" {
								sheets = v
								break
							}
						}
					}
					erd := eru_reads.ExcelReadData{Sheets: sheets}
					jsonData, jsonErr := erd.ReadAsJson(ctx, unzippedFileBytes)
					if jsonErr != nil {
						err = jsonErr
						return
					}
					fo.File = jsonData
				}
			} else {
				fo.File = base64.StdEncoding.EncodeToString(unzippedFileBytes)
			}
			fo.FileType = fMime.String()
			files[zipFile.Name] = fo
		}
	}
	return files, e
}
func (ms *ModuleStore) DownloadFile(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (file []byte, mimeType string, err error) {
	logs.WithContext(ctx).Debug("DownloadFile - Start")
	ctx = context.WithValue(ctx, "eruauthbaseurl", os.Getenv("ERUAUTH_BASEURL"))
	storageObjClone, prj, sErr := ms.GetStorageClone(ctx, projectId, storageName, s)
	if sErr != nil {
		return
	}
	keyName, kpErr := storageObjClone.GetAttribute("key_pair")
	if err != nil {
		err = kpErr
		return
	}
	kmsName, kpErr := storageObjClone.GetAttribute("key_id")
	if kpErr != nil {
		err = kpErr
		return
	}
	kmsMap, kmsErr := s.FetchKms(ctx, projectId)
	if kmsErr != nil {
		logs.WithContext(ctx).Error(kmsErr.Error())
		//err = kmsErr
		//return
	} else {
		storageObjClone.SetKms(ctx, kmsMap[kmsName.(string)])
	}

	file, err = storageObjClone.DownloadFile(ctx, projectId, fileDownloadRequest.FolderPath, fileDownloadRequest.FileName, prj.AesKeys[keyName.(string)])
	mimetype.SetLimit(2000)
	return file, mimetype.Detect(file).String(), err
}

func (ms *ModuleStore) DownloadFileAsJson(ctx context.Context, projectId string, storageName string, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (jsonData []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("DownloadFileAsJson - Start")
	f, m, e := ms.DownloadFile(ctx, projectId, storageName, fileDownloadRequest, s)
	if e != nil {
		logs.WithContext(ctx).Error(e.Error())
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint(m))
	mimetype.SetLimit(2000)
	fMime := mimetype.Detect(f)
	logs.WithContext(ctx).Info(fmt.Sprint("fileDownloadRequest.ExcelAsJson = ", fileDownloadRequest.ExcelAsJson))
	logs.WithContext(ctx).Info(fmt.Sprint("fMime ", fMime))

	if fileDownloadRequest.CsvAsJson && (fMime.Is(MIME_TEXT) || fMime.Is(MIME_CSV)) {
		jsonData, err = csvToJson(ctx, f, fileDownloadRequest)
		if err != nil {
			return
		}
	} else if fileDownloadRequest.ExcelAsJson && fMime.Is(MIME_XLSX) {
		var sheets map[string]eru_reads.FileReadData
		if fileDownloadRequest.ExcelSheets != nil {
			for fn, v := range fileDownloadRequest.ExcelSheets {
				if fn == fileDownloadRequest.FileName || fn == "*" {
					sheets = v
					break
				}
			}
		}
		erd := eru_reads.ExcelReadData{Sheets: sheets}
		jsonDataObj, jsonErr := erd.ReadAsJson(ctx, f)
		if jsonErr != nil {
			err = jsonErr
			return
		}
		jsonData = append(jsonData, jsonDataObj)
	}
	return
}

func (ms *ModuleStore) SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveProject - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	//TODO to handle edit project once new project attributes are finalized
	if _, ok := ms.Projects[projectId]; !ok {
		project := new(file_model.Project)
		project.ProjectId = projectId
		if ms.Projects == nil {
			ms.Projects = make(map[string]*file_model.Project)
		}
		if project.Storages == nil {
			project.Storages = make(map[string]storage.StorageI)
		}
		if project.RsaKeyPairs == nil {
			project.RsaKeyPairs = make(map[string]erursa.RsaKeyPair)
		}
		if project.AesKeys == nil {
			project.AesKeys = make(map[string]eruaes.AesKey)
		}
		ms.Projects[projectId] = project
		if persist == true {
			logs.WithContext(ctx).Info("SaveStore called from SaveProject")
			return realStore.SaveStore(ctx, projectId, "", realStore)
		} else {
			return nil
		}
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " already exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}

func (ms *ModuleStore) RemoveStorage(ctx context.Context, storageName string, projectId string, cloudDelete bool, forceDelete bool, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveStorage - Start")
	ctx = context.WithValue(ctx, "eruauthbaseurl", os.Getenv("ERUAUTH_BASEURL"))
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if prg, ok := ms.Projects[projectId]; ok {
		if _, ok := prg.Storages[storageName]; ok {
			if cloudDelete {

				sn, snerr := prg.Storages[storageName].GetAttribute("storage_name")
				if snerr != nil {
					return snerr
				}

				storageObjClone, _, err := ms.GetStorageClone(ctx, projectId, sn.(string), realStore)
				if err != nil {
					return err
				}

				err = prg.Storages[storageName].DeleteStorage(ctx, projectId, forceDelete, storageObjClone)
				if err != nil {
					return err
				}
			}

			delete(prg.Storages, storageName)
			logs.WithContext(ctx).Info("SaveStore called from RemoveStorage")
			return realStore.SaveStore(ctx, projectId, "", realStore)
		} else {
			err := errors.New(fmt.Sprint("Storage ", storageName, " does not exists"))
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}

func (ms *ModuleStore) RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveProject - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if _, ok := ms.Projects[projectId]; ok {
		delete(ms.Projects, projectId)
		logs.WithContext(ctx).Info("SaveStore called from RemoveProject")
		return realStore.SaveStore(ctx, projectId, "", realStore)
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
}
func (ms *ModuleStore) GetExtendedProjectConfig(ctx context.Context, projectId string, realStore ModuleStoreI) (ePrj file_model.ExtendedProject, err error) {
	logs.WithContext(ctx).Debug("GetExtendedProjectConfig - Start")
	ePrj = file_model.ExtendedProject{}
	if prj, ok := ms.Projects[projectId]; ok {
		ePrj.Variables, err = realStore.FetchVars(ctx, projectId)
		ePrj.SecretManager, err = realStore.FetchSm(ctx, projectId)
		ePrj.ProjectId = prj.ProjectId
		ePrj.Storages = prj.Storages
		ePrj.ProjectSettings = prj.ProjectSettings
		ePrj.AesKeys = prj.AesKeys
		ePrj.RsaKeyPairs = prj.RsaKeyPairs
		return ePrj, nil
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		return file_model.ExtendedProject{}, err
	}
}
func (ms *ModuleStore) GetProjectConfig(ctx context.Context, projectId string) (*file_model.Project, error) {
	logs.WithContext(ctx).Debug("GetProjectConfig - Start")
	if _, ok := ms.Projects[projectId]; ok {
		return ms.Projects[projectId], nil
	} else {
		err := errors.New(fmt.Sprint("Project ", projectId, " does not exists"))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

func (ms *ModuleStore) GetProjectList(ctx context.Context) []map[string]interface{} {
	logs.WithContext(ctx).Debug("GetProjectList - Start")
	projects := make([]map[string]interface{}, len(ms.Projects))
	i := 0
	for k := range ms.Projects {
		project := make(map[string]interface{})
		project["project_name"] = k
		//project["lastUpdateDate"] = time.Now()
		projects[i] = project
		i++
	}
	return projects
}

func (ms *ModuleStore) SaveProjectSettings(ctx context.Context, projectId string, projectSettings file_model.ProjectSettings, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("SaveProjectConfig - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	ms.Projects[projectId].ProjectSettings = projectSettings
	logs.WithContext(ctx).Info("SaveStore called from SaveProjectSettings")
	return realStore.SaveStore(ctx, projectId, "", realStore)
}

func readZipFile(ctx context.Context, zipFile *zip.File) ([]byte, error) {
	logs.WithContext(ctx).Debug("readZipFile - Start")
	zf, err := zipFile.Open()
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	defer zf.Close()
	return io.ReadAll(zf)
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

func (ms *ModuleStore) ExcelToJson(ctx context.Context, projectId string, file multipart.File, header *multipart.FileHeader, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (jsonObj interface{}, err error) {
	logs.WithContext(ctx).Info("ExcelToJson - Start")

	var byteContainer []byte
	byteContainer, err = io.ReadAll(file)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return ms.BytesToJson(ctx, projectId, byteContainer, fileDownloadRequest, s)
}

func (ms *ModuleStore) BytesToJson(ctx context.Context, projectId string, f []byte, fileDownloadRequest FileDownloadRequest, s ModuleStoreI) (jsonObj []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("bytesToJson - Start")

	mimetype.SetLimit(2000)
	fMime := mimetype.Detect(f)
	logs.WithContext(ctx).Info(fmt.Sprint("fMime ", fMime))

	if fileDownloadRequest.CsvAsJson && (fMime.Is(MIME_TEXT) || fMime.Is(MIME_CSV)) {
		jsonObj, err = csvToJson(ctx, f, fileDownloadRequest)
		if err != nil {
			return
		}
	} else if fileDownloadRequest.ExcelAsJson && fMime.Is(MIME_XLSX) {
		var sheets map[string]eru_reads.FileReadData
		if fileDownloadRequest.ExcelSheets != nil {
			for fn, v := range fileDownloadRequest.ExcelSheets {
				if fn == fileDownloadRequest.FileName || fn == "*" {
					sheets = v
					break
				}
			}
		}
		erd := eru_reads.ExcelReadData{Sheets: sheets}
		jsonDataObj, jsonErr := erd.ReadAsJson(ctx, f)
		if jsonErr != nil {
			err = jsonErr
			return
		}
		jsonObj = append(jsonObj, jsonDataObj)
	}
	return
}
func LoadStore(ctx context.Context, StoreTableName string, StoreTenantTableName string) (ModuleStoreI, error) {
	logs.WithContext(ctx).Info("Loading store")
	storeType := strings.ToUpper(os.Getenv("STORE_TYPE"))
	if storeType == "" {
		storeType = "STANDALONE"
		logs.WithContext(ctx).Info("STORE_TYPE environment variable not found - loading default standlone store")
	}
	var myStore ModuleStoreI
	var err error
	switch storeType {
	case "POSTGRES":
		myStore = new(ModuleDbStore)
		myStore.SetDbType(storeType)
		myStore.SetStoreTableName(StoreTableName)
		//myStore.SetStoreTenantTableName(StoreTenantTableName)
		//myStore.CreateConn()
	case "STANDALONE":
		// myStore, err = store.LoadStoreFromFile()
		myStore = new(ModuleFileStore)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New(fmt.Sprint("Invalid STORE_TYPE ", storeType))
	}
	storeBytes, err := myStore.GetStoreByteArray("")
	if err == nil {
		UnMarshalStore(ctx, storeBytes, myStore)
	} else {
		logs.WithContext(ctx).Error(err.Error())
	}
	//s.Store = myStore
	return myStore, err
}
