package ecomm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server "github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
)

type AmazonTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// Amazon SP-API FinancialEvents Response Structures
type FinancialEventsResponse struct {
	Payload struct {
		FinancialEvents FinancialEvents `json:"FinancialEvents"`
		NextToken       string          `json:"NextToken,omitempty"`
	} `json:"payload"`
}

type FinancialEvents struct {
	ShipmentEventList                      []ShipmentEvent                      `json:"ShipmentEventList,omitempty"`
	RefundEventList                        []ShipmentEvent                      `json:"RefundEventList,omitempty"`
	GuaranteeClaimEventList                []ShipmentEvent                      `json:"GuaranteeClaimEventList,omitempty"`
	ChargebackEventList                    []ShipmentEvent                      `json:"ChargebackEventList,omitempty"`
	PayWithAmazonEventList                 []PayWithAmazonEvent                 `json:"PayWithAmazonEventList,omitempty"`
	ServiceProviderCreditEventList         []SolutionProviderCreditEvent        `json:"ServiceProviderCreditEventList,omitempty"`
	RetrochargeEventList                   []RetrochargeEvent                   `json:"RetrochargeEventList,omitempty"`
	RentalTransactionEventList             []RentalTransactionEvent             `json:"RentalTransactionEventList,omitempty"`
	ProductAdsPaymentEventList             []ProductAdsPaymentEvent             `json:"ProductAdsPaymentEventList,omitempty"`
	ServiceFeeEventList                    []ServiceFeeEvent                    `json:"ServiceFeeEventList,omitempty"`
	SellerDealPaymentEventList             []SellerDealPaymentEvent             `json:"SellerDealPaymentEventList,omitempty"`
	DebtRecoveryEventList                  []DebtRecoveryEvent                  `json:"DebtRecoveryEventList,omitempty"`
	LoanServicingEventList                 []LoanServicingEvent                 `json:"LoanServicingEventList,omitempty"`
	AdjustmentEventList                    []AdjustmentEvent                    `json:"AdjustmentEventList,omitempty"`
	SAFETReimbursementEventList            []SAFETReimbursementEvent            `json:"SAFETReimbursementEventList,omitempty"`
	SellerReviewEnrollmentPaymentEventList []SellerReviewEnrollmentPaymentEvent `json:"SellerReviewEnrollmentPaymentEventList,omitempty"`
	FBALiquidationEventList                []FBALiquidationEvent                `json:"FBALiquidationEventList,omitempty"`
	CouponPaymentEventList                 []CouponPaymentEvent                 `json:"CouponPaymentEventList,omitempty"`
	ImagingServicesFeeEventList            []ImagingServicesFeeEvent            `json:"ImagingServicesFeeEventList,omitempty"`
	NetworkComminglingTransactionEventList []NetworkComminglingTransactionEvent `json:"NetworkComminglingTransactionEventList,omitempty"`
	AffordabilityExpenseEventList          []AffordabilityExpenseEvent          `json:"AffordabilityExpenseEventList,omitempty"`
	AffordabilityExpenseReversalEventList  []AffordabilityExpenseEvent          `json:"AffordabilityExpenseReversalEventList,omitempty"`
	RemovalShipmentEventList               []RemovalShipmentEvent               `json:"RemovalShipmentEventList,omitempty"`
	RemovalShipmentAdjustmentEventList     []RemovalShipmentAdjustmentEvent     `json:"RemovalShipmentAdjustmentEventList,omitempty"`
	TrialShipmentEventList                 []TrialShipmentEvent                 `json:"TrialShipmentEventList,omitempty"`
	TDSReimbursementEventList              []TDSReimbursementEvent              `json:"TDSReimbursementEventList,omitempty"`
	AdhocDisbursementEventList             []AdhocDisbursementEvent             `json:"AdhocDisbursementEventList,omitempty"`
	TaxWithholdingEventList                []TaxWithholdingEvent                `json:"TaxWithholdingEventList,omitempty"`
	ChargeRefundEventList                  []ChargeRefundEvent                  `json:"ChargeRefundEventList,omitempty"`
	FailedAdhocDisbursementEventList       []FailedAdhocDisbursementEvent       `json:"FailedAdhocDisbursementEventList,omitempty"`
	ValueAddedServiceChargeEventList       []ValueAddedServiceChargeEvent       `json:"ValueAddedServiceChargeEventList,omitempty"`
	CapacityReservationBillingEventList    []CapacityReservationBillingEvent    `json:"CapacityReservationBillingEventList,omitempty"`
	PaymentEventList                       []DebtRecoveryEvent                  `json:"PaymentEventList,omitempty"`
	CreateInboundShipmentPlanEventList     []CreateInboundShipmentPlanEvent     `json:"CreateInboundShipmentPlanEventList,omitempty"`
	ShippingLabelEventList                 []ShippingLabelEvent                 `json:"ShippingLabelEventList,omitempty"`
}

type Currency struct {
	CurrencyCode   string  `json:"CurrencyCode"`
	CurrencyAmount float64 `json:"CurrencyAmount"`
}

type ShipmentEvent struct {
	AmazonOrderId              string            `json:"AmazonOrderId,omitempty"`
	SellerOrderId              string            `json:"SellerOrderId,omitempty"`
	MarketplaceName            string            `json:"MarketplaceName,omitempty"`
	OrderChargeList            []ChargeComponent `json:"OrderChargeList,omitempty"`
	OrderChargeAdjustmentList  []ChargeComponent `json:"OrderChargeAdjustmentList,omitempty"`
	ShipmentFeeList            []FeeComponent    `json:"ShipmentFeeList,omitempty"`
	ShipmentFeeAdjustmentList  []FeeComponent    `json:"ShipmentFeeAdjustmentList,omitempty"`
	OrderFeeList               []FeeComponent    `json:"OrderFeeList,omitempty"`
	OrderFeeAdjustmentList     []FeeComponent    `json:"OrderFeeAdjustmentList,omitempty"`
	DirectPaymentList          []DirectPayment   `json:"DirectPaymentList,omitempty"`
	PostedDate                 string            `json:"PostedDate,omitempty"`
	ShipmentItemList           []ShipmentItem    `json:"ShipmentItemList,omitempty"`
	ShipmentItemAdjustmentList []ShipmentItem    `json:"ShipmentItemAdjustmentList,omitempty"`
}

type ChargeComponent struct {
	ChargeType   string   `json:"ChargeType,omitempty"`
	ChargeAmount Currency `json:"ChargeAmount,omitempty"`
}

type FeeComponent struct {
	FeeType   string   `json:"FeeType,omitempty"`
	FeeAmount Currency `json:"FeeAmount,omitempty"`
}

type DirectPayment struct {
	DirectPaymentType   string   `json:"DirectPaymentType,omitempty"`
	DirectPaymentAmount Currency `json:"DirectPaymentAmount,omitempty"`
}

