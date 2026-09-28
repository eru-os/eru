package module_server

import (
	"net/http"

	"github.com/eru-os/eru/eru-html-image/html_image"
	module_handlers "github.com/eru-os/eru/eru-html-image/module_server/handlers"
	"github.com/eru-os/eru/eru-html-image/module_store"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func SetServiceName() {
	server_handlers.ServerName = "eru-html-image"
}

func AddModuleRoutes(serverRouter *mux.Router, sh *module_store.StoreHolder, renderer *html_image.Renderer) {
	imageRouter := serverRouter.PathPrefix("/html-image").Subrouter()

	imageRouter.Name("convert").Methods(http.MethodPost).Path("/convert").HandlerFunc(module_handlers.HtmlToImageHandler(renderer))
	imageRouter.Name("convert_encode").Methods(http.MethodPost).Path("/convert/{encode}").HandlerFunc(module_handlers.HtmlToImageHandler(renderer))
}
