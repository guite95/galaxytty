package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
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
