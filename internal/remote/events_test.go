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
	events chan protocol.Envelope
	gaps   chan SequenceGap
}

func (client *fakeEventClient) Events() <-chan protocol.Envelope { return client.events }
func (client *fakeEventClient) SequenceGaps() <-chan SequenceGap { return client.gaps }

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

func TestEventSourceReportsSequenceGapForRecovery(t *testing.T) {
	client := &fakeEventClient{events: make(chan protocol.Envelope), gaps: make(chan SequenceGap, 1)}
	source := NewEventSource(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, errorsChannel := source.SubscribeMessages(ctx)
	client.gaps <- SequenceGap{Expected: 10, Received: 12}
	select {
	case err := <-errorsChannel:
		if !errors.Is(err, ErrSequenceGap) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing gap error")
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
