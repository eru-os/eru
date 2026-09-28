package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	agents "github.com/eru-os/eru/eru-ai/agents"
	"github.com/eru-os/eru/eru-ai/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

var (
	streamCallbackUrlOnce  sync.Once
	streamCallbackUrlValue string
)

// streamCallbackUrl is this pod's own directly reachable address, handed to
// sub-agents so their events come back to this process rather than to whichever
// instance a load balancer happens to pick. Empty when the address cannot be
// determined, in which case sub-agent events are simply not relayed.
func streamCallbackUrl() string {
	streamCallbackUrlOnce.Do(func() {
		port := module_store.Eruaiport
		if port == "" {
			return
		}
		address, err := eru_utils.GetServiceAddress(context.Background(), port)
		if err != nil {
			logs.WithContext(context.Background()).Error(fmt.Sprint("sub-agent event relay disabled - could not detect service address : ", err.Error()))
			return
		}
		streamCallbackUrlValue = address
	})
	return streamCallbackUrlValue
}

func parseStreamTarget(r *http.Request) (agents.StreamTarget, bool) {
	target := agents.StreamTarget{
		StreamId:    r.Header.Get(agents.HeaderStreamId),
		CallbackUrl: r.Header.Get(agents.HeaderStreamCallback),
	}
	if target.StreamId == "" || target.CallbackUrl == "" {
		return agents.StreamTarget{}, false
	}
	return target, true
}

// AgentStreamEventHandler receives a batch of live events from a sub-agent and
// queues them onto the SSE connection that started the orchestration. It always
// answers 202: relaying is best-effort, and a sub-agent must never fail or slow
// down because the client stream ended or its buffer was full.
func AgentStreamEventHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("AgentStreamEventHandler - Start")
		streamId := mux.Vars(r)["streamid"]

		var batch streamEventBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprint("AgentStreamEventHandler - malformed batch : ", err.Error()))
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		accepted := activeStreams.publish(streamId, batch.Events)
		if accepted < len(batch.Events) {
			logs.WithContext(r.Context()).Debug(fmt.Sprint("AgentStreamEventHandler - dropped ", len(batch.Events)-accepted, " of ", len(batch.Events), " event(s) for stream ", streamId))
		}
		server_handlers.FormatResponse(w, 202)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"accepted": accepted})
	}
}
