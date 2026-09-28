package a2a

import "encoding/json"

type TaskState string

const (
	TaskStateSubmitted     TaskState = "submitted"
	TaskStateWorking       TaskState = "working"
	TaskStateInputRequired TaskState = "input-required"
	TaskStateCompleted     TaskState = "completed"
	TaskStateCanceled      TaskState = "canceled"
	TaskStateFailed        TaskState = "failed"
	TaskStateRejected      TaskState = "rejected"
	TaskStateAuthRequired  TaskState = "auth-required"
	TaskStateUnknown       TaskState = "unknown"
)

type Part struct {
	Kind     string                 `json:"kind"`
	Text     string                 `json:"text,omitempty"`
	File     *FileContent           `json:"file,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type FileContent struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
}

type Message struct {
	Kind      string                 `json:"kind"`
	MessageId string                 `json:"messageId"`
	ContextId string                 `json:"contextId,omitempty"`
	TaskId    string                 `json:"taskId,omitempty"`
	Role      string                 `json:"role"`
	Parts     []Part                 `json:"parts"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type TaskStatus struct {
	State     TaskState `json:"state"`
	Message   *Message  `json:"message,omitempty"`
	Timestamp string    `json:"timestamp"`
}

type Artifact struct {
	ArtifactId  string                 `json:"artifactId"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Parts       []Part                 `json:"parts"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type Task struct {
	Kind      string                 `json:"kind"`
	Id        string                 `json:"id"`
	ContextId string                 `json:"contextId"`
	Status    TaskStatus             `json:"status"`
	Artifacts []Artifact             `json:"artifacts,omitempty"`
	History   []Message              `json:"history,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type AgentCapabilities struct {
	Streaming              bool `json:"streaming"`
	PushNotifications      bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

type AgentSkill struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

type AgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

type AgentCard struct {
	Name               string            `json:"name"`
	Description        string            `json:"description,omitempty"`
	URL                string            `json:"url"`
	Version            string            `json:"version"`
	Capabilities       AgentCapabilities `json:"capabilities"`
	Skills             []AgentSkill      `json:"skills"`
	DefaultInputModes  []string          `json:"defaultInputModes"`
	DefaultOutputModes []string          `json:"defaultOutputModes"`
	Provider           *AgentProvider    `json:"provider,omitempty"`
	DocumentationURL   string            `json:"documentationUrl,omitempty"`
}

type JSONRPCRequest struct {
	JSONRPCVersion string          `json:"jsonrpc"`
	ID             interface{}     `json:"id,omitempty"`
	Method         string          `json:"method"`
	Params         json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPCVersion string        `json:"jsonrpc"`
	ID             interface{}   `json:"id,omitempty"`
	Result         interface{}   `json:"result,omitempty"`
	Error          *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type MessageSendConfiguration struct {
	AcceptedOutputModes []string `json:"acceptedOutputModes,omitempty"`
	Blocking            bool     `json:"blocking,omitempty"`
	HistoryLength       int      `json:"historyLength,omitempty"`
}

type MessageSendParams struct {
	Message       Message                   `json:"message"`
	Configuration *MessageSendConfiguration `json:"configuration,omitempty"`
}

type TaskGetParams struct {
	ID            string `json:"id"`
	HistoryLength int    `json:"historyLength,omitempty"`
}

type TaskCancelParams struct {
	ID string `json:"id"`
}

type TaskStatusUpdateEvent struct {
	Kind      string     `json:"kind"`
	TaskId    string     `json:"taskId"`
	ContextId string     `json:"contextId"`
	Status    TaskStatus `json:"status"`
	Final     bool       `json:"final"`
}

type TaskArtifactUpdateEvent struct {
	Kind      string   `json:"kind"`
	TaskId    string   `json:"taskId"`
	ContextId string   `json:"contextId"`
	Artifact  Artifact `json:"artifact"`
	Append    bool     `json:"append"`
	LastChunk bool     `json:"lastChunk"`
}
