package handlers

import (
	"net/http"
	"strings"
)

func extractHostUrl(request *http.Request) (string, string) {
	return strings.Split(request.Host, ":")[0], request.URL.Path
}
