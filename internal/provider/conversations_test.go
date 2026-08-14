package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type queuedShell struct {
	outputs [][]byte
	errors  []error
	calls   [][]string
	index   int
}

func (q *queuedShell) Shell(_ context.Context, args ...string) ([]byte, error) {
	q.calls = append(q.calls, append([]string(nil), args...))
	if q.index >= len(q.outputs) {
		return nil, errors.New("unexpected provider query")
	}
	output := q.outputs[q.index]
	var err error
	if q.index < len(q.errors) {
		err = q.errors[q.index]
	}
	q.index++
	return output, err
}

func TestParseRecipientIDsUsesWhitespaceAndRejectsMalformed(t *testing.T) {
	ids, err := parseRecipientIDs(" 12  34 ")
	if err != nil || len(ids) != 2 || ids[0] != 12 || ids[1] != 34 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if _, err := parseRecipientIDs("12 nope"); err == nil {
		t.Fatal("expected malformed recipient ID")
	}
}

func TestConversationsMapsGroupContactsAndFallback(t *testing.T) {
	shell := &queuedShell{outputs: [][]byte{
		[]byte("Row: 0 _id=5, recipient_ids=12 34, unread_count=2, date=1700000000000, snippet=synthetic snippet\n"),
		[]byte("Row: 0 _id=12, address=010-1234-5678\nRow: 1 _id=34, address=1588-0000\n"),
		[]byte("Row: 0 data1=01012345678, data4=+821012345678, display_name=Synthetic Person\n"),
	}}
	store := NewStore(shell)
	conversations, err := store.Conversations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 {
		t.Fatalf("conversations=%+v", conversations)
	}
	conversation := conversations[0]
	if conversation.ThreadID != 5 || conversation.Title != "Synthetic Person, 1588-0000" || conversation.UnreadCount != 2 || conversation.Snippet != "synthetic snippet" {
		t.Fatalf("conversation=%+v", conversation)
	}
	if !conversation.UpdatedAt.Equal(time.UnixMilli(1700000000000)) {
		t.Fatalf("updated=%v", conversation.UpdatedAt)
	}
	if len(conversation.Participants) != 2 || conversation.Participants[0].Phone != "010-1234-5678" || conversation.Participants[1].DisplayName != "1588-0000" {
		t.Fatalf("participants=%+v", conversation.Participants)
	}
	if got := queryArg(shell.calls[0], "--sort"); got != "\"date DESC\"" {
		t.Fatalf("conversation sort=%q", got)
	}
}

func TestConversationsUsesUnknownParticipantForMissingCanonical(t *testing.T) {
	shell := &queuedShell{outputs: [][]byte{
		[]byte("Row: 0 _id=5, recipient_ids=99, unread_count=0, date=1700000000000, snippet=synthetic\n"),
		[]byte("No result found.\n"),
		[]byte("No result found.\n"),
	}}
	conversations, err := NewStore(shell).Conversations(context.Background())
	if err != nil || conversations[0].Title != "Unknown participant" || conversations[0].Participants[0].ID != 99 {
		t.Fatalf("conversations=%+v err=%v", conversations, err)
	}
}

func TestConversationsRetriesContactLoadAfterErrorAndCachesSuccess(t *testing.T) {
	conversation := []byte("Row: 0 _id=5, recipient_ids=12, unread_count=0, date=1700000000000, snippet=synthetic\n")
	canonical := []byte("Row: 0 _id=12, address=01012345678\n")
	contacts := []byte("Row: 0 data1=01012345678, data4=+821012345678, display_name=Synthetic Person\n")
	shell := &queuedShell{
		outputs: [][]byte{conversation, canonical, nil, conversation, canonical, contacts, conversation, canonical},
		errors:  []error{nil, nil, errors.New("temporary ADB error")},
	}
	store := NewStore(shell)
	if _, err := store.Conversations(context.Background()); err == nil || !strings.Contains(err.Error(), "temporary ADB error") {
		t.Fatalf("first err=%v", err)
	}
	if _, err := store.Conversations(context.Background()); err != nil {
		t.Fatalf("retry err=%v", err)
	}
	if _, err := store.Conversations(context.Background()); err != nil {
		t.Fatalf("cached err=%v", err)
	}
	if shell.index != 8 {
		t.Fatalf("queries=%d want=8", shell.index)
	}
}
