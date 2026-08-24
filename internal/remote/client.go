package remote

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/pairing"
	"github.com/galaxytty/galaxytty/internal/protocol"
)

var (
	ErrNotConnected     = errors.New("Galaxy Helper is not connected")
	ErrDisconnected     = errors.New("Galaxy Helper connection closed")
	ErrHeartbeatTimeout = errors.New("Galaxy Helper heartbeat timed out")
	ErrPairingRequired  = errors.New("Galaxy Helper pairing is required")
	ErrAuthentication   = errors.New("Galaxy Helper authentication failed")
	ErrInvalidHandshake = errors.New("invalid Galaxy Helper handshake")
)

type ConnectionState string

const (
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateDisconnected ConnectionState = "disconnected"
)

type StateChange struct {
	State ConnectionState
	Err   error
}

type SequenceGap struct {
	Expected uint64
	Received uint64
}

type Config struct {
	Address           string
	DialTimeout       time.Duration
	HeartbeatInterval time.Duration
	PongTimeout       time.Duration
	ReconnectMinimum  time.Duration
	ReconnectMaximum  time.Duration
	Credentials       pairing.Store
}

type dialFunc func(context.Context, string, string) (net.Conn, error)
type addressFunc func(context.Context) (string, error)

type AddressResolver interface {
	Resolve(context.Context) (string, error)
}

type response struct {
	envelope protocol.Envelope
	err      error
}

type Client struct {
	config  Config
	dial    dialFunc
	address addressFunc

	mu      sync.Mutex
	conn    net.Conn
	secure  *protocol.SecureChannel
	pending map[string]chan response
	writeMu sync.Mutex

	events chan protocol.Envelope
	gaps   chan SequenceGap
	states chan StateChange

	lastPong      atomic.Int64
	lastSequence  atomic.Uint64
	connected     chan struct{}
	connectErrors chan error
	connectedOnce sync.Once
	statusMu      sync.RWMutex
	state         ConnectionState
	deviceName    string
}

func NewClient(config Config) (*Client, error) {
	config = config.withDefaults()
	if config.Address == "" {
		return nil, errors.New("Galaxy Helper address is required")
	}
	dialer := &net.Dialer{Timeout: config.DialTimeout}
	return newClient(config, dialer.DialContext), nil
}

func newClient(config Config, dial dialFunc) *Client {
	staticAddress := config.Address
	return &Client{
		config:        config.withDefaults(),
		dial:          dial,
		address:       func(context.Context) (string, error) { return staticAddress, nil },
		pending:       make(map[string]chan response),
		events:        make(chan protocol.Envelope, 64),
		gaps:          make(chan SequenceGap, 8),
		states:        make(chan StateChange, 16),
		connected:     make(chan struct{}),
		connectErrors: make(chan error, 1),
		state:         StateDisconnected,
	}
}

func NewDiscoveredClient(config Config, resolver AddressResolver) (*Client, error) {
	if resolver == nil {
		return nil, errors.New("Galaxy Helper resolver is required")
	}
	config = config.withDefaults()
	dialer := &net.Dialer{Timeout: config.DialTimeout}
	client := newClient(config, dialer.DialContext)
	client.address = resolver.Resolve
	return client, nil
}

func (config Config) withDefaults() Config {
	if config.DialTimeout <= 0 {
		config.DialTimeout = 5 * time.Second
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 15 * time.Second
	}
	if config.PongTimeout <= 0 {
		config.PongTimeout = 45 * time.Second
	}
	if config.ReconnectMinimum <= 0 {
		config.ReconnectMinimum = 250 * time.Millisecond
	}
	if config.ReconnectMaximum < config.ReconnectMinimum {
		config.ReconnectMaximum = 5 * time.Second
	}
	return config
}

func (client *Client) Events() <-chan protocol.Envelope { return client.events }
func (client *Client) SequenceGaps() <-chan SequenceGap { return client.gaps }
func (client *Client) States() <-chan StateChange       { return client.states }

