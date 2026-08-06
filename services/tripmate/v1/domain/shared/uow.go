package shared

import "context"

type UnitOfWork interface {
	Do(context.Context, func(context.Context) error) error
}
