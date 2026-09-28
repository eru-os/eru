package functions

import (
	"bytes"
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-templates/gotemplate"
	"github.com/google/uuid"
)

type SafeVarsMap struct {
	mu   sync.RWMutex
	vars map[string]*TemplateVars
}

func NewSafeVarsMap() *SafeVarsMap {
	return &SafeVarsMap{
		vars: make(map[string]*TemplateVars),
	}
}

func (s *SafeVarsMap) Set(key string, value *TemplateVars) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vars[key] = value
}

func (s *SafeVarsMap) Get(key string) (*TemplateVars, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.vars[key]
	return val, ok
}

func (s *SafeVarsMap) Clone(ctx context.Context) (map[string]*TemplateVars, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clonedMap := make(map[string]*TemplateVars)
	for k, v := range s.vars {
		if v != nil {
			clonedV, err := cloneInterface(ctx, v)
			if err != nil {
				return nil, err
			}
			clonedMap[k] = clonedV.(*TemplateVars)
		} else {
			clonedMap[k] = nil
		}
	}
	return clonedMap, nil
}

func (s *SafeVarsMap) ToMap() map[string]*TemplateVars {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*TemplateVars)
	for k, v := range s.vars {
		result[k] = v
	}
	return result
}

//func fetchClaimsFromToken(ctx context.Context, strToken string, jwkUrl string) (claims interface{}, err error) {
//	return erujwt.DecryptTokenJWK(ctx, strToken, jwkUrl)
//}

func createFormFileCopy(w *multipart.Writer, part *multipart.Part) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, part.FormName(), part.FileName()))
	h.Set("Content-Type", part.Header.Get("Content-Type"))
	return w.CreatePart(h)
}

func createFormFile(w *multipart.Writer, contentType string, fieldName string, fileName string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileName))
	h.Set("Content-Type", contentType)
	return w.CreatePart(h)
}

func loadRequestVars(ctx context.Context, vars *TemplateVars, request *http.Request, tokenHeaderKey string) (err error) {
	logs.WithContext(ctx).Debug("loadRequestVars - Start")
	vars.Headers = make(map[string]interface{})
	for k, v := range request.Header {
		vars.Headers[k] = v
	}
	tokenStr := request.Header.Get(tokenHeaderKey)
	if tokenStr != "" {
		err = json.Unmarshal([]byte(tokenStr), &vars.Token)
		if err != nil {
			err = logs.Err(ctx, err, "")
			return
		}
	}
	vars.Params = make(map[string]interface{})
	for k, v := range request.URL.Query() {
		vars.Params[k] = v
	}

	reqContentType := strings.Split(request.Header.Get("Content-type"), ";")[0]
	if reqContentType == applicationjson && request.ContentLength > 0 {

		tmplBodyFromReq := json.NewDecoder(request.Body)
		tmplBodyFromReq.DisallowUnknownFields()
		if err = tmplBodyFromReq.Decode(&vars.Body); err != nil {
			err = logs.Err(ctx, fmt.Errorf("error decode request body : %w", err), "")
			return err
		}
		body, err := json.Marshal(vars.Body)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("json.Marshal(vars.Body) error : %w", err), "")
			return err
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.Header.Set("Content-Length", strconv.Itoa(len(body)))
		request.ContentLength = int64(len(body))
	}
	if vars.Vars == nil {
		vars.Vars = make(map[string]interface{})
	}
	vars.OrgBody = vars.Body
	return
}

