package messengers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"reflect"
	"strings"

	aes "github.com/eru-os/eru/eru-crypto/aes"
	rsa "github.com/eru-os/eru/eru-crypto/rsa"
	erusha "github.com/eru-os/eru/eru-crypto/sha"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	server "github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gabriel-vasile/mimetype"
)

const (
	INSERT_ENPOINT_REQUEST  = "insert into eruai_wa_endpoint (project_id, tenant_id, request_body,decrypted_request_body) values ($1, $2, $3, $4)"
	INSERT_CALLBACK_REQUEST = "insert into eruai_cb_whatsapp (project_id, tenant_id, waba_id, msg, msg_params, msg_from) values ($1, $2, $3, $4, $5, $6)"
	WHATSAPP_BASE_URL       = "https://graph.facebook.com"
)

type WhatsAppCreateTemplateRequest struct {
	Name       string                      `json:"name" eru:"required"`
	Category   string                      `json:"category" eru:"required"`
	Language   string                      `json:"language" eru:"required"`
	Components []WhatsAppTemplateComponent `json:"components" eru:"required"`
}

type WhatsAppEditTemplateRequest struct {
	Components []WhatsAppTemplateComponent `json:"components" eru:"required"`
}

type WhatsAppTemplateComponent struct {
	Type      string                   `json:"type" eru:"required"`
	Format    string                   `json:"format,omitempty"`
	Text      string                   `json:"text,omitempty"`
	Buttons   []WhatsAppTemplateButton `json:"buttons,omitempty"`
	Example   *WhatsAppTemplateExample `json:"example,omitempty"`
	CardIndex *int                     `json:"card_index,omitempty"`
	Cards     []interface{}            `json:"cards,omitempty"` // Can be refined if needed
}

type WhatsAppTemplateButton struct {
	Type           string   `json:"type" eru:"required"`
	Text           string   `json:"text,omitempty"`
	PhoneNumber    string   `json:"phone_number,omitempty"`
	Url            string   `json:"url,omitempty"`
	OtpType        string   `json:"otp_type,omitempty"`
	Autofill       *bool    `json:"autofill,omitempty"`
	Example        []string `json:"example,omitempty"`
	FlowAction     string   `json:"flow_action,omitempty"`
	FlowId         int64    `json:"flow_id,omitempty"`
	NavigateScreen string   `json:"navigate_screen,omitempty"`
}

type WhatsAppTemplateExample struct {
	HeaderHandle []string   `json:"header_handle,omitempty"`
	HeaderText   []string   `json:"header_text,omitempty"`
	BodyText     [][]string `json:"body_text,omitempty"`
}

type WhatsAppDownloadFlowDocumentRequest struct {
	CdnUrl             string                     `json:"cdn_url" eru:"required"`
	MediaId            string                     `json:"media_id"`
	FileName           string                     `json:"file_name" eru:"required"`
	EncryptionMetadata WhatsAppEncryptionMetadata `json:"encryption_metadata" eru:"required"`
}

type WhatsAppEncryptionMetadata struct {
	Iv            string `json:"iv" eru:"required"`
	HmacKey       string `json:"hmac_key" eru:"required"`
	EncryptedHash string `json:"encrypted_hash" eru:"required"`
	EncryptionKey string `json:"encryption_key" eru:"required"`
	PlaintextHash string `json:"plaintext_hash" eru:"required"`
}

type WhatsAppTool struct {
	tools.Tool
	WhatsAppAccount WhatsAppAccount `json:"whatsapp_account"`
}

type WhatsAppAccount struct {
	PhoneNumberId            string `json:"phone_number_id" eru:"required"`
	BusinessAccountId        string `json:"business_account_id"`
	AppId                    string `json:"app_id"`
	ApiKey                   string `json:"api_key" secret:"true" eru:"required"`
	WebhookUrl               string `json:"webhook_url"`
	ApiVersion               string `json:"api_version"`
	PrivateKey               string `json:"private_key" secret:"true"`
	WebhookSubscriptionToken string `json:"webhook_subscription_token" secret:"true"`
}

func (whatsAppTool *WhatsAppTool) GetActionsList() []tools.ActionInfo {
	return []tools.ActionInfo{
		{Name: SendMessage},
		{Name: SubscribeWebhooks},
		{Name: GetMessageStatus},
		{Name: UploadMedia},
		{Name: ResumableUpload},
		{Name: RetrieveMedia},
		{Name: DeleteMedia},
		{Name: GetMediaUrl},
		{Name: GetBusinessProfile},
		{Name: GetMessageTemplates},
		{Name: MarkMessageAsRead},
		{Name: SendTypingIndicator},
		{Name: GetThroughput},
		{Name: CreateGroup},
		{Name: RegisterPublicKey},
		{Name: FetchPublicKey},
		{Name: FlowEndpoint},
		{Name: SaveMessageTemplate},
		{Name: DownloadFlowDocument},
		{Name: FetchTemplates},
		{Name: DeleteMessageTemplate},
		{Name: Callback},
	}
}

func (whatsAppTool *WhatsAppTool) GetSpec() tools.Tooling {
	return whatsAppTool
}

