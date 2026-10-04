package eru_utils

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	httpurl "net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	models "github.com/eru-os/eru/eru-models"
	"github.com/google/go-cmp/cmp"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	encodedForm     = "application/x-www-form-urlencoded"
	multiPartForm   = "multipart/form-data"
	applicationJson = "application/json"
)

var sharedHttpTransport = func() *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	timeout := 60 * time.Second
	if v := os.Getenv("HTTP_RESPONSE_HEADER_TIMEOUT_SEC"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
		}
	}
	tr.ResponseHeaderTimeout = timeout
	return tr
}()

/* var httpClient = http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
} */

func getAttr(ctx context.Context, obj interface{}, fieldName string) reflect.Value {
	logs.WithContext(ctx).Debug("getAttr - Start")
	pointToStruct := reflect.ValueOf(obj) // addressable
	curStruct := pointToStruct.Elem()
	if curStruct.Kind() != reflect.Struct {
		logs.WithContext(ctx).Error("not a struct")
	}
	curField := curStruct.FieldByName(fieldName) // type: reflect.Value
	if !curField.IsValid() {
		logs.WithContext(ctx).Error(fmt.Sprint(("not found:" + fieldName)))
	}
	return curField
}

func SetStructValue(ctx context.Context, obj interface{}, propName string, propValue interface{}) {
	logs.WithContext(ctx).Debug("SetStructValue - Start")
	v := getAttr(ctx, obj, propName)
	v.Set(reflect.ValueOf(propValue))
}

/*func GetSha512(s string) string {
	h := sha512.New()
	h.Write([]byte(s))
	sha := base64.URLEncoding.EncodeToString(h.Sum(nil))
	return sha
}*/

