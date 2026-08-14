package tui

import (
	"context"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
	"github.com/galaxytty/galaxytty/internal/readonly"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (Model, *mock.Backend) {
	t.Helper()
	b := mock.New()
	n := &mock.Notifier{}
	l := app.NewLifecycle(&mock.Display{}, n)
	_ = l.Transition(app.Connecting)
	_ = l.Transition(app.Ready)
	s := app.NewService(b, b, n, l, app.NotificationPolicy{}, domain.ApplicationStatus{Label: "Mock Connected"})
	_ = s.InitializePolling(context.Background())
	m := NewModel(context.Background(), s, time.Hour)
	updated, _ := m.Update(conversationsMsg{items: b.ConversationsData})
	return updated.(Model), b
}
func open(t *testing.T, m Model) Model {
	t.Helper()
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing load command")
	}
	msg := cmd()
	u, _ = u.(Model).Update(msg)
	return u.(Model)
}
func typeText(m Model, s string) Model {
	for _, r := range s {
		u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = u.(Model)
	}
	return m
}
func TestSelectResizeAndEsc(t *testing.T) {
	m, _ := fixture(t)
	m = open(t, m)
	if m.screen != chatScreen {
		t.Fatal(m.Debug())
	}
	u, _ := m.Update(tea.WindowSizeMsg{Width: 10, Height: 3})
	m = u.(Model)
	if m.width < 30 || m.height < 10 {
		t.Fatal(m.width, m.height)
	}
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if u.(Model).screen != conversationsScreen {
		t.Fatal("not back")
	}
}
func TestComposerSend(t *testing.T) {
	m, b := fixture(t)
	m = open(t, m)
	m = typeText(m, "hello")
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	u, _ = u.(Model).Update(msg)
	if len(b.Sent) != 1 || b.Sent[0].Address != "01012345678" {
		t.Fatal(b.Sent)
	}
}
func TestSlashCommands(t *testing.T) {
	for _, name := range []string{"/exit", "/quit"} {
		m, _ := fixture(t)
		m = open(t, m)
		m = typeText(m, name)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal(name)
		}
		if _, ok := cmd().(shutdownMsg); !ok {
			t.Fatalf("%s did not shutdown", name)
		}
	}
	m, _ := fixture(t)
	m = open(t, m)
	m = typeText(m, "/help")
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(u.(Model).errorText, "Enter send") {
		t.Fatal(u.(Model).errorText)
	}
	m = typeText(u.(Model), "/wat")
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(u.(Model).errorText, "unknown") {
		t.Fatal(u.(Model).errorText)
	}
}

func TestCtrlCShutsDown(t *testing.T) {
	m, _ := fixture(t)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("missing shutdown command")
	}
	updated, cmd = updated.(Model).Update(cmd())
	if cmd == nil {
		t.Fatal("missing quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("shutdown did not quit")
	}
}

func TestAsyncIncomingRefresh(t *testing.T) {
	m, b := fixture(t)
	m = open(t, m)
	b.SimulateIncoming(1, "async")
	u, cmd := m.Update(tickMsg(time.Now()))
	batch := cmd()
	_ = batch // Batch execution belongs to Bubble Tea; inject deterministic poll result below.
	u, cmd = m.Update(pollMsg{items: []domain.Message{{ID: 5, ThreadID: 1}}})
	if cmd == nil {
		t.Fatal("expected refresh")
	}
	_ = u
}

type mutableStatus struct{ value domain.ApplicationStatus }

func (s *mutableStatus) Status(context.Context) domain.ApplicationStatus { return s.value }

