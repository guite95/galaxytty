package views

import (
	"strings"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/tui/components"
)

type ChatProps struct {
	Theme         components.Theme
	Width, Height int
	Device        string
	State         string
	Conversation  domain.Conversation
	Messages      []domain.Message
	Offset        int
	Composer      string
	Notice        string
	Error         string
	LoadingOlder  bool
}

func Chat(props ChatProps) string {
	metadata := chatMetadata(props.Conversation, props.Messages)
	top := []string{
		components.Header(props.Theme, props.Width, props.Device, props.State),
		"",
		props.Theme.Heading.Render(components.Truncate(props.Conversation.Title, props.Width)),
		props.Theme.Muted.Render(components.Truncate(metadata, props.Width)),
		"",
	}
	bottom := chatFooter(props)
	available := max(0, props.Height-len(top)-len(bottom))
	body := visibleMessages(props, available)
	return frame(top, body, bottom, props.Width, props.Height)
}

func chatFooter(props ChatProps) []string {
	composer := strings.ReplaceAll(props.Composer, "\x00", "")
	bottom := []string{components.Clip(composer, props.Width)}
	if line := components.NoticeLine(props.Theme, props.Notice, props.Width); line != "" {
		bottom = append(bottom, line)
	}
	if line := components.ErrorLine(props.Theme, props.Error, props.Width); line != "" {
		bottom = append(bottom, line)
	}
	help := props.Theme.Muted.Render("esc back   pgup older   /search")
	bottom = append(bottom, components.Columns(help, components.ConnectionStatus(props.Theme, props.State), props.Width))
	return bottom
}

func visibleMessages(props ChatProps, height int) []string {
	if height <= 0 {
		return nil
	}
	end := len(props.Messages) - max(0, props.Offset)
	end = min(len(props.Messages), max(0, end))
	blocks := make([][]string, 0)
	used := 0
	for index := end - 1; index >= 0; index-- {
		block := strings.Split(components.Message(props.Theme, props.Messages[index], props.Width), "\n")
		block = append(block, "")
		if used+len(block) > height && len(blocks) > 0 {
			break
		}
		if len(block) > height {
			block = block[len(block)-height:]
		}
		blocks = append(blocks, block)
		used += len(block)
		if used >= height {
			break
		}
	}

	lines := make([]string, 0, min(used, height))
	for index := len(blocks) - 1; index >= 0; index-- {
		lines = append(lines, blocks[index]...)
	}
	if props.LoadingOlder && len(lines) < height {
		lines = append([]string{props.Theme.Muted.Render("Loading older messages…"), ""}, lines...)
	}
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	return lines
}

func chatMetadata(conversation domain.Conversation, messages []domain.Message) string {
	parts := make([]string, 0, 2)
	if len(conversation.Participants) == 1 {
		parts = append(parts, formatPhone(conversation.Participants[0].Phone))
	}
	transport := "Samsung Messages"
	for _, message := range messages {
		switch message.Type {
		case domain.MessageRCS:
			transport = "채팅+"
		case domain.MessageMMS:
			if transport != "채팅+" {
				transport = "MMS"
			}
		case domain.MessageSMS:
			if transport != "채팅+" && transport != "MMS" {
				transport = "문자"
			}
		}
	}
	parts = append(parts, transport)
	return strings.Join(parts, " · ")
}

func formatPhone(value string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	if len(digits) == 11 && strings.HasPrefix(digits, "010") {
		return digits[:3] + "-" + digits[3:7] + "-" + digits[7:]
	}
	return value
}
