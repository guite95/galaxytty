package tui

import (
	"context"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/galaxytty/galaxytty/internal/domain"
)

func (m Model) loadConversations() tea.Cmd {
	return func() tea.Msg {
		items, err := m.service.Conversations(m.ctx)
		return conversationsMsg{items: items, err: err}
	}
}

func (m Model) loadMessages(threadID int64) tea.Cmd {
	return func() tea.Msg {
		items, err := m.service.Messages(m.ctx, threadID, domain.MessageQuery{Limit: historyPageLimit})
		return messagesMsg{threadID: threadID, items: items, err: err}
	}
}

func (m Model) loadOlderMessages(threadID, beforeID int64) tea.Cmd {
	return func() tea.Msg {
		items, err := m.service.Messages(m.ctx, threadID, domain.MessageQuery{BeforeID: beforeID, Limit: historyPageLimit})
		return olderMessagesMsg{threadID: threadID, items: items, err: err}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(m.pollInterval, func(value time.Time) tea.Msg { return tickMsg(value) })
}

func (m Model) connectEvents() tea.Cmd {
	return func() tea.Msg {
		messages, errorsChannel := m.service.SubscribeMessages(m.ctx)
		return eventStreamMsg{messages: messages, errors: errorsChannel}
	}
}

func (m Model) connectStatusEvents() tea.Cmd {
	return func() tea.Msg {
		return statusStreamMsg{updates: m.service.SubscribeStatus(m.ctx)}
	}
}

func (m Model) waitForStatus() tea.Cmd {
	updates := m.statusUpdates
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
			return statusEventMsg{closed: true}
		case status, open := <-updates:
			return statusEventMsg{status: status, closed: !open}
		}
	}
}

func (m Model) waitForEvent() tea.Cmd {
	messages := m.eventMessages
	errorsChannel := m.eventErrors
	return func() tea.Msg {
		for messages != nil || errorsChannel != nil {
			select {
			case <-m.ctx.Done():
				return messageEventMsg{closed: true}
			case message, open := <-messages:
				if !open {
					messages = nil
					continue
				}
				return messageEventMsg{message: message}
			case err, open := <-errorsChannel:
				if !open {
					errorsChannel = nil
					continue
				}
				return messageEventMsg{err: err}
			}
		}
		return messageEventMsg{closed: true}
	}
}

func (m Model) poll() tea.Cmd {
	focused := int64(0)
	if m.screen == chatScreen {
		focused = m.selectedID()
	}
	return func() tea.Msg {
		items, err := m.service.Poll(m.ctx, focused)
		return pollMsg{items: items, err: err}
	}
}

