package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/domain"
)

type screen uint8

const (
	conversationsScreen screen = iota
	chatScreen
)

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
type shutdownMsg struct{ err error }
type tickMsg time.Time

type Model struct {
	ctx               context.Context
	cancel            context.CancelFunc
	service           app.API
	composer          textinput.Model
	conversations     []domain.Conversation
	messages          []domain.Message
	cursor            int
	screen            screen
	width, height     int
	status, errorText string
	pollInterval      time.Duration
	sending           bool
	chatOffset        int
	loadingOlder      bool
	hasOlder          bool
}

const historyPageLimit = 200

func NewModel(ctx context.Context, service app.API, interval time.Duration) Model {
	modelCtx, cancel := context.WithCancel(ctx)
	i := textinput.New()
	i.Placeholder = "메시지 입력..."
	i.Prompt = "> "
	i.CharLimit = 4000
	i.Focus()
	if interval <= 0 {
		interval = time.Second
	}
	return Model{ctx: modelCtx, cancel: cancel, service: service, composer: i, pollInterval: interval, status: "Loading…", width: 80, height: 24}
}
func (m Model) Init() tea.Cmd { return tea.Batch(m.loadConversations(), m.tick()) }
func (m Model) loadConversations() tea.Cmd {
	return func() tea.Msg { v, e := m.service.Conversations(m.ctx); return conversationsMsg{v, e} }
}
func (m Model) loadMessages(id int64) tea.Cmd {
	return func() tea.Msg {
		v, e := m.service.Messages(m.ctx, id, domain.MessageQuery{Limit: historyPageLimit})
		return messagesMsg{threadID: id, items: v, err: e}
	}
}
func (m Model) loadOlderMessages(id, beforeID int64) tea.Cmd {
	return func() tea.Msg {
		v, e := m.service.Messages(m.ctx, id, domain.MessageQuery{BeforeID: beforeID, Limit: historyPageLimit})
		return olderMessagesMsg{threadID: id, items: v, err: e}
	}
}
func (m Model) tick() tea.Cmd {
	return tea.Tick(m.pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m Model) poll() tea.Cmd {
	focused := int64(0)
	if m.screen == chatScreen && len(m.conversations) > 0 {
		focused = m.conversations[m.cursor].ThreadID
	}
	return func() tea.Msg { v, e := m.service.Poll(m.ctx, focused); return pollMsg{v, e} }
}
func (m Model) shutdown() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	return func() tea.Msg { return shutdownMsg{m.service.Shutdown(context.Background())} }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(30, x.Width)
		m.height = max(10, x.Height)
		m.clampChatOffset()
	case conversationsMsg:
		m.replaceConversations(x.items)
		m.setError(x.err)
		m.refreshStatus()
	case messagesMsg:
		if x.threadID != 0 && x.threadID != m.selectedID() {
			return m, nil
		}
		m.messages = x.items
		m.chatOffset = 0
		m.loadingOlder = false
		m.hasOlder = len(x.items) == historyPageLimit
		m.setError(x.err)
		m.refreshStatus()
		if x.err == nil {
			m.screen = chatScreen
			m.composer.Focus()
		}
	case olderMessagesMsg:
		if x.threadID != m.selectedID() {
			return m, nil
		}
		m.loadingOlder = false
		m.setError(x.err)
		m.refreshStatus()
		if x.err == nil {
			m.messages, _ = mergeMessages(m.messages, x.items)
			m.hasOlder = len(x.items) == historyPageLimit
			m.clampChatOffset()
		}
	case sentMsg:
		m.sending = false
		m.setError(x.err)
		m.refreshStatus()
		if x.err == nil {
			m.composer.SetValue("")
			m.errorText = ""
			m.chatOffset = 0
			return m, tea.Batch(m.loadConversations(), m.loadMessages(m.selectedID()))
		}
	case tickMsg:
		return m, tea.Batch(m.poll(), m.tick())
	case pollMsg:
		m.setError(x.err)
		m.refreshStatus()
		if len(x.items) > 0 {
			if m.screen == chatScreen {
				selected := make([]domain.Message, 0, len(x.items))
				for _, message := range x.items {
					if message.ThreadID == m.selectedID() {
						selected = append(selected, message)
					}
				}
				var appended int
				m.messages, appended = mergeMessages(m.messages, selected)
				if m.chatOffset > 0 {
					m.chatOffset += appended
				}
				m.clampChatOffset()
				return m, m.loadConversations()
			}
			return m, m.loadConversations()
		}
	case shutdownMsg:
		m.setError(x.err)
		return m, tea.Quit
	case tea.KeyMsg:
		if x.Type == tea.KeyCtrlC {
			return m, m.shutdown()
		}
		if m.sending {
			return m, nil
		}
		if x.Type == tea.KeyEsc && m.screen == chatScreen {
			m.screen = conversationsScreen
			m.composer.SetValue("")
			m.errorText = ""
			return m, nil
		}
		if m.screen == conversationsScreen {
			return m.updateList(x)
		}
		switch x.Type {
		case tea.KeyPgUp:
			return m.scrollOlder()
		case tea.KeyPgDown:
			m.chatOffset = max(0, m.chatOffset-m.chatPageSize())
			return m, nil
		case tea.KeyEnd:
			m.chatOffset = 0
			return m, nil
		}
		if x.Type == tea.KeyEnter {
			return m.submit()
		}
		var cmd tea.Cmd
		m.composer, cmd = m.composer.Update(x)
		return m, cmd
	}
	return m, nil
}
func (m Model) updateList(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down":
		if m.cursor < len(m.conversations)-1 {
			m.cursor++
		}
		return m, nil
	case "enter":
		if strings.HasPrefix(strings.TrimSpace(m.composer.Value()), "/") {
			return m.submit()
		}
		if len(m.conversations) > 0 {
			return m, m.loadMessages(m.selectedID())
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(k)
	return m, cmd
}

func (m Model) scrollOlder() (tea.Model, tea.Cmd) {
	maximum := max(0, len(m.messages)-m.chatPageSize())
	if m.chatOffset < maximum {
		m.chatOffset = min(maximum, m.chatOffset+m.chatPageSize())
		return m, nil
	}
	if m.loadingOlder || !m.hasOlder || len(m.messages) == 0 {
		return m, nil
	}
	m.loadingOlder = true
	return m, m.loadOlderMessages(m.selectedID(), m.messages[0].ID)
}

func (m Model) chatPageSize() int {
	return max(1, max(4, m.height-6)-1)
}

func (m *Model) clampChatOffset() {
	m.chatOffset = min(max(0, m.chatOffset), max(0, len(m.messages)-m.chatPageSize()))
}

func mergeMessages(existing, incoming []domain.Message) ([]domain.Message, int) {
	byID := make(map[int64]domain.Message, len(existing)+len(incoming))
	var latestID int64
	for _, message := range existing {
		byID[message.ID] = message
		if message.ID > latestID {
			latestID = message.ID
		}
	}
	appended := 0
	for _, message := range incoming {
		if _, found := byID[message.ID]; found {
			continue
		}
		byID[message.ID] = message
		if message.ID > latestID {
			appended++
		}
	}
	merged := make([]domain.Message, 0, len(byID))
	for _, message := range byID {
		merged = append(merged, message)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].ID < merged[j].ID })
	return merged, appended
}
func (m Model) submit() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.composer.Value())
	if value == "" {
		return m, nil
	}
	if strings.HasPrefix(value, "/") {
		c, e := ParseCommand(value)
		if e != nil {
			m.errorText = e.Error()
			return m, nil
		}
		switch c.Name {
		case "exit", "quit":
			return m, m.shutdown()
		case "help":
			m.errorText = "Enter send · Esc back · /exit /quit exit"
			m.composer.SetValue("")
			return m, nil
		default:
			m.errorText = "/" + c.Name + ": not implemented yet"
			m.composer.SetValue("")
			return m, nil
		}
	}
	id := m.selectedID()
	m.sending = true
	m.status = "Sending…"
	m.errorText = ""
	return m, func() tea.Msg {
		result, err := m.service.SendToConversation(m.ctx, id, value)
		return sentMsg{result: result, err: err}
	}
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