func (whatsAppTool *WhatsAppTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &whatsAppTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (whatsAppTool *WhatsAppTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("WhatsAppTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case SendMessage:
		toolResult, toolRequest, persistStore, err = whatsAppTool.SendMessage(ctx, params)
	case SubscribeWebhooks:
		toolResult, toolRequest, persistStore, err = whatsAppTool.SubscribeWebhooks(ctx, projectId, tenantId, params)
	case GetMessageStatus:
		toolResult, toolRequest, persistStore, err = whatsAppTool.GetMessageStatus(ctx, params)
	case UploadMedia:
		toolResult, toolRequest, persistStore, err = whatsAppTool.UploadMedia(ctx, params)
	case ResumableUpload:
		toolResult, toolRequest, persistStore, err = whatsAppTool.ResumableUpload(ctx, params)
	case RetrieveMedia:
		toolResult, toolRequest, persistStore, err = whatsAppTool.RetrieveMedia(ctx, params)
	case DeleteMedia:
		toolResult, toolRequest, persistStore, err = whatsAppTool.DeleteMedia(ctx, params)
	case GetMediaUrl:
		toolResult, toolRequest, persistStore, err = whatsAppTool.GetMediaUrl(ctx, params)
	case GetBusinessProfile:
		toolResult, toolRequest, persistStore, err = whatsAppTool.GetBusinessProfile(ctx, params)
	case GetMessageTemplates:
		toolResult, toolRequest, persistStore, err = whatsAppTool.GetMessageTemplates(ctx, params)
	case MarkMessageAsRead:
		toolResult, toolRequest, persistStore, err = whatsAppTool.MarkMessageAsRead(ctx, params)
	case SendTypingIndicator:
		toolResult, toolRequest, persistStore, err = whatsAppTool.SendTypingIndicator(ctx, params)
	case GetThroughput:
		toolResult, toolRequest, persistStore, err = whatsAppTool.GetThroughput(ctx, params)
	case CreateGroup:
		toolResult, toolRequest, persistStore, err = whatsAppTool.CreateGroup(ctx, params)
	case RegisterPublicKey:
		toolResult, toolRequest, persistStore, err = whatsAppTool.RegisterPublicKey(ctx, params)
	case SaveMessageTemplate:
		toolResult, toolRequest, persistStore, err = whatsAppTool.SaveMessageTemplate(ctx, params)
	case FetchPublicKey:
		toolResult, toolRequest, persistStore, err = whatsAppTool.FetchPublicKey(ctx, params)
	case FlowEndpoint:
		toolResult, toolRequest, persistStore, err = whatsAppTool.FlowEndpoint(ctx, params, projectId, tenantId)
	case DownloadFlowDocument:
		toolResult, toolRequest, persistStore, err = whatsAppTool.DownloadFlowDocument(ctx, params)
	case FetchTemplates:
		toolResult, toolRequest, persistStore, err = whatsAppTool.FetchTemplates(ctx, params)
	case DeleteMessageTemplate:
		toolResult, toolRequest, persistStore, err = whatsAppTool.DeleteMessageTemplate(ctx, params)
	default:
		return nil, false, fmt.Errorf("action %s not found", actionName)
	}

	gm := server.GetGlobalGoroutineManager(ctx)
	gm.SafeGoWithRestartBehavior("tool-post-execute-hook", func(bgCtx context.Context) {
		bgCtx = tools.CopyClaims(ctx, bgCtx)
		efurl := ctx.Value(tools.EruFuncBaseUrlKey)
		if efurl == nil {
			err = errors.New("erufuncbaseurl not found in context")
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		efurlString, ok := efurl.(string)
		if !ok {
			err = errors.New("erufuncbaseurl is not a string")
			logs.WithContext(ctx).Error(err.Error())
			return
		} else {
			bgCtx = context.WithValue(bgCtx, tools.EruFuncBaseUrlKey, efurlString)
		}

		body := make(map[string]interface{})
		if toolRequest != nil {
			body["request"] = toolRequest
		}
		if toolResult != nil {
			body["response"] = toolResult
		}
		body["tenant_id"] = tenantId
		body["project_id"] = projectId

		if params["metadata"] != nil {
			body["metadata"] = params["metadata"]
		}

		hookResult, err := whatsAppTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (whatsAppTool *WhatsAppTool) SendMessage(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SendMessage Execute - Start")

	messageType, messageTypeOk := params["message_type"]
	if !messageTypeOk {
		err = logs.Err(ctx, errors.New("message_type parameter is required"), "message_type parameter is required")
		return nil, nil, false, err
	}
	messageTypeStr, messageTypeStrOk := messageType.(string)
	if !messageTypeStrOk {
		err = logs.Err(ctx, errors.New("message_type must be a string"), "message_type must be a string")
		return nil, nil, false, err
	}

	messageSubType, messageSubTypeOk := params["message_sub_type"]
	messageSubTypeStr := ""
	if messageSubTypeOk {
		messageSubTypeStr, _ = messageSubType.(string)
	}
	to, toOk := params["to"]
	if !toOk {
		err = logs.Err(ctx, errors.New("recipient phone number is required"), "recipient phone number is required")
		return nil, nil, false, err
	}
	toStr, toStrOk := to.(string)
	if !toStrOk {
		err = logs.Err(ctx, errors.New("recipient phone number must be a string"), "recipient phone number must be a string")
		return nil, nil, false, err
	}
	if toStr == "" {
		err = logs.Err(ctx, errors.New("recipient phone number cannot be empty"), "recipient phone number cannot be empty")
		return nil, nil, false, err
	}

	messagePayloadParams, messagePayloadParamsOk := params["message_payload"]
	if !messagePayloadParamsOk {
		err = logs.Err(ctx, errors.New("message_payload parameter is required"), "message_payload parameter is required")
		return nil, nil, false, err
	}

	messagePayloadParamsBytes, err := json.Marshal(messagePayloadParams)
	if err != nil {
		err = logs.Err(ctx, errors.New("failed to marshal message payload"), "failed to marshal message payload")
		return nil, nil, false, err
	}

	whatsAppMessagePayload := WhatsAppMessagePayload{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               toStr,
		Type:             messageTypeStr,
	}
	switch messageTypeStr {
	case "template":
		messagePayload := WhatsAppTemplateMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal template message payload: %s", err.Error()), "failed to unmarshal template message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Template = &messagePayload
		if whatsAppMessagePayload.Template.Name == "" || whatsAppMessagePayload.Template.Language.Code == "" {
			err = logs.Err(ctx, fmt.Errorf("incorrect template payload"), fmt.Sprintf("incorrect template payload: %s", err.Error()))
			return nil, nil, false, err
		}
	case "text":
		messagePayload := WhatsAppTextMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal text message payload: %s", err.Error()), "failed to unmarshal text message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Text = &messagePayload
		if whatsAppMessagePayload.Text.Body == "" {
			err = logs.Err(ctx, fmt.Errorf("incorrect text payload"), fmt.Sprintf("incorrect text payload: %s", err.Error()))
			return nil, nil, false, err
		}
	case "reaction":
		messagePayload := WhatsAppReactionMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal reaction message payload: %s", err.Error()), "failed to unmarshal reaction message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Reaction = &messagePayload
		if whatsAppMessagePayload.Reaction.MessageId == "" || whatsAppMessagePayload.Reaction.Emoji == "" {
			err = logs.Err(ctx, fmt.Errorf("incorrect reaction payload"), fmt.Sprintf("incorrect reaction payload: %s", err.Error()))
			return nil, nil, false, err
		}
	case "image":
		messagePayload := WhatsAppMediaMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal image message payload: %s", err.Error()), "failed to unmarshal image message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Image = &messagePayload
		if (whatsAppMessagePayload.Image.Id == "" && whatsAppMessagePayload.Image.Link == "") || (whatsAppMessagePayload.Image.Id != "" && whatsAppMessagePayload.Image.Link != "") {
			err = logs.Err(ctx, fmt.Errorf("either id or link is required in image payload"), fmt.Sprintf("either id or link is required in image payload: %s", err.Error()))
			return nil, nil, false, err
		}
	case "video":
		messagePayload := WhatsAppMediaMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal video message payload: %s", err.Error()), "failed to unmarshal video message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Video = &messagePayload
		if (whatsAppMessagePayload.Video.Id == "" && whatsAppMessagePayload.Video.Link == "") || (whatsAppMessagePayload.Video.Id != "" && whatsAppMessagePayload.Video.Link != "") {
			err = logs.Err(ctx, fmt.Errorf("either id or link is required in video payload"), fmt.Sprintf("either id or link is required in video payload: %s", err.Error()))
			return nil, nil, false, err
		}
	case "audio":
		messagePayload := WhatsAppMediaMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal audio message payload: %s", err.Error()), "failed to unmarshal audio message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Audio = &messagePayload
		if (whatsAppMessagePayload.Audio.Id == "" && whatsAppMessagePayload.Audio.Link == "") || (whatsAppMessagePayload.Audio.Id != "" && whatsAppMessagePayload.Audio.Link != "") {
			err = logs.Err(ctx, fmt.Errorf("either id or link is required in audio payload"), fmt.Sprintf("either id or link is required in audio payload: %s", err.Error()))
			return nil, nil, false, err
		}
	case "document":
		messagePayload := WhatsAppMediaMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal document message payload: %s", err.Error()), "failed to unmarshal document message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Document = &messagePayload
		if (whatsAppMessagePayload.Document.Id == "" && whatsAppMessagePayload.Document.Link == "") || (whatsAppMessagePayload.Document.Id != "" && whatsAppMessagePayload.Document.Link != "") {
			err = logs.Err(ctx, fmt.Errorf("either id or link is required in document payload"), fmt.Sprintf("either id or link is required in document payload: %s", err.Error()))
			return nil, nil, false, err
		}
	case "sticker":
		messagePayload := WhatsAppMediaMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal sticker message payload: %s", err.Error()), "failed to unmarshal sticker message payload")
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Sticker = &messagePayload
		if (whatsAppMessagePayload.Sticker.Id == "" && whatsAppMessagePayload.Sticker.Link == "") || (whatsAppMessagePayload.Sticker.Id != "" && whatsAppMessagePayload.Sticker.Link != "") {
			err = logs.Err(ctx, fmt.Errorf("either id or link is required in sticker payload"), fmt.Sprintf("either id or link is required in sticker payload: %s", err.Error()))
			return nil, nil, false, err
		}
	case "contacts":
		messagePayload := []WhatsAppContactMessagePayload{}

		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal contact message payload: %s", err.Error()), "failed to unmarshal contact message payload")
			return nil, nil, false, err
		}

		whatsAppMessagePayload.Contacts = messagePayload
		for _, contact := range whatsAppMessagePayload.Contacts {
			if contact.Name.FormattedName == "" {
				err = logs.Err(ctx, fmt.Errorf("formatted name is required in contact payload"), fmt.Sprintf("formatted name is required in contact payload: %s", err.Error()))
				return nil, nil, false, err
			}
		}
	case "location":
		messagePayload := WhatsAppLocationMessagePayload{}
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal location message payload: %s", err.Error()), "failed to unmarshal location message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Location = &messagePayload

	case "interactive":
		if messageSubTypeStr == "" {
			err = logs.Err(ctx, fmt.Errorf("message_sub_type parameter is required"), "message_sub_type parameter is required")
			return nil, nil, false, err
		}
		var messagePayload WhatsAppInteractiveMessagePayload
		err = json.Unmarshal(messagePayloadParamsBytes, &messagePayload)
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to unmarshal interactive message payload: %s", err.Error()), "failed to unmarshal interactive message payload")
			return nil, nil, false, err
		}
		err = utils.ValidateStruct(ctx, messagePayload, "")
		if err != nil {
			err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
			return nil, nil, false, err
		}
		whatsAppMessagePayload.Interactive = &messagePayload
		switch messageSubTypeStr {
		case "cta_url":
			if whatsAppMessagePayload.Interactive.Action.Name == "" || whatsAppMessagePayload.Interactive.Action.Parameters == nil || whatsAppMessagePayload.Interactive.Action.Parameters.DisplayText == "" || whatsAppMessagePayload.Interactive.Action.Parameters.Url == "" {
				err = logs.Err(ctx, fmt.Errorf("name , display text and url are required in action payload"), "name , display text and url are required in action payload")
				return nil, nil, false, err
			}
		case "list":
			if whatsAppMessagePayload.Interactive.Action.Button == "" {
				err = logs.Err(ctx, fmt.Errorf("button is required in action payload"), "button is required in action payload")
				return nil, nil, false, err
			}
			if len(whatsAppMessagePayload.Interactive.Action.Sections) == 0 {
				err = logs.Err(ctx, fmt.Errorf("sections are required in action payload"), "sections are required in action payload")
				return nil, nil, false, err
			}
			for _, section := range whatsAppMessagePayload.Interactive.Action.Sections {
				if section.Title == "" {
					err = logs.Err(ctx, fmt.Errorf("title is required in section payload"), "title is required in section payload")
					return nil, nil, false, err
				}
				if len(section.Rows) == 0 {
					err = logs.Err(ctx, fmt.Errorf("rows are required in section payload"), "rows are required in section payload")
					return nil, nil, false, err
				}
				for _, row := range section.Rows {
					if row.Id == "" || row.Title == "" || row.Description == "" {
						err = logs.Err(ctx, fmt.Errorf("id, title and description are required in row payload"), "id, title and description are required in row payload")
						return nil, nil, false, err
					}
				}
			}
		case "carousel":
			if len(whatsAppMessagePayload.Interactive.Action.Cards) < 2 || len(whatsAppMessagePayload.Interactive.Action.Cards) > 10 {
				err = logs.Err(ctx, fmt.Errorf("cards must be between 2 and 10"), "cards must be between 2 and 10")
				return nil, nil, false, err
			}
			for _, card := range whatsAppMessagePayload.Interactive.Action.Cards {
				if card.CardIndex < 0 || card.CardIndex > 9 || card.Type == "" {
					err = logs.Err(ctx, fmt.Errorf("card index (0-9) and type are required in card payload"), "card index, type are required in card payload")
					return nil, nil, false, err
				}
				if card.Type == "cta_url" && (card.Body == nil || card.Body.Text == "" || card.Action == nil || card.Action.Name == "" || card.Action.Parameters == nil || card.Action.Parameters.DisplayText == "" || card.Action.Parameters.Url == "") {
					err = logs.Err(ctx, fmt.Errorf("body text, action name, display text and url are required in action payload"), "body text, action name, display text and url are required in action payload")
					return nil, nil, false, err
				}
				if card.Type == "product" && (card.Action == nil || card.Action.ProductRetailerId == "" || card.Action.CatalogId == "") {
					err = logs.Err(ctx, fmt.Errorf("product retailer id, catalog id are required in action payload"), "product retailer id, catalog id are required in action payload")
					return nil, nil, false, err
				}
			}
		case "button":
			if len(whatsAppMessagePayload.Interactive.Action.Buttons) == 0 || len(whatsAppMessagePayload.Interactive.Action.Buttons) > 3 {
				err = logs.Err(ctx, fmt.Errorf("buttons must be between 1 and 3"), "buttons must be between 1 and 3")
				return nil, nil, false, err
			}
			for _, button := range whatsAppMessagePayload.Interactive.Action.Buttons {
				if button.Type == "" || button.Reply == nil || button.Reply.Id == "" || button.Reply.Title == "" {
					err = logs.Err(ctx, fmt.Errorf("type, id and title are required in button payload"), "type, id and title are required in button payload")
					return nil, nil, false, err
				}
			}
		}
	case "location_request_message":
		if whatsAppMessagePayload.Interactive.Action.Name != "send_location" {
			err = logs.Err(ctx, fmt.Errorf("name must be send_location in action payload"), "name must be send_location in action payload")
			return nil, nil, false, err
		}
	case "address_message":
		if whatsAppMessagePayload.Interactive.Action.Name != "address_message" {
			err = logs.Err(ctx, fmt.Errorf("name must be address_message in action payload"), "name must be send_address in action payload")
			return nil, nil, false, err
		}
		if whatsAppMessagePayload.Interactive.Action.Parameters == nil || whatsAppMessagePayload.Interactive.Action.Parameters.Country == "" {
			err = logs.Err(ctx, fmt.Errorf("country is required in parameters payload"), "country is required in parameters payload")
			return nil, nil, false, err
		}
	}
	err = utils.ValidateStruct(ctx, whatsAppMessagePayload, "")
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("incorrect message payload: %s", err.Error()), fmt.Sprintf("incorrect message payload: %s", err.Error()))
		return nil, nil, false, err
	}

	whatsAppMessagePayloadBytes, err := json.Marshal(whatsAppMessagePayload)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to marshal message payload: %s", err.Error()), "failed to marshal message payload")
		return nil, nil, false, err
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Sending message payload: %s", string(whatsAppMessagePayloadBytes)))

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s/messages", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, whatsAppMessagePayload)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to send message: %s", err.Error()), "failed to send message")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if resMap, resMapOk := res.(map[string]interface{}); resMapOk {
		toolResult["response"] = resMap
		toolResult["status"] = "sent"

		if messages, messagesOk := resMap["messages"].([]interface{}); messagesOk && len(messages) > 0 {
			if messageMap, messageMapOk := messages[0].(map[string]interface{}); messageMapOk {
				if messageId, messageIdOk := messageMap["id"]; messageIdOk {
					toolResult["message_id"] = messageId
				}
			}
		}
	} else {
		toolResult["response"] = res
		toolResult["status"] = "sent"
	}

	return toolResult, map[string]interface{}{"body": whatsAppMessagePayload}, false, nil
}