func (m Model) shutdown() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	return func() tea.Msg {
		return shutdownMsg{err: m.service.Shutdown(context.Background())}
	}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := message.(type) {
	case tea.WindowSizeMsg:
		m.width = max(30, typed.Width)
		m.height = max(10, typed.Height)
		m.resizeComposer()
		m.clampChatOffset()
	case conversationsMsg:
		m.replaceConversations(typed.items)
		m.setError(typed.err)
		m.refreshStatus()
	case messagesMsg:
		if typed.threadID != 0 && typed.threadID != m.selectedID() {
			return m, nil
		}
		m.messages = typed.items
		m.chatOffset = 0
		m.loadingOlder = false
		m.hasOlder = len(typed.items) == historyPageLimit
		m.setError(typed.err)
		m.refreshStatus()
		if typed.err == nil {
			m.screen = chatScreen
			m.composer.Focus()
		}
	case olderMessagesMsg:
		if typed.threadID != 0 && typed.threadID != m.selectedID() {
			return m, nil
		}
		m.loadingOlder = false
		m.setError(typed.err)
		m.refreshStatus()
		if typed.err == nil {
			m.messages, _ = mergeMessages(m.messages, typed.items)
			m.hasOlder = len(typed.items) == historyPageLimit
			m.clampChatOffset()
		}
	case sentMsg:
		m.sending = false
		m.setError(typed.err)
		m.refreshStatus()
		if typed.err == nil {
			m.composer.SetValue("")
			m.errorText = ""
			m.noticeText = ""
			if typed.result.Outcome == domain.SendOutcomeAcceptedUnverified {
				m.noticeText = "Samsung Messages accepted the reply · delivery unverified"
			}
			m.chatOffset = 0
			return m, tea.Batch(m.loadConversations(), m.loadMessages(m.selectedID()))
		}
	case tickMsg:
		if m.eventDriven {
			return m, nil
		}
		return m, tea.Batch(m.poll(), m.tick())
	case eventStreamMsg:
		m.eventMessages = typed.messages
		m.eventErrors = typed.errors
		m.eventDriven = typed.messages != nil || typed.errors != nil
		if m.eventDriven {
			return m, m.waitForEvent()
		}
	case statusStreamMsg:
		m.statusUpdates = typed.updates
		if m.statusUpdates != nil {
			return m, m.waitForStatus()
		}
	case statusEventMsg:
		if typed.closed {
			m.statusUpdates = nil
			return m, nil
		}
		m.applyStatus(typed.status)
		return m, m.waitForStatus()
	case messageEventMsg:
		if typed.closed {
			m.eventDriven = false
			m.eventMessages = nil
			m.eventErrors = nil
			return m, m.tick()
		}
		if typed.err != nil {
			m.setError(typed.err)
			commands := []tea.Cmd{m.waitForEvent(), m.loadConversations()}
			if m.screen == chatScreen {
				commands = append(commands, m.loadMessages(m.selectedID()))
			}
			return m, tea.Batch(commands...)
		}
		if typed.message.ThreadID == m.selectedID() && m.screen == chatScreen {
			var appended int
			m.messages, appended = mergeMessages(m.messages, []domain.Message{typed.message})
			if m.chatOffset > 0 {
				m.chatOffset += appended
			}
			m.clampChatOffset()
		}
		return m, tea.Batch(m.waitForEvent(), m.loadConversations())
	case pollMsg:
		m.setError(typed.err)
		m.refreshStatus()
		if len(typed.items) > 0 {
			if m.screen == chatScreen {
				selected := make([]domain.Message, 0, len(typed.items))
				for _, item := range typed.items {
					if item.ThreadID == m.selectedID() {
						selected = append(selected, item)
					}
				}
				var appended int
				m.messages, appended = mergeMessages(m.messages, selected)
				if m.chatOffset > 0 {
					m.chatOffset += appended
				}
				m.clampChatOffset()
			}
			return m, m.loadConversations()
		}
	case shutdownMsg:
		m.setError(typed.err)
		return m, tea.Quit
	case tea.KeyMsg:
		return m.updateKey(typed)
	}
	return m, nil
}

func (m Model) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyCtrlC {
		return m, m.shutdown()
	}
	if m.sending {
		return m, nil
	}
	if key.Type == tea.KeyEsc && m.screen == chatScreen {
		m.screen = conversationsScreen
		m.composer.SetValue("")
		m.errorText = ""
		m.noticeText = ""
		m.searchQuery = ""
		return m, nil
	}
	if m.screen == conversationsScreen {
		return m.updateList(key)
	}
	switch key.Type {
	case tea.KeyPgUp:
		return m.scrollOlder()
	case tea.KeyPgDown:
		m.chatOffset = max(0, m.chatOffset-m.chatPageSize())
		return m, nil
	case tea.KeyEnd:
		m.chatOffset = 0
		return m, nil
	case tea.KeyEnter:
		return m.submit()
	}
	var command tea.Cmd
	m.composer, command = m.composer.Update(key)
	return m, command
}