type ShipmentItem struct {
	SellerSKU                string                 `json:"SellerSKU,omitempty"`
	OrderItemId              string                 `json:"OrderItemId,omitempty"`
	OrderAdjustmentItemId    string                 `json:"OrderAdjustmentItemId,omitempty"`
	QuantityShipped          int                    `json:"QuantityShipped,omitempty"`
	ItemChargeList           []ChargeComponent      `json:"ItemChargeList,omitempty"`
	ItemChargeAdjustmentList []ChargeComponent      `json:"ItemChargeAdjustmentList,omitempty"`
	ItemFeeList              []FeeComponent         `json:"ItemFeeList,omitempty"`
	ItemFeeAdjustmentList    []FeeComponent         `json:"ItemFeeAdjustmentList,omitempty"`
	ItemTaxWithheldList      []TaxWithheldComponent `json:"ItemTaxWithheldList,omitempty"`
	PromotionList            []Promotion            `json:"PromotionList,omitempty"`
	PromotionAdjustmentList  []Promotion            `json:"PromotionAdjustmentList,omitempty"`
	CostOfPointsGranted      Currency               `json:"CostOfPointsGranted,omitempty"`
	CostOfPointsReturned     Currency               `json:"CostOfPointsReturned,omitempty"`
}

type TaxWithheldComponent struct {
	TaxCollectionModel string            `json:"TaxCollectionModel,omitempty"`
	TaxesWithheld      []ChargeComponent `json:"TaxesWithheld,omitempty"`
}

type Promotion struct {
	PromotionType   string   `json:"PromotionType,omitempty"`
	PromotionId     string   `json:"PromotionId,omitempty"`
	PromotionAmount Currency `json:"PromotionAmount,omitempty"`
}

type PayWithAmazonEvent struct {
	SellerOrderId         string          `json:"SellerOrderId,omitempty"`
	TransactionPostedDate string          `json:"TransactionPostedDate,omitempty"`
	BusinessObjectType    string          `json:"BusinessObjectType,omitempty"`
	SalesChannel          string          `json:"SalesChannel,omitempty"`
	Charge                ChargeComponent `json:"Charge,omitempty"`
	FeeList               []FeeComponent  `json:"FeeList,omitempty"`
	PaymentAmountType     string          `json:"PaymentAmountType,omitempty"`
	AmountDescription     string          `json:"AmountDescription,omitempty"`
	FulfillmentChannel    string          `json:"FulfillmentChannel,omitempty"`
	StoreName             string          `json:"StoreName,omitempty"`
}

type SolutionProviderCreditEvent struct {
	ProviderTransactionType string   `json:"ProviderTransactionType,omitempty"`
	SellerOrderId           string   `json:"SellerOrderId,omitempty"`
	MarketplaceId           string   `json:"MarketplaceId,omitempty"`
	MarketplaceCountryCode  string   `json:"MarketplaceCountryCode,omitempty"`
	SellerId                string   `json:"SellerId,omitempty"`
	SellerStoreName         string   `json:"SellerStoreName,omitempty"`
	ProviderId              string   `json:"ProviderId,omitempty"`
	ProviderStoreName       string   `json:"ProviderStoreName,omitempty"`
	TransactionAmount       Currency `json:"TransactionAmount,omitempty"`
	TransactionCreationDate string   `json:"TransactionCreationDate,omitempty"`
}

type RetrochargeEvent struct {
	RetrochargeEventType       string                 `json:"RetrochargeEventType,omitempty"`
	AmazonOrderId              string                 `json:"AmazonOrderId,omitempty"`
	PostedDate                 string                 `json:"PostedDate,omitempty"`
	BaseTax                    Currency               `json:"BaseTax,omitempty"`
	ShippingTax                Currency               `json:"ShippingTax,omitempty"`
	MarketplaceName            string                 `json:"MarketplaceName,omitempty"`
	RetrochargeTaxWithheldList []TaxWithheldComponent `json:"RetrochargeTaxWithheldList,omitempty"`
}

type RentalTransactionEvent struct {
	AmazonOrderId         string                 `json:"AmazonOrderId,omitempty"`
	RentalEventType       string                 `json:"RentalEventType,omitempty"`
	ExtensionLength       int                    `json:"ExtensionLength,omitempty"`
	PostedDate            string                 `json:"PostedDate,omitempty"`
	RentalChargeList      []ChargeComponent      `json:"RentalChargeList,omitempty"`
	RentalFeeList         []FeeComponent         `json:"RentalFeeList,omitempty"`
	MarketplaceName       string                 `json:"MarketplaceName,omitempty"`
	RentalInitialValue    Currency               `json:"RentalInitialValue,omitempty"`
	RentalReimbursement   Currency               `json:"RentalReimbursement,omitempty"`
	RentalTaxWithheldList []TaxWithheldComponent `json:"RentalTaxWithheldList,omitempty"`
}

type ProductAdsPaymentEvent struct {
	PostedDate       string   `json:"PostedDate,omitempty"`
	TransactionType  string   `json:"TransactionType,omitempty"`
	InvoiceId        string   `json:"InvoiceId,omitempty"`
	BaseValue        Currency `json:"BaseValue,omitempty"`
	TaxValue         Currency `json:"TaxValue,omitempty"`
	TransactionValue Currency `json:"TransactionValue,omitempty"`
}

type ServiceFeeEvent struct {
	AmazonOrderId  string         `json:"AmazonOrderId,omitempty"`
	FeeReason      string         `json:"FeeReason,omitempty"`
	FeeList        []FeeComponent `json:"FeeList,omitempty"`
	SellerSKU      string         `json:"SellerSKU,omitempty"`
	FnSKU          string         `json:"FnSKU,omitempty"`
	FeeDescription string         `json:"FeeDescription,omitempty"`
	ASIN           string         `json:"ASIN,omitempty"`
}

type SellerDealPaymentEvent struct {
	PostedDate      string   `json:"PostedDate,omitempty"`
	DealId          string   `json:"DealId,omitempty"`
	DealDescription string   `json:"DealDescription,omitempty"`
	EventType       string   `json:"EventType,omitempty"`
	FeeType         string   `json:"FeeType,omitempty"`
	FeeAmount       Currency `json:"FeeAmount,omitempty"`
	TaxAmount       Currency `json:"TaxAmount,omitempty"`
	TotalAmount     Currency `json:"TotalAmount,omitempty"`
}

type DebtRecoveryEvent struct {
	DebtRecoveryType     string             `json:"DebtRecoveryType,omitempty"`
	RecoveryAmount       Currency           `json:"RecoveryAmount,omitempty"`
	OverPaymentCredit    Currency           `json:"OverPaymentCredit,omitempty"`
	DebtRecoveryItemList []DebtRecoveryItem `json:"DebtRecoveryItemList,omitempty"`
	ChargeInstrumentList []ChargeInstrument `json:"ChargeInstrumentList,omitempty"`
}

type DebtRecoveryItem struct {
	RecoveryAmount Currency `json:"RecoveryAmount,omitempty"`
	OriginalAmount Currency `json:"OriginalAmount,omitempty"`
	GroupBeginDate string   `json:"GroupBeginDate,omitempty"`
	GroupEndDate   string   `json:"GroupEndDate,omitempty"`
}

type ChargeInstrument struct {
	Description string   `json:"Description,omitempty"`
	Tail        string   `json:"Tail,omitempty"`
	Amount      Currency `json:"Amount,omitempty"`
}

type LoanServicingEvent struct {
	LoanAmount              Currency `json:"LoanAmount,omitempty"`
	SourceBusinessEventType string   `json:"SourceBusinessEventType,omitempty"`
}