func (whatsAppTool *WhatsAppTool) SubscribeWebhooks(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SubscribeWebhooks Execute - Start")

	webhookUrl := whatsAppTool.GetToolCbUrl(projectId, tenantId)
	logs.WithContext(ctx).Info(fmt.Sprintf("Webhook URL: %s", webhookUrl))

	toolResult = make(map[string]interface{})
	toolResult["webhook_url"] = webhookUrl
	toolResult["verification_token"] = whatsAppTool.WhatsAppAccount.WebhookUrl
	toolResult["status"] = "configured"
	toolResult["instructions"] = "Configure this webhook URL in your WhatsApp Business API settings with the provided verification token"

	return toolResult, map[string]interface{}{"body": params}, false, nil
}

func (whatsAppTool *WhatsAppTool) GetMessageStatus(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetMessageStatus Execute - Start")

	messageId, messageIdOk := params["message_id"]
	if !messageIdOk {
		err = logs.Err(ctx, fmt.Errorf("message_id parameter is required"), fmt.Sprintf("message_id parameter is required: %s", err.Error()))
		return nil, nil, false, err
	}

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s", WHATSAPP_BASE_URL, apiVersion, messageId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to get message status: %s", err.Error()), "failed to get message status")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["message_status"] = res

	return toolResult, map[string]interface{}{"query": map[string]string{"message_id": fmt.Sprint(messageId)}}, false, nil
}

func (whatsAppTool *WhatsAppTool) UploadMedia(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("UploadMedia Execute - Start")

	mimeLimit := uint32(2000)
	mimeLimitParam, mimeLimitParamOk := params["mime_limit"]
	if mimeLimitParamOk {
		mimeLimitInt, ok := mimeLimitParam.(uint32)
		if ok {
			mimeLimit = mimeLimitInt
		}
	}

	mediaFile, mediaFileOk := params["file"]
	if !mediaFileOk {
		err = logs.Err(ctx, fmt.Errorf("file parameter is required (base64 encoded content)"), "file parameter is required (base64 encoded content)")
		return nil, nil, false, err
	}

	mediaFileStr, ok := mediaFile.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("file must be a base64 encoded string"), "file must be a base64 encoded string")
		return nil, nil, false, err
	}

	mediaFileName, mediaFileNameOk := params["file_name"]
	if !mediaFileNameOk {
		err = logs.Err(ctx, fmt.Errorf("file_name parameter is required"), "file_name parameter is required")
		return nil, nil, false, err
	}
	mediaFileNameStr, ok := mediaFileName.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("file_name must be a string"), "file_name must be a string")
		return nil, nil, false, err
	}
	fileBytes, err := base64.StdEncoding.DecodeString(mediaFileStr)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to decode base64 file: %s", err.Error()), "failed to decode base64 file")
		return nil, nil, false, err
	}
	mimetype.SetLimit(mimeLimit)
	fMime := mimetype.Detect(fileBytes)
	logs.WithContext(ctx).Info(fmt.Sprintf("File MIME: %s", fMime.String()))
	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s/media", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)

	var reqBody bytes.Buffer
	multipartWriter := multipart.NewWriter(&reqBody)

	typeField, err := multipartWriter.CreateFormField("type")
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create type field: %s", err.Error()), "failed to create type field")
		return nil, nil, false, err
	}
	_, err = typeField.Write([]byte(fMime.String()))
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to write type field: %s", err.Error()), "failed to write type field")
		return nil, nil, false, err
	}

	messagingProductField, err := multipartWriter.CreateFormField("messaging_product")
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create messaging_product field: %s", err.Error()), "failed to create messaging_product field")
		return nil, nil, false, err
	}
	_, err = messagingProductField.Write([]byte("whatsapp"))
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to write messaging_product field: %s", err.Error()), "failed to write messaging_product field")
		return nil, nil, false, err
	}

	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, mediaFileNameStr))
	fileHeader.Set("Content-Type", fMime.String())

	fileWriter, err := multipartWriter.CreatePart(fileHeader)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create file field: %s", err.Error()), "failed to create file field")
		return nil, nil, false, err
	}
	_, err = io.Copy(fileWriter, bytes.NewReader(fileBytes))
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to write file: %s", err.Error()), "failed to write file")
		return nil, nil, false, err
	}

	err = multipartWriter.Close()
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to close multipart writer: %s", err.Error()), "failed to close multipart writer")
		return nil, nil, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &reqBody)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create request: %s", err.Error()), "failed to create request")
		return nil, nil, false, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to upload media: %s", err.Error()), "failed to upload media")
		return nil, nil, false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to read response: %s", err.Error()), "failed to read response")
		return nil, nil, false, err
	}

	if resp.StatusCode != http.StatusOK {
		err = logs.Err(ctx, fmt.Errorf("media upload failed with status %d: %s", resp.StatusCode, string(body)), "media upload failed")
		return nil, nil, false, err
	}

	var uploadResponse map[string]interface{}
	err = json.Unmarshal(body, &uploadResponse)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to parse response: %s", err.Error()), "failed to parse response")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["response"] = uploadResponse
	if id, ok := uploadResponse["id"]; ok {
		toolResult["media_id"] = id
	}

	return toolResult, map[string]interface{}{
		"body": map[string]interface{}{
			"file_name":         mediaFileNameStr,
			"type":              fMime.String(),
			"messaging_product": "whatsapp",
		},
	}, false, nil
}

