package ecomm

func (amazonTool *AmazonTool) convertToSimplifiedEvents(financialEvents *FinancialEvents) []SimplifiedFinancialEvent {
	var simplified []SimplifiedFinancialEvent

	// Process Shipment Events
	for _, event := range financialEvents.ShipmentEventList {
		// Process order charges
		for _, charge := range event.OrderChargeList {
			if charge.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "shipment_order_charge",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          charge.ChargeAmount.CurrencyAmount,
					CurrencyCode:    charge.ChargeAmount.CurrencyCode,
					ChargeType:      charge.ChargeType,
				})
			}
		}
		// Process order charge adjustments
		for _, charge := range event.OrderChargeAdjustmentList {
			if charge.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "shipment_order_charge_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          charge.ChargeAmount.CurrencyAmount,
					CurrencyCode:    charge.ChargeAmount.CurrencyCode,
					ChargeType:      charge.ChargeType,
				})
			}
		}
		// Process shipment fees
		for _, fee := range event.ShipmentFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "shipment_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		// Process shipment fee adjustments
		for _, fee := range event.ShipmentFeeAdjustmentList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "shipment_fee_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		// Process order fees
		for _, fee := range event.OrderFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "order_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		// Process order fee adjustments
		for _, fee := range event.OrderFeeAdjustmentList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "order_fee_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		// Process direct payments
		for _, payment := range event.DirectPaymentList {
			if payment.DirectPaymentAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "direct_payment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          payment.DirectPaymentAmount.CurrencyAmount,
					CurrencyCode:    payment.DirectPaymentAmount.CurrencyCode,
					Description:     payment.DirectPaymentType,
				})
			}
		}
		// Process shipment items
		for _, item := range event.ShipmentItemList {
			// Process item charges
			for _, charge := range item.ItemChargeList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_charge",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process item charge adjustments
			for _, charge := range item.ItemChargeAdjustmentList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_charge_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process item fees
			for _, fee := range item.ItemFeeList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_fee",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process item fee adjustments
			for _, fee := range item.ItemFeeAdjustmentList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_fee_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process item tax withheld
			for _, taxWithheld := range item.ItemTaxWithheldList {
				for _, tax := range taxWithheld.TaxesWithheld {
					if tax.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "shipment_item_tax_withheld",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          tax.ChargeAmount.CurrencyAmount,
							CurrencyCode:    tax.ChargeAmount.CurrencyCode,
							Description:     tax.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}
			}
			// Process promotions
			for _, promotion := range item.PromotionList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "promotion",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process promotion adjustments
			for _, promotion := range item.PromotionAdjustmentList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "promotion_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process cost of points granted
			if item.CostOfPointsGranted.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "cost_of_points_granted",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsGranted.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsGranted.CurrencyCode,
					Description:     "Cost of Points Granted",
					Quantity:        item.QuantityShipped,
				})
			}
			// Process cost of points returned
			if item.CostOfPointsReturned.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "cost_of_points_returned",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsReturned.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsReturned.CurrencyCode,
					Description:     "Cost of Points Returned",
					Quantity:        item.QuantityShipped,
				})
			}
		}
		// Process shipment item adjustments
		for _, item := range event.ShipmentItemAdjustmentList {
			// Process item charges
			for _, charge := range item.ItemChargeList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_adjustment_charge",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process item charge adjustments
			for _, charge := range item.ItemChargeAdjustmentList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_adjustment_charge_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process item fees
			for _, fee := range item.ItemFeeList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_adjustment_fee",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process item fee adjustments
			for _, fee := range item.ItemFeeAdjustmentList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_adjustment_fee_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process item tax withheld
			for _, taxWithheld := range item.ItemTaxWithheldList {
				for _, tax := range taxWithheld.TaxesWithheld {
					if tax.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "shipment_item_adjustment_tax_withheld",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          tax.ChargeAmount.CurrencyAmount,
							CurrencyCode:    tax.ChargeAmount.CurrencyCode,
							Description:     tax.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}
			}
			// Process promotions
			for _, promotion := range item.PromotionList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_adjustment_promotion",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process promotion adjustments
			for _, promotion := range item.PromotionAdjustmentList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "shipment_item_adjustment_promotion_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			// Process cost of points granted
			if item.CostOfPointsGranted.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "shipment_item_adjustment_cost_of_points_granted",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsGranted.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsGranted.CurrencyCode,
					Description:     "Cost of Points Granted",
					Quantity:        item.QuantityShipped,
				})
			}
			// Process cost of points returned
			if item.CostOfPointsReturned.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "shipment_item_adjustment_cost_of_points_returned",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsReturned.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsReturned.CurrencyCode,
					Description:     "Cost of Points Returned",
					Quantity:        item.QuantityShipped,
				})
			}
		}
	}

	// Process Refund Events (same structure as shipment events)
	for _, event := range financialEvents.RefundEventList {
		// Order charges
		for _, charge := range event.OrderChargeList {
			if charge.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_order_charge",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          charge.ChargeAmount.CurrencyAmount,
					CurrencyCode:    charge.ChargeAmount.CurrencyCode,
					ChargeType:      charge.ChargeType,
				})
			}
		}
		for _, charge := range event.OrderChargeAdjustmentList {
			if charge.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_order_charge_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          charge.ChargeAmount.CurrencyAmount,
					CurrencyCode:    charge.ChargeAmount.CurrencyCode,
					ChargeType:      charge.ChargeType,
				})
			}
		}
		for _, fee := range event.ShipmentFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_shipment_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		for _, fee := range event.ShipmentFeeAdjustmentList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_shipment_fee_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		for _, fee := range event.OrderFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_order_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		for _, fee := range event.OrderFeeAdjustmentList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_order_fee_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		for _, payment := range event.DirectPaymentList {
			if payment.DirectPaymentAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_direct_payment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          payment.DirectPaymentAmount.CurrencyAmount,
					CurrencyCode:    payment.DirectPaymentAmount.CurrencyCode,
					Description:     payment.DirectPaymentType,
				})
			}
		}
		for _, item := range event.ShipmentItemList {
			for _, charge := range item.ItemChargeList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_charge",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, charge := range item.ItemChargeAdjustmentList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_charge_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, fee := range item.ItemFeeList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_fee",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, fee := range item.ItemFeeAdjustmentList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_fee_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, taxWithheld := range item.ItemTaxWithheldList {
				for _, tax := range taxWithheld.TaxesWithheld {
					if tax.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "refund_item_tax_withheld",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          tax.ChargeAmount.CurrencyAmount,
							CurrencyCode:    tax.ChargeAmount.CurrencyCode,
							Description:     tax.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}
			}
			for _, promotion := range item.PromotionList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_promotion",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, promotion := range item.PromotionAdjustmentList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_promotion_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			if item.CostOfPointsGranted.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_cost_of_points_granted",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsGranted.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsGranted.CurrencyCode,
					Description:     "Cost of Points Granted",
					Quantity:        item.QuantityShipped,
				})
			}
			if item.CostOfPointsReturned.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_cost_of_points_returned",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsReturned.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsReturned.CurrencyCode,
					Description:     "Cost of Points Returned",
					Quantity:        item.QuantityShipped,
				})
			}
		}
		for _, item := range event.ShipmentItemAdjustmentList {
			for _, charge := range item.ItemChargeList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_adjustment_charge",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, charge := range item.ItemChargeAdjustmentList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_adjustment_charge_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, fee := range item.ItemFeeList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_adjustment_fee",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, fee := range item.ItemFeeAdjustmentList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_adjustment_fee_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, taxWithheld := range item.ItemTaxWithheldList {
				for _, tax := range taxWithheld.TaxesWithheld {
					if tax.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "refund_item_adjustment_tax_withheld",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          tax.ChargeAmount.CurrencyAmount,
							CurrencyCode:    tax.ChargeAmount.CurrencyCode,
							Description:     tax.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}
			}
			for _, promotion := range item.PromotionList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_adjustment_promotion",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, promotion := range item.PromotionAdjustmentList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "refund_item_adjustment_promotion_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			if item.CostOfPointsGranted.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_item_adjustment_cost_of_points_granted",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsGranted.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsGranted.CurrencyCode,
					Description:     "Cost of Points Granted",
					Quantity:        item.QuantityShipped,
				})
			}
			if item.CostOfPointsReturned.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "refund_item_adjustment_cost_of_points_returned",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsReturned.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsReturned.CurrencyCode,
					Description:     "Cost of Points Returned",
					Quantity:        item.QuantityShipped,
				})
			}
		}
	}

	// Process Guarantee Claim Events (same structure as refund events)
	for _, event := range financialEvents.GuaranteeClaimEventList {
		prefix := "guarantee_claim_"
		// Order charges
		for _, charge := range event.OrderChargeList {
			if charge.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "order_charge",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          charge.ChargeAmount.CurrencyAmount,
					CurrencyCode:    charge.ChargeAmount.CurrencyCode,
					ChargeType:      charge.ChargeType,
				})
			}
		}
		for _, charge := range event.OrderChargeAdjustmentList {
			if charge.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "order_charge_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          charge.ChargeAmount.CurrencyAmount,
					CurrencyCode:    charge.ChargeAmount.CurrencyCode,
					ChargeType:      charge.ChargeType,
				})
			}
		}
		for _, fee := range event.ShipmentFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "shipment_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		for _, fee := range event.ShipmentFeeAdjustmentList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "shipment_fee_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		for _, fee := range event.OrderFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "order_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		for _, fee := range event.OrderFeeAdjustmentList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "order_fee_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}
		for _, payment := range event.DirectPaymentList {
			if payment.DirectPaymentAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "direct_payment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          payment.DirectPaymentAmount.CurrencyAmount,
					CurrencyCode:    payment.DirectPaymentAmount.CurrencyCode,
					Description:     payment.DirectPaymentType,
				})
			}
		}
		for _, item := range event.ShipmentItemList {
			for _, charge := range item.ItemChargeList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_charge",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, charge := range item.ItemChargeAdjustmentList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_charge_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, fee := range item.ItemFeeList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_fee",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, fee := range item.ItemFeeAdjustmentList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_fee_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, taxWithheld := range item.ItemTaxWithheldList {
				for _, tax := range taxWithheld.TaxesWithheld {
					if tax.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       prefix + "item_tax_withheld",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          tax.ChargeAmount.CurrencyAmount,
							CurrencyCode:    tax.ChargeAmount.CurrencyCode,
							Description:     tax.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}
			}
			for _, promotion := range item.PromotionList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "promotion",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, promotion := range item.PromotionAdjustmentList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "promotion_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			if item.CostOfPointsGranted.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "cost_of_points_granted",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsGranted.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsGranted.CurrencyCode,
					Description:     "Cost of Points Granted",
					Quantity:        item.QuantityShipped,
				})
			}
			if item.CostOfPointsReturned.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "cost_of_points_returned",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsReturned.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsReturned.CurrencyCode,
					Description:     "Cost of Points Returned",
					Quantity:        item.QuantityShipped,
				})
			}
		}
		for _, item := range event.ShipmentItemAdjustmentList {
			for _, charge := range item.ItemChargeList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_adjustment_charge",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, charge := range item.ItemChargeAdjustmentList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_adjustment_charge_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, fee := range item.ItemFeeList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_adjustment_fee",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, fee := range item.ItemFeeAdjustmentList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_adjustment_fee_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, taxWithheld := range item.ItemTaxWithheldList {
				for _, tax := range taxWithheld.TaxesWithheld {
					if tax.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       prefix + "item_adjustment_tax_withheld",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          tax.ChargeAmount.CurrencyAmount,
							CurrencyCode:    tax.ChargeAmount.CurrencyCode,
							Description:     tax.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}
			}
			for _, promotion := range item.PromotionList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_adjustment_promotion",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			for _, promotion := range item.PromotionAdjustmentList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       prefix + "item_adjustment_promotion_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}
			}
			if item.CostOfPointsGranted.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "item_adjustment_cost_of_points_granted",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsGranted.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsGranted.CurrencyCode,
					Description:     "Cost of Points Granted",
					Quantity:        item.QuantityShipped,
				})
			}
			if item.CostOfPointsReturned.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       prefix + "item_adjustment_cost_of_points_returned",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					SellerSKU:       item.SellerSKU,
					Amount:          item.CostOfPointsReturned.CurrencyAmount,
					CurrencyCode:    item.CostOfPointsReturned.CurrencyCode,
					Description:     "Cost of Points Returned",
					Quantity:        item.QuantityShipped,
				})
			}
		}
	}

	// Process Chargeback Events (same structure as RefundEventList)
	for _, event := range financialEvents.ChargebackEventList {
		// Process order charges
		for _, charge := range event.OrderChargeList {
			if charge.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "chargeback_order_charge",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          charge.ChargeAmount.CurrencyAmount,
					CurrencyCode:    charge.ChargeAmount.CurrencyCode,
					ChargeType:      charge.ChargeType,
				})
			}
		}

		// Process order charge adjustments
		for _, adjustment := range event.OrderChargeAdjustmentList {
			if adjustment.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "chargeback_order_charge_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          adjustment.ChargeAmount.CurrencyAmount,
					CurrencyCode:    adjustment.ChargeAmount.CurrencyCode,
					ChargeType:      adjustment.ChargeType,
				})
			}
		}

		// Process shipment fees
		for _, fee := range event.ShipmentFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "chargeback_shipment_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}

		// Process shipment fee adjustments
		for _, adjustment := range event.ShipmentFeeAdjustmentList {
			if adjustment.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "chargeback_shipment_fee_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          adjustment.FeeAmount.CurrencyAmount,
					CurrencyCode:    adjustment.FeeAmount.CurrencyCode,
					FeeType:         adjustment.FeeType,
				})
			}
		}

		// Process order fees
		for _, fee := range event.OrderFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "chargeback_order_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
				})
			}
		}

		// Process order fee adjustments
		for _, adjustment := range event.OrderFeeAdjustmentList {
			if adjustment.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "chargeback_order_fee_adjustment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          adjustment.FeeAmount.CurrencyAmount,
					CurrencyCode:    adjustment.FeeAmount.CurrencyCode,
					FeeType:         adjustment.FeeType,
				})
			}
		}

		// Process direct payments
		for _, payment := range event.DirectPaymentList {
			if payment.DirectPaymentAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "chargeback_direct_payment",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					SellerOrderId:   event.SellerOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          payment.DirectPaymentAmount.CurrencyAmount,
					CurrencyCode:    payment.DirectPaymentAmount.CurrencyCode,
					Description:     payment.DirectPaymentType,
				})
			}
		}

		// Process shipment items
		for _, item := range event.ShipmentItemList {
			// Process item charges
			for _, charge := range item.ItemChargeList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_charge",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          charge.ChargeAmount.CurrencyAmount,
						CurrencyCode:    charge.ChargeAmount.CurrencyCode,
						ChargeType:      charge.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}

			// Process item charge adjustments
			for _, adjustment := range item.ItemChargeAdjustmentList {
				if adjustment.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_charge_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          adjustment.ChargeAmount.CurrencyAmount,
						CurrencyCode:    adjustment.ChargeAmount.CurrencyCode,
						ChargeType:      adjustment.ChargeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}

			// Process item fees
			for _, fee := range item.ItemFeeList {
				if fee.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_fee",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          fee.FeeAmount.CurrencyAmount,
						CurrencyCode:    fee.FeeAmount.CurrencyCode,
						FeeType:         fee.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}

			// Process item fee adjustments
			for _, adjustment := range item.ItemFeeAdjustmentList {
				if adjustment.FeeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_fee_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          adjustment.FeeAmount.CurrencyAmount,
						CurrencyCode:    adjustment.FeeAmount.CurrencyCode,
						FeeType:         adjustment.FeeType,
						Quantity:        item.QuantityShipped,
					})
				}
			}

			// Process tax withheld
			for _, taxWithheld := range item.ItemTaxWithheldList {
				for _, tax := range taxWithheld.TaxesWithheld {
					if tax.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "chargeback_item_tax_withheld",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          tax.ChargeAmount.CurrencyAmount,
							CurrencyCode:    tax.ChargeAmount.CurrencyCode,
							Description:     tax.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}
			}

			// Process promotions
			for _, promotion := range item.PromotionList {
				if promotion.PromotionAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_promotion",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          promotion.PromotionAmount.CurrencyAmount,
						CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
						Description:     promotion.PromotionType,
						ReferenceId:     promotion.PromotionId,
						Quantity:        item.QuantityShipped,
					})
				}

				// Process promotion adjustments
				for _, adjustment := range item.PromotionAdjustmentList {
					if adjustment.PromotionAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "chargeback_item_promotion_adjustment",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          adjustment.PromotionAmount.CurrencyAmount,
							CurrencyCode:    adjustment.PromotionAmount.CurrencyCode,
							Description:     adjustment.PromotionType,
							ReferenceId:     adjustment.PromotionId,
							Quantity:        item.QuantityShipped,
						})
					}
				}

				// Process cost of points granted
				if item.CostOfPointsGranted.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_cost_of_points_granted",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          item.CostOfPointsGranted.CurrencyAmount,
						CurrencyCode:    item.CostOfPointsGranted.CurrencyCode,
						Quantity:        item.QuantityShipped,
					})
				}

				// Process cost of points returned
				if item.CostOfPointsReturned.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_cost_of_points_returned",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          item.CostOfPointsReturned.CurrencyAmount,
						CurrencyCode:    item.CostOfPointsReturned.CurrencyCode,
						Quantity:        item.QuantityShipped,
					})
				}
			}

			// Process shipment item adjustments
			for _, item := range event.ShipmentItemAdjustmentList {
				// Process item charges
				for _, charge := range item.ItemChargeList {
					if charge.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "chargeback_item_charge_adjustment",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          charge.ChargeAmount.CurrencyAmount,
							CurrencyCode:    charge.ChargeAmount.CurrencyCode,
							ChargeType:      charge.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}

				// Process item charge adjustments
				for _, adjustment := range item.ItemChargeAdjustmentList {
					if adjustment.ChargeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "chargeback_item_charge_adjustment",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          adjustment.ChargeAmount.CurrencyAmount,
							CurrencyCode:    adjustment.ChargeAmount.CurrencyCode,
							ChargeType:      adjustment.ChargeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}

				// Process item fees
				for _, fee := range item.ItemFeeList {
					if fee.FeeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "chargeback_item_fee_adjustment",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          fee.FeeAmount.CurrencyAmount,
							CurrencyCode:    fee.FeeAmount.CurrencyCode,
							FeeType:         fee.FeeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}

				// Process item fee adjustments
				for _, adjustment := range item.ItemFeeAdjustmentList {
					if adjustment.FeeAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "chargeback_item_fee_adjustment",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          adjustment.FeeAmount.CurrencyAmount,
							CurrencyCode:    adjustment.FeeAmount.CurrencyCode,
							FeeType:         adjustment.FeeType,
							Quantity:        item.QuantityShipped,
						})
					}
				}

				// Process tax withheld
				for _, taxWithheld := range item.ItemTaxWithheldList {
					for _, tax := range taxWithheld.TaxesWithheld {
						if tax.ChargeAmount.CurrencyAmount != 0 {
							simplified = append(simplified, SimplifiedFinancialEvent{
								EventType:       "chargeback_item_tax_withheld_adjustment",
								PostedDate:      event.PostedDate,
								AmazonOrderId:   event.AmazonOrderId,
								SellerOrderId:   event.SellerOrderId,
								MarketplaceName: event.MarketplaceName,
								SellerSKU:       item.SellerSKU,
								Amount:          tax.ChargeAmount.CurrencyAmount,
								CurrencyCode:    tax.ChargeAmount.CurrencyCode,
								Description:     tax.ChargeType,
								Quantity:        item.QuantityShipped,
							})
						}
					}
				}

				// Process promotions
				for _, promotion := range item.PromotionList {
					if promotion.PromotionAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "chargeback_item_promotion_adjustment",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          promotion.PromotionAmount.CurrencyAmount,
							CurrencyCode:    promotion.PromotionAmount.CurrencyCode,
							Description:     promotion.PromotionType,
							ReferenceId:     promotion.PromotionId,
							Quantity:        item.QuantityShipped,
						})
					}
				}

				// Process promotion adjustments
				for _, adjustment := range item.PromotionAdjustmentList {
					if adjustment.PromotionAmount.CurrencyAmount != 0 {
						simplified = append(simplified, SimplifiedFinancialEvent{
							EventType:       "chargeback_item_promotion_adjustment",
							PostedDate:      event.PostedDate,
							AmazonOrderId:   event.AmazonOrderId,
							SellerOrderId:   event.SellerOrderId,
							MarketplaceName: event.MarketplaceName,
							SellerSKU:       item.SellerSKU,
							Amount:          adjustment.PromotionAmount.CurrencyAmount,
							CurrencyCode:    adjustment.PromotionAmount.CurrencyCode,
							Description:     adjustment.PromotionType,
							ReferenceId:     adjustment.PromotionId,
							Quantity:        item.QuantityShipped,
						})
					}
				}

				// Process cost of points granted
				if item.CostOfPointsGranted.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_cost_of_points_granted_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          item.CostOfPointsGranted.CurrencyAmount,
						CurrencyCode:    item.CostOfPointsGranted.CurrencyCode,
						Quantity:        item.QuantityShipped,
					})
				}

				// Process cost of points returned
				if item.CostOfPointsReturned.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "chargeback_item_cost_of_points_returned_adjustment",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						SellerOrderId:   event.SellerOrderId,
						MarketplaceName: event.MarketplaceName,
						SellerSKU:       item.SellerSKU,
						Amount:          item.CostOfPointsReturned.CurrencyAmount,
						CurrencyCode:    item.CostOfPointsReturned.CurrencyCode,
						Quantity:        item.QuantityShipped,
					})
				}
			}
		}
	}

	// Process Service Fee Events
	for _, event := range financialEvents.ServiceFeeEventList {
		for _, fee := range event.FeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:     "service_fee",
					AmazonOrderId: event.AmazonOrderId,
					SellerSKU:     event.SellerSKU,
					ASIN:          event.ASIN,
					Amount:        fee.FeeAmount.CurrencyAmount,
					CurrencyCode:  fee.FeeAmount.CurrencyCode,
					FeeType:       fee.FeeType,
					Description:   event.FeeDescription,
					ReasonCode:    event.FeeReason,
				})
			}
		}
	}

	// Process Product Ads Payment Events
	for _, event := range financialEvents.ProductAdsPaymentEventList {
		if event.TransactionValue.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "product_ads_payment",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.TransactionValue.CurrencyAmount,
				CurrencyCode:    event.TransactionValue.CurrencyCode,
				ReferenceId:     event.InvoiceId,
			})
		}
	}

	// Process Adjustment Events
	for _, event := range financialEvents.AdjustmentEventList {
		if event.AdjustmentAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "adjustment",
				PostedDate:   event.PostedDate,
				Amount:       event.AdjustmentAmount.CurrencyAmount,
				CurrencyCode: event.AdjustmentAmount.CurrencyCode,
				Description:  event.AdjustmentType,
			})
		}
		// Process adjustment items
		for _, item := range event.AdjustmentItemList {
			if item.TotalAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:    "adjustment_item",
					PostedDate:   event.PostedDate,
					SellerSKU:    item.SellerSKU,
					ASIN:         item.ASIN,
					Amount:       item.TotalAmount.CurrencyAmount,
					CurrencyCode: item.TotalAmount.CurrencyCode,
					Description:  item.ProductDescription,
					Quantity:     0, // Convert string to int if needed
				})
			}
		}
	}

	// Process Debt Recovery Events
	for _, event := range financialEvents.DebtRecoveryEventList {
		if event.RecoveryAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "debt_recovery",
				Amount:       event.RecoveryAmount.CurrencyAmount,
				CurrencyCode: event.RecoveryAmount.CurrencyCode,
				Description:  event.DebtRecoveryType,
			})
		}
	}

	// Process Loan Servicing Events
	for _, event := range financialEvents.LoanServicingEventList {
		if event.LoanAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "loan_servicing",
				Amount:       event.LoanAmount.CurrencyAmount,
				CurrencyCode: event.LoanAmount.CurrencyCode,
				Description:  event.SourceBusinessEventType,
			})
		}
	}

	// Process Tax Withholding Events
	for _, event := range financialEvents.TaxWithholdingEventList {
		if event.WithheldAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "tax_withholding",
				PostedDate:   event.PostedDate,
				Amount:       event.WithheldAmount.CurrencyAmount,
				CurrencyCode: event.WithheldAmount.CurrencyCode,
				Description:  "Tax Withholding",
			})
		}
	}

	// Process Charge Refund Events
	for _, event := range financialEvents.ChargeRefundEventList {
		if event.RefundAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "charge_refund",
				PostedDate:   event.PostedDate,
				Amount:       event.RefundAmount.CurrencyAmount,
				CurrencyCode: event.RefundAmount.CurrencyCode,
				ReasonCode:   event.ReasonCode,
				ReferenceId:  event.ReferenceId,
			})
		}
	}

	// Process Failed Adhoc Disbursement Events
	for _, event := range financialEvents.FailedAdhocDisbursementEventList {
		if event.PrincipalAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "failed_adhoc_disbursement_principal",
				Amount:       event.PrincipalAmount.CurrencyAmount,
				CurrencyCode: event.PrincipalAmount.CurrencyCode,
				Description:  event.FundsRequestType,
			})
		}
		if event.EscrowAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "failed_adhoc_disbursement_escrow",
				Amount:       event.EscrowAmount.CurrencyAmount,
				CurrencyCode: event.EscrowAmount.CurrencyCode,
				Description:  event.FundsRequestType,
			})
		}
	}

	// Process Value Added Service Charge Events
	for _, event := range financialEvents.ValueAddedServiceChargeEventList {
		if event.TransactionAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "value_added_service_charge",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.TransactionAmount.CurrencyAmount,
				CurrencyCode:    event.TransactionAmount.CurrencyCode,
				Description:     event.Description,
			})
		}
	}

	// Process Capacity Reservation Billing Events
	for _, event := range financialEvents.CapacityReservationBillingEventList {
		if event.TransactionAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "capacity_reservation_billing",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.TransactionAmount.CurrencyAmount,
				CurrencyCode:    event.TransactionAmount.CurrencyCode,
				Description:     event.Description,
			})
		}
	}

	// Process Create Inbound Shipment Plan Events
	for _, event := range financialEvents.CreateInboundShipmentPlanEventList {
		for _, fee := range event.FeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:    "create_inbound_shipment_plan_fee",
					PostedDate:   event.PostedDate,
					Amount:       fee.FeeAmount.CurrencyAmount,
					CurrencyCode: fee.FeeAmount.CurrencyCode,
					FeeType:      fee.FeeType,
					ReferenceId:  event.ShipmentPlanId,
				})
			}
		}
	}

	// Process Shipping Label Events
	for _, event := range financialEvents.ShippingLabelEventList {
		for _, fee := range event.FeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:    "shipping_label_fee",
					PostedDate:   event.PostedDate,
					Amount:       fee.FeeAmount.CurrencyAmount,
					CurrencyCode: fee.FeeAmount.CurrencyCode,
					FeeType:      fee.FeeType,
					ReferenceId:  event.ShipmentId,
				})
			}
		}
	}

	// Process Pay With Amazon Events
	for _, event := range financialEvents.PayWithAmazonEventList {
		if event.Charge.ChargeAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "pay_with_amazon_charge",
				PostedDate:      event.TransactionPostedDate,
				SellerOrderId:   event.SellerOrderId,
				TransactionType: event.BusinessObjectType,
				Amount:          event.Charge.ChargeAmount.CurrencyAmount,
				CurrencyCode:    event.Charge.ChargeAmount.CurrencyCode,
				ChargeType:      event.Charge.ChargeType,
				Description:     event.AmountDescription,
			})
		}
		for _, fee := range event.FeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "pay_with_amazon_fee",
					PostedDate:      event.TransactionPostedDate,
					SellerOrderId:   event.SellerOrderId,
					TransactionType: event.BusinessObjectType,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
					Description:     event.AmountDescription,
				})
			}
		}
	}

	// Process Service Provider Credit Events
	for _, event := range financialEvents.ServiceProviderCreditEventList {
		if event.TransactionAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "service_provider_credit",
				PostedDate:      event.TransactionCreationDate,
				SellerOrderId:   event.SellerOrderId,
				TransactionType: event.ProviderTransactionType,
				Amount:          event.TransactionAmount.CurrencyAmount,
				CurrencyCode:    event.TransactionAmount.CurrencyCode,
				Description:     "Service Provider Credit",
				ReferenceId:     event.ProviderId,
			})
		}
	}

	// Process Retrocharge Events
	for _, event := range financialEvents.RetrochargeEventList {
		if event.BaseTax.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "retrocharge_base_tax",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				MarketplaceName: event.MarketplaceName,
				Amount:          event.BaseTax.CurrencyAmount,
				CurrencyCode:    event.BaseTax.CurrencyCode,
				Description:     event.RetrochargeEventType,
			})
		}
		if event.ShippingTax.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "retrocharge_shipping_tax",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				MarketplaceName: event.MarketplaceName,
				Amount:          event.ShippingTax.CurrencyAmount,
				CurrencyCode:    event.ShippingTax.CurrencyCode,
				Description:     event.RetrochargeEventType,
			})
		}
		for _, taxWithheld := range event.RetrochargeTaxWithheldList {
			for _, tax := range taxWithheld.TaxesWithheld {
				if tax.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "retrocharge_tax_withheld",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						MarketplaceName: event.MarketplaceName,
						Amount:          tax.ChargeAmount.CurrencyAmount,
						CurrencyCode:    tax.ChargeAmount.CurrencyCode,
						Description:     event.RetrochargeEventType,
					})
				}
			}
		}
	}

	// Process Rental Transaction Events
	for _, event := range financialEvents.RentalTransactionEventList {
		for _, charge := range event.RentalChargeList {
			if charge.ChargeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "rental_charge",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          charge.ChargeAmount.CurrencyAmount,
					CurrencyCode:    charge.ChargeAmount.CurrencyCode,
					ChargeType:      charge.ChargeType,
					Description:     event.RentalEventType,
				})
			}
		}
		for _, fee := range event.RentalFeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "rental_fee",
					PostedDate:      event.PostedDate,
					AmazonOrderId:   event.AmazonOrderId,
					MarketplaceName: event.MarketplaceName,
					Amount:          fee.FeeAmount.CurrencyAmount,
					CurrencyCode:    fee.FeeAmount.CurrencyCode,
					FeeType:         fee.FeeType,
					Description:     event.RentalEventType,
				})
			}
		}
		if event.RentalInitialValue.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "rental_initial_value",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				MarketplaceName: event.MarketplaceName,
				Amount:          event.RentalInitialValue.CurrencyAmount,
				CurrencyCode:    event.RentalInitialValue.CurrencyCode,
				Description:     event.RentalEventType,
			})
		}
		if event.RentalReimbursement.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "rental_reimbursement",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				MarketplaceName: event.MarketplaceName,
				Amount:          event.RentalReimbursement.CurrencyAmount,
				CurrencyCode:    event.RentalReimbursement.CurrencyCode,
				Description:     event.RentalEventType,
			})
		}
		for _, taxWithheld := range event.RentalTaxWithheldList {
			for _, tax := range taxWithheld.TaxesWithheld {
				if tax.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:       "rental_tax_withheld",
						PostedDate:      event.PostedDate,
						AmazonOrderId:   event.AmazonOrderId,
						MarketplaceName: event.MarketplaceName,
						Amount:          tax.ChargeAmount.CurrencyAmount,
						CurrencyCode:    tax.ChargeAmount.CurrencyCode,
						Description:     event.RentalEventType,
					})
				}
			}
		}
	}

	// Process Product Ads Payment Events
	for _, event := range financialEvents.ProductAdsPaymentEventList {
		if event.BaseValue.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "product_ads_base_value",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.BaseValue.CurrencyAmount,
				CurrencyCode:    event.BaseValue.CurrencyCode,
				ReferenceId:     event.InvoiceId,
			})
		}
		if event.TaxValue.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "product_ads_tax_value",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.TaxValue.CurrencyAmount,
				CurrencyCode:    event.TaxValue.CurrencyCode,
				ReferenceId:     event.InvoiceId,
			})
		}
		if event.TransactionValue.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "product_ads_transaction_value",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.TransactionValue.CurrencyAmount,
				CurrencyCode:    event.TransactionValue.CurrencyCode,
				ReferenceId:     event.InvoiceId,
			})
		}
	}

	// Process Seller Deal Payment Events
	for _, event := range financialEvents.SellerDealPaymentEventList {
		if event.FeeAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "seller_deal_fee",
				PostedDate:   event.PostedDate,
				Amount:       event.FeeAmount.CurrencyAmount,
				CurrencyCode: event.FeeAmount.CurrencyCode,
				FeeType:      event.FeeType,
				Description:  event.DealDescription,
				ReferenceId:  event.DealId,
			})
		}
		if event.TaxAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "seller_deal_tax",
				PostedDate:   event.PostedDate,
				Amount:       event.TaxAmount.CurrencyAmount,
				CurrencyCode: event.TaxAmount.CurrencyCode,
				Description:  event.DealDescription,
				ReferenceId:  event.DealId,
			})
		}
		if event.TotalAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "seller_deal_total",
				PostedDate:   event.PostedDate,
				Amount:       event.TotalAmount.CurrencyAmount,
				CurrencyCode: event.TotalAmount.CurrencyCode,
				Description:  event.DealDescription,
				ReferenceId:  event.DealId,
			})
		}
	}

	// Process SAFET Reimbursement Events
	for _, event := range financialEvents.SAFETReimbursementEventList {
		if event.ReimbursedAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "safet_reimbursement",
				PostedDate:   event.PostedDate,
				Amount:       event.ReimbursedAmount.CurrencyAmount,
				CurrencyCode: event.ReimbursedAmount.CurrencyCode,
				ReasonCode:   event.ReasonCode,
				ReferenceId:  event.SAFETClaimId,
			})
		}
		for _, item := range event.SAFETReimbursementItemList {
			for _, charge := range item.ItemChargeList {
				if charge.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:    "safet_reimbursement_item_charge",
						PostedDate:   event.PostedDate,
						Amount:       charge.ChargeAmount.CurrencyAmount,
						CurrencyCode: charge.ChargeAmount.CurrencyCode,
						ChargeType:   charge.ChargeType,
						Description:  item.ProductDescription,
						ReferenceId:  event.SAFETClaimId,
					})
				}
			}
		}
	}

	// Process Seller Review Enrollment Payment Events
	for _, event := range financialEvents.SellerReviewEnrollmentPaymentEventList {
		if event.FeeComponent.FeeAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "seller_review_enrollment_fee",
				PostedDate:   event.PostedDate,
				ASIN:         event.ParentASIN,
				Amount:       event.FeeComponent.FeeAmount.CurrencyAmount,
				CurrencyCode: event.FeeComponent.FeeAmount.CurrencyCode,
				FeeType:      event.FeeComponent.FeeType,
				ReferenceId:  event.EnrollmentId,
			})
		}
		if event.ChargeComponent.ChargeAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "seller_review_enrollment_charge",
				PostedDate:   event.PostedDate,
				ASIN:         event.ParentASIN,
				Amount:       event.ChargeComponent.ChargeAmount.CurrencyAmount,
				CurrencyCode: event.ChargeComponent.ChargeAmount.CurrencyCode,
				ChargeType:   event.ChargeComponent.ChargeType,
				ReferenceId:  event.EnrollmentId,
			})
		}
		if event.TotalAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "seller_review_enrollment_total",
				PostedDate:   event.PostedDate,
				ASIN:         event.ParentASIN,
				Amount:       event.TotalAmount.CurrencyAmount,
				CurrencyCode: event.TotalAmount.CurrencyCode,
				ReferenceId:  event.EnrollmentId,
			})
		}
	}

	// Process FBA Liquidation Events
	for _, event := range financialEvents.FBALiquidationEventList {
		if event.LiquidationProceedsAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "fba_liquidation_proceeds",
				PostedDate:   event.PostedDate,
				Amount:       event.LiquidationProceedsAmount.CurrencyAmount,
				CurrencyCode: event.LiquidationProceedsAmount.CurrencyCode,
				ReferenceId:  event.OriginalRemovalOrderId,
			})
		}
		if event.LiquidationFeeAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "fba_liquidation_fee",
				PostedDate:   event.PostedDate,
				Amount:       event.LiquidationFeeAmount.CurrencyAmount,
				CurrencyCode: event.LiquidationFeeAmount.CurrencyCode,
				ReferenceId:  event.OriginalRemovalOrderId,
			})
		}
	}

	// Process Coupon Payment Events
	for _, event := range financialEvents.CouponPaymentEventList {
		if event.FeeComponent.FeeAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "coupon_payment_fee",
				PostedDate:   event.PostedDate,
				Amount:       event.FeeComponent.FeeAmount.CurrencyAmount,
				CurrencyCode: event.FeeComponent.FeeAmount.CurrencyCode,
				FeeType:      event.FeeComponent.FeeType,
				Description:  event.SellerCouponDescription,
				ReferenceId:  event.CouponId,
			})
		}
		if event.ChargeComponent.ChargeAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "coupon_payment_charge",
				PostedDate:   event.PostedDate,
				Amount:       event.ChargeComponent.ChargeAmount.CurrencyAmount,
				CurrencyCode: event.ChargeComponent.ChargeAmount.CurrencyCode,
				ChargeType:   event.ChargeComponent.ChargeType,
				Description:  event.SellerCouponDescription,
				ReferenceId:  event.CouponId,
			})
		}
		if event.TotalAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "coupon_payment_total",
				PostedDate:   event.PostedDate,
				Amount:       event.TotalAmount.CurrencyAmount,
				CurrencyCode: event.TotalAmount.CurrencyCode,
				Description:  event.SellerCouponDescription,
				ReferenceId:  event.CouponId,
			})
		}
	}

	// Process Imaging Services Fee Events
	for _, event := range financialEvents.ImagingServicesFeeEventList {
		for _, fee := range event.FeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:    "imaging_services_fee",
					PostedDate:   event.PostedDate,
					ASIN:         event.ASIN,
					Amount:       fee.FeeAmount.CurrencyAmount,
					CurrencyCode: fee.FeeAmount.CurrencyCode,
					FeeType:      fee.FeeType,
					ReferenceId:  event.ImagingRequestBillingItemID,
				})
			}
		}
	}

	// Process Network Commingling Transaction Events
	for _, event := range financialEvents.NetworkComminglingTransactionEventList {
		if event.TaxExclusiveAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "network_commingling_tax_exclusive",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				ASIN:            event.ASIN,
				Amount:          event.TaxExclusiveAmount.CurrencyAmount,
				CurrencyCode:    event.TaxExclusiveAmount.CurrencyCode,
				ReferenceId:     event.NetCoTransactionID,
			})
		}
		if event.TaxAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "network_commingling_tax",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				ASIN:            event.ASIN,
				Amount:          event.TaxAmount.CurrencyAmount,
				CurrencyCode:    event.TaxAmount.CurrencyCode,
				ReferenceId:     event.NetCoTransactionID,
			})
		}
	}

	// Process Affordability Expense Events
	for _, event := range financialEvents.AffordabilityExpenseEventList {
		if event.BaseExpense.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_base_expense",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.BaseExpense.CurrencyAmount,
				CurrencyCode:    event.BaseExpense.CurrencyCode,
			})
		}
		if event.TaxTypeIGST.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_igst",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.TaxTypeIGST.CurrencyAmount,
				CurrencyCode:    event.TaxTypeIGST.CurrencyCode,
			})
		}
		if event.TaxTypeCGST.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_cgst",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.TaxTypeCGST.CurrencyAmount,
				CurrencyCode:    event.TaxTypeCGST.CurrencyCode,
			})
		}
		if event.TaxTypeSGST.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_sgst",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.TaxTypeSGST.CurrencyAmount,
				CurrencyCode:    event.TaxTypeSGST.CurrencyCode,
			})
		}
		if event.TotalExpense.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_total_expense",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.TotalExpense.CurrencyAmount,
				CurrencyCode:    event.TotalExpense.CurrencyCode,
			})
		}
	}

	// Process Affordability Expense Reversal Events (same structure as AffordabilityExpenseEventList)
	for _, event := range financialEvents.AffordabilityExpenseReversalEventList {
		if event.BaseExpense.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_reversal_base_expense",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.BaseExpense.CurrencyAmount,
				CurrencyCode:    event.BaseExpense.CurrencyCode,
			})
		}
		if event.TaxTypeIGST.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_reversal_igst",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.TaxTypeIGST.CurrencyAmount,
				CurrencyCode:    event.TaxTypeIGST.CurrencyCode,
			})
		}
		if event.TaxTypeCGST.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_reversal_cgst",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.TaxTypeCGST.CurrencyAmount,
				CurrencyCode:    event.TaxTypeCGST.CurrencyCode,
			})
		}
		if event.TaxTypeSGST.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_reversal_sgst",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.TaxTypeSGST.CurrencyAmount,
				CurrencyCode:    event.TaxTypeSGST.CurrencyCode,
			})
		}
		if event.TotalExpense.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "affordability_reversal_total_expense",
				PostedDate:      event.PostedDate,
				AmazonOrderId:   event.AmazonOrderId,
				TransactionType: event.TransactionType,
				Amount:          event.TotalExpense.CurrencyAmount,
				CurrencyCode:    event.TotalExpense.CurrencyCode,
			})
		}
	}

	// Process Removal Shipment Events
	for _, event := range financialEvents.RemovalShipmentEventList {
		for _, item := range event.RemovalShipmentItemList {
			if item.Revenue.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "removal_shipment_revenue",
					PostedDate:      event.PostedDate,
					TransactionType: event.TransactionType,
					Amount:          item.Revenue.CurrencyAmount,
					CurrencyCode:    item.Revenue.CurrencyCode,
					ReferenceId:     item.RemovalShipmentItemId,
				})
			}
			if item.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "removal_shipment_fee",
					PostedDate:      event.PostedDate,
					TransactionType: event.TransactionType,
					Amount:          item.FeeAmount.CurrencyAmount,
					CurrencyCode:    item.FeeAmount.CurrencyCode,
					ReferenceId:     item.RemovalShipmentItemId,
				})
			}
			if item.TaxAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "removal_shipment_tax",
					PostedDate:      event.PostedDate,
					TransactionType: event.TransactionType,
					Amount:          item.TaxAmount.CurrencyAmount,
					CurrencyCode:    item.TaxAmount.CurrencyCode,
					ReferenceId:     item.RemovalShipmentItemId,
				})
			}
			if item.TaxWithheld.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "removal_shipment_tax_withheld",
					PostedDate:      event.PostedDate,
					TransactionType: event.TransactionType,
					Amount:          item.TaxWithheld.CurrencyAmount,
					CurrencyCode:    item.TaxWithheld.CurrencyCode,
					ReferenceId:     item.RemovalShipmentItemId,
				})
			}
		}
	}

	// Process Removal Shipment Adjustment Events
	for _, event := range financialEvents.RemovalShipmentAdjustmentEventList {
		for _, item := range event.RemovalShipmentItemAdjustmentList {
			if item.Revenue.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "removal_shipment_adjustment_revenue",
					PostedDate:      event.PostedDate,
					TransactionType: event.TransactionType,
					Amount:          item.Revenue.CurrencyAmount,
					CurrencyCode:    item.Revenue.CurrencyCode,
					ReferenceId:     item.RemovalShipmentItemId,
				})
			}
			if item.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "removal_shipment_adjustment_fee",
					PostedDate:      event.PostedDate,
					TransactionType: event.TransactionType,
					Amount:          item.FeeAmount.CurrencyAmount,
					CurrencyCode:    item.FeeAmount.CurrencyCode,
					ReferenceId:     item.RemovalShipmentItemId,
				})
			}
			if item.TaxAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "removal_shipment_adjustment_tax",
					PostedDate:      event.PostedDate,
					TransactionType: event.TransactionType,
					Amount:          item.TaxAmount.CurrencyAmount,
					CurrencyCode:    item.TaxAmount.CurrencyCode,
					ReferenceId:     item.RemovalShipmentItemId,
				})
			}
			if item.TaxWithheld.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:       "removal_shipment_adjustment_tax_withheld",
					PostedDate:      event.PostedDate,
					TransactionType: event.TransactionType,
					Amount:          item.TaxWithheld.CurrencyAmount,
					CurrencyCode:    item.TaxWithheld.CurrencyCode,
					ReferenceId:     item.RemovalShipmentItemId,
				})
			}
		}
	}

	// Process Trial Shipment Events
	for _, event := range financialEvents.TrialShipmentEventList {
		for _, fee := range event.FeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:     "trial_shipment_fee",
					PostedDate:    event.PostedDate,
					AmazonOrderId: event.AmazonOrderId,
					SellerSKU:     event.SKU,
					Amount:        fee.FeeAmount.CurrencyAmount,
					CurrencyCode:  fee.FeeAmount.CurrencyCode,
					FeeType:       fee.FeeType,
				})
			}
		}
	}

	// Process TDS Reimbursement Events
	for _, event := range financialEvents.TDSReimbursementEventList {
		if event.ReimbursedAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "tds_reimbursement",
				PostedDate:   event.PostedDate,
				Amount:       event.ReimbursedAmount.CurrencyAmount,
				CurrencyCode: event.ReimbursedAmount.CurrencyCode,
				ReferenceId:  event.TdsOrderId,
			})
		}
	}

	// Process Adhoc Disbursement Events
	for _, event := range financialEvents.AdhocDisbursementEventList {
		if event.AdhocDisbursementAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "adhoc_disbursement",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.AdhocDisbursementAmount.CurrencyAmount,
				CurrencyCode:    event.AdhocDisbursementAmount.CurrencyCode,
				ReferenceId:     event.SourceOrderId,
			})
		}
	}

	// Process Tax Withholding Events
	for _, event := range financialEvents.TaxWithholdingEventList {
		if event.BaseAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "tax_withholding_base",
				PostedDate:   event.PostedDate,
				Amount:       event.BaseAmount.CurrencyAmount,
				CurrencyCode: event.BaseAmount.CurrencyCode,
				Description:  "Tax Withholding Base Amount",
			})
		}
		if event.WithheldAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "tax_withholding_withheld",
				PostedDate:   event.PostedDate,
				Amount:       event.WithheldAmount.CurrencyAmount,
				CurrencyCode: event.WithheldAmount.CurrencyCode,
				Description:  "Tax Withholding Withheld Amount",
			})
		}
		for _, taxWithheld := range event.TaxesWithheld {
			for _, tax := range taxWithheld.TaxesWithheld {
				if tax.ChargeAmount.CurrencyAmount != 0 {
					simplified = append(simplified, SimplifiedFinancialEvent{
						EventType:    "tax_withholding_detail",
						PostedDate:   event.PostedDate,
						Amount:       tax.ChargeAmount.CurrencyAmount,
						CurrencyCode: tax.ChargeAmount.CurrencyCode,
						Description:  "Tax Withholding Detail",
					})
				}
			}
		}
	}

	// Process Charge Refund Events
	for _, event := range financialEvents.ChargeRefundEventList {
		if event.RefundAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "charge_refund",
				PostedDate:   event.PostedDate,
				Amount:       event.RefundAmount.CurrencyAmount,
				CurrencyCode: event.RefundAmount.CurrencyCode,
				ReasonCode:   event.ReasonCode,
				ReferenceId:  event.ReferenceId,
			})
		}
	}

	// Process Failed Adhoc Disbursement Events
	for _, event := range financialEvents.FailedAdhocDisbursementEventList {
		if event.PrincipalAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "failed_adhoc_disbursement_principal",
				Amount:       event.PrincipalAmount.CurrencyAmount,
				CurrencyCode: event.PrincipalAmount.CurrencyCode,
				Description:  event.FundsRequestType,
			})
		}
		if event.EscrowAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:    "failed_adhoc_disbursement_escrow",
				Amount:       event.EscrowAmount.CurrencyAmount,
				CurrencyCode: event.EscrowAmount.CurrencyCode,
				Description:  event.FundsRequestType,
			})
		}
	}

	// Process Value Added Service Charge Events
	for _, event := range financialEvents.ValueAddedServiceChargeEventList {
		if event.TransactionAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "value_added_service_charge",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.TransactionAmount.CurrencyAmount,
				CurrencyCode:    event.TransactionAmount.CurrencyCode,
				Description:     event.Description,
			})
		}
	}

	// Process Capacity Reservation Billing Events
	for _, event := range financialEvents.CapacityReservationBillingEventList {
		if event.TransactionAmount.CurrencyAmount != 0 {
			simplified = append(simplified, SimplifiedFinancialEvent{
				EventType:       "capacity_reservation_billing",
				PostedDate:      event.PostedDate,
				TransactionType: event.TransactionType,
				Amount:          event.TransactionAmount.CurrencyAmount,
				CurrencyCode:    event.TransactionAmount.CurrencyCode,
				Description:     event.Description,
			})
		}
	}

	// Process Create Inbound Shipment Plan Events
	for _, event := range financialEvents.CreateInboundShipmentPlanEventList {
		for _, fee := range event.FeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:    "create_inbound_shipment_plan_fee",
					PostedDate:   event.PostedDate,
					Amount:       fee.FeeAmount.CurrencyAmount,
					CurrencyCode: fee.FeeAmount.CurrencyCode,
					FeeType:      fee.FeeType,
					ReferenceId:  event.ShipmentPlanId,
				})
			}
		}
	}

	// Process Shipping Label Events
	for _, event := range financialEvents.ShippingLabelEventList {
		for _, fee := range event.FeeList {
			if fee.FeeAmount.CurrencyAmount != 0 {
				simplified = append(simplified, SimplifiedFinancialEvent{
					EventType:    "shipping_label_fee",
					PostedDate:   event.PostedDate,
					Amount:       fee.FeeAmount.CurrencyAmount,
					CurrencyCode: fee.FeeAmount.CurrencyCode,
					FeeType:      fee.FeeType,
					ReferenceId:  event.ShipmentId,
				})
			}
		}
	}

	return simplified
}

