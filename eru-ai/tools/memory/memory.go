package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	eru_utils "github.com/eru-os/eru/eru-utils"
	vectorstore "github.com/eru-os/eru/eru-vectorstore/vectorstore"
)

const (
	Remember = "remember"
	Recall   = "recall"
	Forget   = "forget"
)

type RememberInput struct {
	Content   string                 `json:"content" eru:"required"`
	Namespace string                 `json:"namespace" eru:"required"`
	Tags      []string               `json:"tags,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type RecallInput struct {
	Query     string                 `json:"query" eru:"required"`
	Namespace string                 `json:"namespace" eru:"required"`
	TopK      int                    `json:"top_k,omitempty"`
	Filter    map[string]interface{} `json:"filter,omitempty"`
}

type ForgetInput struct {
	Ids       []string               `json:"ids" eru:"required"`
	Namespace string                 `json:"namespace" eru:"required"`
	Filter    map[string]interface{} `json:"filter,omitempty"`
}

var memoryToolActions = []tools.ToolAction{
	{
		ActionName:   Remember,
		Description:  "Save knowledge to semantic memory for future recall",
		SystemPrompt: "Extract and save important knowledge, facts, or learnings from the conversation to semantic memory. Include relevant metadata and tags for future retrieval.",
		GetParameters: func() eru_models.JSONSchema {
			return eru_utils.StructToJSONSchema(reflect.TypeOf(RememberInput{}), []string{})
		},
	},
	{
		ActionName:   Recall,
		Description:  "Search semantic memory for relevant prior knowledge",
		SystemPrompt: "Search semantic memory for knowledge relevant to the current context. Use natural language queries to find previously stored information.",
		GetParameters: func() eru_models.JSONSchema {
			return eru_utils.StructToJSONSchema(reflect.TypeOf(RecallInput{}), []string{})
		},
	},
	{
		ActionName:   Forget,
		Description:  "Remove outdated knowledge from semantic memory",
		SystemPrompt: "Remove specific knowledge entries from semantic memory that are no longer accurate or relevant.",
		GetParameters: func() eru_models.JSONSchema {
			return eru_utils.StructToJSONSchema(reflect.TypeOf(ForgetInput{}), []string{})
		},
	},
}

type MemoryTool struct {
	tools.Tool
	VectorStore     vectorstore.VectorStoreI `json:"-"`
	VectorStoreName string                   `json:"vectorstore_name"`
}

func (mt *MemoryTool) GetActionsList() []tools.ActionInfo {
	infos := make([]tools.ActionInfo, len(memoryToolActions))
	for i, action := range memoryToolActions {
		infos[i] = tools.ActionInfo{Name: action.ActionName, Description: action.Description}
	}
	return infos
}

func (mt *MemoryTool) GetActions() []tools.ToolAction {
	return memoryToolActions
}

func (mt *MemoryTool) GetSpec() tools.Tooling {
	return mt
}

func (mt *MemoryTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MemoryTool MakeFromJson - Start")
	err := json.Unmarshal(*rj, &mt)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (mt *MemoryTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	newTool := &MemoryTool{}
	err := json.Unmarshal(toolObjJson, newTool)
	if err != nil {
		return nil, err
	}
	return newTool, nil
}

func (mt *MemoryTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("MemoryTool Execute - Start")

	if mt.VectorStore == nil {
		return nil, false, fmt.Errorf("vector store not configured for memory tool")
	}

	switch actionName {
	case Remember:
		toolResult, err = mt.remember(ctx, params)
	case Recall:
		toolResult, err = mt.recall(ctx, params)
	case Forget:
		toolResult, err = mt.forget(ctx, params)
	default:
		return nil, false, fmt.Errorf("action %s not found", actionName)
	}

	return toolResult, false, err
}

func (mt *MemoryTool) remember(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	logs.WithContext(ctx).Debug("MemoryTool remember - Start")

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("error marshalling params: %w", err)
	}
	var input RememberInput
	if err := json.Unmarshal(paramsBytes, &input); err != nil {
		return nil, fmt.Errorf("error unmarshalling remember input: %w", err)
	}

	if input.Content == "" {
		return nil, fmt.Errorf("content is required for remember action")
	}
	if input.Namespace == "" {
		return nil, fmt.Errorf("namespace is required for remember action")
	}

	metadata := make(map[string]interface{})
	for k, v := range input.Metadata {
		metadata[k] = v
	}
	metadata["content"] = input.Content
	metadata["created_at"] = time.Now().UTC().Format(time.RFC3339)
	if len(input.Tags) > 0 {
		metadata["tags"] = input.Tags
	}

	vectorId := fmt.Sprintf("mem_%d", time.Now().UnixNano())

	vectorRecords := vectorstore.VectorRecords{
		Namespace: input.Namespace,
		Vectors: []vectorstore.Vector{
			{
				Id:       vectorId,
				Metadata: metadata,
			},
		},
	}

	if err := mt.VectorStore.SaveVectors(ctx, vectorRecords); err != nil {
		return nil, fmt.Errorf("error saving to memory: %w", err)
	}

	return map[string]interface{}{
		"message": "memory saved successfully",
		"id":      vectorId,
	}, nil
}

func (mt *MemoryTool) recall(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	logs.WithContext(ctx).Debug("MemoryTool recall - Start")

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("error marshalling params: %w", err)
	}
	var input RecallInput
	if err := json.Unmarshal(paramsBytes, &input); err != nil {
		return nil, fmt.Errorf("error unmarshalling recall input: %w", err)
	}

	if input.Query == "" {
		return nil, fmt.Errorf("query is required for recall action")
	}
	if input.Namespace == "" {
		return nil, fmt.Errorf("namespace is required for recall action")
	}

	topK := input.TopK
	if topK <= 0 {
		topK = 5
	}

	searchRequest := vectorstore.VectorRecordsSearch{
		Namespace:      input.Namespace,
		TopK:           topK,
		ReturnMetadata: true,
		Filter:         input.Filter,
		Inputs:         map[string]string{"text": input.Query},
	}

	results, err := mt.VectorStore.SearchVectors(ctx, searchRequest)
	if err != nil {
		return nil, fmt.Errorf("error searching memory: %w", err)
	}

	memories := make([]map[string]interface{}, 0, len(results.Records))
	for _, record := range results.Records {
		memory := map[string]interface{}{
			"id":       record.Id,
			"metadata": record.Metadata,
		}
		if content, ok := record.Metadata["content"]; ok {
			memory["content"] = content
		}
		memories = append(memories, memory)
	}

	return map[string]interface{}{
		"memories": memories,
		"count":    len(memories),
	}, nil
}

func (mt *MemoryTool) forget(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	logs.WithContext(ctx).Debug("MemoryTool forget - Start")

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("error marshalling params: %w", err)
	}
	var input ForgetInput
	if err := json.Unmarshal(paramsBytes, &input); err != nil {
		return nil, fmt.Errorf("error unmarshalling forget input: %w", err)
	}

	if len(input.Ids) == 0 && len(input.Filter) == 0 {
		return nil, fmt.Errorf("ids or filter required for forget action")
	}
	if input.Namespace == "" {
		return nil, fmt.Errorf("namespace is required for forget action")
	}

	deleteRequest := vectorstore.VectorRecordsDelete{
		Ids:       input.Ids,
		Namespace: input.Namespace,
		Filter:    input.Filter,
	}

	if err := mt.VectorStore.DeleteVectors(ctx, deleteRequest); err != nil {
		return nil, fmt.Errorf("error deleting from memory: %w", err)
	}

	return map[string]interface{}{
		"message": "memory entries deleted successfully",
		"count":   len(input.Ids),
	}, nil
}

func (mt *MemoryTool) GetAttribute(ctx context.Context, attributeName string) (interface{}, error) {
	switch attributeName {
	case "tool_name":
		return mt.ToolName, nil
	case "tool_type":
		return mt.ToolType, nil
	case "system_prompt":
		return mt.SystemPrompt, nil
	case "output_schema":
		return mt.OutputSchema, nil
	case "parameters":
		return mt.Parameters, nil
	case "description":
		return mt.Description, nil
	case "vectorstore_name":
		return mt.VectorStoreName, nil
	default:
		return nil, fmt.Errorf("attribute %s not found", attributeName)
	}
}

func (mt *MemoryTool) SetAttribute(ctx context.Context, attributeName string, attributeValue interface{}) error {
	switch attributeName {
	case "tool_name":
		mt.ToolName = attributeValue.(string)
	case "tool_type":
		mt.ToolType = attributeValue.(string)
	case "system_prompt":
		mt.SystemPrompt = attributeValue.(string)
	case "output_schema":
		mt.OutputSchema = attributeValue.(eru_models.JSONSchema)
	case "parameters":
		mt.Parameters = attributeValue.(eru_models.JSONSchema)
	case "description":
		mt.Description = attributeValue.(string)
	case "vectorstore":
		mt.VectorStore = attributeValue.(vectorstore.VectorStoreI)
	default:
		return fmt.Errorf("attribute %s not found", attributeName)
	}
	return nil
}

func init() {
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:      false,
		ToolType:    "Memory",
		Category:    "AI",
		Description: "Semantic memory for cross-agent and cross-session knowledge sharing",
		Actions:     []tools.ActionInfo{{Name: Remember}, {Name: Recall}, {Name: Forget}},
		ToolSchema:  eru_utils.StructToJSONSchema(reflect.TypeOf(MemoryTool{}), []string{}),
	})
}
