package mock

import (
	"context"
	"testing"
)

func TestSendStoresMessageInMatchingParticipantThread(t *testing.T) {
	backend := New()
	phone := backend.ConversationsData[1].Participants[0].Phone
	if err := backend.Send(context.Background(), phone, "synthetic hello"); err != nil {
		t.Fatal(err)
	}
	if got := backend.Sent[len(backend.Sent)-1].ThreadID; got != 2 {
		t.Fatalf("ThreadID = %d, want 2", got)
	}
}
