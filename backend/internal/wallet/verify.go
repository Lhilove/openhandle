package wallet

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/mr-tron/base58"
)

// ErrBadAddress indicates the supplied address is not a 32-byte ed25519 key.
var ErrBadAddress = errors.New("wallet address is not a valid ed25519 public key")

// VerifySignature verifies that signature (base58) is a valid ed25519 signature
// over messageBytes, produced by the private key corresponding to address
// (base58-encoded 32-byte public key).
//
// The library used is crypto/ed25519 from the standard library; we do not
// implement any cryptography ourselves.
func VerifySignature(address string, messageBytes []byte, signature string) error {
	pub, err := base58.Decode(address)
	if err != nil {
		return ErrBadAddress
	}
	if len(pub) != ed25519.PublicKeySize {
		return ErrBadAddress
	}
	sig, err := base58.Decode(signature)
	if err != nil {
		return fmt.Errorf("signature is not valid base58: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("signature has wrong length: %d", len(sig))
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), messageBytes, sig) {
		return errors.New("signature verification failed")
	}
	return nil
}