func CloneRequest(ctx context.Context, request *http.Request) (req *http.Request, err error) {
	logs.WithContext(ctx).Debug("CloneRequest - Start")
	req = request.Clone(request.Context())

	//Only request.clone does not work - need to handle multipart request as under

	reqContentType := strings.Split(req.Header.Get("Content-type"), ";")[0]
	if reqContentType == multiPartForm {
		var reqBody bytes.Buffer
		var reqOldBody bytes.Buffer
		multipartWriter := multipart.NewWriter(&reqBody)
		multiPart, err1 := request.MultipartReader()
		if err1 != nil {
			logs.WithContext(ctx).Error(err1.Error())
		} else {
			for {
				part, errPart := multiPart.NextRawPart()
				if errPart == io.EOF {
					err = logs.Err(ctx, fmt.Errorf("inside EOF error"), "")
					break
				}
				if part.FileName() != "" {
					var tempFile *os.File
					//TODO - hard coded temp file name to be removed and remove deprecated ioutil
					tempFile, err = ioutil.TempFile(os.TempDir(), "spa")
					defer tempFile.Close()
					if err != nil {
						err = logs.Err(ctx, fmt.Errorf("Temp file creation failed : %w", err), "")
						return
					}
					fileWriter, err2 := createFormFileCopy(multipartWriter, part)
					if err2 != nil {
						err = err2
						return
					}
					//_, err = fileWriter.Write()
					_, err = io.Copy(fileWriter, part)
					if err != nil {
						err = logs.Err(ctx, fmt.Errorf("io.Copy error : %w", err), "")
						return
					}

				} else {
					buf := new(bytes.Buffer)
					buf.ReadFrom(part)
					fieldWriter, err3 := multipartWriter.CreateFormField(part.FormName())
					if err3 != nil {
						err = err3
						err = logs.Err(ctx, fmt.Errorf("multipartWriter.CreateFormField error : %w", err), "")
						return
					}
					_, err = fieldWriter.Write(buf.Bytes())
					if err != nil {
						err = logs.Err(ctx, fmt.Errorf("fileWriter.Write error : %w", err), "")
						return
					}
				}
			}
		}
		multipartWriter.Close()
		reqOldBody = reqBody
		req.Body = io.NopCloser(&reqBody)
		req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		req.Header.Set("Content-Length", strconv.Itoa(reqBody.Len()))
		req.ContentLength = int64(reqBody.Len())
		request.Body = io.NopCloser(&reqOldBody)
		request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		request.Header.Set("Content-Length", strconv.Itoa(reqOldBody.Len()))
		request.ContentLength = int64(reqOldBody.Len())

	} else if reqContentType == encodedForm {
		formData := url.Values{}
		rpfErr := request.ParseForm()
		if rpfErr != nil {
			err = logs.Err(ctx, fmt.Errorf("error from request.ParseForm() : %w", rpfErr), "")
			return
		}
		if request.Form != nil {
			for k, v := range request.Form {
				formData.Set(k, strings.Join(v, ","))
			}
		}
		req.Body = io.NopCloser(strings.NewReader(formData.Encode()))
		req.Header.Add("Content-Length", strconv.Itoa(len(formData.Encode())))

		request.Body = io.NopCloser(strings.NewReader(formData.Encode()))
		request.Header.Add("Content-Length", strconv.Itoa(len(formData.Encode())))
	} else {
		body, err3 := io.ReadAll(req.Body)
		if err3 != nil {
			err = logs.Err(ctx, fmt.Errorf("io.ReadAll error : %w", err3), "")
			return
		}
		// A body already read to the end reads back empty rather than failing, so
		// a caller that clones the same request twice silently gets an empty one
		// the second time - and the step then dies decoding a body its
		// Content-Length promised. GetBody exists to hand the body out again;
		// when the source offers one, use it rather than the spent reader.
		if len(body) == 0 && request.ContentLength > 0 && request.GetBody != nil {
			replay, errGb := request.GetBody()
			if errGb != nil {
				err = logs.Err(ctx, fmt.Errorf("request.GetBody error : %w", errGb), "")
				return
			}
			body, err3 = io.ReadAll(replay)
			_ = replay.Close()
			if err3 != nil {
				err = logs.Err(ctx, fmt.Errorf("io.ReadAll(GetBody) error : %w", err3), "")
				return
			}
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	return
}

func processTemplate(ctx context.Context, templateName string, templateString string, vars *FuncTemplateVars, outputType string, tokenHeaderKey string) (output []byte, err error) {
	logs.WithContext(ctx).Debug("processTemplate - Start")
	goTmpl := gotemplate.GoTemplate{Name: templateName, Template: templateString}
	outputObj, err := goTmpl.Execute(ctx, vars, outputType)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error()) // send original error as consumer has to handle <no value> string
		return nil, err
	} else {
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		err = encoder.Encode(outputObj)
		//output, err = json.Marshal(outputObj)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("encoder.Encode error : %w", err), "")
			return nil, err
		}
		output = []byte(strings.TrimSuffix(buffer.String(), "\n"))
		if len(string(output)) < 1000 {
			logs.WithContext(ctx).Info(fmt.Sprint("output ===== ", string(output)))
		} else {
			logs.WithContext(ctx).Info(fmt.Sprint("output ===== ", string(output)[:1000]))
		}

		if string(output) == "null" || string(output) == `"null"` {
			_ = logs.Err(ctx, fmt.Errorf("inside string(output) == \"null\" - %s", templateString), "")
			output = []byte("")
		}
		return
	}
}
func makeMultipart(ctx context.Context, request *http.Request, formData []Headers, fileData []FilePart, vars *TemplateVars, reqVars map[string]*TemplateVars, resVars map[string]*TemplateVars, tokenSecretKey string) (varsFormData map[string]interface{}, varsFormDataKeyArray []string, err error) {
	logs.WithContext(ctx).Debug("makeMultipart - Start")

	reqContentType := strings.Split(request.Header.Get("Content-type"), ";")[0]
	varsFormData = make(map[string]interface{})
	if reqContentType == encodedForm || reqContentType == multiPartForm {
		var reqBody bytes.Buffer
		multipartWriter := multipart.NewWriter(&reqBody)
		for _, fd := range formData {
			fieldWriter, errfw := multipartWriter.CreateFormField(fd.Key)
			if errfw != nil {
				err = errfw
				err = logs.Err(ctx, err, "")
				return nil, nil, err
			}
			if fd.IsTemplate {
				fvars := &FuncTemplateVars{}
				fvars.Vars = vars
				fvars.ResVars = resVars
				fvars.ReqVars = reqVars

				output, errop := processTemplate(ctx, fd.Key, fd.Value, fvars, "string", tokenSecretKey)
				if errop != nil {
					err = errop
					return
				}
				outputStr := string(output)
				if str, err := strconv.Unquote(outputStr); err == nil {
					outputStr = str
				}
				_, err = fieldWriter.Write([]byte(outputStr))

			} else {
				_, err = fieldWriter.Write([]byte(fd.Value))
			}
			if err != nil {
				err = logs.Err(ctx, err, "")
				return nil, nil, err
			}
			varsFormData[fd.Key] = fd.Value
			varsFormDataKeyArray = append(varsFormDataKeyArray, fd.Key)
		}
		for _, fl := range fileData {
			fvars := &FuncTemplateVars{}
			fvars.Vars = vars
			fvars.ResVars = resVars
			fvars.ReqVars = reqVars
			filename, errop := processTemplate(ctx, "filename", fl.FileName, fvars, "string", tokenSecretKey)
			if errop != nil {
				err = errop
				return
			}
			filenameStr := string(filename)
			if str, err := strconv.Unquote(filenameStr); err == nil {
				filenameStr = str
			}
			f2vars := &FuncTemplateVars{}
			f2vars.Vars = vars
			f2vars.ResVars = resVars
			f2vars.ReqVars = reqVars
			filevarname, errop := processTemplate(ctx, "filevarname", fl.FileVarName, f2vars, "string", tokenSecretKey)
			if errop != nil {
				err = errop
				return
			}
			filevarnameStr := string(filevarname)
			if str, err := strconv.Unquote(filevarnameStr); err == nil {
				filevarnameStr = str
			}
			f3vars := &FuncTemplateVars{}
			f3vars.Vars = vars
			f3vars.ResVars = resVars
			f3vars.ReqVars = reqVars
			filecontent, errop := processTemplate(ctx, "filecontent", fl.FileContent, f3vars, "string", tokenSecretKey)
			if errop != nil {
				err = errop
				return
			}
			filecontentStr := string(filecontent)
			str := ""
			if str, err = strconv.Unquote(filecontentStr); err == nil {
				filecontentStr = str
			}
			decodeBytes := []byte("")

			decodeBytes, err = b64.StdEncoding.DecodeString(filecontentStr)
			if err != nil {
				err = logs.Err(ctx, err, "")
				return
			}

			var tempFile *os.File
			fn, _ := uuid.NewUUID()
			tempFile, err = ioutil.TempFile(os.TempDir(), fn.String())
			defer tempFile.Close()
			if err != nil {
				err = logs.Err(ctx, fmt.Errorf("Temp file creation failed : %w", err), "")
				return
			}
			//TODO - hard coded pdf content type to be removed
			fileWriter, err := createFormFile(multipartWriter, "application/pdf", filevarnameStr, filenameStr)
			if err != nil {
				return nil, nil, err
			}
			_, err = io.Copy(fileWriter, bytes.NewBuffer(decodeBytes))
			if err != nil {
				err = logs.Err(ctx, err, "")
				return nil, nil, err
			}
		}
		multipartWriter.Close()
		request.Body = io.NopCloser(&reqBody)
		request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		request.Header.Set("Content-Length", strconv.Itoa(reqBody.Len()))
		request.ContentLength = int64(reqBody.Len())
	}
	return
}

