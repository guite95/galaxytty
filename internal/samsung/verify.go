package samsung

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type SendBaseline struct {
	SMSID        int64
	MMSID        int64
	MMSAvailable bool
}

func captureSendBaseline(ctx context.Context, store domain.MessageStore) (SendBaseline, error) {
	smsID, err := store.LatestMessageID(ctx)
	if err != nil {
		return SendBaseline{}, err
	}
	baseline := SendBaseline{SMSID: smsID}
	mmsStore, ok := store.(domain.MMSMessageStore)
	if !ok {
		return baseline, nil
	}
	mmsID, err := mmsStore.LatestMMSMessageID(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isDeviceUnavailable(err) {
			return SendBaseline{}, err
		}
		return baseline, nil
	}
	baseline.MMSID = mmsID
	baseline.MMSAvailable = true
	return baseline, nil
}

func verifySent(
	ctx context.Context,
	store domain.MessageStore,
	baseline int64,
	recipient, body string,
	timeout, interval time.Duration,
) (domain.SendResult, error) {
	return verifySentFromBaseline(ctx, store, SendBaseline{SMSID: baseline}, recipient, body, timeout, interval)
}

func verifySentFromBaseline(
	ctx context.Context,
	store domain.MessageStore,
	baseline SendBaseline,
	recipient, body string,
	timeout, interval time.Duration,
) (domain.SendResult, error) {
	normalizedRecipient := domain.NormalizePhone(recipient)
	if store == nil || baseline.SMSID < 0 || baseline.MMSID < 0 || normalizedRecipient == "" || strings.TrimSpace(body) == "" || timeout <= 0 || interval <= 0 {
		return domain.SendResult{}, fmt.Errorf("invalid send verification configuration")
	}
	mmsStore, hasMMSStore := store.(domain.MMSMessageStore)
	verifyMMS := baseline.MMSAvailable && hasMMSStore

	verifyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		messages, err := store.MessagesAfter(verifyCtx, baseline.SMSID)
		if err != nil {
			if ctx.Err() != nil {
				return domain.SendResult{}, ctx.Err()
			}
			if verifyCtx.Err() != nil {
				return domain.SendResult{}, verificationTimeoutError()
			}
			return domain.SendResult{}, fmt.Errorf("verify sent message: %w", err)
		}
		for _, message := range messages {
			if message.ID <= baseline.SMSID || message.Direction != domain.DirectionOutgoing {
				continue
			}
			if domain.NormalizePhone(message.Address) != normalizedRecipient || message.Body != body {
				continue
			}
			return domain.SendResult{MessageID: message.ID, ThreadID: message.ThreadID}, nil
		}
		if verifyMMS {
			mmsMessages, err := mmsStore.MMSMessagesAfter(verifyCtx, baseline.MMSID)
			if err != nil {
				if ctx.Err() != nil {
					return domain.SendResult{}, ctx.Err()
				}
				if verifyCtx.Err() != nil {
					return domain.SendResult{}, verificationTimeoutError()
				}
				return domain.SendResult{}, fmt.Errorf("verify sent MMS: %w", err)
			}
			for _, message := range mmsMessages {
				if message.ID <= baseline.MMSID || message.Direction != domain.DirectionOutgoing || message.Type != domain.MessageMMS {
					continue
				}
				if domain.NormalizePhone(message.Address) != normalizedRecipient || message.Body != body {
					continue
				}
				return domain.SendResult{MessageID: message.ID, ThreadID: message.ThreadID, Transport: domain.MessageMMS}, nil
			}
		}

		timer := time.NewTimer(interval)
		select {
		case <-verifyCtx.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return domain.SendResult{}, ctx.Err()
			}
			return domain.SendResult{}, verificationTimeoutError()
		case <-timer.C:
		}
	}
}

func verificationTimeoutError() error {
	return fmt.Errorf("%w: check Samsung Messages layout coordinates; the selected transport may not expose verifiable provider evidence", domain.ErrSendVerificationTimeout)
}

func isDeviceUnavailable(err error) bool {
	return errors.Is(err, domain.ErrOffline) || errors.Is(err, domain.ErrNoDevices) || errors.Is(err, domain.ErrUnauthorized)
}
