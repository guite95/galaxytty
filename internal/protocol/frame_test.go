package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestFrameRoundTripPreservesUnicodeAndCorrelation(t *testing.T) {
	want, err := NewEnvelope(TypeMessageReceived, "request-42", 1842, map[string]any{
		"body": "한글과 emoji 😀",
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	if err := WriteFrame(&wire, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(&wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != CurrentVersion || got.Type != TypeMessageReceived || got.RequestID != "request-42" || got.Sequence != 1842 {
		t.Fatalf("envelope=%+v", got)
	}
	if !bytes.Contains(got.Payload, []byte("한글과 emoji")) {
		t.Fatalf("payload=%s", got.Payload)
	}
}

func TestReadFrameRejectsInvalidLengths(t *testing.T) {
	tests := []struct {
		name   string
		length uint32
		want   error
	}{
		{name: "empty", length: 0, want: ErrEmptyFrame},
		{name: "oversized", length: MaxFrameSize + 1, want: ErrFrameTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var wire bytes.Buffer
			if err := binary.Write(&wire, binary.BigEndian, test.length); err != nil {
				t.Fatal(err)
			}
			_, err := ReadFrame(&wire)
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
		})
	}
}

func TestEnvelopeValidationRejectsUnknownVersionAndType(t *testing.T) {
	tests := []Envelope{
		{Version: 999, Type: TypeHello, Payload: []byte("{}")},
		{Version: CurrentVersion, Type: Type("SEND_SMS"), Payload: []byte("{}")},
		{Version: CurrentVersion, Type: TypePing, Payload: []byte("not-json")},
	}
	for _, envelope := range tests {
		if err := envelope.Validate(); err == nil {
			t.Fatalf("expected validation error for %+v", envelope)
		}
	}
}

func TestProtocolExposesTransportNeutralSendCommands(t *testing.T) {
	for _, messageType := range KnownTypes() {
		if strings.Contains(string(messageType), "SMS") || strings.Contains(string(messageType), "RCS") || strings.Contains(string(messageType), "MMS") {
			t.Fatalf("transport-specific command leaked: %s", messageType)
		}
	}
	if !IsKnownType(TypeSendMessage) || !IsKnownType(TypeSendReply) {
		t.Fatal("missing transport-neutral send commands")
	}
}
