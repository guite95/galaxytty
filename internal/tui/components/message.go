package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func Message(theme Theme, message domain.Message, width int) string {
	if width <= 0 {
		return ""
	}
	body := strings.TrimSpace(message.Body)
	if body == "" && len(message.Attachments) > 0 {
		body = attachmentLabel(message.Attachments)
	}
	if body == "" {
		body = "Empty message"
	}

	bodyWidth := min(width, max(8, width*4/5))
	written := Wrap(body, bodyWidth)
	lines := strings.Split(written, "\n")
	for index, line := range lines {
		line = theme.Text.Render(line)
		if message.Direction == domain.DirectionOutgoing {
			line = theme.Outgoing.Render(line)
			line = AlignRight(line, width)
		}
		lines[index] = line
	}

	if !message.Timestamp.IsZero() {
		metadata := theme.Muted.Render(message.Timestamp.Format("15:04"))
		if message.Direction == domain.DirectionOutgoing {
			metadata = AlignRight(metadata, width)
		}
		lines = append(lines, metadata)
	}
	return strings.Join(lines, "\n")
}

func attachmentLabel(attachments []domain.Attachment) string {
	if len(attachments) == 1 {
		if strings.HasPrefix(attachments[0].MIMEType, "image/") {
			return "Image"
		}
		return "Attachment"
	}
	return fmt.Sprintf("%d attachments", len(attachments))
}

func RelativeTime(now, value time.Time) string {
	if value.IsZero() {
		return ""
	}
	delta := now.Sub(value)
	if delta < 0 {
		delta = 0
	}
	switch {
	case delta < time.Minute:
		return "now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm", int(delta/time.Minute))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh", int(delta/time.Hour))
	case delta < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(delta/(24*time.Hour)))
	default:
		return value.Format("Jan 2")
	}
}
