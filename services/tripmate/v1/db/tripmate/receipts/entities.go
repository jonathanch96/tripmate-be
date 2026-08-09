package receipts

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Receipt struct {
	ID            uuid.UUID `gorm:"column:id;primaryKey"`
	TripID        uuid.UUID
	ExpenseID     *uuid.UUID
	UploadedBy    uuid.UUID
	StorageKey    string
	ContentType   string
	SizeBytes     int64
	Merchant      *string
	ReceiptDate   *time.Time `gorm:"type:date"`
	Currency      *string
	Subtotal      *decimal.Decimal `gorm:"type:numeric(20,6)"`
	Tax           *decimal.Decimal `gorm:"type:numeric(20,6)"`
	ServiceCharge *decimal.Decimal `gorm:"type:numeric(20,6)"`
	Total         *decimal.Decimal `gorm:"type:numeric(20,6)"`
	Provider      *string
	ProviderModel *string
	RawResponse   *string `gorm:"type:jsonb"`
	Status        string
	FailureReason *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt

	UploaderEmail, UploaderName string  `gorm:"->"`
	UploaderAvatarURL           *string `gorm:"->"`
}

func (Receipt) TableName() string { return "tripmate.receipts" }

type Item struct {
	ID         uuid.UUID `gorm:"column:id;primaryKey"`
	ReceiptID  uuid.UUID
	Name       string
	Quantity   decimal.Decimal `gorm:"type:numeric(12,3)"`
	UnitPrice  decimal.Decimal `gorm:"type:numeric(20,6)"`
	TotalPrice decimal.Decimal `gorm:"type:numeric(20,6)"`
	SortOrder  int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (Item) TableName() string { return "tripmate.receipt_items" }

type Assignment struct {
	ID            uuid.UUID `gorm:"column:id;primaryKey"`
	ReceiptItemID uuid.UUID
	UserID        uuid.UUID
	CreatedAt     time.Time
}

func (Assignment) TableName() string { return "tripmate.receipt_item_assignments" }
