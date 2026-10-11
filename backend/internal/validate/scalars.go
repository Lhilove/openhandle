package validate

import (
	"errors"
	"strings"
	"unicode"

	"github.com/mr-tron/base58"
)

// ValidateEmail performs a pragmatic structural email check. Real deliverability
// cannot be verified without sending mail; this only rejects obviously invalid
// input and normalizes to lowercase.
func ValidateEmail(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	if e == "" {
		return "", errors.New("email is required")
	}
	if len(e) > 254 {
		return "", errors.New("email is too long")
	}
	at := strings.LastIndex(e, "@")
	if at <= 0 || at == len(e)-1 {
		return "", errors.New("email is not valid")
	}
	local, domain := e[:at], e[at+1:]
	if strings.ContainsAny(local, " \t") || len(local) > 64 {
		return "", errors.New("email is not valid")
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", errors.New("email is not valid")
	}
	for _, r := range domain {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-') {
			return "", errors.New("email is not valid")
		}
	}
	return e, nil
}

// ValidatePassword enforces a conservative minimum. Maximum avoids bcrypt's
// 72-byte truncation surprise.
func ValidatePassword(pw string) error {
	if len(pw) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(pw) > 72 {
		return errors.New("password must be at most 72 characters")
	}
	return nil
}

// IsValidSolanaAddress verifies that s decodes to a 32-byte ed25519 public key,
// which is the canonical shape of a Solana wallet address.
func IsValidSolanaAddress(s string) bool {
	b, err := base58.Decode(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return len(b) == 32
}
