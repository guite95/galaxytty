package samsung

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func verifySent(
	ctx context.Context,
	store domain.MessageStore,
	baseline int64,
	recipient, body string,
	timeout, interval time.Duration,
) (domain.SendResult, error) {
	normalizedRecipient := domain.NormalizePhone(recipient)
	if store == nil || baseline < 0 || normalizedRecipient == "" || strings.TrimSpace(body) == "" || timeout <= 0 || interval <= 0 {
		return domain.SendResult{}, fmt.Errorf("invalid send verification configuration")
	}

	verifyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		messages, err := store.MessagesAfter(verifyCtx, baseline)
		if err != nil {
			if ctx.Err() != nil {
				return domain.SendResult{}, ctx.Err()
			}
			if verifyCtx.Err() != nil {
				return domain.SendResult{}, domain.ErrSendVerificationTimeout
			}
			return domain.SendResult{}, fmt.Errorf("verify sent message: %w", err)
		}
		for _, message := range messages {
			if message.ID <= baseline || message.Direction != domain.DirectionOutgoing {
				continue
			}
			if domain.NormalizePhone(message.Address) != normalizedRecipient || message.Body != body {
				continue
			}
			return domain.SendResult{MessageID: message.ID, ThreadID: message.ThreadID}, nil
		}

		timer := time.NewTimer(interval)
		select {
		case <-verifyCtx.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return domain.SendResult{}, ctx.Err()
			}
			return domain.SendResult{}, domain.ErrSendVerificationTimeout
		case <-timer.C:
		}
	}
}