func ValidateStruct(ctx context.Context, s interface{}, parentKey string) error {
	logs.WithContext(ctx).Debug("ValidateStruct - Start")
	if parentKey != "" {
		parentKey = parentKey + "."
	}
	f := reflect.ValueOf(s)
	if f.Type().Kind().String() == "ptr" {
		f = reflect.Indirect(reflect.ValueOf(s))
	}
	var errs []string
	for i := 0; i < f.NumField(); i++ {
		isError := false
		isRequired := false
		isOptional := false
		projectTags := f.Type().Field(i).Tag.Get("eru")
		if strings.Contains(projectTags, "required") {
			isRequired = true
			if f.Field(i).IsZero() && f.Field(i).Kind() != reflect.Bool {
				errs = append(errs, fmt.Sprint(parentKey, f.Type().Field(i).Name))
				isError = true
			}
		}
		if strings.Contains(projectTags, "optional") {
			isOptional = true
		}
		if !isError && !isOptional {
			switch f.Field(i).Kind().String() {
			case "struct":
				if f.Field(i).CanInterface() {
					e := ValidateStruct(ctx, f.Field(i).Interface(), fmt.Sprint(parentKey, f.Type().Field(i).Name))
					if e != nil {
						errs = append(errs, e.Error())
					}
				} else {
					logs.WithContext(ctx).Error(fmt.Sprintf("field %s is not interface", f.Type().Field(i).Name))
				}
			case "slice":
				ff := f.Field(i)
				if ff.Len() == 0 && isRequired {
					errs = append(errs, fmt.Sprint(parentKey, f.Type().Field(i).Name))
				} else {
					if ff.Len() > 0 {
						if ff.Index(0).Kind().String() == "struct" || ff.Index(0).Kind().String() == "slice" {
							for ii := 0; ii < ff.Len(); ii++ {
								e := ValidateStruct(ctx, ff.Index(ii).Interface(), fmt.Sprint(parentKey, f.Type().Field(i).Name, "[", ii, "]"))
								if e != nil {
									errs = append(errs, e.Error())
								}
							}
						}
					}
				}
			default:
				//do nothing
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	err := errors.New(strings.Join(errs, " , "))
	logs.WithContext(ctx).Error(err.Error())
	return err
}

func GetArrayPosition(s []string, value string) int {
	for p, v := range s {
		if v == value {
			return p
		}
	}
	return -1
}

func ReplaceUnderscoresWithDots(str string) string {
	return strings.Replace(strings.Replace(str, "___", ".", 1), "__", ".", 1)
}

func PrintResponseBody(ctx context.Context, response *http.Response, msg string) {
	logs.WithContext(ctx).Debug("PrintResponseBody - Start")
	logs.WithContext(ctx).Info(msg)
	if response != nil {
		if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
			return
		}
		if strings.HasPrefix(response.Header.Get("Content-Type"), "image/") {
			logs.WithContext(ctx).Info(fmt.Sprint("skipped printing ", response.Header.Get("Content-Type"), " response body of ", response.ContentLength, " bytes"))
			return
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		if response.Request != nil {
			logs.WithContext(ctx).Info(fmt.Sprint(response.Request.URL))
		}
		cl, _ := strconv.Atoi(response.Header.Get("Content-Length"))
		if cl > 1000 || len(string(body)) > 1000 {
			logs.WithContext(ctx).Info(string(body)[0:1000])
		} else {
			logs.WithContext(ctx).Info(string(body))
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
	} else {
		logs.WithContext(ctx).Info("response is nil")

	}
}

func PrintRequestBody(ctx context.Context, request *http.Request, msg string) {
	logs.WithContext(ctx).Debug("PrintRequestBody - Start")
	logs.WithContext(ctx).Info(msg)
	logs.WithContext(ctx).Info(fmt.Sprintf("request.URL: %+v", request.URL))
	logs.WithContext(ctx).Info(fmt.Sprintf("request.URL: %+v", request.Header))

	if request != nil {
		if request.Body != nil {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}

			logs.WithContext(ctx).Info(fmt.Sprint(request.URL))
			cl, _ := strconv.Atoi(request.Header.Get("Content-Length"))
			if cl > 1000 || len(string(body)) > 1000 {
				logs.WithContext(ctx).Info(string(body)[0:1000])
			} else {
				logs.WithContext(ctx).Info(string(body))
			}
			request.Body = io.NopCloser(bytes.NewReader(body))
		} else {
			logs.WithContext(ctx).Info("request body is nil")
		}
	} else {
		logs.WithContext(ctx).Info("request is nil")
	}
}

func CallParallelHttp(ctx context.Context, method string, url string, headers http.Header, formData map[string]string, reqCookies []*http.Cookie, params map[string]string, postBody interface{}, rc chan *http.Response) (err error) {
	logs.WithContext(ctx).Debug("CallParallelHttp - Start")
	resp, err := callHttp(ctx, method, url, headers, formData, reqCookies, params, postBody)
	if err == nil {
		rc <- resp
	}
	return err
}

func ExecuteParallelHttp(ctx context.Context, req *http.Request, rc chan *http.Response) (err error) {
	logs.WithContext(ctx).Debug("ExecuteParallelHttp - Start")
	resp, err := ExecuteHttp(ctx, req)
	if err == nil {
		rc <- resp
	}
	return err
}

func ExecuteHttp(ctx context.Context, req *http.Request) (resp *http.Response, err error) {
	logs.WithContext(ctx).Debug("ExecuteHttp - Start")
	//logs.WithContext(ctx).Info(fmt.Sprintf("ctx: %+v", ctx))

	req = req.WithContext(ctx)
	requestId := ctx.Value("request_id")
	if requestId != nil {
		req.Header.Set("request_id", requestId.(string))
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("req.URL.Host: %+v, req.Host: %+v", req.URL.Host, req.Host))
	req.Host = req.URL.Host
	//req.Header.Add("Host", req.URL.Host)

	/*
			host := req.URL.Host
			ips, err := net.LookupIP(host)
			if err != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("DNS lookup failed: %v", err))
			} else {
				logs.WithContext(ctx).Info(fmt.Sprintf("DNS resolution for %s: %v", host, ips))
			}

			 cmd := exec.Command("traceroute", "-n", host)
		    output, err := cmd.CombinedOutput()
		    if err != nil {
		        logs.WithContext(ctx).Error(fmt.Sprintf("Traceroute failed: %v", err))
		    } else {
		        logs.WithContext(ctx).Info(fmt.Sprintf("Network path to %s:\n%s", host, string(output)))
		    }

			transport := &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					start := time.Now()
					logs.WithContext(ctx).Info(fmt.Sprintf("Attempting connection to %s at %v", addr, start))

					dialer := &net.Dialer{
						Timeout:   30 * time.Second,
						KeepAlive: 30 * time.Second,
					}

					conn, err := dialer.DialContext(ctx, network, addr)
					if err != nil {
						logs.WithContext(ctx).Error(fmt.Sprintf("Connection failed to %s after %v: %v",
							addr, time.Since(start), err))
						return nil, err
					}

					logs.WithContext(ctx).Info(fmt.Sprintf("Connection established to %s in %v",
						addr, time.Since(start)))

					// Log connection details
					if tcpConn, ok := conn.(*net.TCPConn); ok {
						localAddr := tcpConn.LocalAddr().String()
						remoteAddr := tcpConn.RemoteAddr().String()
						logs.WithContext(ctx).Info(fmt.Sprintf("TCP Connection: Local=%s, Remote=%s",
							localAddr, remoteAddr))
					}

					return conn, nil
				},
			} */
	/* tr := &http.Transport{
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	} */
	PrintRequestBody(ctx, req, "printing request just before utils.ExecuteHttp")
	//Transport: otelhttp.NewTransport(http.DefaultTransport),
	/* client := &http.Client{
		Transport: http.DefaultTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err = client.Do(req) */

	//resp, err = httpClient.Do(req)
	//for _, c := range req.Cookies() {
	//	logs.WithContext(ctx).Info(c.String())
	//}
	logs.WithContext(ctx).Info("before HTTPClientTransporter")
	resp, err = HTTPClientTransporter(sharedHttpTransport).RoundTrip(req)
	logs.WithContext(ctx).Info("after HTTPClientTransporter")
	logs.WithContext(ctx).Info(fmt.Sprintf("resp: %+v", resp))
	//resp, err = otelhttp.NewTransport(http.DefaultTransport).RoundTrip(req)

	//resp, err = http.DefaultTransport.RoundTrip(req)
	/* client := &http.Client{
		Transport: HTTPClientTransporter(http.DefaultTransport),
		//Transport: transport,
		Timeout:   300 * time.Second, // Add timeout
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Keep your existing redirect policy
		},
	} */

	//resp, err = client.Do(req)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	PrintResponseBody(ctx, resp, "printing response immediately after utils.ExecuteHttp")

	allowedOriginsI := ctx.Value("allowed_origins")
	originI := ctx.Value("origin")

	allowedOrigins := ""
	if allowedOriginsI != nil {
		allowedOrigins = allowedOriginsI.(string)
	}

	origin := ""
	if originI != nil {
		origin = originI.(string)
	}
	if req.Header.Get("Origin") == "" && origin != "" && allowedOrigins != "" {
		logs.WithContext(ctx).Info(fmt.Sprint("setting cors headers as origin is blank"))
		envOrigin := strings.Split(allowedOrigins, ",")
		for _, o := range envOrigin {
			oo := strings.Replace(o, "*.", "", -1)
			if strings.Contains(origin, oo) {
				resp.Header.Set("Access-Control-Allow-Origin", origin)
				resp.Header.Set("Access-Control-Allow-Credentials", "true")
				resp.Header.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
				resp.Header.Set("Access-Control-Expose-Headers", "Authorization, Content-Type")
				break
			}
		}
	}
	return
}

func HTTPClientTransporter(rt http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(rt)
}

func CreateHttpClientWithTLS(ctx context.Context, clientCertB64 string, clientKeyB64 string, timeout time.Duration) (*http.Client, error) {
	tlsCfg := &tls.Config{}
	if clientCertB64 != "" && clientKeyB64 != "" {
		clientKeyBytes, err := base64.StdEncoding.DecodeString(clientKeyB64)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("error decoding client key: %v", err))
			return nil, fmt.Errorf("error decoding client key: %w", err)
		}
		clientCertBytes, err := base64.StdEncoding.DecodeString(clientCertB64)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("error decoding client certificate: %v", err))
			return nil, fmt.Errorf("error decoding client certificate: %w", err)
		}
		cert, err := tls.X509KeyPair(clientCertBytes, clientKeyBytes)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("error loading client certificate: %v", err))
			return nil, fmt.Errorf("error loading client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{TLSClientConfig: tlsCfg}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Transport: HTTPClientTransporter(transport), Timeout: timeout}
	return client, nil
}

