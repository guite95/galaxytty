package remote

import (
	"context"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/protocol"
)

type latencyTestClient struct {
	events    chan protocol.Envelope
	gaps      chan SequenceGap
	responses []protocol.Envelope
}

func (client *latencyTestClient) Events() <-chan protocol.Envelope { return client.events }
func (client *latencyTestClient) SequenceGaps() <-chan SequenceGap { return client.gaps }
func (client *latencyTestClient) Request(_ context.Context, messageType protocol.Type, _ any) (protocol.Envelope, error) {
	if messageType != protocol.TypePing {
		panic("unexpected request type " + messageType)
	}
	response := client.responses[0]
	client.responses = client.responses[1:]
	return response, nil
}

func TestMeasureNextMessageCalibratesClockWithoutReturningContent(t *testing.T) {
	base := time.UnixMilli(1_777_000_000_000)
	client := &latencyTestClient{
		events: make(chan protocol.Envelope, 1),
		gaps:   make(chan SequenceGap),
		responses: []protocol.Envelope{
			envelope(t, protocol.TypePong, map[string]any{"receivedAt": base.Add(55 * time.Millisecond).UnixMilli()}),
			envelope(t, protocol.TypePong, map[string]any{"receivedAt": base.Add(155 * time.Millisecond).UnixMilli()}),
		},
	}
	client.events <- envelope(t, protocol.TypeMessageReceived, map[string]any{
		"contentIncluded": true,
		"postedAt":        base.Add(250 * time.Millisecond).UnixMilli(),
		"message": map[string]any{
			"id": 7, "threadId": 3, "body": "must not escape", "postedAt": base.UnixMilli(),
			"direction": "incoming", "messageType": "unknown",
		},
	})

	times := []time.Time{
		base, base.Add(10 * time.Millisecond),
		base.Add(100 * time.Millisecond), base.Add(110 * time.Millisecond),
		base.Add(300 * time.Millisecond),
	}
	now := func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	result, err := measureNextMessage(context.Background(), client, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.CalibrationRTT != 10*time.Millisecond || result.ClockOffset != 50*time.Millisecond {
		t.Fatalf("calibration=%+v", result)
	}
	if result.NotificationToMac != 100*time.Millisecond {
		t.Fatalf("notification latency=%s", result.NotificationToMac)
	}
}