func (m Model) updateList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case tea.KeyDown:
		if m.cursor < len(m.conversations)-1 {
			m.cursor++
		}
		return m, nil
	case tea.KeyEnter:
		if strings.HasPrefix(strings.TrimSpace(m.composer.Value()), "/") {
			return m.submit()
		}
		if len(m.conversations) > 0 {
			return m, m.loadMessages(m.selectedID())
		}
		return m, nil
	}
	var command tea.Cmd
	m.composer, command = m.composer.Update(key)
	return m, command
}

func (m Model) submit() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.composer.Value())
	if value == "" {
		return m, nil
	}
	if strings.HasPrefix(value, "/") {
		command, err := ParseCommand(value)
		if err != nil {
			m.errorText = err.Error()
			return m, nil
		}
		switch command.Name {
		case "exit", "quit":
			return m, m.shutdown()
		case "help":
			m.errorText = "Enter send · Esc back · PgUp/PgDn history · /exit /quit exit"
			m.composer.SetValue("")
			return m, nil
		case "search":
			return m.search(command.Args)
		default:
			m.errorText = "/" + command.Name + ": not implemented yet"
			m.composer.SetValue("")
			return m, nil
		}
	}
	threadID := m.selectedID()
	m.sending = true
	m.status = "Sending…"
	m.connectionState = "sending…"
	m.errorText = ""
	m.noticeText = ""
	return m, func() tea.Msg {
		result, err := m.service.SendToConversation(m.ctx, threadID, value)
		return sentMsg{result: result, err: err}
	}
}

func (m Model) search(arguments []string) (tea.Model, tea.Cmd) {
	query := strings.TrimSpace(strings.Join(arguments, " "))
	if query == "" {
		m.errorText = "Usage: /search text"
		return m, nil
	}
	normalized := strings.ToLower(query)
	repeat := normalized == m.searchQuery && m.searchScreen == m.screen
	m.searchQuery = normalized
	m.searchScreen = m.screen
	m.composer.SetValue("")
	m.errorText = ""

	if m.screen == conversationsScreen {
		start := 0
		if repeat && len(m.conversations) > 0 {
			start = (m.cursor + 1) % len(m.conversations)
		}
		for offset := range len(m.conversations) {
			index := (start + offset) % len(m.conversations)
			if conversationContains(m.conversations[index], normalized) {
				m.cursor = index
				return m, nil
			}
		}
	} else {
		start := len(m.messages) - 1
		if repeat {
			start = len(m.messages) - 2 - m.chatOffset
			if start < 0 {
				start = len(m.messages) - 1
			}
		}
		for offset := range len(m.messages) {
			index := start - offset
			if index < 0 {
				index += len(m.messages)
			}
			if messageContains(m.messages[index], normalized) {
				m.chatOffset = len(m.messages) - 1 - index
				return m, nil
			}
		}
	}
	m.errorText = "No match for “" + query + "”"
	return m, nil
}

func conversationContains(conversation domain.Conversation, normalized string) bool {
	parts := []string{conversation.Title, conversation.Snippet}
	for _, participant := range conversation.Participants {
		parts = append(parts, participant.DisplayName, participant.Phone)
	}
	return strings.Contains(strings.ToLower(strings.Join(parts, "\n")), normalized)
}

func messageContains(message domain.Message, normalized string) bool {
	return strings.Contains(strings.ToLower(message.Body+"\n"+message.Address), normalized)
}

func (m Model) scrollOlder() (tea.Model, tea.Cmd) {
	maximum := max(0, len(m.messages)-1)
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

func (m Model) chatPageSize() int { return max(1, (m.height-8)/3) }

func (m *Model) clampChatOffset() {
	m.chatOffset = min(max(0, m.chatOffset), max(0, len(m.messages)-1))
}

func mergeMessages(existing, incoming []domain.Message) ([]domain.Message, int) {
	byID := make(map[int64]domain.Message, len(existing)+len(incoming))
	var latestID int64
	for _, message := range existing {
		byID[message.ID] = message
		latestID = max(latestID, message.ID)
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
	sort.Slice(merged, func(left, right int) bool { return merged[left].ID < merged[right].ID })
	return merged, appended
}
