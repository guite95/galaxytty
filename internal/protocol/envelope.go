package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

const CurrentVersion = 1

type Type string

const (
	TypeHello            Type = "HELLO"
	TypeAuth             Type = "AUTH"
	TypeSecure           Type = "SECURE"
	TypePing             Type = "PING"
	TypePong             Type = "PONG"
	TypeMessageReceived  Type = "MESSAGE_RECEIVED"
	TypeGetConversations Type = "GET_CONVERSATIONS"
	TypeConversations    Type = "CONVERSATIONS"
	TypeGetMessages      Type = "GET_MESSAGES"
	TypeMessages         Type = "MESSAGES"
	TypeSendMessage      Type = "SEND_MESSAGE"
	TypeSendReply        Type = "SEND_REPLY"
	TypeSendResult       Type = "SEND_RESULT"
	TypeSyncRequest      Type = "SYNC_REQUEST"
	TypeSyncMessage      Type = "SYNC_MESSAGE"
	TypeError            Type = "ERROR"
)

var knownTypes = []Type{
	TypeHello,
	TypeAuth,
	TypeSecure,
	TypePing,
	TypePong,
	TypeMessageReceived,
	TypeGetConversations,
	TypeConversations,
	TypeGetMessages,
	TypeMessages,
	TypeSendMessage,
	TypeSendReply,
	TypeSendResult,
	TypeSyncRequest,
	TypeSyncMessage,
	TypeError,
}

var (
	ErrUnsupportedVersion = errors.New("unsupported protocol version")
	ErrUnknownType        = errors.New("unknown protocol message type")
	ErrInvalidPayload     = errors.New("invalid JSON payload")
)

type Envelope struct {
	Version   int             `json:"version"`
	Sequence  uint64          `json:"sequence,omitempty"`
	Type      Type            `json:"type"`
	RequestID string          `json:"requestId,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

func NewEnvelope(messageType Type, requestID string, sequence uint64, payload any) (Envelope, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal protocol payload: %w", err)
	}
	envelope := Envelope{
		Version:   CurrentVersion,
		Sequence:  sequence,
		Type:      messageType,
		RequestID: requestID,
		Payload:   encoded,
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func (e Envelope) Validate() error {
	if e.Version != CurrentVersion {
		return fmt.Errorf("%w: %d", ErrUnsupportedVersion, e.Version)
	}
	if !IsKnownType(e.Type) {
		return fmt.Errorf("%w: %q", ErrUnknownType, e.Type)
	}
	payload := bytes.TrimSpace(e.Payload)
	if len(payload) == 0 || !json.Valid(payload) {
		return ErrInvalidPayload
	}
	return nil
}

func (e Envelope) DecodePayload(target any) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := json.Unmarshal(e.Payload, target); err != nil {
		return fmt.Errorf("decode %s payload: %w", e.Type, err)
	}
	return nil
}

func KnownTypes() []Type { return append([]Type(nil), knownTypes...) }

func IsKnownType(candidate Type) bool {
	for _, known := range knownTypes {
		if candidate == known {
			return true
		}
	}
	return false
}

type HelloPayload struct {
	DeviceID      string         `json:"deviceId"`
	DeviceName    string         `json:"deviceName"`
	Capabilities  []string       `json:"capabilities"`
	EventSequence uint64         `json:"eventSequence,omitempty"`
	Authenticated bool           `json:"authenticated"`
	ReadOnlyPOC   bool           `json:"readOnlyPoc"`
	Auth          *AuthChallenge `json:"auth,omitempty"`
}

type AuthChallenge struct {
	Mode      string `json:"mode"`
	Challenge string `json:"challenge"`
}

type AuthRequestPayload struct {
	ClientID string `json:"clientId"`
	Proof    string `json:"proof"`
}

type AuthResponsePayload struct {
	Authenticated bool   `json:"authenticated"`
	ServerProof   string `json:"serverProof"`
	SecureMode    string `json:"secureMode"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