func callHttp(ctx context.Context, method string, url string, headers http.Header, formData map[string]string, reqCookies []*http.Cookie, params map[string]string, postBody interface{}) (resp *http.Response, err error) {
	logs.WithContext(ctx).Debug("callHttp - Start")
	req := &http.Request{}
	contentType := headers.Get("Content-Type")
	if contentType == "application/x-ndjson" {
		if postBodyBytes, postBodyBytesOk := postBody.([]byte); postBodyBytesOk {
			req, err = http.NewRequest(method, url, bytes.NewBuffer(postBodyBytes))
		} else {
			return nil, errors.New("postBody is not a []byte")
		}
	} else if contentType == "text/plain" {
		if postBodyBytes, ok := postBody.([]byte); ok {
			req, err = http.NewRequest(method, url, bytes.NewBuffer(postBodyBytes))
		} else if postBodyStr, ok := postBody.(string); ok {
			req, err = http.NewRequest(method, url, bytes.NewBufferString(postBodyStr))
		} else {
			return nil, errors.New("postBody must be string or []byte for text/plain content type")
		}
	} else if postBody != nil {
		reqBody, reqBodyerr := json.Marshal(postBody)
		if reqBodyerr != nil {
			logs.WithContext(ctx).Error(reqBodyerr.Error())
			return nil, reqBodyerr
		}
		req, err = http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	for _, v := range reqCookies {
		req.AddCookie(v)
	}
	for k, v := range headers {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}

	reqParams := req.URL.Query()
	for k, v := range params {
		reqParams.Add(k, v)
	}
	req.URL.RawQuery = reqParams.Encode()
	reqContentType := strings.Split(req.Header.Get("Content-type"), ";")[0]
	if reqContentType == multiPartForm {
		var reqBodyNew bytes.Buffer
		multipartWriter := multipart.NewWriter(&reqBodyNew)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
		for fk, fd := range formData {
			fieldWriter, err := multipartWriter.CreateFormField(fk)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, err
			}
			_, err = fieldWriter.Write([]byte(fd))
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, err
			}
		}
		multipartWriter.Close()
		req.Body = io.NopCloser(&reqBodyNew)
		if reqContentType == multiPartForm {
			req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		}
		req.Header.Set("Content-Length", strconv.Itoa(reqBodyNew.Len()))
		req.ContentLength = int64(reqBodyNew.Len())
	}
	if reqContentType == encodedForm {
		data := httpurl.Values{}
		var reqBodyNew bytes.Buffer
		for fk, fd := range formData {
			data.Add(fk, fd)
		}
		encodedData := data.Encode()
		reqBodyNew.WriteString(encodedData)
		req.Body = io.NopCloser(&reqBodyNew)
		req.Header.Set("Content-Length", strconv.Itoa(len(data.Encode())))
		req.ContentLength = int64(len(data.Encode()))
	}
	//else {
	//	req.Header.Set("Content-Length", strconv.Itoa(bytes.NewReader(reqBody).Len()))
	//}
	return ExecuteHttp(ctx, req)
}

func CallHttp(ctx context.Context, method string, url string, headers http.Header, formData map[string]string, reqCookies []*http.Cookie, params map[string]string, postBody interface{}) (res interface{}, respHeaders http.Header, respCookies []*http.Cookie, statusCode int, err error) {
	logs.WithContext(ctx).Debug("CallHttp - Start")
	resp, err := callHttp(ctx, method, url, headers, formData, reqCookies, params, postBody)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, nil, 0, err
	}
	statusCode = resp.StatusCode
	respHeaders = resp.Header
	respCookies = resp.Cookies()
	defer resp.Body.Close()
	//todo - check if below change from reqContentType to header.get breaks anything
	//todo - merge conflict - main had below first if commented
	contentType := strings.Split(headers.Get("Content-Type"), ";")[0]
	respcontentType := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	if resp.ContentLength > 0 || contentType == encodedForm || contentType == applicationJson {
		if respcontentType == applicationJson && resp.StatusCode != 204 {
			if err = json.NewDecoder(resp.Body).Decode(&res); err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, nil, nil, resp.StatusCode, err
			}
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}
			resBody := make(map[string]interface{})
			resBody["body"] = string(body)
			res = resBody
		}
	} else {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		resBody := make(map[string]interface{})
		resBody["body"] = string(body)
		res = resBody
	}
	if resp.StatusCode >= 400 {
		statusCode = resp.StatusCode
		resBytes, bytesErr := json.Marshal(res)
		if bytesErr != nil {
			logs.WithContext(ctx).Error(bytesErr.Error())
			return nil, nil, nil, statusCode, bytesErr
		}
		err = errors.New(string(resBytes))
		logs.WithContext(ctx).Error(err.Error())
		return nil, resp.Header, resp.Cookies(), statusCode, err
	}
	return
}

