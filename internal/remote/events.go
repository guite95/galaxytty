package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/protocol"
)

var ErrSequenceGap = errors.New("Galaxy Helper event sequence gap")

type eventClient interface {
	Events() <-chan protocol.Envelope
	SequenceGaps() <-chan SequenceGap
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
		for events != nil || gaps != nil {
			select {
			case <-ctx.Done():
				return
			case gap, open := <-gaps:
				if !open {
					gaps = nil
					continue
				}
				select {
				case errorsChannel <- fmt.Errorf("%w: expected=%d received=%d", ErrSequenceGap, gap.Expected, gap.Received):
				case <-ctx.Done():
					return
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
				select {
				case messages <- message:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return messages, errorsChannel
}

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
