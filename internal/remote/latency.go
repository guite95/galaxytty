package remote

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/galaxytty/galaxytty/internal/protocol"
)

var ErrMissingObservationTime = errors.New("Galaxy Helper event has no observation timestamp")

type latencyClient interface {
	Requester
	eventClient
}

type LatencyResult struct {
	NotificationToMac time.Duration
	ClockOffset       time.Duration
	CalibrationRTT    time.Duration
}

// MeasureNextMessage calibrates the Galaxy wall clock over the encrypted TCP
// session, then waits for one content-bearing message event. It never returns
// message content or identifiers to the caller.
func MeasureNextMessage(ctx context.Context, client latencyClient, samples int) (LatencyResult, error) {
	return measureNextMessage(ctx, client, samples, time.Now)
}

func measureNextMessage(
	ctx context.Context,
	client latencyClient,
	samples int,
	now func() time.Time,
) (LatencyResult, error) {
	if samples <= 0 {
		samples = 7
	}
	if samples > 20 {
		samples = 20
	}

	var bestRTT time.Duration
	var bestOffset time.Duration
	for sample := 0; sample < samples; sample++ {
		startedAt := now()
		response, err := client.Request(ctx, protocol.TypePing, map[string]any{"latencyProbe": true})
		finishedAt := now()
		if err != nil {
			return LatencyResult{}, fmt.Errorf("latency clock calibration: %w", err)
		}
		if err := expectType(response, protocol.TypePong); err != nil {
			return LatencyResult{}, err
		}
		var payload struct {
			ReceivedAt int64 `json:"receivedAt"`
		}
		if err := response.DecodePayload(&payload); err != nil {
			return LatencyResult{}, fmt.Errorf("decode latency PONG: %w", err)
		}
		if payload.ReceivedAt <= 0 {
			return LatencyResult{}, errors.New("Galaxy Helper PONG has no receive timestamp")
		}
		rtt := finishedAt.Sub(startedAt)
		midpoint := startedAt.Add(rtt / 2)
		offset := time.UnixMilli(payload.ReceivedAt).Sub(midpoint)
		if sample == 0 || rtt < bestRTT {
			bestRTT = rtt
			bestOffset = offset
		}
	}

	messages, failures := NewEventSource(client).SubscribeMessages(ctx)
	for messages != nil || failures != nil {
		select {
		case <-ctx.Done():
			return LatencyResult{}, ctx.Err()
		case err, open := <-failures:
			if !open {
				failures = nil
				continue
			}
			return LatencyResult{}, err
		case message, open := <-messages:
			if !open {
				messages = nil
				continue
			}
			if message.ObservedAt.IsZero() {
				return LatencyResult{}, ErrMissingObservationTime
			}
			receivedAt := now()
			macEquivalentObservedAt := message.ObservedAt.Add(-bestOffset)
			latency := receivedAt.Sub(macEquivalentObservedAt)
			if latency < 0 && -latency <= bestRTT/2 {
				latency = 0
			}
			return LatencyResult{
				NotificationToMac: latency,
				ClockOffset:       bestOffset,
				CalibrationRTT:    bestRTT,
			}, nil
		}
	}
	return LatencyResult{}, ErrDisconnected
}
