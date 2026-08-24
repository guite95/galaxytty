package remote

import (
	"context"
	"errors"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/protocol"
)

type fakeRequester struct {
	responses []protocol.Envelope
	types     []protocol.Type
	payloads  []any
}

func (requester *fakeRequester) Request(_ context.Context, messageType protocol.Type, payload any) (protocol.Envelope, error) {
	requester.types = append(requester.types, messageType)
	requester.payloads = append(requester.payloads, payload)
	if len(requester.responses) == 0 {
		return protocol.Envelope{}, errors.New("missing fake response")
	}
	response := requester.responses[0]
	requester.responses = requester.responses[1:]
	return response, nil
}

func TestRemoteStoreMapsProtocolDTOsToDomain(t *testing.T) {
	conversations := envelope(t, protocol.TypeConversations, map[string]any{
		"items": []any{map[string]any{
			"threadId": 9, "title": "장욱", "snippet": "안녕", "updatedAt": 1_777_000_000_000,
			"unreadCount": 1, "participants": []any{map[string]any{"id": 2, "displayName": "장욱", "phone": "01012345678"}},
		}},
	})
	messages := envelope(t, protocol.TypeMessages, map[string]any{
		"items": []any{map[string]any{
			"id": 11, "threadId": 9, "body": "채팅+ 😀", "postedAt": 1_777_000_000_000,
			"direction": "incoming", "messageType": "unknown", "read": false,
		}},
	})
	requester := &fakeRequester{responses: []protocol.Envelope{conversations, messages}}
	store := NewStore(requester)

	items, err := store.Conversations(context.Background())
	if err != nil || len(items) != 1 || items[0].ThreadID != 9 || items[0].Participants[0].Phone != "01012345678" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	history, err := store.Messages(context.Background(), 9, domain.MessageQuery{Limit: 50, BeforeID: 20})
	if err != nil || len(history) != 1 || history[0].Type != domain.MessageUnknown || history[0].Direction != domain.DirectionIncoming {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if requester.types[0] != protocol.TypeGetConversations || requester.types[1] != protocol.TypeGetMessages {
		t.Fatalf("types=%v", requester.types)
	}
	query := requester.payloads[1].(getMessagesPayload)
	if query.ThreadID != 9 || query.Limit != 50 || query.BeforeID != 20 {
		t.Fatalf("query=%+v", query)
	}
}

func TestRemoteStoreMapsSMSProviderHistoryDTO(t *testing.T) {
	response := envelope(t, protocol.TypeMessages, map[string]any{
		"items": []any{map[string]any{
			"id": 12, "threadId": 9, "address": "01012345678", "body": "sent text",
			"postedAt": 1_777_000_000_000, "direction": "outgoing", "messageType": "sms", "read": true,
		}},
	})
	store := NewStore(&fakeRequester{responses: []protocol.Envelope{response}})

	history, err := store.Messages(context.Background(), 9, domain.MessageQuery{})
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	message := history[0]
	if message.Type != domain.MessageSMS || message.Direction != domain.DirectionOutgoing || !message.Read || message.Address != "01012345678" {
		t.Fatalf("message=%+v", message)
	}
}

func TestRemoteSenderDistinguishesAcceptedAndVerifiedOutcomes(t *testing.T) {
	requester := &fakeRequester{responses: []protocol.Envelope{
		envelope(t, protocol.TypeSendResult, map[string]any{"outcome": "accepted_unverified", "evidence": "remote_input_accepted", "threadId": 7}),
		envelope(t, protocol.TypeSendResult, map[string]any{"outcome": "accepted_unverified", "evidence": "remote_input_accepted", "threadId": 7}),
		envelope(t, protocol.TypeSendResult, map[string]any{"outcome": "verified", "evidence": "outgoing_provider", "messageId": 41, "threadId": 7}),
	}}
	sender := NewSender(requester)
	accepted, err := sender.Send(context.Background(), "01012345678", "synthetic")
	if err != nil || accepted.Outcome != domain.SendOutcomeAcceptedUnverified || accepted.ThreadID != 7 || accepted.Evidence != "remote_input_accepted" {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	accepted, err = sender.SendToConversation(context.Background(), 7, "thread reply")
	if err != nil || accepted.Outcome != domain.SendOutcomeAcceptedUnverified {
		t.Fatalf("accepted reply=%+v err=%v", accepted, err)
	}
	result, err := sender.Send(context.Background(), "01012345678", "synthetic")
	if err != nil || result != (domain.SendResult{
		MessageID: 41,
		ThreadID:  7,
		Outcome:   domain.SendOutcomeVerified,
		Evidence:  "outgoing_provider",
	}) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if requester.types[0] != protocol.TypeSendMessage || requester.types[1] != protocol.TypeSendReply {
		t.Fatalf("types=%v", requester.types)
	}
	reply := requester.payloads[1].(sendReplyPayload)
	if reply.ThreadID != 7 || reply.Text != "thread reply" {
		t.Fatalf("reply=%+v", reply)
	}
}

func envelope(t *testing.T, messageType protocol.Type, payload any) protocol.Envelope {
	t.Helper()
	envelope, err := protocol.NewEnvelope(messageType, "request", 0, payload)
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}
