package remote

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/protocol"
)

var (
	ErrUnexpectedResponse = errors.New("unexpected Galaxy Helper response")
)

type Requester interface {
	Request(context.Context, protocol.Type, any) (protocol.Envelope, error)
}

type Store struct{ client Requester }
type Sender struct{ client Requester }

func NewStore(client Requester) *Store   { return &Store{client: client} }
func NewSender(client Requester) *Sender { return &Sender{client: client} }

func (store *Store) Conversations(ctx context.Context) ([]domain.Conversation, error) {
	response, err := store.client.Request(ctx, protocol.TypeGetConversations, map[string]any{})
	if err != nil {
		return nil, err
	}
	if err := expectType(response, protocol.TypeConversations); err != nil {
		return nil, err
	}
	var payload conversationsPayload
	if err := response.DecodePayload(&payload); err != nil {
		return nil, err
	}
	items := make([]domain.Conversation, 0, len(payload.Items))
	for _, item := range payload.Items {
		items = append(items, item.domain())
	}
	return items, nil
}

func (store *Store) Messages(ctx context.Context, threadID int64, query domain.MessageQuery) ([]domain.Message, error) {
	return store.messages(ctx, getMessagesPayload{
		ThreadID: threadID,
		Limit:    query.Limit,
		BeforeID: query.BeforeID,
	})
}

func (store *Store) MessagesAfter(ctx context.Context, afterID int64) ([]domain.Message, error) {
	return store.messages(ctx, getMessagesPayload{AfterID: afterID})
}

func (store *Store) LatestMessageID(ctx context.Context) (int64, error) {
	messages, err := store.messages(ctx, getMessagesPayload{Limit: 1, Latest: true})
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, message := range messages {
		latest = max(latest, message.ID)
	}
	return latest, nil
}

func (store *Store) messages(ctx context.Context, request getMessagesPayload) ([]domain.Message, error) {
	response, err := store.client.Request(ctx, protocol.TypeGetMessages, request)
	if err != nil {
		return nil, err
	}
	if err := expectType(response, protocol.TypeMessages); err != nil {
		return nil, err
	}
	var payload messagesPayload
	if err := response.DecodePayload(&payload); err != nil {
		return nil, err
	}
	items := make([]domain.Message, 0, len(payload.Items))
	for _, item := range payload.Items {
		message, err := item.domain()
		if err != nil {
			return nil, err
		}
		items = append(items, message)
	}
	return items, nil
}

func (sender *Sender) Send(ctx context.Context, address, text string) (domain.SendResult, error) {
	return sender.send(ctx, protocol.TypeSendMessage, sendMessagePayload{
		Address: address,
		Text:    text,
	})
}

func (sender *Sender) SendToConversation(ctx context.Context, threadID int64, text string) (domain.SendResult, error) {
	return sender.send(ctx, protocol.TypeSendReply, sendReplyPayload{
		ThreadID: threadID,
		Text:     text,
	})
}

func (sender *Sender) send(ctx context.Context, messageType protocol.Type, requestPayload any) (domain.SendResult, error) {
	response, err := sender.client.Request(ctx, messageType, requestPayload)
	if err != nil {
		return domain.SendResult{}, err
	}
	if err := expectType(response, protocol.TypeSendResult); err != nil {
		return domain.SendResult{}, err
	}
	var resultPayload sendResultPayload
	if err := response.DecodePayload(&resultPayload); err != nil {
		return domain.SendResult{}, err
	}
	switch resultPayload.Outcome {
	case "verified":
		return domain.SendResult{
			MessageID: resultPayload.MessageID,
			ThreadID:  resultPayload.ThreadID,
			Outcome:   domain.SendOutcomeVerified,
			Evidence:  resultPayload.Evidence,
		}, nil
	case "accepted_unverified":
		return domain.SendResult{
			ThreadID: resultPayload.ThreadID,
			Outcome:  domain.SendOutcomeAcceptedUnverified,
			Evidence: resultPayload.Evidence,
		}, nil
	default:
		if strings.TrimSpace(resultPayload.Error) == "" {
			resultPayload.Error = "Galaxy Helper reported send failure"
		}
		return domain.SendResult{}, errors.New(resultPayload.Error)
	}
}

