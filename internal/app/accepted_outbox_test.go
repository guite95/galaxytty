package app

import (
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestAcceptedOutboxOverlaysUnverifiedOutgoingAndRespectsPageBoundary(t *testing.T) {
	now := time.Date(2026, time.August, 24, 17, 0, 0, 0, time.Local)
	outbox := newAcceptedOutbox()
	outbox.now = func() time.Time { return now }
	outbox.add(7, "synthetic reply")

	source := []domain.Message{{ID: 10, ThreadID: 7, Body: "incoming", Direction: domain.DirectionIncoming}}
	messages := outbox.overlay(7, source, domain.MessageQuery{Limit: 10})
	if len(messages) != 2 {
		t.Fatalf("messages=%+v", messages)
	}
	local := messages[1]
	if local.ID <= source[0].ID || local.ThreadID != 7 || local.Body != "synthetic reply" ||
		local.Direction != domain.DirectionOutgoing || local.Timestamp != now ||
		local.SendOutcome != domain.SendOutcomeAcceptedUnverified {
		t.Fatalf("local=%+v", local)
	}
	older := outbox.overlay(7, source, domain.MessageQuery{BeforeID: 10, Limit: 10})
	if len(older) != 1 || older[0].ID != source[0].ID || older[0].Body != source[0].Body {
		t.Fatalf("older=%+v", older)
	}
}

func TestAcceptedOutboxRemovesLocalEchoWhenOutgoingEvidenceAppears(t *testing.T) {
	now := time.Date(2026, time.August, 24, 17, 0, 0, 0, time.Local)
	outbox := newAcceptedOutbox()
	outbox.now = func() time.Time { return now }
	outbox.add(7, "synthetic reply")
	evidence := domain.Message{
		ID: 11, ThreadID: 7, Body: "synthetic reply", Timestamp: now.Add(time.Second),
		Direction: domain.DirectionOutgoing, Type: domain.MessageSMS,
	}

	messages := outbox.overlay(7, []domain.Message{evidence}, domain.MessageQuery{})
	if len(messages) != 1 || messages[0].ID != evidence.ID || messages[0].Body != evidence.Body {
		t.Fatalf("messages=%+v", messages)
	}
	messages = outbox.overlay(7, nil, domain.MessageQuery{})
	if len(messages) != 0 {
		t.Fatalf("reconciled local echo returned: %+v", messages)
	}
}

func TestAcceptedOutboxDoesNotReconcileUnrelatedOutgoingMessage(t *testing.T) {
	now := time.Date(2026, time.August, 24, 17, 0, 0, 0, time.Local)
	outbox := newAcceptedOutbox()
	outbox.now = func() time.Time { return now }
	outbox.add(7, "synthetic reply")
	unrelated := domain.Message{
		ID: 11, ThreadID: 7, Body: "different text", Timestamp: now.Add(time.Second),
		Direction: domain.DirectionOutgoing,
	}

	messages := outbox.overlay(7, []domain.Message{unrelated}, domain.MessageQuery{})
	if len(messages) != 2 || messages[1].SendOutcome != domain.SendOutcomeAcceptedUnverified {
		t.Fatalf("messages=%+v", messages)
	}
}

func TestAcceptedOutboxConsumesOutgoingEvidenceOnce(t *testing.T) {
	now := time.Date(2026, time.August, 24, 17, 0, 0, 0, time.Local)
	outbox := newAcceptedOutbox()
	outbox.now = func() time.Time { return now }
	outbox.add(7, "repeated reply")
	outbox.add(7, "repeated reply")
	evidence := domain.Message{
		ID: 11, ThreadID: 7, Body: "repeated reply", Timestamp: now.Add(time.Second),
		Direction: domain.DirectionOutgoing,
	}

	messages := outbox.overlay(7, []domain.Message{evidence}, domain.MessageQuery{})
	if len(messages) != 2 || messages[0].ID != evidence.ID ||
		messages[1].SendOutcome != domain.SendOutcomeAcceptedUnverified {
		t.Fatalf("messages=%+v", messages)
	}
}

func TestAcceptedOutboxBoundsMessagesAndThreads(t *testing.T) {
	outbox := newAcceptedOutbox()
	for index := 0; index <= maxAcceptedPerThread; index++ {
		outbox.add(7, "bounded reply")
	}
	if messages := outbox.overlay(7, nil, domain.MessageQuery{}); len(messages) != maxAcceptedPerThread {
		t.Fatalf("per-thread messages=%d", len(messages))
	}

	for threadID := int64(1); threadID <= maxAcceptedThreads+1; threadID++ {
		outbox.add(1000+threadID, "thread reply")
	}
	if messages := outbox.overlay(1001, nil, domain.MessageQuery{}); len(messages) != 0 {
		t.Fatalf("oldest thread was not evicted: %+v", messages)
	}
	if messages := outbox.overlay(1000+maxAcceptedThreads+1, nil, domain.MessageQuery{}); len(messages) != 1 {
		t.Fatalf("newest thread messages=%+v", messages)
	}
}
