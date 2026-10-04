package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	agents "github.com/eru-os/eru/eru-ai/agents"
	"github.com/eru-os/eru/eru-ai/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

// maxClientReplyBytes bounds a reply: a few compressed screenshots and a
// geometry report, never an arbitrary upload.
const maxClientReplyBytes = 12 << 20

// ClientReplyHandler stores a browser's answer to a client request, where the
// agent that asked - on whichever instance it runs - reads it back.
func ClientReplyHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		projectId, tenantId := vars["project"], vars["tenant"]
		agentName, requestId := vars["agentname"], vars["requestid"]

		r.Body = http.MaxBytesReader(w, r.Body, maxClientReplyBytes)
		reply := map[string]interface{}{}
		if err := json.NewDecoder(r.Body).Decode(&reply); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "reply must be a JSON object: " + err.Error()})
			return
		}
		agent, err := sh.Store.GetAgent(r.Context(), projectId, tenantId, "", agentName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 404)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if err := agents.StoreClientReply(r.Context(), agent.GetChatMemory(), projectId, tenantId, agentName, requestId, reply); err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprint("client reply ", requestId, " not stored: ", err.Error()))
			server_handlers.FormatResponse(w, 500)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"stored": requestId})
	}
}