type FileData struct {
	FieldName   string
	FileName    string
	Content     []byte
	ContentType string
}

func createFormFilePart(w *multipart.Writer, fieldName string, fileName string, contentType string) (io.Writer, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileName))
	h.Set("Content-Type", contentType)
	return w.CreatePart(h)
}

func CallHttpWithFiles(ctx context.Context, method string, url string, headers http.Header, formData map[string]string, files []FileData, reqCookies []*http.Cookie, params map[string]string) (res interface{}, respHeaders http.Header, respCookies []*http.Cookie, statusCode int, err error) {
	logs.WithContext(ctx).Debug("CallHttpWithFiles - Start")

	var reqBody bytes.Buffer
	multipartWriter := multipart.NewWriter(&reqBody)

	for fk, fd := range formData {
		fieldWriter, fErr := multipartWriter.CreateFormField(fk)
		if fErr != nil {
			err = logs.Err(ctx, fErr, "failed to create form field")
			return nil, nil, nil, 0, err
		}
		_, fErr = fieldWriter.Write([]byte(fd))
		if fErr != nil {
			err = logs.Err(ctx, fErr, "failed to write form field")
			return nil, nil, nil, 0, err
		}
	}

	for _, f := range files {
		fileWriter, fErr := createFormFilePart(multipartWriter, f.FieldName, f.FileName, f.ContentType)
		if fErr != nil {
			err = logs.Err(ctx, fErr, "failed to create form file")
			return nil, nil, nil, 0, err
		}
		_, fErr = fileWriter.Write(f.Content)
		if fErr != nil {
			err = logs.Err(ctx, fErr, "failed to write form file")
			return nil, nil, nil, 0, err
		}
	}

	multipartWriter.Close()

	req, err := http.NewRequest(method, url, &reqBody)
	if err != nil {
		err = logs.Err(ctx, err, "failed to create request")
		return nil, nil, nil, 0, err
	}

	for _, v := range reqCookies {
		req.AddCookie(v)
	}
	for k, v := range headers {
		req.Header[k] = v
	}
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("Content-Length", strconv.Itoa(reqBody.Len()))
	req.ContentLength = int64(reqBody.Len())

	reqParams := req.URL.Query()
	for k, v := range params {
		reqParams.Add(k, v)
	}
	req.URL.RawQuery = reqParams.Encode()
	PrintRequestBody(ctx, req, "request from CallHttpWithFiles")
	resp, err := ExecuteHttp(ctx, req)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	statusCode = resp.StatusCode
	respHeaders = resp.Header
	respCookies = resp.Cookies()
	defer resp.Body.Close()

	respContentType := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	if respContentType == applicationJson {
		if decErr := json.NewDecoder(resp.Body).Decode(&res); decErr != nil {
			err = logs.Err(ctx, decErr, "failed to decode response")
			return nil, nil, nil, resp.StatusCode, err
		}
	} else {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			logs.WithContext(ctx).Error(readErr.Error())
		}
		resBody := make(map[string]interface{})
		resBody["body"] = string(body)
		res = resBody
	}

	if resp.StatusCode >= 400 {
		statusCode = resp.StatusCode
		resBytes, bytesErr := json.Marshal(res)
		if bytesErr != nil {
			return nil, nil, nil, statusCode, bytesErr
		}
		err = errors.New(string(resBytes))
		return nil, resp.Header, resp.Cookies(), statusCode, err
	}
	return
}

