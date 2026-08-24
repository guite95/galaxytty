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
	if got := u.(Model); !got.sending || got.status != "Sending…" {
		t.Fatalf("sending=%t status=%q", got.sending, got.status)
	}
	msg := cmd()
	u, refresh := u.(Model).Update(msg)
	if len(b.Sent) != 1 || b.Sent[0].Address != "01012345678" {
		t.Fatal(b.Sent)
	}
	got := u.(Model)
	if got.sending || got.composer.Value() != "" || got.errorText != "" {
		t.Fatalf("sending=%t composer=%q error=%q", got.sending, got.composer.Value(), got.errorText)
	}
	refreshMsg := refresh()
	batch, ok := refreshMsg.(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("refresh=%T %#v", refreshMsg, refreshMsg)
	}
	foundConversations, foundMessages := false, false
	for _, command := range batch {
		switch command().(type) {
		case conversationsMsg:
			foundConversations = true
		case messagesMsg:
			foundMessages = true
		}
	}
	if !foundConversations || !foundMessages {
		t.Fatalf("conversation refresh=%t message refresh=%t", foundConversations, foundMessages)
	}
}

type acceptedUnverifiedSendAPI struct{ app.API }

func (api acceptedUnverifiedSendAPI) SendToConversation(_ context.Context, threadID int64, _ string) (domain.SendResult, error) {
	return domain.SendResult{
		ThreadID: threadID,
		Outcome:  domain.SendOutcomeAcceptedUnverified,
		Evidence: "remote_input_pending_intent_accepted",
	}, nil
}

func TestAcceptedUnverifiedReplyClearsComposerAndShowsNotice(t *testing.T) {
	model, _ := fixture(t)
	model = open(t, model)
	model.service = acceptedUnverifiedSendAPI{API: model.service}
	model = typeText(model, "synthetic reply")

	updated, send := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated, refresh := updated.(Model).Update(send())
	got := updated.(Model)
	if refresh == nil || got.sending || got.composer.Value() != "" || got.errorText != "" {
		t.Fatalf("refresh=%v sending=%t composer=%q error=%q", refresh, got.sending, got.composer.Value(), got.errorText)
	}
	if got.noticeText != "Samsung Messages accepted the reply · delivery unverified" {
		t.Fatalf("notice=%q", got.noticeText)
	}
	if !strings.Contains(got.View(), got.noticeText) {
		t.Fatalf("view did not render notice: %q", got.View())
	}
}

type acceptedConversationSenderForTUI struct{}

func (acceptedConversationSenderForTUI) Send(context.Context, string, string) (domain.SendResult, error) {
	return domain.SendResult{}, errors.New("address send must not be used")
}

func (acceptedConversationSenderForTUI) SendToConversation(_ context.Context, threadID int64, _ string) (domain.SendResult, error) {
	return domain.SendResult{ThreadID: threadID, Outcome: domain.SendOutcomeAcceptedUnverified}, nil
}

func TestAcceptedUnverifiedReplyReloadsAsOutgoingBubble(t *testing.T) {
	backend := mock.New()
	service := app.NewService(
		backend,
		acceptedConversationSenderForTUI{},
		nil,
		nil,
		app.NotificationPolicy{},
		domain.ApplicationStatus{Label: "Mock Connected"},
	)
	model := NewModel(context.Background(), service, time.Hour)
	updated, _ := model.Update(conversationsMsg{items: backend.ConversationsData})
	model = open(t, updated.(Model))
	model = typeText(model, "synthetic RCS reply")

	updated, send := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated, refresh := updated.(Model).Update(send())
	refreshMessage := refresh()
	batch, ok := refreshMessage.(tea.BatchMsg)
	if !ok {
		t.Fatalf("refresh=%T", refreshMessage)
	}
	for _, command := range batch {
		message := command()
		if _, isMessages := message.(messagesMsg); isMessages {
			updated, _ = updated.(Model).Update(message)
		}
	}
	got := updated.(Model)
	if len(got.messages) == 0 {
		t.Fatal("accepted outgoing bubble was not loaded")
	}
	local := got.messages[len(got.messages)-1]
	if local.Body != "synthetic RCS reply" || local.Direction != domain.DirectionOutgoing ||
		local.SendOutcome != domain.SendOutcomeAcceptedUnverified {
		t.Fatalf("local=%+v", local)
	}
	if !strings.Contains(got.View(), "전송 요청됨 · 미검증") {
		t.Fatalf("view=%q", got.View())
	}
}

