package samsung

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type verificationStore struct {
	batches [][]domain.Message
	err     error
	block   bool
	calls   int
}

func (s *verificationStore) Conversations(context.Context) ([]domain.Conversation, error) {
	return nil, nil
}
func (s *verificationStore) Messages(context.Context, int64, domain.MessageQuery) ([]domain.Message, error) {
	return nil, nil
}
func (s *verificationStore) LatestMessageID(context.Context) (int64, error) { return 0, nil }
func (s *verificationStore) MessagesAfter(ctx context.Context, _ int64) ([]domain.Message, error) {
	s.calls++
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	if len(s.batches) == 0 {
		return nil, nil
	}
	batch := s.batches[0]
	s.batches = s.batches[1:]
	return batch, nil
}

func TestVerifySentMatchesOnlyExactOutgoingRowAfterBaseline(t *testing.T) {
	const body = "안녕하세요 😀"
	store := &verificationStore{batches: [][]domain.Message{
		{
			{ID: 99, ThreadID: 49, Address: "+82 10-1234-5678", Body: body, Direction: domain.DirectionOutgoing},
			{ID: 101, ThreadID: 49, Address: "+82 10-1234-5678", Body: body, Direction: domain.DirectionIncoming},
			{ID: 102, ThreadID: 49, Address: "01099999999", Body: body, Direction: domain.DirectionOutgoing},
			{ID: 103, ThreadID: 49, Address: "+82 10-1234-5678", Body: "wrong body", Direction: domain.DirectionOutgoing},
		},
		{{ID: 104, ThreadID: 49, Address: "+82 10-1234-5678", Body: body, Direction: domain.DirectionOutgoing}},
	}}

	result, err := verifySent(context.Background(), store, 100, "01012345678", body, 100*time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if result.MessageID != 104 || result.ThreadID != 49 || store.calls != 2 {
		t.Fatalf("result=%+v calls=%d", result, store.calls)
	}
}

func TestVerifySentTimesOutWithoutLeakingRecipientOrBody(t *testing.T) {
	const recipient = "01012345678"
	const body = "private test body"
	store := &verificationStore{batches: [][]domain.Message{{
		{ID: 101, ThreadID: 49, Address: recipient, Body: body, Direction: domain.DirectionIncoming},
		{ID: 102, ThreadID: 49, Address: recipient, Body: "unrelated", Direction: domain.DirectionOutgoing},
	}}}

	_, err := verifySent(context.Background(), store, 100, recipient, body, 15*time.Millisecond, time.Millisecond)
	if !errors.Is(err, domain.ErrSendVerificationTimeout) {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), recipient) || strings.Contains(err.Error(), body) {
		t.Fatalf("verification error leaked private values: %q", err)
	}
}

func TestVerifySentMapsQueryDeadlineToVerificationTimeout(t *testing.T) {
	_, err := verifySent(context.Background(), &verificationStore{block: true}, 100, "01012345678", "body", 5*time.Millisecond, time.Millisecond)
	if !errors.Is(err, domain.ErrSendVerificationTimeout) {
		t.Fatalf("err=%v", err)
	}
}

func TestVerifySentReturnsProviderErrorAndCancellation(t *testing.T) {
	providerErr := errors.New("synthetic provider failure")
	if _, err := verifySent(context.Background(), &verificationStore{err: providerErr}, 100, "01012345678", "body", time.Second, time.Millisecond); !errors.Is(err, providerErr) {
		t.Fatalf("provider err=%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := verifySent(ctx, &verificationStore{}, 100, "01012345678", "body", time.Second, time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
}

func TestVerifySentRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recipient string
		body      string
		timeout   time.Duration
		interval  time.Duration
	}{
		{"recipient", "", "body", time.Second, time.Millisecond},
		{"body", "01012345678", "", time.Second, time.Millisecond},
		{"timeout", "01012345678", "body", 0, time.Millisecond},
		{"interval", "01012345678", "body", time.Second, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifySent(context.Background(), &verificationStore{}, 100, tc.recipient, tc.body, tc.timeout, tc.interval); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
