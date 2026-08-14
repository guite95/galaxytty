package app

import (
	"context"
	"errors"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
	"testing"
)

func serviceFixture(policy NotificationPolicy) (*Service, *mock.Backend, *mock.Notifier) {
	b := mock.New()
	n := &mock.Notifier{}
	l := NewLifecycle(&mock.Display{}, n)
	_ = l.Transition(Connecting)
	_ = l.Transition(Ready)
	return NewService(b, b, n, l, policy, domain.ApplicationStatus{Label: "Mock Connected"}), b, n
}
func TestSendToConversationResolvesParticipant(t *testing.T) {
	s, b, _ := serviceFixture(NotificationPolicy{})
	for _, tc := range []struct {
		id    int64
		phone string
	}{{1, "01012345678"}, {2, "01098765432"}} {
		result, e := s.SendToConversation(context.Background(), tc.id, "hello")
		if e != nil {
			t.Fatal(e)
		}
		if got := b.Sent[len(b.Sent)-1].Address; got != tc.phone {
			t.Fatalf("thread %d: %s", tc.id, got)
		}
		if result.MessageID != b.Sent[len(b.Sent)-1].ID || result.ThreadID != tc.id {
			t.Fatalf("thread %d result=%+v", tc.id, result)
		}
	}
}
func TestGroupSendUnsupported(t *testing.T) {
	s, b, _ := serviceFixture(NotificationPolicy{})
	b.ConversationsData[0].Participants = append(b.ConversationsData[0].Participants, domain.Contact{Phone: "01000000000"})
	before := len(b.Sent)
	if _, e := s.SendToConversation(context.Background(), 1, "hello"); !errors.Is(e, ErrGroupSendUnsupported) {
		t.Fatal(e)
	}
	if len(b.Sent) != before {
		t.Fatal("group rejection called sender")
	}
}
func TestPollingAndNotificationPolicy(t *testing.T) {
	s, b, n := serviceFixture(NotificationPolicy{Enabled: true})
	ctx := context.Background()
	if e := s.InitializePolling(ctx); e != nil {
		t.Fatal(e)
	}
	b.SimulateIncoming(2, "new")
	ms, e := s.Poll(ctx, 1)
	if e != nil || len(ms) != 1 || len(n.Items) != 1 {
		t.Fatal(ms, n.Items, e)
	}
	b.SimulateIncoming(1, "focused")
	_, _ = s.Poll(ctx, 1)
	if len(n.Items) != 1 {
		t.Fatal("focused notification emitted")
	}
	b.MessagesData = append(b.MessagesData, domain.Message{ID: 7, ThreadID: 2, Direction: domain.DirectionOutgoing})
	_, _ = s.Poll(ctx, 1)
	if len(n.Items) != 1 {
		t.Fatal("outgoing notification emitted")
	}
}
func TestNotificationsDisabledAndFocusedEnabled(t *testing.T) {
	s, b, n := serviceFixture(NotificationPolicy{})
	_ = s.InitializePolling(context.Background())
	b.SimulateIncoming(2, "new")
	_, _ = s.Poll(context.Background(), 1)
	if len(n.Items) != 0 {
		t.Fatal(n.Items)
	}
	s2, b2, n2 := serviceFixture(NotificationPolicy{Enabled: true, ShowWhenFocused: true})
	_ = s2.InitializePolling(context.Background())
	b2.SimulateIncoming(1, "focused")
	_, _ = s2.Poll(context.Background(), 1)
	if len(n2.Items) != 1 {
		t.Fatal(n2.Items)
	}
}

type fixedStatus struct{ value domain.ApplicationStatus }

func (f fixedStatus) Status(context.Context) domain.ApplicationStatus { return f.value }

func TestServiceUsesDynamicStatusProvider(t *testing.T) {
	service, _, _ := serviceFixture(NotificationPolicy{})
	service.WithStatusProvider(fixedStatus{value: domain.ApplicationStatus{
		State: "disconnected", Label: "Offline",
	}})
	got := service.Status(context.Background())
	if got.Label != "Offline" || got.State != "disconnected" {
		t.Fatalf("%+v", got)
	}
}
