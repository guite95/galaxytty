package views

import (
	"strings"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestChatMetadataDoesNotGuessUnknownNotificationTransport(t *testing.T) {
	conversation := domain.Conversation{Title: "대화"}
	unknown := chatMetadata(conversation, []domain.Message{{Type: domain.MessageUnknown}})
	if unknown != "Samsung Messages" {
		t.Fatalf("unknown metadata=%q", unknown)
	}
	rcs := chatMetadata(conversation, []domain.Message{{Type: domain.MessageUnknown}, {Type: domain.MessageRCS}})
	if !strings.Contains(rcs, "채팅+") {
		t.Fatalf("rcs metadata=%q", rcs)
	}
}
