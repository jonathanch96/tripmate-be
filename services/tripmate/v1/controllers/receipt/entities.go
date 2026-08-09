package receipt

import (
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/money"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
	domainreceipt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/receipt"
	userresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/user"
	"github.com/shopspring/decimal"
)

type itemResponse struct {
	ID          uuid.UUID   `json:"id"`
	ReceiptID   uuid.UUID   `json:"receipt_id"`
	Name        string      `json:"name"`
	Quantity    string      `json:"quantity"`
	UnitPrice   string      `json:"unit_price"`
	TotalPrice  string      `json:"total_price"`
	SortOrder   int         `json:"sort_order"`
	AssigneeIDs []uuid.UUID `json:"assignee_ids"`
}

type receiptResponse struct {
	ID            uuid.UUID          `json:"id"`
	TripID        uuid.UUID          `json:"trip_id"`
	ExpenseID     *uuid.UUID         `json:"expense_id"`
	UploadedBy    uuid.UUID          `json:"uploaded_by"`
	ContentType   string             `json:"content_type"`
	SizeBytes     int64              `json:"size_bytes"`
	Merchant      *string            `json:"merchant"`
	ReceiptDate   *string            `json:"receipt_date"`
	Currency      *string            `json:"currency"`
	Subtotal      *string            `json:"subtotal"`
	Tax           *string            `json:"tax"`
	ServiceCharge *string            `json:"service_charge"`
	Total         *string            `json:"total"`
	Provider      *string            `json:"provider"`
	ProviderModel *string            `json:"provider_model"`
	Status        string             `json:"status"`
	FailureReason *string            `json:"failure_reason"`
	ImageURL      string             `json:"image_url,omitempty"`
	Items         []itemResponse     `json:"items"`
	Uploader      *userresponse.User `json:"uploader"`
	Discrepancy   bool               `json:"discrepancy"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

type shareResponse struct {
	Currency        string              `json:"currency"`
	ItemsTotal      string              `json:"items_total"`
	Tax             string              `json:"tax"`
	ServiceCharge   string              `json:"service_charge"`
	Total           string              `json:"total"`
	PerUser         []userShareResponse `json:"per_user"`
	UnassignedItems []uuid.UUID         `json:"unassigned_items"`
	Discrepancy     bool                `json:"discrepancy"`
}

type userShareResponse struct {
	UserID        uuid.UUID `json:"user_id"`
	ItemsSubtotal string    `json:"items_subtotal"`
	Tax           string    `json:"tax"`
	ServiceCharge string    `json:"service_charge"`
	Total         string    `json:"total"`
}

func fromReceipt(entity domainreceipt.Receipt) receiptResponse {
	result := receiptResponse{ID: entity.ID, TripID: entity.TripID, ExpenseID: entity.ExpenseID,
		UploadedBy: entity.UploadedBy, ContentType: entity.ContentType, SizeBytes: entity.SizeBytes,
		Merchant: entity.Merchant, Currency: entity.Currency, Provider: entity.Provider,
		ProviderModel: entity.ProviderModel, Status: string(entity.Status), FailureReason: entity.FailureReason,
		ImageURL: entity.ImageURL, Items: make([]itemResponse, len(entity.Items)), CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.UpdatedAt, Subtotal: amountPointer(entity.Subtotal), Tax: amountPointer(entity.Tax),
		ServiceCharge: amountPointer(entity.ServiceCharge), Total: amountPointer(entity.Total)}
	if entity.ReceiptDate != nil {
		value := entity.ReceiptDate.Format("2006-01-02")
		result.ReceiptDate = &value
	}
	if entity.Uploader != nil {
		value := userresponse.User{ID: entity.Uploader.ID, Email: entity.Uploader.Email, Name: entity.Uploader.Name,
			AvatarURL: entity.Uploader.AvatarURL, CreatedAt: entity.Uploader.CreatedAt, UpdatedAt: entity.Uploader.UpdatedAt}
		result.Uploader = &value
	}
	for index, item := range entity.Items {
		result.Items[index] = fromItem(item)
	}
	if entity.Subtotal != nil {
		result.Discrepancy = !money.IsZero(entity.ItemsTotal().Sub(*entity.Subtotal))
	}
	return result
}

func fromItem(item domainreceipt.Item) itemResponse {
	return itemResponse{ID: item.ID, ReceiptID: item.ReceiptID, Name: item.Name,
		Quantity: item.Quantity.String(), UnitPrice: item.UnitPrice.String(), TotalPrice: item.TotalPrice.String(),
		SortOrder: item.SortOrder, AssigneeIDs: append([]uuid.UUID(nil), item.Assignees...)}
}

func fromShares(value *receiptdomain.ShareBreakdown) shareResponse {
	result := shareResponse{Currency: value.Currency, ItemsTotal: value.ItemsTotal.String(), Tax: value.Tax.String(),
		ServiceCharge: value.ServiceCharge.String(), Total: value.Total.String(),
		UnassignedItems: append([]uuid.UUID(nil), value.UnassignedItems...), Discrepancy: value.Discrepancy,
		PerUser: make([]userShareResponse, len(value.PerUser))}
	for index, row := range value.PerUser {
		result.PerUser[index] = userShareResponse{UserID: row.UserID, ItemsSubtotal: row.ItemsSubtotal.String(),
			Tax: row.Tax.String(), ServiceCharge: row.ServiceCharge.String(), Total: row.Total.String()}
	}
	return result
}

func amountPointer(value *decimal.Decimal) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

type itemRequest struct {
	Name       *string `json:"name"`
	Quantity   *string `json:"quantity"`
	UnitPrice  *string `json:"unit_price"`
	TotalPrice *string `json:"total_price"`
}

type assignmentRequest struct {
	ItemID  string   `json:"item_id"`
	UserIDs []string `json:"user_ids"`
}

type convertRequest struct {
	Description string `json:"description"`
	ExpenseDate string `json:"expense_date"`
	Payers      []struct {
		UserID string `json:"user_id"`
		Amount string `json:"amount"`
	} `json:"payers"`
}
