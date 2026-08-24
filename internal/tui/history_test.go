package tui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestConversationListKeepsJKQAsComposerInput(t *testing.T) {
	for _, input := range []string{"j", "k", "q", "/search j k q"} {
		model, _ := fixture(t)
		model.cursor = 1
		model = typeText(model, input)
		if got := model.composer.Value(); got != input {
			t.Fatalf("input=%q composer=%q", input, got)
		}
		if model.cursor != 1 {
			t.Fatalf("input=%q moved cursor=%d", input, model.cursor)
		}
	}
}

func TestConversationListUsesArrowNavigation(t *testing.T) {
	model, _ := fixture(t)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if model.cursor != 1 || model.composer.Value() != "" {
		t.Fatalf("down cursor=%d composer=%q", model.cursor, model.composer.Value())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.cursor != 0 || model.composer.Value() != "" {
		t.Fatalf("up cursor=%d composer=%q", model.cursor, model.composer.Value())
	}
}

type blockingSendAPI struct {
	app.API
	started chan struct{}
	release chan struct{}
}

func (api *blockingSendAPI) SendToConversation(ctx context.Context, _ int64, _ string) (domain.SendResult, error) {
	close(api.started)
	select {
	case <-ctx.Done():
		return domain.SendResult{}, ctx.Err()
	case <-api.release:
		return domain.SendResult{}, errors.New("test released blocked send")
	}
}

func TestCtrlCDuringSendCancelsOperationBeforeShutdown(t *testing.T) {
	model, _ := fixture(t)
	model = open(t, model)
	blocking := &blockingSendAPI{API: model.service, started: make(chan struct{}), release: make(chan struct{})}
	defer close(blocking.release)
	model.service = blocking
	model = typeText(model, "synthetic send")

	updated, send := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	result := make(chan tea.Msg, 1)
	go func() { result <- send() }()
	<-blocking.started

	_, shutdown := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if shutdown == nil {
		t.Fatal("missing shutdown command")
	}
	select {
	case message := <-result:
		sent, ok := message.(sentMsg)
		if !ok || !errors.Is(sent.err, context.Canceled) {
			t.Fatalf("send result=%#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("send context was not canceled")
	}
	_ = shutdown()
}

type recordingMessagesAPI struct {
	app.API
	queries []domain.MessageQuery
	older   []domain.Message
}

func (api *recordingMessagesAPI) Messages(ctx context.Context, threadID int64, query domain.MessageQuery) ([]domain.Message, error) {
	api.queries = append(api.queries, query)
	if query.BeforeID > 0 {
		return append([]domain.Message(nil), api.older...), nil
	}
	return api.API.Messages(ctx, threadID, query)
}

func historyModel(t *testing.T, count int) Model {
	t.Helper()
	model, _ := fixture(t)
	model.screen = chatScreen
	model.height = 20
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

func TestInitialHistoryRequestIsBounded(t *testing.T) {
	model, _ := fixture(t)
	recorder := &recordingMessagesAPI{API: model.service}
	model.service = recorder
	_, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("missing history command")
	}
	_ = command()
	if len(recorder.queries) != 1 || recorder.queries[0] != (domain.MessageQuery{Limit: historyPageLimit}) {
		t.Fatalf("queries=%+v", recorder.queries)
	}
}

func TestPageKeysScrollAndReturnToLatest(t *testing.T) {
	model := historyModel(t, 30)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	if model.chatOffset == 0 {
		t.Fatal("page up did not move history offset")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)
	if model.chatOffset != 0 {
		t.Fatalf("page down offset=%d", model.chatOffset)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnd})
	if got := updated.(Model).chatOffset; got != 0 {
		t.Fatalf("end offset=%d", got)
	}
}

func TestLazyOlderPageUsesBeforeIDAndPreservesAnchor(t *testing.T) {
	model := historyModel(t, historyPageLimit)
	for index := range model.messages {
		model.messages[index].ID += 100
	}
	older := make([]domain.Message, 100)
	for index := range older {
		older[index] = domain.Message{ID: int64(index + 1), ThreadID: 1, Body: fmt.Sprintf("Older %03d", index+1)}
	}
	recorder := &recordingMessagesAPI{API: model.service, older: older}
	model.service = recorder
	model.hasOlder = true
	model.chatOffset = len(model.messages) - 1
	anchoredID := model.messages[len(model.messages)-1-model.chatOffset].ID

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	if command == nil || !model.loadingOlder {
		t.Fatal("missing lazy history load")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if len(recorder.queries) != 1 || recorder.queries[0] != (domain.MessageQuery{BeforeID: 101, Limit: historyPageLimit}) {
		t.Fatalf("queries=%+v", recorder.queries)
	}
	if got := model.messages[len(model.messages)-1-model.chatOffset].ID; got != anchoredID {
		t.Fatalf("anchor moved from %d to %d", anchoredID, got)
	}
}

func TestIncomingEventFollowsLatestOrPreservesScrolledAnchor(t *testing.T) {
	latest := historyModel(t, 20)
	updated, _ := latest.Update(pollMsg{items: []domain.Message{{ID: 21, ThreadID: 1, Body: "latest"}}})
	latest = updated.(Model)
	if latest.chatOffset != 0 || latest.messages[len(latest.messages)-1].ID != 21 {
		t.Fatalf("latest offset=%d messages=%+v", latest.chatOffset, latest.messages)
	}

	scrolled := historyModel(t, 20)
	scrolled.chatOffset = 5
	anchorID := scrolled.messages[len(scrolled.messages)-1-scrolled.chatOffset].ID
	updated, _ = scrolled.Update(pollMsg{items: []domain.Message{
		{ID: 21, ThreadID: 1, Body: "latest"},
		{ID: 21, ThreadID: 1, Body: "duplicate"},
		{ID: 22, ThreadID: 2, Body: "other thread"},
	}})
	scrolled = updated.(Model)
	if got := scrolled.messages[len(scrolled.messages)-1-scrolled.chatOffset].ID; got != anchorID {
		t.Fatalf("anchor moved from %d to %d", anchorID, got)
	}
	count := 0
	for _, message := range scrolled.messages {
		if message.ID == 21 {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate count=%d", count)
	}
}

func TestStaleHistoryResultDoesNotReplaceSelectedChat(t *testing.T) {
	model := historyModel(t, 3)
	model.cursor = 1
	before := append([]domain.Message(nil), model.messages...)
	updated, _ := model.Update(messagesMsg{threadID: 1, items: []domain.Message{{ID: 99, ThreadID: 1}}})
	model = updated.(Model)
	if len(model.messages) != len(before) || model.messages[0].ID != before[0].ID {
		t.Fatalf("stale result replaced messages: %+v", model.messages)
	}
}
