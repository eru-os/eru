package module_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eru-os/eru/eru-ai/agents"
	"github.com/eru-os/eru/eru-ai/agents/a2a"
	"github.com/eru-os/eru/eru-ai/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-server/server"
	"github.com/google/uuid"
)

const (
	a2aVersion = "1.0.0"
	a2aNameSep = "__"
)

type EruAIA2AServer struct {
	store     *module_store.StoreHolder
	taskStore *a2a.TaskStore
}

func NewEruAIA2AServer(store *module_store.StoreHolder) *EruAIA2AServer {
	return &EruAIA2AServer{
		store:     store,
		taskStore: a2a.NewTaskStore(),
	}
}

func (s *EruAIA2AServer) GetAgentCard(ctx context.Context, baseURL string, projectId, tenantId string) a2a.AgentCard {
	skills := s.buildSkills(ctx, projectId, tenantId)
	return a2a.AgentCard{
		Name:        "Eru AI",
		Description: "Eru AI Agent Platform - multi-tenant AI agents",
		URL:         baseURL + "/a2a",
		Version:     a2aVersion,
		Capabilities: a2a.AgentCapabilities{
			Streaming:              true,
			PushNotifications:      false,
			StateTransitionHistory: false,
		},
		Skills:             skills,
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Provider: &a2a.AgentProvider{
			Organization: "Eru Tech",
		},
	}
}

func (s *EruAIA2AServer) buildSkills(ctx context.Context, projectId, tenantId string) []a2a.AgentSkill {
	var skills []a2a.AgentSkill
	projectList := s.store.Store.GetProjectList(ctx)
	for _, projectInfo := range projectList {
		projectName, ok := projectInfo["project_name"].(string)
		if !ok {
			continue
		}
		if projectId == "" || projectId == projectName {
			project, err := s.store.Store.GetProjectConfig(ctx, projectName)
			if err != nil {
				continue
			}
			for _, tenant := range project.Tenants {
				if tenantId == "" || tenant.TenantId == tenantId || tenant.TenantId == projectId {
					agentNames, err := s.store.Store.GetAgentNames(ctx, projectName, tenant.TenantId)
					if err != nil {
						continue
					}
					for _, agentName := range agentNames {
						agent, err := s.store.Store.GetAgent(ctx, projectName, tenant.TenantId, "", agentName, s.store.Store)
						if err != nil {
							continue
						}
						description := ""
						if desc, err := agent.GetAttribute(ctx, "description"); err == nil {
							if descStr, ok := desc.(string); ok {
								description = descStr
							}
						}
						if description == "" {
							description = fmt.Sprintf("AI Agent: %s", agentName)
						}
						if tenantId == projectId && projectId != "" {
							agentName = strings.Join([]string{projectName, agentName}, a2aNameSep)
						}
						skillId := agentName
						skills = append(skills, a2a.AgentSkill{
							Id:          skillId,
							Name:        agentName,
							Description: description,
							InputModes:  []string{"text/plain"},
							OutputModes: []string{"text/plain"},
						})
					}
				}
			}
		}
	}
	if skills == nil {
		skills = []a2a.AgentSkill{}
	}
	return skills
}

func (s *EruAIA2AServer) HandleMessage(ctx context.Context, data []byte, w http.ResponseWriter) error {
	var req a2a.JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return s.writeError(w, nil, -32700, "Parse error", nil)
	}
	if req.JSONRPCVersion != "2.0" {
		return s.writeError(w, req.ID, -32600, "Invalid Request", nil)
	}

	switch req.Method {
	case "message/send":
		return s.handleMessageSend(ctx, req, w)
	case "message/stream":
		return s.handleMessageStream(ctx, req, w)
	case "tasks/get":
		return s.handleTasksGet(ctx, req, w)
	case "tasks/cancel":
		return s.handleTasksCancel(ctx, req, w)
	default:
		if req.ID == nil {
			return nil
		}
		return s.writeError(w, req.ID, -32601, "Method not found", nil)
	}
}

