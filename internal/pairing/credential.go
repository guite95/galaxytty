package pairing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
)

const CredentialSize = 20

const transcriptDomain = "galaxytty-auth-v1"
const serverTranscriptDomain = "galaxytty-auth-server-v1"

var codeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func FormatCode(secret []byte) string {
	encoded := codeEncoding.EncodeToString(secret)
	groups := make([]string, 0, (len(encoded)+3)/4)
	for len(encoded) > 0 {
		width := min(4, len(encoded))
		groups = append(groups, encoded[:width])
		encoded = encoded[width:]
	}
	return strings.Join(groups, "-")
}

func ParseCode(value string) ([]byte, error) {
	normalized := strings.NewReplacer("-", "", " ", "", "\t", "", "\r", "", "\n", "").Replace(strings.ToUpper(value))
	decoded, err := codeEncoding.DecodeString(normalized)
	if err != nil {
		return nil, errors.New("pairing code contains invalid characters")
	}
	if len(decoded) != CredentialSize {
		return nil, fmt.Errorf("pairing code must contain %d base32 characters", codeEncoding.EncodedLen(CredentialSize))
	}
	return decoded, nil
}

func Proof(secret []byte, deviceID, clientID, challenge string) []byte {
	return proof(secret, transcriptDomain, deviceID, clientID, challenge)
}

func ServerProof(secret []byte, deviceID, clientID, challenge string) []byte {
	return proof(secret, serverTranscriptDomain, deviceID, clientID, challenge)
}

func proof(secret []byte, domain, deviceID, clientID, challenge string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(transcript(domain, deviceID, clientID, challenge)))
	return mac.Sum(nil)
}

func transcript(domain, deviceID, clientID, challenge string) string {
	return strings.Join([]string{domain, deviceID, clientID, challenge}, "\x00")
}