func processMultipart(ctx context.Context, reqContentType string, request *http.Request, formDataRemove []string, formData map[string]interface{}, fileData []FilePart) (varsFormData map[string]interface{}, varsFormDataKeyArray []string, varsFileData []FilePart, err error) {
	logs.WithContext(ctx).Debug("processMultipart - Start")
	varsFormData = make(map[string]interface{})
	if reqContentType == encodedForm || reqContentType == multiPartForm {
		var reqBody bytes.Buffer
		multipartWriter := multipart.NewWriter(&reqBody)
		multiPart, mErr := request.MultipartReader()
		requestHasMultipart := true
		if mErr != nil {
			//err = mErr
			err = logs.Err(ctx, fmt.Errorf("error from request.MultipartReader() : %w", mErr), "")
			requestHasMultipart = false
		}
		i := 0
		if requestHasMultipart {
			for {
				i++
				removeFlag := false
				part, errPart := multiPart.NextRawPart()
				if errPart == io.EOF {
					err = logs.Err(ctx, fmt.Errorf("breaking becuase of eof"), "")
					err = nil // this is to avoid the error being returned to the caller
					break
				}
				if formDataRemove != nil {
					for _, v := range formDataRemove {
						if part.FormName() == v {
							removeFlag = true
							break
						}
					}
				}
				if !removeFlag && part != nil {
					if part.FileName() != "" {
						fileWriter, err := createFormFileCopy(multipartWriter, part)
						if err != nil {
							return nil, nil, nil, err
						}
						buf := new(bytes.Buffer)
						_, err = buf.ReadFrom(part)
						if err != nil {
							err = logs.Err(ctx, err, "")
							return nil, nil, nil, err
						}
						_, ferr := fileWriter.Write(buf.Bytes())
						if ferr != nil {
							err = logs.Err(ctx, ferr, "")
							return nil, nil, nil, err
						}
						//fk := fmt.Sprint("file_", i)
						formName := strings.Replace(strings.Replace(part.FormName(), "[", "", -1), "]", "", -1)
						filePart := FilePart{}
						filePart.FileName = part.FileName()
						filePart.FileVarName = formName
						filePart.FileContent = b64.StdEncoding.EncodeToString(buf.Bytes())

						varsFileData = append(varsFileData, filePart)
						//varsFormData[fk] = b64.StdEncoding.EncodeToString(buf.Bytes())
						_, err = io.Copy(fileWriter, part)
						if err != nil {
							err = logs.Err(ctx, err, "")
							return nil, nil, nil, err
						}

					} else {
						buf := new(bytes.Buffer)
						buf.ReadFrom(part)
						fieldWriter, err := multipartWriter.CreateFormField(part.FormName())
						if err != nil {
							err = logs.Err(ctx, err, "")
							return nil, nil, nil, err
						}
						_, err = fieldWriter.Write(buf.Bytes())
						if err != nil {
							err = logs.Err(ctx, err, "")
							return nil, nil, nil, err
						}
						formName := strings.Replace(strings.Replace(part.FormName(), "[", "", -1), "]", "", -1)
						varsFormData[formName] = buf.String()
						varsFormDataKeyArray = append(varsFormDataKeyArray, formName)
					}
				} else {
					//break the for loop
					break
				}
			}

		}
		for fk, fd := range formData {
			toIgnore := false
			for _, k := range varsFormDataKeyArray {
				if k == fk {
					toIgnore = true
					break
				}
			}
			if !toIgnore {
				fieldWriter, err := multipartWriter.CreateFormField(fk)
				if err != nil {
					err = logs.Err(ctx, err, "")
					return nil, nil, nil, err
				}
				_, err = fieldWriter.Write([]byte(fd.(string)))
				if err != nil {
					err = logs.Err(ctx, err, "")
					return nil, nil, nil, err
				}
				varsFormData[fk] = fd
				varsFormDataKeyArray = append(varsFormDataKeyArray, fk)
			}
		}
		for _, fl := range fileData {
			filenameStr := string(fl.FileName)
			if str, err := strconv.Unquote(filenameStr); err == nil {
				filenameStr = str
			}
			filevarnameStr := string(fl.FileVarName)
			if str, err := strconv.Unquote(filevarnameStr); err == nil {
				filevarnameStr = str
			}
			filecontentStr := string(fl.FileContent)
			str := ""
			if str, err = strconv.Unquote(filecontentStr); err == nil {
				filecontentStr = str
			}
			decodeBytes := []byte("")
			decodeBytes, err = b64.StdEncoding.DecodeString(filecontentStr)
			if err != nil {
				err = logs.Err(ctx, err, "")
				return
			}

			var tempFile *os.File
			fn, _ := uuid.NewUUID()

			tempFile, err = ioutil.TempFile(os.TempDir(), fn.String())
			defer tempFile.Close()
			if err != nil {
				err = logs.Err(ctx, fmt.Errorf("Temp file creation failed : %w", err), "")
				return
			}
			//TODO - hard coded pdf content type to be removed

			fileWriter, err := createFormFile(multipartWriter, "application/pdf", filevarnameStr, filenameStr)
			if err != nil {
				return nil, nil, nil, err
			}
			_, err = io.Copy(fileWriter, bytes.NewBuffer(decodeBytes))
			if err != nil {
				err = logs.Err(ctx, err, "")
				return nil, nil, nil, err
			}
		}
		multipartWriter.Close()
		request.Body = io.NopCloser(&reqBody)
		request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		request.Header.Set("Content-Length", strconv.Itoa(reqBody.Len()))
		request.ContentLength = int64(reqBody.Len())
		//defer request.Body.Close()
	}
	return
}

