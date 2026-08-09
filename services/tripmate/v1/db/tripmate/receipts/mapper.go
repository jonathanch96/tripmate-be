package receipts

import (
	"encoding/json"

	domainreceipt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/receipt"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

func fromDomain(e domainreceipt.Receipt) Receipt {
	return Receipt{ID: e.ID, TripID: e.TripID, ExpenseID: e.ExpenseID, UploadedBy: e.UploadedBy,
		StorageKey: e.StorageKey, ContentType: e.ContentType, SizeBytes: e.SizeBytes,
		Merchant: e.Merchant, ReceiptDate: e.ReceiptDate, Currency: e.Currency,
		Subtotal: e.Subtotal, Tax: e.Tax, ServiceCharge: e.ServiceCharge, Total: e.Total,
		Provider: e.Provider, ProviderModel: e.ProviderModel, RawResponse: rawToColumn(e.RawResponse),
		Status: string(e.Status), FailureReason: e.FailureReason,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}

func toDomain(m Receipt) domainreceipt.Receipt {
	e := domainreceipt.Receipt{ID: m.ID, TripID: m.TripID, ExpenseID: m.ExpenseID, UploadedBy: m.UploadedBy,
		StorageKey: m.StorageKey, ContentType: m.ContentType, SizeBytes: m.SizeBytes,
		Merchant: m.Merchant, ReceiptDate: m.ReceiptDate, Currency: m.Currency,
		Subtotal: m.Subtotal, Tax: m.Tax, ServiceCharge: m.ServiceCharge, Total: m.Total,
		Provider: m.Provider, ProviderModel: m.ProviderModel, RawResponse: rawFromColumn(m.RawResponse),
		Status: domainreceipt.Status(m.Status), FailureReason: m.FailureReason,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
	if m.UploaderEmail != "" || m.UploaderName != "" {
		e.Uploader = &domainuser.PublicUser{ID: m.UploadedBy, Email: m.UploaderEmail, Name: m.UploaderName, AvatarURL: m.UploaderAvatarURL}
	}
	return e
}

func itemFromDomain(e domainreceipt.Item) Item {
	return Item{ID: e.ID, ReceiptID: e.ReceiptID, Name: e.Name, Quantity: e.Quantity,
		UnitPrice: e.UnitPrice, TotalPrice: e.TotalPrice, SortOrder: e.SortOrder,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}

func itemToDomain(m Item) domainreceipt.Item {
	return domainreceipt.Item{ID: m.ID, ReceiptID: m.ReceiptID, Name: m.Name, Quantity: m.Quantity,
		UnitPrice: m.UnitPrice, TotalPrice: m.TotalPrice, SortOrder: m.SortOrder,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// rawToColumn keeps the model's own words for debugging. Anything that is not valid JSON is stored
// as a JSON string rather than rejected, because a jsonb column will not take arbitrary bytes and
// losing the evidence is worse than quoting it.
func rawToColumn(raw json.RawMessage) *string {
	if len(raw) == 0 {
		return nil
	}
	if json.Valid(raw) {
		text := string(raw)
		return &text
	}
	quoted, err := json.Marshal(string(raw))
	if err != nil {
		return nil
	}
	text := string(quoted)
	return &text
}

func rawFromColumn(value *string) json.RawMessage {
	if value == nil {
		return nil
	}
	return json.RawMessage(*value)
}
