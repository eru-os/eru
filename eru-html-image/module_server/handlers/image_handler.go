package handlers

import (
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/eru-os/eru/eru-html-image/html_image"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func HtmlToImageHandler(renderer *html_image.Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("HtmlToImageHandler - Start")
		vars := mux.Vars(r)
		encode := vars["encode"]

		renderRequest, err := parseRenderRequest(r)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		image, format, err := renderer.Render(r.Context(), renderRequest)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if encode == "encode" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"image": b64.StdEncoding.EncodeToString(image), "format": format})
			return
		}
		w.Header().Set("Content-Type", html_image.ContentType(format))
		w.Header().Set("Content-Disposition", fmt.Sprint("inline; filename=output.", format))
		_, _ = w.Write(image)
	}
}

func parseRenderRequest(r *http.Request) (renderRequest html_image.RenderRequest, err error) {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType == "text/html" || contentType == "text/plain" {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			logs.WithContext(r.Context()).Error(readErr.Error())
			return renderRequest, readErr
		}
		renderRequest.Html = string(body)
		applyQueryParams(&renderRequest, r.URL.Query())
	} else {
		if err = json.NewDecoder(r.Body).Decode(&renderRequest); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			return renderRequest, err
		}
	}
	if strings.TrimSpace(renderRequest.Html) == "" {
		err = errors.New("html is required")
		logs.WithContext(r.Context()).Error(err.Error())
		return renderRequest, err
	}
	return renderRequest, nil
}

func applyQueryParams(renderRequest *html_image.RenderRequest, params url.Values) {
	if v, err := strconv.Atoi(params.Get("width")); err == nil {
		renderRequest.Width = v
	}
	if v, err := strconv.Atoi(params.Get("height")); err == nil {
		renderRequest.Height = v
	}
	if v, err := strconv.ParseFloat(params.Get("scale"), 64); err == nil {
		renderRequest.Scale = v
	}
	if v, err := strconv.Atoi(params.Get("quality")); err == nil {
		renderRequest.Quality = v
	}
	if v, err := strconv.Atoi(params.Get("timeout_seconds")); err == nil {
		renderRequest.TimeoutSeconds = v
	}
	if v := params.Get("format"); v != "" {
		renderRequest.Format = v
	}
	if v := params.Get("selector"); v != "" {
		renderRequest.Selector = v
	}
	if v := params.Get("background_color"); v != "" {
		renderRequest.BackgroundColor = v
	}
}