func processParams(ctx context.Context, request *http.Request, queryParamsRemove []string, queryParams []Headers, vars *TemplateVars, reqVars map[string]*TemplateVars, resVars map[string]*TemplateVars, tokenHeaderKey string) (err error, errs []string) {
	logs.WithContext(ctx).Debug("processParams - Start")
	pvars := &FuncTemplateVars{}
	pvars.Vars = vars
	pvars.ReqVars = reqVars
	pvars.ResVars = resVars
	params := request.URL.Query()
	for _, p := range queryParams {
		if p.IsTemplate {
			valueBytes, terr := processTemplate(ctx, p.Key, p.Value, pvars, "string", tokenHeaderKey)
			if terr != nil {
				errs = append(errs, terr.Error())
			}
			valueStr, uerr := strconv.Unquote(string(valueBytes))
			if uerr != nil {
				err = logs.Err(ctx, uerr, "")
				return
			}
			params.Set(p.Key, valueStr)
			vars.Params[p.Key] = valueStr
		} else {
			params.Set(p.Key, p.Value)
			vars.Params[p.Key] = p.Value
		}
	}

	if queryParamsRemove != nil {
		for _, v := range queryParamsRemove {
			params.Del(v)
		}
	}
	request.URL.RawQuery = params.Encode()
	return
}

