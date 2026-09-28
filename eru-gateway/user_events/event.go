package user_events

import (
	"net"
	"net/http"
	"strings"
	"time"

	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
)

const maxHeaderValueLen = 512

type UserEvent struct {
	RequestId   string
	TraceId     string
	UserId      string
	Host        string
	Path        string
	Method      string
	Status      int
	DurationMs  int
	TargetHost  string
	ClientIp    string
	Headers     map[string]string
	RequestTime time.Time
	sizeBytes   int
}

var deniedHeaders = map[string]bool{
	"authorization":        true,
	"proxy-authorization":  true,
	"cookie":               true,
	"set-cookie":           true,
	utils.ClaimsHeaderKey:  true,
	"x-api-key":            true,
	"x-amz-security-token": true,
}

func NewEvent(r *http.Request, w http.ResponseWriter, at time.Time, allowedHeaders []string) UserEvent {
	e := UserEvent{
		RequestId:   firstNonEmpty(w.Header().Get(server_handlers.RequestIdKey), r.Header.Get(server_handlers.RequestIdKey)),
		TraceId:     w.Header().Get("trace_id"),
		Host:        strings.Split(r.Host, ":")[0],
		Path:        r.URL.Path,
		Method:      r.Method,
		ClientIp:    clientIp(r),
		Headers:     captureHeaders(r.Header, allowedHeaders),
		RequestTime: at,
	}
	e.sizeBytes = e.approxSize()
	return e
}

func captureHeaders(h http.Header, allowed []string) map[string]string {
	captured := make(map[string]string, len(allowed))
	for _, name := range allowed {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || deniedHeaders[key] {
			continue
		}
		value := h.Get(key)
		if value == "" {
			continue
		}
		if len(value) > maxHeaderValueLen {
			value = value[:maxHeaderValueLen]
		}
		captured[key] = value
	}
	return captured
}

func clientIp(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if realIp := r.Header.Get("X-Real-Ip"); realIp != "" {
		return strings.TrimSpace(realIp)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (e *UserEvent) approxSize() int {
	size := len(e.RequestId) + len(e.TraceId) + len(e.UserId) + len(e.Host) +
		len(e.Path) + len(e.Method) + len(e.TargetHost) + len(e.ClientIp) + 64
	for k, v := range e.Headers {
		size += len(k) + len(v) + 16
	}
	return size
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