func (client *Client) WaitConnected(ctx context.Context) error {
	select {
	case <-client.connected:
		return nil
	case err := <-client.connectErrors:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (client *Client) Status(context.Context) domain.ApplicationStatus {
	client.statusMu.RLock()
	defer client.statusMu.RUnlock()
	label := "Offline"
	if client.state == StateConnecting {
		label = "Connecting"
	} else if client.state == StateConnected {
		label = "Local Wi-Fi"
	}
	return domain.ApplicationStatus{
		State: string(client.state), Connection: domain.ConnectionWireless,
		Label: label, Device: client.deviceName,
	}
}

// Run maintains one persistent session and reconnects with bounded exponential
// backoff until the context is canceled.
func (client *Client) Run(ctx context.Context) error {
	backoff := client.config.ReconnectMinimum
	for {
		if err := ctx.Err(); err != nil {
			client.closeCurrent()
			return nil
		}
		client.publishState(StateChange{State: StateConnecting})
		address, err := client.address(ctx)
		if err != nil {
			client.publishState(StateChange{State: StateDisconnected, Err: err})
			if !waitContext(ctx, backoff) {
				return nil
			}
			backoff = min(backoff*2, client.config.ReconnectMaximum)
			continue
		}
		conn, err := client.dial(ctx, "tcp", address)
		if err != nil {
			client.publishState(StateChange{State: StateDisconnected, Err: err})
			if !waitContext(ctx, backoff) {
				return nil
			}
			backoff = min(backoff*2, client.config.ReconnectMaximum)
			continue
		}

		client.setConnection(conn)
		backoff = client.config.ReconnectMinimum
		err = client.serve(ctx, conn)
		client.clearConnection(conn)
		_ = conn.Close()
		client.failPending(ErrDisconnected)
		if ctx.Err() != nil {
			return nil
		}
		client.publishState(StateChange{State: StateDisconnected, Err: err})
		if isPermanentConnectionError(err) {
			client.publishConnectError(err)
			return err
		}
		if !waitContext(ctx, backoff) {
			return nil
		}
	}
}

func (client *Client) Request(ctx context.Context, messageType protocol.Type, payload any) (protocol.Envelope, error) {
	requestID, err := newRequestID()
	if err != nil {
		return protocol.Envelope{}, err
	}
	envelope, err := protocol.NewEnvelope(messageType, requestID, 0, payload)
	if err != nil {
		return protocol.Envelope{}, err
	}
	result := make(chan response, 1)
	client.mu.Lock()
	if client.conn == nil {
		client.mu.Unlock()
		return protocol.Envelope{}, ErrNotConnected
	}
	client.pending[requestID] = result
	client.mu.Unlock()

	if err := client.write(envelope); err != nil {
		client.removePending(requestID)
		return protocol.Envelope{}, err
	}
	select {
	case received := <-result:
		return received.envelope, received.err
	case <-ctx.Done():
		client.removePending(requestID)
		return protocol.Envelope{}, ctx.Err()
	}
}

func (client *Client) serve(ctx context.Context, conn net.Conn) error {
	helloEnvelope, hello, secure, err := client.handshake(ctx, conn)
	if err != nil {
		return err
	}
	client.setSecureConnection(conn, secure)
	client.statusMu.Lock()
	client.deviceName = hello.DeviceName
	client.statusMu.Unlock()
	client.publishState(StateChange{State: StateConnected})
	client.connectedOnce.Do(func() { close(client.connected) })
	select {
	case client.events <- helloEnvelope:
	case <-ctx.Done():
		return nil
	}
	client.lastPong.Store(time.Now().UnixNano())
	readErrors := make(chan error, 1)
	go func() { readErrors <- client.readLoop(ctx, conn, secure) }()
	ticker := time.NewTicker(client.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErrors:
			return err
		case now := <-ticker.C:
			lastPong := time.Unix(0, client.lastPong.Load())
			if now.Sub(lastPong) > client.config.PongTimeout {
				return ErrHeartbeatTimeout
			}
			requestID, err := newRequestID()
			if err != nil {
				return err
			}
			ping, err := protocol.NewEnvelope(protocol.TypePing, requestID, 0, map[string]any{
				"sentAt": now.UnixMilli(),
			})
			if err != nil {
				return err
			}
			if err := client.writeTo(conn, ping); err != nil {
				return err
			}
		}
	}
}

func (client *Client) readLoop(ctx context.Context, conn net.Conn, secure *protocol.SecureChannel) error {
	for {
		var envelope protocol.Envelope
		var err error
		if secure == nil {
			envelope, err = protocol.ReadFrame(conn)
		} else {
			envelope, err = secure.Read(conn)
		}
		if err != nil {
			return err
		}
		if envelope.Type == protocol.TypePong {
			client.lastPong.Store(time.Now().UnixNano())
		}
		if envelope.Type == protocol.TypeHello {
			return fmt.Errorf("%w: unexpected second HELLO", ErrInvalidHandshake)
		}
		if client.deliverResponse(envelope) {
			continue
		}
		if envelope.Type == protocol.TypePong {
			continue
		}
		client.trackSequence(ctx, envelope.Sequence)
		select {
		case client.events <- envelope:
		case <-ctx.Done():
			return nil
		}
	}
}

func (client *Client) trackSequence(ctx context.Context, received uint64) {
	if received == 0 {
		return
	}
	previous := client.lastSequence.Swap(received)
	if previous == 0 || received <= previous+1 {
		return
	}
	select {
	case client.gaps <- SequenceGap{Expected: previous + 1, Received: received}:
	case <-ctx.Done():
	}
}

func (client *Client) write(envelope protocol.Envelope) error {
	client.mu.Lock()
	conn := client.conn
	client.mu.Unlock()
	if conn == nil {
		return ErrNotConnected
	}
	return client.writeTo(conn, envelope)
}

func (client *Client) writeTo(conn net.Conn, envelope protocol.Envelope) error {
	client.mu.Lock()
	secure := client.secure
	if client.conn != conn {
		secure = nil
	}
	client.mu.Unlock()
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	var err error
	if secure == nil {
		err = protocol.WriteFrame(conn, envelope)
	} else {
		err = secure.Write(conn, envelope)
	}
	if err != nil {
		return fmt.Errorf("write Galaxy Helper message: %w", err)
	}
	return nil
}

func (client *Client) deliverResponse(envelope protocol.Envelope) bool {
	if envelope.RequestID == "" {
		return false
	}
	client.mu.Lock()
	pending, found := client.pending[envelope.RequestID]
	if found {
		delete(client.pending, envelope.RequestID)
	}
	client.mu.Unlock()
	if found {
		pending <- response{envelope: envelope}
	}
	return found
}

func (client *Client) removePending(requestID string) {
	client.mu.Lock()
	delete(client.pending, requestID)
	client.mu.Unlock()
}

func (client *Client) failPending(err error) {
	client.mu.Lock()
	pending := client.pending
	client.pending = make(map[string]chan response)
	client.mu.Unlock()
	for _, result := range pending {
		result <- response{err: err}
	}
}

func (client *Client) setConnection(conn net.Conn) {
	client.mu.Lock()
	client.conn = conn
	client.secure = nil
	client.mu.Unlock()
}

func (client *Client) setSecureConnection(conn net.Conn, secure *protocol.SecureChannel) {
	client.mu.Lock()
	if client.conn == conn {
		client.secure = secure
	}
	client.mu.Unlock()
}

func (client *Client) clearConnection(conn net.Conn) {
	client.mu.Lock()
	if client.conn == conn {
		client.conn = nil
		client.secure = nil
	}
	client.mu.Unlock()
}

func (client *Client) closeCurrent() {
	client.mu.Lock()
	conn := client.conn
	client.conn = nil
	client.secure = nil
	client.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (client *Client) publishState(change StateChange) {
	client.statusMu.Lock()
	client.state = change.State
	client.statusMu.Unlock()
	select {
	case client.states <- change:
	default:
	}
}

func (client *Client) handshake(ctx context.Context, conn net.Conn) (protocol.Envelope, protocol.HelloPayload, *protocol.SecureChannel, error) {
	deadline := time.Now().Add(client.config.DialTimeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return protocol.Envelope{}, protocol.HelloPayload{}, nil, fmt.Errorf("set Galaxy Helper handshake deadline: %w", err)
	}
	defer conn.SetDeadline(time.Time{})
	helloEnvelope, err := protocol.ReadFrame(conn)
	if err != nil {
		return protocol.Envelope{}, protocol.HelloPayload{}, nil, fmt.Errorf("read Galaxy Helper HELLO: %w", err)
	}
	if helloEnvelope.Type != protocol.TypeHello {
		return protocol.Envelope{}, protocol.HelloPayload{}, nil, fmt.Errorf("%w: first frame is %s", ErrInvalidHandshake, helloEnvelope.Type)
	}
	var hello protocol.HelloPayload
	if err := helloEnvelope.DecodePayload(&hello); err != nil {
		return protocol.Envelope{}, protocol.HelloPayload{}, nil, fmt.Errorf("%w: %v", ErrInvalidHandshake, err)
	}
	if hello.DeviceID == "" || hello.DeviceName == "" {
		return protocol.Envelope{}, protocol.HelloPayload{}, nil, fmt.Errorf("%w: missing device identity", ErrInvalidHandshake)
	}
	var secure *protocol.SecureChannel
	if hello.Auth != nil {
		secure, err = client.authenticate(conn, hello)
		if err != nil {
			return protocol.Envelope{}, protocol.HelloPayload{}, nil, err
		}
	}
	return helloEnvelope, hello, secure, nil
}

func (client *Client) authenticate(conn net.Conn, hello protocol.HelloPayload) (*protocol.SecureChannel, error) {
	if hello.Auth.Mode != "hmac-sha256" {
		return nil, fmt.Errorf("%w: unsupported auth mode", ErrInvalidHandshake)
	}
	challenge, err := base64.RawURLEncoding.DecodeString(hello.Auth.Challenge)
	if err != nil || len(challenge) != 32 {
		return nil, fmt.Errorf("%w: invalid auth challenge", ErrInvalidHandshake)
	}
	if client.config.Credentials == nil {
		return nil, ErrPairingRequired
	}
	credential, err := client.config.Credentials.Load(hello.DeviceID)
	if errors.Is(err, pairing.ErrNotPaired) {
		return nil, ErrPairingRequired
	}
	if err != nil {
		return nil, fmt.Errorf("load Galaxy Helper credential: %w", err)
	}
	requestID, err := newRequestID()
	if err != nil {
		return nil, err
	}
	request, err := protocol.NewEnvelope(protocol.TypeAuth, requestID, 0, protocol.AuthRequestPayload{
		ClientID: credential.ClientID,
		Proof: base64.RawURLEncoding.EncodeToString(
			pairing.Proof(credential.Secret, hello.DeviceID, credential.ClientID, hello.Auth.Challenge),
		),
	})
	if err != nil {
		return nil, err
	}
	if err := client.writeTo(conn, request); err != nil {
		return nil, err
	}
	response, err := protocol.ReadFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("read Galaxy Helper AUTH: %w", err)
	}
	if response.RequestID != requestID {
		return nil, fmt.Errorf("%w: AUTH correlation mismatch", ErrInvalidHandshake)
	}
	if response.Type == protocol.TypeError {
		return nil, ErrAuthentication
	}
	if response.Type != protocol.TypeAuth {
		return nil, fmt.Errorf("%w: expected AUTH response", ErrInvalidHandshake)
	}
	var auth protocol.AuthResponsePayload
	if err := response.DecodePayload(&auth); err != nil || !auth.Authenticated {
		return nil, ErrAuthentication
	}
	if auth.SecureMode != protocol.SecureMode {
		return nil, fmt.Errorf("%w: unsupported secure mode", ErrInvalidHandshake)
	}
	serverProof, err := base64.RawURLEncoding.DecodeString(auth.ServerProof)
	if err != nil || !hmac.Equal(
		serverProof,
		pairing.ServerProof(credential.Secret, hello.DeviceID, credential.ClientID, hello.Auth.Challenge),
	) {
		return nil, ErrAuthentication
	}
	secure, err := protocol.NewSecureChannel(credential.Secret, challenge, protocol.SecureRoleClient)
	if err != nil {
		return nil, fmt.Errorf("configure encrypted Galaxy Helper session: %w", err)
	}
	return secure, nil
}

func (client *Client) publishConnectError(err error) {
	select {
	case client.connectErrors <- err:
	default:
	}
}

func isPermanentConnectionError(err error) bool {
	return errors.Is(err, ErrPairingRequired) || errors.Is(err, ErrAuthentication) || errors.Is(err, ErrInvalidHandshake)
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func newRequestID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("create protocol request ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