func callHttpWithTLS(ctx context.Context, method string, url string, headers http.Header, formData map[string]string, reqCookies []*http.Cookie, params map[string]string, postBody interface{}, clientCertB64 string, clientKeyB64 string, timeout time.Duration) (resp *http.Response, err error) {
	logs.WithContext(ctx).Debug("callHttpWithTLS - Start")
	req := &http.Request{}
	if headers.Get("Content-Type") == "application/x-ndjson" {
		if postBodyBytes, postBodyBytesOk := postBody.([]byte); postBodyBytesOk {
			req, err = http.NewRequest(method, url, bytes.NewBuffer(postBodyBytes))
		} else {
			return nil, errors.New("postBody is not a []byte")
		}
	} else if postBody != nil {
		reqBody, reqBodyerr := json.Marshal(postBody)
		if reqBodyerr != nil {
			logs.WithContext(ctx).Error(reqBodyerr.Error())
			return nil, reqBodyerr
		}
		req, err = http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	req = req.WithContext(ctx)
	req.Host = req.URL.Host

	for _, v := range reqCookies {
		req.AddCookie(v)
	}
	for k, v := range headers {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}

	reqParams := req.URL.Query()
	for k, v := range params {
		reqParams.Add(k, v)
	}
	req.URL.RawQuery = reqParams.Encode()
	reqContentType := strings.Split(req.Header.Get("Content-type"), ";")[0]
	if reqContentType == multiPartForm {
		var reqBodyNew bytes.Buffer
		multipartWriter := multipart.NewWriter(&reqBodyNew)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
		for fk, fd := range formData {
			fieldWriter, err := multipartWriter.CreateFormField(fk)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, err
			}
			_, err = fieldWriter.Write([]byte(fd))
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, err
			}
		}
		multipartWriter.Close()
		req.Body = io.NopCloser(&reqBodyNew)
		if reqContentType == multiPartForm {
			req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		}
		req.Header.Set("Content-Length", strconv.Itoa(reqBodyNew.Len()))
		req.ContentLength = int64(reqBodyNew.Len())
	}
	if reqContentType == encodedForm {
		data := httpurl.Values{}
		var reqBodyNew bytes.Buffer
		for fk, fd := range formData {
			data.Add(fk, fd)
		}
		encodedData := data.Encode()
		reqBodyNew.WriteString(encodedData)
		req.Body = io.NopCloser(&reqBodyNew)
		req.Header.Set("Content-Length", strconv.Itoa(len(data.Encode())))
		req.ContentLength = int64(len(data.Encode()))
	}

	PrintRequestBody(ctx, req, "printing request just before utils.callHttpWithTLS")

	client, err := CreateHttpClientWithTLS(ctx, clientCertB64, clientKeyB64, timeout)
	if err != nil {
		return nil, err
	}

	resp, err = client.Do(req)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	PrintResponseBody(ctx, resp, "printing response immediately after utils.callHttpWithTLS")

	allowedOriginsI := ctx.Value("allowed_origins")
	originI := ctx.Value("origin")

	allowedOrigins := ""
	if allowedOriginsI != nil {
		allowedOrigins = allowedOriginsI.(string)
	}

	origin := ""
	if originI != nil {
		origin = originI.(string)
	}
	if req.Header.Get("Origin") == "" && origin != "" && allowedOrigins != "" {
		logs.WithContext(ctx).Info(fmt.Sprint("setting cors headers as origin is blank"))
		envOrigin := strings.Split(allowedOrigins, ",")
		for _, o := range envOrigin {
			oo := strings.Replace(o, "*.", "", -1)
			if strings.Contains(origin, oo) {
				resp.Header.Set("Access-Control-Allow-Origin", origin)
				resp.Header.Set("Access-Control-Allow-Credentials", "true")
				resp.Header.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
				resp.Header.Set("Access-Control-Expose-Headers", "Authorization, Content-Type")
				break
			}
		}
	}
	return
}

func CallHttpWithTLS(ctx context.Context, method string, url string, headers http.Header, formData map[string]string, reqCookies []*http.Cookie, params map[string]string, postBody interface{}, clientCertB64 string, clientKeyB64 string, timeout time.Duration) (res interface{}, respHeaders http.Header, respCookies []*http.Cookie, statusCode int, err error) {
	logs.WithContext(ctx).Debug("CallHttpWithTLS - Start")
	resp, err := callHttpWithTLS(ctx, method, url, headers, formData, reqCookies, params, postBody, clientCertB64, clientKeyB64, timeout)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, nil, 0, err
	}
	statusCode = resp.StatusCode
	respHeaders = resp.Header
	respCookies = resp.Cookies()
	defer resp.Body.Close()
	contentType := strings.Split(headers.Get("Content-Type"), ";")[0]
	respcontentType := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	if resp.ContentLength > 0 || contentType == encodedForm || contentType == applicationJson {
		if respcontentType == applicationJson {
			if err = json.NewDecoder(resp.Body).Decode(&res); err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, nil, nil, resp.StatusCode, err
			}
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}
			resBody := make(map[string]interface{})
			resBody["body"] = string(body)
			res = resBody
		}
	} else {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		resBody := make(map[string]interface{})
		resBody["body"] = string(body)
		res = resBody
	}
	if resp.StatusCode >= 400 {
		statusCode = resp.StatusCode
		resBytes, bytesErr := json.Marshal(res)
		if bytesErr != nil {
			logs.WithContext(ctx).Error(bytesErr.Error())
			return nil, nil, nil, statusCode, bytesErr
		}
		err = errors.New(string(resBytes))
		logs.WithContext(ctx).Error(err.Error())
		return nil, resp.Header, resp.Cookies(), statusCode, err
	}
	return
}

func CsvToMap(ctx context.Context, csvData [][]string, lowerCaseHeader bool) (jsonData []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("CsvToMap - Start")
	charsToRemove := []string{"."}
	for j, _ := range csvData[0] {
		csvData[0][j] = regexp.MustCompile(`[^a-zA-Z0-9 ]+`).ReplaceAllString(csvData[0][j], "")
		if lowerCaseHeader {
			csvData[0][j] = strings.ToLower(csvData[0][j])
		}
		csvData[0][j] = strings.Replace(csvData[0][j], " ", "_", -1)
		for _, v := range charsToRemove {
			csvData[0][j] = strings.Replace(csvData[0][j], v, "", -1)
		}
	}
	for i, line := range csvData {
		if i > 0 {
			jsonMap := make(map[string]interface{})
			for j, field := range line {
				jsonMap[csvData[0][j]] = field
			}
			jsonData = append(jsonData, jsonMap)
		}
	}
	return
}

type DiffOutput struct {
	Path   string `json:"path"`
	AddStr string `json:"add_str"`
	DelStr string `json:"del_str"`
}

type DiffReporter struct {
	path    cmp.Path
	diffs   map[string]DiffOutput
	diffStr []string
}

