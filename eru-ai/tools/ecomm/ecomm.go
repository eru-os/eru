package ecomm

type AmazonAccount struct {
	UserAgent               string `json:"user_agent"`
	AccessToken             string `json:"-"`
	RefreshToken            string `json:"-"`
	TokenExpirationDateTime string `json:"token_expiration_date_time"`
}

type amazonAccountWithToken struct {
	UserAgent               string `json:"user_agent"`
	AccessToken             string `json:"access_token"`
	RefreshToken            string `json:"refresh_token"`
	TokenExpirationDateTime string `json:"token_expiration_date_time"`
}

const (
	GetOrders               = "get_orders"
	GetOrderItems           = "get_order_items"
	GetFinancialEvents      = "get_financial_events"
	GetFinancialEventGroups = "get_financial_event_groups"
	Login                   = "login"
	RenewToken              = "renew_token"
	GetSsoUrl               = "get_sso_url"
	StopAutoRenew           = "stop_auto_renew"
)
