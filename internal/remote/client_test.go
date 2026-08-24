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

func TestClientAuthenticatesBeforeReportingConnected(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	secret := make([]byte, pairing.CredentialSize)
	for index := range secret {
		secret[index] = byte(index)
	}
	challenge := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	store := &memoryCredentialStore{credential: pairing.Credential{
		DeviceID: "device-1", ClientID: "client-123", Secret: secret,
	}}
	client := newClient(Config{
		Address: "synthetic", Credentials: store,
		DialTimeout: time.Second, HeartbeatInterval: time.Hour, PongTimeout: 2 * time.Hour,
	}, func(context.Context, string, string) (net.Conn, error) { return clientConn, nil })
	serverDone := make(chan error, 1)
	go func() {
		hello, _ := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{
			DeviceID: "device-1", DeviceName: "Galaxy Test",
			Auth: &protocol.AuthChallenge{Mode: "hmac-sha256", Challenge: challenge},
		})
		if err := protocol.WriteFrame(serverConn, hello); err != nil {
			serverDone <- err
			return
		}
		request, err := protocol.ReadFrame(serverConn)
		if err != nil {
			serverDone <- err
			return
		}
		var auth protocol.AuthRequestPayload
		if err := request.DecodePayload(&auth); err != nil {
			serverDone <- err
			return
		}
		want := base64.RawURLEncoding.EncodeToString(pairing.Proof(secret, "device-1", "client-123", challenge))
		if auth.ClientID != "client-123" || auth.Proof != want {
			serverDone <- ErrAuthentication
			return
		}
		response, _ := protocol.NewEnvelope(protocol.TypeAuth, request.RequestID, 0, protocol.AuthResponsePayload{
			Authenticated: true,
			ServerProof: base64.RawURLEncoding.EncodeToString(
				pairing.ServerProof(secret, "device-1", "client-123", challenge),
			),
			SecureMode: protocol.SecureMode,
		})
		if err := protocol.WriteFrame(serverConn, response); err != nil {
			serverDone <- err
			return
		}
		secure, err := protocol.NewSecureChannel(secret, make([]byte, 32), protocol.SecureRoleServer)
		if err != nil {
			serverDone <- err
			return
		}
		ping, err := secure.Read(serverConn)
		if err != nil {
			serverDone <- err
			return
		}
		if ping.Type != protocol.TypePing {
			serverDone <- ErrInvalidHandshake
			return
		}
		pong, _ := protocol.NewEnvelope(protocol.TypePong, ping.RequestID, 0, map[string]any{"ok": true})
		serverDone <- secure.Write(serverConn, pong)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx) }()
	waitContext, waitCancel := context.WithTimeout(ctx, time.Second)
	if err := client.WaitConnected(waitContext); err != nil {
		waitCancel()
		t.Fatal(err)
	}
	waitCancel()
	if status := client.Status(ctx); status.State != string(StateConnected) {
		t.Fatalf("status=%+v", status)
	}
	requestContext, requestCancel := context.WithTimeout(ctx, time.Second)
	response, err := client.Request(requestContext, protocol.TypePing, map[string]any{"test": true})
	requestCancel()
	if err != nil || response.Type != protocol.TypePong {
		t.Fatalf("encrypted response=%+v err=%v", response, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not stop")
	}
}

func TestClientReportsPairingRequiredWithoutSendingProtocolCommands(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	challenge := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	client := newClient(Config{
		Address: "synthetic", Credentials: &memoryCredentialStore{},
		DialTimeout: time.Second, HeartbeatInterval: time.Hour, PongTimeout: 2 * time.Hour,
	}, func(context.Context, string, string) (net.Conn, error) { return clientConn, nil })
	go func() {
		hello, _ := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{
			DeviceID: "device-1", DeviceName: "Galaxy Test",
			Auth: &protocol.AuthChallenge{Mode: "hmac-sha256", Challenge: challenge},
		})
		_ = protocol.WriteFrame(serverConn, hello)
	}()
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(context.Background()) }()
	waitContext, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := client.WaitConnected(waitContext); !errors.Is(err, ErrPairingRequired) {
		t.Fatalf("wait error=%v", err)
	}
	if err := <-runDone; !errors.Is(err, ErrPairingRequired) {
		t.Fatalf("run error=%v", err)
	}
}

