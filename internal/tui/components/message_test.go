package components

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestMessageUsesWhitespaceForDirectionAndWraps(t *testing.T) {
	theme := DefaultTheme()
	message := domain.Message{
		Body:      "한글과 emoji 😀가 함께 있는 매우 긴 outgoing message",
		Timestamp: time.Date(2026, time.August, 24, 12, 42, 0, 0, time.Local),
		Direction: domain.DirectionOutgoing,
	}
	rendered := Message(theme, message, 32)
	plain := ansi.Strip(rendered)
	if strings.ContainsAny(plain, "←→│─") {
		t.Fatalf("directional or box glyph leaked into message: %q", plain)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if got := CellWidth(line); got > 32 {
			t.Fatalf("line width=%d line=%q", got, line)
		}
	}
	if first := strings.Split(plain, "\n")[0]; !strings.HasPrefix(first, " ") {
		t.Fatalf("outgoing message is not right aligned: %q", first)
	}
}

func TestIncomingMessageStaysLeftAligned(t *testing.T) {
	rendered := ansi.Strip(Message(DefaultTheme(), domain.Message{
		Body:      "안녕하세요",
		Timestamp: time.Date(2026, time.August, 24, 12, 41, 0, 0, time.Local),
		Direction: domain.DirectionIncoming,
	}, 32))
	if strings.HasPrefix(rendered, " ") {
		t.Fatalf("incoming message was indented: %q", rendered)
	}
	if !strings.Contains(rendered, "12:41") {
		t.Fatalf("missing muted timestamp: %q", rendered)
	}
}

func TestAcceptedUnverifiedOutgoingMessageShowsHonestState(t *testing.T) {
	rendered := ansi.Strip(Message(DefaultTheme(), domain.Message{
		Body:        "보낸 RCS 답장",
		Timestamp:   time.Date(2026, time.August, 24, 17, 0, 0, 0, time.Local),
		Direction:   domain.DirectionOutgoing,
		SendOutcome: domain.SendOutcomeAcceptedUnverified,
	}, 48))
	if !strings.Contains(rendered, "17:00  전송 요청됨 · 미검증") {
		t.Fatalf("missing unverified metadata: %q", rendered)
	}
	if strings.Contains(rendered, "전송 완료") || strings.Contains(rendered, "✓") {
		t.Fatalf("message falsely claimed delivery: %q", rendered)
	}
}
