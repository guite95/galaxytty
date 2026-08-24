package pairing

import (
	"encoding/hex"
	"testing"
)

func TestPairingCodeRoundTripAndFormatting(t *testing.T) {
	secret := make([]byte, CredentialSize)
	for index := range secret {
		secret[index] = byte(index)
	}
	formatted := FormatCode(secret)
	if formatted != "AAAQ-EAYE-AUDA-OCAJ-BIFQ-YDIO-B4IB-CEQT" {
		t.Fatalf("formatted=%q", formatted)
	}
	parsed, err := ParseCode("aaaq eaye-auda-ocaj-bifq-ydio-b4ib-ceqt")
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(parsed) != hex.EncodeToString(secret) {
		t.Fatalf("parsed=%x want=%x", parsed, secret)
	}
}

func TestPairingCodeRejectsWrongLengthAndAlphabet(t *testing.T) {
	for _, input := range []string{"short", "AAAQ-EAYE-AUDA-OCAJ-BIFQ-YDIO-B4IB-CEQ!"} {
		if _, err := ParseCode(input); err == nil {
			t.Fatalf("input %q should fail", input)
		}
	}
}

func TestProofUsesStableDomainSeparatedTranscript(t *testing.T) {
	secret := make([]byte, CredentialSize)
	for index := range secret {
		secret[index] = byte(index)
	}
	proof := Proof(secret, "device-1", "client-1", "challenge-1")
	if got := hex.EncodeToString(proof); got != "0cba84724a158c5f22774a19bc907252d9482980a7f0d77ac2f87a428fd228e4" {
		t.Fatalf("proof=%s", got)
	}
	serverProof := ServerProof(secret, "device-1", "client-1", "challenge-1")
	if got := hex.EncodeToString(serverProof); got != "d0a7cda23a4f9203374e49ae9614da85d76ec19641dbc706df78dda42210853b" {
		t.Fatalf("server proof=%s", got)
	}
}
