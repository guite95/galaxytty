//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestCollectRCSSendObservationReadsEvidenceAfterOneVerificationTimeout(t *testing.T) {
	sends := 0
	observations := 0
	wantEvidence := rcsProviderObservation{exactCorrelation: true, exactOutgoing: true, messageID: 42}

	got, err := collectRCSSendObservation(
		func() (domain.SendResult, error) {
			sends++
			return domain.SendResult{}, domain.ErrSendVerificationTimeout
		},
		func() (rcsProviderObservation, error) {
			observations++
			return wantEvidence, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if sends != 1 || observations != 1 {
		t.Fatalf("sends=%d observations=%d; want one of each", sends, observations)
	}
	if !errors.Is(got.sendErr, domain.ErrSendVerificationTimeout) || got.evidence.exactCorrelation != wantEvidence.exactCorrelation || got.evidence.exactOutgoing != wantEvidence.exactOutgoing || got.evidence.messageID != wantEvidence.messageID {
		t.Fatalf("observation=%+v", got)
	}
}

func TestCollectRCSSendObservationDoesNotMaskControllerFailure(t *testing.T) {
	sends := 0
	observations := 0
	wantErr := errors.New("synthetic controller failure")

	_, err := collectRCSSendObservation(
		func() (domain.SendResult, error) {
			sends++
			return domain.SendResult{}, wantErr
		},
		func() (rcsProviderObservation, error) {
			observations++
			return rcsProviderObservation{}, nil
		},
	)
	if !errors.Is(err, wantErr) || sends != 1 || observations != 0 {
		t.Fatalf("err=%v sends=%d observations=%d", err, sends, observations)
	}
}

func TestFindRCSProviderCorrelationRequiresNewExactRecipientAndBody(t *testing.T) {
	rows := []map[string]string{
		{"_id": "40", "address": "010-0000-0000", "type": "2", "body": "exact body"},
		{"_id": "41", "address": "010-1111-1111", "type": "2", "body": "wrong body"},
		{"_id": "42", "address": "010-2222-2222", "type": "1", "body": "exact body"},
		{"_id": "43", "address": "010-2222-2222", "type": "2", "body": "exact body"},
	}

	got, err := findRCSProviderCorrelation(rows, 40, "01022222222", "exact body")
	if err != nil {
		t.Fatal(err)
	}
	if !got.exactCorrelation || !got.exactOutgoing || got.messageID != 43 {
		t.Fatalf("correlation=%+v", got)
	}
}

func TestFindRCSProviderCorrelationRetainsUnknownTypeWithoutCallingItOutgoing(t *testing.T) {
	rows := []map[string]string{
		{"_id": "44", "address": "010-2222-2222", "type": "7", "body": "exact body"},
	}

	got, err := findRCSProviderCorrelation(rows, 40, "01022222222", "exact body")
	if err != nil {
		t.Fatal(err)
	}
	if !got.exactCorrelation || got.exactOutgoing || got.messageID != 44 || got.smsType != 7 {
		t.Fatalf("correlation=%+v", got)
	}
}

func TestObserveRCSProviderEvidencePropagatesBasicProviderFailure(t *testing.T) {
	wantErr := errors.New("synthetic provider failure")
	_, err := observeRCSProviderEvidence(context.Background(), func(context.Context, ...string) ([]byte, error) {
		return nil, wantErr
	}, 40, "01022222222", "exact body")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v", err)
	}
}

func TestObserveRCSProviderEvidencePropagatesDisconnectDuringExtensionQuery(t *testing.T) {
	wantErr := errors.New("synthetic device disconnect")
	calls := 0
	_, err := observeRCSProviderEvidence(context.Background(), func(context.Context, ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("Row: 0 _id=43, address=01022222222, type=2, body=exact body\n"), nil
		}
		return nil, wantErr
	}, 40, "01022222222", "exact body")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
