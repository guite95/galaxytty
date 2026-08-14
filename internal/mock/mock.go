package mock

import (
	"context"
	"fmt"
	"github.com/galaxytty/galaxytty/internal/domain"
	"sync"
	"time"
)

type Backend struct {
	mu                sync.Mutex
	ConversationsData []domain.Conversation
	MessagesData      []domain.Message
	Sent              []domain.Message
	nextID            int64
}

func New() *Backend {
	now := time.Now()
	b := &Backend{nextID: 5}
	b.ConversationsData = []domain.Conversation{
		{ThreadID: 1, Title: "김형주", Participants: []domain.Contact{{ID: 1, DisplayName: "김형주", Phone: "01012345678"}}, Snippet: "지금 어디야?", UpdatedAt: now, UnreadCount: 1},
		{ThreadID: 2, Title: "어머니", Participants: []domain.Contact{{ID: 2, DisplayName: "어머니", Phone: "01098765432"}}, Snippet: "저녁 먹고 와", UpdatedAt: now.Add(-time.Hour)},
		{ThreadID: 3, Title: "1588-xxxx", Participants: []domain.Contact{{ID: 3, DisplayName: "1588-xxxx", Phone: "15880000"}}, Snippet: "[Web발신] 결제가 완료되었습니다.", UpdatedAt: now.Add(-2 * time.Hour), UnreadCount: 1},
	}
	b.MessagesData = []domain.Message{{ID: 1, ThreadID: 1, Address: "01012345678", Body: "지금 어디야?", Timestamp: now.Add(-time.Minute), Direction: domain.DirectionIncoming, Type: domain.MessageSMS}, {ID: 2, ThreadID: 1, Address: "01012345678", Body: "지금 출발했어", Timestamp: now, Direction: domain.DirectionOutgoing, Read: true, Type: domain.MessageSMS}, {ID: 3, ThreadID: 2, Body: "저녁 먹고 와", Timestamp: now, Direction: domain.DirectionIncoming, Type: domain.MessageSMS}, {ID: 4, ThreadID: 3, Body: "", Timestamp: now, Direction: domain.DirectionIncoming, Type: domain.MessageMMS, Attachments: []domain.Attachment{{MIMEType: "image/jpeg"}}}}
	return b
}
func (b *Backend) Conversations(context.Context) ([]domain.Conversation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]domain.Conversation(nil), b.ConversationsData...), nil
}
func (b *Backend) Messages(_ context.Context, id int64, q domain.MessageQuery) ([]domain.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var r []domain.Message
	for _, m := range b.MessagesData {
		if m.ThreadID == id && (q.BeforeID == 0 || m.ID < q.BeforeID) {
			r = append(r, m)
		}
	}
	return r, nil
}
func (b *Backend) MessagesAfter(_ context.Context, id int64) ([]domain.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var r []domain.Message
	for _, m := range b.MessagesData {
		if m.ID > id {
			r = append(r, m)
		}
	}
	return r, nil
}
func (b *Backend) LatestMessageID(context.Context) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var id int64
	for _, m := range b.MessagesData {
		if m.ID > id {
			id = m.ID
		}
	}
	return id, nil
}
func (b *Backend) Send(_ context.Context, phone, text string) (domain.SendResult, error) {
	normalizedPhone := domain.NormalizePhone(phone)
	if normalizedPhone == "" || text == "" {
		return domain.SendResult{}, fmt.Errorf("phone and text are required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var threadID int64
	for _, conversation := range b.ConversationsData {
		for _, participant := range conversation.Participants {
			if domain.NormalizePhone(participant.Phone) == normalizedPhone {
				threadID = conversation.ThreadID
				break
			}
		}
		if threadID != 0 {
			break
		}
	}
	if threadID == 0 {
		return domain.SendResult{}, fmt.Errorf("mock conversation for phone not found")
	}
	m := domain.Message{ID: b.nextID, ThreadID: threadID, Address: phone, Body: text, Timestamp: time.Now(), Direction: domain.DirectionOutgoing, Read: true, Type: domain.MessageSMS}
	b.nextID++
	b.MessagesData = append(b.MessagesData, m)
	b.Sent = append(b.Sent, m)
	return domain.SendResult{MessageID: m.ID, ThreadID: m.ThreadID}, nil
}
func (b *Backend) SimulateIncoming(thread int64, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.MessagesData = append(b.MessagesData, domain.Message{ID: b.nextID, ThreadID: thread, Body: text, Timestamp: time.Now(), Direction: domain.DirectionIncoming, Type: domain.MessageSMS})
	for i := range b.ConversationsData {
		if b.ConversationsData[i].ThreadID == thread {
			b.ConversationsData[i].Snippet = text
			b.ConversationsData[i].UnreadCount++
			b.ConversationsData[i].UpdatedAt = time.Now()
		}
	}
	b.nextID++
}

type Display struct {
	Started bool
	Stops   int
}

func (d *Display) Start(context.Context) (domain.VirtualDisplay, error) {
	d.Started = true
	return domain.VirtualDisplay{AndroidDisplayID: 18, Width: 1080, Height: 1920}, nil
}
func (d *Display) Stop(context.Context) error   { d.Started = false; d.Stops++; return nil }
func (d *Display) Healthy(context.Context) bool { return d.Started }

type Notifier struct {
	Items  []domain.Notification
	Closed int
}

func (n *Notifier) NotifyMessage(_ context.Context, x domain.Notification) error {
	n.Items = append(n.Items, x)
	return nil
}
func (n *Notifier) Close(context.Context) error { n.Closed++; return nil }