func processHeaderTemplates(ctx context.Context, request *http.Request, headersToRemove []string, headers []Headers, reqVarsLoaded bool, vars *TemplateVars, tokenSecretKey string, reqVars map[string]*TemplateVars, resVars map[string]*TemplateVars) (err error, errs []string) {
	logs.WithContext(ctx).Debug("processHeaderTemplates - Start")
	//TODO remove reqVarsLoaded unused parameter
	if headersToRemove != nil {
		for _, v := range headersToRemove {
			request.Header.Del(v)
		}
	}

	for _, h := range headers {
		if h.IsTemplate {

			//TODO check if commenting below block as an impact elsewhere as we are loading vars only once before transform request is called.
			/*
				if !reqVarsLoaded {
					err = loadRequestVars(vars, request)
					if err != nil {
						return
					}
					reqVarsLoaded = true
				}
			*/

			fvars := &FuncTemplateVars{}
			fvars.Vars = vars
			fvars.ResVars = resVars
			fvars.ReqVars = reqVars

			koutputStr := h.Key
			if strings.HasPrefix(h.Key, "{{") {
				koutput, kErr := processTemplate(ctx, "headerkey", h.Key, fvars, "string", tokenSecretKey)
				if kErr != nil {
					errs = append(errs, kErr.Error())
				}
				koutputStr = string(koutput)
				if str, err := strconv.Unquote(koutputStr); err == nil {
					koutputStr = str
				} else {
					_ = logs.Err(ctx, err, "")
				}
			}

			output, vErr := processTemplate(ctx, h.Key, h.Value, fvars, "string", tokenSecretKey)
			if vErr != nil {
				errs = append(errs, vErr.Error())
			}
			outputStr := string(output)
			if str, err := strconv.Unquote(outputStr); err == nil {
				outputStr = str
			}
			request.Header.Set(koutputStr, outputStr)
		}
	}
	return
}

