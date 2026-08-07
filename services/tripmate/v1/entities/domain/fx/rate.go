package fx

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Source string

const (
	SourceManual   Source = "manual"
	SourceSeed     Source = "seed"
	SourceProvider Source = "provider"
)

type Rate struct {
	ID                       uuid.UUID
	TripID                   *uuid.UUID
	FromCurrency, ToCurrency string
	Rate                     decimal.Decimal
	IsFinal                  bool
	Source                   Source
	EffectiveAt              time.Time
	CreatedAt, UpdatedAt     time.Time
}
