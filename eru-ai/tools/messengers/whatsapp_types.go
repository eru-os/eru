package messengers

type WhatsAppMessagePayload struct {
	MessagingProduct string                             `json:"messaging_product"`
	RecipientType    string                             `json:"recipient_type"`
	To               string                             `json:"to"`
	Type             string                             `json:"type"`
	Context          *WhatsAppContext                   `json:"context,omitempty"`
	Template         *WhatsAppTemplateMessagePayload    `json:"template,omitempty"`
	Text             *WhatsAppTextMessagePayload        `json:"text,omitempty"`
	Reaction         *WhatsAppReactionMessagePayload    `json:"reaction,omitempty"`
	Image            *WhatsAppMediaMessagePayload       `json:"image,omitempty"`
	Video            *WhatsAppMediaMessagePayload       `json:"video,omitempty"`
	Audio            *WhatsAppMediaMessagePayload       `json:"audio,omitempty"`
	Document         *WhatsAppMediaMessagePayload       `json:"document,omitempty"`
	Sticker          *WhatsAppMediaMessagePayload       `json:"sticker,omitempty"`
	Contacts         []WhatsAppContactMessagePayload    `json:"contacts,omitempty"`
	Location         *WhatsAppLocationMessagePayload    `json:"location,omitempty"`
	Interactive      *WhatsAppInteractiveMessagePayload `json:"interactive,omitempty"`
}

type WhatsAppTemplateMessagePayload struct {
	Components []WhatsAppMessageComponent `json:"components"`
	Language   WhatsAppLanguage           `json:"language" eru:"required"`
	Name       string                     `json:"name,omitempty"`
}
type WhatsAppTextMessagePayload struct {
	Body       string `json:"body" eru:"required"`
	PreviewUrl bool   `json:"preview_url,omitempty"`
}
type WhatsAppReactionMessagePayload struct {
	MessageId string `json:"message_id" eru:"required"`
	Emoji     string `json:"emoji" eru:"required"`
}
type WhatsAppMediaMessagePayload struct {
	Link     string `json:"link,omitempty"`
	Id       string `json:"id,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
	Voice    bool   `json:"voice,omitempty"`
}
type WhatsAppLanguage struct {
	Code string `json:"code" eru:"required"`
}
type WhatsAppContext struct {
	MessageId string `json:"message_id" eru:"required"`
}
type WhatsAppMessageComponent struct {
	Index      *int                       `json:"index,omitempty"`
	Type       string                     `json:"type" eru:"required"`
	SubType    string                     `json:"sub_type,omitempty"`
	Parameters []WhatsAppMessageParameter `json:"parameters,omitempty"`
}
type WhatsAppLocationMessagePayload struct {
	Latitude  string `json:"latitude" eru:"required"`
	Longitude string `json:"longitude" eru:"required"`
	Name      string `json:"name,omitempty"`
	Address   string `json:"address,omitempty"`
}
type WhatsAppInteractiveHeader struct {
	Type     string `json:"type" eru:"required"`
	Document *struct {
		Link string `json:"link,omitempty"`
	} `json:"document,omitempty"`
	Image *struct {
		Id   string `json:"id,omitempty"`
		Link string `json:"link,omitempty"`
	} `json:"image,omitempty"`
	Video *struct {
		Link string `json:"link,omitempty"`
	} `json:"video,omitempty"`
	Text string `json:"text,omitempty"`
}
type WhatsAppInteractiveBody struct {
	Text string `json:"text,omitempty"`
}
type WhatsAppInteractiveAction struct {
	Name       string `json:"name,omitempty"`
	Parameters *struct {
		Country     string `json:"country,omitempty"`
		Url         string `json:"url,omitempty"`
		DisplayText string `json:"display_text,omitempty"`
	} `json:"parameters,omitempty"`
	Button  string `json:"button,omitempty"`
	Buttons []struct {
		Type  string `json:"type"`
		Reply *struct {
			Id    string `json:"id"`
			Title string `json:"title"`
		} `json:"reply,omitempty"`
	} `json:"buttons,omitempty"`
	Sections []struct {
		Title string `json:"title"`
		Rows  []struct {
			Id          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"rows"`
	} `json:"sections,omitempty"`
	Cards []struct {
		CardIndex int                        `json:"card_index,omitempty"`
		Type      string                     `json:"type"`
		Header    *WhatsAppInteractiveHeader `json:"header,omitempty"`
		Body      *WhatsAppInteractiveBody   `json:"body,omitempty"`
		Footer    *WhatsAppInteractiveFooter `json:"footer,omitempty"`
		Action    *struct {
			ProductRetailerId string `json:"product_retailer_id,omitempty"`
			CatalogId         string `json:"catalog_id,omitempty"`
			Name              string `json:"name,omitempty"`
			Parameters        *struct {
				Url         string `json:"url"`
				DisplayText string `json:"display_text"`
			} `json:"parameters,omitempty"`
		} `json:"action,omitempty"`
	} `json:"cards,omitempty"`
}