func TestClientRejectsServerWithoutValidKeyConfirmation(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	secret := make([]byte, pairing.CredentialSize)
	challenge := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	client := newClient(Config{
		Address: "synthetic",
		Credentials: &memoryCredentialStore{credential: pairing.Credential{
			DeviceID: "device-1", ClientID: "client-123", Secret: secret,
		}},
		DialTimeout: time.Second, HeartbeatInterval: time.Hour, PongTimeout: 2 * time.Hour,
	}, func(context.Context, string, string) (net.Conn, error) { return clientConn, nil })
	go func() {
		hello, _ := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{
			DeviceID: "device-1", DeviceName: "Galaxy Test",
			Auth: &protocol.AuthChallenge{Mode: "hmac-sha256", Challenge: challenge},
		})
		_ = protocol.WriteFrame(serverConn, hello)
		request, _ := protocol.ReadFrame(serverConn)
		response, _ := protocol.NewEnvelope(protocol.TypeAuth, request.RequestID, 0, protocol.AuthResponsePayload{
			Authenticated: true,
			ServerProof:   base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
			SecureMode:    protocol.SecureMode,
		})
		_ = protocol.WriteFrame(serverConn, response)
	}()
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(context.Background()) }()
	waitContext, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := client.WaitConnected(waitContext); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("wait error=%v", err)
	}
	if err := <-runDone; !errors.Is(err, ErrAuthentication) {
		t.Fatalf("run error=%v", err)
	}
}

func TestClientHeartbeatRequestCorrelationAndSequenceGap(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		hello, _ := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{DeviceID: "synthetic", DeviceName: "Galaxy test"})
		if err := protocol.WriteFrame(conn, hello); err != nil {
			serverDone <- err
			return
		}

		for {
			request, err := protocol.ReadFrame(conn)
			if err != nil {
				serverDone <- err
				return
			}
			switch request.Type {
			case protocol.TypePing:
				pong, _ := protocol.NewEnvelope(protocol.TypePong, request.RequestID, 0, map[string]any{"ok": true})
				if err := protocol.WriteFrame(conn, pong); err != nil {
					serverDone <- err
					return
				}
			case protocol.TypeGetConversations:
				response, _ := protocol.NewEnvelope(protocol.TypeConversations, request.RequestID, 0, map[string]any{"items": []any{}})
				if err := protocol.WriteFrame(conn, response); err != nil {
					serverDone <- err
					return
				}
				first, _ := protocol.NewEnvelope(protocol.TypeMessageReceived, "", 1, map[string]any{"id": 1})
				third, _ := protocol.NewEnvelope(protocol.TypeMessageReceived, "", 3, map[string]any{"id": 3})
				if err := protocol.WriteFrame(conn, first); err != nil {
					serverDone <- err
					return
				}
				serverDone <- protocol.WriteFrame(conn, third)
				return
			}
		}
	}()

	client, err := NewClient(Config{
		Address:           listener.Addr().String(),
		DialTimeout:       time.Second,
		HeartbeatInterval: 20 * time.Millisecond,
		PongTimeout:       200 * time.Millisecond,
		ReconnectMinimum:  time.Second,
		ReconnectMaximum:  time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx) }()

	waitForEventType(t, client.Events(), protocol.TypeHello)
	requestContext, requestCancel := context.WithTimeout(ctx, time.Second)
	response, err := client.Request(requestContext, protocol.TypeGetConversations, map[string]any{})
	requestCancel()
	if err != nil || response.Type != protocol.TypeConversations || response.RequestID == "" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	waitForEventType(t, client.Events(), protocol.TypeMessageReceived)
	waitForEventType(t, client.Events(), protocol.TypeMessageReceived)

	select {
	case gap := <-client.SequenceGaps():
		if gap != (SequenceGap{Expected: 2, Received: 3}) {
			t.Fatalf("gap=%+v", gap)
		}
	case <-time.After(time.Second):
		t.Fatal("missing sequence gap")
	}

	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not stop")
	}
}