func (r *DiffReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *DiffReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		vx, vy := r.path.Last().Values()
		do := DiffOutput{}
		path := fmt.Sprintf("%v ", r.path)
		//do.Path = fmt.Sprintf("%v", strings.Replace(r.path.Last().String(), "\"", "", -1))
		do.Path = fmt.Sprintf("%v", strings.Replace(r.path.GoString(), "\"", "", -1))
		do.AddStr = fmt.Sprintf("%+v", vy)
		do.DelStr = fmt.Sprintf("%+v", vx)
		if r.diffs == nil {
			r.diffs = make(map[string]DiffOutput)
		}
		r.diffs[path] = do
		r.diffStr = append(r.diffStr, fmt.Sprintf("%#v:\n\t-: %+v\n\t+: %+v\n", r.path, vx, vy))
	}
}

func (r *DiffReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *DiffReporter) String() string {
	return strings.Join(r.diffStr, "\n")
}

func (r *DiffReporter) Output() map[string]DiffOutput {
	return r.diffs
}

func CloneInterface(ctx context.Context, i interface{}) (iClone interface{}, err error) {
	logs.WithContext(ctx).Debug("cloneInterface - Start")
	iBytes, err := json.Marshal(i)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	iCloneI := reflect.New(reflect.TypeOf(i))
	err = json.Unmarshal(iBytes, iCloneI.Interface())
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return iCloneI.Elem().Interface(), nil
}

func ExecuteDbFetch(ctx context.Context, db *sqlx.DB, query models.Queries) (output []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ExecuteDbFetch - Start")
	if db == nil {
		logs.WithContext(ctx).Error("db connection is nil")
		return nil, errors.New("db connection is nil")
	}
	rows, err := db.Queryx(query.Query, query.Vals...)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	mapping := make(map[string]interface{})
	colsType, ee := rows.ColumnTypes()
	if ee != nil {
		return nil, ee
	}
	for rows.Next() {
		innerResultRow := make(map[string]interface{})
		ee = rows.MapScan(mapping)
		if ee != nil {
			return nil, ee
		}
		for _, colType := range colsType {
			if colType.DatabaseTypeName() == "NUMERIC" && mapping[colType.Name()] != nil {
				f := 0.0
				if reflect.TypeOf(mapping[colType.Name()]).String() == "[]uint8" {
					f, err = strconv.ParseFloat(string(mapping[colType.Name()].([]byte)), 64)
					mapping[colType.Name()] = f
				} else if reflect.TypeOf(mapping[colType.Name()]).String() == "float64" {
					f = mapping[colType.Name()].(float64)
					mapping[colType.Name()] = f
				}
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return nil, err
				}
			} else if (colType.DatabaseTypeName() == "JSONB" || colType.DatabaseTypeName() == "JSON") && mapping[colType.Name()] != nil {
				bytesToUnmarshal := mapping[colType.Name()].([]byte)
				var v map[string]interface{}
				err = json.Unmarshal(bytesToUnmarshal, &v)
				if err != nil {
					return nil, err
				}
				mapping[colType.Name()] = &v
			}
			innerResultRow[colType.Name()] = mapping[colType.Name()]
		}
		output = append(output, innerResultRow)
	}
	return
}

func ExecuteDbSave(ctx context.Context, db *sqlx.DB, queries []*models.Queries) (output [][]map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ExecuteDbSave - Start")
	if db == nil {
		logs.WithContext(ctx).Error("db connection is nil")
		return nil, errors.New("db connection is nil")
	}
	tx := db.MustBegin()
	for _, q := range queries {
		stmt, err := tx.PreparexContext(ctx, q.Query)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("Error in tx.PreparexContext : ", err.Error()))
			tx.Rollback()
			return nil, err
		}
		rw, err := stmt.QueryxContext(ctx, q.Vals...)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("Error in stmt.QueryxContext : ", err.Error()))
			tx.Rollback()
			return nil, err
		}
		var innerOutput []map[string]interface{}
		for rw.Rows.Next() {
			resDoc := make(map[string]interface{})
			err = rw.MapScan(resDoc)
			if err != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("Error in rw.MapScan : ", err.Error()))
				tx.Rollback()
				return nil, err
			}
			innerOutput = append(innerOutput, resDoc)
		}
		output = append(output, innerOutput)
	}
	err = tx.Commit()
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("Error in tx.Commit : ", err.Error()))
		tx.Rollback()
	}
	return
}

func ImplContains[T comparable](s []T, e T) bool {
	for _, v := range s {
		if v == e {
			return true
		}
	}
	return false
}

func ImplArrayContains[T comparable](s []T, e []T) bool {
	for _, v := range s {
		for _, vv := range e {
			if v == vv {
				return true
			}
		}
	}
	return false
}

func ImplArrayNotContains[T comparable](s []T, e []T) bool {
	for _, v := range s {
		for _, vv := range e {
			if v == vv {
				return false
			}
		}
	}
	return true
}

func ImplCompare[T comparable](s T, e T) bool {
	return s == e
}