func clubResponses(ctx context.Context, responses []*http.Response, errs []error) (response *http.Response, err error) {
	logs.WithContext(ctx).Debug("clubResponses - Start")

	if len(responses) > 0 {
		if responses[0] != nil {
			reqContentType := strings.Split(responses[0].Header.Get("Content-type"), ";")[0]
			if reqContentType != applicationjson {
				response = responses[0]
				//if len(trResVars) == 1 {
				//	trResVar = trResVars[0]
				//}
				if len(errs) == 1 {
					err = errs[0]
				}
				return
			}
		}
	}
	var errMsg []string
	errorFound := false
	for _, e := range errs {
		if e != nil {
			errorFound = true
			errMsg = append(errMsg, e.Error())
		} else {
			errMsg = append(errMsg, "-")
		}
	}
	/*
		//trResVar - copying all attributes of first element as it will be same except response body

		trResVar = &TemplateVars{}
		if trResVars != nil {
			if trResVars[0] != nil {
				trResVar.LoopVars = trResVars[0].LoopVars
				trResVar.Vars = trResVars[0].Vars
				trResVar.FormData = trResVars[0].FormData
				trResVar.Params = trResVars[0].Params
				trResVar.Headers = trResVars[0].Headers
				trResVar.FormDataKeyArray = trResVars[0].FormDataKeyArray
				trResVar.Token = trResVars[0].Token
				var resBody []interface{}
				for _, tr := range trResVars {
					resBody = append(resBody, tr.Body)
				}
				trResVar.Body = resBody
			}
		}
	*/
	if errorFound {
		return nil, errors.New(strings.Join(errMsg, " , "))
	}

	defer func(resps []*http.Response) {
		for _, resp := range resps {
			resp.Body.Close()
		}
	}(responses)
	respHeader := http.Header{}
	newR := &http.Request{}
	if len(responses) > 0 {
		if responses[0] != nil {
			newR = responses[0].Request
			for k, v := range responses[0].Header {
				if k != "Content-Length" { //TODO is this needed? || route.LoopVariable == ""
					for _, h := range v {
						respHeader.Set(k, h)
					}
				}
			}
		}
	}
	var rJsonArray []interface{}
	statusCode := http.StatusOK
	for _, rp := range responses {
		if rp != nil {
			if rp.Body != nil {
				var rJson interface{}
				reqContentTypeCheck := strings.Split(rp.Header.Get("Content-type"), ";")[0]
				if reqContentTypeCheck == applicationjson {
					err = json.NewDecoder(rp.Body).Decode(&rJson)
					if err != nil {
						err = logs.Err(ctx, err, "")
						return nil, err
					}
					rJson = stripSingleElement(rJson)
				} else {
					rJson, err = io.ReadAll(rp.Body)
					if err != nil {
						err = logs.Err(ctx, err, "")
						return nil, err
					}
				}
				rJsonArray = append(rJsonArray, rJson)
				//this will set status code of last response which will be passed
				statusCode = rp.StatusCode
			}
		}
	}

	contentLength := 0

	rJsonArrayBytes, eee := json.Marshal(stripSingleElement(rJsonArray))
	if eee != nil {
		err = logs.Err(ctx, eee, "")
		return nil, err
	}
	if len(rJsonArray) == 0 {
		respHeader.Set("Content-Type", applicationjson)
		rJsonArrayBytes = []byte("{}")
	}
	contentLength = len(rJsonArrayBytes)
	respHeader.Set("Content-Length", fmt.Sprint(contentLength))
	response = &http.Response{
		StatusCode:    statusCode,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          io.NopCloser(bytes.NewBuffer(rJsonArrayBytes)),
		ContentLength: int64(contentLength),
		Request:       newR,
		Header:        respHeader,
	}
	return
}

