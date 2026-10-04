package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
	"github.com/eru-os/eru/eru-cache/cache"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/google/uuid"
)

// A client request asks the browser that is streaming this run to do
// something only it can - render a page with the user's data, measure it,
// capture it - and waits for the answer.
//
// The request goes out as a stream event. The browser answers through the
// client reply endpoint, which may land on any instance, so the answer is
// written to the agent's persisted chat memory and the waiting agent reads it
// back from there. No instance address travels through the browser, so nothing
// a client sends can make the server call an address of its choosing.
const (
	StreamEventClientRequest = "client_request"

	// ClientCapabilitiesParam lists what the calling client can do for an
	// agent mid-run, e.g. ["render_review"]. A request is only sent to a client
	// that said it can answer it.
	ClientCapabilitiesParam = "client_capabilities"

	clientReplyKeyPrefix = "client_reply::"
	clientReplyTTL       = 15 * time.Minute
)

// ClientRequest is the payload of a client_request stream event.
type ClientRequest struct {
	RequestId string                 `json:"request_id"`
	Kind      string                 `json:"kind"`
	AgentName string                 `json:"agent_name"`
	Payload   map[string]interface{} `json:"payload"`
	TimeoutMs int64                  `json:"timeout_ms"`
}

// ErrNoClient means the run has no client able to answer: no stream, or the
// client did not offer the capability.
var ErrNoClient = errors.New("no client can answer this request")

// ClientReplyKey is the cache key a reply to requestId is stored under.
func ClientReplyKey(requestId string) string {
	return clientReplyKeyPrefix + requestId
}

// ClientOffers reports whether the request's params list capability.
func ClientOffers(params map[string]interface{}, capability string) bool {
	switch list := params[ClientCapabilitiesParam].(type) {
	case []interface{}:
		for _, c := range list {
			if s, ok := c.(string); ok && s == capability {
				return true
			}
		}
	case []string:
		for _, s := range list {
			if s == capability {
				return true
			}
		}
	case string:
		for _, s := range strings.Split(list, ",") {
			if strings.TrimSpace(s) == capability {
				return true
			}
		}
	}
	return false
}

// UserIdFromContext is the caller's subject from the request claims, the
// identity chat memory rows are keyed by.
func UserIdFromContext(ctx context.Context) string {
	claims := tools.ClaimsFromContext(ctx)
	if claims == "" {
		return ""
	}
	claimsMap := map[string]interface{}{}
	if json.Unmarshal([]byte(claims), &claimsMap) != nil {
		return ""
	}
	sub, _ := claimsMap["sub"].(string)
	return sub
}

// AskClient sends a client request and waits up to timeout for the reply.
// memory must persist to the shared database; without that the reply could
// not reach this instance, so the request is not sent at all.
func AskClient(ctx context.Context, memory cache.CacheStoreI, projectId, tenantId, agentName, kind string,
	payload map[string]interface{}, timeout time.Duration) (map[string]interface{}, error) {
	if GetStreamCallback(ctx) == nil {
		return nil, ErrNoClient
	}
	if memory == nil {
		return nil, fmt.Errorf("%w: agent %s has no chat memory to receive the reply", ErrNoClient, agentName)
	}
	if persist, _ := memory.GetAttribute(ctx, "persist_enabled"); persist != true {
		return nil, fmt.Errorf("%w: agent %s chat memory does not persist, so a reply could not reach this instance", ErrNoClient, agentName)
	}
	request := ClientRequest{
		RequestId: uuid.New().String(),
		Kind:      kind,
		AgentName: agentName,
		Payload:   payload,
		TimeoutMs: timeout.Milliseconds(),
	}
	Emit(ctx, StreamEvent{Event: StreamEventClientRequest, Data: request})
	logs.WithContext(ctx).Info(fmt.Sprintf("client request %s (%s) sent for agent %s; waiting up to %s", request.RequestId, kind, agentName, timeout))

	userId := UserIdFromContext(ctx)
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
		rows, err := memory.LoadFromDatabase(ctx, projectId, tenantId, ClientReplyKey(request.RequestId), agentName, userId)
		if err == nil && len(rows) > 0 {
			reply := map[string]interface{}{}
			if uErr := json.Unmarshal([]byte(rows[len(rows)-1].CacheValue), &reply); uErr != nil {
				return nil, fmt.Errorf("client reply %s could not be read: %w", request.RequestId, uErr)
			}
			logs.WithContext(ctx).Info(fmt.Sprintf("client request %s answered", request.RequestId))
			return reply, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("client request %s timed out after %s", request.RequestId, timeout)
		}
	}
}

// StoreClientReply writes a client's reply where the waiting agent reads it.
func StoreClientReply(ctx context.Context, memory cache.CacheStoreI, projectId, tenantId, agentName, requestId string, reply map[string]interface{}) error {
	if memory == nil {
		return fmt.Errorf("agent %s has no chat memory", agentName)
	}
	encoded, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	now := time.Now()
	return memory.SyncToDatabase(ctx, projectId, []cache.CacheData{{
		CacheKey:     ClientReplyKey(requestId),
		CacheValue:   string(encoded),
		ProjectId:    projectId,
		TenantId:     tenantId,
		CreatedAt:    now,
		UpdatedAt:    now,
		ExpiresAt:    now.Add(clientReplyTTL),
		LastAccessed: now,
		CreatedBy:    UserIdFromContext(ctx),
		AgentName:    agentName,
	}})
}