func GetNestedFieldValue(ctx context.Context, data interface{}, fieldPath string) (interface{}, error) {
	logs.WithContext(ctx).Debug("GetNestedFieldValue - Start")
	fields := strings.Split(fieldPath, ".")
	val := reflect.ValueOf(data)
	for _, field := range fields {
		if val.Kind() == reflect.Map {
			val = val.MapIndex(reflect.ValueOf(field))
			if !val.IsValid() {
				err := errors.New("invalid value")
				logs.WithContext(ctx).Error(err.Error())
				return nil, nil
			}
			if val.Kind() == reflect.Interface {
				val = reflect.ValueOf(val.Interface())
			}
		} else {
			err := errors.New("not a map")
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
	}
	return val.Interface(), nil
}

func GetJsonSchemaObject(ctx context.Context, jsonSchema string) (eru_models.JSONSchema, error) {
	logs.WithContext(ctx).Debug("GetJsonSchemaObject - Start")
	logs.WithContext(ctx).Info(fmt.Sprint(jsonSchema))
	jsonSchemaMap := eru_models.JSONSchema{}
	err := json.Unmarshal([]byte(jsonSchema), &jsonSchemaMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return eru_models.JSONSchema{}, err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(jsonSchemaMap))
	return jsonSchemaMap, nil
}
func UniqueStrings(input []string) []string {
	uniqueMap := make(map[string]struct{})
	var uniqueList []string

	for _, str := range input {
		if _, exists := uniqueMap[str]; !exists {
			uniqueMap[str] = struct{}{}          // Add to map to track uniqueness
			uniqueList = append(uniqueList, str) // Add to the unique list
		}
	}

	return uniqueList
}

func ReplaceVariables(ctx context.Context, str string, vars map[string]interface{}) (resStr string) {
	logs.WithContext(ctx).Debug("ReplaceVariables - Start")
	resStr = str
	for k, v := range vars {
		resStr = strings.Replace(resStr, "$"+k, fmt.Sprint(v), -1)
	}
	return
}

func UnqotePlainText(ctx context.Context, response *http.Response) (responseNew *http.Response, err error) {
	logs.WithContext(ctx).Debug("UnqotePlainText - Start")
	if response != nil {
		if response.Header.Get("Content-Type") == "text/plain" {
			body, berr := io.ReadAll(response.Body)
			if berr != nil {
				err = logs.Err(ctx, fmt.Errorf("io.ReadAll error : %w", berr), "")
				return nil, err
			}
			bodyStr := string(body)
			bodyStr, err = strconv.Unquote(bodyStr)
			if err != nil {
				err = logs.Err(ctx, fmt.Errorf("strconv.Unquote error : %w", err), "")
				return nil, err
			}
			response.ContentLength = int64(len(bodyStr))
			response.Header.Set("Content-Length", fmt.Sprint(len(bodyStr)))
			response.Body = io.NopCloser(strings.NewReader(bodyStr))
		}
	} else {
		logs.WithContext(ctx).Info("response is nil")
	}
	return response, nil
}

func GenerateJSONSchema(ctx context.Context, data map[string]interface{}) eru_models.JSONSchema {
	logs.WithContext(ctx).Debug("GenerateJSONSchema - Start")
	schema := eru_models.JSONSchema{
		Type:       "object",
		Properties: make(map[string]eru_models.JSONSchema),
	}

	for key, value := range data {
		fieldSchema := eru_models.JSONSchema{}

		switch v := value.(type) {
		case map[string]interface{}:
			// Recursively handle nested objects
			fieldSchema = GenerateJSONSchema(ctx, v)
		case []interface{}:
			// Handle arrays
			fieldSchema.Type = "array"
			if len(v) > 0 {
				// Check first element to determine items schema
				switch firstElem := v[0].(type) {
				case map[string]interface{}:
					// If array contains objects, recursively generate schema
					itemsSchema := GenerateJSONSchema(ctx, firstElem)
					fieldSchema.Items = &itemsSchema
				default:
					// For primitive types in array
					itemsSchema := eru_models.JSONSchema{
						Type: getTypeFromValue(firstElem),
					}
					fieldSchema.Items = &itemsSchema
				}
			} else {
				// Empty array - use string as default type
				itemsSchema := eru_models.JSONSchema{
					Type: "string",
				}
				fieldSchema.Items = &itemsSchema
			}
		default:
			// Handle primitive types
			fieldSchema.Type = getTypeFromValue(v)
		}

		schema.Properties[key] = fieldSchema
	}

	return schema
}

func getTypeFromValue(value interface{}) string {
	switch value.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case int, int32, int64:
		return "integer"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "string" // Default to string for unknown types
	}
}

// Helper functions for safe type assertions
func GetStringField(m map[string]interface{}, key string) string {
	if val, exists := m[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
		_ = logs.Err(context.Background(), fmt.Errorf("GetStringField - %s is not a string ", key), "")
		return ""
	}
	_ = logs.Err(context.Background(), fmt.Errorf("GetStringField - %s not found ", key), "")
	return ""
}

func GetMapField(m map[string]interface{}, key string) map[string]interface{} {
	if val, exists := m[key]; exists {
		if val == nil {
			return make(map[string]interface{})
		}
		// Handle pointer to map
		if ptr, ok := val.(*map[string]interface{}); ok {
			if ptr == nil {
				return make(map[string]interface{})
			}
			return *ptr
		}
		// Handle direct map
		if mapVal, ok := val.(map[string]interface{}); ok {
			return mapVal
		}
		_ = logs.Err(context.Background(), fmt.Errorf("GetMapField - %s is not a map %+v", key, val), "")
		return make(map[string]interface{})
	}
	_ = logs.Err(context.Background(), fmt.Errorf("GetMapField - %s not found ", key), "")
	return make(map[string]interface{})
}

func GetCronStr(ctx context.Context, nextRun time.Time) string {
	cronStr := fmt.Sprintf("%d %d %d %d *", nextRun.Minute(), nextRun.Hour(), nextRun.Day(), nextRun.Month())
	logs.WithContext(ctx).Info(fmt.Sprint("Scheduling job to run at: ", nextRun.Format(time.RFC3339)))
	return cronStr
}

