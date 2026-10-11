package validate

import (
	"errors"
	"fmt"
	"strings"
)

// Handle rules (documented in the API docs and UI):
//   - normalized to lowercase ASCII
//   - length 3..20
//   - characters a-z, 0-9, and single internal underscores
//   - must start and end with an alphanumeric character
//   - no consecutive underscores
const (
	HandleMinLen = 3
	HandleMaxLen = 20
)

// ErrHandleInvalid is the sentinel for a malformed handle.
var ErrHandleInvalid = errors.New("invalid handle")

// reserved contains system/route names that must never be claimable.
var reserved = map[string]struct{}{}

func init() {
	for _, r := range []string{
		"admin", "administrator", "support", "security", "api", "help", "root",
		"system", "sys", "me", "you", "settings", "login", "logout", "register",
		"signup", "signin", "signout", "app", "docs", "doc", "about", "pricing",
		"terms", "privacy", "legal", "blog", "news", "status", "health", "healthz",
		"readyz", "explore", "how-it-works", "howitworks", "developers", "developer",
		"wallet", "wallets", "payment", "payments", "pay", "send", "receive",
		"identity", "identities", "profile", "profiles", "handle", "handles",
		"openhandle", "www", "mail", "email", "ftp", "cdn", "static", "assets",
		"public", "private", "null", "undefined", "true", "false", "anonymous",
		"owner", "mod", "moderator", "staff", "team", "official", "verify", "verified",
	} {
		reserved[r] = struct{}{}
	}
}

// NormalizeHandle lowercases and trims a raw handle string.
func NormalizeHandle(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// IsReserved reports whether a normalized handle is reserved.
func IsReserved(handle string) bool {
	_, ok := reserved[handle]
	return ok
}

// ValidateHandle normalizes and validates a handle, returning the normalized
// value or a descriptive error.
func ValidateHandle(raw string) (string, error) {
	h := NormalizeHandle(raw)
	if h == "" {
		return "", fmt.Errorf("%w: handle is required", ErrHandleInvalid)
	}
	if len(h) < HandleMinLen || len(h) > HandleMaxLen {
		return "", fmt.Errorf("%w: handle must be between %d and %d characters", ErrHandleInvalid, HandleMinLen, HandleMaxLen)
	}
	if strings.HasPrefix(h, "_") || strings.HasSuffix(h, "_") {
		return "", fmt.Errorf("%w: handle must start and end with a letter or number", ErrHandleInvalid)
	}
	if strings.Contains(h, "__") {
		return "", fmt.Errorf("%w: handle cannot contain consecutive underscores", ErrHandleInvalid)
	}
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return "", fmt.Errorf("%w: only lowercase letters, numbers, and underscores are allowed", ErrHandleInvalid)
		}
	}
	if IsReserved(h) {
		return "", fmt.Errorf("%w: this handle is reserved", ErrHandleInvalid)
	}
	return h, nil
}

// ValidateDisplayName validates an optional display name (<= 60 chars).
func ValidateDisplayName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if len([]rune(n)) > 60 {
		return "", errors.New("display name must be 60 characters or fewer")
	}
	return n, nil
}

// ValidateBio validates an optional bio (<= 280 chars).
func ValidateBio(bio string) (string, error) {
	b := strings.TrimSpace(bio)
	if len([]rune(b)) > 280 {
		return "", errors.New("bio must be 280 characters or fewer")
	}
	return b, nil
}
