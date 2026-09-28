package file_server

import (
	"context"
	"net/http"

	file_handlers "github.com/eru-os/eru/eru-files/file_server/handlers"
	"github.com/eru-os/eru/eru-files/module_store"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func tokenKeyPrefixMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("token_key_prefix")
		if prefix == "" {
			prefix = r.Header.Get("X-Token-Key-Prefix")
		}
		if prefix != "" {
			r = r.WithContext(context.WithValue(r.Context(), "tokenkeyprefix", prefix))
		}
		next.ServeHTTP(w, r)
	})
}

func SetServiceName() {
	server_handlers.ServerName = "eru-files"
}
func AddFileRoutes(serverRouter *mux.Router, sh *module_store.StoreHolder) {

	//store functions specific to files
	storeRouter := serverRouter.PathPrefix("/store").Subrouter()
	storeRouter.Use(tokenKeyPrefixMw)
	storeRouter.Methods(http.MethodGet).Path("/load").HandlerFunc(file_handlers.StoreLoadHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/compare").HandlerFunc(file_handlers.StoreCompareHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/storage/save/{storagename}/{storagetype}").HandlerFunc(file_handlers.StorageSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/storage/remove/{storagename}").HandlerFunc(file_handlers.StorageRemoveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/storage/remove/{storagename}/{clouddelete}").HandlerFunc(file_handlers.StorageRemoveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/storage/remove/{storagename}/{clouddelete}/{forcedelete}").HandlerFunc(file_handlers.StorageRemoveHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/save").HandlerFunc(file_handlers.ProjectSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/remove").HandlerFunc(file_handlers.ProjectRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/project/list").HandlerFunc(file_handlers.ProjectListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/config").HandlerFunc(file_handlers.ProjectConfigHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/rsakeypair/generate/{keypairname}").HandlerFunc(file_handlers.RsaKeyPairGenerateHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/aeskey/generate/{keyname}").HandlerFunc(file_handlers.AesKeyGenerateHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/settings/save").HandlerFunc(file_handlers.ProjectSetingsSaveHandler(sh))

	// functions for file events
	fileRouter := serverRouter.PathPrefix("/files/{project}").Subrouter()
	fileRouter.Use(tokenKeyPrefixMw)
	fileRouter.Methods(http.MethodGet).Path("/{storagename}/gettoken").HandlerFunc(file_handlers.GetStorageTokenHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/{storagename}/upload").HandlerFunc(file_handlers.FileUploadHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/{storagename}/uploadb64").HandlerFunc(file_handlers.FileUploadHandlerB64(sh))
	fileRouter.Methods(http.MethodPost).Path("/{storagename}/uploadfromurl").HandlerFunc(file_handlers.FileUploadHandlerFromUrl(sh))
	fileRouter.Methods(http.MethodPost, http.MethodGet).Path("/{storagename}/download/{folderpath}/{filename}").HandlerFunc(file_handlers.FileDownloadHandler(sh))
	fileRouter.Methods(http.MethodPost, http.MethodGet).Path("/{storagename}/download").HandlerFunc(file_handlers.FileDownloadHandler(sh))
	fileRouter.Methods(http.MethodPost, http.MethodGet).Path("/{storagename}/downloadb64/{folderpath}/{filename}").HandlerFunc(file_handlers.FileDownloadHandlerB64(sh))
	fileRouter.Methods(http.MethodPost, http.MethodGet).Path("/{storagename}/downloadb64").HandlerFunc(file_handlers.FileDownloadHandlerB64(sh))
	fileRouter.Methods(http.MethodPost, http.MethodGet).Path("/{storagename}/downloadunzip").HandlerFunc(file_handlers.FileDownloadHandlerUnzip(sh))
	fileRouter.Methods(http.MethodPost).Path("/{storagename}/gdrive/watch/changes").HandlerFunc(file_handlers.GdriveWatchChangesHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/{storagename}/gdrive/watch/file/{file_id}").HandlerFunc(file_handlers.GdriveWatchFileHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/{storagename}/gdrive/watch/stop").HandlerFunc(file_handlers.GdriveStopWatchHandler(sh))
	fileRouter.Methods(http.MethodGet, http.MethodPost).Path("/{storagename}/gdrive/inspect/{file_id}").HandlerFunc(file_handlers.GdriveInspectFileHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/{storagename}/gdrive/sheetmirror/{file_id}").HandlerFunc(file_handlers.GdriveCreateSheetMirrorHandler(sh))
	fileRouter.Methods(http.MethodGet, http.MethodPost).Path("/{storagename}/gdrive/sheetvalues/{file_id}").HandlerFunc(file_handlers.GdriveReadSheetValuesHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/{storagename}/gdrive/changes").HandlerFunc(file_handlers.GdriveListChangesHandler(sh))
	fileRouter.Methods(http.MethodPost, http.MethodGet).Path("/stringtofile").HandlerFunc(file_handlers.StringToFileHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/exceltojson").HandlerFunc(file_handlers.ExcelToJsonHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/exceltojsonb64").HandlerFunc(file_handlers.ExcelToJsonB64Handler(sh))
	fileRouter.Methods(http.MethodPost).Path("/csvdatatojson").HandlerFunc(file_handlers.CsvDataToJsonHandler(sh))
	//fileRouter.Methods(http.MethodPost).Path("/exceltojsonurl").HandlerFunc(file_handlers.ExcelToJsonUrlHandler(sh))
	fileRouter.Methods(http.MethodPost).Path("/jsonvalidate").HandlerFunc(file_handlers.JsonValidatorHandler(sh))
}