type WhatsAppInteractiveFooter struct {
	Text string `json:"text,omitempty"`
}
type WhatsAppInteractiveMessagePayload struct {
	Type   string                     `json:"type" eru:"required"`
	Header *WhatsAppInteractiveHeader `json:"header,omitempty"`
	Body   *WhatsAppInteractiveBody   `json:"body" eru:"required"`
	Action *WhatsAppInteractiveAction `json:"action" eru:"required"`
	Footer *WhatsAppInteractiveFooter `json:"footer,omitempty"`
}
type WhatsAppContactMessagePayload struct {
	Addresses []struct {
		Street      string `json:"street"`
		City        string `json:"city"`
		State       string `json:"state"`
		Zip         string `json:"zip"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Type        string `json:"type"`
	} `json:"addresses,omitempty"`
	Birthday string `json:"birthday,omitempty"`
	Emails   []struct {
		Email string `json:"email"`
		Type  string `json:"type"`
	} `json:"emails,omitempty"`
	Name struct {
		FormattedName string `json:"formatted_name" eru:"required"`
		FirstName     string `json:"first_name"`
		LastName      string `json:"last_name"`
		MiddleName    string `json:"middle_name"`
		Suffix        string `json:"suffix"`
		Prefix        string `json:"prefix"`
	} `json:"name" eru:"required"`
	Org struct {
		Company    string `json:"company"`
		Department string `json:"department"`
		Title      string `json:"title"`
	} `json:"org"`
	Phones []struct {
		Phone string `json:"phone"`
		WaId  string `json:"wa_id,omitempty"`
		Type  string `json:"type"`
	} `json:"phones,omitempty"`
	Urls []struct {
		Url  string `json:"url"`
		Type string `json:"type"`
	} `json:"urls,omitempty"`
}

type WhatsAppMessageParameter struct {
	Index *int   `json:"index,omitempty"`
	Type  string `json:"type" eru:"required"`
	Text  string `json:"text,omitempty"`
	Image *struct {
		Link string `json:"link,omitempty"`
		Id   string `json:"id,omitempty"`
	} `json:"image,omitempty"`
	Video *struct {
		Link string `json:"link,omitempty"`
		Id   string `json:"id,omitempty"`
	} `json:"video,omitempty"`
	Document *struct {
		Link string `json:"link,omitempty"`
		Id   string `json:"id,omitempty"`
	} `json:"document,omitempty"`
	Location *struct {
		Latitude  string `json:"latitude,omitempty"`
		Longitude string `json:"longitude,omitempty"`
		Name      string `json:"name,omitempty"`
		Address   string `json:"address,omitempty"`
	} `json:"location,omitempty"`
	Action struct {
		FlowActionData map[string]interface{} `json:"flow_action_data,omitempty"`
		FlowToken      string                 `json:"flow_token,omitempty"`
	} `json:"action,omitempty"`
}

type WhatsAppWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Id            string                   `json:"id,omitempty"`
		Changes       []map[string]interface{} `json:"changes,omitempty"`
		ChangedFields []string                 `json:"changed_fields,omitempty"`
		Time          int64                    `json:"time,omitempty"`
	} `json:"entry,omitempty"`
}

/* type WhatsAppWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Id      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberId      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaId string `json:"wa_id"`
				} `json:"contacts,omitempty"`
				Messages []struct {
					From      string `json:"from"`
					Id        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text,omitempty"`
					Image struct {
						Caption  string `json:"caption,omitempty"`
						MimeType string `json:"mime_type"`
						Sha256   string `json:"sha256"`
						Id       string `json:"id"`
					} `json:"image,omitempty"`
					Audio struct {
						Id       string `json:"id"`
						MimeType string `json:"mime_type"`
					} `json:"audio,omitempty"`
					Video struct {
						Caption  string `json:"caption,omitempty"`
						Filename string `json:"filename,omitempty"`
						Id       string `json:"id"`
						MimeType string `json:"mime_type"`
					} `json:"video,omitempty"`
					Document struct {
						Caption  string `json:"caption,omitempty"`
						Filename string `json:"filename"`
						Id       string `json:"id"`
						MimeType string `json:"mime_type"`
					} `json:"document,omitempty"`
					Location struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
						Name      string  `json:"name,omitempty"`
						Address   string  `json:"address,omitempty"`
					} `json:"location,omitempty"`
					Contacts []struct {
						Name struct {
							FormattedName string `json:"formatted_name"`
							FirstName     string `json:"first_name,omitempty"`
							LastName      string `json:"last_name,omitempty"`
						} `json:"name"`
						Phones []struct {
							Phone string `json:"phone"`
							Type  string `json:"type,omitempty"`
						} `json:"phones,omitempty"`
					} `json:"contacts,omitempty"`
					Interactive struct {
						Type        string `json:"type"`
						ButtonReply struct {
							Id    string `json:"id"`
							Title string `json:"title"`
						} `json:"button_reply,omitempty"`
						ListReply struct {
							Id          string `json:"id"`
							Title       string `json:"title"`
							Description string `json:"description,omitempty"`
						} `json:"list_reply,omitempty"`
					} `json:"interactive,omitempty"`
					Reaction struct {
						MessageId string `json:"message_id"`
						Emoji     string `json:"emoji"`
					} `json:"reaction,omitempty"`
				} `json:"messages,omitempty"`
				Statuses []struct {
					Id           string `json:"id"`
					Status       string `json:"status"`
					Timestamp    string `json:"timestamp"`
					RecipientId  string `json:"recipient_id"`
					Conversation struct {
						Id                  string `json:"id"`
						ExpirationTimestamp string `json:"expiration_timestamp,omitempty"`
						Origin              struct {
							Type string `json:"type"`
						} `json:"origin"`
					} `json:"conversation,omitempty"`
					Pricing struct {
						Billable     bool   `json:"billable"`
						PricingModel string `json:"pricing_model"`
						Category     string `json:"category"`
					} `json:"pricing,omitempty"`
				} `json:"statuses,omitempty"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
} */

type WhatsAppMessageResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input string `json:"input"`
		WaId  string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		Id string `json:"id"`
	} `json:"messages"`
}

type WhatsAppErrorResponse struct {
	Error struct {
		Message   string `json:"message"`
		Type      string `json:"type"`
		Code      int    `json:"code"`
		ErrorData struct {
			MessagingProduct string `json:"messaging_product"`
			Details          string `json:"details"`
		} `json:"error_data,omitempty"`
		ErrorSubcode int    `json:"error_subcode,omitempty"`
		FbtraceId    string `json:"fbtrace_id,omitempty"`
	} `json:"error"`
}

type MediaUploadResponse struct {
	Id string `json:"id"`
}

type BusinessProfile struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description,omitempty"`
	Email       string   `json:"email,omitempty"`
	Address     string   `json:"address,omitempty"`
	Website     []string `json:"website,omitempty"`
}