func (s *EruAIA2AServer) handleMessageSend(ctx context.Context, req a2a.JSONRPCRequest, w http.ResponseWriter) error {
	var params a2a.MessageSendParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.writeError(w, req.ID, -32602, "Invalid params", nil)
	}

	taskId := uuid.New().String()
	contextId := params.Message.ContextId
	if contextId == "" {
		contextId = uuid.New().String()
	}

	now := time.Now().UTC().Format(time.RFC3339)
	task := &a2a.Task{
		Kind:      "task",
		Id:        taskId,
		ContextId: contextId,
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateSubmitted,
			Timestamp: now,
		},
		History: []a2a.Message{params.Message},
	}
	s.taskStore.Save(task)

	_ = s.taskStore.UpdateStatus(taskId, a2a.TaskStatus{State: a2a.TaskStateWorking, Timestamp: time.Now().UTC().Format(time.RFC3339)})

	projectId, tenantId, agentName := s.resolveSkill(ctx, params.Message)
	if agentName == "" {
		_ = s.taskStore.UpdateStatus(taskId, a2a.TaskStatus{
			State:     a2a.TaskStateFailed,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return s.writeError(w, req.ID, -32602, "No skill found for the request", nil)
	}

	agentMsg := s.buildAgentMessage(params.Message)
	agent, err := s.store.Store.GetAgent(ctx, projectId, tenantId, contextId, agentName, s.store.Store)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("A2A: GetAgent failed: %v", err))
		_ = s.taskStore.UpdateStatus(taskId, a2a.TaskStatus{
			State:     a2a.TaskStateFailed,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return s.writeError(w, req.ID, -32603, "Agent not found: "+agentName, nil)
	}

	result, err := agent.Execute(ctx, agentMsg, contextId, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("A2A: agent Execute failed: %v", err))
		_ = s.taskStore.UpdateStatus(taskId, a2a.TaskStatus{
			State:     a2a.TaskStateFailed,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return s.writeError(w, req.ID, -32603, "Agent execution failed: "+err.Error(), nil)
	}

	responseMsg := s.buildResponseMessage(result, taskId, contextId)
	completedStatus := a2a.TaskStatus{
		State:     a2a.TaskStateCompleted,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message:   &responseMsg,
	}
	_ = s.taskStore.UpdateStatus(taskId, completedStatus)

	task, _ = s.taskStore.Get(taskId)
	task.History = append(task.History, responseMsg)

	return s.writeResult(w, req.ID, task)
}

func (s *EruAIA2AServer) handleMessageStream(ctx context.Context, req a2a.JSONRPCRequest, w http.ResponseWriter) error {
	var params a2a.MessageSendParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.writeError(w, req.ID, -32602, "Invalid params", nil)
	}

	taskId := uuid.New().String()
	contextId := params.Message.ContextId
	if contextId == "" {
		contextId = uuid.New().String()
	}

	now := time.Now().UTC().Format(time.RFC3339)
	task := &a2a.Task{
		Kind:      "task",
		Id:        taskId,
		ContextId: contextId,
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateSubmitted,
			Timestamp: now,
		},
		History: []a2a.Message{params.Message},
	}
	s.taskStore.Save(task)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	sendEvent := func(event interface{}) {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		if canFlush {
			flusher.Flush()
		}
	}

	sendEvent(a2a.TaskStatusUpdateEvent{
		Kind:      "status-update",
		TaskId:    taskId,
		ContextId: contextId,
		Status:    a2a.TaskStatus{State: a2a.TaskStateWorking, Timestamp: time.Now().UTC().Format(time.RFC3339)},
		Final:     false,
	})

	projectId, tenantId, agentName := s.resolveSkill(ctx, params.Message)
	if agentName == "" {
		sendEvent(a2a.TaskStatusUpdateEvent{
			Kind:      "status-update",
			TaskId:    taskId,
			ContextId: contextId,
			Status:    a2a.TaskStatus{State: a2a.TaskStateFailed, Timestamp: time.Now().UTC().Format(time.RFC3339)},
			Final:     true,
		})
		return nil
	}

	agentMsg := s.buildAgentMessage(params.Message)
	agent, err := s.store.Store.GetAgent(ctx, projectId, tenantId, contextId, agentName, s.store.Store)
	if err != nil {
		sendEvent(a2a.TaskStatusUpdateEvent{
			Kind:      "status-update",
			TaskId:    taskId,
			ContextId: contextId,
			Status:    a2a.TaskStatus{State: a2a.TaskStateFailed, Timestamp: time.Now().UTC().Format(time.RFC3339)},
			Final:     true,
		})
		return nil
	}

	result, err := agent.Execute(ctx, agentMsg, contextId, projectId, tenantId)
	if err != nil {
		sendEvent(a2a.TaskStatusUpdateEvent{
			Kind:      "status-update",
			TaskId:    taskId,
			ContextId: contextId,
			Status:    a2a.TaskStatus{State: a2a.TaskStateFailed, Timestamp: time.Now().UTC().Format(time.RFC3339)},
			Final:     true,
		})
		return nil
	}

	responseMsg := s.buildResponseMessage(result, taskId, contextId)

	artifact := a2a.Artifact{
		ArtifactId: uuid.New().String(),
		Parts:      responseMsg.Parts,
	}
	sendEvent(a2a.TaskArtifactUpdateEvent{
		Kind:      "artifact-update",
		TaskId:    taskId,
		ContextId: contextId,
		Artifact:  artifact,
		LastChunk: true,
	})

	completedStatus := a2a.TaskStatus{
		State:     a2a.TaskStateCompleted,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message:   &responseMsg,
	}
	_ = s.taskStore.UpdateStatus(taskId, completedStatus)

	sendEvent(a2a.TaskStatusUpdateEvent{
		Kind:      "status-update",
		TaskId:    taskId,
		ContextId: contextId,
		Status:    completedStatus,
		Final:     true,
	})

	return nil
}

func (s *EruAIA2AServer) handleTasksGet(ctx context.Context, req a2a.JSONRPCRequest, w http.ResponseWriter) error {
	var params a2a.TaskGetParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.writeError(w, req.ID, -32602, "Invalid params", nil)
	}
	task, err := s.taskStore.Get(params.ID)
	if err != nil {
		return s.writeError(w, req.ID, -32001, err.Error(), nil)
	}
	return s.writeResult(w, req.ID, task)
}

