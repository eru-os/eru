package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/eru-os/eru/eru-events/events"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	"github.com/eru-os/eru/eru-repos/repos"
	scheduler "github.com/eru-os/eru/eru-scheduler/scheduler"
	kms "github.com/eru-os/eru/eru-secret-manager/kms"
	sm "github.com/eru-os/eru/eru-secret-manager/sm"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

var ServerName = "unkown"
var InstanceId = "unkown"
var RepoName = "unkown.json"
var AllowedOrigins = ""
var RequestIdKey = "request_id"
var BaseUrl = ""
var EruqlBaseUrl = ""

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/hello" {
		http.Error(w, "404 not found.", http.StatusNotFound)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}
	fmt.Fprint(w, "Hello ", ServerName)
}
func SetInstanceId() {
	InstanceId = fmt.Sprintf("%s-%s", ServerName, uuid.New().String())
}
func EnvHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("EnvHandler - Start")
		vars := mux.Vars(r)
		env := vars["env"]
		env_value := os.Getenv(env)
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{env: env_value})
	}
}

func StateHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("StateHandler - Start")
		type serviceState struct {
			NumCPU       int
			NumGoroutine int
			Mem          runtime.MemStats
		}
		serviceStateObj := serviceState{}

		serviceStateObj.NumCPU = runtime.NumCPU()
		serviceStateObj.NumGoroutine = runtime.NumGoroutine()
		runtime.ReadMemStats(&serviceStateObj.Mem)

		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"service_state": serviceStateObj})
	}
}

func SaveVarHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveVarHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		varJson := json.NewDecoder(r.Body)
		varJson.DisallowUnknownFields()
		var sVar store.Vars
		if err := varJson.Decode(&sVar); err == nil {
			err = s.SaveVar(r.Context(), projectId, sVar, s)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Variable with key ", sVar.Key, " saved successfully.")})
	}
}

func RemoveVarHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("RemoveVarHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		varKey := vars["key"]
		err := s.RemoveVar(r.Context(), projectId, varKey, s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Variable with key ", varKey, " removed successfully.")})
	}
}

func SaveEnvVarHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveEnvVarHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		varJson := json.NewDecoder(r.Body)
		varJson.DisallowUnknownFields()
		var sVar store.EnvVars
		if err := varJson.Decode(&sVar); err == nil {
			err = s.SaveEnvVar(r.Context(), projectId, sVar, s)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("env. variable with key ", sVar.Key, " saved successfully")})
	}
}

func RemoveEnvVarHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("RemoveSecretHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		varKey := vars["key"]
		err := s.RemoveEnvVar(r.Context(), projectId, varKey, s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Env. Variable with key ", varKey, " removed successfully.")})
	}
}

func SaveSecretHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveSecretHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		varJson := json.NewDecoder(r.Body)
		varJson.DisallowUnknownFields()
		var sVar store.Secrets
		if err := varJson.Decode(&sVar); err == nil {
			err = s.SaveSecret(r.Context(), projectId, sVar, s)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Secret with key ", sVar.Key, " saved successfully.")})
	}
}

func RemoveSecretHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("RemoveSecretHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		varKey := vars["key"]
		err := s.RemoveSecret(r.Context(), projectId, varKey, s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Secret with key ", varKey, " removed successfully.")})
	}
}

func FetchVarsHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FetchVarsHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		variables, err := s.FetchVars(r.Context(), projectId)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(variables)
	}
}

func SaveRepoHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveRepoHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		repoType := vars["repotype"]
		varJson := json.NewDecoder(r.Body)
		varJson.DisallowUnknownFields()
		sRepoI := repos.GetRepo(repoType)
		if err := varJson.Decode(&sRepoI); err == nil {
			err = s.SaveRepo(r.Context(), projectId, sRepoI, s, true)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Repo for project ", projectId, " saved successfully.")})
	}
}

func SaveRepoTokenHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveRepoTokenHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		varJson := json.NewDecoder(r.Body)
		varJson.DisallowUnknownFields()
		var sRepoToken repos.RepoToken
		if err := varJson.Decode(&sRepoToken); err == nil {
			err = s.SaveRepoToken(r.Context(), projectId, sRepoToken, s)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Repo Token for project ", projectId, " saved successfully.")})
	}
}

func FetchSmHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("FetchSmHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		smObj, err := s.FetchSm(r.Context(), projectId)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(smObj)
	}
}

func LoadSmValueHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("LoadSmValueHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		err := s.LoadSmValue(r.Context(), projectId)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": "secret values loaded successfully"})
	}
}
func SetSmValueHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("SetSmValueHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		if projectId == "" {
			projectId = "gateway"
		}

		type smMap struct {
			SecretName  string            `json:"secret_name"`
			SecretValue map[string]string `json:"secret_value"`
		}
		smMapObj := smMap{}

		smJson := json.NewDecoder(r.Body)
		smJson.DisallowUnknownFields()
		if err := smJson.Decode(&smMapObj); err == nil {
			if tenantId != "" {
				smMapObj.SecretName = tenantId
				for k, v := range smMapObj.SecretValue {
					sVar := store.Secrets{Key: k, Value: v}
					err = s.SaveTenantSecret(r.Context(), projectId, tenantId, sVar, s)
					if err != nil {
						logs.WithContext(r.Context()).Info(err.Error())
						FormatResponse(w, 400)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
						return
					}
				}
			}
			err = s.SetSmValue(r.Context(), projectId, smMapObj.SecretName, smMapObj.SecretValue)
			if err != nil {
				logs.WithContext(r.Context()).Info(err.Error())
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			logs.WithContext(r.Context()).Info("error")
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": "secret values set successfully"})
	}
}

func GetSmValueHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("GetSmValueHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		if projectId == "" {
			projectId = "gateway"
		}
		if fmt.Sprint(strings.Split(r.Host, ":")[0]) != "localhost" {
			err := errors.New("you can call this route only locally")
			logs.WithContext(r.Context()).Error(err.Error())
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		type smMap struct {
			SecretName  string `json:"secret_name"`
			SecretKey   string `json:"secret_key"`
			ForceDelete bool   `json:"force_delete"`
		}
		smMapObj := smMap{}
		var secret_value interface{}
		smJson := json.NewDecoder(r.Body)
		smJson.DisallowUnknownFields()
		if err := smJson.Decode(&smMapObj); err == nil {
			if tenantId != "" {
				smMapObj.SecretName = tenantId
			}
			secret_value, err = s.GetSmValue(r.Context(), projectId, smMapObj.SecretName, smMapObj.SecretKey, smMapObj.ForceDelete)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			logs.WithContext(r.Context()).Info("error")
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{smMapObj.SecretKey: secret_value})
	}
}

func UnsetSmValueHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("UnsetSmValueHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		if projectId == "" {
			projectId = "gateway"
		}

		type smMap struct {
			SecretName string `json:"secret_name"`
			SecretKey  string `json:"secret_key"`
		}
		smMapObj := smMap{}

		smJson := json.NewDecoder(r.Body)
		smJson.DisallowUnknownFields()
		if err := smJson.Decode(&smMapObj); err == nil {
			if tenantId != "" {
				smMapObj.SecretName = tenantId
				err = s.RemoveTenantSecret(r.Context(), projectId, tenantId, smMapObj.SecretKey, s)
				if err != nil {
					logs.WithContext(r.Context()).Info(err.Error())
					FormatResponse(w, 400)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
					return
				}
			}
			err = s.UnsetSmValue(r.Context(), projectId, smMapObj.SecretName, smMapObj.SecretKey)
			if err != nil {
				logs.WithContext(r.Context()).Info(err.Error())
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			logs.WithContext(r.Context()).Info("error")
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": "secret values unset successfully"})
	}
}

func SaveSmHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveSmHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		smType := vars["smtype"]

		smJson := json.NewDecoder(r.Body)
		smJson.DisallowUnknownFields()

		var smObj = sm.GetSm(smType)
		if err := smJson.Decode(&smObj); err == nil {
			err = s.SaveSm(r.Context(), projectId, smObj, s, true)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Secret Manager for project ", projectId, " saved successfully.")})
	}
}

func FetchRepoHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FetchRepoHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		repo, err := s.FetchRepo(r.Context(), projectId)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(repo)
	}
}

func CommitRepoHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("CommitRepoHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		err := s.CommitRepo(r.Context(), projectId, s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("config for project ", projectId, " commited successfully.")})
		//_ = json.NewEncoder(w).Encode(config)
	}
}

func FetchKmsHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("FetchKmsHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		kmsObj, err := s.FetchKms(r.Context(), projectId)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(kmsObj)
	}
}

func SaveKmsHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveKmsHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		kmsType := vars["kmstype"]

		kmsJson := json.NewDecoder(r.Body)
		kmsJson.DisallowUnknownFields()

		var kmsObj = kms.GetKms(kmsType)
		if err := kmsJson.Decode(&kmsObj); err == nil {
			err = s.SaveKms(r.Context(), projectId, kmsObj, s, true)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Key for project ", projectId, " saved successfully.")})
	}
}

func RemoveKmsHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveKmsHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		kmsName := vars["kmsname"]
		cloudDelete := vars["clouddelete"]
		cd := false
		if cloudDelete == "true" {
			cd = true
		}
		deleteDays := vars["deletedays"]
		var dd int64 = 7
		var err error
		if deleteDays != "" {
			dd, err = strconv.ParseInt(deleteDays, 10, 32)
			if err != nil {
				FormatResponse(w, 400)
				logs.WithContext(r.Context()).Info(err.Error())
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "invalid delete days"})
				return
			}
		}

		err = s.RemoveKms(r.Context(), projectId, kmsName, cd, int32(dd), s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Key for project ", projectId, " removed successfully.")})
	}
}

func FetchEventsHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("FetchEventsHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		eventObj, err := s.FetchEvents(r.Context(), projectId)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(eventObj)
	}
}

func SaveEventHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveEventHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		eventType := vars["eventtype"]

		eventJson := json.NewDecoder(r.Body)
		eventJson.DisallowUnknownFields()

		var eventObj = events.GetEvent(eventType)
		if err := eventJson.Decode(&eventObj); err == nil {
			err = s.SaveEvent(r.Context(), projectId, eventObj, s, true)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("event for project ", projectId, " saved successfully.")})
	}
}

func PublishEventHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("PublishEventHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		eventName := vars["eventname"]

		eventJson := json.NewDecoder(r.Body)
		eventJson.DisallowUnknownFields()

		type msgType struct {
			Msg interface{} `json:"msg"`
		}
		var msgObj msgType
		msgId := ""
		if err := eventJson.Decode(&msgObj); err == nil {
			msgId, err = s.PublishEvent(r.Context(), projectId, eventName, msgObj, s)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("event published for project ", projectId, " : ", msgId)})
	}
}

func PollEventHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("PollEventHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		eventName := vars["eventname"]

		err := s.PollEvent(r.Context(), projectId, eventName, s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("event polled for project ", projectId, ".")})
	}
}

func SubscribeEventHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SubscribeEventHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		eventName := vars["eventname"]

		eventJson := json.NewDecoder(r.Body)
		eventJson.DisallowUnknownFields()

		subscription := make(map[string]interface{})
		if err := eventJson.Decode(&subscription); err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		eventI, err := s.FetchEvent(r.Context(), projectId, eventName, s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if err = eventI.Subscribe(r.Context(), subscription); err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if err = s.SaveEvent(r.Context(), projectId, eventI, s, false); err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if err = s.SaveStore(r.Context(), projectId, "", s); err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		response := map[string]interface{}{
			"msg": fmt.Sprint("subscription for event ", eventName, " for project ", projectId, " subscribed successfully."),
		}
		if topicName, attrErr := eventI.GetAttribute("topic_name"); attrErr == nil {
			response["topic_name"] = topicName
		}
		if oidcAud, attrErr := eventI.GetAttribute("oidc_audience"); attrErr == nil {
			response["oidc_audience"] = oidcAud
		}
		if oidcSa, attrErr := eventI.GetAttribute("oidc_service_account"); attrErr == nil {
			response["oidc_service_account"] = oidcSa
		}
		if subId, ok := subscription["subscription_id"]; ok {
			response["subscription_id"] = subId
		}

		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(response)
	}
}

func UnsubscribeEventHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("UnsubscribeEventHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		eventName := vars["eventname"]
		subscriptionId := vars["subscriptionid"]
		logs.WithContext(r.Context()).Info(fmt.Sprintf("eventname: %s", eventName))
		logs.WithContext(r.Context()).Info(fmt.Sprintf("subscriptionid: %s", subscriptionId))
		eventI, err := s.FetchEvent(r.Context(), projectId, eventName, s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		logs.WithContext(r.Context()).Info(fmt.Sprintf("eventI: %v", eventI))
		err = eventI.Unsubscribe(r.Context(), subscriptionId)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		err = s.SaveStore(r.Context(), projectId, "", s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("subscription for event ", eventName, " for project ", projectId, " unsubscribed successfully.")})
	}
}

func ListSubscriptionsEventHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ListSubscriptionsEventHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		eventName := vars["eventname"]
		eventI := events.GetEvent(eventName)
		subscriptions, err := eventI.ListSubscriptions(r.Context())
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(subscriptions)
	}
}

func RemoveEventHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("RemoveEventHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		eventName := vars["eventname"]
		cloudDelete := vars["clouddelete"]
		cd := false
		if cloudDelete == "true" {
			cd = true
		}

		err := s.RemoveEvent(r.Context(), projectId, eventName, cd, s)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("event ", eventName, "for project ", projectId, " removed successfully.")})
	}
}

func SaveSchedulerHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("SaveSchedulerHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		schedulerType := vars["schedulertype"]

		schedulerJson := json.NewDecoder(r.Body)
		schedulerJson.DisallowUnknownFields()

		var schedulerObj = scheduler.GetScheduler(schedulerType)
		if err := schedulerJson.Decode(&schedulerObj); err == nil {
			err = s.SaveScheduler(r.Context(), projectId, schedulerObj, s, true)
			if err != nil {
				FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		} else {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Scheduler for project ", projectId, " saved successfully.")})
	}
}

func FetchSchedulerHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Info("FetchSchedulerHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		if projectId == "" {
			projectId = "gateway"
		}
		schedulerObj, err := s.FetchScheduler(r.Context(), projectId)
		if err != nil {
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(schedulerObj)
	}
}
func RequestListHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncRequestListHandler - Start")
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		resourceName := vars["resource"]
		requests, err := s.GetRequests(r.Context(), projectId, tenantId, resourceName, s)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(requests)
	}
}
func RequestSaveHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncRequestSaveHandler - Start")
		defer r.Body.Close()
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]

		var sampleRequest models.SampleRequest
		err := json.NewDecoder(r.Body).Decode(&sampleRequest)
		if err != nil {
			err = logs.Err(r.Context(), err, "error decode request body")
			FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), sampleRequest, "")
			if err != nil {
				FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		err = s.SaveRequest(r.Context(), sampleRequest, projectId, tenantId, s)
		if err != nil {
			err = logs.Err(r.Context(), err, "error saving function request")
			FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprintf("function request %s saved successfully", sampleRequest.RequestName)})
		}
	}
}

func RequestRemoveHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FuncRequestRemoveHandler - Start")
		defer r.Body.Close()
		vars := mux.Vars(r)
		requestId := vars["requestid"]
		err := s.RemoveRequest(r.Context(), requestId, s)
		if err != nil {
			err = logs.Err(r.Context(), err, "error removing function request")
			FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"msg": fmt.Sprintf("function request %s removed successfully", requestId)})
	}
}
