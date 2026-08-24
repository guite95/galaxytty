package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
)

const SecureMode = "aes-256-gcm-hkdf-sha256"

const (
	secureDomain       = "galaxytty-secure-v1"
	secureKeyBytes     = 32
	secureNoncePrefix  = 4
	secureChallengeLen = 32
	maxSecurePlaintext = int(MaxFrameSize)*3/4 - 1024
)

var (
	ErrSecureFrameExpected    = errors.New("encrypted Galaxy Helper frame required")
	ErrSecureCounter          = errors.New("invalid encrypted frame counter")
	ErrSecureAuthentication   = errors.New("encrypted frame authentication failed")
	ErrSecureCounterExhausted = errors.New("encrypted frame counter exhausted")
)

type SecureRole int

const (
	SecureRoleClient SecureRole = iota
	SecureRoleServer
)

type securePayload struct {
	Counter    uint64 `json:"counter"`
	Ciphertext string `json:"ciphertext"`
}

type sessionMaterial struct {
	clientToServerKey   []byte
	serverToClientKey   []byte
	clientToServerNonce []byte
	serverToClientNonce []byte
}

type SecureChannel struct {
	sendAEAD         cipher.AEAD
	receiveAEAD      cipher.AEAD
	sendPrefix       []byte
	receivePrefix    []byte
	sendDirection    string
	receiveDirection string

	sendMu         sync.Mutex
	receiveMu      sync.Mutex
	sendCounter    uint64
	receiveCounter uint64
}

func NewSecureChannel(secret, challenge []byte, role SecureRole) (*SecureChannel, error) {
	material, err := deriveSessionMaterial(secret, challenge)
	if err != nil {
		return nil, err
	}
	var sendKey, receiveKey, sendPrefix, receivePrefix []byte
	var sendDirection, receiveDirection string
	switch role {
	case SecureRoleClient:
		sendKey, receiveKey = material.clientToServerKey, material.serverToClientKey
		sendPrefix, receivePrefix = material.clientToServerNonce, material.serverToClientNonce
		sendDirection, receiveDirection = "client-to-server", "server-to-client"
	case SecureRoleServer:
		sendKey, receiveKey = material.serverToClientKey, material.clientToServerKey
		sendPrefix, receivePrefix = material.serverToClientNonce, material.clientToServerNonce
		sendDirection, receiveDirection = "server-to-client", "client-to-server"
	default:
		return nil, errors.New("invalid secure channel role")
	}
	sendAEAD, err := newGCM(sendKey)
	if err != nil {
		return nil, err
	}
	receiveAEAD, err := newGCM(receiveKey)
	if err != nil {
		return nil, err
	}
	return &SecureChannel{
		sendAEAD: sendAEAD, receiveAEAD: receiveAEAD,
		sendPrefix: append([]byte(nil), sendPrefix...), receivePrefix: append([]byte(nil), receivePrefix...),
		sendDirection: sendDirection, receiveDirection: receiveDirection,
	}, nil
}

func (channel *SecureChannel) Write(writer io.Writer, envelope Envelope) error {
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("validate encrypted envelope: %w", err)
	}
	plaintext, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal encrypted envelope: %w", err)
	}
	if len(plaintext) > maxSecurePlaintext {
		return ErrFrameTooLarge
	}
	channel.sendMu.Lock()
	defer channel.sendMu.Unlock()
	if channel.sendCounter == math.MaxUint64 {
		return ErrSecureCounterExhausted
	}
	channel.sendCounter++
	counter := channel.sendCounter
	ciphertext := channel.sendAEAD.Seal(nil, nonce(channel.sendPrefix, counter), plaintext, additionalData(channel.sendDirection, counter))
	outer, err := NewEnvelope(TypeSecure, "", 0, securePayload{
		Counter: counter, Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		return err
	}
	return WriteFrame(writer, outer)
}

func (channel *SecureChannel) Read(reader io.Reader) (Envelope, error) {
	channel.receiveMu.Lock()
	defer channel.receiveMu.Unlock()
	outer, err := ReadFrame(reader)
	if err != nil {
		return Envelope{}, err
	}
	if outer.Type != TypeSecure {
		return Envelope{}, ErrSecureFrameExpected
	}
	var payload securePayload
	if err := outer.DecodePayload(&payload); err != nil {
		return Envelope{}, err
	}
	if payload.Counter == 0 || channel.receiveCounter == math.MaxUint64 || payload.Counter != channel.receiveCounter+1 {
		return Envelope{}, fmt.Errorf("%w: received=%d expected=%d", ErrSecureCounter, payload.Counter, channel.receiveCounter+1)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return Envelope{}, ErrSecureAuthentication
	}
	plaintext, err := channel.receiveAEAD.Open(
		nil,
		nonce(channel.receivePrefix, payload.Counter),
		ciphertext,
		additionalData(channel.receiveDirection, payload.Counter),
	)
	if err != nil {
		return Envelope{}, ErrSecureAuthentication
	}
	var envelope Envelope
	if err := json.Unmarshal(plaintext, &envelope); err != nil {
		return Envelope{}, ErrSecureAuthentication
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, ErrSecureAuthentication
	}
	if envelope.Type == TypeSecure {
		return Envelope{}, ErrSecureAuthentication
	}
	channel.receiveCounter = payload.Counter
	return envelope, nil
}

func deriveSessionMaterial(secret, challenge []byte) (sessionMaterial, error) {
	if len(secret) < 16 {
		return sessionMaterial{}, errors.New("secure channel credential is too short")
	}
	if len(challenge) != secureChallengeLen {
		return sessionMaterial{}, errors.New("secure channel challenge must be 32 bytes")
	}
	prk := hmacSHA256(challenge, secret)
	return sessionMaterial{
		clientToServerKey:   hkdfExpand(prk, secureDomain+"/client-to-server/key", secureKeyBytes),
		serverToClientKey:   hkdfExpand(prk, secureDomain+"/server-to-client/key", secureKeyBytes),
		clientToServerNonce: hkdfExpand(prk, secureDomain+"/client-to-server/nonce", secureNoncePrefix),
		serverToClientNonce: hkdfExpand(prk, secureDomain+"/server-to-client/nonce", secureNoncePrefix),
	}, nil
}

func hkdfExpand(prk []byte, info string, length int) []byte {
	result := make([]byte, 0, length)
	var previous []byte
	for counter := byte(1); len(result) < length; counter++ {
		mac := hmac.New(sha256.New, prk)
		_, _ = mac.Write(previous)
		_, _ = mac.Write([]byte(info))
		_, _ = mac.Write([]byte{counter})
		previous = mac.Sum(nil)
		result = append(result, previous...)
	}
	return result[:length]
}

func hmacSHA256(key, value []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(value)
	return mac.Sum(nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func nonce(prefix []byte, counter uint64) []byte {
	value := make([]byte, secureNoncePrefix+8)
	copy(value, prefix)
	binary.BigEndian.PutUint64(value[secureNoncePrefix:], counter)
	return value
}

func additionalData(direction string, counter uint64) []byte {
	prefix := []byte(secureDomain + "\x00" + direction + "\x00")
	value := make([]byte, len(prefix)+8)
	copy(value, prefix)
	binary.BigEndian.PutUint64(value[len(prefix):], counter)
	return value
}