func expectType(response protocol.Envelope, want protocol.Type) error {
	if response.Type == protocol.TypeError {
		var payload protocol.ErrorPayload
		if err := response.DecodePayload(&payload); err != nil {
			return err
		}
		return fmt.Errorf("Galaxy Helper %s: %s", payload.Code, payload.Message)
	}
	if response.Type != want {
		return fmt.Errorf("%w: got %s want %s", ErrUnexpectedResponse, response.Type, want)
	}
	return nil
}

type conversationsPayload struct {
	Items []conversationDTO `json:"items"`
}

type conversationDTO struct {
	ThreadID     int64        `json:"threadId"`
	Title        string       `json:"title"`
	Participants []contactDTO `json:"participants"`
	Snippet      string       `json:"snippet"`
	UpdatedAt    int64        `json:"updatedAt"`
	UnreadCount  int          `json:"unreadCount"`
}

type contactDTO struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
}

func (item conversationDTO) domain() domain.Conversation {
	participants := make([]domain.Contact, 0, len(item.Participants))
	for _, participant := range item.Participants {
		participants = append(participants, domain.Contact{
			ID: participant.ID, DisplayName: participant.DisplayName, Phone: participant.Phone,
		})
	}
	return domain.Conversation{
		ThreadID: item.ThreadID, Title: item.Title, Participants: participants,
		Snippet: item.Snippet, UpdatedAt: unixMillis(item.UpdatedAt), UnreadCount: item.UnreadCount,
	}
}

type getMessagesPayload struct {
	ThreadID int64 `json:"threadId,omitempty"`
	Limit    int   `json:"limit,omitempty"`
	BeforeID int64 `json:"beforeId,omitempty"`
	AfterID  int64 `json:"afterId,omitempty"`
	Latest   bool  `json:"latest,omitempty"`
}

type messagesPayload struct {
	Items []messageDTO `json:"items"`
}

type messageDTO struct {
	ID          int64           `json:"id"`
	ThreadID    int64           `json:"threadId"`
	Address     string          `json:"address"`
	Body        string          `json:"body"`
	PostedAt    int64           `json:"postedAt"`
	Direction   string          `json:"direction"`
	Read        bool            `json:"read"`
	MessageType string          `json:"messageType"`
	Attachments []attachmentDTO `json:"attachments"`
}

type attachmentDTO struct {
	ID       string `json:"id"`
	MIMEType string `json:"mimeType"`
	Name     string `json:"name"`
	URI      string `json:"uri"`
	Size     int64  `json:"size"`
}

func (item messageDTO) domain() (domain.Message, error) {
	var direction domain.MessageDirection
	switch item.Direction {
	case "incoming":
		direction = domain.DirectionIncoming
	case "outgoing":
		direction = domain.DirectionOutgoing
	default:
		return domain.Message{}, fmt.Errorf("unknown remote message direction %q", item.Direction)
	}
	messageType := domain.MessageType(item.MessageType)
	switch messageType {
	case domain.MessageUnknown, domain.MessageSMS, domain.MessageMMS, domain.MessageRCS:
	default:
		return domain.Message{}, fmt.Errorf("unknown remote message type %q", item.MessageType)
	}
	attachments := make([]domain.Attachment, 0, len(item.Attachments))
	for _, attachment := range item.Attachments {
		attachments = append(attachments, domain.Attachment{
			ID: attachment.ID, MIMEType: attachment.MIMEType, Name: attachment.Name,
			URI: attachment.URI, Size: attachment.Size,
		})
	}
	return domain.Message{
		ID: item.ID, ThreadID: item.ThreadID, Address: item.Address, Body: item.Body,
		Timestamp: unixMillis(item.PostedAt), Direction: direction, Read: item.Read,
		Type: messageType, Attachments: attachments,
	}, nil
}

type sendMessagePayload struct {
	Address string `json:"address"`
	Text    string `json:"text"`
}

type sendReplyPayload struct {
	ThreadID int64  `json:"threadId"`
	Text     string `json:"text"`
}

type sendResultPayload struct {
	Outcome   string `json:"outcome"`
	Evidence  string `json:"evidence"`
	MessageID int64  `json:"messageId"`
	ThreadID  int64  `json:"threadId"`
	Error     string `json:"error"`
}

func unixMillis(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}

var _ domain.ConversationMessageSender = (*Sender)(nil)