type AdjustmentEvent struct {
	AdjustmentType     string           `json:"AdjustmentType,omitempty"`
	PostedDate         string           `json:"PostedDate,omitempty"`
	AdjustmentAmount   Currency         `json:"AdjustmentAmount,omitempty"`
	AdjustmentItemList []AdjustmentItem `json:"AdjustmentItemList,omitempty"`
}

type AdjustmentItem struct {
	Quantity           string   `json:"Quantity,omitempty"`
	PerUnitAmount      Currency `json:"PerUnitAmount,omitempty"`
	TotalAmount        Currency `json:"TotalAmount,omitempty"`
	SellerSKU          string   `json:"SellerSKU,omitempty"`
	FnSKU              string   `json:"FnSKU,omitempty"`
	ProductDescription string   `json:"ProductDescription,omitempty"`
	ASIN               string   `json:"ASIN,omitempty"`
}

type SAFETReimbursementEvent struct {
	PostedDate                 string                   `json:"PostedDate,omitempty"`
	SAFETClaimId               string                   `json:"SAFETClaimId,omitempty"`
	ReimbursedAmount           Currency                 `json:"ReimbursedAmount,omitempty"`
	ReasonCode                 string                   `json:"ReasonCode,omitempty"`
	SAFETReimbursementItemList []SAFETReimbursementItem `json:"SAFETReimbursementItemList,omitempty"`
}

type SAFETReimbursementItem struct {
	ItemChargeList     []ChargeComponent `json:"ItemChargeList,omitempty"`
	ProductDescription string            `json:"ProductDescription,omitempty"`
	Quantity           string            `json:"Quantity,omitempty"`
}

type SellerReviewEnrollmentPaymentEvent struct {
	PostedDate      string          `json:"PostedDate,omitempty"`
	EnrollmentId    string          `json:"EnrollmentId,omitempty"`
	ParentASIN      string          `json:"ParentASIN,omitempty"`
	FeeComponent    FeeComponent    `json:"FeeComponent,omitempty"`
	ChargeComponent ChargeComponent `json:"ChargeComponent,omitempty"`
	TotalAmount     Currency        `json:"TotalAmount,omitempty"`
}

type FBALiquidationEvent struct {
	PostedDate                string   `json:"PostedDate,omitempty"`
	OriginalRemovalOrderId    string   `json:"OriginalRemovalOrderId,omitempty"`
	LiquidationProceedsAmount Currency `json:"LiquidationProceedsAmount,omitempty"`
	LiquidationFeeAmount      Currency `json:"LiquidationFeeAmount,omitempty"`
}

type CouponPaymentEvent struct {
	PostedDate              string          `json:"PostedDate,omitempty"`
	CouponId                string          `json:"CouponId,omitempty"`
	SellerCouponDescription string          `json:"SellerCouponDescription,omitempty"`
	ClipOrRedemptionCount   int             `json:"ClipOrRedemptionCount,omitempty"`
	PaymentEventId          string          `json:"PaymentEventId,omitempty"`
	FeeComponent            FeeComponent    `json:"FeeComponent,omitempty"`
	ChargeComponent         ChargeComponent `json:"ChargeComponent,omitempty"`
	TotalAmount             Currency        `json:"TotalAmount,omitempty"`
}

type ImagingServicesFeeEvent struct {
	ImagingRequestBillingItemID string         `json:"ImagingRequestBillingItemID,omitempty"`
	ASIN                        string         `json:"ASIN,omitempty"`
	PostedDate                  string         `json:"PostedDate,omitempty"`
	FeeList                     []FeeComponent `json:"FeeList,omitempty"`
}

type NetworkComminglingTransactionEvent struct {
	TransactionType    string   `json:"TransactionType,omitempty"`
	PostedDate         string   `json:"PostedDate,omitempty"`
	NetCoTransactionID string   `json:"NetCoTransactionID,omitempty"`
	SwapReason         string   `json:"SwapReason,omitempty"`
	ASIN               string   `json:"ASIN,omitempty"`
	MarketplaceId      string   `json:"MarketplaceId,omitempty"`
	TaxExclusiveAmount Currency `json:"TaxExclusiveAmount,omitempty"`
	TaxAmount          Currency `json:"TaxAmount,omitempty"`
}

type AffordabilityExpenseEvent struct {
	AmazonOrderId   string   `json:"AmazonOrderId,omitempty"`
	PostedDate      string   `json:"PostedDate,omitempty"`
	MarketplaceId   string   `json:"MarketplaceId,omitempty"`
	TransactionType string   `json:"TransactionType,omitempty"`
	BaseExpense     Currency `json:"BaseExpense,omitempty"`
	TaxTypeIGST     Currency `json:"TaxTypeIGST,omitempty"`
	TaxTypeCGST     Currency `json:"TaxTypeCGST,omitempty"`
	TaxTypeSGST     Currency `json:"TaxTypeSGST,omitempty"`
	TotalExpense    Currency `json:"TotalExpense,omitempty"`
}

type RemovalShipmentEvent struct {
	PostedDate              string                `json:"PostedDate,omitempty"`
	OrderId                 string                `json:"OrderId,omitempty"`
	TransactionType         string                `json:"TransactionType,omitempty"`
	RemovalShipmentItemList []RemovalShipmentItem `json:"RemovalShipmentItemList,omitempty"`
}

type RemovalShipmentItem struct {
	RemovalShipmentItemId string   `json:"RemovalShipmentItemId,omitempty"`
	TaxCollectionModel    string   `json:"TaxCollectionModel,omitempty"`
	FulfillmentNetworkSKU string   `json:"FulfillmentNetworkSKU,omitempty"`
	Quantity              int      `json:"Quantity,omitempty"`
	Revenue               Currency `json:"Revenue,omitempty"`
	FeeAmount             Currency `json:"FeeAmount,omitempty"`
	TaxAmount             Currency `json:"TaxAmount,omitempty"`
	TaxWithheld           Currency `json:"TaxWithheld,omitempty"`
}

type RemovalShipmentAdjustmentEvent struct {
	PostedDate                        string                `json:"PostedDate,omitempty"`
	AdjustmentEventId                 string                `json:"AdjustmentEventId,omitempty"`
	MerchantOrderId                   string                `json:"MerchantOrderId,omitempty"`
	OrderId                           string                `json:"OrderId,omitempty"`
	TransactionType                   string                `json:"TransactionType,omitempty"`
	RemovalShipmentItemAdjustmentList []RemovalShipmentItem `json:"RemovalShipmentItemAdjustmentList,omitempty"`
}

type TrialShipmentEvent struct {
	AmazonOrderId         string         `json:"AmazonOrderId,omitempty"`
	FinancialEventGroupId string         `json:"FinancialEventGroupId,omitempty"`
	PostedDate            string         `json:"PostedDate,omitempty"`
	SKU                   string         `json:"SKU,omitempty"`
	FeeList               []FeeComponent `json:"FeeList,omitempty"`
}

type TDSReimbursementEvent struct {
	PostedDate       string   `json:"PostedDate,omitempty"`
	TdsOrderId       string   `json:"TdsOrderId,omitempty"`
	ReimbursedAmount Currency `json:"ReimbursedAmount,omitempty"`
}

type AdhocDisbursementEvent struct {
	TransactionType         string   `json:"TransactionType,omitempty"`
	PostedDate              string   `json:"PostedDate,omitempty"`
	SourceOrderId           string   `json:"SourceOrderId,omitempty"`
	SourceOrderItemId       string   `json:"SourceOrderItemId,omitempty"`
	AdhocDisbursementAmount Currency `json:"AdhocDisbursementAmount,omitempty"`
}

