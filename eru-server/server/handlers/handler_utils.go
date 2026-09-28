package handlers

import (
	"fmt"
	"net/http"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/rs/cors"
)

func FormatResponse(w http.ResponseWriter, status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
}

// makeCorsObject takes required config and make a new cors object
func MakeCorsObject() *cors.Cors {
	return cors.New(cors.Options{
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "PUT", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Original-Endpoint", "Id_token", "Claims", "Mcp-Session-Id", "Mcp-Protocol-Version", "X-Config-Api-Key"},
		ExposedHeaders:   []string{"Mcp-Session-Id", "Mcp-Protocol-Version", "WWW-Authenticate"},
		AllowOriginRequestFunc: func(r *http.Request, s string) bool {
			if AllowedOrigins == "" {
				return true
			}
			parts := strings.SplitN(s, "//", 2)
			if len(parts) < 2 {
				return true
			}
			dn := parts[1]
			logs.Logger.Info(fmt.Sprint("dn = ", dn))
			envOrigin := strings.Split(AllowedOrigins, ",")
			for _, o := range envOrigin {
				oo := strings.Replace(o, "*.", "", -1)
				if strings.Contains(dn, oo) {
					return true
				}
			}
			return false
		},
		Debug: false,
	})
}

func AllowCorsObject() *cors.Cors {
	return cors.New(cors.Options{
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "PUT", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Original-Endpoint", "Id_token", "Claims"},
		AllowOriginRequestFunc: func(r *http.Request, s string) bool {
			return true
		},
		//AllowedOrigins: []string{"*"},
		Debug: false,
		//ExposedHeaders: []string{"Authorization", "Content-Type"},
	})
}