func stripSingleElement(obj interface{}) interface{} {
	if objArray, ok := obj.([]interface{}); !ok {
		return obj
	} else if len(objArray) == 1 {
		return objArray[0]
	} else {
		return obj
	}
}

func protect(f func()) {
	defer func() {
		if err := recover(); err != nil {
			logs.Logger.Panic(fmt.Sprintf("Recovered: %v", err))
		}
	}()

	f()
}

func errorResponse(ctx context.Context, errMsg string, request *http.Request) (response *http.Response) {
	logs.WithContext(ctx).Debug("errorResponse - Start")
	errRespHeader := http.Header{}
	errRespHeader.Set("Content-Type", "application/json")
	response = &http.Response{
		StatusCode:    http.StatusBadRequest,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          io.NopCloser(bytes.NewBufferString(errMsg)),
		ContentLength: int64(len(errMsg)),
		Request:       request,
		Header:        errRespHeader,
	}
	return
}

var varsMutex sync.RWMutex

func cloneInterface(ctx context.Context, i interface{}) (iClone interface{}, err error) {
	logs.WithContext(ctx).Debug("cloneInterface - Start")

	varsMutex.RLock()
	defer varsMutex.RUnlock()

	iBytes, err := json.Marshal(i)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return
	}
	iCloneI := reflect.New(reflect.TypeOf(i))
	err = json.Unmarshal(iBytes, iCloneI.Interface())
	if err != nil {
		err = logs.Err(ctx, err, "")
		return
	}
	return iCloneI.Elem().Interface(), nil
}

