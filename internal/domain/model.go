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

type Device interface {
	State(context.Context) (DeviceState, error)
	Shell(context.Context, ...string) ([]byte, error)
}
type MessageStore interface {
	Conversations(context.Context) ([]Conversation, error)
	Messages(context.Context, int64, MessageQuery) ([]Message, error)
	MessagesAfter(context.Context, int64) ([]Message, error)
}
type Contacts interface {
	Resolve(context.Context, string) (Contact, error)
}
type VirtualDisplayManager interface {
	Start(context.Context) (VirtualDisplay, error)
	Stop(context.Context) error
	Healthy(context.Context) bool
}
type MessageSender interface {
	Send(context.Context, string, string) error
}
type ConversationController interface {
	OpenConversation(context.Context, string) error
}
type Clipboard interface {
	Set(context.Context, string) error
}
type Notifier interface {
	NotifyMessage(context.Context, Notification) error
	Close(context.Context) error
}
