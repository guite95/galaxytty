package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestConversationViewIsWhitespaceFirst(t *testing.T) {
	model, _ := fixture(t)
	model.width = 72
	model.height = 22
	plain := ansi.Strip(model.View())
	if strings.ContainsAny(plain, "│─←→") {
		t.Fatalf("box-heavy glyph found: %q", plain)
	}
	for _, expected := range []string{"GalaxyTTY", "Messages", "김형주", "↑↓ select", "connected"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("missing %q in %q", expected, plain)
		}
	}
}

func TestChatViewWrapsWideTextWithoutOverflow(t *testing.T) {
	model, _ := fixture(t)
	model = open(t, model)
	model.width = 48
	model.height = 24
	model.messages = []domain.Message{
		{ID: 1, ThreadID: 1, Body: "받은 한글 메시지와 emoji 😀가 충분히 길어서 여러 줄로 보여야 합니다", Direction: domain.DirectionIncoming},
		{ID: 2, ThreadID: 1, Body: "보낸 답장도 terminal display cell width로 오른쪽 정렬됩니다 😀", Direction: domain.DirectionOutgoing},
	}
	rendered := model.View()
	plain := ansi.Strip(rendered)
	if strings.ContainsAny(plain, "│─←→") {
		t.Fatalf("box-heavy glyph found: %q", plain)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got > model.width {
			t.Fatalf("line width=%d want<=%d line=%q", got, model.width, line)
		}
	}
	if !strings.Contains(plain, "esc back") || !strings.Contains(plain, "메시지 입력") {
		t.Fatalf("missing chat controls: %q", plain)
	}
}
