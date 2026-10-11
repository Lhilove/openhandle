package wallet

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/mr-tron/base58"
)

func TestVerifySignature_RoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	addr := base58.Encode(pub)
	msg := []byte("OpenHandle wallet verification\n\nNonce: abc123")
	sig := base58.Encode(ed25519.Sign(priv, msg))

	if err := VerifySignature(addr, msg, sig); err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}
}

func TestVerifySignature_TamperedMessage(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	addr := base58.Encode(pub)
	msg := []byte("original message")
	sig := base58.Encode(ed25519.Sign(priv, msg))

	if err := VerifySignature(addr, []byte("tampered message"), sig); err == nil {
		t.Fatal("expected verification to fail for a tampered message")
	}
}

func TestVerifySignature_WrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	msg := []byte("hello")
	sig := base58.Encode(ed25519.Sign(priv, msg))

	if err := VerifySignature(base58.Encode(otherPub), msg, sig); err == nil {
		t.Fatal("expected verification to fail for a mismatched key")
	}
}

func TestVerifySignature_BadAddress(t *testing.T) {
	if err := VerifySignature("not-base58!!!", []byte("x"), "sig"); err == nil {
		t.Fatal("expected error for invalid address")
	}
}