func (m *Model) setError(e error) {
	if e != nil {
		if errors.Is(e, domain.ErrSendingNotImplemented) {
			m.errorText = "Sending is not available yet."
			return
		}
		if errors.Is(e, app.ErrGroupSendUnsupported) {
			m.errorText = "Group conversation sending is not supported yet."
			return
		}
		m.errorText = e.Error()
	}
}

func (m *Model) refreshStatus() {
	if m.sending {
		return
	}
	if status := m.service.Status(m.ctx); status.Label != "" {
		m.status = status.Label
	}
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	outgoingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

func (m Model) View() string {
	w := max(30, m.width)
	header := headerStyle.Render("GalaxyTTY") + strings.Repeat(" ", max(1, w-25)) + "● " + m.status
	bodyHeight := max(4, m.height-6)
	leftW := max(14, min(28, w/3))
	left := m.renderConversations(leftW, bodyHeight)
	right := m.renderChat(max(14, w-leftW-3), bodyHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " │ ", right)
	footer := m.errorText
	if m.screen == chatScreen {
		footer = m.composer.View() + "\nPgUp/PgDn scroll · End latest · Esc back\n" + footer
	} else {
		footer = m.composer.View() + "\n↑/↓ select · Enter open · slash commands\n" + footer
	}
	return lipgloss.NewStyle().Width(w).Render(header + "\n" + strings.Repeat("─", w) + "\n" + body + "\n" + strings.Repeat("─", w) + "\n" + footer)
}
func (m Model) renderConversations(w, h int) string {
	lines := []string{"Conversations"}
	visible := max(1, (h-1)/2)
	start := max(0, m.cursor-visible/2)
	end := min(len(m.conversations), start+visible)
	start = max(0, end-visible)
	for i := start; i < end; i++ {
		c := m.conversations[i]
		mark := "  "
		if c.UnreadCount > 0 {
			mark = "● "
		}
		name := truncate(c.Title, w-4)
		if i == m.cursor {
			name = selectedStyle.Render(name)
		}
		lines = append(lines, mark+name, "  "+truncate(c.Snippet, w-2))
	}
	return lipgloss.NewStyle().Width(w).Height(h).Render(strings.Join(lines, "\n"))
}
func (m Model) renderChat(w, h int) string {
	if m.screen != chatScreen || len(m.conversations) == 0 {
		return lipgloss.NewStyle().Width(w).Height(h).Render("Select a conversation")
	}
	lines := []string{headerStyle.Render(truncate(m.conversations[m.cursor].Title, w))}
	visible := max(1, h-1)
	maximumOffset := max(0, len(m.messages)-visible)
	offset := min(max(0, m.chatOffset), maximumOffset)
	start := max(0, len(m.messages)-visible-offset)
	end := min(len(m.messages), start+visible)
	for _, msg := range m.messages[start:end] {
		body := msg.Body
		if len(msg.Attachments) > 0 {
			body = "🖼 이미지"
		}
		body = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(body)
		body = truncate(body, max(1, w-3))
		if msg.Direction == domain.DirectionOutgoing {
			body = outgoingStyle.Render("→ " + body)
		} else {
			body = "← " + body
		}
		lines = append(lines, body)
	}
	return lipgloss.NewStyle().Width(w).Height(h).Render(strings.Join(lines, "\n"))
}
func truncate(s string, w int) string {
	r := []rune(s)
	if w <= 0 {
		return ""
	}
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// Run owns the Bubble Tea program; all business operations remain asynchronous tea.Cmd calls.
func Run(ctx context.Context, in io.Reader, out io.Writer, service app.API, interval time.Duration) error {
	_, err := tea.NewProgram(NewModel(ctx, service, interval), tea.WithAltScreen(), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

func (m Model) Debug() string { return fmt.Sprintf("screen=%d selected=%d", m.screen, m.selectedID()) }
