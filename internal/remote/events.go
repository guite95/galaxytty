package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/protocol"
)

var (
	ErrSequenceGap                = errors.New("Galaxy Helper event sequence gap")
	ErrSequenceRecoveryIncomplete = errors.New("Galaxy Helper sequence recovery incomplete")
	ErrSequenceEpochReset         = errors.New("Galaxy Helper event sequence epoch reset")
)

type eventClient interface {
	Events() <-chan protocol.Envelope
	SequenceGaps() <-chan SequenceGap
	Request(context.Context, protocol.Type, any) (protocol.Envelope, error)
}

type EventSource struct{ client eventClient }

func NewEventSource(client eventClient) *EventSource { return &EventSource{client: client} }

func (source *EventSource) SubscribeMessages(ctx context.Context) (<-chan domain.Message, <-chan error) {
	messages := make(chan domain.Message, 32)
	errorsChannel := make(chan error, 8)
	go func() {
		defer close(messages)
		defer close(errorsChannel)
		events := source.client.Events()
		gaps := source.client.SequenceGaps()
		var lastMessageID int64
		seenIDs := make(map[int64]struct{})
		seenOrder := make([]int64, 0, maxRememberedEventMessages)
		emit := func(message domain.Message) bool {
			if _, found := seenIDs[message.ID]; found {
				return true
			}
			seenIDs[message.ID] = struct{}{}
			seenOrder = append(seenOrder, message.ID)
			if len(seenOrder) > maxRememberedEventMessages {
				delete(seenIDs, seenOrder[0])
				seenOrder = seenOrder[1:]
			}
			if message.ID > lastMessageID {
				lastMessageID = message.ID
			}
			select {
			case messages <- message:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for events != nil || gaps != nil {
			select {
			case <-ctx.Done():
				return
			case gap, open := <-gaps:
				if !open {
					gaps = nil
					continue
				}
				if gap.Reset {
					select {
					case errorsChannel <- fmt.Errorf(
						"%w: %w: previous next=%d current=%d",
						ErrSequenceGap,
						ErrSequenceEpochReset,
						gap.Expected,
						gap.Received,
					):
					case <-ctx.Done():
						return
					}
					continue
				}
				recovered, complete, err := source.recover(ctx, gap, lastMessageID)
				if err != nil {
					select {
					case errorsChannel <- fmt.Errorf(
						"%w: expected=%d received=%d: %v",
						ErrSequenceGap,
						gap.Expected,
						gap.Received,
						err,
					):
					case <-ctx.Done():
						return
					}
					continue
				}
				for _, message := range recovered {
					if !emit(message) {
						return
					}
				}
				if !complete {
					select {
					case errorsChannel <- fmt.Errorf(
						"%w: %w: expected=%d received=%d",
						ErrSequenceGap,
						ErrSequenceRecoveryIncomplete,
						gap.Expected,
						gap.Received,
					):
					case <-ctx.Done():
						return
					}
				}
			case envelope, open := <-events:
				if !open {
					events = nil
					continue
				}
				message, included, err := decodeMessageEvent(envelope)
				if err != nil {
					select {
					case errorsChannel <- err:
					case <-ctx.Done():
						return
					}
					continue
				}
				if !included {
					continue
				}
				if !emit(message) {
					return
				}
			}
		}
	}()
	return messages, errorsChannel
}

type syncRequestPayload struct {
	FromSequence    uint64 `json:"fromSequence"`
	ThroughSequence uint64 `json:"throughSequence"`
	AfterMessageID  int64  `json:"afterMessageId,omitempty"`
	Limit           int    `json:"limit"`
}

type syncResponsePayload struct {
	FromSequence    uint64       `json:"fromSequence"`
	ThroughSequence uint64       `json:"throughSequence"`
	Complete        bool         `json:"complete"`
	Items           []messageDTO `json:"items"`
}

func (source *EventSource) recover(ctx context.Context, gap SequenceGap, afterMessageID int64) ([]domain.Message, bool, error) {
	if gap.Expected == 0 || gap.Received <= gap.Expected {
		return nil, false, errors.New("invalid sequence gap")
	}
	recoveryContext, cancel := context.WithTimeout(ctx, sequenceRecoveryTimeout)
	defer cancel()
	request := syncRequestPayload{
		FromSequence:    gap.Expected,
		ThroughSequence: gap.Received - 1,
		AfterMessageID:  afterMessageID,
		Limit:           maxSequenceRecoveryMessages,
	}
	response, err := source.client.Request(recoveryContext, protocol.TypeSyncRequest, request)
	if err != nil {
		return nil, false, err
	}
	if err := expectType(response, protocol.TypeSyncMessage); err != nil {
		return nil, false, err
	}
	var payload syncResponsePayload
	if err := response.DecodePayload(&payload); err != nil {
		return nil, false, err
	}
	if payload.FromSequence != request.FromSequence || payload.ThroughSequence != request.ThroughSequence {
		return nil, false, fmt.Errorf(
			"unexpected recovery range %d-%d",
			payload.FromSequence,
			payload.ThroughSequence,
		)
	}
	messages := make([]domain.Message, 0, len(payload.Items))
	for _, item := range payload.Items {
		message, err := item.domain()
		if err != nil {
			return nil, false, err
		}
		messages = append(messages, message)
	}
	return messages, payload.Complete, nil
}

const (
	sequenceRecoveryTimeout     = 5 * time.Second
	maxSequenceRecoveryMessages = 500
	maxRememberedEventMessages  = 1024
)

func decodeMessageEvent(envelope protocol.Envelope) (domain.Message, bool, error) {
	if envelope.Type != protocol.TypeMessageReceived {
		return domain.Message{}, false, nil
	}
	var header struct {
		ContentIncluded bool  `json:"contentIncluded"`
		ObservedAt      int64 `json:"postedAt"`
	}
	if err := json.Unmarshal(envelope.Payload, &header); err != nil {
		return domain.Message{}, false, fmt.Errorf("decode message event header: %w", err)
	}
	if !header.ContentIncluded {
		return domain.Message{}, false, nil
	}
	var payload struct {
		ContentIncluded bool       `json:"contentIncluded"`
		Message         messageDTO `json:"message"`
	}
	if err := envelope.DecodePayload(&payload); err != nil {
		return domain.Message{}, false, err
	}
	message, err := payload.Message.domain()
	if err != nil {
		return domain.Message{}, false, err
	}
	message.ObservedAt = unixMillis(header.ObservedAt)
	return message, true, nil
}