func safeCloneVarsMap(ctx context.Context, vars map[string]*TemplateVars) (map[string]*TemplateVars, error) {
	varsMutex.RLock()
	defer varsMutex.RUnlock()

	clonedMap := make(map[string]*TemplateVars)
	for k, v := range vars {
		if v != nil {
			iBytes, err := json.Marshal(v)
			if err != nil {
				return nil, logs.Err(ctx, err, "")
			}
			var clonedV *TemplateVars
			err = json.Unmarshal(iBytes, &clonedV)
			if err != nil {
				return nil, logs.Err(ctx, err, "")
			}
			clonedMap[k] = clonedV
		} else {
			clonedMap[k] = nil
		}
	}
	return clonedMap, nil
}

func safeSetVar(vars map[string]*TemplateVars, key string, value *TemplateVars) {
	varsMutex.Lock()
	defer varsMutex.Unlock()
	vars[key] = value
}

func safeGetVar(vars map[string]*TemplateVars, key string) (*TemplateVars, bool) {
	varsMutex.RLock()
	defer varsMutex.RUnlock()
	val, ok := vars[key]
	return val, ok
}

func safeBatchSetVars(vars map[string]*TemplateVars, updates map[string]*TemplateVars) {
	varsMutex.Lock()
	defer varsMutex.Unlock()
	for k, v := range updates {
		vars[k] = v
	}
}

func removeFieldsFromTemplateVars(ctx context.Context, fields []string, vars map[string]*TemplateVars) (err error) {
	if fields != nil {
		for _, f := range fields {
			for k, _ := range vars {
				//Body
				if vars[k].Body != nil {
					if bodyMap, bodyMapOk := vars[k].Body.(map[string]interface{}); bodyMapOk {
						for kk, _ := range bodyMap {
							if kk == f {
								bodyMap[kk] = nil
							}
						}
						vars[k].Body = bodyMap
					}
				}
				//OrgBody
				if vars[k].OrgBody != nil {
					if bodyMap, bodyMapOk := vars[k].OrgBody.(map[string]interface{}); bodyMapOk {
						for kk, _ := range bodyMap {
							if kk == f {
								bodyMap[kk] = nil
							}
						}
						vars[k].OrgBody = bodyMap
					}
				}

				//Body
				if vars[k].Vars != nil {
					if vars[k].Vars["Body"] != nil {
						if bodyMap, bodyMapOk := vars[k].Vars["Body"].(map[string]interface{}); bodyMapOk {
							for kk, _ := range bodyMap {
								if kk == f {
									bodyMap[kk] = nil
								}
							}
							vars[k].Vars["Body"] = bodyMap
						}
					}

					if vars[k].Vars["OrgBody"] != nil {
						if bodyMap, bodyMapOk := vars[k].Vars["OrgBody"].(map[string]interface{}); bodyMapOk {
							for kk, _ := range bodyMap {
								if kk == f {
									bodyMap[kk] = nil
								}
							}
							vars[k].Vars["OrgBody"] = bodyMap
						}
					}

				}

			}
		}
	}
	return
}

func responseBodyForError(ctx context.Context, response *http.Response) string {
	if response == nil || response.Body == nil {
		return ""
	}
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return ""
	}
	response.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	if len(bodyBytes) > 2000 {
		return string(bodyBytes[:2000])
	}
	return string(bodyBytes)
}
