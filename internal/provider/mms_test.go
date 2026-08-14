package provider

import (
	"context"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestLatestMMSMessageIDUsesSingleRowQuery(t *testing.T) {
	shell := &fakeShell{output: []byte("Row: 0 _id=42\n")}
	id, err := NewStore(shell).LatestMMSMessageID(context.Background())
	if err != nil || id != 42 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if got := queryArg(shell.args, "--where"); got != "\"msg_box = 2 AND m_type = 128\"" {
		t.Fatalf("where=%q", got)
	}
	if got := queryArg(shell.args, "--sort"); got != "\"_id DESC LIMIT 1\"" {
		t.Fatalf("sort=%q", got)
	}
}

func TestMMSMessagesAfterMapsExactOutgoingTextEvidence(t *testing.T) {
	shell := &queuedShell{outputs: [][]byte{
		[]byte("Row: 0 _id=43, thread_id=7, date=1700000000, msg_box=2, m_type=128\n" +
			"Row: 1 _id=44, thread_id=7, date=1700000001, msg_box=1, m_type=132\n"),
		[]byte("Row: 0 type=151, address=+82 10-1234-5678\n"),
		[]byte("Row: 0 _id=9, mid=43, ct=text/plain, text=한글, equals = 😀\n둘째 줄\n"),
	}}
	messages, err := NewStore(shell).MMSMessagesAfter(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages=%+v", messages)
	}
	message := messages[0]
	if message.ID != 43 || message.ThreadID != 7 || message.Address != "+82 10-1234-5678" || message.Body != "한글, equals = 😀\n둘째 줄" || message.Direction != domain.DirectionOutgoing || message.Type != domain.MessageMMS {
		t.Fatalf("message=%+v", message)
	}
	if !message.Timestamp.Equal(time.Unix(1700000000, 0)) {
		t.Fatalf("timestamp=%v", message.Timestamp)
	}
	if got := queryArg(shell.calls[0], "--where"); got != "\"_id > 42 AND msg_box = 2 AND m_type = 128\"" {
		t.Fatalf("message where=%q", got)
	}
	if got := queryArg(shell.calls[1], "--where"); got != "\"type = 151\"" {
		t.Fatalf("address where=%q", got)
	}
	if got := queryArg(shell.calls[2], "--where"); got != "\"mid = 43 AND ct = 'text/plain'\"" {
		t.Fatalf("part where=%q", got)
	}
}

func TestMMSMessagesAfterWaitsForUnambiguousRecipientAndTextPart(t *testing.T) {
	for _, tc := range []struct {
		name      string
		addresses string
		parts     string
	}{
		{
			name:      "missing recipient",
			addresses: "No result found.\n",
			parts:     "Row: 0 _id=9, mid=43, ct=text/plain, text=body\n",
		},
		{
			name:      "multiple recipients",
			addresses: "Row: 0 type=151, address=01000000000\nRow: 1 type=151, address=01011111111\n",
			parts:     "Row: 0 _id=9, mid=43, ct=text/plain, text=body\n",
		},
		{
			name:      "multiple text parts",
			addresses: "Row: 0 type=151, address=01000000000\n",
			parts:     "Row: 0 _id=9, mid=43, ct=text/plain, text=one\nRow: 1 _id=10, mid=43, ct=text/plain, text=two\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shell := &queuedShell{outputs: [][]byte{
				[]byte("Row: 0 _id=43, thread_id=7, date=1700000000, msg_box=2, m_type=128\n"),
				[]byte(tc.addresses),
				[]byte(tc.parts),
			}}
			messages, err := NewStore(shell).MMSMessagesAfter(context.Background(), 42)
			if err != nil || len(messages) != 0 {
				t.Fatalf("messages=%+v err=%v", messages, err)
			}
		})
	}
}

func TestMMSQueriesRejectNegativeBaseline(t *testing.T) {
	if _, err := NewStore(&fakeShell{}).MMSMessagesAfter(context.Background(), -1); err == nil {
		t.Fatal("expected invalid MMS baseline error")
	}
}
