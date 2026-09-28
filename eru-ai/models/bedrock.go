package models

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type BedrockModel struct {
	Model
	client *bedrockruntime.Client
}

// Request Types
type BedrockChatRequest struct {
	Prompt        string   `json:"prompt"`
	MaxTokens     int      `json:"max_tokens_to_sample"`
	Temperature   float64  `json:"temperature"`
	TopP          float64  `json:"top_p"`
	TopK          int      `json:"top_k"`
	StopSequences []string `json:"stop_sequences"`
}

type BedrockMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response Types
type BedrockChatResponse struct {
	Completion string `json:"completion"`
	StopReason string `json:"stop_reason"`
}

func (bedrockModel *BedrockModel) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &bedrockModel)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	// Initialize AWS Bedrock client
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		logs.WithContext(ctx).Error("unable to load AWS SDK config: " + err.Error())
		return err
	}

	// Create Bedrock client
	bedrockModel.client = bedrockruntime.NewFromConfig(cfg)
	return nil
}

func (bedrockModel *BedrockModel) QueryModel(ctx context.Context, chatRequest ChatRequest) (queryResponse Message, err error) {
	logs.WithContext(ctx).Debug("QueryModel - Start")

	bedrockRequest, err := bedrockModel.makeBedrockChatRequest(ctx, chatRequest)
	if err != nil {
		return
	}

	bedrockResponse, err := bedrockModel.queryModel(ctx, bedrockRequest)
	if err != nil {
		return
	}

	queryResponse = Message{
		Content: bedrockResponse.Completion,
		Role:    "assistant",
	}
	return
}

func (bedrockModel *BedrockModel) makeBedrockChatRequest(ctx context.Context, chatRequest ChatRequest) (bedrockRequest BedrockChatRequest, err error) {
	logs.WithContext(ctx).Debug("makeBedrockChatRequest - Start")

	// Combine messages into a prompt
	var prompt string
	for _, msg := range chatRequest.Messages {
		prompt += fmt.Sprintf("\n\n%s: %s", msg.Role, msg.Content)
	}

	bedrockRequest = BedrockChatRequest{
		Prompt:        prompt,
		MaxTokens:     1024,
		Temperature:   bedrockModel.Temprature,
		TopP:          0.9,
		TopK:          250,
		StopSequences: []string{"\n\nHuman:", "\n\nAssistant:"},
	}
	return
}

func (bedrockModel *BedrockModel) queryModel(ctx context.Context, chatRequest BedrockChatRequest) (bedrockResponse BedrockChatResponse, err error) {
	logs.WithContext(ctx).Debug("queryModel - Start")

	// Convert request to JSON
	payload, err := json.Marshal(chatRequest)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	// Create Bedrock invoke request
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     &bedrockModel.ModelName,
		Body:        payload,
		ContentType: aws.String("application/json"),
	}

	// Call Bedrock
	output, err := bedrockModel.client.InvokeModel(ctx, input)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	// Parse response
	err = json.Unmarshal(output.Body, &bedrockResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	return
}

func (bedrockModel *BedrockModel) QueryModelWithTool(ctx context.Context, chatRequest ChatRequest, tools map[string]tools.Tooling, agentName string, agentPrompt string) (response JsonMessage, err error) {
	err = fmt.Errorf("tool support not yet implemented for AWS Bedrock")
	logs.WithContext(ctx).Error(err.Error())
	return
}