func (whatsAppTool *WhatsAppTool) ResumableUpload(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ResumableUpload Execute - Start")

	mimeLimit := uint32(2000)
	mimeLimitParam, mimeLimitParamOk := params["mime_limit"]
	if mimeLimitParamOk {
		mimeLimitInt, ok := mimeLimitParam.(uint32)
		if ok {
			mimeLimit = mimeLimitInt
		}
	}

	appIdStr := whatsAppTool.WhatsAppAccount.AppId
	if appIdStr == "" {
		err = logs.Err(ctx, fmt.Errorf("app_id is required in tool definition"), "app_id is required in tool definition")
		return nil, nil, false, err
	}

	mediaFile, mediaFileOk := params["file"]
	if !mediaFileOk {
		err = logs.Err(ctx, fmt.Errorf("file parameter is required (base64 encoded content)"), "file parameter is required (base64 encoded content)")
		return nil, nil, false, err
	}
	mediaFileStr, ok := mediaFile.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("file must be a base64 encoded string"), "file must be a base64 encoded string")
		return nil, nil, false, err
	}

	mediaFileName, mediaFileNameOk := params["file_name"]
	if !mediaFileNameOk {
		err = logs.Err(ctx, fmt.Errorf("file_name parameter is required"), "file_name parameter is required")
		return nil, nil, false, err
	}
	mediaFileNameStr, ok := mediaFileName.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("file_name must be a string"), "file_name must be a string")
		return nil, nil, false, err
	}

	fileBytes, err := base64.StdEncoding.DecodeString(mediaFileStr)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to decode base64 file: %s", err.Error()), "failed to decode base64 file")
		return nil, nil, false, err
	}

	fileType := ""
	if fileTypeParam, fileTypeParamOk := params["file_type"]; fileTypeParamOk {
		fileType, _ = fileTypeParam.(string)
	}
	if fileType == "" {
		mimetype.SetLimit(mimeLimit)
		fMime := mimetype.Detect(fileBytes)
		fileType = fMime.String()
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("File type: %s, size: %d", fileType, len(fileBytes)))

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	sessionUrl := fmt.Sprintf("%s/%s/%s/uploads", WHATSAPP_BASE_URL, apiVersion, appIdStr)
	sessionQueryParams := map[string]string{
		"file_name":    mediaFileNameStr,
		"file_length":  fmt.Sprintf("%d", len(fileBytes)),
		"file_type":    fileType,
		"access_token": whatsAppTool.WhatsAppAccount.ApiKey,
	}
	sessionHeaders := http.Header{}
	sessionHeaders.Set("Accept", "application/json")

	sessionRes, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, sessionUrl, sessionHeaders, map[string]string{}, []*http.Cookie{}, sessionQueryParams, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create upload session: %s", err.Error()), "failed to create upload session")
		return nil, nil, false, err
	}

	sessionResMap, sessionResMapOk := sessionRes.(map[string]interface{})
	if !sessionResMapOk {
		err = logs.Err(ctx, fmt.Errorf("unexpected upload session response"), "unexpected upload session response")
		return nil, nil, false, err
	}
	uploadSessionId, uploadSessionIdOk := sessionResMap["id"].(string)
	if !uploadSessionIdOk || uploadSessionId == "" {
		err = logs.Err(ctx, fmt.Errorf("upload session id not found in response"), "upload session id not found in response")
		return nil, nil, false, err
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Upload session id: %s", uploadSessionId))

	uploadUrl := fmt.Sprintf("%s/%s/%s", WHATSAPP_BASE_URL, apiVersion, uploadSessionId)
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadUrl, bytes.NewReader(fileBytes))
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create upload request: %s", err.Error()), "failed to create upload request")
		return nil, nil, false, err
	}
	uploadReq.Header.Set("Authorization", fmt.Sprintf("OAuth %s", whatsAppTool.WhatsAppAccount.ApiKey))
	uploadReq.Header.Set("file_offset", "0")

	client := &http.Client{}
	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to upload file: %s", err.Error()), "failed to upload file")
		return nil, nil, false, err
	}
	defer uploadResp.Body.Close()

	uploadBody, err := io.ReadAll(uploadResp.Body)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to read upload response: %s", err.Error()), "failed to read upload response")
		return nil, nil, false, err
	}

	if uploadResp.StatusCode != http.StatusOK {
		err = logs.Err(ctx, fmt.Errorf("resumable upload failed with status %d: %s", uploadResp.StatusCode, string(uploadBody)), "resumable upload failed")
		return nil, nil, false, err
	}

	var uploadResponse map[string]interface{}
	err = json.Unmarshal(uploadBody, &uploadResponse)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to parse upload response: %s", err.Error()), "failed to parse upload response")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	//toolResult["response"] = uploadResponse
	//toolResult["upload_session_id"] = uploadSessionId
	if h, hOk := uploadResponse["h"]; hOk {
		toolResult["file_handle"] = h
	}

	return toolResult, map[string]interface{}{
		"body": map[string]interface{}{
			"app_id":      appIdStr,
			"file_name":   mediaFileNameStr,
			"file_type":   fileType,
			"file_length": len(fileBytes),
		},
	}, false, nil
}

func (whatsAppTool *WhatsAppTool) RetrieveMedia(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("RetrieveMedia Execute - Start")

	mediaUrl, urlOk := params["url"]
	if !urlOk {
		err = logs.Err(ctx, fmt.Errorf("url parameter is required"), "url parameter is required")
		return nil, nil, false, err
	}

	mediaUrlStr, ok := mediaUrl.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("url must be a string"), "url must be a string")
		return nil, nil, false, err
	}

	if mediaUrlStr == "" {
		err = logs.Err(ctx, fmt.Errorf("url cannot be empty"), "url cannot be empty")
		return nil, nil, false, err
	}

	expectedSha256, shaOk := params["sha256"]
	expectedSha256Str := ""
	if shaOk {
		if shaStr, ok := expectedSha256.(string); ok {
			expectedSha256Str = shaStr
		}
	}

	mimeType, mimeOk := params["mime_type"]
	mimeTypeStr := ""
	if mimeOk {
		if mimeStr, ok := mimeType.(string); ok {
			mimeTypeStr = mimeStr
		}
	}

	downloadHeaders := http.Header{}
	downloadHeaders.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))

	downloadReq, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaUrlStr, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create download request: %s", err.Error()), "failed to create download request")
		return nil, nil, false, err
	}
	downloadReq.Header = downloadHeaders

	client := &http.Client{}
	downloadResp, err := client.Do(downloadReq)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to download media: %s", err.Error()), "failed to download media")
		return nil, nil, false, err
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		err = logs.Err(ctx, fmt.Errorf("media download failed with status %d", downloadResp.StatusCode), "media download failed")
		return nil, nil, false, err
	}

	fileBytes, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to read downloaded file: %s", err.Error()), "failed to read downloaded file")
		return nil, nil, false, err
	}

	hash := erusha.NewSHA256(fileBytes)
	calculatedSha256 := hex.EncodeToString(hash[:])

	if shaOk && expectedSha256Str != "" {
		if !strings.EqualFold(calculatedSha256, expectedSha256Str) {
			err = logs.Err(ctx, fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedSha256Str, calculatedSha256), "SHA256 verification failed")
			return nil, nil, false, err
		}
	}

	detectedMime := mimetype.Detect(fileBytes)
	finalMimeType := mimeTypeStr
	if !mimeOk || mimeTypeStr == "" {
		finalMimeType = detectedMime.String()
	}

	fileName := "media"
	ext := detectedMime.Extension()
	if ext != "" {
		fileName = "media" + ext
	}

	fileBase64 := base64.StdEncoding.EncodeToString(fileBytes)

	toolResult = make(map[string]interface{})
	toolResult["file_content"] = fileBase64
	toolResult["file_name"] = fileName
	toolResult["mime_type"] = finalMimeType
	toolResult["sha256"] = calculatedSha256
	toolResult["file_size"] = len(fileBytes)
	toolResult["media_url"] = mediaUrlStr

	return toolResult, map[string]interface{}{"query": map[string]string{"url": mediaUrlStr}}, false, nil
}

func (whatsAppTool *WhatsAppTool) DeleteMedia(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("DeleteMedia Execute - Start")

	mediaId, mediaIdOk := params["media_id"]
	if !mediaIdOk {
		err = logs.Err(ctx, fmt.Errorf("media_id parameter is required"), "media_id parameter is required")
		return nil, nil, false, err
	}

	mediaIdStr, ok := mediaId.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("media_id must be a string"), "media_id must be a string")
		return nil, nil, false, err
	}

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s", WHATSAPP_BASE_URL, apiVersion, mediaIdStr)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodDelete, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to delete media: %s", err.Error()), "failed to delete media")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if statusCode == http.StatusOK || statusCode == http.StatusNoContent {
		toolResult["status"] = "deleted"
		toolResult["media_id"] = mediaIdStr
	} else {
		toolResult["response"] = res
		toolResult["status_code"] = statusCode
	}

	return toolResult, map[string]interface{}{"query": map[string]string{"media_id": mediaIdStr}}, false, nil
}

func (whatsAppTool *WhatsAppTool) GetMediaUrl(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetMediaUrl Execute - Start")

	mediaId, mediaIdOk := params["media_id"]
	if !mediaIdOk {
		err = logs.Err(ctx, fmt.Errorf("media_id parameter is required"), "media_id parameter is required")
		return nil, nil, false, err
	}

	mediaIdStr, ok := mediaId.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("media_id must be a string"), "media_id must be a string")
		return nil, nil, false, err
	}

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s", WHATSAPP_BASE_URL, apiVersion, mediaIdStr)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to get media URL: %s", err.Error()), "failed to get media URL")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if statusCode == http.StatusOK {
		if resMap, ok := res.(map[string]interface{}); ok {
			mediaUrl, urlOk := resMap["url"].(string)
			expectedSha256, shaOk := resMap["sha256"].(string)
			mimeType, mimeOk := resMap["mime_type"].(string)

			if !urlOk || mediaUrl == "" {
				err = logs.Err(ctx, fmt.Errorf("media URL not found in response"), "media URL not found in response")
				return nil, nil, false, err
			}

			retrieveParams := map[string]interface{}{
				"url": mediaUrl,
			}
			if shaOk && expectedSha256 != "" {
				retrieveParams["sha256"] = expectedSha256
			}
			if mimeOk && mimeType != "" {
				retrieveParams["mime_type"] = mimeType
			}

			retrieveResult, _, _, retrieveErr := whatsAppTool.RetrieveMedia(ctx, retrieveParams)
			if retrieveErr != nil {
				return nil, nil, false, retrieveErr
			}

			toolResult = retrieveResult
		} else {
			toolResult["response"] = res
		}
	} else {
		toolResult["response"] = res
		toolResult["status_code"] = statusCode
	}

	return toolResult, map[string]interface{}{"query": map[string]string{"media_id": mediaIdStr}}, false, nil
}

func (whatsAppTool *WhatsAppTool) GetBusinessProfile(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetBusinessProfile Execute - Start")

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s/whatsapp_business_profile", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to get business profile: %s", err.Error()), "failed to get business profile")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["business_profile"] = res

	return toolResult, map[string]interface{}{"body": params}, false, nil
}