type TaxWithholdingEvent struct {
	PostedDate           string                 `json:"PostedDate,omitempty"`
	BaseAmount           Currency               `json:"BaseAmount,omitempty"`
	WithheldAmount       Currency               `json:"WithheldAmount,omitempty"`
	TaxWithholdingPeriod TaxWithholdingPeriod   `json:"TaxWithholdingPeriod,omitempty"`
	TaxesWithheld        []TaxWithheldComponent `json:"TaxesWithheld,omitempty"`
}

type TaxWithholdingPeriod struct {
	StartDate string `json:"StartDate,omitempty"`
	EndDate   string `json:"EndDate,omitempty"`
}

type ChargeRefundEvent struct {
	PostedDate   string   `json:"PostedDate,omitempty"`
	ReasonCode   string   `json:"ReasonCode,omitempty"`
	ReferenceId  string   `json:"ReferenceId,omitempty"`
	RefundAmount Currency `json:"RefundAmount,omitempty"`
}

type FailedAdhocDisbursementEvent struct {
	FundsRequestType string   `json:"FundsRequestType,omitempty"`
	PrincipalAmount  Currency `json:"PrincipalAmount,omitempty"`
	EscrowAmount     Currency `json:"EscrowAmount,omitempty"`
}

type ValueAddedServiceChargeEvent struct {
	TransactionType   string   `json:"TransactionType,omitempty"`
	PostedDate        string   `json:"PostedDate,omitempty"`
	Description       string   `json:"Description,omitempty"`
	TransactionAmount Currency `json:"TransactionAmount,omitempty"`
}

type CapacityReservationBillingEvent struct {
	TransactionType   string   `json:"TransactionType,omitempty"`
	PostedDate        string   `json:"PostedDate,omitempty"`
	Description       string   `json:"Description,omitempty"`
	TransactionAmount Currency `json:"TransactionAmount,omitempty"`
}

type CreateInboundShipmentPlanEvent struct {
	ShipmentPlanId string         `json:"ShipmentPlanId,omitempty"`
	PostedDate     string         `json:"PostedDate,omitempty"`
	FeeList        []FeeComponent `json:"FeeList,omitempty"`
}

type ShippingLabelEvent struct {
	ShipmentId string         `json:"ShipmentId,omitempty"`
	PostedDate string         `json:"PostedDate,omitempty"`
	FeeList    []FeeComponent `json:"FeeList,omitempty"`
}

// Order Items Response Structures
type OrderItemsResponse struct {
	Payload struct {
		AmazonOrderId string      `json:"AmazonOrderId"`
		OrderItems    []OrderItem `json:"OrderItems"`
		NextToken     string      `json:"NextToken,omitempty"`
	} `json:"payload"`
}

type OrderItem struct {
	ASIN                       string        `json:"ASIN,omitempty"`
	SellerSKU                  string        `json:"SellerSKU,omitempty"`
	OrderItemId                string        `json:"OrderItemId,omitempty"`
	Title                      string        `json:"Title,omitempty"`
	QuantityOrdered            int           `json:"QuantityOrdered,omitempty"`
	QuantityShipped            int           `json:"QuantityShipped,omitempty"`
	ProductInfo                ProductInfo   `json:"ProductInfo,omitempty"`
	PointsGranted              PointsGranted `json:"PointsGranted,omitempty"`
	ItemPrice                  Money         `json:"ItemPrice,omitempty"`
	ShippingPrice              Money         `json:"ShippingPrice,omitempty"`
	ItemTax                    Money         `json:"ItemTax,omitempty"`
	ShippingTax                Money         `json:"ShippingTax,omitempty"`
	ShippingDiscount           Money         `json:"ShippingDiscount,omitempty"`
	ShippingDiscountTax        Money         `json:"ShippingDiscountTax,omitempty"`
	PromotionDiscount          Money         `json:"PromotionDiscount,omitempty"`
	PromotionDiscountTax       Money         `json:"PromotionDiscountTax,omitempty"`
	PromotionIds               []string      `json:"PromotionIds,omitempty"`
	CODFee                     Money         `json:"CODFee,omitempty"`
	CODFeeDiscount             Money         `json:"CODFeeDiscount,omitempty"`
	IsGift                     bool          `json:"IsGift,omitempty"`
	ConditionNote              string        `json:"ConditionNote,omitempty"`
	ConditionId                string        `json:"ConditionId,omitempty"`
	ConditionSubtypeId         string        `json:"ConditionSubtypeId,omitempty"`
	ScheduledDeliveryStartDate string        `json:"ScheduledDeliveryStartDate,omitempty"`
	ScheduledDeliveryEndDate   string        `json:"ScheduledDeliveryEndDate,omitempty"`
	PriceDesignation           string        `json:"PriceDesignation,omitempty"`
	TaxCollection              TaxCollection `json:"TaxCollection,omitempty"`
	SerialNumberRequired       bool          `json:"SerialNumberRequired,omitempty"`
	IsTransparency             bool          `json:"IsTransparency,omitempty"`
	IossNumber                 string        `json:"IossNumber,omitempty"`
	StoreChainStoreId          string        `json:"StoreChainStoreId,omitempty"`
	DeemedResellerCategory     string        `json:"DeemedResellerCategory,omitempty"`
	BuyerInfo                  BuyerInfo     `json:"BuyerInfo,omitempty"`
}

type ProductInfo struct {
	NumberOfItems int `json:"NumberOfItems,omitempty"`
}

type PointsGranted struct {
	PointsNumber        int   `json:"PointsNumber,omitempty"`
	PointsMonetaryValue Money `json:"PointsMonetaryValue,omitempty"`
}

type Money struct {
	CurrencyCode string `json:"CurrencyCode,omitempty"`
	Amount       string `json:"Amount,omitempty"`
}

type TaxCollection struct {
	Model            string `json:"Model,omitempty"`
	ResponsibleParty string `json:"ResponsibleParty,omitempty"`
}

type BuyerInfo struct {
	BuyerCustomizedInfo BuyerCustomizedInfo `json:"BuyerCustomizedInfo,omitempty"`
	GiftWrapPrice       Money               `json:"GiftWrapPrice,omitempty"`
	GiftWrapTax         Money               `json:"GiftWrapTax,omitempty"`
	GiftMessageText     string              `json:"GiftMessageText,omitempty"`
	GiftWrapLevel       string              `json:"GiftWrapLevel,omitempty"`
}

type BuyerCustomizedInfo struct {
	CustomizedURL string `json:"CustomizedURL,omitempty"`
}

// Financial Event Groups Response Structures
type FinancialEventGroupsResponse struct {
	Payload struct {
		FinancialEventGroupList []FinancialEventGroup `json:"FinancialEventGroupList"`
		NextToken               string                `json:"NextToken,omitempty"`
	} `json:"payload"`
}