func TestClientUsesReconnectHelloSequenceForImmediateRecovery(t *testing.T) {
	client := newClient(Config{}, nil)
	client.lastSequence.Store(4)
	client.trackHelloSequence(context.Background(), 7)

	select {
	case gap := <-client.SequenceGaps():
		if gap != (SequenceGap{Expected: 5, Received: 8}) {
			t.Fatalf("gap=%+v", gap)
		}
	case <-time.After(time.Second):
		t.Fatal("missing reconnect gap")
	}
	if current := client.lastSequence.Load(); current != 7 {
		t.Fatalf("sequence=%d", current)
	}

	client.trackHelloSequence(context.Background(), 2)
	select {
	case gap := <-client.SequenceGaps():
		if gap != (SequenceGap{Expected: 8, Received: 2, Reset: true}) {
			t.Fatalf("reset gap=%+v", gap)
		}
	case <-time.After(time.Second):
		t.Fatal("missing sequence reset")
	}
}

func TestClientIgnoresDuplicateOrOutOfOrderEventSequence(t *testing.T) {
	client := newClient(Config{}, nil)
	client.lastSequence.Store(8)
	client.trackSequence(context.Background(), 8)
	client.trackSequence(context.Background(), 7)

	if current := client.lastSequence.Load(); current != 8 {
		t.Fatalf("sequence regressed to %d", current)
	}
	select {
	case gap := <-client.SequenceGaps():
		t.Fatalf("unexpected gap=%+v", gap)
	default:
	}
}

func TestClientReconnectsAfterDisconnect(t *testing.T) {
	connections := make(chan net.Conn, 2)
	clientSide := make(chan net.Conn, 2)
	for range 2 {
		clientConn, serverConn := net.Pipe()
		clientSide <- clientConn
		connections <- serverConn
	}
	client := newClient(Config{
		Address:           "synthetic",
		HeartbeatInterval: time.Hour,
		PongTimeout:       2 * time.Hour,
		ReconnectMinimum:  time.Millisecond,
		ReconnectMaximum:  time.Millisecond,
	}, func(context.Context, string, string) (net.Conn, error) {
		return <-clientSide, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx) }()

	first := <-connections
	hello, _ := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{DeviceID: "synthetic", DeviceName: "Galaxy test"})
	go protocol.WriteFrame(first, hello)
	waitForEventType(t, client.Events(), protocol.TypeHello)
	_ = first.Close()

	second := <-connections
	hello, _ = protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{DeviceID: "synthetic", DeviceName: "Galaxy test"})
	go protocol.WriteFrame(second, hello)
	waitForEventType(t, client.Events(), protocol.TypeHello)
	_ = second.Close()
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not stop")
	}
}

func TestWaitConnectedRequiresHelloHandshake(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	client := newClient(Config{
		Address:           "synthetic",
		HeartbeatInterval: time.Hour,
		PongTimeout:       2 * time.Hour,
		ReconnectMinimum:  time.Second,
		ReconnectMaximum:  time.Second,
	}, func(context.Context, string, string) (net.Conn, error) {
		return clientConn, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = client.Run(ctx) }()

	beforeHello, beforeHelloCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	if err := client.WaitConnected(beforeHello); err == nil {
		beforeHelloCancel()
		t.Fatal("client reported connected before HELLO")
	}
	beforeHelloCancel()

	hello, err := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{
		DeviceID: "synthetic", DeviceName: "Galaxy test",
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = protocol.WriteFrame(serverConn, hello) }()
	afterHello, afterHelloCancel := context.WithTimeout(ctx, time.Second)
	defer afterHelloCancel()
	if err := client.WaitConnected(afterHello); err != nil {
		t.Fatalf("wait after HELLO: %v", err)
	}
	status := client.Status(ctx)
	if status.State != string(StateConnected) || status.Device != "Galaxy test" {
		t.Fatalf("status=%+v", status)
	}
}

func waitForEventType(t *testing.T, events <-chan protocol.Envelope, messageType protocol.Type) protocol.Envelope {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != messageType {
			t.Fatalf("event type=%s want=%s", event.Type, messageType)
		}
		return event
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", messageType)
		return protocol.Envelope{}
	}
}