func (whatsAppTool *WhatsAppTool) GetMessageTemplates(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetMessageTemplates Execute - Start")

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	// WhatsApp Business Account ID is required for templates endpoint
	businessAccountId := whatsAppTool.WhatsAppAccount.BusinessAccountId
	if businessAccountId == "" {
		err = logs.Err(ctx, fmt.Errorf("business_account_id is required to retrieve message templates"), fmt.Sprintf("business_account_id is required to retrieve message templates: %s", err.Error()))
		return nil, nil, false, err
	}

	url := fmt.Sprintf("%s/%s/%s/message_templates", WHATSAPP_BASE_URL, apiVersion, businessAccountId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	queryParams := map[string]string{}

	// Optional parameters
	if limit, limitOk := params["limit"]; limitOk {
		queryParams["limit"] = fmt.Sprintf("%v", limit)
	}
	if status, statusOk := params["status"]; statusOk {
		queryParams["status"] = fmt.Sprintf("%v", status) // APPROVED, PENDING, REJECTED
	}
	if category, categoryOk := params["category"]; categoryOk {
		queryParams["category"] = fmt.Sprintf("%v", category) // AUTHENTICATION, MARKETING, UTILITY
	}
	if language, languageOk := params["language"]; languageOk {
		queryParams["language"] = fmt.Sprintf("%v", language) // e.g., en_US, es_ES
	}
	if name, nameOk := params["name"]; nameOk {
		queryParams["name"] = fmt.Sprintf("%v", name) // Template name filter
	}

	allTemplates := make([]interface{}, 0)
	err = whatsAppTool.fetchTemplatesRecursive(ctx, url, headers, queryParams, &allTemplates)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to get message templates: %s", err.Error()), "failed to get message templates")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["message_templates"] = allTemplates

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (whatsAppTool *WhatsAppTool) FetchTemplates(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("FetchTemplates Execute - Start")

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")

	// 1. Fetch by ID
	if templateId, ok := params["id"].(string); ok && templateId != "" {
		url := fmt.Sprintf("%s/%s/%s", WHATSAPP_BASE_URL, apiVersion, templateId)
		res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, nil, nil)
		if err != nil {
			return nil, nil, false, logs.Err(ctx, err, "failed to fetch template by id")
		}
		toolResult = make(map[string]interface{})
		toolResult["templates"] = []interface{}{res}
		return toolResult, map[string]interface{}{"query": map[string]string{"id": templateId}}, false, nil
	}

	// 2. Fetch by Name or All
	businessAccountId := whatsAppTool.WhatsAppAccount.BusinessAccountId
	if businessAccountId == "" {
		return nil, nil, false, logs.Err(ctx, errors.New("business_account_id is required"), "business_account_id is required")
	}

	url := fmt.Sprintf("%s/%s/%s/message_templates", WHATSAPP_BASE_URL, apiVersion, businessAccountId)
	queryParams := map[string]string{}

	if name, ok := params["name"].(string); ok && name != "" {
		queryParams["name"] = name
	}
	if fields, ok := params["fields"].(string); ok && fields != "" {
		queryParams["fields"] = fields
	}
	if status, ok := params["status"].(string); ok && status != "" {
		queryParams["status"] = status
	}
	if limit, ok := params["limit"]; ok {
		queryParams["limit"] = fmt.Sprintf("%v", limit)
	}

	allTemplates := make([]interface{}, 0)
	err = whatsAppTool.fetchTemplatesRecursive(ctx, url, headers, queryParams, &allTemplates)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to fetch templates")
	}

	toolResult = make(map[string]interface{})
	toolResult["templates"] = allTemplates
	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (whatsAppTool *WhatsAppTool) fetchTemplatesRecursive(ctx context.Context, url string, headers http.Header, queryParams map[string]string, allTemplates *[]interface{}) error {
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		return err
	}

	resMap, ok := res.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format")
	}

	if data, exists := resMap["data"]; exists {
		if templatesArray, ok := data.([]interface{}); ok {
			*allTemplates = append(*allTemplates, templatesArray...)
		}
	}

	if paging, exists := resMap["paging"].(map[string]interface{}); exists {
		if cursors, exists := paging["cursors"].(map[string]interface{}); exists {
			if after, exists := cursors["after"].(string); exists && after != "" {
				nextQueryParams := make(map[string]string)
				for k, v := range queryParams {
					nextQueryParams[k] = v
				}
				nextQueryParams["after"] = after
				return whatsAppTool.fetchTemplatesRecursive(ctx, url, headers, nextQueryParams, allTemplates)
			}
		}
	}

	return nil
}

func (whatsAppTool *WhatsAppTool) MarkMessageAsRead(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("MarkMessageAsRead Execute - Start")

	messageId, messageIdOk := params["message_id"]
	if !messageIdOk {
		err = logs.Err(ctx, fmt.Errorf("message_id parameter is required"), "message_id parameter is required")
		return nil, nil, false, err
	}

	messageIdStr, ok := messageId.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("message_id must be a string"), "message_id must be a string")
		return nil, nil, false, err
	}

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s/messages", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageIdStr,
	}

	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, payload)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to mark message as read: %s", err.Error()), "failed to mark message as read")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if statusCode == http.StatusOK {
		if resMap, ok := res.(map[string]interface{}); ok {
			if success, successOk := resMap["success"].(bool); successOk && success {
				toolResult["status"] = "read"
				toolResult["message_id"] = messageIdStr
			} else {
				toolResult["response"] = resMap
			}
		} else {
			toolResult["response"] = res
		}
	} else {
		toolResult["response"] = res
		toolResult["status_code"] = statusCode
	}

	return toolResult, map[string]interface{}{"body": payload}, false, nil
}

func (whatsAppTool *WhatsAppTool) SendTypingIndicator(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SendTypingIndicator Execute - Start")

	messageId, messageIdOk := params["message_id"]
	if !messageIdOk {
		err = logs.Err(ctx, fmt.Errorf("message_id parameter is required"), "message_id parameter is required")
		return nil, nil, false, err
	}

	messageIdStr, ok := messageId.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("message_id must be a string"), "message_id must be a string")
		return nil, nil, false, err
	}

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s/messages", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	typingType := "text"
	if indicatorType, typeOk := params["typing_type"]; typeOk {
		if typeStr, ok := indicatorType.(string); ok {
			typingType = typeStr
		}
	}

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageIdStr,
		"typing_indicator": map[string]interface{}{
			"type": typingType,
		},
	}

	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, payload)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to send typing indicator: %s", err.Error()), "failed to send typing indicator")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if statusCode == http.StatusOK {
		if resMap, ok := res.(map[string]interface{}); ok {
			if success, successOk := resMap["success"].(bool); successOk && success {
				toolResult["status"] = "typing_indicator_sent"
				toolResult["message_id"] = messageIdStr
			} else {
				toolResult["response"] = resMap
			}
		} else {
			toolResult["response"] = res
		}
	} else {
		toolResult["response"] = res
		toolResult["status_code"] = statusCode
	}

	return toolResult, map[string]interface{}{"body": payload}, false, nil
}

func (whatsAppTool *WhatsAppTool) GetThroughput(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetThroughput Execute - Start")

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s?fields=throughput", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	queryParams := map[string]string{}

	if startTime, startOk := params["start_time"]; startOk {
		queryParams["start_time"] = fmt.Sprintf("%v", startTime)
	}
	if endTime, endOk := params["end_time"]; endOk {
		queryParams["end_time"] = fmt.Sprintf("%v", endTime)
	}

	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to get throughput: %s", err.Error()), "failed to get throughput")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if statusCode == http.StatusOK {
		toolResult["throughput"] = res
	} else {
		toolResult["response"] = res
		toolResult["status_code"] = statusCode
	}

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (whatsAppTool *WhatsAppTool) CreateGroup(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("CreateGroup Execute - Start")

	subject, subjectOk := params["subject"]
	if !subjectOk {
		err = logs.Err(ctx, fmt.Errorf("subject parameter is required"), "subject parameter is required")
		return nil, nil, false, err
	}

	subjectStr, ok := subject.(string)
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("subject must be a string"), "subject must be a string")
		return nil, nil, false, err
	}

	if subjectStr == "" {
		err = logs.Err(ctx, fmt.Errorf("subject cannot be empty"), "subject cannot be empty")
		return nil, nil, false, err
	}

	description, descriptionOk := params["description"]
	descriptionStr := ""
	if descriptionOk {
		if descStr, ok := description.(string); ok {
			descriptionStr = descStr
		}
	}

	joinApprovalMode, joinApprovalModeOk := params["join_approval_mode"]
	joinApprovalModeStr := ""
	if joinApprovalModeOk {
		if modeStr, ok := joinApprovalMode.(string); ok {
			if modeStr == "auto_approve " || modeStr == "approval_required " {
				joinApprovalModeStr = modeStr
			} else {
				err = logs.Err(ctx, fmt.Errorf("join_approval_mode must be auto_approve or approval_required"), "join_approval_mode must be auto_approve or approval_required")
				return nil, nil, false, err
			}
		}
	}

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	url := fmt.Sprintf("%s/%s/%s/groups", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"subject":           subjectStr,
	}
	if joinApprovalModeStr != "" {
		payload["join_approval_mode"] = joinApprovalModeStr
	}

	if descriptionStr != "" {
		payload["description"] = descriptionStr
	}

	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, payload)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create group: %s", err.Error()), "failed to create group")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if statusCode == http.StatusOK || statusCode == http.StatusCreated {
		if resMap, ok := res.(map[string]interface{}); ok {
			toolResult["response"] = resMap
			if groupId, groupIdOk := resMap["id"]; groupIdOk {
				toolResult["group_id"] = groupId
			}
			toolResult["status"] = "created"
		} else {
			toolResult["response"] = res
			toolResult["status"] = "created"
		}
	} else {
		toolResult["response"] = res
		toolResult["status_code"] = statusCode
	}

	return toolResult, map[string]interface{}{"body": payload}, false, nil
}

func (whatsAppTool *WhatsAppTool) RegisterPublicKey(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("RegisterPublicKey Execute - Start")

	key, keyOk := params["business_public_key"]
	if !keyOk {
		err = logs.Err(ctx, errors.New("business_public_key parameter is required"), "business_public_key parameter is required")
		return nil, nil, false, err
	}
	keyStr, ok := key.(string)
	if !ok {
		err = logs.Err(ctx, errors.New("business_public_key must be a string"), "business_public_key must be a string")
		return nil, nil, false, err
	}
	keyText, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to decode business_public_key: %s", err.Error()), "failed to decode business_public_key")
		return nil, nil, false, err
	}

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	urlPath := fmt.Sprintf("%s/%s/%s/whatsapp_business_encryption", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)

	formData := map[string]string{
		"business_public_key": string(keyText),
	}

	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Accept", "application/json")

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, urlPath, headers, formData, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to register public key: %s", err.Error()), "failed to register public key")
		return nil, nil, false, err
	}

	toolResult = map[string]interface{}{
		"response": res,
	}

	return toolResult, map[string]interface{}{"body": formData}, false, nil
}

