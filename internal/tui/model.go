package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/domain"
)

type screen uint8

const (
	conversationsScreen screen = iota
	chatScreen
)

const historyPageLimit = 200

type conversationsMsg struct {
	items []domain.Conversation
	err   error
}

type messagesMsg struct {
	threadID int64
	items    []domain.Message
	err      error
}

type olderMessagesMsg struct {
	threadID int64
	items    []domain.Message
	err      error
}

type sentMsg struct {
	result domain.SendResult
	err    error
}

type pollMsg struct {
	items []domain.Message
	err   error
}

type eventStreamMsg struct {
	messages <-chan domain.Message
	errors   <-chan error
}

type messageEventMsg struct {
	message domain.Message
	err     error
	closed  bool
}

type statusStreamMsg struct {
	updates <-chan domain.ApplicationStatus
}
type statusEventMsg struct {
	status domain.ApplicationStatus
	closed bool
}

type shutdownMsg struct{ err error }
type tickMsg time.Time

type Model struct {
	ctx             context.Context
	cancel          context.CancelFunc
	service         app.API
	composer        textinput.Model
	conversations   []domain.Conversation
	messages        []domain.Message
	cursor          int
	screen          screen
	width, height   int
	status          string
	connectionState string
	deviceName      string
	errorText       string
	noticeText      string
	pollInterval    time.Duration
	sending         bool
	chatOffset      int
	loadingOlder    bool
	hasOlder        bool
	eventMessages   <-chan domain.Message
	eventErrors     <-chan error
	eventDriven     bool
	statusUpdates   <-chan domain.ApplicationStatus
	searchQuery     string
	searchScreen    screen
}

func NewModel(ctx context.Context, service app.API, interval time.Duration) Model {
	modelContext, cancel := context.WithCancel(ctx)
	composer := textinput.New()
	composer.Placeholder = "메시지 입력..."
	composer.Prompt = "› "
	composer.CharLimit = 4000
	composer.Focus()
	if interval <= 0 {
		interval = time.Second
	}
	model := Model{
		ctx:          modelContext,
		cancel:       cancel,
		service:      service,
		composer:     composer,
		pollInterval: interval,
		status:       "Loading…",
		width:        80,
		height:       24,
	}
	model.resizeComposer()
	return model
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadConversations(), m.connectEvents(), m.connectStatusEvents(), m.tick())
}

func (m Model) selectedID() int64 {
	if m.cursor >= 0 && m.cursor < len(m.conversations) {
		return m.conversations[m.cursor].ThreadID
	}
	return 0
}

func (m *Model) replaceConversations(items []domain.Conversation) {
	selectedThreadID := m.selectedID()
	m.conversations = items
	if len(items) == 0 {
		m.cursor = 0
		if m.screen == chatScreen {
			m.screen = conversationsScreen
			m.messages = nil
		}
		return
	}
	if selectedThreadID != 0 {
		for index, conversation := range items {
			if conversation.ThreadID == selectedThreadID {
				m.cursor = index
				return
			}
		}
	}
	m.cursor = min(m.cursor, len(items)-1)
}

func (m *Model) setError(err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, domain.ErrSendingNotImplemented):
		m.errorText = "Sending is not available yet."
	case errors.Is(err, app.ErrGroupSendUnsupported):
		m.errorText = "Group conversation sending is not supported yet."
	default:
		m.errorText = err.Error()
	}
}

func (m *Model) refreshStatus() {
	if m.sending {
		return
	}
	m.applyStatus(m.service.Status(m.ctx))
}

func (m *Model) applyStatus(status domain.ApplicationStatus) {
	if status.Label != "" {
		m.status = status.Label
	}
	if status.State != "" {
		m.connectionState = status.State
	}
	if status.Device != "" {
		m.deviceName = status.Device
	}
}

func (m *Model) resizeComposer() { m.composer.Width = max(1, m.width-2) }

func (m Model) Debug() string { return fmt.Sprintf("screen=%d selected=%d", m.screen, m.selectedID()) }

// Run owns the Bubble Tea program; business operations remain asynchronous
// commands below the application-service boundary.
func Run(ctx context.Context, in io.Reader, out io.Writer, service app.API, interval time.Duration) error {
	_, err := tea.NewProgram(
		NewModel(ctx, service, interval),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithInput(in),
		tea.WithOutput(out),
	).Run()
	return err
}
