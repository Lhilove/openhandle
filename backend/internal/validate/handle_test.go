package validate

import "testing"

func TestValidateHandle_Normalizes(t *testing.T) {
	cases := map[string]string{
		"Love":      "love",
		"  alice  ": "alice",
		"a_b_c":     "a_b_c",
		"user123":   "user123",
		"LOVE_99":   "love_99",
	}
	for in, want := range cases {
		got, err := ValidateHandle(in)
		if err != nil {
			t.Fatalf("ValidateHandle(%q) error: %v", in, err)
		}
		if got != want {
			t.Fatalf("ValidateHandle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateHandle_Rejects(t *testing.T) {
	bad := []string{
		"",                            // empty
		"ab",                          // too short
		"this_handle_is_way_too_long", // too long
		"_start",                      // leading underscore
		"end_",                        // trailing underscore
		"dou__ble",                    // consecutive underscores
		"has space",                   // space
		"has-dash",                    // hyphen
		"café",                        // non-ascii
		"admin",                       // reserved
		"support",                     // reserved
		"api",                         // reserved
		"root",                        // reserved
		"settings",                    // reserved route name
	}
	for _, in := range bad {
		if _, err := ValidateHandle(in); err == nil {
			t.Fatalf("ValidateHandle(%q) unexpectedly succeeded", in)
		}
	}
}

func TestIsReserved_CaseInsensitiveAfterNormalize(t *testing.T) {
	if !IsReserved(NormalizeHandle("ADMIN")) {
		t.Fatal("expected ADMIN to normalize to a reserved handle")
	}
	if !IsReserved(NormalizeHandle("  Support ")) {
		t.Fatal("expected Support to normalize to a reserved handle")
	}
	if IsReserved("alice") {
		t.Fatal("alice should not be reserved")
	}
}

func TestValidateEmail(t *testing.T) {
	good := []string{"a@b.co", "User@Example.COM "}
	for _, e := range good {
		if _, err := ValidateEmail(e); err != nil {
			t.Fatalf("ValidateEmail(%q) unexpected error: %v", e, err)
		}
	}
	bad := []string{"", "no-at", "a@b", "a@.com", "a@b.", "@b.com", "a b@c.com"}
	for _, e := range bad {
		if _, err := ValidateEmail(e); err == nil {
			t.Fatalf("ValidateEmail(%q) unexpectedly succeeded", e)
		}
	}
}

func TestIsValidSolanaAddress(t *testing.T) {
	// A real base58 ed25519 public key (32 bytes) from devnet docs.
	if !IsValidSolanaAddress("11111111111111111111111111111111") {
		t.Fatal("expected system program id (32 zero bytes) to be a valid address")
	}
	if IsValidSolanaAddress("not-a-real-address") {
		t.Fatal("expected bogus address to be rejected")
	}
}