func TestDuplicateEnterAndComposerChangesAreSuppressedWhileSending(t *testing.T) {
	m, _ := fixture(t)
	m = open(t, m)
	m = typeText(m, "hello")
	updated, send := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if send == nil || !m.sending {
		t.Fatal("send did not start")
	}
	updated, duplicate := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if duplicate != nil || updated.(Model).composer.Value() != "hello" {
		t.Fatalf("duplicate=%v composer=%q", duplicate, updated.(Model).composer.Value())
	}
	updated, typed := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if typed != nil || updated.(Model).composer.Value() != "hello" {
		t.Fatalf("typed=%v composer=%q", typed, updated.(Model).composer.Value())
	}
}

func TestSendingStatusSurvivesBackgroundRefresh(t *testing.T) {
	m, backend := fixture(t)
	m = open(t, m)
	m = typeText(m, "hello")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(pollMsg{})
	m = updated.(Model)
	if m.status != "Sending…" {
		t.Fatalf("status after poll=%q", m.status)
	}
	updated, _ = m.Update(conversationsMsg{items: backend.ConversationsData})
	if got := updated.(Model).status; got != "Sending…" {
		t.Fatalf("status after conversations=%q", got)
	}
}

func TestGroupSendFailureKeepsComposerAndClearsSending(t *testing.T) {
	m, backend := fixture(t)
	backend.ConversationsData[0].Participants = append(backend.ConversationsData[0].Participants, domain.Contact{Phone: "01000000000"})
	m.conversations[0].Participants = append(m.conversations[0].Participants, domain.Contact{Phone: "01000000000"})
	m = open(t, m)
	m = typeText(m, "group text")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated, _ = updated.(Model).Update(cmd())
	got := updated.(Model)
	if got.sending || got.composer.Value() != "group text" || got.errorText != "Group conversation sending is not supported yet." {
		t.Fatalf("sending=%t composer=%q error=%q", got.sending, got.composer.Value(), got.errorText)
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

func TestSearchSelectsConversationAndCyclesMatches(t *testing.T) {
	m, _ := fixture(t)
	m.conversations = []domain.Conversation{
		{ThreadID: 1, Title: "장욱", Snippet: "저녁 약속"},
		{ThreadID: 2, Title: "김철수", Snippet: "프로젝트 저녁 회의"},
		{ThreadID: 3, Title: "엄마", Snippet: "집에 언제 와?"},
	}
	m = typeText(m, "/search 저녁")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.cursor != 0 || m.composer.Value() != "" || m.errorText != "" {
		t.Fatalf("first search cursor=%d composer=%q error=%q", m.cursor, m.composer.Value(), m.errorText)
	}
	m = typeText(m, "/search 저녁")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Fatalf("repeated search cursor=%d", m.cursor)
	}
}

func TestSearchMovesChatAnchorUsingKoreanAndEmoji(t *testing.T) {
	m := historyModel(t, 4)
	m.messages[0].Body = "첫 메시지 😀"
	m.messages[2].Body = "한글 검색 결과"
	m = typeText(m, "/search 한글")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.chatOffset != 1 || m.errorText != "" {
		t.Fatalf("chat offset=%d error=%q", m.chatOffset, m.errorText)
	}
	m = typeText(m, "/search 없음")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(updated.(Model).errorText, "No match") {
		t.Fatalf("error=%q", updated.(Model).errorText)
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
	if !strings.Contains(rendered, "Recent") || !strings.Contains(rendered, "message") {
		t.Fatal("latest multiline message was not rendered")
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
	if !updated.(Model).sending {
		t.Fatal("send state did not start")
	}
	updated, _ = updated.(Model).Update(cmd())
	got := updated.(Model)
	if got.sending || got.composer.Value() != "synthetic hello" || got.errorText != "Sending is not available yet." {
		t.Fatalf("sending=%t composer=%q error=%q", got.sending, got.composer.Value(), got.errorText)
	}
}