func (s *EruAIA2AServer) handleTasksCancel(ctx context.Context, req a2a.JSONRPCRequest, w http.ResponseWriter) error {
	var params a2a.TaskCancelParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.writeError(w, req.ID, -32602, "Invalid params", nil)
	}
	task, err := s.taskStore.Cancel(params.ID)
	if err != nil {
		return s.writeError(w, req.ID, -32001, err.Error(), nil)
	}
	return s.writeResult(w, req.ID, task)
}

func (s *EruAIA2AServer) resolveSkill(ctx context.Context, msg a2a.Message) (projectId, tenantId, agentName string) {
	if msg.Metadata != nil {
		if skillId, ok := msg.Metadata["skillId"].(string); ok && skillId != "" {
			parts := strings.Split(skillId, a2aNameSep)
			switch len(parts) {
			case 3:
				return parts[0], parts[1], parts[2]
			case 2:
				return s.findAgentInProject(ctx, parts[0], parts[1])
			case 1:
				return s.findAgentByName(ctx, parts[0])
			}
		}
	}
	return s.findAgentByName(ctx, "")
}

func (s *EruAIA2AServer) findAgentInProject(ctx context.Context, projectName, targetAgentName string) (string, string, string) {
	project, err := s.store.Store.GetProjectConfig(ctx, projectName)
	if err != nil {
		return "", "", ""
	}
	for _, tenant := range project.Tenants {
		agentNames, err := s.store.Store.GetAgentNames(ctx, projectName, tenant.TenantId)
		if err != nil {
			continue
		}
		for _, name := range agentNames {
			if name == targetAgentName {
				return projectName, tenant.TenantId, name
			}
		}
	}
	return "", "", ""
}

func (s *EruAIA2AServer) findAgentByName(ctx context.Context, targetAgentName string) (string, string, string) {
	projectList := s.store.Store.GetProjectList(ctx)
	for _, projectInfo := range projectList {
		pName, ok := projectInfo["project_name"].(string)
		if !ok {
			continue
		}
		project, err := s.store.Store.GetProjectConfig(ctx, pName)
		if err != nil {
			continue
		}
		for _, tenant := range project.Tenants {
			agentNames, err := s.store.Store.GetAgentNames(ctx, pName, tenant.TenantId)
			if err != nil || len(agentNames) == 0 {
				continue
			}
			if targetAgentName == "" {
				return pName, tenant.TenantId, agentNames[0]
			}
			for _, name := range agentNames {
				if name == targetAgentName {
					return pName, tenant.TenantId, name
				}
			}
		}
	}
	return "", "", ""
}

func (s *EruAIA2AServer) buildAgentMessage(msg a2a.Message) agents.AgentMessage {
	var textParts []string
	for _, part := range msg.Parts {
		if part.Kind == "text" && part.Text != "" {
			textParts = append(textParts, part.Text)
		}
	}
	content := strings.Join(textParts, "\n")
	params := make(map[string]interface{})
	if msg.Metadata != nil {
		for k, v := range msg.Metadata {
			if k != "skillId" {
				params[k] = v
			}
		}
	}
	return agents.AgentMessage{
		Content:   content,
		Params:    params,
		MessageId: msg.MessageId,
		Role:      "user",
	}
}

func (s *EruAIA2AServer) buildResponseMessage(result agents.AgentMessage, taskId, contextId string) a2a.Message {
	text := result.Content
	if text == "" && len(result.Actions) > 0 {
		b, _ := json.Marshal(result.Actions)
		text = string(b)
	}
	return a2a.Message{
		Kind:      "message",
		MessageId: uuid.New().String(),
		TaskId:    taskId,
		ContextId: contextId,
		Role:      "agent",
		Parts: []a2a.Part{
			{Kind: "text", Text: text},
		},
	}
}

func (s *EruAIA2AServer) writeResult(w http.ResponseWriter, id interface{}, result interface{}) error {
	resp := a2a.JSONRPCResponse{
		JSONRPCVersion: "2.0",
		ID:             id,
		Result:         result,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}

func (s *EruAIA2AServer) writeError(w http.ResponseWriter, id interface{}, code int, message string, data interface{}) error {
	resp := a2a.JSONRPCResponse{
		JSONRPCVersion: "2.0",
		ID:             id,
		Error: &a2a.JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}

func (s *EruAIA2AServer) CreateHttpHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("A2AHttpHandler - Start")
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if err := s.HandleMessage(r.Context(), data, w); err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprintf("A2A handler error: %v", err))
		}
	}
}

func (s *EruAIA2AServer) CreateAgentCardHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("A2AAgentCardHandler - Start")
		baseURL := getA2ABaseURL(r)
		projectId := server.RequestProject(r)
		ctx, tenantId := server.RequestTenantContext(r)
		card := s.GetAgentCard(ctx, baseURL, projectId, tenantId)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(card)
	}
}

func getA2ABaseURL(r *http.Request) string {
	baseURL := os.Getenv("ERUAI_BASE_URL")
	if baseURL != "" {
		return baseURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