func (amazonTool *AmazonTool) mergeFinancialEvents(consolidated *FinancialEvents, current *FinancialEvents) {
	// Merge all 35+ event arrays from current page into consolidated
	consolidated.ShipmentEventList = append(consolidated.ShipmentEventList, current.ShipmentEventList...)
	consolidated.RefundEventList = append(consolidated.RefundEventList, current.RefundEventList...)
	consolidated.GuaranteeClaimEventList = append(consolidated.GuaranteeClaimEventList, current.GuaranteeClaimEventList...)
	consolidated.ChargebackEventList = append(consolidated.ChargebackEventList, current.ChargebackEventList...)
	consolidated.PayWithAmazonEventList = append(consolidated.PayWithAmazonEventList, current.PayWithAmazonEventList...)
	consolidated.ServiceProviderCreditEventList = append(consolidated.ServiceProviderCreditEventList, current.ServiceProviderCreditEventList...)
	consolidated.RetrochargeEventList = append(consolidated.RetrochargeEventList, current.RetrochargeEventList...)
	consolidated.RentalTransactionEventList = append(consolidated.RentalTransactionEventList, current.RentalTransactionEventList...)
	consolidated.ProductAdsPaymentEventList = append(consolidated.ProductAdsPaymentEventList, current.ProductAdsPaymentEventList...)
	consolidated.ServiceFeeEventList = append(consolidated.ServiceFeeEventList, current.ServiceFeeEventList...)
	consolidated.SellerDealPaymentEventList = append(consolidated.SellerDealPaymentEventList, current.SellerDealPaymentEventList...)
	consolidated.DebtRecoveryEventList = append(consolidated.DebtRecoveryEventList, current.DebtRecoveryEventList...)
	consolidated.LoanServicingEventList = append(consolidated.LoanServicingEventList, current.LoanServicingEventList...)
	consolidated.AdjustmentEventList = append(consolidated.AdjustmentEventList, current.AdjustmentEventList...)
	consolidated.SAFETReimbursementEventList = append(consolidated.SAFETReimbursementEventList, current.SAFETReimbursementEventList...)
	consolidated.SellerReviewEnrollmentPaymentEventList = append(consolidated.SellerReviewEnrollmentPaymentEventList, current.SellerReviewEnrollmentPaymentEventList...)
	consolidated.FBALiquidationEventList = append(consolidated.FBALiquidationEventList, current.FBALiquidationEventList...)
	consolidated.CouponPaymentEventList = append(consolidated.CouponPaymentEventList, current.CouponPaymentEventList...)
	consolidated.ImagingServicesFeeEventList = append(consolidated.ImagingServicesFeeEventList, current.ImagingServicesFeeEventList...)
	consolidated.NetworkComminglingTransactionEventList = append(consolidated.NetworkComminglingTransactionEventList, current.NetworkComminglingTransactionEventList...)
	consolidated.AffordabilityExpenseEventList = append(consolidated.AffordabilityExpenseEventList, current.AffordabilityExpenseEventList...)
	consolidated.AffordabilityExpenseReversalEventList = append(consolidated.AffordabilityExpenseReversalEventList, current.AffordabilityExpenseReversalEventList...)
	consolidated.RemovalShipmentEventList = append(consolidated.RemovalShipmentEventList, current.RemovalShipmentEventList...)
	consolidated.RemovalShipmentAdjustmentEventList = append(consolidated.RemovalShipmentAdjustmentEventList, current.RemovalShipmentAdjustmentEventList...)
	consolidated.TrialShipmentEventList = append(consolidated.TrialShipmentEventList, current.TrialShipmentEventList...)
	consolidated.TDSReimbursementEventList = append(consolidated.TDSReimbursementEventList, current.TDSReimbursementEventList...)
	consolidated.AdhocDisbursementEventList = append(consolidated.AdhocDisbursementEventList, current.AdhocDisbursementEventList...)
	consolidated.TaxWithholdingEventList = append(consolidated.TaxWithholdingEventList, current.TaxWithholdingEventList...)
	consolidated.ChargeRefundEventList = append(consolidated.ChargeRefundEventList, current.ChargeRefundEventList...)
	consolidated.FailedAdhocDisbursementEventList = append(consolidated.FailedAdhocDisbursementEventList, current.FailedAdhocDisbursementEventList...)
	consolidated.ValueAddedServiceChargeEventList = append(consolidated.ValueAddedServiceChargeEventList, current.ValueAddedServiceChargeEventList...)
	consolidated.CapacityReservationBillingEventList = append(consolidated.CapacityReservationBillingEventList, current.CapacityReservationBillingEventList...)
	consolidated.PaymentEventList = append(consolidated.PaymentEventList, current.PaymentEventList...)
	consolidated.CreateInboundShipmentPlanEventList = append(consolidated.CreateInboundShipmentPlanEventList, current.CreateInboundShipmentPlanEventList...)
	consolidated.ShippingLabelEventList = append(consolidated.ShippingLabelEventList, current.ShippingLabelEventList...)
}
