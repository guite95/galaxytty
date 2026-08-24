package protocol

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

func TestSecureChannelRoundTripBothDirections(t *testing.T) {
	secret, challenge := secureFixture()
	client, err := NewSecureChannel(secret, challenge, SecureRoleClient)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewSecureChannel(secret, challenge, SecureRoleServer)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := NewEnvelope(TypePing, "request-1", 0, map[string]any{"text": "한글 😀"})
	var clientWire bytes.Buffer
	if err := client.Write(&clientWire, request); err != nil {
		t.Fatal(err)
	}
	received, err := server.Read(&clientWire)
	if err != nil || received.Type != TypePing || received.RequestID != "request-1" {
		t.Fatalf("received=%+v err=%v", received, err)
	}
	response, _ := NewEnvelope(TypePong, "request-1", 0, map[string]any{"ok": true})
	var serverWire bytes.Buffer
	if err := server.Write(&serverWire, response); err != nil {
		t.Fatal(err)
	}
	received, err = client.Read(&serverWire)
	if err != nil || received.Type != TypePong {
		t.Fatalf("received=%+v err=%v", received, err)
	}
}

func TestSecureChannelRejectsPlaintextReplayAndTampering(t *testing.T) {
	secret, challenge := secureFixture()
	client, _ := NewSecureChannel(secret, challenge, SecureRoleClient)
	server, _ := NewSecureChannel(secret, challenge, SecureRoleServer)
	plain, _ := NewEnvelope(TypePing, "plain", 0, map[string]any{})
	var plaintext bytes.Buffer
	_ = WriteFrame(&plaintext, plain)
	if _, err := server.Read(&plaintext); !errors.Is(err, ErrSecureFrameExpected) {
		t.Fatalf("plaintext err=%v", err)
	}

	secureServer, _ := NewSecureChannel(secret, challenge, SecureRoleServer)
	var wire bytes.Buffer
	_ = client.Write(&wire, plain)
	encoded := append([]byte(nil), wire.Bytes()...)
	if _, err := secureServer.Read(bytes.NewReader(encoded)); err != nil {
		t.Fatal(err)
	}
	if _, err := secureServer.Read(bytes.NewReader(encoded)); !errors.Is(err, ErrSecureCounter) {
		t.Fatalf("replay err=%v", err)
	}

	tamperClient, _ := NewSecureChannel(secret, challenge, SecureRoleClient)
	tamperServer, _ := NewSecureChannel(secret, challenge, SecureRoleServer)
	wire.Reset()
	_ = tamperClient.Write(&wire, plain)
	outer, err := ReadFrame(&wire)
	if err != nil {
		t.Fatal(err)
	}
	var payload securePayload
	if err := outer.DecodePayload(&payload); err != nil {
		t.Fatal(err)
	}
	first := payload.Ciphertext[0]
	if first == 'A' {
		first = 'B'
	} else {
		first = 'A'
	}
	payload.Ciphertext = string(first) + payload.Ciphertext[1:]
	outer, _ = NewEnvelope(TypeSecure, "", 0, payload)
	wire.Reset()
	_ = WriteFrame(&wire, outer)
	if _, err := tamperServer.Read(&wire); !errors.Is(err, ErrSecureAuthentication) {
		t.Fatalf("tamper err=%v", err)
	}
}

func TestSecureSessionMaterialMatchesCrossPlatformVector(t *testing.T) {
	secret, challenge := secureFixture()
	material, err := deriveSessionMaterial(secret, challenge)
	if err != nil {
		t.Fatal(err)
	}
	values := []struct {
		name string
		got  []byte
		want string
	}{
		{"client key", material.clientToServerKey, "63ddb312c9aab6b965b5d8e87b238bb8d5165ba70f21e7d3ac2d6e896638daf4"},
		{"server key", material.serverToClientKey, "10910caf0c2e2dc1dd2eb5e763da0b2789c3318b482a2ef6d6bdda07e272d8b1"},
		{"client nonce", material.clientToServerNonce, "56c98f09"},
		{"server nonce", material.serverToClientNonce, "aa4fe1bd"},
	}
	for _, value := range values {
		if got := hex.EncodeToString(value.got); got != value.want {
			t.Errorf("%s=%s", value.name, got)
		}
	}
}

func secureFixture() ([]byte, []byte) {
	secret := make([]byte, 20)
	challenge := make([]byte, 32)
	for index := range secret {
		secret[index] = byte(index)
	}
	for index := range challenge {
		challenge[index] = byte(0xa0 + index)
	}
	return secret, challenge
}
