package remote

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/protocol"
)

type fakeEventClient struct {
	events  chan protocol.Envelope
	gaps    chan SequenceGap
	request func(context.Context, protocol.Type, any) (protocol.Envelope, error)
}

func (client *fakeEventClient) Events() <-chan protocol.Envelope { return client.events }
func (client *fakeEventClient) SequenceGaps() <-chan SequenceGap { return client.gaps }
func (client *fakeEventClient) Request(ctx context.Context, messageType protocol.Type, payload any) (protocol.Envelope, error) {
	if client.request == nil {
		return protocol.Envelope{}, errors.New("unexpected recovery request")
	}
	return client.request(ctx, messageType, payload)
}

func TestEventSourceSkipsRedactedPoCEventsAndEmitsContentEvents(t *testing.T) {
	client := &fakeEventClient{events: make(chan protocol.Envelope, 2), gaps: make(chan SequenceGap, 1)}
	source := NewEventSource(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	messages, errorsChannel := source.SubscribeMessages(ctx)
	client.events <- envelope(t, protocol.TypeMessageReceived, map[string]any{
		"contentIncluded": false,
		"notificationKey": "redacted",
	})
	client.events <- envelope(t, protocol.TypeMessageReceived, map[string]any{
		"contentIncluded": true,
		"postedAt":        1_777_000_000_010,
		"message": map[string]any{
			"id": 7, "threadId": 3, "body": "실시간 😀", "postedAt": 1_777_000_000_000,
			"direction": "incoming", "messageType": "unknown",
		},
	})
	select {
	case message := <-messages:
		if message.ID != 7 || message.ThreadID != 3 || message.Type != domain.MessageUnknown ||
			message.ObservedAt.UnixMilli() != 1_777_000_000_010 {
			t.Fatalf("message=%+v", message)
		}
	case err := <-errorsChannel:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("missing message event")
	}
}

func TestEventSourceRecoversSequenceGap(t *testing.T) {
	client := &fakeEventClient{events: make(chan protocol.Envelope), gaps: make(chan SequenceGap, 1)}
	client.request = func(_ context.Context, messageType protocol.Type, payload any) (protocol.Envelope, error) {
		if messageType != protocol.TypeSyncRequest {
			t.Fatalf("type=%s", messageType)
		}
		request := payload.(syncRequestPayload)
		if request.FromSequence != 10 || request.ThroughSequence != 11 {
			t.Fatalf("request=%+v", request)
		}
		return envelope(t, protocol.TypeSyncMessage, map[string]any{
			"fromSequence": 10, "throughSequence": 11, "complete": true,
			"items": []any{map[string]any{
				"id": 41, "threadId": 7, "body": "recovered", "postedAt": 100,
				"direction": "incoming", "messageType": "unknown", "read": false,
			}},
		}), nil
	}
	source := NewEventSource(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	messages, errorsChannel := source.SubscribeMessages(ctx)
	client.gaps <- SequenceGap{Expected: 10, Received: 12}
	select {
	case message := <-messages:
		if message.ID != 41 || message.ThreadID != 7 {
			t.Fatalf("message=%+v", message)
		}
	case err := <-errorsChannel:
		t.Fatalf("unexpected recovery error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("missing recovered message")
	}
}

func TestEventSourceReportsIncompleteSequenceRecovery(t *testing.T) {
	client := &fakeEventClient{events: make(chan protocol.Envelope), gaps: make(chan SequenceGap, 1)}
	client.request = func(_ context.Context, _ protocol.Type, _ any) (protocol.Envelope, error) {
		return envelope(t, protocol.TypeSyncMessage, map[string]any{
			"fromSequence": 10, "throughSequence": 11, "complete": false, "items": []any{},
		}), nil
	}
	source := NewEventSource(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, errorsChannel := source.SubscribeMessages(ctx)
	client.gaps <- SequenceGap{Expected: 10, Received: 12}
	select {
	case err := <-errorsChannel:
		if !errors.Is(err, ErrSequenceGap) || !errors.Is(err, ErrSequenceRecoveryIncomplete) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing incomplete recovery error")
	}
}

func TestEventSourceReportsSequenceEpochResetWithoutRequest(t *testing.T) {
	requested := false
	client := &fakeEventClient{events: make(chan protocol.Envelope), gaps: make(chan SequenceGap, 1)}
	client.request = func(_ context.Context, _ protocol.Type, _ any) (protocol.Envelope, error) {
		requested = true
		return protocol.Envelope{}, nil
	}
	source := NewEventSource(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, errorsChannel := source.SubscribeMessages(ctx)
	client.gaps <- SequenceGap{Expected: 20, Received: 2, Reset: true}
	select {
	case err := <-errorsChannel:
		if !errors.Is(err, ErrSequenceGap) || !errors.Is(err, ErrSequenceEpochReset) {
			t.Fatalf("err=%v", err)
		}
		if requested {
			t.Fatal("epoch reset attempted bounded sequence replay")
		}
	case <-time.After(time.Second):
		t.Fatal("missing epoch reset error")
	}
}

func TestEventSourceClosesWhenClientChannelsClose(t *testing.T) {
	client := &fakeEventClient{events: make(chan protocol.Envelope), gaps: make(chan SequenceGap)}
	source := NewEventSource(client)
	messages, errorsChannel := source.SubscribeMessages(context.Background())
	close(client.events)
	close(client.gaps)
	select {
	case _, open := <-messages:
		if open {
			t.Fatal("messages channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("messages channel did not close")
	}
	if _, open := <-errorsChannel; open {
		t.Fatal("errors channel remained open")
	}
}
