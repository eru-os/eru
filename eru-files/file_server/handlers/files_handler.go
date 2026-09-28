package server

import (
	"bytes"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/eru-os/eru/eru-files/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_reads "github.com/eru-os/eru/eru-read-write/eru-reads"
	"github.com/eru-os/eru/eru-read-write/validator"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"

	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
	"github.com/tidwall/gjson"
	//"github.com/aws/aws-sdk-go/aws/session"
	//"github.com/aws/aws-sdk-go/service/s3"
)

const (
	encodedForm   = "application/x-www-form-urlencoded"
	multiPartForm = "multipart/form-data"
)

func StringToFileHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("StringToFileHandler - Start")

		strFileReq := json.NewDecoder(r.Body)
		strFileReq.DisallowUnknownFields()

		type stringToFile struct {
			Str      string `json:"str"`
			FileType string `json:"file_type"`
			FileName string `json:"file_name"`
		}

		strFileObj := stringToFile{}
		if err := strFileReq.Decode(&strFileObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", strFileObj.FileType)
		w.Header().Set("Content-Disposition", fmt.Sprint("attachment; filename=", strFileObj.FileName))
		_, _ = io.Copy(w, bytes.NewReader([]byte(strFileObj.Str)))
	}
}

func FileDownloadHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("FileDownloadHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		fileName := vars["filename"]
		folderPath := vars["folderpath"]
		//var err error

		//ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		//defer cancel()
		//_ = ctx

		dfFromObj := module_store.FileDownloadRequest{}

		if fileName != "" && folderPath != "" {
			dfFromObj.FolderPath = folderPath
			dfFromObj.FileName = fileName
		} else {
			dfFromReq := json.NewDecoder(r.Body)
			dfFromReq.DisallowUnknownFields()
			//dfFromObj := make(map[string]string)
			if err := dfFromReq.Decode(&dfFromObj); err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}
		if dfFromObj.ExcelAsJson || dfFromObj.CsvAsJson {
			file, err := sh.Store.DownloadFileAsJson(r.Context(), projectId, storageName, dfFromObj, sh.Store)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			server_handlers.FormatResponse(w, http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"file": file})
		} else {
			file, mimeType, err := sh.Store.DownloadFile(r.Context(), projectId, storageName, dfFromObj, sh.Store)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			//server_handlers.FormatResponse(w,http.StatusOK)
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", mimeType)
			w.Header().Set("Content-Disposition", fmt.Sprint("attachment; filename=", strings.Replace(dfFromObj.FileName, ".enc", "", -1)))
			_, _ = io.Copy(w, bytes.NewReader(file))
		}
	}
}

func FileDownloadHandlerB64(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("FileDownloadHandlerB64 - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		fileName := vars["filename"]
		folderPath := vars["folderpath"]
		var err error

		//ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		//defer cancel()
		//_ = ctx
		dfFromObj := module_store.FileDownloadRequest{}
		if fileName != "" && folderPath != "" {
			dfFromObj.FolderPath = folderPath
			dfFromObj.FileName = fileName
		} else {
			dfFromReq := json.NewDecoder(r.Body)
			dfFromReq.DisallowUnknownFields()
			//dfFromObj := make(map[string]string)
			if err := dfFromReq.Decode(&dfFromObj); err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}

		if dfFromObj.FileId != "" || dfFromObj.SharedWithMe || dfFromObj.OwnerEmail != "" || dfFromObj.ModifiedAfter != "" || dfFromObj.MimeType != "" || dfFromObj.ExportMimeType != "" {
			fileB64, mimeType, fileName, fileId, fileMeta, candidates, sErr := sh.Store.DownloadFileSmart(r.Context(), projectId, storageName, dfFromObj, sh.Store)
			if sErr != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": sErr.Error()})
				return
			}
			if len(candidates) > 0 {
				server_handlers.FormatResponse(w, http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"status":     "multiple_matches",
					"message":    "more than one file matches; call again with file_id from candidates",
					"candidates": candidates,
				})
				return
			}
			server_handlers.FormatResponse(w, http.StatusOK)
			downloadRes := map[string]interface{}{"file": fileB64, "file_type": mimeType, "file_name": fileName, "file_id": fileId}
			for k, v := range fileMeta {
				downloadRes[k] = v
			}
			json.NewEncoder(w).Encode(downloadRes)
			return
		}

		fileB64, mimeType, err := sh.Store.DownloadFileB64(r.Context(), projectId, storageName, dfFromObj, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"file": fileB64, "file_type": mimeType})
	}
}
func FileDownloadHandlerUnzip(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("FileDownloadHandlerUnzip - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]

		var err error
		//ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		//defer cancel()
		//_ = ctx

		dfFromReq := json.NewDecoder(r.Body)
		dfFromReq.DisallowUnknownFields()
		dfFromObj := module_store.FileDownloadRequest{}
		if err := dfFromReq.Decode(&dfFromObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		vErr := utils.ValidateStruct(r.Context(), dfFromObj, "")
		if vErr != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": vErr.Error()})
			return
		}

		files, err := sh.Store.DownloadFileUnzip(r.Context(), projectId, storageName, dfFromObj, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"files": files})
	}
}