func readOnlyFixture(t *testing.T) (Model, *mutableStatus) {
	t.Helper()
	backend := mock.New()
	status := &mutableStatus{value: domain.ApplicationStatus{State: "connected", Connection: domain.ConnectionUSB, Label: "USB"}}
	lifecycle := app.NewLifecycle(nil, readonly.Notifier{})
	_ = lifecycle.Transition(app.Connecting)
	_ = lifecycle.Transition(app.Ready)
	service := app.NewService(backend, readonly.Sender{}, readonly.Notifier{}, lifecycle, app.NotificationPolicy{}, status.value).WithStatusProvider(status)
	if err := service.InitializePolling(context.Background()); err != nil {
		t.Fatal(err)
	}
	model := NewModel(context.Background(), service, time.Hour)
	updated, _ := model.Update(conversationsMsg{items: backend.ConversationsData})
	return updated.(Model), status
}

func TestProviderErrorRefreshesOfflineStatus(t *testing.T) {
	model, status := readOnlyFixture(t)
	status.value = domain.ApplicationStatus{State: "disconnected", Connection: domain.ConnectionUSB, Label: "Offline"}
	updated, _ := model.Update(messagesMsg{err: errors.New("synthetic provider error")})
	if got := updated.(Model).status; got != "Offline" {
		t.Fatalf("status=%q", got)
	}
}

func TestConversationRefreshKeepsSelectedThread(t *testing.T) {
	model, _ := fixture(t)
	model.cursor = 1
	selectedThreadID := model.selectedID()
	refreshed := []domain.Conversation{
		{ThreadID: selectedThreadID, Title: "Selected"},
		{ThreadID: 1, Title: "First"},
		{ThreadID: 3, Title: "Third"},
	}
	updated, _ := model.Update(conversationsMsg{items: refreshed})
	got := updated.(Model)
	if got.cursor != 0 || got.selectedID() != selectedThreadID {
		t.Fatalf("cursor=%d selected=%d", got.cursor, got.selectedID())
	}
}

func TestConversationListRendersOnlyVisibleWindow(t *testing.T) {
	model, _ := fixture(t)
	model.conversations = make([]domain.Conversation, 30)
	for index := range model.conversations {
		model.conversations[index] = domain.Conversation{
			ThreadID: int64(index + 1),
			Title:    fmt.Sprintf("Conversation %02d", index),
			Snippet:  "Synthetic snippet",
		}
	}
	model.cursor = 20
	rendered := model.renderConversations(24, 10)
	if !strings.Contains(rendered, "Conversation 20") {
		t.Fatal("selected conversation is outside rendered window")
	}
	if strings.Contains(rendered, "Conversation 00") {
		t.Fatal("off-screen conversation was rendered")
	}
	if lines := strings.Count(rendered, "\n") + 1; lines > 10 {
		t.Fatalf("rendered lines=%d", lines)
	}
}

func TestChatRendersLatestVisibleMessageWindow(t *testing.T) {
	model, _ := fixture(t)
	model.screen = chatScreen
	model.messages = make([]domain.Message, 30)
	for index := range model.messages {
		model.messages[index] = domain.Message{
			ID:        int64(index + 1),
			ThreadID:  1,
			Body:      fmt.Sprintf("Message %02d", index),
			Direction: domain.DirectionIncoming,
		}
	}
	model.messages[len(model.messages)-1].Body = "Recent\nmessage"
	rendered := model.renderChat(24, 10)
	if !strings.Contains(rendered, "Recent message") {
		t.Fatal("latest multiline message was not rendered as one line")
	}
	if strings.Contains(rendered, "Message 00") {
		t.Fatal("off-screen message was rendered")
	}
	if lines := strings.Count(rendered, "\n") + 1; lines > 10 {
		t.Fatalf("rendered lines=%d", lines)
	}
}

func TestReadOnlySendKeepsComposerAndShowsUnavailable(t *testing.T) {
	model, _ := readOnlyFixture(t)
	model = open(t, model)
	model = typeText(model, "synthetic hello")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing send command")
	}
	updated, _ = updated.(Model).Update(cmd())
	got := updated.(Model)
	if got.composer.Value() != "synthetic hello" || got.errorText != "Sending is not available yet." {
		t.Fatalf("composer=%q error=%q", got.composer.Value(), got.errorText)
	}
}
