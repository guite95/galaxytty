package readonly

import (
	"context"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type Sender struct{}

func (Sender) Send(context.Context, string, string) (domain.SendResult, error) {
	return domain.SendResult{}, domain.ErrSendingNotImplemented
}

type Notifier struct{}

func (Notifier) NotifyMessage(context.Context, domain.Notification) error { return nil }
func (Notifier) Close(context.Context) error                              { return nil }