func (whatsAppTool *WhatsAppTool) FetchPublicKey(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("FetchPublicKey Execute - Start")

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	urlPath := fmt.Sprintf("%s/%s/%s/whatsapp_business_encryption", WHATSAPP_BASE_URL, apiVersion, whatsAppTool.WhatsAppAccount.PhoneNumberId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")

	res, _, _, statusCode, err := utils.CallHttp(ctx, http.MethodGet, urlPath, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to fetch public key: %s", err.Error()), "failed to fetch public key")
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if statusCode == http.StatusOK {
		toolResult["public_key_info"] = res
	} else {
		toolResult["response"] = res
		toolResult["status_code"] = statusCode
	}

	return toolResult, map[string]interface{}{"body": params}, false, nil
}

func (whatsAppTool *WhatsAppTool) FlowEndpoint(ctx context.Context, params map[string]interface{}, projectId string, tenantId string) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("FlowEndpoint Execute - Start")

	type flowRequestParams struct {
		Endpoint          string `json:"endpoint" eru:"required"`
		EncryptedFlowData string `json:"encrypted_flow_data" eru:"required"`
		EncryptedAESKey   string `json:"encrypted_aes_key" eru:"required"`
		InitialVector     string `json:"initial_vector" eru:"required"`
	}

	flowRequest := flowRequestParams{}
	flowRequestBytes, err := json.Marshal(params)
	if err != nil {
		return nil, nil, false, fmt.Errorf("error marshalling flow request: %w", err)
	}

	err = json.Unmarshal(flowRequestBytes, &flowRequest)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	err = utils.ValidateStruct(ctx, flowRequest, "")
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	privateKeyBytes, err := base64.StdEncoding.DecodeString(whatsAppTool.WhatsAppAccount.PrivateKey)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	privateKey := string(privateKeyBytes)

	// Decrypt the AES key
	encryptedAESKeyBytes, _ := base64.StdEncoding.DecodeString(flowRequest.EncryptedAESKey)
	decryptedKeyBytes, err := rsa.DecryptWithKey(ctx, encryptedAESKeyBytes, privateKey)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	initialVectorBytes, _ := base64.StdEncoding.DecodeString(flowRequest.InitialVector)
	flowDataBytes, _ := base64.StdEncoding.DecodeString(flowRequest.EncryptedFlowData)

	decryptedFlowDataBytes, err := aes.DecryptGCM(ctx, flowDataBytes, decryptedKeyBytes, initialVectorBytes)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	var decryptedBody map[string]interface{}
	if err := json.Unmarshal(decryptedFlowDataBytes, &decryptedBody); err != nil {
		err = logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	var insertQueries []*models.Queries
	insertQueryFuncAsync := models.Queries{}
	insertQueryFuncAsync.Query = whatsAppTool.ToolDb.GetDbQuery(ctx, INSERT_ENPOINT_REQUEST)
	insertQueryFuncAsync.Vals = append(insertQueryFuncAsync.Vals, projectId, tenantId, string(flowRequestBytes), string(decryptedFlowDataBytes))
	insertQueryFuncAsync.Rank = 1
	insertQueries = append(insertQueries, &insertQueryFuncAsync)

	_, insertOutputErr := utils.ExecuteDbSave(ctx, whatsAppTool.ToolDb.GetConn(), insertQueries)
	if insertOutputErr != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to insert query: %s", err.Error()), "failed to insert query")
		return
	}

	action, actionOk := decryptedBody["action"]
	if !actionOk {
		err = logs.Err(ctx, fmt.Errorf("action not found in decrypted body"), "action not found in decrypted body")
		return nil, nil, false, err
	}
	actionString, _ := action.(string)

	// Create a response object
	response := make(map[string]interface{})
	if actionString == "ping" {
		response = map[string]interface{}{
			"data": map[string]interface{}{"status": "active"},
		}
	} else {
		res, err := whatsAppTool.ExecuteFunction(ctx, projectId, tenantId, flowRequest.Endpoint, decryptedBody, nil)
		if err != nil {
			err = logs.Err(ctx, err, "")
			return nil, nil, false, err
		}
		responseOk := false
		response, responseOk = res.(map[string]interface{})
		if !responseOk {
			err = logs.Err(ctx, fmt.Errorf("response is not a map[string]interface{}"), "response is not a map[string]interface{}")
			return nil, nil, false, err
		}
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	encryptedResponseBytes, err := aes.EncryptGCM(ctx, responseBytes, decryptedKeyBytes, initialVectorBytes)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	encryptedResponse := base64.StdEncoding.EncodeToString(encryptedResponseBytes)
	toolResult = map[string]interface{}{
		"encrypted_response": encryptedResponse,
	}
	return toolResult, map[string]interface{}{"body": decryptedBody}, false, nil
}

func (whatsAppTool *WhatsAppTool) GetToolCallback() tools.ToolCallback {
	return tools.ToolCallback{
		ResponseContentType: "plain/text",
	}
}

func (whatsAppTool *WhatsAppTool) Callback(ctx context.Context, projectId string, tenantId string, actionName string, body map[string]interface{}, params map[string][]string) (callbackResult interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Callback Execute - Start")
	// This callback handles:
	// 1. Webhook verification challenges from WhatsApp
	// 2. Incoming message notifications
	// 3. Message status updates (sent, delivered, read, failed) for tracking delivery
	// All callbacks are stored in database and forwarded to eru-functions for processing

	hubMode := ""
	hubChallenge := ""
	hubVerifyToken := ""

	if mode, modeOk := params["hub.mode"]; modeOk && len(mode) > 0 {
		hubMode = mode[0]
	}
	if challenge, challengeOk := params["hub.challenge"]; challengeOk && len(challenge) > 0 {
		hubChallenge = challenge[0]
	}
	if verifyToken, verifyTokenOk := params["hub.verify_token"]; verifyTokenOk && len(verifyToken) > 0 {
		hubVerifyToken = verifyToken[0]
	}

	gm := server.GetGlobalGoroutineManager(ctx)
	gm.SafeGoWithRestartBehavior("whatsapp-webhook-callback", func(bgCtx context.Context) {
		efurl := ctx.Value(tools.EruFuncBaseUrlKey)
		if efurl == nil {
			err = errors.New("erufuncbaseurl not found in context")
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		efurlString, ok := efurl.(string)
		if !ok {
			err = errors.New("erufuncbaseurl is not a string")
			logs.WithContext(ctx).Error(err.Error())
			return
		} else {
			bgCtx = context.WithValue(bgCtx, tools.EruFuncBaseUrlKey, efurlString)
		}
		body["tenant_id"] = tenantId
		body["project_id"] = projectId

		bodyBytes, err := json.Marshal(body)
		if err != nil {
			err = logs.Err(bgCtx, fmt.Errorf("failed to marshal body: %s", err.Error()), "failed to marshal body")
			return
		}

		paramBytes, err := json.Marshal(params)
		if err != nil {
			err = logs.Err(bgCtx, fmt.Errorf("failed to marshal params: %s", err.Error()), "failed to marshal params")
			return
		}

		var webhookPayload WhatsAppWebhookPayload
		err = json.Unmarshal(bodyBytes, &webhookPayload)
		if err != nil {
			err = logs.Err(bgCtx, fmt.Errorf("failed to unmarshal webhook payload: %s", err.Error()), "failed to unmarshal webhook payload")
			return
		}

		wabaId := ""
		msgFrom := ""

		if len(webhookPayload.Entry) > 0 {
			wabaId = webhookPayload.Entry[0].Id
			if len(webhookPayload.Entry[0].Changes) > 0 {
				c := webhookPayload.Entry[0].Changes[0]
				if cValue, cValueOk := c["value"]; cValueOk {
					if cValueMap, cValueMapOk := cValue.(map[string]interface{}); cValueMapOk {
						if cValueMetaData, cValueMetaDataOk := cValueMap["metadata"]; cValueMetaDataOk {
							if cValueMetaDataMap, cValueMetaDataMapOk := cValueMetaData.(map[string]interface{}); cValueMetaDataMapOk {
								if cValueMetaDataMap["display_phone_number"] != nil {
									msgFrom = cValueMetaDataMap["display_phone_number"].(string)
								}
							}
						}
					}
				}
			}
		}

		var insertQueries []*models.Queries
		insertQueryCallbackRequest := models.Queries{}
		insertQueryCallbackRequest.Query = whatsAppTool.ToolDb.GetDbQuery(bgCtx, INSERT_CALLBACK_REQUEST)
		insertQueryCallbackRequest.Vals = append(insertQueryCallbackRequest.Vals, projectId, tenantId, wabaId, string(bodyBytes), string(paramBytes), msgFrom)
		insertQueryCallbackRequest.Rank = 1
		insertQueries = append(insertQueries, &insertQueryCallbackRequest)

		_, insertOutputErr := utils.ExecuteDbSave(bgCtx, whatsAppTool.ToolDb.GetConn(), insertQueries)
		if insertOutputErr != nil {
			err = logs.Err(bgCtx, fmt.Errorf("failed to insert query: %s", err.Error()), "failed to insert query")
			return
		}

		hookResult, err := whatsAppTool.ExecuteHook(bgCtx, "clbk", "", projectId, tenantId, body, params)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))

	}, server.ContinueOnMaxRetries)

	if hubMode == "subscribe" {
		if hubVerifyToken == whatsAppTool.WhatsAppAccount.WebhookSubscriptionToken {
			logs.WithContext(ctx).Info("Webhook verification successful")
			return hubChallenge, false, nil
		} else {
			logs.WithContext(ctx).Info("Webhook verification failed")
			return "", false, errors.New("webhook verification failed")
		}
	}
	return "OK", false, nil
}

func (whatsAppTool *WhatsAppTool) CreateMessageTemplate(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("CreateMessageTemplate Execute - Start")

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	businessAccountId := whatsAppTool.WhatsAppAccount.BusinessAccountId
	if businessAccountId == "" {
		err = logs.Err(ctx, fmt.Errorf("business_account_id is required to create message templates"), "business_account_id is required to create message templates")
		return nil, nil, false, err
	}

	url := fmt.Sprintf("%s/%s/%s/message_templates", WHATSAPP_BASE_URL, apiVersion, businessAccountId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")

	// Map params to WhatsAppCreateTemplateRequest
	var req WhatsAppCreateTemplateRequest
	reqBytes, err := json.Marshal(params)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to marshal template creation request")
	}
	err = json.Unmarshal(reqBytes, &req)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to unmarshal template creation request into struct")
	}

	// Double check required fields (though they might be validated by graph API)
	if req.Name == "" || req.Category == "" || req.Language == "" || len(req.Components) == 0 {
		return nil, nil, false, logs.Err(ctx, fmt.Errorf("name, category, language and components are required"), "name, category, language and components are required")
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, nil, req)
	if err != nil {
		if errResult := whatsAppTemplateErrorFromCallErr(err); errResult != nil {
			_ = logs.Err(ctx, err, "WhatsApp API returned an error")
			return errResult, map[string]interface{}{"body": req}, false, nil
		}
		return nil, nil, false, logs.Err(ctx, err, "failed to call WhatsApp API for template creation")
	}

	resMap, ok := res.(map[string]interface{})
	if !ok {
		return nil, nil, false, logs.Err(ctx, fmt.Errorf("unexpected response format from WhatsApp API"), "unexpected response format")
	}

	// Check for error in response; log but return success status 200 to our caller
	if errorVal, exists := resMap["error"]; exists {
		_ = logs.Err(ctx, whatsAppApiError(errorVal), "WhatsApp API returned an error")
		return whatsAppTemplateErrorResult(errorVal), map[string]interface{}{"body": req}, false, nil
	}

	toolResult = whatsAppNormalizeResult(resMap)
	return toolResult, map[string]interface{}{"body": req}, false, nil
}

