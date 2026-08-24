package views

import (
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/tui/components"
)

type ConversationsProps struct {
	Theme         components.Theme
	Width, Height int
	Device        string
	State         string
	Items         []domain.Conversation
	Cursor        int
	Input         string
	Error         string
	Now           time.Time
}

func Conversations(props ConversationsProps) string {
	top := []string{
		components.Header(props.Theme, props.Width, props.Device, props.State),
		"",
		props.Theme.Heading.Render("Messages"),
		"",
	}
	bottom := conversationFooter(props)
	available := max(0, props.Height-len(top)-len(bottom))
	visible := max(1, available/3)
	start := max(0, props.Cursor-visible/2)
	end := min(len(props.Items), start+visible)
	start = max(0, end-visible)

	body := make([]string, 0, visible*3)
	for index := start; index < end; index++ {
		item := props.Items[index]
		name := components.Truncate(item.Title, max(1, props.Width-2))
		if index == props.Cursor {
			name = props.Theme.Accent.Render(name)
		} else {
			name = props.Theme.Text.Render(name)
		}
		body = append(body, "  "+name)

		meta := components.RelativeTime(props.Now, item.UpdatedAt)
		if item.UnreadCount > 0 {
			if meta != "" {
				meta += "   "
			}
			meta += props.Theme.Accent.Render("●")
		}
		snippet := props.Theme.Muted.Render(components.Truncate(singleLine(item.Snippet), max(1, props.Width-components.CellWidth(meta)-5)))
		body = append(body, components.Columns("  "+snippet, meta, props.Width), "")
	}
	if len(props.Items) == 0 {
		body = append(body, props.Theme.Muted.Render("  No conversations yet"))
	}
	return frame(top, body, bottom, props.Width, props.Height)
}

func conversationFooter(props ConversationsProps) []string {
	bottom := make([]string, 0, 3)
	if value := strings.TrimSpace(props.Input); value != "" {
		bottom = append(bottom, components.Truncate("› "+value, props.Width))
	}
	if line := components.ErrorLine(props.Theme, props.Error, props.Width); line != "" {
		bottom = append(bottom, line)
	}
	help := props.Theme.Muted.Render("↑↓ select   enter open   /search")
	bottom = append(bottom, components.Columns(help, components.ConnectionStatus(props.Theme, props.State), props.Width))
	return bottom
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
