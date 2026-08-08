package fxresponse

import (
	"github.com/google/uuid"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
)

type Rate struct {
	ID     uuid.UUID  `json:"id"`
	TripID *uuid.UUID `json:"trip_id"`
	From   string     `json:"from"`
	To     string     `json:"to"`
	Rate   string     `json:"rate"`
	// IsFinal marks a rate locked at finalization; locked rates survive an unfinalize.
	IsFinal bool   `json:"is_final"`
	Source  string `json:"source"`
}

func FromDomain(entity domainfx.Rate) Rate {
	return Rate{ID: entity.ID, TripID: entity.TripID, From: entity.FromCurrency, To: entity.ToCurrency,
		Rate: entity.Rate.String(), IsFinal: entity.IsFinal, Source: string(entity.Source)}
}

func ListFromDomain(entities []domainfx.Rate) []Rate {
	rows := make([]Rate, len(entities))
	for i, entity := range entities {
		rows[i] = FromDomain(entity)
	}
	return rows
}
