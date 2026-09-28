package messengers

const (
	SendMessage         = "send_message"
	SubscribeWebhooks   = "subscribe_webhooks"
	GetMessageStatus    = "get_message_status"
	UploadMedia         = "upload_media"
	ResumableUpload     = "resumable_upload"
	RetrieveMedia       = "retrieve_media"
	DeleteMedia         = "delete_media"
	GetMediaUrl         = "get_media_url"
	GetBusinessProfile  = "get_business_profile"
	GetMessageTemplates = "get_message_templates"
	MarkMessageAsRead   = "mark_message_as_read"
	SendTypingIndicator = "send_typing_indicator"
	GetThroughput       = "get_throughput"
	CreateGroup         = "create_group"
	Callback            = "callback"
	ReadMessages        = "read_messages"
	// Slack-specific actions
	ListChannels    = "list_channels"
	ListUsers       = "list_users"
	CreateChannel   = "create_channel"
	InviteToChannel = "invite_to_channel"
	DownloadMedia   = "download_media"

	Login     = "login"
	GetSsoUrl = "get_sso_url"

	RegisterPublicKey     = "register_public_key"
	FetchPublicKey        = "fetch_public_key"
	FlowEndpoint          = "flow_endpoint"
	SaveMessageTemplate   = "save_message_template"
	DownloadFlowDocument  = "download_flow_document"
	FetchTemplates        = "fetch_templates"
	DeleteMessageTemplate = "delete_message_template"
)

type MessengerAccount struct {
	PhoneNumberId      string `json:"phone_number_id"`
	BusinessAccountId  string `json:"business_account_id"`
	AccessToken        string `json:"-"`
	WebhookVerifyToken string `json:"-"`
	WebhookUrl         string `json:"webhook_url"`
	ApiVersion         string `json:"api_version"`
}

type messengerAccountWithToken struct {
	PhoneNumberId      string `json:"phone_number_id"`
	BusinessAccountId  string `json:"business_account_id"`
	AccessToken        string `json:"access_token"`
	WebhookVerifyToken string `json:"webhook_verify_token"`
	WebhookUrl         string `json:"webhook_url"`
	ApiVersion         string `json:"api_version"`
}
