package app

import (
	"context"
	"errors"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
	"strings"
	"testing"
)

type conversationSender struct {
	threadID int64
	text     string
}

type acceptedConversationSender struct{}

type userActionConversationSender struct{}

func (acceptedConversationSender) Send(context.Context, string, string) (domain.SendResult, error) {
	return domain.SendResult{}, errors.New("address send must not be used")
}

func (acceptedConversationSender) SendToConversation(_ context.Context, threadID int64, _ string) (domain.SendResult, error) {
	return domain.SendResult{
		ThreadID: threadID,
		Outcome:  domain.SendOutcomeAcceptedUnverified,
		Evidence: "remote_input_pending_intent_accepted",
	}, nil
}

func (userActionConversationSender) Send(context.Context, string, string) (domain.SendResult, error) {
	return domain.SendResult{}, errors.New("address send must not be used")
}

func (userActionConversationSender) SendToConversation(_ context.Context, threadID int64, _ string) (domain.SendResult, error) {
	return domain.SendResult{
		ThreadID: threadID,
		Outcome:  domain.SendOutcomeUserActionRequired,
		Evidence: "samsung_compose_notification_posted",
	}, nil
}

func (*conversationSender) Send(context.Context, string, string) (domain.SendResult, error) {
	return domain.SendResult{}, errors.New("address send must not be used")
}

func (sender *conversationSender) SendToConversation(_ context.Context, threadID int64, text string) (domain.SendResult, error) {
	sender.threadID = threadID
	sender.text = text
	return domain.SendResult{ThreadID: threadID}, nil
}

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

func TestConversationAwareSenderDoesNotRequireMacPhoneParticipant(t *testing.T) {
	_, backend, notifier := serviceFixture(NotificationPolicy{})
	backend.ConversationsData[0].Participants = nil
	sender := &conversationSender{}
	service := NewService(
		backend,
		sender,
		notifier,
		nil,
		NotificationPolicy{},
		domain.ApplicationStatus{},
	)

	result, err := service.SendToConversation(context.Background(), 1, "reply through thread")
	if err != nil || result.ThreadID != 1 || sender.threadID != 1 || sender.text != "reply through thread" {
		t.Fatalf("result=%+v sender=%+v err=%v", result, sender, err)
	}
	if _, err := service.SendToConversation(context.Background(), 1, "   "); err == nil ||
		!strings.Contains(err.Error(), "text is required") {
		t.Fatalf("empty text err=%v", err)
	}
}

func TestAcceptedUnverifiedConversationReplyAppearsInSessionMessages(t *testing.T) {
	_, backend, notifier := serviceFixture(NotificationPolicy{})
	service := NewService(
		backend,
		acceptedConversationSender{},
		notifier,
		nil,
		NotificationPolicy{},
		domain.ApplicationStatus{},
	)

	result, err := service.SendToConversation(context.Background(), 1, "synthetic reply")
	if err != nil || result.Outcome != domain.SendOutcomeAcceptedUnverified {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	messages, err := service.Messages(context.Background(), 1, domain.MessageQuery{})
	if err != nil || len(messages) != 3 {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
	local := messages[len(messages)-1]
	if local.Body != "synthetic reply" || local.Direction != domain.DirectionOutgoing ||
		local.SendOutcome != domain.SendOutcomeAcceptedUnverified {
		t.Fatalf("local=%+v", local)
	}

	// The echo belongs to the application session, not the Android source store.
	source, err := backend.Messages(context.Background(), 1, domain.MessageQuery{})
	if err != nil || len(source) != 2 {
		t.Fatalf("source=%+v err=%v", source, err)
	}
}

func TestUserActionRequiredDoesNotCreateOutgoingSessionMessage(t *testing.T) {
	_, backend, notifier := serviceFixture(NotificationPolicy{})
	service := NewService(
		backend,
		userActionConversationSender{},
		notifier,
		nil,
		NotificationPolicy{},
		domain.ApplicationStatus{},
	)

	result, err := service.SendToConversation(context.Background(), 1, "synthetic draft")
	if err != nil || result.Outcome != domain.SendOutcomeUserActionRequired {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	messages, err := service.Messages(context.Background(), 1, domain.MessageQuery{})
	if err != nil || len(messages) != 2 {
		t.Fatalf("compose handoff must not create an outgoing message: messages=%+v err=%v", messages, err)
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

type fixedStatusEvents struct {
	fixedStatus
	updates chan domain.ApplicationStatus
}

func (events fixedStatusEvents) SubscribeStatus(context.Context) <-chan domain.ApplicationStatus {
	return events.updates
}

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

func TestServiceExposesStatusEventsFromDynamicProvider(t *testing.T) {
	service, _, _ := serviceFixture(NotificationPolicy{})
	if updates := service.SubscribeStatus(context.Background()); updates != nil {
		t.Fatal("legacy service unexpectedly exposed status events")
	}
	events := fixedStatusEvents{
		fixedStatus: fixedStatus{value: domain.ApplicationStatus{State: "connected"}},
		updates:     make(chan domain.ApplicationStatus, 1),
	}
	service.WithStatusProvider(events)
	if updates := service.SubscribeStatus(context.Background()); updates != events.updates {
		t.Fatal("dynamic status event source was not exposed")
	}
}

type fixedEvents struct {
	messages chan domain.Message
	errors   chan error
}

func (events fixedEvents) SubscribeMessages(context.Context) (<-chan domain.Message, <-chan error) {
	return events.messages, events.errors
}

func TestServiceExposesOptionalMessageEventSource(t *testing.T) {
	service, _, _ := serviceFixture(NotificationPolicy{})
	if messages, errorsChannel := service.SubscribeMessages(context.Background()); messages != nil || errorsChannel != nil {
		t.Fatal("legacy service unexpectedly exposed events")
	}
	events := fixedEvents{messages: make(chan domain.Message, 1), errors: make(chan error, 1)}
	service.WithMessageEvents(events)
	messages, errorsChannel := service.SubscribeMessages(context.Background())
	if messages == nil || errorsChannel == nil {
		t.Fatal("event source was not exposed")
	}
}
