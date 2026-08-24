package remote

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/pairing"
	"github.com/galaxytty/galaxytty/internal/protocol"
)

type memoryCredentialStore struct {
	credential pairing.Credential
}

func (store *memoryCredentialStore) Load(deviceID string) (pairing.Credential, error) {
	if store.credential.DeviceID != deviceID {
		return pairing.Credential{}, pairing.ErrNotPaired
	}
	return store.credential, nil
}

func (store *memoryCredentialStore) Save(credential pairing.Credential) error {
	store.credential = credential
	return nil
}

func TestPairProvesCodeWithoutSendingItAndStoresCredential(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	secret := make([]byte, pairing.CredentialSize)
	for index := range secret {
		secret[index] = byte(index)
	}
	challenge := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		hello, _ := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{
			DeviceID: "device-1", DeviceName: "Galaxy Test",
			Auth: &protocol.AuthChallenge{Mode: "hmac-sha256", Challenge: challenge},
		})
		if err := protocol.WriteFrame(conn, hello); err != nil {
			serverDone <- err
			return
		}
		request, err := protocol.ReadFrame(conn)
		if err != nil {
			serverDone <- err
			return
		}
		var payload protocol.AuthRequestPayload
		if err := request.DecodePayload(&payload); err != nil {
			serverDone <- err
			return
		}
		wantProof := base64.RawURLEncoding.EncodeToString(pairing.Proof(secret, "device-1", payload.ClientID, challenge))
		if request.Type != protocol.TypeAuth || payload.Proof != wantProof || payload.Proof == pairing.FormatCode(secret) {
			serverDone <- ErrAuthentication
			return
		}
		response, _ := protocol.NewEnvelope(protocol.TypeAuth, request.RequestID, 0, protocol.AuthResponsePayload{
			Authenticated: true,
			ServerProof: base64.RawURLEncoding.EncodeToString(
				pairing.ServerProof(secret, "device-1", payload.ClientID, challenge),
			),
			SecureMode: protocol.SecureMode,
		})
		serverDone <- protocol.WriteFrame(conn, response)
	}()

	store := &memoryCredentialStore{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := Pair(ctx, Config{Address: listener.Addr().String(), DialTimeout: time.Second}, nil, pairing.FormatCode(secret), store)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeviceName != "Galaxy Test" || store.credential.DeviceID != "device-1" || store.credential.ClientID == "" {
		t.Fatalf("result=%+v credential=%+v", result, store.credential)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestPairDoesNotStoreRejectedCredential(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		challenge := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
		hello, _ := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{
			DeviceID: "device-1", DeviceName: "Galaxy Test",
			Auth: &protocol.AuthChallenge{Mode: "hmac-sha256", Challenge: challenge},
		})
		_ = protocol.WriteFrame(conn, hello)
		request, _ := protocol.ReadFrame(conn)
		response, _ := protocol.NewEnvelope(protocol.TypeError, request.RequestID, 0, protocol.ErrorPayload{Code: "AUTH_FAILED", Message: "rejected"})
		_ = protocol.WriteFrame(conn, response)
	}()

	store := &memoryCredentialStore{}
	code := pairing.FormatCode(make([]byte, pairing.CredentialSize))
	_, err = Pair(context.Background(), Config{Address: listener.Addr().String(), DialTimeout: time.Second}, nil, code, store)
	if err == nil || store.credential.DeviceID != "" {
		t.Fatalf("err=%v credential=%+v", err, store.credential)
	}
}

func TestPairRejectsUnauthenticatedServerProof(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		challenge := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
		hello, _ := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{
			DeviceID: "device-1", DeviceName: "Galaxy Test",
			Auth: &protocol.AuthChallenge{Mode: "hmac-sha256", Challenge: challenge},
		})
		_ = protocol.WriteFrame(conn, hello)
		request, _ := protocol.ReadFrame(conn)
		response, _ := protocol.NewEnvelope(protocol.TypeAuth, request.RequestID, 0, protocol.AuthResponsePayload{
			Authenticated: true,
			ServerProof:   base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
			SecureMode:    protocol.SecureMode,
		})
		_ = protocol.WriteFrame(conn, response)
	}()

	store := &memoryCredentialStore{}
	code := pairing.FormatCode(make([]byte, pairing.CredentialSize))
	_, err = Pair(context.Background(), Config{Address: listener.Addr().String(), DialTimeout: time.Second}, nil, code, store)
	if !errors.Is(err, ErrAuthentication) || store.credential.DeviceID != "" {
		t.Fatalf("err=%v credential=%+v", err, store.credential)
	}
}
