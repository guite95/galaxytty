package readonly

import (
	"context"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type Sender struct{}

func (Sender) Send(context.Context, string, string) error {
	return domain.ErrSendingNotImplemented
}
