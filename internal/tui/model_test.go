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

func TestConversationListKeepsJKQAsComposerInput(t *testing.T) {
	for _, input := range []string{"j", "k", "q", "/search j k q"} {
		m, _ := fixture(t)
		m.cursor = 1
		m = typeText(m, input)
		if got := m.composer.Value(); got != input {
			t.Fatalf("input=%q composer=%q", input, got)
		}
		if m.cursor != 1 {
			t.Fatalf("input=%q moved conversation cursor to %d", input, m.cursor)
		}
	}
}

func TestConversationListUsesOnlyArrowNavigation(t *testing.T) {
	m, _ := fixture(t)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursor != 1 || m.composer.Value() != "" {
		t.Fatalf("down cursor=%d composer=%q", m.cursor, m.composer.Value())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.cursor != 0 || m.composer.Value() != "" {
		t.Fatalf("up cursor=%d composer=%q", m.cursor, m.composer.Value())
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

type blockingSendAPI struct {
	app.API
	started chan struct{}
	release chan struct{}
}

func (s *blockingSendAPI) SendToConversation(ctx context.Context, _ int64, _ string) (domain.SendResult, error) {
	close(s.started)
	select {
	case <-ctx.Done():
		return domain.SendResult{}, ctx.Err()
	case <-s.release:
		return domain.SendResult{}, errors.New("test released blocked send")
	}
}

func TestCtrlCDuringSendCancelsOperationBeforeShutdown(t *testing.T) {
	m, _ := fixture(t)
	m = open(t, m)
	blocking := &blockingSendAPI{API: m.service, started: make(chan struct{}), release: make(chan struct{})}
	defer close(blocking.release)
	m.service = blocking
	m = typeText(m, "synthetic send")

	updated, send := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	sendResult := make(chan tea.Msg, 1)
	go func() { sendResult <- send() }()
	<-blocking.started

	_, shutdown := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if shutdown == nil {
		t.Fatal("missing shutdown command")
	}
	select {
	case result := <-sendResult:
		if sent, ok := result.(sentMsg); !ok || !errors.Is(sent.err, context.Canceled) {
			t.Fatalf("send result=%#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("send context was not canceled by Ctrl+C")
	}
	_ = shutdown()
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

func scrollingModel(t *testing.T, count int) Model {
	t.Helper()
	model, _ := fixture(t)
	model.screen = chatScreen
	model.height = 12
	model.messages = make([]domain.Message, count)
	for index := range model.messages {
		model.messages[index] = domain.Message{
			ID:        int64(index + 1),
			ThreadID:  1,
			Body:      fmt.Sprintf("Message %03d", index+1),
			Direction: domain.DirectionIncoming,
		}
	}
	return model
}

func TestChatPageKeysScrollOlderNewerAndLatest(t *testing.T) {
	model := scrollingModel(t, 20)
	latest := model.renderChat(28, 6)
	if !strings.Contains(latest, "Message 020") || strings.Contains(latest, "Message 001") {
		t.Fatalf("latest viewport=%q", latest)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	older := model.renderChat(28, 6)
	if !strings.Contains(older, "Message 011") || strings.Contains(older, "Message 020") {
		t.Fatalf("older viewport=%q", older)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)
	if got := model.renderChat(28, 6); got != latest {
		t.Fatalf("page down viewport=%q want=%q", got, latest)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if got := updated.(Model).renderChat(28, 6); got != latest {
		t.Fatalf("end viewport=%q want=%q", got, latest)
	}
}

type recordingMessagesAPI struct {
	app.API
	queries []domain.MessageQuery
	older   []domain.Message
}

func (s *recordingMessagesAPI) Messages(ctx context.Context, id int64, query domain.MessageQuery) ([]domain.Message, error) {
	s.queries = append(s.queries, query)
	if query.BeforeID > 0 {
		return append([]domain.Message(nil), s.older...), nil
	}
	return s.API.Messages(ctx, id, query)
}

func TestChatLoadsOlderPageWithoutMovingVisualAnchor(t *testing.T) {
	model := scrollingModel(t, 200)
	for index := range model.messages {
		model.messages[index].ID += 100
		model.messages[index].Body = fmt.Sprintf("Message %03d", index+101)
	}
	older := make([]domain.Message, 100)
	for index := range older {
		older[index] = domain.Message{ID: int64(index + 1), ThreadID: 1, Body: fmt.Sprintf("Message %03d", index+1), Direction: domain.DirectionIncoming}
	}
	recorder := &recordingMessagesAPI{API: model.service, older: older}
	model.service = recorder
	updated, _ := model.Update(messagesMsg{items: model.messages})
	model = updated.(Model)

	for range 39 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
		model = updated.(Model)
	}
	anchored := model.renderChat(28, 6)
	if !strings.Contains(anchored, "Message 101") {
		t.Fatalf("top loaded viewport=%q", anchored)
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("missing older page load command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if len(recorder.queries) != 1 || recorder.queries[0] != (domain.MessageQuery{BeforeID: 101, Limit: 200}) {
		t.Fatalf("queries=%+v", recorder.queries)
	}
	if got := model.renderChat(28, 6); got != anchored {
		t.Fatalf("prepend moved anchor: got=%q want=%q", got, anchored)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if got := updated.(Model).renderChat(28, 6); !strings.Contains(got, "Message 096") || strings.Contains(got, "Message 101") {
		t.Fatalf("new older page viewport=%q", got)
	}
}

func TestPollingFollowsBottomButPreservesScrolledViewport(t *testing.T) {
	bottom := scrollingModel(t, 20)
	updated, _ := bottom.Update(pollMsg{items: []domain.Message{{ID: 21, ThreadID: 1, Body: "Message 021", Direction: domain.DirectionIncoming}}})
	bottom = updated.(Model)
	if got := bottom.renderChat(28, 6); !strings.Contains(got, "Message 021") {
		t.Fatalf("bottom did not follow new message: %q", got)
	}

	scrolled := scrollingModel(t, 20)
	updated, _ = scrolled.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	scrolled = updated.(Model)
	before := scrolled.renderChat(28, 6)
	updated, _ = scrolled.Update(pollMsg{items: []domain.Message{
		{ID: 21, ThreadID: 1, Body: "Message 021", Direction: domain.DirectionIncoming},
		{ID: 21, ThreadID: 1, Body: "Message 021", Direction: domain.DirectionIncoming},
		{ID: 22, ThreadID: 2, Body: "other thread", Direction: domain.DirectionIncoming},
	}})
	scrolled = updated.(Model)
	if got := scrolled.renderChat(28, 6); got != before {
		t.Fatalf("poll moved scrolled viewport: got=%q want=%q", got, before)
	}
	count := 0
	for _, message := range scrolled.messages {
		if message.ID == 21 {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("new message duplicate count=%d", count)
	}
}

func TestSuccessfulSendReturnsScrolledChatToLatest(t *testing.T) {
	model := scrollingModel(t, 20)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = typeText(updated.(Model), "synthetic send")
	if before := model.renderChat(28, 6); strings.Contains(before, "Message 020") {
		t.Fatalf("test did not start scrolled: %q", before)
	}
	updated, send := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if send == nil {
		t.Fatal("missing send command")
	}
	updated, _ = updated.(Model).Update(send())
	if got := updated.(Model).renderChat(28, 6); !strings.Contains(got, "Message 020") {
		t.Fatalf("successful send did not return to latest: %q", got)
	}
}

func TestChatResizeKeepsSelectionAndValidScrollPosition(t *testing.T) {
	model := scrollingModel(t, 20)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	beforeThread := model.selectedID()
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 42, Height: 30})
	model = updated.(Model)
	if model.selectedID() != beforeThread || model.screen != chatScreen {
		t.Fatalf("selected=%d screen=%d", model.selectedID(), model.screen)
	}
	_ = model.View()
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
