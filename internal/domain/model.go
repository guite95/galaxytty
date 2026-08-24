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
	MessageUnknown MessageType = "unknown"
	MessageSMS     MessageType = "sms"
	MessageMMS     MessageType = "mms"
	MessageRCS     MessageType = "rcs"
)

type Attachment struct {
	ID, MIMEType, Name, URI string
	Size                    int64
}
type Message struct {
	ID, ThreadID  int64
	Address, Body string
	Timestamp     time.Time
	// ObservedAt is the source adapter's event observation time. It is kept
	// separate from the message timestamp for latency diagnostics.
	ObservedAt time.Time
	Direction  MessageDirection
	Read       bool
	Type       MessageType
	// SendOutcome is set only for a local outgoing echo when Samsung Messages
	// accepted an action but no independent outgoing evidence is available.
	SendOutcome SendOutcome
	Attachments []Attachment
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

type SendOutcome string

const (
	SendOutcomeVerified           SendOutcome = "verified"
	SendOutcomeAcceptedUnverified SendOutcome = "accepted_unverified"
	SendOutcomeUserActionRequired SendOutcome = "user_action_required"
)

type SendResult struct {
	MessageID int64       `json:"message_id"`
	ThreadID  int64       `json:"thread_id"`
	Outcome   SendOutcome `json:"outcome,omitempty"`
	Evidence  string      `json:"evidence,omitempty"`
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

type ApplicationStatus struct {
	State      string
	Connection ConnectionKind
	Label      string
	Device     string
}
type StatusProvider interface {
	Status(context.Context) ApplicationStatus
}
type StatusEventSource interface {
	SubscribeStatus(context.Context) <-chan ApplicationStatus
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

// ConversationMessageSender is implemented by transports, such as a
// notification RemoteInput reply, that address an existing conversation
// without requiring a phone number on the Mac.
type ConversationMessageSender interface {
	SendToConversation(context.Context, int64, string) (SendResult, error)
}
type MessageEventSource interface {
	SubscribeMessages(context.Context) (<-chan Message, <-chan error)
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
