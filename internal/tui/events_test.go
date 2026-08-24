package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/domain"
)

type eventAPI struct {
	messages chan domain.Message
	errors   chan error
	app.API
}

func (api eventAPI) SubscribeMessages(context.Context) (<-chan domain.Message, <-chan error) {
	return api.messages, api.errors
}

func TestEventDrivenModelAppendsIncomingWithoutPolling(t *testing.T) {
	model, _ := fixture(t)
	model = open(t, model)
	stream := eventAPI{
		API:      model.service,
		messages: make(chan domain.Message, 1),
		errors:   make(chan error, 1),
	}
	model.service = stream
	updated, command := model.Update(eventStreamMsg{messages: stream.messages, errors: stream.errors})
	model = updated.(Model)
	if command == nil || !model.eventDriven {
		t.Fatal("event stream did not start")
	}
	stream.messages <- domain.Message{ID: 99, ThreadID: 1, Body: "push", Direction: domain.DirectionIncoming}
	updated, refresh := model.Update(command())
	model = updated.(Model)
	if len(model.messages) == 0 || model.messages[len(model.messages)-1].ID != 99 {
		t.Fatalf("messages=%+v", model.messages)
	}
	if refresh == nil {
		t.Fatal("event listener was not rearmed")
	}
	updated, tick := model.Update(tickMsg(time.Now()))
	if tick != nil || !updated.(Model).eventDriven {
		t.Fatal("polling continued while event stream was active")
	}
}

func TestClosedEventStreamFallsBackToTick(t *testing.T) {
	model, _ := fixture(t)
	model.eventDriven = true
	model.pollInterval = time.Millisecond
	updated, command := model.Update(messageEventMsg{closed: true})
	if command == nil || updated.(Model).eventDriven {
		t.Fatal("event stream did not fall back")
	}
	message := command()
	if _, ok := message.(tickMsg); !ok {
		t.Fatalf("fallback command=%T", message)
	}
}

var _ tea.Model = Model{}
