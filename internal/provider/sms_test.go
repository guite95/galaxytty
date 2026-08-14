package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const syntheticSMSRows = "Row: 0 _id=8, thread_id=2, address=01000000000, date=1700000000000, type=2, read=1, body=second\n" +
	"Row: 1 _id=7, thread_id=2, address=01000000000, date=1699999999000, type=1, read=0, body=first, with = text\n"

func TestMessagesBuildsBoundedHistoryQueryAndReturnsOldestFirst(t *testing.T) {
	shell := &fakeShell{output: []byte(syntheticSMSRows)}
	store := NewStore(shell)
	messages, err := store.Messages(context.Background(), 2, domain.MessageQuery{BeforeID: 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != 7 || messages[1].ID != 8 {
		t.Fatalf("messages=%+v", messages)
	}
	if messages[0].Direction != domain.DirectionIncoming || messages[1].Direction != domain.DirectionOutgoing {
		t.Fatalf("directions=%v,%v", messages[0].Direction, messages[1].Direction)
	}
	if messages[0].Read || !messages[1].Read || messages[0].Type != domain.MessageSMS {
		t.Fatalf("mapped=%+v %+v", messages[0], messages[1])
	}
	if !messages[1].Timestamp.Equal(time.UnixMilli(1700000000000)) {
		t.Fatalf("timestamp=%v", messages[1].Timestamp)
	}
	if got := queryArg(shell.args, "--where"); got != "\"thread_id = 2 AND type IN (1,2) AND _id < 9\"" {
		t.Fatalf("where=%q", got)
	}
	if got := queryArg(shell.args, "--sort"); got != "\"_id DESC LIMIT 200\"" {
		t.Fatalf("sort=%q", got)
	}
}

func TestMessagesCapsRequestedLimit(t *testing.T) {
	shell := &fakeShell{output: []byte("No result found.\n")}
	store := NewStore(shell)
	if _, err := store.Messages(context.Background(), 2, domain.MessageQuery{Limit: 5000}); err != nil {
		t.Fatal(err)
	}
	if got := queryArg(shell.args, "--sort"); got != "\"_id DESC LIMIT 1000\"" {
		t.Fatalf("sort=%q", got)
	}
}

func TestLatestMessageIDUsesSingleRowQuery(t *testing.T) {
	shell := &fakeShell{output: []byte("Row: 0 _id=42\n")}
	store := NewStore(shell)
	id, err := store.LatestMessageID(context.Background())
	if err != nil || id != 42 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if got := queryArg(shell.args, "--sort"); got != "\"_id DESC LIMIT 1\"" {
		t.Fatalf("sort=%q", got)
	}
	if got := queryArg(shell.args, "--where"); got != "\"type IN (1,2)\"" {
		t.Fatalf("where=%q", got)
	}
}

func TestLatestMessageIDReturnsZeroForNoRows(t *testing.T) {
	store := NewStore(&fakeShell{output: []byte("No result found.\n")})
	id, err := store.LatestMessageID(context.Background())
	if err != nil || id != 0 {
		t.Fatalf("id=%d err=%v", id, err)
	}
}

func TestMessagesAfterUsesAscendingIncrementalQuery(t *testing.T) {
	rows := strings.Replace(syntheticSMSRows, "Row: 0 _id=8", "Row: 0 _id=7", 1)
	rows = strings.Replace(rows, "Row: 1 _id=7", "Row: 1 _id=8", 1)
	shell := &fakeShell{output: []byte(rows)}
	store := NewStore(shell)
	messages, err := store.MessagesAfter(context.Background(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != 7 || messages[1].ID != 8 {
		t.Fatalf("messages=%+v", messages)
	}
	if got := queryArg(shell.args, "--where"); got != "\"_id > 6 AND type IN (1,2)\"" {
		t.Fatalf("where=%q", got)
	}
	if got := queryArg(shell.args, "--sort"); got != "\"_id ASC LIMIT 500\"" {
		t.Fatalf("sort=%q", got)
	}
}

func TestSMSMappingSkipsKnownNonHistoryAndUnknownTypes(t *testing.T) {
	rows := []map[string]string{
		smsRow("10", "1", "0"),
		smsRow("bad-draft-id", "3", "bad-read"),
		smsRow("bad-outbox-id", "4", "bad-read"),
		smsRow("bad-failed-id", "5", "bad-read"),
		smsRow("bad-queued-id", "6", "bad-read"),
		smsRow("bad-unknown-id", "99", "bad-read"),
		smsRow("16", "2", "1"),
	}
	messages, err := mapSMSRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != 10 || messages[0].Direction != domain.DirectionIncoming || messages[1].ID != 16 || messages[1].Direction != domain.DirectionOutgoing {
		t.Fatalf("messages=%+v", messages)
	}
}

func TestSMSMappingRejectsMalformedSupportedFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  map[string]string
	}{
		{name: "id", row: smsRow("bad", "1", "1")},
		{name: "read", row: smsRow("1", "1", "2")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := mapSMSRow(tc.row); err == nil {
				t.Fatal("expected mapping error")
			}
		})
	}
}

func TestSMSQueriesRejectNegativeOrMissingIDs(t *testing.T) {
	store := NewStore(&fakeShell{output: []byte("No result found.\n")})
	for _, call := range []func() error{
		func() error { _, err := store.Messages(context.Background(), 0, domain.MessageQuery{}); return err },
		func() error {
			_, err := store.Messages(context.Background(), 1, domain.MessageQuery{BeforeID: -1})
			return err
		},
		func() error {
			_, err := store.Messages(context.Background(), 1, domain.MessageQuery{Limit: -1})
			return err
		},
		func() error { _, err := store.MessagesAfter(context.Background(), -1); return err },
	} {
		if err := call(); err == nil {
			t.Fatal("expected invalid query error")
		}
	}
}

func smsRow(id, messageType, read string) map[string]string {
	return map[string]string{
		"_id": id, "thread_id": "2", "address": "01000000000",
		"date": "1700000000000", "type": messageType, "read": read, "body": "synthetic",
	}
}

func queryArg(args []string, name string) string {
	for i := range args {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
