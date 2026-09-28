package emails

type EmailAccount struct {
	DisplayName string `json:"display_name"`
	//SecretName  string `json:"secret_name"`
	AccessToken                    string `json:"-"`
	RefreshToken                   string `json:"-"`
	SubscriptionId                 string `json:"subscription_id"`
	SubscriptionExpirationDateTime string `json:"subscription_expiration_date_time"`
	TokenExpirationDateTime        string `json:"token_expiration_date_time"`
	HistoryId                      string `json:"history_id,omitempty"`
}

type emailAccountWithToken struct {
	DisplayName                    string `json:"display_name"`
	AccessToken                    string `json:"access_token"`
	RefreshToken                   string `json:"refresh_token"`
	SubscriptionId                 string `json:"subscription_id"`
	SubscriptionExpirationDateTime string `json:"subscription_expiration_date_time"`
	TokenExpirationDateTime        string `json:"token_expiration_date_time"`
	HistoryId                      string `json:"history_id,omitempty"`
}

const (
	SendEmail         = "send_email"
	ReadEmail         = "read_email"
	ReadConversation  = "read_conversation"
	SubscribeEmail    = "subscribe_email"
	ReadMessage       = "read_message"
	Callback          = "callback"
	Login             = "login"
	RenewToken        = "renew_token"
	GetSsoUrl         = "get_sso_url"
	RenewSubscription = "renew_subscription"
	StopAutoRenew     = "stop_auto_renew"
	StopSubscription  = "stop_subscription"
	ReadHistoryRange  = "read_history_range"
)
