package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/galaxytty/galaxytty/internal/domain"
)

var (
	ErrConversationNotFound = errors.New("conversation not found")
	ErrGroupSendUnsupported = errors.New("group conversation sending is not supported")
)

type API interface {
	Conversations(context.Context) ([]domain.Conversation, error)
	Unread(context.Context) ([]domain.Conversation, error)
	Messages(context.Context, int64, domain.MessageQuery) ([]domain.Message, error)
	SendToConversation(context.Context, int64, string) error
	SendToAddress(context.Context, string, string) error
	InitializePolling(context.Context) error
	Poll(context.Context, int64) ([]domain.Message, error)
	Status(context.Context) domain.ApplicationStatus
	Shutdown(context.Context) error
}

type NotificationPolicy struct{ Enabled, ShowWhenFocused bool }

type Service struct {
	store     domain.MessageStore
	sender    domain.MessageSender
	notifier  domain.Notifier
	lifecycle *Lifecycle
	poller    *Poller
	policy    NotificationPolicy
	status    domain.ApplicationStatus
	mu        sync.Mutex
}

func NewService(store domain.MessageStore, sender domain.MessageSender, notifier domain.Notifier, lifecycle *Lifecycle, policy NotificationPolicy, status domain.ApplicationStatus) *Service {
	return &Service{store: store, sender: sender, notifier: notifier, lifecycle: lifecycle, poller: &Poller{Store: store}, policy: policy, status: status}
}
func (s *Service) Conversations(ctx context.Context) ([]domain.Conversation, error) {
	return s.store.Conversations(ctx)
}
func (s *Service) Unread(ctx context.Context) ([]domain.Conversation, error) {
	cs, e := s.Conversations(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]domain.Conversation, 0)
	for _, c := range cs {
		if c.UnreadCount > 0 {
			out = append(out, c)
		}
	}
	return out, nil
}
func (s *Service) Messages(ctx context.Context, id int64, q domain.MessageQuery) ([]domain.Message, error) {
	return s.store.Messages(ctx, id, q)
}
func (s *Service) SendToAddress(ctx context.Context, phone, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("message text is required")
	}
	return s.sender.Send(ctx, phone, text)
}
func (s *Service) SendToConversation(ctx context.Context, id int64, text string) error {
	cs, e := s.Conversations(ctx)
	if e != nil {
		return e
	}
	for _, c := range cs {
		if c.ThreadID != id {
			continue
		}
		if len(c.Participants) != 1 {
			return ErrGroupSendUnsupported
		}
		return s.SendToAddress(ctx, c.Participants[0].Phone, text)
	}
	return ErrConversationNotFound
}
func (s *Service) InitializePolling(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.poller.Initialize(ctx)
}
func (s *Service) Poll(ctx context.Context, focused int64) ([]domain.Message, error) {
	s.mu.Lock()
	ms, e := s.poller.Poll(ctx)
	s.mu.Unlock()
	if e != nil {
		return nil, e
	}
	for _, m := range ms {
		if !s.shouldNotify(m, focused) || s.notifier == nil {
			continue
		}
		_ = s.notifier.NotifyMessage(ctx, domain.Notification{ThreadID: m.ThreadID, Title: m.Address, Body: m.Body})
	}
	return ms, nil
}
func (s *Service) shouldNotify(m domain.Message, focused int64) bool {
	return s.policy.Enabled && m.Direction == domain.DirectionIncoming && (s.policy.ShowWhenFocused || m.ThreadID != focused)
}
func (s *Service) Status(context.Context) domain.ApplicationStatus {
	state := Ready
	if s.lifecycle != nil {
		state = s.lifecycle.State()
	}
	status := s.status
	status.State = string(state)
	return status
}
func (s *Service) Shutdown(ctx context.Context) error {
	if s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.Shutdown(ctx)
}
func (s *Service) String() string {
	return fmt.Sprintf("GalaxyTTY service (%s)", s.Status(context.Background()).State)
}
