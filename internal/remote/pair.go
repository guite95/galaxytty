package remote

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/galaxytty/galaxytty/internal/pairing"
	"github.com/galaxytty/galaxytty/internal/protocol"
)

type PairResult struct {
	DeviceID   string
	DeviceName string
}

func Pair(
	ctx context.Context,
	config Config,
	resolver AddressResolver,
	code string,
	store pairing.Store,
) (PairResult, error) {
	if store == nil {
		return PairResult{}, errors.New("Galaxy Helper credential store is required")
	}
	secret, err := pairing.ParseCode(code)
	if err != nil {
		return PairResult{}, err
	}
	config = config.withDefaults()
	address := config.Address
	if address == "" {
		if resolver == nil {
			return PairResult{}, errors.New("Galaxy Helper resolver is required")
		}
		address, err = resolver.Resolve(ctx)
		if err != nil {
			return PairResult{}, err
		}
	}
	dialer := &net.Dialer{Timeout: config.DialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return PairResult{}, fmt.Errorf("connect to Galaxy Helper for pairing: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(config.DialTimeout)); err != nil {
		return PairResult{}, fmt.Errorf("set pairing deadline: %w", err)
	}

	helloEnvelope, err := protocol.ReadFrame(conn)
	if err != nil {
		return PairResult{}, fmt.Errorf("read Galaxy Helper pairing HELLO: %w", err)
	}
	if helloEnvelope.Type != protocol.TypeHello {
		return PairResult{}, fmt.Errorf("%w: first frame is %s", ErrInvalidHandshake, helloEnvelope.Type)
	}
	var hello protocol.HelloPayload
	if err := helloEnvelope.DecodePayload(&hello); err != nil {
		return PairResult{}, fmt.Errorf("%w: %v", ErrInvalidHandshake, err)
	}
	if hello.DeviceID == "" || hello.DeviceName == "" || hello.Auth == nil || hello.Auth.Mode != "hmac-sha256" {
		return PairResult{}, fmt.Errorf("%w: Helper does not offer supported pairing", ErrInvalidHandshake)
	}
	challenge, err := base64.RawURLEncoding.DecodeString(hello.Auth.Challenge)
	if err != nil || len(challenge) != 32 {
		return PairResult{}, fmt.Errorf("%w: invalid auth challenge", ErrInvalidHandshake)
	}
	clientID, err := pairing.NewClientID()
	if err != nil {
		return PairResult{}, err
	}
	requestID, err := newRequestID()
	if err != nil {
		return PairResult{}, err
	}
	request, err := protocol.NewEnvelope(protocol.TypeAuth, requestID, 0, protocol.AuthRequestPayload{
		ClientID: clientID,
		Proof: base64.RawURLEncoding.EncodeToString(
			pairing.Proof(secret, hello.DeviceID, clientID, hello.Auth.Challenge),
		),
	})
	if err != nil {
		return PairResult{}, err
	}
	if err := protocol.WriteFrame(conn, request); err != nil {
		return PairResult{}, fmt.Errorf("write Galaxy Helper pairing proof: %w", err)
	}
	response, err := protocol.ReadFrame(conn)
	if err != nil {
		return PairResult{}, fmt.Errorf("read Galaxy Helper pairing response: %w", err)
	}
	if response.RequestID != requestID {
		return PairResult{}, fmt.Errorf("%w: AUTH correlation mismatch", ErrInvalidHandshake)
	}
	if response.Type == protocol.TypeError {
		return PairResult{}, ErrAuthentication
	}
	if response.Type != protocol.TypeAuth {
		return PairResult{}, fmt.Errorf("%w: expected AUTH response", ErrInvalidHandshake)
	}
	var auth protocol.AuthResponsePayload
	if err := response.DecodePayload(&auth); err != nil || !auth.Authenticated {
		return PairResult{}, ErrAuthentication
	}
	if auth.SecureMode != protocol.SecureMode {
		return PairResult{}, fmt.Errorf("%w: unsupported secure mode", ErrInvalidHandshake)
	}
	serverProof, err := base64.RawURLEncoding.DecodeString(auth.ServerProof)
	if err != nil || !hmac.Equal(serverProof, pairing.ServerProof(secret, hello.DeviceID, clientID, hello.Auth.Challenge)) {
		return PairResult{}, ErrAuthentication
	}
	if err := store.Save(pairing.Credential{
		DeviceID: hello.DeviceID, DeviceName: hello.DeviceName,
		ClientID: clientID, Secret: secret,
	}); err != nil {
		return PairResult{}, fmt.Errorf("save Galaxy Helper credential: %w", err)
	}
	return PairResult{DeviceID: hello.DeviceID, DeviceName: hello.DeviceName}, nil
}
