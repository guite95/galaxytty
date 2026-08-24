package tui

import (
	"time"

	"github.com/galaxytty/galaxytty/internal/tui/views"
)

func (m Model) View() string {
	width := max(30, m.width)
	height := max(10, m.height)
	state := m.connectionState
	if state == "" {
		state = m.status
	}
	device := m.deviceName
	if device == "" && m.status == "Mock Connected" {
		device = "Mock Galaxy"
	}
	if m.screen == chatScreen && len(m.conversations) > 0 {
		return views.Chat(views.ChatProps{
			Theme:        defaultTheme,
			Width:        width,
			Height:       height,
			Device:       device,
			State:        state,
			Conversation: m.conversations[m.cursor],
			Messages:     m.messages,
			Offset:       m.chatOffset,
			Composer:     m.composer.View(),
			Notice:       m.noticeText,
			Error:        m.errorText,
			LoadingOlder: m.loadingOlder,
		})
	}
	return views.Conversations(views.ConversationsProps{
		Theme:  defaultTheme,
		Width:  width,
		Height: height,
		Device: device,
		State:  state,
		Items:  m.conversations,
		Cursor: m.cursor,
		Input:  m.composer.Value(),
		Error:  m.errorText,
		Now:    time.Now(),
	})
}

func (m Model) renderConversations(width, height int) string {
	return views.Conversations(views.ConversationsProps{
		Theme: defaultTheme, Width: width, Height: height,
		Items: m.conversations, Cursor: m.cursor, Input: m.composer.Value(), Now: time.Now(),
	})
}

func (m Model) renderChat(width, height int) string {
	if len(m.conversations) == 0 {
		return ""
	}
	return views.Chat(views.ChatProps{
		Theme: defaultTheme, Width: width, Height: height,
		Conversation: m.conversations[m.cursor], Messages: m.messages,
		Offset: m.chatOffset, Composer: m.composer.View(), LoadingOlder: m.loadingOlder,
	})
}