type FinancialEventGroup struct {
	FinancialEventGroupId    string   `json:"FinancialEventGroupId,omitempty"`
	ProcessingStatus         string   `json:"ProcessingStatus,omitempty"`
	FundTransferStatus       string   `json:"FundTransferStatus,omitempty"`
	OriginalTotal            Currency `json:"OriginalTotal,omitempty"`
	ConvertedTotal           Currency `json:"ConvertedTotal,omitempty"`
	FundTransferDate         string   `json:"FundTransferDate,omitempty"`
	TraceId                  string   `json:"TraceId,omitempty"`
	AccountTail              string   `json:"AccountTail,omitempty"`
	BeginningBalance         Currency `json:"BeginningBalance,omitempty"`
	FinancialEventGroupStart string   `json:"FinancialEventGroupStart,omitempty"`
	FinancialEventGroupEnd   string   `json:"FinancialEventGroupEnd,omitempty"`
}

// Simplified Financial Event Structure - Flat format with amounts only
type SimplifiedFinancialEvent struct {
	EventType       string  `json:"event_type"`
	PostedDate      string  `json:"posted_date,omitempty"`
	AmazonOrderId   string  `json:"amazon_order_id,omitempty"`
	SellerOrderId   string  `json:"seller_order_id,omitempty"`
	MarketplaceName string  `json:"marketplace_name,omitempty"`
	ASIN            string  `json:"asin,omitempty"`
	SellerSKU       string  `json:"seller_sku,omitempty"`
	TransactionType string  `json:"transaction_type,omitempty"`
	Amount          float64 `json:"amount"`
	CurrencyCode    string  `json:"currency_code"`
	FeeType         string  `json:"fee_type,omitempty"`
	ChargeType      string  `json:"charge_type,omitempty"`
	Description     string  `json:"description,omitempty"`
	Quantity        int     `json:"quantity,omitempty"`
	ReasonCode      string  `json:"reason_code,omitempty"`
	ReferenceId     string  `json:"reference_id,omitempty"`
}

type AmazonTool struct {
	tools.Tool
	AmazonAccount AmazonAccount `json:"amazon_account"`
	AuthName      string        `json:"auth_name"`
}
type amazonToolWithToken struct {
	tools.Tool
	AmazonAccount amazonAccountWithToken
	AuthName      string
}

const (
	SellerBaseUrl = "https://sellingpartnerapi-eu.amazon.com"
)

func (amazonTool *AmazonTool) GetActionsList() []tools.ActionInfo {
	return []tools.ActionInfo{
		{Name: GetOrders},
		{Name: GetOrderItems},
		{Name: GetFinancialEvents},
		{Name: GetFinancialEventGroups},
		{Name: Login},
		{Name: RenewToken},
		{Name: GetSsoUrl},
		{Name: StopAutoRenew},
	}
}

func (amazonTool *AmazonTool) GetSpec() tools.Tooling {
	return amazonTool
}

