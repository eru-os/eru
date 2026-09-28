package server

import (
	"net/http"

	"github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/eru-os/eru/eru-store/store"
)

const (
	ConfigUpdatedHeader  = "Eru-Config-Updated"
	ConfigInstanceHeader = "Eru-Config-Instance"
)

type configChangeWriter struct {
	http.ResponseWriter
	change      *store.ConfigChange
	wroteHeader bool
}

func (w *configChangeWriter) stampHeaders() {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if w.change.Changed() {
		w.Header().Set(ConfigUpdatedHeader, handlers.ServerName)
		w.Header().Set(ConfigInstanceHeader, handlers.InstanceId)
	}
}

func (w *configChangeWriter) WriteHeader(code int) {
	w.stampHeaders()
	w.ResponseWriter.WriteHeader(code)
}

func (w *configChangeWriter) Write(b []byte) (int, error) {
	w.stampHeaders()
	return w.ResponseWriter.Write(b)
}

func (w *configChangeWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *configChangeWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func configChangeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		change := &store.ConfigChange{}
		r = r.WithContext(store.ContextWithConfigChange(r.Context(), change))
		next.ServeHTTP(&configChangeWriter{ResponseWriter: rw, change: change}, r)
	})
}
