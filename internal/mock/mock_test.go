package mock

import (
	"context"
	"testing"
)

func TestSendStoresMessageInMatchingParticipantThread(t *testing.T) {
	backend := New()
	phone := backend.ConversationsData[1].Participants[0].Phone
	result, err := backend.Send(context.Background(), phone, "synthetic hello")
	if err != nil {
		t.Fatal(err)
	}
	if got := backend.Sent[len(backend.Sent)-1].ThreadID; got != 2 {
		t.Fatalf("ThreadID = %d, want 2", got)
	}
	if result.MessageID != backend.Sent[len(backend.Sent)-1].ID || result.ThreadID != 2 {
		t.Fatalf("result=%+v", result)
	}
}