func whatsAppApiError(errorVal interface{}) error {
	if errMap, ok := errorVal.(map[string]interface{}); ok {
		title, _ := errMap["error_user_title"].(string)
		msg, _ := errMap["error_user_msg"].(string)
		if title != "" || msg != "" {
			if title != "" && msg != "" {
				return fmt.Errorf("%s: %s", title, msg)
			}
			return fmt.Errorf("%s%s", title, msg)
		}
	}
	return fmt.Errorf("WhatsApp API error: %v", errorVal)
}

func whatsAppNormalizeResult(resMap map[string]interface{}) map[string]interface{} {
	if body, ok := resMap["body"].(string); ok {
		var parsed map[string]interface{}
		if json.Unmarshal([]byte(body), &parsed) == nil {
			return parsed
		}
	}
	return resMap
}

func whatsAppTemplateErrorFromCallErr(callErr error) map[string]interface{} {
	var parsed map[string]interface{}
	if json.Unmarshal([]byte(callErr.Error()), &parsed) == nil {
		if errorVal, exists := parsed["error"]; exists {
			return whatsAppTemplateErrorResult(errorVal)
		}
	}
	return nil
}

func whatsAppTemplateErrorResult(errorVal interface{}) map[string]interface{} {
	result := map[string]interface{}{"status": "error"}
	if errMap, ok := errorVal.(map[string]interface{}); ok {
		if title, ok := errMap["error_user_title"].(string); ok {
			result["error_user_title"] = title
		}
		if msg, ok := errMap["error_user_msg"].(string); ok {
			result["error_user_msg"] = msg
		}
	}
	return result
}

func (whatsAppTool *WhatsAppTool) SaveMessageTemplate(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SaveMessageTemplate Execute - Start")

	if templateId, ok := params["id"].(string); ok && templateId != "" {
		return whatsAppTool.EditMessageTemplate(ctx, params)
	}
	return whatsAppTool.CreateMessageTemplate(ctx, params)
}

func (whatsAppTool *WhatsAppTool) EditMessageTemplate(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("EditMessageTemplate Execute - Start")

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	templateId, ok := params["id"].(string)
	if !ok || templateId == "" {
		return nil, nil, false, logs.Err(ctx, errors.New("id is required to edit a message template"), "id is required")
	}

	url := fmt.Sprintf("%s/%s/%s", WHATSAPP_BASE_URL, apiVersion, templateId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")

	// name, category and language cannot be edited on WhatsApp; only components are allowed
	var req WhatsAppEditTemplateRequest
	reqBytes, err := json.Marshal(params)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to marshal template edit request")
	}
	err = json.Unmarshal(reqBytes, &req)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to unmarshal template edit request into struct")
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, nil, req)
	if err != nil {
		if errResult := whatsAppTemplateErrorFromCallErr(err); errResult != nil {
			_ = logs.Err(ctx, err, "WhatsApp API returned an error")
			return errResult, map[string]interface{}{"body": req}, false, nil
		}
		return nil, nil, false, logs.Err(ctx, err, "failed to call WhatsApp API for template edit")
	}

	resMap, ok := res.(map[string]interface{})
	if !ok {
		return nil, nil, false, logs.Err(ctx, fmt.Errorf("unexpected response format from WhatsApp API"), "unexpected response format")
	}

	if errorVal, exists := resMap["error"]; exists {
		_ = logs.Err(ctx, whatsAppApiError(errorVal), "WhatsApp API returned an error")
		return whatsAppTemplateErrorResult(errorVal), map[string]interface{}{"body": req}, false, nil
	}

	toolResult = whatsAppNormalizeResult(resMap)
	toolResult["status"] = "PENDING"
	return toolResult, map[string]interface{}{"body": req}, false, nil
}

func (whatsAppTool *WhatsAppTool) DeleteMessageTemplate(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("DeleteMessageTemplate Execute - Start")

	apiVersion := whatsAppTool.WhatsAppAccount.ApiVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}

	businessAccountId := whatsAppTool.WhatsAppAccount.BusinessAccountId
	if businessAccountId == "" {
		return nil, nil, false, logs.Err(ctx, errors.New("business_account_id is required"), "business_account_id is required")
	}

	templateName, ok := params["name"].(string)
	if !ok || templateName == "" {
		return nil, nil, false, logs.Err(ctx, errors.New("template name is required"), "name is required")
	}

	url := fmt.Sprintf("%s/%s/%s/message_templates", WHATSAPP_BASE_URL, apiVersion, businessAccountId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", whatsAppTool.WhatsAppAccount.ApiKey))

	queryParams := map[string]string{}
	queryParams["name"] = templateName

	if templateId, ok := params["id"].(string); ok && templateId != "" {
		queryParams["hsm_id"] = templateId
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodDelete, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to delete message template")
	}

	resMap, ok := res.(map[string]interface{})
	if !ok {
		return nil, nil, false, logs.Err(ctx, fmt.Errorf("unexpected response format from WhatsApp API"), "unexpected response format")
	}

	if errorVal, exists := resMap["error"]; exists {
		return nil, nil, false, logs.Err(ctx, whatsAppApiError(errorVal), "WhatsApp API returned an error")
	}

	toolResult = resMap
	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (whatsAppTool *WhatsAppTool) GetToolCbUrl(projectId string, tenantId string) string {
	return fmt.Sprint(whatsAppTool.CallbackBaseUrl, "/", projectId, "/", tenantId, "/callback/tool/", whatsAppTool.ToolName)
}

func (whatsAppTool *WhatsAppTool) GetBytes(ctx context.Context) ([]byte, error) {
	toolJson, err := json.Marshal(whatsAppTool)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to marshal tool: %s", err.Error()), "failed to marshal tool")
		return nil, err
	}
	return toolJson, nil
}

func (whatsAppTool *WhatsAppTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	newTool := &WhatsAppTool{}
	err := json.Unmarshal(toolObjJson, newTool)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to unmarshal tool: %s", err.Error()), "failed to unmarshal tool")
		return nil, err
	}
	return newTool, nil
}

