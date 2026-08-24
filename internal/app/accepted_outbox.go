package app

import (
	"sync"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const (
	firstLocalMessageID  int64 = 1 << 62
	maxAcceptedPerThread       = 50
	maxAcceptedThreads         = 100
)

type acceptedOutbox struct {
	mu      sync.Mutex
	nextID  int64
	items   map[int64][]domain.Message
	threads []int64
	now     func() time.Time
}

func newAcceptedOutbox() *acceptedOutbox {
	return &acceptedOutbox{
		nextID: firstLocalMessageID,
		items:  make(map[int64][]domain.Message),
		now:    time.Now,
	}
}

func (outbox *acceptedOutbox) add(threadID int64, body string) {
	if outbox == nil || threadID <= 0 {
		return
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	outbox.nextID++
	messages := append(outbox.items[threadID], domain.Message{
		ID:          outbox.nextID,
		ThreadID:    threadID,
		Body:        body,
		Timestamp:   outbox.now(),
		Direction:   domain.DirectionOutgoing,
		Type:        domain.MessageUnknown,
		SendOutcome: domain.SendOutcomeAcceptedUnverified,
	})
	if len(messages) > maxAcceptedPerThread {
		messages = messages[len(messages)-maxAcceptedPerThread:]
	}
	outbox.items[threadID] = messages
	outbox.touchThread(threadID)
}

func (outbox *acceptedOutbox) overlay(
	threadID int64,
	source []domain.Message,
	query domain.MessageQuery,
) []domain.Message {
	result := append([]domain.Message(nil), source...)
	if outbox == nil || threadID <= 0 || query.BeforeID > 0 {
		return result
	}

	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	pending := outbox.items[threadID]
	remaining := pending[:0]
	matchedEvidence := make([]bool, len(source))
	for _, local := range pending {
		if evidenceIndex := outgoingEvidenceIndex(source, local, matchedEvidence); evidenceIndex >= 0 {
			matchedEvidence[evidenceIndex] = true
			continue
		}
		remaining = append(remaining, local)
		result = append(result, local)
	}
	if len(remaining) == 0 {
		delete(outbox.items, threadID)
		outbox.removeThread(threadID)
	} else {
		outbox.items[threadID] = remaining
	}
	if query.Limit > 0 && len(result) > query.Limit {
		result = result[len(result)-query.Limit:]
	}
	return result
}

func (outbox *acceptedOutbox) touchThread(threadID int64) {
	outbox.removeThread(threadID)
	outbox.threads = append(outbox.threads, threadID)
	for len(outbox.threads) > maxAcceptedThreads {
		oldest := outbox.threads[0]
		outbox.threads = outbox.threads[1:]
		delete(outbox.items, oldest)
	}
}

func (outbox *acceptedOutbox) removeThread(threadID int64) {
	for index, existing := range outbox.threads {
		if existing != threadID {
			continue
		}
		outbox.threads = append(outbox.threads[:index], outbox.threads[index+1:]...)
		return
	}
}

func outgoingEvidenceIndex(source []domain.Message, local domain.Message, matched []bool) int {
	const evidenceWindow = 10 * time.Minute
	for index, message := range source {
		if matched[index] {
			continue
		}
		if message.Direction != domain.DirectionOutgoing || message.Body != local.Body {
			continue
		}
		if message.Timestamp.IsZero() || local.Timestamp.IsZero() {
			continue
		}
		if !message.Timestamp.Before(local.Timestamp.Add(-5*time.Second)) &&
			!message.Timestamp.After(local.Timestamp.Add(evidenceWindow)) {
			return index
		}
	}
	return -1
}