func FileUploadHandlerB64(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("FileUploadHandlerB64 - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]

		var err error
		//ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		//defer cancel()
		//_ = ctx

		ufFromReq := json.NewDecoder(r.Body)
		ufFromReq.DisallowUnknownFields()
		ufFromObj := make(map[string]string)

		if err := ufFromReq.Decode(&ufFromObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		file, ok := ufFromObj["file"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "file attribute missing"})
			return
		}
		docType, ok := ufFromObj["doc_type"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "doc_type attribute missing"})
			return
		}
		fileName, ok := ufFromObj["file_name"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "file_name attribute missing"})
			return
		}
		folderPath, ok := ufFromObj["folder_path"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "folder_path attribute missing"})
			return
		}
		fileBytes, err := b64.StdEncoding.DecodeString(file)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "base64 decode failed"})
			return
		}
		docId, err := sh.Store.UploadFileB64(r.Context(), projectId, storageName, fileBytes, fileName, docType, folderPath, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		fileNames := make(map[string]string)
		fileNames[fileName] = docId
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"files": fileNames})
	}
}

func FileUploadHandlerFromUrl(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("FileUploadHandlerFromUrl - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]

		var err error
		//ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		//defer cancel()
		//_ = ctx

		ufFromReq := json.NewDecoder(r.Body)
		ufFromReq.DisallowUnknownFields()
		ufFromObj := make(map[string]string)

		if err := ufFromReq.Decode(&ufFromObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		url, ok := ufFromObj["url"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "file attribute missing"})
			return
		}
		docType, ok := ufFromObj["doc_type"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "doc_type attribute missing"})
			return
		}

		fileName, ok := ufFromObj["file_name"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "file_name attribute missing"})
			return
		}

		fileType, ok := ufFromObj["file_type"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "file_type attribute missing"})
			return
		}

		folderPath, ok := ufFromObj["folder_path"]
		if !ok {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "folder_path attribute missing"})
			return
		}
		docId, err := sh.Store.UploadFileFromUrl(r.Context(), projectId, storageName, url, fileName, docType, folderPath, fileType, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		fileNames := make(map[string]string)
		fileNames[fileName] = docId
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"files": fileNames})
	}
}

func FileUploadHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("FileUploadHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]

		var err error

		reqContentType := strings.Split(r.Header.Get("Content-type"), ";")[0]
		if reqContentType == encodedForm || reqContentType == multiPartForm {
			err = r.ParseMultipartForm((1 << 20) * 10)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
			}

			formData := r.MultipartForm
			folderPath := formData.Value["folderpath"][0]
			docTypes := formData.Value["doctype"]

			//keyPairName := formData.Value["keyPairName"][0]
			fileNames := make(map[string]string)
			files := formData.File["files"]
			for _, f := range files {
				docType := ""
				for _, dt := range docTypes {
					tmpDt := strings.Split(dt, ":")
					if tmpDt[1] == f.Filename {
						docType = tmpDt[0]
						break
					}
				}
				file, err := f.Open()
				defer file.Close()
				if err != nil {
					fmt.Fprintln(w, err)
					return
				}
				//TODO - check for file size and check for file meme
				docId, err := sh.Store.UploadFile(r.Context(), projectId, storageName, file, f, docType, folderPath, sh.Store)
				if err != nil {
					server_handlers.FormatResponse(w, 400)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
					return
				}
				fileNames[f.Filename] = docId
			}
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"files": fileNames})

			return
		} else {
			err = r.ParseForm()
		}
		if err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprintf("Could not parse form: %v", err))
			return
		}
	}
}

func ExcelToJsonHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		logs.WithContext(r.Context()).Debug("FileUploadHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		var err error

		reqContentType := strings.Split(r.Header.Get("Content-type"), ";")[0]
		if reqContentType == encodedForm || reqContentType == multiPartForm {
			err = r.ParseMultipartForm((1 << 20) * 10)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
			}

			formData := r.MultipartForm

			fileJsons := make(map[string]interface{})
			for _, files := range formData.File {
				for _, f := range files {
					fdr := module_store.FileDownloadRequest{}
					fdr.ExcelAsJson = true
					sheets := formData.Value["excel_sheets"]
					if sheets != nil {
						err = json.Unmarshal([]byte(sheets[0]), &fdr.ExcelSheets)
						if err != nil {
							logs.WithContext(r.Context()).Error(err.Error())
							server_handlers.FormatResponse(w, 200)
							_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "error reading the sheets attributes"})
						}
					} else {
						fdr.ExcelSheets = make(map[string]map[string]eru_reads.FileReadData)
						fdr.ExcelSheets["*"] = make(map[string]eru_reads.FileReadData)
						rd := eru_reads.FileReadData{}
						fdr.ExcelSheets["*"]["*"] = rd
					}
					logs.WithContext(r.Context()).Info(fmt.Sprint(fdr))

					file, err := f.Open()
					defer file.Close()
					if err != nil {
						logs.WithContext(r.Context()).Error(err.Error())
						server_handlers.FormatResponse(w, 200)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
						return
					}
					//TODO - check for file size and check for file meme
					fileJson, err := sh.Store.ExcelToJson(r.Context(), projectId, file, f, fdr, sh.Store)
					if err != nil {
						server_handlers.FormatResponse(w, 400)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
						return
					}
					fileJsons[f.Filename] = fileJson
				}
			}

			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(fileJsons)

			return
		} else {
			err = r.ParseForm()
		}
		if err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprintf("Could not parse form: %v", err))
			return
		}
	}
}
func CsvDataToJsonHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		vars := mux.Vars(r)
		projectId := vars["project"]
		var err error

		ufFromReq := json.NewDecoder(r.Body)
		ufFromReq.DisallowUnknownFields()
		type ufFromObj struct {
			CsvData string `json:"csv_data"`
		}
		ufFromMap := ufFromObj{}

		if err := ufFromReq.Decode(&ufFromMap); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		csvBytes, err := b64.StdEncoding.DecodeString(ufFromMap.CsvData)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "base64 decode failed"})
			return
		}

		fdr := module_store.FileDownloadRequest{}
		fdr.CsvAsJson = true

		logs.WithContext(r.Context()).Info(fmt.Sprint(fdr))
		fileJson, err := sh.Store.BytesToJson(r.Context(), projectId, csvBytes, fdr, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(fileJson)

		return
	}
}

func ExcelToJsonB64Handler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		vars := mux.Vars(r)
		projectId := vars["project"]
		var err error

		ufFromReq := json.NewDecoder(r.Body)
		ufFromReq.DisallowUnknownFields()
		type ufFromObj struct {
			FileName    string                                       `json:"file_name"`
			ExcelSheets map[string]map[string]eru_reads.FileReadData `json:"excel_sheets"`
			File        string                                       `json:"file"`
		}
		ufFromMap := ufFromObj{}

		if err := ufFromReq.Decode(&ufFromMap); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		fileJsons := make(map[string]interface{})
		fileBytes, err := b64.StdEncoding.DecodeString(ufFromMap.File)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "base64 decode failed"})
			return
		}
		fdr := module_store.FileDownloadRequest{}
		fdr.ExcelAsJson = true
		sheets := ufFromMap.ExcelSheets
		if sheets != nil {
			fdr.ExcelSheets = sheets
		} else {
			fdr.ExcelSheets = make(map[string]map[string]eru_reads.FileReadData)
			fdr.ExcelSheets["*"] = make(map[string]eru_reads.FileReadData)
			rd := eru_reads.FileReadData{}
			fdr.ExcelSheets["*"]["*"] = rd
		}
		logs.WithContext(r.Context()).Info(fmt.Sprint(fdr))
		fileJson, err := sh.Store.BytesToJson(r.Context(), projectId, fileBytes, fdr, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		fileJsons[ufFromMap.FileName] = fileJson

		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(fileJsons)

		return
	}
}
func JsonValidatorHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("JsonValidatorHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}

		type schemePostBody struct {
			Fields []*json.RawMessage `json:"fields" eru:"required"`
			Data   []interface{}      `json:"data" eru:"required"`
		}
		spb := schemePostBody{}
		schema := validator.Schema{}
		schJson := json.NewDecoder(r.Body)
		schJson.DisallowUnknownFields()
		if err := schJson.Decode(&spb); err == nil {
			err = schema.SetFields(r.Context(), spb.Fields)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 200)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			var jsonData gjson.Result
			dataBytes, dataBytesErr := json.Marshal(spb.Data)
			if dataBytesErr != nil {
				logs.WithContext(r.Context()).Error(dataBytesErr.Error())
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": dataBytesErr.Error()})
				return
			}
			jsonData = gjson.ParseBytes(dataBytes)
			rec, errRec := schema.Validate(r.Context(), jsonData)
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"records": rec, "error": errRec})
			return
		}
	}
}

func GetStorageTokenHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("GetStorageTokenHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]

		storageObj, _, err := sh.Store.GetStorageClone(r.Context(), projectId, storageName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		authNameI, err := storageObj.GetAttribute("auth_name")
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		authName, _ := authNameI.(string)
		if authName == "" {
			err = errors.New("storage has no auth_name configured; only IdP-backed storages support gettoken")
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		baseUrl := os.Getenv("ERUAUTH_BASEURL")
		if baseUrl == "" {
			err = errors.New("ERUAUTH_BASEURL env not set")
			server_handlers.FormatResponse(w, 500)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		url := fmt.Sprintf("%s/%s/%s/gettoken", strings.TrimRight(baseUrl, "/"), projectId, authName)
		headers := http.Header{}
		headers.Set("Content-Type", "application/json")
		qParams := map[string]string{}
		if prefix, _ := r.Context().Value("tokenkeyprefix").(string); prefix != "" {
			qParams["token_key_prefix"] = prefix
		}
		res, _, _, status, err := utils.CallHttp(r.Context(), http.MethodGet, url, headers, map[string]string{}, nil, qParams, nil)
		if err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprintf("eruauth gettoken failed (status %d): %s", status, err.Error()))
			server_handlers.FormatResponse(w, http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
	}
}

type gdriveWatchChangesBody struct {
	ChannelId    string `json:"channel_id"`
	PushEndpoint string `json:"push_endpoint"`
	ExpirationMs int64  `json:"expiration_ms"`
}

type gdriveWatchFileBody struct {
	ChannelId    string `json:"channel_id"`
	PushEndpoint string `json:"push_endpoint"`
	ExpirationMs int64  `json:"expiration_ms"`
}

type gdriveStopWatchBody struct {
	ChannelId  string `json:"channel_id"`
	ResourceId string `json:"resource_id"`
}

type gdriveListChangesBody struct {
	PageToken string `json:"page_token"`
}

func GdriveWatchChangesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		body := gdriveWatchChangesBody{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if body.ChannelId == "" || body.PushEndpoint == "" {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "channel_id and push_endpoint are required"})
			return
		}
		resourceId, startPageToken, expiration, err := sh.Store.GdriveWatchChanges(r.Context(), projectId, storageName, body.ChannelId, body.PushEndpoint, body.ExpirationMs, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"channel_id":       body.ChannelId,
			"resource_id":      resourceId,
			"start_page_token": startPageToken,
			"expiration":       expiration,
		})
	}
}

func GdriveWatchFileHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		fileId := vars["file_id"]
		body := gdriveWatchFileBody{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if body.ChannelId == "" || body.PushEndpoint == "" || fileId == "" {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "file_id, channel_id and push_endpoint are required"})
			return
		}
		resourceId, expiration, err := sh.Store.GdriveWatchFile(r.Context(), projectId, storageName, fileId, body.ChannelId, body.PushEndpoint, body.ExpirationMs, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"channel_id":  body.ChannelId,
			"resource_id": resourceId,
			"file_id":     fileId,
			"expiration":  expiration,
		})
	}
}

func GdriveStopWatchHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		body := gdriveStopWatchBody{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if err := sh.Store.GdriveStopWatch(r.Context(), projectId, storageName, body.ChannelId, body.ResourceId, sh.Store); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}
}

func GdriveInspectFileHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		fileId := vars["file_id"]
		res, err := sh.Store.GdriveInspectFile(r.Context(), projectId, storageName, fileId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
	}
}

type gdriveSheetValuesBody struct {
	Ranges          []string `json:"ranges"`
	ConvertIfOffice *bool    `json:"convert_if_office"`
}

type gdriveSheetMirrorBody struct {
	Name string `json:"name"`
}

func GdriveCreateSheetMirrorHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		fileId := vars["file_id"]
		body := gdriveSheetMirrorBody{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		res, err := sh.Store.GdriveCreateSheetMirror(r.Context(), projectId, storageName, fileId, body.Name, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
	}
}

func GdriveReadSheetValuesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		fileId := vars["file_id"]
		body := gdriveSheetValuesBody{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		convertIfOffice := false
		if body.ConvertIfOffice != nil {
			convertIfOffice = *body.ConvertIfOffice
		}
		res, err := sh.Store.GdriveReadSheetValues(r.Context(), projectId, storageName, fileId, body.Ranges, convertIfOffice, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
	}
}

func GdriveListChangesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		storageName := vars["storagename"]
		body := gdriveListChangesBody{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if body.PageToken == "" {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "page_token is required"})
			return
		}
		changes, newStartPageToken, nextPageToken, err := sh.Store.GdriveListChanges(r.Context(), projectId, storageName, body.PageToken, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"changes":              changes,
			"new_start_page_token": newStartPageToken,
			"next_page_token":      nextPageToken,
		})
	}
}