// GetServiceAddress automatically detects the service's IP address and port
// Returns a URL in the format "http://ip:port" or "https://ip:port"
func GetServiceAddress(ctx context.Context, port string) (string, error) {
	logs.WithContext(ctx).Debug("GetServiceAddress - Start")

	// Get local IP address
	localIP, err := getLocalIP()
	if err != nil {
		return "", fmt.Errorf("failed to get local IP: %w", err)
	}

	// Determine scheme (http or https)
	scheme := "http"
	if os.Getenv("HTTPS_ENABLED") == "true" {
		scheme = "https"
	}

	// Construct service address
	serviceAddress := fmt.Sprintf("%s://%s:%s", scheme, localIP, port)
	logs.WithContext(ctx).Info(fmt.Sprintf("Detected service address: %s", serviceAddress))

	return serviceAddress, nil
}

// getLocalIP returns the local IP address that can be reached from other machines
func getLocalIP() (string, error) {
	// Try to get the IP from environment variable first (useful for Docker/K8s)
	if envIP := os.Getenv("SERVICE_IP"); envIP != "" {
		return envIP, nil
	}

	// Try to get IP from Kubernetes downward API
	if k8sIP := os.Getenv("POD_IP"); k8sIP != "" {
		return k8sIP, nil
	}

	// Try to get IP from Docker environment
	if dockerIP := os.Getenv("HOST_IP"); dockerIP != "" {
		return dockerIP, nil
	}

	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	// Look for a non-loopback interface with a valid IP
	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				// Skip IPv6 and loopback addresses
				if v.IP.To4() != nil && !v.IP.IsLoopback() {
					return v.IP.String(), nil
				}
			case *net.IPAddr:
				// Skip IPv6 and loopback addresses
				if v.IP.To4() != nil && !v.IP.IsLoopback() {
					return v.IP.String(), nil
				}
			}
		}
	}

	// Fallback to localhost if no external IP found
	return "localhost", nil
}

func StructToJSONSchema(t reflect.Type, seenFields []string) eru_models.JSONSchema {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := eru_models.JSONSchema{
		Type:       "object",
		Properties: make(map[string]eru_models.JSONSchema),
	}

	var requiredFields []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		isSeen := false
		for _, seenField := range seenFields {
			if seenField == field.Name {
				isSeen = true
				break
			}
		}
		if isSeen {
			continue
		}
		seenFields = append(seenFields, field.Name)

		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		if jsonTag == "" {
			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				embedded := StructToJSONSchema(field.Type, seenFields)
				for k, v := range embedded.Properties {
					schema.Properties[k] = v
				}
				requiredFields = append(requiredFields, embedded.Required...)
			}
			continue
		}
		name := jsonTag
		if commaIdx := findComma(jsonTag); commaIdx != -1 {
			name = jsonTag[:commaIdx]
		}

		fieldSchema := goTypeToSchema(field.Type, seenFields)

		// Handle required
		if field.Tag.Get("eru") == "required" {
			requiredFields = append(requiredFields, name)
		}

		// Add description from field tag if present
		if desc := field.Tag.Get("desc"); desc != "" {
			fieldSchema.Description = desc
		}

		// Add format from field tag if present
		if format := field.Tag.Get("format"); format != "" {
			fieldSchema.Format = format
		}

		// A `secret:"true"` field holds a credential. Its value is a
		// $SECRET_<name> reference, never the secret itself, and it is never
		// returned once saved.
		if field.Tag.Get("secret") == "true" {
			fieldSchema.WriteOnly = true
			if fieldSchema.Format == "" {
				fieldSchema.Format = "password"
			}
		}

		// Add the allowed values from an `enum:"a,b,c"` tag.
		//
		// Worth the tag rather than prose in `desc`: the enum travels into the
		// provider's own tool schema, so a value outside the set is rejected
		// against the tool call itself instead of reaching the service and
		// coming back as a retry. A list applies to the items, not the array -
		// `enum` on a []string means each element is one of the set.
		if enumTag := field.Tag.Get("enum"); enumTag != "" {
			values := make([]interface{}, 0)
			for _, v := range strings.Split(enumTag, ",") {
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					values = append(values, trimmed)
				}
			}
			if len(values) > 0 {
				if fieldSchema.Type == "array" && fieldSchema.Items != nil {
					items := *fieldSchema.Items
					items.Enum = values
					fieldSchema.Items = &items
				} else {
					fieldSchema.Enum = values
				}
			}
		}
		if defaultVal := field.Tag.Get("default"); defaultVal != "" {
			fieldSchema.Description += fmt.Sprint(" - default value: ", defaultVal)
			requiredFields = append(requiredFields, name)
		}
		schema.Properties[name] = fieldSchema
	}

	if len(requiredFields) > 0 {
		schema.Required = requiredFields
	}

	return schema
}

func goTypeToSchema(t reflect.Type, seenFields []string) eru_models.JSONSchema {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	kind := t.Kind()

	switch kind {
	case reflect.String:
		return eru_models.JSONSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return eru_models.JSONSchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return eru_models.JSONSchema{Type: "number"}
	case reflect.Bool:
		return eru_models.JSONSchema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		return eru_models.JSONSchema{
			Type:  "array",
			Items: ptr(goTypeToSchema(t.Elem(), seenFields)),
		}
	case reflect.Map, reflect.Struct:
		if t.Kind() == reflect.Map {
			valueSchema := goTypeToSchema(t.Elem(), seenFields)
			return eru_models.JSONSchema{
				Type:                 "object",
				AdditionalProperties: valueSchema,
				Description:          "A map where each key is a unique identifier derived from the step type or system and user instructions",
			}
		}
		return StructToJSONSchema(t, seenFields)
	default:
		return eru_models.JSONSchema{Type: "string"} // Fallback
	}
}

func findComma(tag string) int {
	for i, c := range tag {
		if c == ',' {
			return i
		}
	}
	return -1
}

func ptr[T any](v T) *T {
	return &v
}