func (amazonTool *AmazonTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &amazonTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (amazonTool *AmazonTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("AmazonTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case GetOrders:
		toolResult, toolRequest, persistStore, err = amazonTool.GetOrders(ctx, params)
	case GetOrderItems:
		toolResult, toolRequest, persistStore, err = amazonTool.GetOrderItems(ctx, params)
	case GetFinancialEvents:
		toolResult, toolRequest, persistStore, err = amazonTool.GetFinancialEvents(ctx, params)
	case GetFinancialEventGroups:
		toolResult, toolRequest, persistStore, err = amazonTool.GetFinancialEventGroups(ctx, params)
	case Login:
		toolResult, toolRequest, persistStore, err = amazonTool.Login(ctx, projectId, tenantId, params, "")
	case RenewToken:
		toolResult, toolRequest, persistStore, err = amazonTool.RenewToken(ctx, projectId, tenantId, params)
	case GetSsoUrl:
		toolResult, toolRequest, persistStore, err = amazonTool.GetSsoUrl(ctx, projectId, tenantId, params)
	case StopAutoRenew:
		toolResult, toolRequest, persistStore, err = amazonTool.StopAutoRenew(ctx, projectId, tenantId, params)
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

		hookResult, err := amazonTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (amazonTool *AmazonTool) GetOrders(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetOrders Execute - Start")

	nextToken := ""

	// Convert params to query parameters
	queryParams := map[string]string{}
	for k, v := range params {
		queryParams[k] = v.(string)
	}

	// Call recursively to get all orders
	consolidatedResponse, err := amazonTool.getOrdersRecursive(ctx, queryParams, nextToken)
	if err != nil {
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["orders"] = consolidatedResponse
	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (amazonTool *AmazonTool) getOrdersRecursive(ctx context.Context, queryParams map[string]string, nextToken string) ([]interface{}, error) {
	logs.WithContext(ctx).Debug("getOrdersRecursive Execute - Start")
	var allOrders []interface{}
	url := fmt.Sprint(SellerBaseUrl, "/orders/v0/orders")
	headers := http.Header{}
	headers.Set("x-amz-access-token", amazonTool.AmazonAccount.AccessToken)
	headers.Set("x-amz-date", time.Now().UTC().Format(time.RFC3339))
	headers.Set("user-agent", amazonTool.AmazonAccount.UserAgent)
	headers.Set("content-type", "application/json")

	// Prepare query parameters based on whether NextToken is provided
	currentQueryParams := make(map[string]string)
	if nextToken != "" {
		// When NextToken is provided, only pass the NextToken parameter
		currentQueryParams["NextToken"] = nextToken
	} else {
		// For the first call, pass all original parameters
		for k, v := range queryParams {
			currentQueryParams[k] = v
		}
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, currentQueryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	// Parse response to extract orders and next token
	responseMap, ok := res.(map[string]interface{})
	if !ok {
		logs.WithContext(ctx).Error("Response is not a map")
		return nil, errors.New("invalid response format")
	}

	// Extract orders from current response
	if responsePayload, exists := responseMap["payload"]; exists {
		if payloadMap, ok := responsePayload.(map[string]interface{}); ok {
			if ordersData, exists := payloadMap["Orders"]; exists {
				if ordersList, ok := ordersData.([]interface{}); ok {
					allOrders = append(allOrders, ordersList...)
				}
			}
			if nt, exists := payloadMap["NextToken"]; exists {
				nextToken = nt.(string)
				logs.WithContext(ctx).Info("Found NextToken: %s, making recursive call")
				// Recursive call with NextToken
				nextedOrders, err := amazonTool.getOrdersRecursive(ctx, queryParams, nextToken)
				if err != nil {
					return nil, err
				}
				allOrders = append(allOrders, nextedOrders...)
			}
		}
	}

	// No more NextToken, return consolidated response
	logs.WithContext(ctx).Debug(fmt.Sprintf("No more NextToken found. Total orders collected: %d", len(allOrders)))

	return allOrders, nil
}

func (amazonTool *AmazonTool) GetOrderItems(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetOrderItems Execute - Start")

	// Extract order_id from params
	orderId, exists := params["order_id"]
	if !exists {
		return nil, nil, false, errors.New("order_id parameter is required")
	}
	orderIdStr := orderId.(string)
	if orderIdStr == "" {
		return nil, nil, false, errors.New("order_id parameter cannot be empty")
	}

	// Build URL with order_id in path
	url := fmt.Sprint(SellerBaseUrl, "/orders/v0/orders/", orderIdStr, "/orderItems")
	headers := http.Header{}
	headers.Set("x-amz-access-token", amazonTool.AmazonAccount.AccessToken)
	headers.Set("x-amz-date", time.Now().UTC().Format(time.RFC3339))
	headers.Set("user-agent", amazonTool.AmazonAccount.UserAgent)
	headers.Set("content-type", "application/json")

	// Prepare query parameters - exclude order_id since it's in the path
	queryParams := make(map[string]string)
	for k, v := range params {
		if k != "order_id" {
			queryParams[k] = v.(string)
		}
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	// Parse response
	responseMap, ok := res.(map[string]interface{})
	if !ok {
		logs.WithContext(ctx).Error("Response is not a map")
		return nil, nil, false, errors.New("invalid response format")
	}

	toolResult = make(map[string]interface{})

	// Extract order items from response
	if responsePayload, exists := responseMap["payload"]; exists {
		if payloadMap, ok := responsePayload.(map[string]interface{}); ok {
			if orderItems, exists := payloadMap["OrderItems"]; exists {
				toolResult["order_items"] = orderItems
			}
			if amazonOrderId, exists := payloadMap["AmazonOrderId"]; exists {
				toolResult["amazon_order_id"] = amazonOrderId
			}
		}
	}

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (amazonTool *AmazonTool) GetFinancialEventGroups(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetFinancialEventGroups Execute - Start")

	nextToken := ""

	// Convert params to query parameters
	queryParams := map[string]string{}
	for k, v := range params {
		queryParams[k] = v.(string)
	}

	// Call recursively to get all financial event groups
	consolidatedResponse, err := amazonTool.getFinancialEventGroupsRecursive(ctx, queryParams, nextToken)
	if err != nil {
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["financial_event_groups"] = consolidatedResponse
	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (amazonTool *AmazonTool) getFinancialEventGroupsRecursive(ctx context.Context, queryParams map[string]string, nextToken string) ([]interface{}, error) {
	logs.WithContext(ctx).Debug("getFinancialEventGroupsRecursive Execute - Start")
	var allGroups []interface{}
	url := fmt.Sprint(SellerBaseUrl, "/finances/v0/financialEventGroups")
	headers := http.Header{}
	headers.Set("x-amz-access-token", amazonTool.AmazonAccount.AccessToken)
	headers.Set("x-amz-date", time.Now().UTC().Format(time.RFC3339))
	headers.Set("user-agent", amazonTool.AmazonAccount.UserAgent)
	headers.Set("content-type", "application/json")

	// Prepare query parameters based on whether NextToken is provided
	currentQueryParams := make(map[string]string)
	if nextToken != "" {
		// When NextToken is provided, only pass the NextToken parameter
		currentQueryParams["NextToken"] = nextToken
	} else {
		// For the first call, pass all original parameters
		for k, v := range queryParams {
			currentQueryParams[k] = v
		}
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, currentQueryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	// Parse response to extract financial event groups and next token
	responseMap, ok := res.(map[string]interface{})
	if !ok {
		logs.WithContext(ctx).Error("Response is not a map")
		return nil, errors.New("invalid response format")
	}

	// Extract financial event groups from current response
	if responsePayload, exists := responseMap["payload"]; exists {
		if payloadMap, ok := responsePayload.(map[string]interface{}); ok {
			if groupsData, exists := payloadMap["FinancialEventGroupList"]; exists {
				if groupsList, ok := groupsData.([]interface{}); ok {
					allGroups = append(allGroups, groupsList...)
				}
			}
			if nt, exists := payloadMap["NextToken"]; exists {
				nextToken = nt.(string)
				logs.WithContext(ctx).Info("Found NextToken: %s, making recursive call")
				// Recursive call with NextToken
				nextGroups, err := amazonTool.getFinancialEventGroupsRecursive(ctx, queryParams, nextToken)
				if err != nil {
					return nil, err
				}
				allGroups = append(allGroups, nextGroups...)
			}
		}
	}

	// No more NextToken, return consolidated response
	logs.WithContext(ctx).Debug(fmt.Sprintf("No more NextToken found. Total financial event groups collected: %d", len(allGroups)))

	return allGroups, nil
}

func (amazonTool *AmazonTool) GetFinancialEvents(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetFinancialEvents Execute - Start")

	nextToken := ""
	financialGroupId := ""
	if params["FinancialEventGroupId"] != nil {
		financialGroupId = params["FinancialEventGroupId"].(string)
	}
	orderId := ""
	if params["AmazonOrderId"] != nil {
		orderId = params["AmazonOrderId"].(string)
	}
	// Convert params to query parameters
	queryParams := map[string]string{}
	for k, v := range params {
		if k != "FinancialEventGroupId" && k != "AmazonOrderId" {
			queryParams[k] = v.(string)
		}
	}

	// Call recursively to get all financial events with structured merging
	consolidatedFinancialEvents, err := amazonTool.getFinancialEventsRecursive(ctx, queryParams, nextToken, financialGroupId, orderId)
	if err != nil {
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})

	// Check output_format parameter
	outputFormat := "simplified" // default
	if params["output_format"] != nil {
		outputFormat = params["output_format"].(string)
	}

	if outputFormat == "raw" {
		// Return complete structured financial events
		toolResult["financial_events"] = consolidatedFinancialEvents
	} else {
		// Return simplified flat structure (default)
		simplifiedEvents := amazonTool.convertToSimplifiedEvents(consolidatedFinancialEvents)
		toolResult["financial_events"] = simplifiedEvents
	}

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (amazonTool *AmazonTool) getFinancialEventsRecursive(ctx context.Context, queryParams map[string]string, nextToken string, financialGroupId string, orderId string) (*FinancialEvents, error) {
	logs.WithContext(ctx).Debug("getFinancialEventsRecursive Execute - Start")

	// Initialize consolidated financial events structure
	consolidatedEvents := &FinancialEvents{}

	return amazonTool.getFinancialEventsRecursiveHelper(ctx, queryParams, nextToken, consolidatedEvents, financialGroupId, orderId)
}

func (amazonTool *AmazonTool) getFinancialEventsRecursiveHelper(ctx context.Context, queryParams map[string]string, nextToken string, consolidatedEvents *FinancialEvents, financialGroupId string, orderId string) (*FinancialEvents, error) {
	url := fmt.Sprint(SellerBaseUrl, "/finances/v0/financialEvents")
	if financialGroupId != "" {
		url = fmt.Sprint(SellerBaseUrl, "/finances/v0/financialEventGroups/", financialGroupId, "/financialEvents")
	}
	if orderId != "" {
		url = fmt.Sprint(SellerBaseUrl, "/finances/v0/orders/", orderId, "/financialEvents")
	}
	headers := http.Header{}
	headers.Set("x-amz-access-token", amazonTool.AmazonAccount.AccessToken)
	headers.Set("x-amz-date", time.Now().UTC().Format(time.RFC3339))
	headers.Set("user-agent", amazonTool.AmazonAccount.UserAgent)
	headers.Set("content-type", "application/json")

	// Prepare query parameters based on whether NextToken is provided
	currentQueryParams := make(map[string]string)
	if nextToken != "" {
		// When NextToken is provided, only pass the NextToken parameter
		currentQueryParams["NextToken"] = nextToken
	} else {
		// For the first call, pass all original parameters except output_format
		for k, v := range queryParams {
			if k != "output_format" {
				currentQueryParams[k] = v
			}
		}
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, currentQueryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	// Parse response to extract financial events and next token
	responseMap, ok := res.(map[string]interface{})
	if !ok {
		logs.WithContext(ctx).Error("Response is not a map")
		return nil, errors.New("invalid response format")
	}

	// Extract financial events from current response
	if responsePayload, exists := responseMap["payload"]; exists {
		if payloadMap, ok := responsePayload.(map[string]interface{}); ok {
			if financialEventsData, exists := payloadMap["FinancialEvents"]; exists {
				// Parse current page's financial events
				eventsBytes, err := json.Marshal(financialEventsData)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal financial events: %w", err)
				}

				var currentPageEvents FinancialEvents
				err = json.Unmarshal(eventsBytes, &currentPageEvents)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal financial events: %w", err)
				}

				// Merge current page events with consolidated events
				amazonTool.mergeFinancialEvents(consolidatedEvents, &currentPageEvents)
			}

			if nt, exists := payloadMap["NextToken"]; exists {
				nextToken = nt.(string)
				logs.WithContext(ctx).Info("Found NextToken: %s, making recursive call")
				// Recursive call with NextToken
				return amazonTool.getFinancialEventsRecursiveHelper(ctx, queryParams, nextToken, consolidatedEvents, financialGroupId, orderId)
			}
		}
	}

	// No more NextToken, return consolidated response
	logs.WithContext(ctx).Debug("No more NextToken found. Consolidated financial events completed")
	return consolidatedEvents, nil
}

func (amazonTool *AmazonTool) GetSsoUrl(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetSsoUrl Execute - Start")

	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", amazonTool.AuthName, "/getssourl")
	logs.WithContext(ctx).Info(fmt.Sprint("url: ", url))
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	qParams := make(map[string]string)
	if params["state"] != nil {
		qParams["state"] = params["state"].(string)
	}
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, qParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	toolResultOk := false
	toolResult, toolResultOk = res.(map[string]interface{})
	if !toolResultOk {
		err = errors.New("toolResult is not a map")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	logs.WithContext(ctx).Info(fmt.Sprint("toolResult: ", toolResult))
	return toolResult, map[string]interface{}{"query": qParams}, false, nil
}
func (amazonTool *AmazonTool) RenewToken(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	params["refresh_token"] = amazonTool.AmazonAccount.RefreshToken
	return amazonTool.Login(ctx, projectId, tenantId, params, "/renew")
}

func (amazonTool *AmazonTool) Login(ctx context.Context, projectId string, tenantId string, params map[string]interface{}, renewStr string) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Login Execute - Start")
	if amazonTool.AuthName == "" {
		err = errors.New("auth name is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", amazonTool.AuthName, "/idptoken", renewStr)
	logs.WithContext(ctx).Info(fmt.Sprint("url: ", url))
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, params)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var amazonTokens AmazonTokens
	resBytes, _ := json.Marshal(res)
	err = json.Unmarshal(resBytes, &amazonTokens)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = json.Unmarshal(resBytes, &amazonTokens)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = amazonTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(amazonTool.ToolName, "_access_token"), amazonTokens.AccessToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = amazonTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(amazonTool.ToolName, "_refresh_token"), amazonTokens.RefreshToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	amazonTool.AmazonAccount.TokenExpirationDateTime = time.Now().UTC().Add(time.Duration(amazonTokens.ExpiresIn) * time.Second).Format(time.RFC3339)
	persistStore = true

	if amazonTool.Tool.Hooks.ARRT != "" {
		hookBody := map[string]interface{}{
			"Vars": map[string]interface{}{
				"Body": map[string]interface{}{
					"tool_name": amazonTool.ToolName,
					"tenant_id": tenantId,
				},
				"OrgBody": map[string]interface{}{
					"tool_name": amazonTool.ToolName,
					"tenant_id": tenantId,
				},
			},
			"ReqVars": map[string]interface{}{},
			"ResVars": map[string]interface{}{},
		}
		hookBodyBytes, err := json.Marshal(hookBody)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, nil, persistStore, err
		}
		jobName := fmt.Sprint(amazonTool.ToolName, "_", amazonTool.Tool.Hooks.ARRT, "_", tenantId)
		err = amazonTool.Scheduler.Unschedule(ctx, "", jobName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			//continue even if unschedule fails
		}

		schedulerCommand := fmt.Sprint("CALL schedule_procedure('", amazonTool.Tool.Hooks.ARRT, "','", string(hookBodyBytes), "','", amazonTool.Scheduler.GetSchedulerName(), "')")

		cronStr := utils.GetCronStr(ctx, time.Now().UTC().Add(1*time.Hour))
		jobId, err := amazonTool.Scheduler.Schedule(ctx, jobName, schedulerCommand, cronStr)
		if err != nil {
			return nil, nil, persistStore, err
		}
		logs.WithContext(ctx).Info(fmt.Sprint("jobId: ", jobId))
	}
	toolResult = make(map[string]interface{})
	toolResult["login_status"] = "success"
	toolResult["access_token"] = amazonTokens.AccessToken
	toolResult["refresh_token"] = amazonTokens.RefreshToken
	toolResult["expires_in"] = amazonTokens.ExpiresIn
	toolResult["token_type"] = amazonTokens.TokenType
	return toolResult, map[string]interface{}{"body": params}, persistStore, nil
}

func (amazonTool *AmazonTool) SetPrivateAttributes(ctx context.Context, realTool tools.Tooling) (err error) {
	amazonTool.AmazonAccount.AccessToken = fmt.Sprint("$SECRET_", amazonTool.ToolName, "_access_token")
	amazonTool.AmazonAccount.RefreshToken = fmt.Sprint("$SECRET_", amazonTool.ToolName, "_refresh_token")
	return nil
}

func (amazonTool *AmazonTool) GetBytes(ctx context.Context) ([]byte, error) {

	amazonToolWithToken := amazonToolWithToken{
		Tool: amazonTool.Tool,
		AmazonAccount: amazonAccountWithToken{
			UserAgent:               amazonTool.AmazonAccount.UserAgent,
			AccessToken:             amazonTool.AmazonAccount.AccessToken,
			RefreshToken:            amazonTool.AmazonAccount.RefreshToken,
			TokenExpirationDateTime: amazonTool.AmazonAccount.TokenExpirationDateTime,
		},
		AuthName: amazonTool.AuthName,
	}

	toolJson, err := json.Marshal(amazonToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}
func (amazonTool *AmazonTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	amazonToolWithToken := amazonToolWithToken{}
	err := json.Unmarshal(toolObjJson, &amazonToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	amazonTool = &AmazonTool{
		Tool: amazonToolWithToken.Tool,
		AmazonAccount: AmazonAccount{
			UserAgent:               amazonToolWithToken.AmazonAccount.UserAgent,
			AccessToken:             amazonToolWithToken.AmazonAccount.AccessToken,
			RefreshToken:            amazonToolWithToken.AmazonAccount.RefreshToken,
			TokenExpirationDateTime: amazonToolWithToken.AmazonAccount.TokenExpirationDateTime,
		},
		AuthName: amazonToolWithToken.AuthName,
	}
	return amazonTool, nil
}

func (amazonTool *AmazonTool) StopAutoRenew(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	if amazonTool.Scheduler == nil {
		err = errors.New("scheduler not defined")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	amazonTool.Scheduler.Unschedule(ctx, "", fmt.Sprint(amazonTool.ToolName, "_", amazonTool.Tool.Hooks.ARRT, "_", tenantId))
	toolResult = make(map[string]interface{})
	toolResult["stop_auto_renew_status"] = "success"
	amazonTool.AmazonAccount.TokenExpirationDateTime = ""
	persistStore = true
	return toolResult, map[string]interface{}{"body": params}, persistStore, nil
}

func init() {
	tools.RegisterTool("AMAZON", func() tools.Tooling { return new(AmazonTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       true,
		ToolType:     "AMAZON",
		Category:     "Ecommerce",
		Description:  "Amazon Selling Partner API for orders, financials, and account management",
		Actions:      []tools.ActionInfo{{Name: GetOrders}, {Name: GetOrderItems}, {Name: GetFinancialEvents}, {Name: GetFinancialEventGroups}, {Name: Login}, {Name: RenewToken}, {Name: GetSsoUrl}, {Name: StopAutoRenew}},
		OAuthEnabled: true,
		Icon:         "PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxZW0iIGhlaWdodD0iMWVtIiB2aWV3Qm94PSIwIDAgMjQgMjQiPjxwYXRoIGZpbGw9IiNGRjk5MDAiIGQ9Ik0uMDQ1IDE4LjAycS4xMDctLjE3NC4zNDgtLjAyMnE1LjQ1NSAzLjE2NSAxMS44NyAzLjE2NnE0LjI3OC0uMDAxIDguNDQ3LTEuNTk1bC4zMTUtLjE0Yy4xMzgtLjA2LjIzNC0uMS4yOTMtLjEzYy4yMjYtLjA4OC4zOS0uMDQ2LjUyNS4xM2MuMTIuMTc0LjA5LjMzNi0uMTIuNDhjLS4yNTYuMTktLjYuNDEtMS4wMDYuNjU0cS0xLjg2NyAxLjExMy00LjE4NSAxLjcyNmExNy42IDE3LjYgMCAwIDEtMTAuOTUxLS41NzdhMTcuOSAxNy45IDAgMCAxLTUuNDMtMy4zNXEtLjE1LS4xMTMtLjE1MS0uMjJjMC0uMDQ3LjAyMS0uMDkuMDUxLS4xM3ptNi41NjUtNi4yMThxMC0xLjUwNy43NDMtMi41NzdjLjQ5NS0uNzEgMS4xNy0xLjI1IDIuMDQtMS42MTVjLjc5Ni0uMzM1IDEuNzU2LS41NzUgMi45MTItLjcyYy4zOS0uMDQ2IDEuMDMzLS4xMDMgMS45Mi0uMTc0di0uMzdjMC0uOTMtLjEwNS0xLjU1OC0uMy0xLjg3NWMtLjMwMi0uNDMtLjc4LS42NS0xLjQ0LS42NWgtLjE4MmMtLjQ4LjA0Ni0uODk2LjE5Ni0xLjI0Ni40NmMtLjM1LjI3LS41NzUuNjMtLjY3NSAxLjA5NmMtLjA2LjMtLjIwNi40NjUtLjQzNS41MWwtMi41Mi0uMzE1Yy0uMjQ4LS4wNi0uMzcyLS4xOC0uMzcyLS4zOWMwLS4wNDYuMDA3LS4wOS4wMjItLjE1cS4zNzItMS45MzUgMS44Mi0yLjg4Yy45NzYtLjYxNiAyLjEtLjk3NSAzLjM5LTEuMDVoLjU0YzEuNjUgMCAyLjk1Ny40MzQgMy44ODggMS4yOWMuMTM1LjE1LjI3LjMuNDA1LjQ4Yy4xMi4xNjUuMjI0LjMxNC4yODMuNDVjLjA3NS4xMzQuMTUuMzMuMTk1LjU3Yy4wNi4yNTQuMTA1LjQyLjEzNS41MWMuMDMuMTA0LjA2Mi4zLjA3Ni42MTVjLjAxLjMxMy4wMi40OTMuMDIuNTUzdjUuMjhjMCAuMzc2LjA2LjcyLjE2NSAxLjAzNnEuMTU3LjQ3MS4zMTUuNjc0bC41MS42NzRxLjEzNi4yMDQuMTM2LjM2cTAgLjE4MS0uMTguMzE0Yy0xLjIgMS4wNS0xLjg2IDEuNjItMS45NjMgMS43MXEtLjI0Ny4yMDMtLjYzLjA0NWE2IDYgMCAwIDEtLjUyNi0uNDk2bC0uMzEtLjM0N2E5IDkgMCAwIDEtLjMxNy0uNDJsLS4zLS40MzVjLS44MS44ODYtMS42MDMgMS40NC0yLjQgMS42NjVjLS40OTQuMTUtMS4wOTMuMjI3LTEuODMuMjI3Yy0xLjExIDAtMi4wNC0uMzQzLTIuNzYtMS4wMzRjLS43Mi0uNjktMS4wOC0xLjY2NS0xLjA4LTIuOTRsLS4wNS0uMDc2em0zLjc1My0uNDM4cS0uMDAxLjg0OC40MjUgMS4zNjRjLjI4NS4zNC42NzUuNTEyIDEuMTU1LjUxMmMuMDQ1IDAgLjEwNi0uMDA3LjE5NS0uMDJjLjA5LS4wMTYuMTM0LS4wMjMuMTY2LS4wMjNjLjYxNC0uMTYgMS4wOC0uNTUzIDEuNDI0LTEuMTc4Yy4xNjUtLjI4LjI4NS0uNTguMzYtLjkxYy4wOS0uMzIuMTItLjU5LjEzNS0uOGMuMDE1LS4xOTUuMDE1LS41NC4wMTUtMS4wMDV2LS41NGMtLjg0IDAtMS40ODQuMDYtMS45Mi4xOGMtMS4yNzUuMzYtMS45MiAxLjE3LTEuOTIgMi40M2wtLjAzNS0uMDJ6bTkuMTYyIDcuMDI3Yy4wMy0uMDYuMDc1LS4xMS4xMzItLjE3cS41NDQtLjM2NSAxLjA1LS41YTggOCAwIDAgMSAxLjYxMi0uMjRjLjE0LS4wMTIuMjggMCAuNDEuMDNjLjY1LjA2IDEuMDUuMTY4IDEuMTcyLjMzYy4wNjMuMDkuMDk5LjIyOC4wOTkuMzl2LjE1YzAgLjUxLS4xNDkgMS4xMS0uNDI0IDEuOHEtLjQxOCAxLjAzNC0xLjE1NiAxLjY4cS0uMTEuMDktLjE5Ny4wOWMtLjAzIDAtLjA2IDAtLjA5LS4wMTJjLS4wOS0uMDQ0LS4xMDctLjEyLS4wNjQtLjI0Yy41NC0xLjI2LjgwNi0yLjE0My44MDYtMi42NGMwLS4xNS0uMDMtLjI3LS4wODctLjM0NGMtLjE0NS0uMTY2LS41NS0uMjU3LTEuMjI0LS4yNTdxLS4zNjQgMC0uODcuMDQ2Yy0uMzYzLjA0NS0uNy4wOS0xIC4xMzVxLS4xMzQgMC0uMTgtLjA0NGMtLjAzLS4wMy0uMDM2LS4wNDctLjAyLS4wNzdjMC0uMDE3LjAwNi0uMDMuMDItLjA2M3YtLjA2eiIvPjwvc3ZnPg==",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(AmazonTool{}), []string{}),
	})
}
