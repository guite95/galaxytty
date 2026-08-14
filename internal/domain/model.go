package domain

import (
	"context"
	"time"
)

type MessageDirection uint8

const (
	DirectionIncoming MessageDirection = iota + 1
	DirectionOutgoing
)

type MessageType string

const (
	MessageSMS MessageType = "sms"
	MessageMMS MessageType = "mms"
	MessageRCS MessageType = "rcs"
)

type TextInputMode string

const (
	TextInputIntentBody TextInputMode = "intent_body"
	TextInputClipboard  TextInputMode = "clipboard"
)

func (m TextInputMode) Valid() bool {
	return m == TextInputIntentBody || m == TextInputClipboard
}

type Attachment struct {
	ID, MIMEType, Name, URI string
	Size                    int64
}
type Message struct {
	ID, ThreadID  int64
	Address, Body string
	Timestamp     time.Time
	Direction     MessageDirection
	Read          bool
	Type          MessageType
	Attachments   []Attachment
}
type Conversation struct {
	ThreadID     int64
	Title        string
	Participants []Contact
	Snippet      string
	UpdatedAt    time.Time
	UnreadCount  int
}
type Contact struct {
	ID                 int64
	DisplayName, Phone string
}
type ConnectionKind string

const (
	ConnectionUSB      ConnectionKind = "usb"
	ConnectionWireless ConnectionKind = "wireless"
)

type DeviceState string

const (
	DeviceDisconnected DeviceState = "disconnected"
	DeviceConnected    DeviceState = "connected"
	DeviceUnauthorized DeviceState = "unauthorized"
)

type DeviceInfo struct {
	Serial, Model string
	State         DeviceState
	Connection    ConnectionKind
}

// AndroidDisplayID and SurfaceFlingerDisplayID deliberately have distinct types/fields.
type VirtualDisplay struct {
	AndroidDisplayID        int64
	SurfaceFlingerDisplayID string
	Width, Height           int
}
type Notification struct {
	ThreadID    int64
	Title, Body string
}
type MessageQuery struct {
	Limit    int
	BeforeID int64
}
type SendResult struct {
	MessageID int64       `json:"message_id"`
	ThreadID  int64       `json:"thread_id"`
	Transport MessageType `json:"transport,omitempty"`
}

type Device interface {
	State(context.Context) (DeviceState, error)
	Shell(context.Context, ...string) ([]byte, error)
}
type MessageStore interface {
	Conversations(context.Context) ([]Conversation, error)
	Messages(context.Context, int64, MessageQuery) ([]Message, error)
	MessagesAfter(context.Context, int64) ([]Message, error)
	LatestMessageID(context.Context) (int64, error)
}

type MMSMessageStore interface {
	LatestMMSMessageID(context.Context) (int64, error)
	MMSMessagesAfter(context.Context, int64) ([]Message, error)
}

type ApplicationStatus struct {
	State      string
	Connection ConnectionKind
	Label      string
}
type StatusProvider interface {
	Status(context.Context) ApplicationStatus
}
type Contacts interface {
	Resolve(context.Context, string) (Contact, error)
}
type VirtualDisplayManager interface {
	Start(context.Context) (VirtualDisplay, error)
	SyncClipboard(context.Context) error
	Stop(context.Context) error
	Healthy(context.Context) bool
}
type MessageSender interface {
	Send(context.Context, string, string) (SendResult, error)
}
type ConversationController interface {
	OpenConversation(context.Context, VirtualDisplay, string) error
}
type Clipboard interface {
	Read(context.Context) (string, error)
	Set(context.Context, string) error
}
type Notifier interface {
	NotifyMessage(context.Context, Notification) error
	Close(context.Context) error
}