func (whatsAppTool *WhatsAppTool) DownloadFlowDocument(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("DownloadFlowDocument Execute - Start")

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to marshal params")
	}

	var request WhatsAppDownloadFlowDocumentRequest
	err = json.Unmarshal(paramsBytes, &request)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to unmarshal download flow document request")
	}

	if err := utils.ValidateStruct(ctx, request, ""); err != nil {
		return nil, nil, false, logs.Err(ctx, err, "invalid request parameters")
	}

	// 1. Download cdn_file file from cdn_url
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, request.CdnUrl, nil)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to create download request")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to download media")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, false, logs.Err(ctx, fmt.Errorf("media download failed with status %d", resp.StatusCode), "media download failed")
	}

	cdnFile, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to read downloaded file")
	}

	// 2. Make sure SHA256(cdn_file) == enc_hash
	encHashBytes, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(request.EncryptionMetadata.EncryptedHash)
	if err != nil {
		encHashBytes, err = base64.StdEncoding.DecodeString(request.EncryptionMetadata.EncryptedHash)
		if err != nil {
			return nil, nil, false, logs.Err(ctx, err, "failed to decode encrypted_hash")
		}
	}
	calculatedEncHash := erusha.NewSHA256(cdnFile)
	if !bytes.Equal(calculatedEncHash, encHashBytes) {
		return nil, nil, false, logs.Err(ctx, errors.New("encrypted hash mismatch"), "encrypted hash mismatch")
	}

	// 3. Validate HMAC-SHA256
	// For reference, cdn_file = ciphertext & hmac10
	if len(cdnFile) < 10 {
		return nil, nil, false, logs.Err(ctx, errors.New("downloaded file too small"), "downloaded file too small")
	}
	ciphertext := cdnFile[:len(cdnFile)-10]
	hmac10 := cdnFile[len(cdnFile)-10:]

	ivBytes, err := base64.StdEncoding.DecodeString(request.EncryptionMetadata.Iv)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to decode iv")
	}
	hmacKeyBytes, err := base64.StdEncoding.DecodeString(request.EncryptionMetadata.HmacKey)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to decode hmac_key")
	}

	// Calculate HMAC with hmac_key, initialization vector (encryption_metadata.iv) and ciphertext
	h := hmac.New(sha256.New, hmacKeyBytes)
	h.Write(ivBytes)
	h.Write(ciphertext)
	calculatedHmac := h.Sum(nil)

	if !bytes.Equal(calculatedHmac[:10], hmac10) {
		return nil, nil, false, logs.Err(ctx, errors.New("HMAC validation failed"), "HMAC validation failed")
	}

	// 4. Decrypt media content
	encryptionKeyBytes, err := base64.StdEncoding.DecodeString(request.EncryptionMetadata.EncryptionKey)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to decode encryption_key")
	}

	decryptedMediaWithPadding, err := aes.DecryptCBC(ctx, ciphertext, encryptionKeyBytes, ivBytes)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to decrypt media")
	}

	// Remove padding (AES256 uses blocks of 16 bytes, padding algorithm is pkcs7)
	decryptedMedia, err := aes.Unpad(decryptedMediaWithPadding)
	if err != nil {
		return nil, nil, false, logs.Err(ctx, err, "failed to unpad decrypted media")
	}

	// 5. Validate the decrypted media
	// Make sure SHA256(decrypted_media) = plaintext_hash
	plaintextHashBytes, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(request.EncryptionMetadata.PlaintextHash)
	if err != nil {
		plaintextHashBytes, err = base64.StdEncoding.DecodeString(request.EncryptionMetadata.PlaintextHash)
		if err != nil {
			return nil, nil, false, logs.Err(ctx, err, "failed to decode plaintext_hash")
		}
	}
	calculatedPlaintextHash := erusha.NewSHA256(decryptedMedia)
	if !bytes.Equal(calculatedPlaintextHash, plaintextHashBytes) {
		return nil, nil, false, logs.Err(ctx, errors.New("plaintext hash mismatch"), "plaintext hash mismatch")
	}

	fileBase64 := base64.StdEncoding.EncodeToString(decryptedMedia)

	toolResult = make(map[string]interface{})
	toolResult["file_content"] = fileBase64
	toolResult["file_name"] = request.FileName
	toolResult["media_id"] = request.MediaId

	return toolResult, map[string]interface{}{"body": request}, false, nil
}

func init() {
	tools.RegisterTool("WHATSAPP", func() tools.Tooling { return new(WhatsAppTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       true,
		ToolType:     "WHATSAPP",
		Category:     "Communication",
		Description:  "WhatsApp Business API for messaging, media, templates, and webhooks",
		Actions:      []tools.ActionInfo{{Name: SendMessage}, {Name: SubscribeWebhooks}, {Name: GetMessageStatus}, {Name: UploadMedia}, {Name: RetrieveMedia}, {Name: DeleteMedia}, {Name: GetMediaUrl}, {Name: GetBusinessProfile}, {Name: GetMessageTemplates}, {Name: MarkMessageAsRead}, {Name: SendTypingIndicator}, {Name: GetThroughput}, {Name: CreateGroup}, {Name: RegisterPublicKey}, {Name: FetchPublicKey}, {Name: FlowEndpoint}, {Name: SaveMessageTemplate}, {Name: DownloadFlowDocument}, {Name: FetchTemplates}, {Name: DeleteMessageTemplate}, {Name: Callback}},
		OAuthEnabled: false,
		Icon:         "PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxZW0iIGhlaWdodD0iMWVtIiB2aWV3Qm94PSIwIDAgMjU2IDI1OCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJTVkdLM0tacTQ5VSIgeDE9IjUwJSIgeDI9IjUwJSIgeTE9IjEwMCUiIHkyPSIwJSI+PHN0b3Agb2Zmc2V0PSIwJSIgc3RvcC1jb2xvcj0iIzFmYWYzOCIvPjxzdG9wIG9mZnNldD0iMTAwJSIgc3RvcC1jb2xvcj0iIzYwZDY2OSIvPjwvbGluZWFyR3JhZGllbnQ+PGxpbmVhckdyYWRpZW50IGlkPSJTVkdlZk1rb0VPZCIgeDE9IjUwJSIgeDI9IjUwJSIgeTE9IjEwMCUiIHkyPSIwJSI+PHN0b3Agb2Zmc2V0PSIwJSIgc3RvcC1jb2xvcj0iI2Y5ZjlmOSIvPjxzdG9wIG9mZnNldD0iMTAwJSIgc3RvcC1jb2xvcj0iI2ZmZiIvPjwvbGluZWFyR3JhZGllbnQ+PC9kZWZzPjxwYXRoIGZpbGw9InVybCgjU1ZHSzNLWnE0OVUpIiBkPSJNNS40NjMgMTI3LjQ1NmMtLjAwNiAyMS42NzcgNS42NTggNDIuODQzIDE2LjQyOCA2MS40OTlMNC40MzMgMjUyLjY5N2w2NS4yMzItMTcuMTA0YTEyMyAxMjMgMCAwIDAgNTguOCAxNC45N2guMDU0YzY3LjgxNSAwIDEyMy4wMTgtNTUuMTgzIDEyMy4wNDctMTIzLjAxYy4wMTMtMzIuODY3LTEyLjc3NS02My43NzMtMzYuMDA5LTg3LjAyNWMtMjMuMjMtMjMuMjUtNTQuMTI1LTM2LjA2MS04Ny4wNDMtMzYuMDc2Yy02Ny44MjMgMC0xMjMuMDIyIDU1LjE4LTEyMy4wNSAxMjMuMDA0Ii8+PHBhdGggZmlsbD0idXJsKCNTVkdlZk1rb0VPZCkiIGQ9Ik0xLjA3IDEyNy40MTZjLS4wMDcgMjIuNDU3IDUuODYgNDQuMzggMTcuMDE0IDYzLjcwNEwwIDI1Ny4xNDdsNjcuNTcxLTE3LjcxN2MxOC42MTggMTAuMTUxIDM5LjU4IDE1LjUwMyA2MC45MSAxNS41MTFoLjA1NWM3MC4yNDggMCAxMjcuNDM0LTU3LjE2OCAxMjcuNDY0LTEyNy40MjNjLjAxMi0zNC4wNDgtMTMuMjM2LTY2LjA2NS0zNy4zLTkwLjE1QzE5NC42MzMgMTMuMjg2IDE2Mi42MzMuMDE0IDEyOC41MzYgMEM1OC4yNzYgMCAxLjA5OSA1Ny4xNiAxLjA3MSAxMjcuNDE2bTQwLjI0IDYwLjM3NmwtMi41MjMtNC4wMDVjLTEwLjYwNi0xNi44NjQtMTYuMjA0LTM2LjM1Mi0xNi4xOTYtNTYuMzYzQzIyLjYxNCA2OS4wMjkgNzAuMTM4IDIxLjUyIDEyOC41NzYgMjEuNTJjMjguMy4wMTIgNTQuODk2IDExLjA0NCA3NC45IDMxLjA2YzIwLjAwMyAyMC4wMTggMzEuMDEgNDYuNjI4IDMxLjAwMyA3NC45M2MtLjAyNiA1OC4zOTUtNDcuNTUxIDEwNS45MS0xMDUuOTQzIDEwNS45MWgtLjA0MmMtMTkuMDEzLS4wMS0zNy42Ni01LjExNi01My45MjItMTQuNzY1bC0zLjg3LTIuMjk1bC00MC4wOTggMTAuNTEzeiIvPjxwYXRoIGZpbGw9IiNmZmYiIGQ9Ik05Ni42NzggNzQuMTQ4Yy0yLjM4Ni01LjMwMy00Ljg5Ny01LjQxLTcuMTY2LTUuNTAzYy0xLjg1OC0uMDgtMy45ODItLjA3NC02LjEwNC0uMDc0Yy0yLjEyNCAwLTUuNTc1Ljc5OS04LjQ5MiAzLjk4NGMtMi45MiAzLjE4OC0xMS4xNDggMTAuODkyLTExLjE0OCAyNi41NjFzMTEuNDEzIDMwLjgxMyAxMy4wMDQgMzIuOTRjMS41OTMgMi4xMjMgMjIuMDMzIDM1LjMwNyA1NC40MDUgNDguMDczYzI2LjkwNCAxMC42MDkgMzIuMzc5IDguNDk5IDM4LjIxOCA3Ljk2N2M1Ljg0LS41MyAxOC44NDQtNy43MDIgMjEuNDk3LTE1LjEzOWMyLjY1NS03LjQzNiAyLjY1NS0xMy44MSAxLjg1OS0xNS4xNDJjLS43OTYtMS4zMjctMi45Mi0yLjEyNC02LjEwNS0zLjcxNnMtMTguODQ0LTkuMjk4LTIxLjc2My0xMC4zNjFjLTIuOTItMS4wNjItNS4wNDMtMS41OTItNy4xNjcgMS41OTdjLTIuMTI0IDMuMTg0LTguMjIzIDEwLjM1Ni0xMC4wODIgMTIuNDhjLTEuODU3IDIuMTI5LTMuNzE2IDIuMzk0LTYuOS44MDFjLTMuMTg3LTEuNTk4LTEzLjQ0NC00Ljk1Ny0yNS42MTMtMTUuODA2Yy05LjQ2OC04LjQ0Mi0xNS44Ni0xOC44NjctMTcuNzE4LTIyLjA1NmMtMS44NTgtMy4xODQtLjE5OS00LjkxIDEuMzk4LTYuNDk3YzEuNDMxLTEuNDI3IDMuMTg2LTMuNzE5IDQuNzgtNS41NzhjMS41ODgtMS44NiAyLjExOC0zLjE4NyAzLjE4LTUuMzExYzEuMDYzLTIuMTI2LjUzMS0zLjk4Ni0uMjY0LTUuNTc5Yy0uNzk4LTEuNTkzLTYuOTg3LTE3LjM0My05LjgxOS0yMy42NCIvPjwvc3ZnPg==",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(WhatsAppTool{}), []string{}),
	})
}
